// types_test.go pins the on-disk contract of Display's JSON keys, in particular that a record
// still carrying the retired fixedRows, shrinkWhenWaitingOnChild and size keys decodes with all ignored.

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDisplayDecodesRetiredKeysIgnored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
	}{
		{"FixedRowsAndShrink", `{"anchor":"below-parent","focus":true,"shrinkWhenWaitingOnChild":true,"fixedRows":3}`},
		{"ContentSize", `{"anchor":"below-parent","focus":true,"size":{"kind":"content","shareNum":1,"shareDen":3,"minRows":3}}`},
		{"SharingSize", `{"anchor":"below-parent","focus":true,"size":{"kind":"sharing"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var d Display
			if err := json.Unmarshal([]byte(tt.raw), &d); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			want := Display{Anchor: AnchorBelowParent, Focus: true}
			if d != want {
				t.Errorf("decoded Display = %+v, want %+v", d, want)
			}
		})
	}
}

func TestDisplayMarshalsWithoutRetiredKeys(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(Display{Anchor: AnchorBelowParent, Focus: true})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, key := range []string{"fixedRows", "shrinkWhenWaitingOnChild", "size"} {
		if strings.Contains(string(raw), key) {
			t.Errorf("marshalled Display %s still carries the retired key %q", raw, key)
		}
	}

	if strings.Contains(string(raw), "keepViewer") {
		t.Errorf("marshalled Display %s carries a keepViewer key for a false KeepViewer", raw)
	}

	want := Display{Anchor: AnchorTop, KeepViewer: true}
	raw, err = json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal keep-viewer Display: %v", err)
	}
	var got Display
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal %s: %v", raw, err)
	}
	if got != want {
		t.Errorf("Display round-tripped through %s as %+v, want %+v", raw, got, want)
	}
}
