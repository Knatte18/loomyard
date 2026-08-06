// set.go implements value-preserving single/multi-key YAML mutation for the non-interactive `lyx
// config <module> --set key=value` path.
// Unlike Reconcile (which merges an entire existing file into a template), SetValues applies a
// small, explicit list of key=value pairs while still routing every write through the
// template-shaped working tree so partial/stale existing files never hide a valid key behind a
// missing node.
// It also grafts any existing top-level key absent from the template onto the working tree whole,
// at root-key granularity, so a hand-edited or template-outgrown key is carried through into the
// merged output instead of silently vanishing.

package yamlengine

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// preservedKeyComment marks a root-level key grafted from existing because
// the template no longer declares it. Direct assignment keeps --set calls
// idempotent.
const preservedKeyComment = "# preserved (not in current template)"

// KV is a single key=value pair.
// Key is a dotted leaf key-path;
// Value is the scalar value.
type KV struct {
	Key   string
	Value string
}

// SetResult is the outcome of a SetValues call.
// Merged holds the new file bytes and is only valid when Unknown is empty.
// Unknown lists requested keys absent from the template's leaf-key set.
// Known is the template's full sorted leaf-key set.
// Preserved lists pre-existing top-level keys carried through (nil/empty when none).
type SetResult struct {
	Merged    []byte
	Unknown   []string
	Known     []string
	Preserved []string
}

// SetValues applies pairs to a template-shaped YAML document, preserving comments and key order.
// If any pairs[i].Key is absent from the template's leaf-key set, SetResult.Unknown is returned
// non-empty and Merged is nil.
// Otherwise every pair is applied to the working tree (later pairs for a repeated key win) and the
// mutated tree is marshalled into SetResult.Merged.
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
// templateNode, in sorted key order, marking each with preservedKeyComment.
// It returns the sorted list of grafted key names (nil when none).
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

// rootMappingNode unwraps a parsed yaml.Node down to its root MappingNode.
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
