// policy.go implements the anchor-to-placement dispatch: filtering strands down to the below-parent
// stack (or excluding them entirely), ordering the stack by insertion,
// and repairing a corrupt cyclic parent table so a persisted parent chain never hangs a caller.
// This is the legible half of the policy layer — adding a new anchor means adding a case here, not
// touching the mechanics layer in layout.go/checksum.go.

package render

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
// spoken for — by the Selvage band, or by an earlier entry in stack.
// It is the structural half of the same rule breakCycles enforces for parent chains: a corrupt
// persisted table must never be able to make Rules emit something the multiplexer answers
// destructively.
//
// A window_layout string names each pane exactly once. Emitting a pane number twice is not rejected
// by tmux — it accepts the string with exit 0, assigns cells positionally, and DESTROYS every pane
// the (now short) cell list no longer covers (verified live, tmux 3.6). The Selvage band is the case
// that makes this reachable rather than hypothetical, because bandSelvage splices its cell in
// independently of the stack body and so cannot see a stack entry naming the same pane.
//
// The engine clears such bindings at load (clearConflictingPaneBindings), so in a healthy process
// this filter never removes anything. It exists so that the destructive outcome is impossible from
// inside this package's own contract rather than only prevented by a caller remembering to sanitize
// first — Rules is documented as a pure, TOTAL function over whatever strand set it is handed.
func removeDuplicatePaneCells(stack []Strand, bandPaneID string) []Strand {
	claimed := make(map[string]bool, len(stack)+1)
	if bandPaneID != "" {
		claimed[bandPaneID] = true
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

// orderStack returns stack ordered by insertion — the persisted strand-table order the caller
// passes — so a newly added strand is the bottom-most and no strand above it moves.
// The parent chain plays no part in the order.
func orderStack(stack []Strand) []Strand {
	ordered := make([]Strand, len(stack))
	copy(ordered, stack)
	return ordered
}
