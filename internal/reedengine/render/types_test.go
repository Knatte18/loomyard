// types_test.go pins the on-disk contract of Display's JSON keys, in particular that a record
// still carrying the retired fixedRows and shrinkWhenWaitingOnChild keys decodes with both ignored.

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDisplayDecodesRetiredKeysIgnored(t *testing.T) {
	t.Parallel()
	var d Display
	if err := json.Unmarshal([]byte(`{"anchor":"below-parent","focus":true,"shrinkWhenWaitingOnChild":true,"fixedRows":3}`), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Anchor != AnchorBelowParent || !d.Focus {
		t.Errorf("decoded Display = %+v, want anchor below-parent and focus true", d)
	}
	if d.Size != (Size{}) {
		t.Errorf("decoded Size = %+v from a record with no size key, want the zero Size", d.Size)
	}
}

func TestDisplayMarshalsWithoutRetiredKeys(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(Display{Anchor: AnchorBelowParent, Focus: true})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"fixedRows", "shrinkWhenWaitingOnChild"} {
		if strings.Contains(string(raw), key) {
			t.Errorf("marshalled Display %s still carries the retired key %q", raw, key)
		}
	}
	if strings.Contains(string(raw), "size") {
		t.Errorf("marshalled Display %s carries a size key for a zero Size", raw)
	}

	want := Display{Anchor: AnchorTop, Size: Size{Kind: SizeContent, ShareNum: 1, ShareDen: 3, MinRows: 3}}
	raw, err = json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal content Display: %v", err)
	}
	var got Display
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal %s: %v", raw, err)
	}
	if got != want {
		t.Errorf("content Display round-tripped through %s as %+v, want %+v", raw, got, want)
	}
}
