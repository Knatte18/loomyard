package reedengine

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// guidsOf returns the strands' guids in table order.
func guidsOf(strands []Strand) []string {
	out := make([]string, len(strands))
	for i, s := range strands {
		out[i] = s.GUID
	}
	return out
}

// TestMoveStrandTo pins the pure table-position step ReplaceStrand uses.
// It gives the new strand the replaced strand's slot.
// The strand added last (the bottom) moves to the first, a middle and the last slot;
// every other strand keeps its relative order.
// An unknown guid or an out-of-range index leaves the table alone.
func TestMoveStrandTo(t *testing.T) {
	tiergate.Require(t, 0)
	base := []Strand{{GUID: "a"}, {GUID: "b"}, {GUID: "c"}, {GUID: "new"}}
	tests := []struct {
		name string
		guid string
		idx  int
		want []string
	}{
		{"first slot", "new", 0, []string{"new", "a", "b", "c"}},
		{"middle slot", "new", 1, []string{"a", "new", "b", "c"}},
		{"last slot", "new", 3, []string{"a", "b", "c", "new"}},
		{"unknown guid", "zzz", 0, []string{"a", "b", "c", "new"}},
		{"negative index", "c", -1, []string{"a", "b", "c", "new"}},
		{"index past end", "a", 5, []string{"a", "b", "c", "new"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]Strand(nil), base...)
			got := guidsOf(moveStrandTo(in, tt.guid, tt.idx))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("moveStrandTo(..., %q, %d) = %v, want %v", tt.guid, tt.idx, got, tt.want)
			}
		})
	}
}
