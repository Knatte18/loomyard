// rules.go composes the policy layer (policy.go, height.go, focus.go) and
// the mechanics layer (layout.go, checksum.go) into Rules, the package's
// single public entry point: a pure, total function from a strand set and a
// window Box to a tmux window_layout string and a focus target.

package render

import (
	"fmt"
	"strings"
)

// Rules computes the tmux window_layout string and focus pane id for
// strands laid out within box, using p's height-policy knobs. It rejects
// any strand declaring AnchorOwnWindow with a non-nil error, since that
// anchor is deferred in v1. For any input that carries no AnchorOwnWindow
// strand, Rules is pure and total — a corrupt cyclic parent table is
// repaired by breakCycles rather than causing an error or a hang.
//
// When p.Header.PaneID is non-empty AND at least one strand is actually
// placed, Rules first carves a fixed-height top band for the header —
// {X:0, Y:0, W:box.W, H:headerHeight} — one divider row for the physical
// border tmux/psmux always renders between vertically adjacent panes
// (mirroring the same one-row-per-gap budget buildStackBody already reserves
// BETWEEN strands), and lays the below-parent stack out in the shrunk region
// below both, {X:0, Y:headerHeight+1, W:box.W, H:box.H-headerHeight-1} — so
// the emitted window_layout enumerates the header cell plus every strand
// cell (the live-pane count the caller's select-layout must match) with
// every cell's declared height leaving room for tmux's own border row.
// Verified against a real tmux instance: omitting this divider budget still
// makes select-layout return success, but tmux inserts the border row
// anyway, pushing every subsequent pane down by one and overflowing the
// bottom of the window by exactly one row (contract_integration_test.go's
// TestHeaderNeverGetsZeroHeightLayoutCell pins this). headerHeight is
// clampHeaderHeight's output, not the raw configured value, called with the
// divider already subtracted from the window's row budget: an oversized
// height_rows can never shrink the strand region below its MinFullRows
// floor, so the header itself yields rows first when the window is too
// short for both — but headerHeight itself is never clamped below 1 row
// (clampHeaderHeight never hands this a zero, since a real tmux/psmux
// select-layout does not cleanly support a genuinely zero-height cell
// either). When no strand is actually placed (an empty stack — the header
// is the window's sole mux-owned pane), no divider is reserved and the
// header claims the whole box, emitted as a bracket-less single-cell body
// (the same shape tmux itself reports for a one-pane window) rather than a
// zero-height cell inside a group; in practice this case never reaches
// select-layout at all, since applyLayoutLocked skips both tmux calls
// whenever fewer than two panes are live or no strand owns a present pane,
// but the pure function's output must still honor the never-zero-height
// rule for any caller. The header is never itself a Strand — it is injected
// here at the Params seam instead of being modelled in the strand slice
// (Shared Decision header-is-not-a-strand) — so partitionByAnchor/orderStack
// below never see it. A zero-value p.Header (empty PaneID) skips all of
// this and Rules behaves exactly as it did before the header pane existed.
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
// child) — correct whenever panes were created in table order, and the
// deterministic shape golden tests assert on.
func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout string, focus string, err error) {
	for _, s := range strands {
		if s.Display.Anchor == AnchorOwnWindow {
			return "", "", fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)
		}
	}

	// Repair any corrupt cyclic parent table before depth-based ordering,
	// so a bad persisted record can never hang layout.
	fixed := breakCycles(strands)
	stack := partitionByAnchor(fixed)
	ordered := orderStack(stack)

	hasHeader := p.Header.PaneID != ""
	if hasHeader && len(ordered) == 0 {
		// No strand placed: the header claims the whole box as a
		// bracket-less single-cell body (see the doc comment) — never a
		// zero-height cell inside a group, which the real multiplexer
		// mishandles. No focus target exists without a placed strand.
		sole := fmt.Sprintf("%dx%d,%d,%d,%s", box.W, box.H, box.X, box.Y, strings.TrimPrefix(p.Header.PaneID, "%"))
		return wrapLayout(sole), "", nil
	}

	stackBox := box
	headerHeight := 0
	if hasHeader {
		// The header and the strand stack are physically adjacent panes, so
		// tmux/psmux always renders a one-row border between them — the same
		// budget buildStackBody already reserves between individual strands
		// (dividers := n-1). That row must come out of the window's total
		// budget before clampHeaderHeight (height.go) decides the
		// window-split: an oversized configured height_rows can never
		// shrink the strand stack below its MinFullRows floor — the header
		// yields rows first. clampToFit (called inside stackHeights below)
		// then distributes rows AMONG strands within whatever (possibly
		// clamped) stack region results.
		const headerDivider = 1
		headerHeight = clampHeaderHeight(p.Header.HeightRows, box.H-headerDivider, p.MinFullRows)
		stackBox = Box{X: box.X, Y: box.Y + headerHeight + headerDivider, W: box.W, H: box.H - headerHeight - headerDivider}
	}

	placements := stackHeights(ordered, stackBox, p)
	placements = resequenceByPaneOrder(placements, paneOrder)

	body := buildStackBody(stackBox, placements)
	if hasHeader {
		body = bandHeader(box, p.Header.PaneID, headerHeight, body)
	}
	focus = focusTarget(ordered)
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
