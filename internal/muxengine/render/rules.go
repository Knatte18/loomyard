// rules.go composes the policy layer (policy.go, height.go, focus.go) and
// the mechanics layer (layout.go, checksum.go) into Rules, the package's
// single public entry point: a pure, total function from a strand set and a
// window Box to a tmux window_layout string and a focus target.

package render

import "fmt"

// Rules computes the tmux window_layout string and focus pane id for
// strands laid out within box, using p's height-policy knobs. It rejects
// any strand declaring AnchorOwnWindow with a non-nil error, since that
// anchor is deferred in v1. For any input that carries no AnchorOwnWindow
// strand, Rules is pure and total — a corrupt cyclic parent table is
// repaired by breakCycles rather than causing an error or a hang.
//
// paneOrder is the window's actual top-to-bottom pane order (pane ids as
// list-panes reports them, sorted by pane_top). psmux applies a layout
// string's cells POSITIONALLY to the window's current pane order and
// ignores the pane numbers embedded in the string for placement (its
// swap-pane/move-pane/join-pane are all silently non-functional, so panes
// cannot be physically reordered either) — the only way to steer a cell to
// a specific pane is to emit it at that pane's position. Rules therefore
// re-sequences its placements to follow paneOrder; every pane keeps its own
// strand's intended height, only the emission order bends to physical
// reality. A nil/empty paneOrder keeps the intended order (parent above
// child, top bands first) — correct whenever panes were created in table
// order, and the deterministic shape golden tests assert on.
func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout string, focus string, err error) {
	for _, s := range strands {
		if s.Display.Anchor == AnchorOwnWindow {
			return "", "", fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)
		}
	}

	// Repair any corrupt cyclic parent table before depth-based ordering,
	// so a bad persisted record can never hang layout.
	fixed := breakCycles(strands)
	top, stack := partitionByAnchor(fixed)
	ordered := orderStack(stack)

	// Reserve a fixed band at the top of box for each AnchorTop strand, each
	// followed by a one-row divider — except the very last top band's
	// divider, which is only reserved when a below-parent stack follows it
	// to consume the region past it. With >=2 top strands and zero stack
	// strands there would otherwise be nothing left to fill the remainder of
	// box.H (buildStackBody would then emit a window_layout string shorter
	// than box.H, which psmux's select-layout rejects — a failure surfacing
	// after launchStrandLocked already created the new pane, orphaning it;
	// orch_04 review 04, finding #2), so the last top band instead stretches
	// to absorb every leftover row: heights + dividers then always sum to
	// exactly box.H regardless of how many top/stack strands are present.
	placements := make([]placement, 0, len(top)+len(stack))
	y := box.Y
	for i, s := range top {
		height := p.TopBandRows
		if s.Display.TopBandRows > 0 {
			height = s.Display.TopBandRows
		}
		isLastTop := i == len(top)-1
		if isLastTop && len(ordered) == 0 {
			height = box.H - (y - box.Y)
		}
		placements = append(placements, placement{id: s.PaneID, height: height})
		y += height
		if !isLastTop || len(ordered) > 0 {
			y++ // one-row divider before the next top band or the stack region
		}
	}

	stackBox := Box{X: box.X, Y: y, W: box.W, H: box.H - (y - box.Y)}
	placements = append(placements, stackHeights(ordered, stackBox, p)...)

	placements = resequenceByPaneOrder(placements, paneOrder)

	body := buildStackBody(box, placements)
	// focus resolves over the below-parent order first, so the bottom
	// (active) pane is the default focus target. With no stack at all
	// (top-only layouts), the last top band — the stretched one — is the
	// fallback: leaving focus unset would let psmux park the active pane on
	// an arbitrary (often 1-row) band after select-layout, and the active
	// pane is both where an attaching operator lands and what an unguarded
	// split would target.
	focus = focusTarget(ordered)
	if focus == "" && len(top) > 0 {
		focus = top[len(top)-1].PaneID
	}
	return wrapLayout(body), focus, nil
}

// resequenceByPaneOrder reorders placements to follow paneOrder, the
// window's actual top-to-bottom pane order (see Rules' contract: psmux
// applies cells positionally and panes cannot be moved). Each placement
// keeps its pane id and height — only its position in the emitted body
// changes, and buildStackBody recomputes the y offsets from the new order,
// so heights + dividers still tile box.H exactly. Placements whose pane is
// absent from paneOrder keep their intended relative order at the tail
// (belt-and-suspenders — callers derive paneOrder from the same list-panes
// snapshot that marked those panes live). A nil/empty paneOrder returns
// placements unchanged.
func resequenceByPaneOrder(placements []placement, paneOrder []string) []placement {
	if len(paneOrder) == 0 || len(placements) < 2 {
		return placements
	}

	byID := make(map[string]placement, len(placements))
	for _, pl := range placements {
		byID[pl.id] = pl
	}

	out := make([]placement, 0, len(placements))
	taken := make(map[string]bool, len(placements))
	for _, id := range paneOrder {
		if pl, ok := byID[id]; ok && !taken[id] {
			out = append(out, pl)
			taken[id] = true
		}
	}
	for _, pl := range placements {
		if !taken[pl.id] {
			out = append(out, pl)
		}
	}
	return out
}
