// set.go implements value-preserving single/multi-key YAML mutation for the
// non-interactive `lyx config <module> --set key=value` path. Unlike Reconcile
// (which merges an entire existing file into a template), SetValues applies a
// small, explicit list of key=value pairs while still routing every write
// through the template-shaped working tree so partial/stale existing files
// never hide a valid key behind a missing node.

package yamlengine

import (
	"sort"

	"gopkg.in/yaml.v3"
)

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
type SetResult struct {
	Merged  []byte
	Unknown []string
	Known   []string
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

	// Layer existing's values onto the template working tree, exactly as
	// Reconcile does, so a --set call preserves whatever the user already
	// customized rather than resetting untouched keys back to defaults.
	if len(existing) > 0 {
		var existingNode yaml.Node
		if err := yaml.Unmarshal(existing, &existingNode); err != nil {
			return SetResult{}, err
		}
		existingLeaves := make(map[string]*yaml.Node)
		collectLeafPaths(&existingNode, existingLeaves)

		for path, existingLeaf := range existingLeaves {
			if templateLeaf, ok := templateLeaves[path]; ok {
				templateLeaf.Value = existingLeaf.Value
				templateLeaf.Tag = existingLeaf.Tag
				templateLeaf.Style = existingLeaf.Style
			}
		}
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

	return SetResult{Merged: merged, Known: known}, nil
}
