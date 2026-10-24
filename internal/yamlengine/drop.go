// drop.go removes retired top-level keys from a YAML document, keeping every other key and comment.

package yamlengine

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// DropKeys removes each named top-level key of the document's mapping together with its value and comments, and returns the rewritten document with one line "<key>: retired, removed" per key removed, in keys' order.
// Every other key and comment is carried through yaml node editing, re-encoded with two-space indent.
// The document comes back unchanged, with no lines, when it is empty or holds none of the keys, so a second pass is a no-op.
// A document that does not parse, or whose root is not a mapping, returns an error.
func DropKeys(existing []byte, keys []string) ([]byte, []string, error) {
	document, root, err := parseMappingRoot(existing, "config")
	if err != nil {
		return nil, nil, err
	}
	if root == nil {
		return existing, nil, nil
	}

	var lines []string
	for _, key := range keys {
		at := mappingIndex(root, key)
		if at < 0 {
			continue
		}
		root.Content = append(root.Content[:at], root.Content[at+2:]...)
		lines = append(lines, fmt.Sprintf("%s: retired, removed", key))
	}
	if len(lines) == 0 {
		return existing, nil, nil
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), lines, nil
}
