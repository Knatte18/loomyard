// layout.go is the layout mechanics layer: it turns a resolved, ordered list of pane placements
// within a Box into a tmux/psmux window_layout body and its checksum-prefixed full string.
// It is region-relative — offsets are anchored to box.X/box.Y rather than the whole window — so the
// stack region can be rendered independently of the Box it is placed within.
// This file makes no placement or height decisions;
// those live in policy.go and height.go.
// It only renders the string from the placements it is given.

package render

import (
	"fmt"
	"strings"
)

// placement is one resolved pane: its tmux pane id and the row height it
// has been assigned. It is the internal handoff between the height policy
// (height.go) and the mechanics that render it (buildStackBody); callers of
// Rules never see it.
type placement struct {
	id     string
	height int
	// strip reports whether this cell's height came from the collapsed-strip
	// budget (an absolute row budget, p.CollapsedStripRows post-clamp) rather
	// than from the equal-split of whatever rows were left. buildStackBody
	// must not read it — it exists for FixedHeightPins (rules.go) to identify
	// which placements to report.
	strip bool
}

// buildStackBody renders panes into a tmux window_layout body positioned
// within box: "<box.W>x<box.H>,<box.X>,<box.Y>[<w>x<h>,<x>,<y>,<paneNum>,...]}".
// Each pane spans box.W, stacking vertically with a one-row divider.
func buildStackBody(box Box, panes []placement) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d,%d,%d[", box.W, box.H, box.X, box.Y)

	y := box.Y
	for i, p := range panes {
		if i > 0 {
			b.WriteByte(',')
		}
		// paneNum is the bare pane number tmux's layout string expects —
		// the tmux pane id minus its leading '%'.
		fmt.Fprintf(&b, "%dx%d,%d,%d,%s", box.W, p.height, box.X, y, strings.TrimPrefix(p.id, "%"))
		y += p.height + 1 // advance past this pane and its one-row divider
	}
	b.WriteByte(']')
	return b.String()
}

// wrapLayout prefixes body with its tmux layout checksum, producing the full
// window_layout string tmux's select-layout accepts.
func wrapLayout(body string) string {
	return layoutChecksum(body) + "," + body
}

// bandSelvage appends a fixed-height Selvage cell to stackBody's pane group,
// producing the full window_layout body when the Selvage band is present.
// stackBody must be a region-relative body string from buildStackBody, not
// checksum-wrapped; this function only splices the band cell on at the end
// and re-wraps at fullBox's dimensions.
func bandSelvage(fullBox Box, selvagePaneID string, bandHeight int, stackBody string) string {
	open := strings.IndexByte(stackBody, '[')
	closeIdx := strings.LastIndexByte(stackBody, ']')

	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d,%d,%d[", fullBox.W, fullBox.H, fullBox.X, fullBox.Y)
	if inner := stackBody[open+1 : closeIdx]; inner != "" {
		b.WriteString(inner)
		b.WriteByte(',')
	}
	fmt.Fprintf(&b, "%dx%d,%d,%d,%s", fullBox.W, bandHeight, fullBox.X, fullBox.Y+fullBox.H-bandHeight, strings.TrimPrefix(selvagePaneID, "%"))
	b.WriteByte(']')
	return b.String()
}
