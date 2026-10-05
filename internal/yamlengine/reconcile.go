// reconcile.go implements config reconciliation and missing-key detection.
// It merges a template with existing user configuration while preserving the template structure and
// user values.

package yamlengine

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reconcile merges a template with existing user configuration, preserving template comments and
// key order.
// Keys present in existing override template defaults;
// keys absent from existing are reported in added, keys absent from template are reported in
// removed.
// Reconcile is idempotent.
//
// Each openMaps entry is a dotted key path whose value is an open map.
// When existing holds the key, its value replaces the template's whole, whatever its kind, and nothing at or under the path is reported added or removed.
// When existing lacks the key, the template's value stays and added reports the path itself.
func Reconcile(template, existing []byte, openMaps ...string) (merged []byte, added, removed []string, err error) {
	// Parse template into node tree
	var templateNode yaml.Node
	if parseErr := yaml.Unmarshal(template, &templateNode); parseErr != nil {
		return nil, nil, nil, fmt.Errorf("parse template YAML: %w", parseErr)
	}

	// Parse existing into node tree (empty/nil existing yields empty document)
	var existingNode yaml.Node
	if len(strings.TrimSpace(string(existing))) == 0 {
		existingNode.Kind = yaml.DocumentNode
		existingNode.Content = []*yaml.Node{}
	} else {
		if parseErr := yaml.Unmarshal(existing, &existingNode); parseErr != nil {
			return nil, nil, nil, fmt.Errorf("parse existing YAML: %w", parseErr)
		}
	}

	// A declared open map present in existing is carried whole before any leaf is collected, so both trees agree under it.
	// One existing lacks is reported as the path itself, below.
	absentOpenMaps := carryOpenMaps(&templateNode, &existingNode, openMaps)

	// Collect all leaf key-paths from the template
	templateLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&templateNode, templateLeaves)

	// Collect all leaf key-paths from existing
	existingLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&existingNode, existingLeaves)

	templateSequences := make(map[string]*yaml.Node)
	collectSequencePaths(&templateNode, "", templateSequences)
	existingSequences := make(map[string]*yaml.Node)
	collectSequencePaths(&existingNode, "", existingSequences)

	// Determine added and removed sets. A sequence element is neither added nor removed while its
	// owning list key is present on both sides: the list is carried whole, whatever its length.
	added = []string{}
	for path := range templateLeaves {
		if _, ok := existingLeaves[path]; ok {
			continue
		}
		if base, isElement := sequenceBasePath(path); isElement && existingSequences[base] != nil {
			continue
		}
		if underAny(path, absentOpenMaps) {
			continue
		}
		added = append(added, path)
	}
	added = append(added, absentOpenMaps...)
	sort.Strings(added)

	removed = []string{}
	for path := range existingLeaves {
		if _, ok := templateLeaves[path]; ok {
			continue
		}
		if base, isElement := sequenceBasePath(path); isElement && templateSequences[base] != nil {
			continue
		}
		removed = append(removed, path)
	}
	sort.Strings(removed)

	// Reconcile: overwrite template leaf values with existing values, then carry existing lists
	// whole.
	applyExistingOverrides(templateLeaves, existingLeaves)
	applyExistingSequences(templateSequences, existingSequences)

	// Marshal the mutated template back to bytes
	merged, err = yaml.Marshal(&templateNode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal merged YAML: %w", err)
	}

	return merged, added, removed, nil
}

