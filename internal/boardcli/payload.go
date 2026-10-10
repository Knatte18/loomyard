// payload.go declares payloadKeys, the key declaration every board payload parser validates against, and payloadLegend, the one statement of the grammar a payload command's `{…}` Use token is written in.

package boardcli

import (
	"fmt"
	"slices"
)

// payloadLegend states the grammar of a board payload command's `{…}` Use token.
const payloadLegend = "A payload is one JSON object as a single quoted argument; ? marks an optional key, a|b exactly one of the two, <task> stands for upsert's payload, and an unknown key is refused."

// payloadKeys declares the keys one payload object accepts, exactly as its command's Use token spells them.
type payloadKeys struct {
	// required keys must be present.
	required []string
	// optional keys may be present.
	optional []string
	// exclusive holds pairs of keys of which exactly one must be present.
	exclusive [][2]string
	// nested declares, for a key whose value is an object or an array of objects, that object's own keys.
	nested map[string]payloadKeys
}

// accepts reports whether k declares key.
func (k payloadKeys) accepts(key string) bool {
	if slices.Contains(k.required, key) || slices.Contains(k.optional, key) {
		return true
	}
	for _, pair := range k.exclusive {
		if pair[0] == key || pair[1] == key {
			return true
		}
	}
	return false
}

// refuseUnknownKey returns the unknown-field refusal for a key of fields that k does not declare, and nil when k declares them all.
func refuseUnknownKey[V any](k payloadKeys, fields map[string]V) error {
	for key := range fields {
		if !k.accepts(key) {
			return fmt.Errorf("unknown field: %q", key)
		}
	}
	return nil
}
