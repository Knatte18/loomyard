package reedengine

import (
	"reflect"
	"testing"
)

// guidsOf returns the strands' guids in table order.
func guidsOf(strands []Strand) []string {
	out := make([]string, len(strands))
	for i, s := range strands {
		out[i] = s.GUID
	}
	return out
}

// TestMoveStrandTo pins the pure table-position step ReplaceStrand uses to give the new strand the
// replaced strand's slot: the strand added last (the bottom) moves to the first, a middle and the
// last slot, and every other strand keeps its relative order.
func TestMoveStrandTo(t *testing.T) {
	base := []Strand{{GUID: "a"}, {GUID: "b"}, {GUID: "c"}, {GUID: "new"}}

	tests := []struct {
		name string
		idx  int
		want []string
	}{
		{"first slot", 0, []string{"new", "a", "b", "c"}},
		{"middle slot", 1, []string{"a", "new", "b", "c"}},
		{"last slot", 3, []string{"a", "b", "c", "new"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]Strand(nil), base...)
			got := guidsOf(moveStrandTo(in, "new", tt.idx))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("moveStrandTo(..., \"new\", %d) = %v, want %v", tt.idx, got, tt.want)
			}
		})
	}
}

// TestMoveStrandTo_NoOps pins that an unknown guid or an out-of-range index leaves the table alone.
func TestMoveStrandTo_NoOps(t *testing.T) {
	base := []Strand{{GUID: "a"}, {GUID: "b"}}
	want := []string{"a", "b"}

	for _, tt := range []struct {
		name string
		guid string
		idx  int
	}{
		{"unknown guid", "zzz", 0},
		{"negative index", "b", -1},
		{"index past end", "a", 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]Strand(nil), base...)
			if got := guidsOf(moveStrandTo(in, tt.guid, tt.idx)); !reflect.DeepEqual(got, want) {
				t.Errorf("moveStrandTo(%q, %d) = %v, want %v", tt.guid, tt.idx, got, want)
			}
		})
	}
}
