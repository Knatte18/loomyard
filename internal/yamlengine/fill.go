// fill.go implements the in-memory merge of a file's missing template keys.

package yamlengine

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FillMissing merges every mapping key the template holds and existing lacks into existing's own document.
// It returns the merged bytes and the sorted dotted key-paths it inserted.
//
// Unlike Reconcile, it builds on existing: file keys the template lacks stay, file values are never replaced, and a shape mismatch is an error rather than a replacement.
// A missing key is appended to its mapping after the file's own keys, in template order, as a deep copy of the template's nodes.
// The reported path is the inserted key's own path, never its descendants.
//
// A key at a declared openMaps path that the file holds is skipped before any shape check,
// so a list at that path against a mapping template is no mismatch;
// a missing one is appended whole like any other key.
//
// Sequences are carried whole and never descended into, so a template key missing inside an element of a present list stays missing, for MissingKeys to report.
// An empty or comments-only existing returns the template bytes verbatim, reporting every top-level template key.
// When nothing is missing, existing is returned byte-for-byte with an empty key list.
func FillMissing(template, existing []byte, openMaps ...string) (filled []byte, keys []string, err error) {
	var templateDoc yaml.Node
	if parseErr := yaml.Unmarshal(template, &templateDoc); parseErr != nil {
		return nil, nil, fmt.Errorf("parse template YAML: %w", parseErr)
	}
	templateRoot := documentRoot(&templateDoc)

	var existingDoc yaml.Node
	if parseErr := yaml.Unmarshal(existing, &existingDoc); parseErr != nil {
		return nil, nil, fmt.Errorf("parse existing YAML: %w", parseErr)
	}
	existingRoot := documentRoot(&existingDoc)

	if existingRoot == nil {
		keys = []string{}
		if templateRoot != nil && templateRoot.Kind == yaml.MappingNode {
			for i := 0; i < len(templateRoot.Content); i += 2 {
				keys = append(keys, templateRoot.Content[i].Value)
			}
		}
		sort.Strings(keys)
		return template, keys, nil
	}
	if templateRoot == nil {
		return existing, []string{}, nil
	}

	keys = []string{}
	if err := fillMapping(templateRoot, existingRoot, "", &keys, openMaps); err != nil {
		return nil, nil, err
	}
	if len(keys) == 0 {
		return existing, keys, nil
	}
	sort.Strings(keys)

	filled, err = yaml.Marshal(&existingDoc)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal filled YAML: %w", err)
	}
	return filled, keys, nil
}

// documentRoot returns the content node of a parsed document, or nil for an empty or comments-only one.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

// fillMapping walks template and existing in parallel and appends each missing template key to existing.
// path is the dotted path of the two mappings; it is empty at the root.
// A present key at an openMaps path is left alone, whatever its shape.
func fillMapping(template, existing *yaml.Node, path string, keys *[]string, openMaps []string) error {
	if template.Kind != yaml.MappingNode {
		// A scalar or sequence template node has nothing to fill.
		return nil
	}
	if existing.Kind != yaml.MappingNode {
		return shapeMismatch(path, "mapping", nodeShape(existing))
	}

	present := make(map[string]*yaml.Node, len(existing.Content)/2)
	for i := 0; i+1 < len(existing.Content); i += 2 {
		present[existing.Content[i].Value] = existing.Content[i+1]
	}

	for i := 0; i+1 < len(template.Content); i += 2 {
		keyNode, valueNode := template.Content[i], template.Content[i+1]
		keyPath := joinKeyPath(path, keyNode.Value)

		fileValue, ok := present[keyNode.Value]
		if !ok {
			existing.Content = append(existing.Content, deepCopyNode(keyNode), deepCopyNode(valueNode))
			*keys = append(*keys, keyPath)
			continue
		}
		if slices.Contains(openMaps, keyPath) {
			continue
		}

		switch valueNode.Kind {
		case yaml.MappingNode:
			if fileValue.Kind != yaml.MappingNode {
				return shapeMismatch(keyPath, "mapping", nodeShape(fileValue))
			}
			if err := fillMapping(valueNode, fileValue, keyPath, keys, openMaps); err != nil {
				return err
			}
		case yaml.ScalarNode:
			if fileValue.Kind == yaml.MappingNode {
				return shapeMismatch(keyPath, "scalar", "mapping")
			}
		}
	}
	return nil
}

func joinKeyPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// nodeShape names a node's shape for a mismatch message; a null scalar reads as null.
func nodeShape(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return "null"
		}
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	}
	return "unknown"
}

func shapeMismatch(path, want, got string) error {
	if strings.TrimSpace(path) == "" {
		path = "<document root>"
	}
	return fmt.Errorf("key %q: template holds a %s, file holds a %s", path, want, got)
}

func deepCopyNode(node *yaml.Node) *yaml.Node {
	clone := *node
	clone.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		clone.Content[i] = deepCopyNode(child)
	}
	if node.Alias != nil {
		clone.Alias = deepCopyNode(node.Alias)
	}
	return &clone
}
