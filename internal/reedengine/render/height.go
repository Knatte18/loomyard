// height.go implements the derived height policy: within the below-parent stack, a shrink:true
// ancestor collapses to a compact strip once it has a present descendant, and the active/bottom
// pane plus every shrink:false strand split the remaining rows equally (remainder to the active
// pane).
// When the window is too short to satisfy that natural policy, a strict-priority clamp reclaims
// rows so every pane still gets a positive height.

package render

// clampHeaderHeight returns headerRows clamped to preserve the strand-stack
// region's minStackRows floor, which the header yields first when the window
// cannot fit both.
func clampHeaderHeight(headerRows, windowRows, minStackRows int) int {
	if headerRows < 0 {
		headerRows = 0
	}
	if windowRows <= 0 {
		return 0
	}
	floor := minStackRows
	if floor < 1 {
		floor = 1
	}
	maxHeader := windowRows - floor
	if maxHeader < 1 {
		// Never fully starve the header once it exists: the stack's own
		// floor is the lesser of two structural violations when the window
		// cannot fit both, since a starved stack strand still renders
		// (clampToFit floors every strand at 1 row already) while a
		// zero-height header cell is mishandled by the real multiplexer.
		maxHeader = 1
	}
	if maxHeader > windowRows {
		maxHeader = windowRows
	}
	if headerRows < 1 {
		headerRows = 1
	}
	if headerRows > maxHeader {
		return maxHeader
	}
	return headerRows
}

// stackHeights computes a height for every strand in stack within box.
// Shrink:true ancestors collapse to a strip height, full panes split the
// remainder equally (with the remainder to the active pane), and clampToFit
// reclaims rows if any would be non-positive.
func stackHeights(stack []Strand, box Box, p Params) []placement {
	n := len(stack)
	if n == 0 {
		return nil
	}

	dividers := n - 1
	usable := box.H - dividers
	activeIdx := n - 1 // orderStack places the deepest/active strand last

	isStrip := make([]bool, n)
	numStrips := 0
	for i, s := range stack {
		isStrip[i] = isAncestor(s, stack) && s.Display.ShrinkWhenWaitingOnChild
		if isStrip[i] {
			numStrips++
		}
	}
	numFull := n - numStrips

	stripRows := p.CollapsedStripRows
	if stripRows < 1 {
		stripRows = 1
	}
	stripDemand := numStrips * stripRows
	fullRemaining := usable - stripDemand

	var fullBase, fullRemainder int
	if numFull > 0 {
		fullBase = fullRemaining / numFull
		fullRemainder = fullRemaining % numFull
	}

	heights := make([]int, n)
	for i := range stack {
		if isStrip[i] {
			heights[i] = stripRows
		} else {
			heights[i] = fullBase
		}
	}
	// The remainder always goes to the active/bottom pane, never split
	// arbitrarily across full panes — this is what makes the split
	// deterministic when usable/numFull does not divide evenly.
	heights[activeIdx] += fullRemainder

	heights = clampToFit(heights, isStrip, activeIdx, p)

	placements := make([]placement, n)
	for i, s := range stack {
		placements[i] = placement{id: s.PaneID, height: heights[i], strip: isStrip[i]}
	}
	return placements
}

// clampToFit repairs any non-positive height left by stackHeights' natural
// split, reclaiming rows from donors in strict priority order: strips first,
// then non-active full panes, then the active pane itself, all floored at 1.
func clampToFit(heights []int, isStrip []bool, activeIdx int, p Params) []int {
	minFull := p.MinFullRows
	if minFull < 1 {
		minFull = 1
	}

	// Bring every non-positive pane up to 1 row, tracking how many rows
	// this borrows so the priority passes below can give them back from
	// elsewhere and keep the total exactly conserved.
	borrowed := 0
	for i, h := range heights {
		if h < 1 {
			borrowed += 1 - h
			heights[i] = 1
		}
	}
	if borrowed == 0 {
		return heights
	}

	reclaim := func(floor int, skip func(i int) bool) {
		for i := range heights {
			if borrowed == 0 {
				return
			}
			if skip(i) {
				continue
			}
			give := heights[i] - floor
			if give <= 0 {
				continue
			}
			if give > borrowed {
				give = borrowed
			}
			heights[i] -= give
			borrowed -= give
		}
	}

	// Priority 1: strips shrink toward 1 row.
	reclaim(1, func(i int) bool { return !isStrip[i] })
	if borrowed == 0 {
		return heights
	}

	// Priority 2: full panes other than the active one shrink toward
	// MinFullRows.
	reclaim(minFull, func(i int) bool { return isStrip[i] || i == activeIdx })
	if borrowed == 0 {
		return heights
	}

	// Priority 3: every remaining donor (excluding the active pane)
	// clamps all the way to 1 row.
	reclaim(1, func(i int) bool { return i == activeIdx })
	if borrowed == 0 {
		return heights
	}

	// Last resort: the active pane itself absorbs whatever is still
	// owed. If the window is shorter than the pane count even this
	// cannot fully repay the debt, but the active pane is still floored
	// at 1 row so no height is ever non-positive.
	heights[activeIdx] -= borrowed
	if heights[activeIdx] < 1 {
		heights[activeIdx] = 1
	}
	return heights
}