// MissingKeys returns the leaf key-paths present in template but absent from existing.
// A key present with an empty value counts as present.
//
// A template leaf at or under a declared openMaps path is satisfied by the presence of that key in existing, whatever its value.
//
// A template list is a DEFAULT, not a minimum length. collectLeafPaths models each sequence element
// as its own indexed leaf path (`key[0]`, `key[1]`, ...), which is right for the reconcile merge but
// wrong here: it made a config that shortened a list -- including to the empty list -- fail to load
// with "missing keys: key[0]", and told the operator to run "lyx config reconcile", which re-adds
// the template's own element and undoes the edit. An emptied list is the only general way to express
// "none of these", and for landing.yaml's require_pr_to_base it is the whole no-pull-request mode
// internal/landingshed's Publish documents; it was unreachable through this loader.
// A sequence-element path is therefore satisfied by the presence of its owning KEY in existing,
// whatever that key's length. Every other leaf path keeps the exact-match rule.
func MissingKeys(template, existing []byte, openMaps ...string) ([]string, error) {
	// Parse template
	var templateNode yaml.Node
	if parseErr := yaml.Unmarshal(template, &templateNode); parseErr != nil {
		return nil, fmt.Errorf("parse template YAML: %w", parseErr)
	}

	// Parse existing (empty/nil yields empty document)
	var existingNode yaml.Node
	if len(strings.TrimSpace(string(existing))) == 0 {
		existingNode.Kind = yaml.DocumentNode
		existingNode.Content = []*yaml.Node{}
	} else {
		if parseErr := yaml.Unmarshal(existing, &existingNode); parseErr != nil {
			return nil, fmt.Errorf("parse existing YAML: %w", parseErr)
		}
	}

	// Collect leaf key-paths from both trees
	templateLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&templateNode, templateLeaves)

	existingLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&existingNode, existingLeaves)

	// Every mapping key present in existing, at any depth, regardless of what its value is. A
	// sequence-valued key has no leaf of its own when its list is empty, so the leaf map above
	// cannot answer "is this key present" for exactly the case that matters here.
	existingKeys := make(map[string]bool)
	collectMappingKeyPaths(&existingNode, "", existingKeys)

	// Find missing keys (in template but not in existing)
	missing := []string{}
	for path := range templateLeaves {
		if _, ok := existingLeaves[path]; ok {
			continue
		}
		if base, isElement := sequenceBasePath(path); isElement && existingKeys[base] {
			continue
		}
		if open, ok := openMapOf(path, openMaps); ok && existingKeys[open] {
			continue
		}
		missing = append(missing, path)
	}
	sort.Strings(missing)

	return missing, nil
}

// underPath reports whether path is p itself or lies under it as a nested key or list element.
func underPath(path, p string) bool {
	return path == p || strings.HasPrefix(path, p+".") || strings.HasPrefix(path, p+"[")
}

// underAny reports whether path is at or under any of the dotted paths.
func underAny(path string, paths []string) bool {
	_, ok := openMapOf(path, paths)
	return ok
}

// openMapOf returns the first declared open-map path that path is at or under.
func openMapOf(path string, openMaps []string) (string, bool) {
	for _, p := range openMaps {
		if underPath(path, p) {
			return p, true
		}
	}
	return "", false
}

// findValueNode returns the value node of the mapping key at the dotted path, or nil when any step
// is absent or not a mapping.
func findValueNode(node *yaml.Node, path string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	for _, key := range strings.Split(path, ".") {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				next = node.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		node = next
	}
	return node
}

// carryOpenMaps replaces the template's value at each declared open-map path with existing's value whole, whatever its kind.
// It returns the sorted declared paths the template holds and existing lacks, which keep the template's value.
func carryOpenMaps(templateNode, existingNode *yaml.Node, openMaps []string) []string {
	var absent []string
	for _, p := range openMaps {
		templateValue := findValueNode(templateNode, p)
		if templateValue == nil {
			continue
		}
		existingValue := findValueNode(existingNode, p)
		if existingValue == nil {
			absent = append(absent, p)
			continue
		}
		*templateValue = *existingValue
	}
	sort.Strings(absent)
	return absent
}

// sequenceBasePath splits a sequence-element leaf path into its owning key path, reporting whether
// path is a sequence element at all.
// "require_pr_to_base[0]" yields ("require_pr_to_base", true); "squash" yields ("", false).
// Only a trailing index is recognised, so a nested element's owning path is the whole prefix before
// that index.
func sequenceBasePath(path string) (base string, isElement bool) {
	if !strings.HasSuffix(path, "]") {
		return "", false
	}
	open := strings.LastIndex(path, "[")
	if open <= 0 {
		return "", false
	}
	index := path[open+1 : len(path)-1]
	if index == "" {
		return "", false
	}
	for _, r := range index {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return path[:open], true
}

// collectMappingKeyPaths walks a YAML node tree and records every mapping key's dotted path in keys,
// whatever the shape of its value. It deliberately does not descend into sequences: a sequence's
// elements are not keys, and the only question this map answers is whether a key is present.
func collectMappingKeyPaths(node *yaml.Node, prefix string, keys map[string]bool) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectMappingKeyPaths(child, prefix, keys)
		}

	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if key == "" {
				continue
			}
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			keys[path] = true
			collectMappingKeyPaths(node.Content[i+1], path, keys)
		}
	}
}

