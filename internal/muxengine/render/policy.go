// policy.go implements the anchor-to-placement dispatch: filtering strands
// down to the below-parent stack (or excluding them entirely), ordering the
// stack deterministically by parent-chain depth, and repairing a corrupt
// cyclic parent table so ordering always terminates. This is the legible
// half of the policy layer — adding a new anchor means adding a case here,
// not touching the mechanics layer in layout.go/checksum.go.

package render

import "sort"

// partitionByAnchor filters strands down to the below-parent stack. It is
// the single filter point that excludes a strand which is AnchorHidden, is
// not Live, or has an empty PaneID: render only ever lays out a strand that
// owns a present window pane, so it can never emit a paneNum built from an
// empty id. AnchorOwnWindow strands are also excluded — that anchor is
// deferred in v1; Rules is responsible for surfacing an error when one is
// present, not this function.
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

// breakCycles returns a copy of strands with any cyclic parent chain broken.
// For each strand it walks its own Parent chain with a fresh visited-set;
// the first repeat encountered means that strand's chain loops back on
// itself, so its own Parent link is severed (treated as a root) instead of
// letting the walk continue forever. This runs before orderStack so a
// corrupt persisted table — e.g. two strands parenting each other — can
// never hang layout; every strand's chain is guaranteed to terminate
// afterward.
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

// severParent clears guid's Parent field in out, identifying out's entry by
// GUID rather than index since out is a copy of the original slice in the
// same order.
func severParent(out []Strand, guid string) {
	for i := range out {
		if out[i].GUID == guid {
			out[i].Parent = ""
			return
		}
	}
}

// orderStack returns stack in deterministic parent-chain-depth order: roots
// first, then children, with siblings that share a parent kept in their
// original insertion order (their index in the input slice). Callers must
// pass an acyclic parent chain — breakCycles guarantees this upstream —
// orderStack itself does not defend against cycles.
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

// chainDepth counts hops from s up its parent chain until it reaches a
// strand with no parent, or a parent that is not part of this stack (its
// actual parent is AnchorHidden, deferred AnchorOwnWindow, not Live, or has
// an empty PaneID, and so was excluded by partitionByAnchor's filter).
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
