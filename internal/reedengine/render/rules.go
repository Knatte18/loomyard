// rules.go composes the policy layer (policy.go, height.go, focus.go) and the mechanics layer
// (layout.go, checksum.go) into Rules, the package's single public entry point: a pure, total
// function from a strand set and a window Box to a tmux window_layout string and a focus target.

package render

import (
	"fmt"
	"strings"
)

// cellPlan is the policy-layer result Rules and FixedHeightPins share: everything decided about
// where strands land and how tall they are, before the mechanics layer (layout.go) turns it into a
// tmux window_layout string. planCells builds it; Rules and FixedHeightPins each read the parts they
// need and perform no policy of their own.
type cellPlan struct {
	// hasBand reports whether p.Selvage.PaneID is non-empty — the Selvage
	// band is being rendered at all.
	hasBand bool
	// soleBand reports whether the Selvage band claims the whole box as a
	// bracket-less single-cell body because no strand was placed. When true,
	// bandHeight, stackBox, ordered, and placements carry no meaning.
	soleBand bool
	// bandHeight is the Selvage band's height after clampBandHeight,
	// valid only when hasBand is true and soleBand is false.
	bandHeight int
	// stackBox is the region the strand stack is laid out within, above the
	// Selvage band and its one-row divider when hasBand is true.
	stackBox Box
	// ordered is the below-parent stack, filtered and ordered by parent-chain
	// depth.
	ordered []Strand
	// placements is ordered's per-strand height assignment from stackHeights.
	placements []placement
}

// planCells performs the policy half of Rules: filtering and ordering strands into the below-parent
// stack, and deciding the Selvage/stack height split. It returns the same AnchorOwnWindow rejection
// error Rules has always returned. Rules and FixedHeightPins are the mechanics layer built on top of
// this shared policy result.
func planCells(strands []Strand, box Box, p Params) (cellPlan, error) {
	for _, s := range strands {
		if s.Display.Anchor == AnchorOwnWindow {
			return cellPlan{}, fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)
		}
	}

	// Repair any corrupt cyclic parent table before depth-based ordering,
	// so a bad persisted record can never hang layout.
	fixed := breakCycles(strands)
	stack := removeDuplicatePaneCells(partitionByAnchor(fixed), p.Selvage.PaneID)
	ordered := orderStack(stack)

	hasBand := p.Selvage.PaneID != ""
	if hasBand && len(ordered) == 0 {
		// No strand placed: the Selvage band claims the whole box as a
		// bracket-less single-cell body (see Rules' doc comment) — never a
		// zero-height cell inside a group, which the real multiplexer
		// mishandles. No focus target exists without a placed strand.
		return cellPlan{hasBand: true, soleBand: true}, nil
	}

	stackBox := box
	bandHeight := 0
	if hasBand {
		// The Selvage band and the strand stack are physically adjacent
		// panes, so tmux/psmux always renders a one-row border between them
		// — the same budget buildStackBody already reserves between
		// individual strands (dividers := n-1). That row must come out of
		// the window's total budget before clampBandHeight (height.go)
		// decides the window-split: an oversized configured height_rows can
		// never shrink the strand stack below its MinFullRows floor — the
		// Selvage band yields rows first. clampToFit (called inside
		// stackHeights below) then distributes rows AMONG strands within
		// whatever (possibly clamped) stack region results.
		const bandDivider = 1
		bandHeight = clampBandHeight(p.Selvage.HeightRows, box.H-bandDivider, p.MinFullRows)
		stackBox = Box{X: box.X, Y: box.Y, W: box.W, H: box.H - bandHeight - bandDivider}
	}

	placements := stackHeights(ordered, stackBox, p)

	return cellPlan{
		hasBand:    hasBand,
		bandHeight: bandHeight,
		stackBox:   stackBox,
		ordered:    ordered,
		placements: placements,
	}, nil
}

// Rules computes the tmux window_layout string and focus pane id for strands laid out within box.
// It rejects any strand declaring AnchorOwnWindow, repairs corrupt cyclic parent chains, and drops
// any strand whose PaneID is already spoken for by the Selvage band or by an earlier strand
// (see removeDuplicatePaneCells for why emitting one pane number twice is destructive).
// When p.Selvage.PaneID is non-empty, Rules carves a fixed-height bottom band for the Selvage before
// laying out the stack above it.
// paneOrder resequences cells to match physical pane position;
// a nil paneOrder keeps the intended (parent above child) order.
func Rules(strands []Strand, box Box, p Params, paneOrder []string) (layout string, focus string, err error) {
	plan, err := planCells(strands, box, p)
	if err != nil {
		return "", "", err
	}

	if plan.soleBand {
		sole := fmt.Sprintf("%dx%d,%d,%d,%s", box.W, box.H, box.X, box.Y, strings.TrimPrefix(p.Selvage.PaneID, "%"))
		return wrapLayout(sole), "", nil
	}

	placements := resequenceByPaneOrder(plan.placements, paneOrder)

	body := buildStackBody(plan.stackBox, placements)
	if plan.hasBand {
		body = bandSelvage(box, p.Selvage.PaneID, plan.bandHeight, body)
	}
	focus = focusTarget(plan.ordered)
	return wrapLayout(body), focus, nil
}

// Pin is one pane whose height is an absolute row budget rather than "whatever is left" — the
// Selvage band or a collapsed strip. Height is the height Rules actually placed the cell at, after
// clampBandHeight/clampToFit — never the raw configured budget (p.Selvage.HeightRows or
// p.CollapsedStripRows read directly), since either can yield rows under a too-short window.
type Pin struct {
	// PaneID is the tmux pane id this pin applies to.
	PaneID string
	// Height is the row height Rules placed this pane's cell at.
	Height int
}

// FixedHeightPins reports the panes whose heights are absolute row budgets — the Selvage band and
// every collapsed strip — at the heights Rules actually placed them at for the identical
// (strands, box, p) inputs. It shares Rules' own policy composition (planCells) so the two can never
// disagree about a placed height.
//
// FixedHeightPins takes no paneOrder: a pin names its pane by tmux pane id, so emission order carries
// no geometry — paneOrder only resequences layout-string cells, which FixedHeightPins never produces.
//
// It is pure and total like Rules. On any error from planCells, on the sole-band shape (there the
// Selvage band claims the whole box and has no absolute budget of its own), or whenever there is
// otherwise nothing to report, it returns nil. A caller must treat a nil return as "nothing is
// pinned", never as "no opinion" — the disposition is exactly as authoritative as a non-nil one.
//
// The Selvage band pin, when present, is always first in the returned slice, ahead of every strip
// pin — hook-array index is fire order, not screen position, so the band pin keeps index 0 even
// though the band itself renders at the bottom of the window.
func FixedHeightPins(strands []Strand, box Box, p Params) []Pin {
	plan, err := planCells(strands, box, p)
	if err != nil || plan.soleBand {
		return nil
	}

	var pins []Pin
	if plan.hasBand {
		pins = append(pins, Pin{PaneID: p.Selvage.PaneID, Height: plan.bandHeight})
	}
	for _, pl := range plan.placements {
		if pl.strip {
			pins = append(pins, Pin{PaneID: pl.id, Height: pl.height})
		}
	}
	return pins
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
