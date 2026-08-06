// reconcile.go implements config reconciliation and missing-key detection.
// It merges a template with existing user configuration while preserving the template structure and user values.

package yamlengine

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reconcile merges a template with existing user configuration, preserving template comments and key order.
// Keys present in existing override template defaults;
// keys absent from existing are reported in added, keys absent from template are reported in removed.
// Reconcile is idempotent.
func Reconcile(template, existing []byte) (merged []byte, added, removed []string, err error) {
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

	// Collect all leaf key-paths from the template
	templateLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&templateNode, templateLeaves)

	// Collect all leaf key-paths from existing
	existingLeaves := make(map[string]*yaml.Node)
	collectLeafPaths(&existingNode, existingLeaves)

	// Determine added and removed sets
	added = []string{}
	for path := range templateLeaves {
		if _, ok := existingLeaves[path]; !ok {
			added = append(added, path)
		}
	}
	sort.Strings(added)

	removed = []string{}
	for path := range existingLeaves {
		if _, ok := templateLeaves[path]; !ok {
			removed = append(removed, path)
		}
	}
	sort.Strings(removed)

	// Reconcile: overwrite template leaf values with existing values
	applyExistingOverrides(templateLeaves, existingLeaves)

	// Marshal the mutated template back to bytes
	merged, err = yaml.Marshal(&templateNode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal merged YAML: %w", err)
	}

	return merged, added, removed, nil
}

// MissingKeys returns the leaf key-paths present in template but absent from existing.
// A key present with an empty value counts as present.
func MissingKeys(template, existing []byte) ([]string, error) {
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

	// Find missing keys (in template but not in existing)
	missing := []string{}
	for path := range templateLeaves {
		if _, ok := existingLeaves[path]; !ok {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)

	return missing, nil
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
