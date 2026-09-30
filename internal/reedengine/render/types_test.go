// types_test.go pins the on-disk contract of Display's JSON keys, in particular that a record
// written before FixedRows existed decodes to the zero budget.

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDisplayDecodesWithoutFixedRowsAsZero(t *testing.T) {
	var d Display
	if err := json.Unmarshal([]byte(`{"anchor":"below-parent","focus":true,"shrinkWhenWaitingOnChild":true}`), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.FixedRows != 0 {
		t.Errorf("FixedRows = %d, want 0 for a record without fixedRows", d.FixedRows)
	}
}

func TestDisplayFixedRowsRoundTripsUnderFixedRowsKey(t *testing.T) {
	raw, err := json.Marshal(Display{Anchor: AnchorBelowParent, FixedRows: 3})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"fixedRows":3`) {
		t.Errorf("marshalled Display %s lacks the key/value %q", raw, `"fixedRows":3`)
	}
	var got Display
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.FixedRows != 3 {
		t.Errorf("round-tripped FixedRows = %d, want 3", got.FixedRows)
	}
}