// applyExistingOverrides copies each existing leaf's value, tag, and style
// onto the matching template leaf, leaving template-only leaves untouched.
func applyExistingOverrides(templateLeaves, existingLeaves map[string]*yaml.Node) {
	for path, existingLeaf := range existingLeaves {
		if templateLeaf, ok := templateLeaves[path]; ok {
			// Preserve the user's value in the template leaf.
			templateLeaf.Value = existingLeaf.Value
			templateLeaf.Tag = existingLeaf.Tag
			templateLeaf.Style = existingLeaf.Style
		}
	}
}

// collectSequencePaths records every sequence-valued mapping key's dotted path and its sequence node
// in sequences. It does not descend into sequences: an element is never itself a list key.
func collectSequencePaths(node *yaml.Node, prefix string, sequences map[string]*yaml.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectSequencePaths(child, prefix, sequences)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if key == "" {
				continue
			}
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			value := node.Content[i+1]
			if value.Kind == yaml.SequenceNode {
				sequences[path] = value
				continue
			}
			collectSequencePaths(value, path, sequences)
		}
	}
}

// applyExistingSequences replaces each template list's elements with the same-path existing list's,
// whole. A template list is a default, not a minimum length (see MissingKeys): merging element by
// element re-added the template's own entries to a shortened or emptied list, so any --set or
// reconcile silently undid landing.yaml's `require_pr_to_base: []`.
func applyExistingSequences(templateSequences, existingSequences map[string]*yaml.Node) {
	for path, existingSequence := range existingSequences {
		if templateSequence, ok := templateSequences[path]; ok {
			templateSequence.Content = existingSequence.Content
			templateSequence.Style = existingSequence.Style
		}
	}
}

// collectLeafPaths walks a YAML node tree and collects all leaf key-paths
// into the leaves map.
func collectLeafPaths(node *yaml.Node, leaves map[string]*yaml.Node) {
	var paths []string
	collectLeafPathsHelper(node, "", leaves, &paths)
}

// collectLeafPathsHelper recursively walks a node and collects leaf key-paths
// using depth-first traversal with dotted notation for nested keys.
func collectLeafPathsHelper(node *yaml.Node, prefix string, leaves map[string]*yaml.Node, paths *[]string) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		// Document node: process the root content (usually one child)
		for _, child := range node.Content {
			collectLeafPathsHelper(child, "", leaves, paths)
		}

	case yaml.MappingNode:
		// Mapping node: iterate over key-value pairs
		for i := 0; i < len(node.Content); i += 2 {
			if i+1 >= len(node.Content) {
				break
			}
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			// Extract the key (should be a scalar)
			key := keyNode.Value
			if key == "" {
				continue
			}

			// Build the dotted path for nested keys
			var path string
			if prefix == "" {
				path = key
			} else {
				path = prefix + "." + key
			}

			// Recurse into the value node
			collectLeafPathsHelper(valNode, path, leaves, paths)
		}

	case yaml.SequenceNode:
		// Sequence node: each element in a sequence is indexed, but we treat
		// scalar elements as leaves.
		for i, elem := range node.Content {
			// Build indexed path (e.g., "items.0", "items.1")
			indexPath := fmt.Sprintf("%s[%d]", prefix, i)
			collectLeafPathsHelper(elem, indexPath, leaves, paths)
		}

	case yaml.ScalarNode:
		// Scalar leaf: record this as a leaf path
		if prefix != "" {
			leaves[prefix] = node
			*paths = append(*paths, prefix)
		}

	case yaml.AliasNode:
		// Alias nodes are references; skip them
	}
}
