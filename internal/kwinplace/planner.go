// planner.go plans a placement from a snapshot and the remembered places: target desktop, grid and window order.

package kwinplace

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/Knatte18/loomyard/internal/beamsetting"
)

// The most columns and rows a desktop's grid has, whatever the screen's size.
const (
	maxGridCols = 5
	maxGridRows = 2
)

// frameTolerance is how far, in logical pixels, a frame's corner may be from a zone's inset corner to start at it.
const frameTolerance = 1

// Grid is a tiling grid's size.
type Grid struct {
	Rows int
	Cols int
}

// Capacity returns the largest grid the screen takes with no zone smaller than the smallest tile.
// The grid is at least one by one and at most two rows by five columns.
// Both tile sizes are positive.
func Capacity(screen Rect, minTileWidth, minTileHeight int) Grid {
	fit := func(length float64, smallest, most int) int {
		return max(1, min(most, int(length)/smallest))
	}
	return Grid{Rows: fit(screen.Height, minTileHeight, maxGridRows), Cols: fit(screen.Width, minTileWidth, maxGridCols)}
}

// GridFor returns the grid for a desktop holding windows of them, within room:
// one row while they fit in the room's columns, otherwise two rows.
func GridFor(windows int, room Grid) Grid {
	if windows <= room.Cols {
		return Grid{Rows: 1, Cols: windows}
	}
	return Grid{Rows: 2, Cols: (windows + 1) / 2}
}

// Anchor returns the desktop the parent worktree's window anchors on, and whether its place is known yet.
// The place is known once the parent's entry is settled:
// the desktop its window is on now, or else the one it was last seen on, which may be "".
func Anchor(snap Snapshot, places Places, parent string) (string, bool) {
	entry, found := places[parent]
	if !found || !entry.Settled() {
		return "", false
	}
	for _, window := range snap.Windows {
		if window.Matches(entry.PID, entry.Title) && len(window.Desktops) == 1 {
			return window.Desktops[0], true
		}
	}
	return entry.LastDesktop, true
}

// CurrentOrder returns the ids of the windows sitting on one desktop in the order they stand in its zones.
// A window counts in the zone that holds it, and an untiled one in the zone whose inset corner its frame starts at.
// Windows in no zone come last, and windows sharing a place keep their stacking order.
func CurrentOrder(tiles []Tile, windows []Window) []string {
	zoneOf := func(window Window) int {
		if zone, ok := zoneHolding(tiles, window.InternalID); ok {
			return zone
		}
		for i, tile := range tiles {
			x, y := tile.AbsoluteGeometry.X+tile.Padding, tile.AbsoluteGeometry.Y+tile.Padding
			if math.Abs(window.Frame.X-x) <= frameTolerance && math.Abs(window.Frame.Y-y) <= frameTolerance {
				return i
			}
		}
		return len(tiles)
	}
	ordered := slices.SortedStableFunc(slices.Values(windows), func(a, b Window) int {
		return cmp.Or(cmp.Compare(zoneOf(a), zoneOf(b)), cmp.Compare(a.Stacking, b.Stacking))
	})
	ids := make([]string, 0, len(ordered))
	for _, window := range ordered {
		ids = append(ids, window.InternalID)
	}
	return ids
}

// PlanInput is what a placement is planned from.
type PlanInput struct {
	Snapshot Snapshot
	Places   Places
	// Mode is the window placement mode, one that places.
	Mode          string
	MinTileWidth  int
	MinTileHeight int
	TilingOutput  string
	// Worktree, PID and Title name the placed window.
	Worktree string
	PID      int
	Title    string
	// Prime reports that the placed window is the prime worktree's own, which has no parent to start after.
	Prime bool
	// Anchor is the desktop the mode counts from.
	Anchor string
	// AnchorFallback reports that Anchor stands in for a parent's unknown place.
	AnchorFallback bool
}

// Placement is what a plan decided.
type Placement struct {
	// Target is the desktop to place on, "" when the window is left where it is.
	Target string
	// Placed is the internal id of the placed window.
	Placed string
	Grid   Grid
	// Zones names the window of each zone of Grid in reading order, "" for an empty one.
	Zones []string
	// Overflowed reports that the window's home was full and Target is another desktop.
	Overflowed bool
	// Homeless reports that Target was counted from a fallback anchor, so the window takes no home.
	Homeless bool
	// Warning says why the window is left where it is, "" when it is placed.
	Warning string
}

