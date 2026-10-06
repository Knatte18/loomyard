// policy_test.go tests the anchor dispatch, insertion-order stack ordering, and cycle-safe traversal
// in policy.go.

package render

import (
	"slices"
	"testing"
)

func TestPartitionByAnchor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		strands []Strand
		want    []string
	}{
		{
			name: "HiddenStrandsDropped",
			strands: []Strand{
				{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorHidden}},
			},
			want: []string{"a"},
		},
		{
			name: "NotLiveOrEmptyPaneIDDropped",
			strands: []Strand{
				{GUID: "stack-not-live", PaneID: "%1", Live: false, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "stack-no-pane", PaneID: "", Live: true, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "stack-ok", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			want: []string{"stack-ok"},
		},
		{
			name: "OwnWindowNotPlaced",
			strands: []Strand{
				{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorOwnWindow}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, s := range partitionByAnchor(tt.strands) {
				got = append(got, s.GUID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("partitionByAnchor() placed %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBreakCyclesTerminatesAndKeepsEveryStrand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		strands []Strand
	}{
		{
			"SelfParent",
			[]Strand{{GUID: "a", Parent: "a"}},
		},
		{
			"MutualCycle",
			[]Strand{
				{GUID: "a", Parent: "b"},
				{GUID: "b", Parent: "a"},
			},
		},
		{
			"DanglingChainIntoCycle",
			[]Strand{
				{GUID: "s", Parent: "a"},
				{GUID: "a", Parent: "b"},
				{GUID: "b", Parent: "a"},
			},
		},
		{
			// The parent chain plays no part in ordering: a child inserted before its parent stays before it, and every strand keeps its input position.
			"AcyclicChildBeforeParentKeepsInsertionOrder",
			[]Strand{
				{GUID: "c", Parent: "a"},
				{GUID: "a", Parent: ""},
				{GUID: "d", Parent: ""},
				{GUID: "b", Parent: "a"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixed := breakCycles(tt.strands)
			if len(fixed) != len(tt.strands) {
				t.Fatalf("breakCycles returned %d strands, want %d", len(fixed), len(tt.strands))
			}

			byGUID := make(map[string]Strand, len(fixed))
			for _, s := range fixed {
				byGUID[s.GUID] = s
			}

			// Every chain must terminate in a bounded number of hops —
			// this is the "renders every strand once" / no-infinite-loop
			// guarantee breakCycles exists to provide.
			for _, s := range fixed {
				visited := map[string]bool{s.GUID: true}
				cur := s.Parent
				hops := 0
				for cur != "" {
					if visited[cur] {
						t.Fatalf("strand %s chain still cycles after breakCycles", s.GUID)
					}
					visited[cur] = true
					parent, ok := byGUID[cur]
					if !ok {
						break
					}
					cur = parent.Parent
					hops++
					if hops > len(fixed) {
						t.Fatalf("strand %s chain exceeds %d hops, still not terminating", s.GUID, len(fixed))
					}
				}
			}

			// orderStack must also produce a total ordering (every strand exactly once) over the repaired chain, in insertion order.
			ordered := orderStack(fixed)
			if len(ordered) != len(tt.strands) {
				t.Fatalf("orderStack(breakCycles(...)) returned %d strands, want %d", len(ordered), len(tt.strands))
			}
			for i, s := range tt.strands {
				if ordered[i].GUID != s.GUID {
					t.Errorf("ordered[%d] = %q, want %q (insertion order)", i, ordered[i].GUID, s.GUID)
				}
			}
		})
	}
}
