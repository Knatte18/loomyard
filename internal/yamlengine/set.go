// set.go implements value-preserving single/multi-key YAML mutation for the
// non-interactive `lyx config <module> --set key=value` path. Unlike Reconcile
// (which merges an entire existing file into a template), SetValues applies a
// small, explicit list of key=value pairs while still routing every write
// through the template-shaped working tree so partial/stale existing files
// never hide a valid key behind a missing node. It also grafts any existing
// top-level key absent from the template onto the working tree whole, at
// root-key granularity, so a hand-edited or template-outgrown key is carried
// through into the merged output instead of silently vanishing.

package yamlengine

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// preservedKeyComment marks a root-level key that SetValues grafted from
// existing onto templateNode because the template no longer (or never did)
// declare it. It is always set via direct assignment, never concatenated
// onto whatever HeadComment the key already carried, so repeat --set calls
// against an already-preserved file stay idempotent instead of growing or
// duplicating the comment.
const preservedKeyComment = "# preserved (not in current template)"

// KV is a single key=value pair to apply via SetValues. Key is a dotted
// leaf key-path (the same shape collectLeafPaths produces, e.g.
// "level1.level2.key"); Value is the raw string to store as the leaf's
// scalar value.
type KV struct {
	Key   string
	Value string
}

// SetResult is the outcome of a SetValues call.
//
// Merged holds the new file bytes and is only valid when Unknown is empty;
// callers must not write Merged to disk otherwise. Unknown is the sorted,
// deduplicated list of requested keys absent from the template's leaf-key
// set. Known is the template's full sorted leaf-key set, included so callers
// can build a helpful "known keys are..." error message without recomputing it.
// Preserved is the sorted list of pre-existing top-level config keys not
// present in the template that were carried through into Merged untouched
// (nil/empty when none).
type SetResult struct {
	Merged    []byte
	Unknown   []string
	Known     []string
	Preserved []string
}

// SetValues applies pairs to a template-shaped YAML document, preserving
// comments, key order, and any values from existing that already agree with
// the template's structure.
//
// The working tree mutated and marshalled is always templateNode, never a
// bare parse of existing: this guarantees every template leaf has a real,
// settable node even when existing is a stale or partial file missing some of
// the template's keys. When existing is non-empty its leaf values are first
// copied onto the matching templateNode leaves (mirroring Reconcile's merge
// step), so a --set call layers on top of whatever the user already
// customized rather than clobbering it back to the template defaults.
//
// When existing is non-empty, SetValues also grafts any of existing's
// top-level keys that have no counterpart in the template's top-level keys
// onto templateNode's root mapping, whole (scalar, mapping, or sequence,
// unmodified) and in sorted key order, marking each grafted key with the
// fixed comment "# preserved (not in current template)". This is a
// root-key-granularity operation independent of the leaf-path override step
// above: it exists so a key the template has outgrown, or one a user hand-
// added, is never silently dropped just because SetValues always marshals
// from templateNode rather than existingNode. The grafted key names are
// reported in SetResult.Preserved.
//
// If any pairs[i].Key is not present in the template's leaf-key set, no
// mutation is performed at all: SetResult.Unknown is returned non-empty and
// Merged is nil. Otherwise every pair is applied to the working tree in the
// given order (a later pair for a repeated key wins) and the mutated tree is
// marshalled into SetResult.Merged.
func SetValues(template, existing []byte, pairs []KV) (SetResult, error) {
	// Parse the template into the tree we will mutate and ultimately marshal.
	var templateNode yaml.Node
	if err := yaml.Unmarshal(template, &templateNode); err != nil {
		return SetResult{}, err
	}

	templateLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&templateNode, templateLeaves)

	known := make([]string, 0, len(templateLeaves))
	for path := range templateLeaves {
		known = append(known, path)
	}
	sort.Strings(known)

	// preserved collects the root-key-granularity graft step's key names
	// (below), reported to the caller via SetResult.Preserved. It stays nil
	// when existing is empty, since there is nothing on disk to preserve.
	var preserved []string

	// Layer existing's values onto the template working tree via the same
	// applyExistingOverrides helper Reconcile uses, so a --set call preserves
	// whatever the user already customized rather than resetting untouched
	// keys back to defaults.
	if len(existing) > 0 {
		var existingNode yaml.Node
		if err := yaml.Unmarshal(existing, &existingNode); err != nil {
			return SetResult{}, err
		}
		existingLeaves := make(map[string]*yaml.Node)
		collectLeafPaths(&existingNode, existingLeaves)

		applyExistingOverrides(templateLeaves, existingLeaves)

		// Graft any of existing's top-level keys with no counterpart in the
		// template onto templateNode's root mapping, whole. This is a
		// root-key-granularity operation independent of the leaf-path
		// override above: a key the template has outgrown, or one the user
		// hand-added, must never vanish just because SetValues always
		// marshals from templateNode rather than existingNode.
		preserved = preserveOrphanRootKeys(&templateNode, &existingNode)
	}

	// Validate every requested key against the template's leaf set before
	// mutating anything, so a single unknown key rejects the whole call
	// rather than silently applying a partial write.
	unknownSet := make(map[string]bool)
	for _, pair := range pairs {
		if _, ok := templateLeaves[pair.Key]; !ok {
			unknownSet[pair.Key] = true
		}
	}
	if len(unknownSet) > 0 {
		unknown := make([]string, 0, len(unknownSet))
		for key := range unknownSet {
			unknown = append(unknown, key)
		}
		sort.Strings(unknown)
		return SetResult{Unknown: unknown, Known: known}, nil
	}

	// Every key is now guaranteed to have a real node in templateNode, since
	// the working tree always contains every template leaf. Apply pairs in
	// order so a repeated key's later value wins.
	for _, pair := range pairs {
		templateLeaves[pair.Key].Value = pair.Value
	}

	merged, err := yaml.Marshal(&templateNode)
	if err != nil {
		return SetResult{}, err
	}

	return SetResult{Merged: merged, Known: known, Preserved: preserved}, nil
}

