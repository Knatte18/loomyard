// rules.go composes the policy layer (policy.go, height.go, focus.go) and the mechanics layer
// (layout.go, checksum.go) into Rules, the package's single public entry point: a pure, total
// function from a strand set and a window Box to a tmux window_layout string and a focus target.

package render

import (
	"fmt"
	"strings"
)

// Rules computes the tmux window_layout string and focus pane id for strands laid out within box.
// It rejects any strand declaring AnchorOwnWindow, repairs corrupt cyclic parent chains, and drops
// any strand whose PaneID is already spoken for by the header band or by an earlier strand
// (see removeDuplicatePaneCells for why emitting one pane number twice is destructive).
// When p.Header.PaneID is non-empty, Rules carves a fixed-height top band for the header before
// laying out the stack below.
// paneOrder resequences cells to match physical pane position;
// a nil paneOrder keeps the intended (parent above child) order.
func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout string, focus string, err error) {
	for _, s := range strands {
		if s.Display.Anchor == AnchorOwnWindow {
			return "", "", fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)
		}
	}

	// Repair any corrupt cyclic parent table before depth-based ordering,
	// so a bad persisted record can never hang layout.
	fixed := breakCycles(strands)
	stack := removeDuplicatePaneCells(partitionByAnchor(fixed), p.Header.PaneID)
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

// resequenceByPaneOrder reorders placements to follow paneOrder.
// Each placement keeps its pane id and height; only the emission order
// changes so buildStackBody recomputes y offsets correctly.
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
