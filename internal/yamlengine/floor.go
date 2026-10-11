// floor.go raises one top-level integer key of a YAML document to a floor, keeping every comment and every other key.

package yamlengine

import (
	"bytes"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// RaiseIntFloor raises the top-level integer key to floor when its stored value is below it or null, and returns the rewritten document with one line "<key>: raised from <old> to <floor>", the old value being null for a null.
// Every other key, value and comment is carried through yaml node editing, re-encoded with two-space indent.
// The document comes back unchanged with an empty line when it is empty, when key is absent, when the value is an integer at or above floor, or when it is any other scalar, an ${env:...} marker included, that does not decode as an integer.
// A document that does not parse, or whose root is not a mapping, returns an error.
func RaiseIntFloor(existing []byte, key string, floor int) ([]byte, string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, "", fmt.Errorf("unmarshal yaml: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, "", nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, "", fmt.Errorf("yaml root is not a mapping")
	}

	var value *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			value = root.Content[i+1]
			break
		}
	}
	if value == nil || value.Kind != yaml.ScalarNode || envMarkerRe.MatchString(value.Value) {
		return existing, "", nil
	}

	var old string
	if value.Tag == "!!null" {
		old = "null"
	} else {
		var stored int
		if err := value.Decode(&stored); err != nil || stored >= floor {
			return existing, "", nil
		}
		old = strconv.Itoa(stored)
	}
	value.Tag = "!!int"
	value.Style = 0
	value.Value = strconv.Itoa(floor)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, "", fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, "", fmt.Errorf("encode yaml: %w", err)
	}
	return out.Bytes(), fmt.Sprintf("%s: raised from %s to %d", key, old, floor), nil
}
