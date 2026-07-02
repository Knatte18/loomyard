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
func Rules(strands []Strand, box Box, p Params) (layout string, focus string, err error) {
	for _, s := range strands {
		if s.Display.Anchor == AnchorOwnWindow {
			return "", "", fmt.Errorf("render: strand %s uses deferred anchor %q", s.GUID, AnchorOwnWindow)
		}
	}

	// Repair any corrupt cyclic parent table before depth-based ordering,
	// so a bad persisted record can never hang layout.
	fixed := breakCycles(strands)
	top, stack := partitionByAnchor(fixed)

	// Reserve a fixed band at the top of box for each AnchorTop strand,
	// each followed by a one-row divider, before laying out the
	// below-parent stack in the remaining region.
	placements := make([]placement, 0, len(top)+len(stack))
	y := box.Y
	for _, s := range top {
		placements = append(placements, placement{id: s.PaneID, height: p.TopBandRows})
		y += p.TopBandRows + 1
	}

	ordered := orderStack(stack)
	stackBox := Box{X: box.X, Y: y, W: box.W, H: box.H - (y - box.Y)}
	placements = append(placements, stackHeights(ordered, stackBox, p)...)

	body := buildStackBody(box, placements)
	// focus resolves over the below-parent order only, mirroring muxpoc's
	// "always select the bottom (active) pane" default; it does not
	// consider the fixed top bands.
	return wrapLayout(body), focusTarget(ordered), nil
}
