// height.go implements the derived height policy for the placed cells, top-anchored first.
// A content cell takes its reported line count, bounded below by its minimum and above by its share of the free height.
// Expanding cells are every live sharing cell, else the bottom-most positional cell, else the bottom-most cell whatever its kind.
// They split the rows the fixed cells leave evenly, and the bottom-most expanding cell takes the remainder.
// Every other positional cell takes exactly Params.CollapsedRows rows.
// When the window is too short to satisfy that natural policy, a strict-priority clamp reclaims rows so every pane still gets a positive height.

package render

// cellRole is how stackHeights treats a cell: which rule sets its height and which clamp stage may reclaim rows from it.
type cellRole int

const (
	// roleCollapsed is a positional cell that is not expanding; it takes Params.CollapsedRows.
	roleCollapsed cellRole = iota
	// roleContent is a content cell that is not expanding; it takes its content height.
	roleContent cellRole = 1
	// roleExpanding is a cell that splits the rows the fixed cells leave.
	roleExpanding cellRole = 2
)

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
// Stack holds the placed cells in order, top-anchored first, and box is the stack region.
// A content cell takes min(R, max(m, floor(box.H × share))) rows, with m its MinRows and R its ContentRows (m while unreported).
// The first cell adds one row when p.TitleRow.
// Expanding cells are every sharing cell, else the bottom-most positional cell, else the bottom-most cell.
// They split the rows the fixed cells leave evenly, and the bottom-most expanding cell takes the remainder.
// Every other positional cell takes p.CollapsedRows rows.
// clampToFit reclaims rows if any would be non-positive.
func stackHeights(stack []Strand, box Box, p Params) []placement {
	n := len(stack)
	if n == 0 {
		return nil
	}

	usable := box.H - (n - 1)

	collapsedRows := p.CollapsedRows
	if collapsedRows < 1 {
		collapsedRows = 1
	}

	roles := cellRoles(stack)
	heights := make([]int, n)
	fixedRows := 0
	var expanding []int
	for i, s := range stack {
		switch roles[i] {
		case roleCollapsed:
			heights[i] = collapsedRows
		case roleContent:
			heights[i] = contentHeight(s, box.H, i == 0 && p.TitleRow)
		case roleExpanding:
			expanding = append(expanding, i)
			continue
		}
		fixedRows += heights[i]
	}
	if len(expanding) > 0 {
		free := usable - fixedRows
		each := free / len(expanding)
		for _, i := range expanding {
			heights[i] = each
		}
		last := expanding[len(expanding)-1]
		heights[last] = free - each*(len(expanding)-1)
	}

	heights = clampToFit(heights, roles)

	placements := make([]placement, n)
	for i, s := range stack {
		isContent := s.Display.Size.Kind == SizeContent
		placements[i] = placement{id: s.PaneID, height: heights[i], strip: roles[i] == roleCollapsed || isContent}
	}
	return placements
}

// cellRoles assigns each cell of stack its role.
// Expanding cells are every sharing cell when there is one, else the bottom-most positional cell when there is one, else the bottom-most cell whatever its kind.
func cellRoles(stack []Strand) []cellRole {
	roles := make([]cellRole, len(stack))
	anySharing := false
	bottomPositional := -1
	for i, s := range stack {
		switch s.Display.Size.Kind {
		case SizeSharing:
			anySharing = true
		case SizePositional:
			bottomPositional = i
		}
	}

	for i, s := range stack {
		switch s.Display.Size.Kind {
		case SizeContent:
			roles[i] = roleContent
		case SizeSharing:
			roles[i] = roleExpanding
		default:
			roles[i] = roleCollapsed
		}
		if !anySharing && i == bottomPositional {
			roles[i] = roleExpanding
		}
	}
	if !anySharing && bottomPositional < 0 {
		roles[len(stack)-1] = roleExpanding
	}
	return roles
}

// contentHeight is a content cell's height: its reported line count, at least its minimum and at most its share of the stack region's rows.
// It adds the title row when the cell sits at window row 0.
func contentHeight(s Strand, regionRows int, titleRow bool) int {
	size := s.Display.Size
	reported := s.ContentRows
	if reported == 0 {
		reported = size.MinRows
	}
	rows := min(reported, max(size.MinRows, regionRows*size.ShareNum/size.ShareDen))
	if titleRow {
		rows++
	}
	return rows
}

// clampToFit repairs any non-positive height left by stackHeights' natural split.
// It reclaims rows from donors in strict priority order: collapsed cells first, then content cells, then expanding cells from the bottom up, all floored at 1.
func clampToFit(heights []int, roles []cellRole) []int {
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

	// Priority 1 and 2: collapsed cells, then content cells, shrink toward 1 row.
	for _, role := range []cellRole{roleCollapsed, roleContent} {
		for i := range heights {
			if borrowed == 0 {
				return heights
			}
			if roles[i] == role {
				borrowed -= reclaimRows(heights, i, borrowed)
			}
		}
	}

	// Last resort: the expanding cells absorb whatever is still owed, bottom-most first.
	// If the window is shorter than the pane count even this cannot fully repay the debt,
	// but every cell is still floored at 1 row so no height is ever non-positive.
	for i := len(heights) - 1; i >= 0 && borrowed > 0; i-- {
		if roles[i] == roleExpanding {
			borrowed -= reclaimRows(heights, i, borrowed)
		}
	}
	return heights
}

// reclaimRows takes up to owed rows from heights[i], never below 1, and returns how many it took.
func reclaimRows(heights []int, i, owed int) int {
	give := min(heights[i]-1, owed)
	if give <= 0 {
		return 0
	}
	heights[i] -= give
	return give
}
