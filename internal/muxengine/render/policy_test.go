// policy_test.go tests the anchor dispatch, deterministic stack ordering,
// and cycle-safe traversal in policy.go.

package render

import "testing"

func TestPartitionByAnchor(t *testing.T) {
	t.Run("HiddenStrandsDropped", func(t *testing.T) {
		strands := []Strand{
			{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorTop}},
			{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorHidden}},
		}
		top, stack := partitionByAnchor(strands)
		if len(top) != 1 || top[0].GUID != "a" {
			t.Errorf("top = %+v, want only strand a", top)
		}
		if len(stack) != 0 {
			t.Errorf("stack = %+v, want empty", stack)
		}
	})

	t.Run("NotLiveOrEmptyPaneIDDroppedFromBothPartitions", func(t *testing.T) {
		strands := []Strand{
			{GUID: "top-not-live", PaneID: "%1", Live: false, Display: Display{Anchor: AnchorTop}},
			{GUID: "top-no-pane", PaneID: "", Live: true, Display: Display{Anchor: AnchorTop}},
			{GUID: "stack-not-live", PaneID: "%2", Live: false, Display: Display{Anchor: AnchorBelowParent}},
			{GUID: "stack-no-pane", PaneID: "", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			{GUID: "top-ok", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorTop}},
			{GUID: "stack-ok", PaneID: "%4", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		}
		top, stack := partitionByAnchor(strands)
		if len(top) != 1 || top[0].GUID != "top-ok" {
			t.Errorf("top = %+v, want only strand top-ok", top)
		}
		if len(stack) != 1 || stack[0].GUID != "stack-ok" {
			t.Errorf("stack = %+v, want only strand stack-ok", stack)
		}
	})

	t.Run("OwnWindowNotPlaced", func(t *testing.T) {
		strands := []Strand{
			{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorOwnWindow}},
		}
		top, stack := partitionByAnchor(strands)
		if len(top) != 0 || len(stack) != 0 {
			t.Errorf("partitionByAnchor(own-window) = (%v, %v), want both empty", top, stack)
		}
	})
}

func TestOrderStackSiblingInsertionOrder(t *testing.T) {
	// b and c both parent under a and must keep their relative input
	// order; d is a second root and must not interleave between them.
	strands := []Strand{
		{GUID: "a", Parent: ""},
		{GUID: "d", Parent: ""},
		{GUID: "c", Parent: "a"},
		{GUID: "b", Parent: "a"},
	}
	ordered := orderStack(strands)

	depthOf := func(guid string) int {
		for i, s := range ordered {
			if s.GUID == guid {
				return i
			}
		}
		t.Fatalf("guid %q not found in ordered result", guid)
		return -1
	}

	if depthOf("a") >= depthOf("c") {
		t.Errorf("root a must precede its child c")
	}
	if depthOf("a") >= depthOf("b") {
		t.Errorf("root a must precede its child b")
	}
	// c was inserted before b in the input, and both share parent a, so c
	// must stay before b in the output.
	if depthOf("c") >= depthOf("b") {
		t.Errorf("siblings c, b must preserve insertion order (c before b)")
	}
}

func TestBreakCyclesTerminatesAndKeepsEveryStrand(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			// orderStack must also produce a total ordering (every strand
			// exactly once) over the repaired chain.
			ordered := orderStack(fixed)
			if len(ordered) != len(tt.strands) {
				t.Fatalf("orderStack(breakCycles(...)) returned %d strands, want %d", len(ordered), len(tt.strands))
			}
		})
	}
}
