// layout.go is the layout mechanics layer: it turns a resolved, ordered list
// of pane placements within a Box into a tmux/psmux window_layout body and
// its checksum-prefixed full string. It is region-relative — offsets are
// anchored to box.X/box.Y rather than the whole window — so the top-band
// region and the below-parent stack region can each be rendered
// independently and then concatenated into one placements list. This file
// makes no placement or height decisions; those live in policy.go and
// height.go. It only renders the string from the placements it is given.

package render

import (
	"fmt"
	"strings"
)

// placement is one resolved pane: its psmux pane id and the row height it
// has been assigned. It is the internal handoff between the height policy
// (height.go) and the mechanics that render it (buildStackBody); callers of
// Rules never see it.
type placement struct {
	id     string
	height int
}

// buildStackBody renders panes — already placed top to bottom and sized —
// as a tmux window_layout body positioned within box:
// "<box.W>x<box.H>,<box.X>,<box.Y>[<w>x<h>,<x>,<y>,<paneNum>,...]". Each pane
// spans box.W at box.X; panes stack vertically with a one-row divider
// between them, with cumulative y starting at box.Y.
func buildStackBody(box Box, panes []placement) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d,%d,%d[", box.W, box.H, box.X, box.Y)

	y := box.Y
	for i, p := range panes {
		if i > 0 {
			b.WriteByte(',')
		}
		// paneNum is the bare pane number tmux's layout string expects —
		// the psmux pane id minus its leading '%'.
		fmt.Fprintf(&b, "%dx%d,%d,%d,%s", box.W, p.height, box.X, y, strings.TrimPrefix(p.id, "%"))
		y += p.height + 1 // advance past this pane and its one-row divider
	}
	b.WriteByte(']')
	return b.String()
}

// wrapLayout prefixes body with its tmux layout checksum, producing the full
// window_layout string psmux's select-layout accepts.
func wrapLayout(body string) string {
	return layoutChecksum(body) + "," + body
}
