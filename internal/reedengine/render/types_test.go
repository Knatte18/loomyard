// types_test.go pins the on-disk contract of Display's JSON keys, in particular that a record
// still carrying the retired fixedRows and shrinkWhenWaitingOnChild keys decodes with both ignored.

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDisplayDecodesRetiredKeysIgnored(t *testing.T) {
	var d Display
	if err := json.Unmarshal([]byte(`{"anchor":"below-parent","focus":true,"shrinkWhenWaitingOnChild":true,"fixedRows":3}`), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Anchor != AnchorBelowParent || !d.Focus {
		t.Errorf("decoded Display = %+v, want anchor below-parent and focus true", d)
	}
}

func TestDisplayMarshalsWithoutRetiredKeys(t *testing.T) {
	raw, err := json.Marshal(Display{Anchor: AnchorBelowParent, Focus: true})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"fixedRows", "shrinkWhenWaitingOnChild"} {
		if strings.Contains(string(raw), key) {
			t.Errorf("marshalled Display %s still carries the retired key %q", raw, key)
		}
	}
}