// preserveOrphanRootKeys grafts every top-level key in existingNode's root
// mapping that has no counterpart in templateNode's root mapping onto
// templateNode's root mapping, whole (scalar, mapping, or sequence,
// unmodified), in sorted key order, marking each grafted key with
// preservedKeyComment. It returns the sorted list of grafted key names
// (nil when none), for SetResult.Preserved.
//
// This is a root-key-granularity operation, deliberately separate from
// applyExistingOverrides' flattened-leaf-path comparison: comparing at the
// root avoids any need to special-case nested or indexed orphan structures,
// since the whole subtree under an orphaned root key is grafted as-is.
func preserveOrphanRootKeys(templateNode, existingNode *yaml.Node) []string {
	templateRoot := rootMappingNode(templateNode)
	existingRoot := rootMappingNode(existingNode)
	if templateRoot == nil || existingRoot == nil {
		return nil
	}

	// Build the template's top-level key-name set to test existing's
	// top-level keys against.
	templateKeys := make(map[string]bool)
	for i := 0; i+1 < len(templateRoot.Content); i += 2 {
		templateKeys[templateRoot.Content[i].Value] = true
	}

	// orphan pairs a top-level key name with its key/value node pair, kept
	// together so the whole pair can be appended to templateRoot.Content and
	// sorted by name before appending.
	type orphan struct {
		name    string
		keyNode *yaml.Node
		valNode *yaml.Node
	}
	var orphans []orphan
	for i := 0; i+1 < len(existingRoot.Content); i += 2 {
		keyNode := existingRoot.Content[i]
		valNode := existingRoot.Content[i+1]
		if templateKeys[keyNode.Value] {
			continue
		}
		orphans = append(orphans, orphan{name: keyNode.Value, keyNode: keyNode, valNode: valNode})
	}
	if len(orphans) == 0 {
		return nil
	}

	sort.Slice(orphans, func(i, j int) bool { return orphans[i].name < orphans[j].name })

	preserved := make([]string, 0, len(orphans))
	for _, o := range orphans {
		// Direct assignment, never concatenation: this must overwrite
		// whatever HeadComment the key carried in existing (including a
		// preservedKeyComment from a prior --set run) so repeat calls stay
		// idempotent instead of duplicating or growing the comment.
		o.keyNode.HeadComment = preservedKeyComment
		templateRoot.Content = append(templateRoot.Content, o.keyNode, o.valNode)
		preserved = append(preserved, o.name)
	}
	return preserved
}

// rootMappingNode unwraps a parsed yaml.Node down to its root MappingNode,
// mirroring how collectLeafPathsHelper's yaml.DocumentNode case descends
// into Content[0]. A yaml.Unmarshal target is always a DocumentNode whose
// single child is the document's root node; for a non-empty mapping
// document that child is the MappingNode itself.
func rootMappingNode(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}
