// changedkeys.go implements ChangedKeys, the key-by-key comparison of one config file at a task's fork point, its tip and its parent's tip.

package configengine

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// AbsentValue is the value ChangedKeys reports for a key, or a whole file, a revision does not hold.
const AbsentValue = "absent"

// KeyChange is one config key whose value the task changed away from both its fork point and its parent.
// Base, Tip and Parent hold the key's value at each revision rendered as text, or AbsentValue.
type KeyChange struct {
	Key    string
	Base   string
	Tip    string
	Parent string
}

// ChangedKeys parses base, tip and parent as YAML documents, flattens each to dotted key paths with their values, and returns, sorted by key, every key whose tip value differs from its base value and from its parent value.
// A nil or empty document, or one holding only comments, holds no key, so a file absent at a revision reads every key as AbsentValue.
// A list or a scalar is one value; a key the tip drops differs from a base that held it.
// A document that does not parse is an error naming which of the three it is.
func ChangedKeys(base, tip, parent []byte) ([]KeyChange, error) {
	baseKeys, err := flattenDocument("base", base)
	if err != nil {
		return nil, err
	}
	tipKeys, err := flattenDocument("tip", tip)
	if err != nil {
		return nil, err
	}
	parentKeys, err := flattenDocument("parent", parent)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, keys := range []map[string]string{baseKeys, tipKeys, parentKeys} {
		for key := range keys {
			seen[key] = true
		}
	}
	var changes []KeyChange
	for key := range seen {
		change := KeyChange{Key: key, Base: AbsentValue, Tip: AbsentValue, Parent: AbsentValue}
		baseValue, inBase := baseKeys[key]
		tipValue, inTip := tipKeys[key]
		parentValue, inParent := parentKeys[key]
		if inBase {
			change.Base = baseValue
		}
		if inTip {
			change.Tip = tipValue
		}
		if inParent {
			change.Parent = parentValue
		}
		if inTip == inBase && tipValue == baseValue {
			continue
		}
		if inTip == inParent && tipValue == parentValue {
			continue
		}
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })
	return changes, nil
}

// flattenDocument parses data as a YAML mapping and returns each leaf's dotted key path and its value rendered as text.
// which names the document in a parse failure.
func flattenDocument(which string, data []byte) (map[string]string, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse the %s config: %w", which, err)
	}
	flat := map[string]string{}
	flattenInto(flat, "", root)
	return flat, nil
}

// flattenInto adds every leaf under node to flat; a mapping contributes its entries under prefix, anything else is one leaf.
func flattenInto(flat map[string]string, prefix string, node map[string]any) {
	for key, value := range node {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if child, ok := value.(map[string]any); ok {
			flattenInto(flat, path, child)
			continue
		}
		flat[path] = fmt.Sprint(value)
	}
}
