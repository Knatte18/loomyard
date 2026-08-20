// policy.go implements the anchor-to-placement dispatch: filtering strands down to the below-parent
// stack (or excluding them entirely), ordering the stack deterministically by parent-chain depth,
// and repairing a corrupt cyclic parent table so ordering always terminates.
// This is the legible half of the policy layer — adding a new anchor means adding a case here, not
// touching the mechanics layer in layout.go/checksum.go.

package render

import "sort"

// partitionByAnchor filters strands down to the below-parent stack,
// excluding AnchorHidden, not-live, and empty-PaneID strands that render
// cannot lay out.
func partitionByAnchor(strands []Strand) (stack []Strand) {
	for _, s := range strands {
		if s.Display.Anchor == AnchorHidden || !s.Live || s.PaneID == "" {
			continue
		}
		switch s.Display.Anchor {
		case AnchorBelowParent:
			stack = append(stack, s)
		default:
			// AnchorOwnWindow (deferred) or any unrecognized value is not
			// placed by this policy.
		}
	}
	return stack
}

// removeDuplicatePaneCells returns stack with every entry dropped whose PaneID has already been
// spoken for — by the header band, or by an earlier entry in stack.
// It is the structural half of the same rule breakCycles enforces for parent chains: a corrupt
// persisted table must never be able to make Rules emit something the multiplexer answers
// destructively.
//
// A window_layout string names each pane exactly once. Emitting a pane number twice is not rejected
// by tmux — it accepts the string with exit 0, assigns cells positionally, and DESTROYS every pane
// the (now short) cell list no longer covers (verified live, tmux 3.6). The header is the case that
// makes this reachable rather than hypothetical, because bandHeader splices its cell in
// independently of the stack body and so cannot see a stack entry naming the same pane.
//
// The engine clears such bindings at load (clearConflictingPaneBindings), so in a healthy process
// this filter never removes anything. It exists so that the destructive outcome is impossible from
// inside this package's own contract rather than only prevented by a caller remembering to sanitize
// first — Rules is documented as a pure, TOTAL function over whatever strand set it is handed.
func removeDuplicatePaneCells(stack []Strand, headerPaneID string) []Strand {
	claimed := make(map[string]bool, len(stack)+1)
	if headerPaneID != "" {
		claimed[headerPaneID] = true
	}

	out := make([]Strand, 0, len(stack))
	for _, s := range stack {
		if claimed[s.PaneID] {
			continue
		}
		claimed[s.PaneID] = true
		out = append(out, s)
	}
	return out
}

// breakCycles returns a copy of strands with any cyclic parent chain broken,
// so a corrupt persisted table can never hang layout.
func breakCycles(strands []Strand) []Strand {
	byGUID := make(map[string]Strand, len(strands))
	for _, s := range strands {
		byGUID[s.GUID] = s
	}

	out := make([]Strand, len(strands))
	copy(out, strands)

	for _, s := range strands {
		if s.Parent == "" {
			continue
		}
		visited := map[string]bool{s.GUID: true}
		prev := s.GUID
		cur := s.Parent
		for cur != "" {
			if visited[cur] {
				// prev's parent link re-enters an already-visited node:
				// sever it here so the chain that led us to this repeat
				// terminates as a root.
				severParent(out, prev)
				break
			}
			visited[cur] = true
			parent, ok := byGUID[cur]
			if !ok {
				break // parent not present in this set — chain ends naturally
			}
			prev = cur
			cur = parent.Parent
		}
	}
	return out
}

// severParent clears guid's Parent field in out.
func severParent(out []Strand, guid string) {
	for i := range out {
		if out[i].GUID == guid {
			out[i].Parent = ""
			return
		}
	}
}

// orderStack returns stack ordered by parent-chain depth (roots first),
// preserving insertion order for siblings.
func orderStack(stack []Strand) []Strand {
	byGUID := make(map[string]Strand, len(stack))
	for _, s := range stack {
		byGUID[s.GUID] = s
	}

	depth := make(map[string]int, len(stack))
	for _, s := range stack {
		depth[s.GUID] = chainDepth(s, byGUID)
	}

	ordered := make([]Strand, len(stack))
	copy(ordered, stack)
	// sort.SliceStable preserves the original slice order among strands at
	// the same depth, which is exactly the "siblings ordered by insertion
	// order" rule.
	sort.SliceStable(ordered, func(i, j int) bool {
		return depth[ordered[i].GUID] < depth[ordered[j].GUID]
	})
	return ordered
}

// chainDepth counts hops from s up its parent chain until reaching a root or
// a parent not in this stack.
func chainDepth(s Strand, byGUID map[string]Strand) int {
	depth := 0
	cur := s.Parent
	for cur != "" {
		parent, ok := byGUID[cur]
		if !ok {
			break
		}
		depth++
		cur = parent.Parent
	}
	return depth
}
