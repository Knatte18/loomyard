// height.go implements the derived height policy for the below-parent stack, one rule for every strand:
// each strand except the bottom-most takes exactly Params.CollapsedRows rows, and the bottom-most
// takes every remaining row.
// When the window is too short to satisfy that natural policy, a strict-priority clamp reclaims
// rows so every pane still gets a positive height.

package render

// clampBandHeight returns bandRows clamped to preserve the strand-stack
// region's minStackRows floor, which the band yields first when the window
// cannot fit both.
func clampBandHeight(bandRows, windowRows, minStackRows int) int {
	if bandRows < 0 {
		bandRows = 0
	}
	if windowRows <= 0 {
		return 0
	}
	floor := minStackRows
	if floor < 1 {
		floor = 1
	}
	maxBand := windowRows - floor
	if maxBand < 1 {
		// Never fully starve the band once it exists: the stack's own
		// floor is the lesser of two structural violations when the window
		// cannot fit both, since a starved stack strand still renders
		// (clampToFit floors every strand at 1 row already) while a
		// zero-height band cell is mishandled by the real multiplexer.
		maxBand = 1
	}
	if maxBand > windowRows {
		maxBand = windowRows
	}
	if bandRows < 1 {
		bandRows = 1
	}
	if bandRows > maxBand {
		return maxBand
	}
	return bandRows
}

// stackHeights computes a height for every strand in stack within box.
// Every strand except the last (the bottom-most, which orderStack places by insertion) is a collapsed
// placement taking p.CollapsedRows rows; the last takes every remaining row.
// clampToFit reclaims rows if any would be non-positive.
func stackHeights(stack []Strand, box Box, p Params) []placement {
	n := len(stack)
	if n == 0 {
		return nil
	}

	dividers := n - 1
	usable := box.H - dividers
	bottomIdx := n - 1 // orderStack places the most recently inserted strand last

	collapsedRows := p.CollapsedRows
	if collapsedRows < 1 {
		collapsedRows = 1
	}

	heights := make([]int, n)
	isCollapsed := make([]bool, n)
	for i := range stack {
		if i == bottomIdx {
			continue
		}
		isCollapsed[i] = true
		heights[i] = collapsedRows
	}
	heights[bottomIdx] = usable - collapsedRows*(n-1)

	heights = clampToFit(heights, isCollapsed, bottomIdx)

	placements := make([]placement, n)
	for i, s := range stack {
		placements[i] = placement{id: s.PaneID, height: heights[i], strip: isCollapsed[i]}
	}
	return placements
}

// clampToFit repairs any non-positive height left by stackHeights' natural
// split, reclaiming rows from donors in strict priority order: collapsed
// placements first, then the bottom-most pane itself, all floored at 1.
func clampToFit(heights []int, isCollapsed []bool, bottomIdx int) []int {
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

	// Priority 1: collapsed placements shrink toward 1 row.
	for i := range heights {
		if borrowed == 0 {
			return heights
		}
		if !isCollapsed[i] {
			continue
		}
		give := heights[i] - 1
		if give <= 0 {
			continue
		}
		if give > borrowed {
			give = borrowed
		}
		heights[i] -= give
		borrowed -= give
	}
	if borrowed == 0 {
		return heights
	}

	// Last resort: the bottom-most pane itself absorbs whatever is still
	// owed. If the window is shorter than the pane count even this
	// cannot fully repay the debt, but the bottom pane is still floored
	// at 1 row so no height is ever non-positive.
	heights[bottomIdx] -= borrowed
	if heights[bottomIdx] < 1 {
		heights[bottomIdx] = 1
	}
	return heights
}