// Plan returns where the placed window goes.
// The window starts at its home while that desktop exists.
// Otherwise it starts at a desktop counted from the anchor by the mode.
// It passes on to later desktops while one is full, so a full desktop is never the target.
// No other desktop's windows are named in the result.
func Plan(in PlanInput) Placement {
	var placed Window
	for _, window := range in.Snapshot.Windows {
		if window.Matches(in.PID, in.Title) {
			placed = window
		}
	}
	if placed.InternalID == "" {
		return Placement{Warning: fmt.Sprintf("window %q is not in KWin's report", in.Title)}
	}
	screen := slices.IndexFunc(in.Snapshot.Outputs, func(o Output) bool { return o.Name == in.TilingOutput })
	if screen < 0 {
		return Placement{Warning: fmt.Sprintf("KWin reports no output %q", in.TilingOutput)}
	}
	capacity := Capacity(in.Snapshot.Outputs[screen].Geometry, in.MinTileWidth, in.MinTileHeight)

	desktops := slices.SortedFunc(slices.Values(in.Snapshot.Desktops), func(a, b Desktop) int { return cmp.Compare(a.Order, b.Order) })
	indexOf := func(id string) int {
		return slices.IndexFunc(desktops, func(d Desktop) bool { return d.ID == id })
	}
	// counted returns the windows that take up room on the desktop, the placed one left out.
	counted := func(desktop string) []Window {
		var windows []Window
		for _, window := range in.Snapshot.Windows {
			if window.InternalID != placed.InternalID && !window.Minimized && window.Output == in.TilingOutput &&
				len(window.Desktops) == 1 && window.Desktops[0] == desktop {
				windows = append(windows, window)
			}
		}
		return windows
	}

	entry := in.Places[in.Worktree]
	start := indexOf(entry.Home)
	fromHome := start >= 0
	if !fromHome {
		start = indexOf(in.Anchor)
		if start < 0 {
			start = max(0, indexOf(in.Snapshot.CurrentDesktop))
		}
		if !in.Prime && in.Mode == beamsetting.WindowPlacementNext {
			start++
		}
	}

	for i := start; i < len(desktops); i++ {
		target := desktops[i].ID
		windows := counted(target)
		if len(windows) >= capacity.Rows*capacity.Cols {
			continue
		}
		grid := GridFor(len(windows)+1, capacity)
		zones := targetOrder(in, target, entry, placed, windows)
		for len(zones) < grid.Rows*grid.Cols {
			zones = append(zones, "")
		}
		return Placement{
			Target:     target,
			Placed:     placed.InternalID,
			Grid:       grid,
			Zones:      zones,
			Overflowed: fromHome && i != start,
			Homeless:   !fromHome && in.AnchorFallback,
		}
	}
	// Under next the start may lie past the last desktop, which then is the one to name.
	name := desktops[min(start, len(desktops)-1)].Name
	return Placement{Placed: placed.InternalID, Warning: fmt.Sprintf("every desktop from %s is full", name)}
}

// targetOrder returns the ids of the target desktop's windows and the placed one in zone order:
// the windows homed there by order key, then the rest as they stand, the placed one last unless the target is its home.
func targetOrder(in PlanInput, target string, entry Entry, placed Window, windows []Window) []string {
	// keys holds the order key of each window homed on the target.
	keys := map[string]int{}
	for _, window := range windows {
		for _, other := range in.Places {
			if other.Home == target && other.Worktree != in.Worktree && window.Matches(other.PID, other.Title) {
				keys[window.InternalID] = other.OrderKey
			}
		}
	}
	ids := append(CurrentOrder(in.Snapshot.Tiles[target], windows), placed.InternalID)
	if entry.Home == target {
		keys[placed.InternalID] = entry.OrderKey
	}
	slices.SortStableFunc(ids, func(a, b string) int {
		keyA, homedA := keys[a]
		keyB, homedB := keys[b]
		switch {
		case homedA && homedB:
			return cmp.Compare(keyA, keyB)
		case homedA:
			return -1
		case homedB:
			return 1
		}
		return 0
	})
	return ids
}
