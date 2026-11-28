// retile.go puts a hub's open windows back on their home desktops and in their zones, run by hand.

package kwinplace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Why a retile does not place an open window.
const (
	reasonPlacing   = "its placement is still running"
	reasonOff       = "window placement is off for its worktree"
	reasonMinimized = "it is minimized"
	reasonNoHome    = "it has no home desktop"
	reasonHomeGone  = "its home desktop no longer exists"
	reasonFull      = "its home desktop has no zone left for it"
	reasonNotLanded = "KWin did not put it on its home desktop"
)

// RetileRequest is a hub whose open windows go back to their remembered places.
type RetileRequest struct {
	// BoltDir is the hub's Bolt checkout, under which the remembered places are kept.
	BoltDir string
	// MinTileWidth and MinTileHeight are the smallest zone a grid may have, both positive.
	MinTileWidth  int
	MinTileHeight int
	// Mode returns the window placement mode in force for the window of a worktree: off, same or next.
	Mode func(worktree string) (string, error)
}

// RetiledWindow is a window of the hub that sits in its zone on its home desktop.
type RetiledWindow struct {
	Worktree string `json:"worktree"`
	// Desktop is the name of the window's home desktop.
	Desktop string `json:"desktop"`
}

// UnplacedWindow is an open window of the hub that a retile did not place on its home, and why.
type UnplacedWindow struct {
	Worktree string `json:"worktree"`
	Reason   string `json:"reason"`
}

// RetileReport is what a retile did with each open window of the hub.
// A window whose worktree has an entry in the places file but whose window is not open is in none of the lists.
type RetileReport struct {
	// Retiled holds the windows put in their zones on desktops whose grid was built anew.
	Retiled []RetiledWindow `json:"retiled"`
	// InPlace holds the windows of desktops whose grid already stood as the retile would build it.
	InPlace []RetiledWindow `json:"in_place"`
	// NotPlaced holds the windows not placed on their homes.
	NotPlaced []UnplacedWindow `json:"not_placed"`
}

// homeGrid is the grid a retile builds on one home desktop.
type homeGrid struct {
	desktop Desktop
	order   ApplyOrder
	// members are the hub's windows the grid holds, in zone order.
	members []homedWindow
	// inPlace reports that the desktop's grid already stands as order would build it.
	inPlace bool
}

// homedWindow is an open window of the hub that goes back to its home desktop.
type homedWindow struct {
	worktree string
	entry    Entry
	window   Window
}

// Retile puts each open window of the hub back on its home desktop, tiled in its zone in order-key order.
// It holds the placement lock throughout, so no placement runs between its query and its applies.
// Each home desktop's grid holds the hub's windows homed there first, then the desktop's other windows as they stand;
// a window of no hub's entry is never moved off its desktop.
// A desktop whose grid already holds its windows so is left untouched.
// A window is left where it is when its placement is still running or its worktree's mode is off or cannot be read.
// It is also left when it is minimized, has no home that is still a desktop, or has a home with no zone left for it.
// A window already on its home keeps its zone there over one coming from another desktop.
// An error from the screen, the lock or the query means no window was placed.
// A ctx that ends during the applies is returned with the report of what was done until then.
// An apply that fails leaves that desktop's windows unplaced, naming the failure,
// and the other desktops are still retiled.
// A window that did not get to its home may still sit on a desktop whose grid was built.
// It is put back into that grid after the desktop's other windows.
// Its reason says so when the grid has no zone for it.
// The windows put on their home are recorded as last seen there; no home or order key changes.
func (f Flow) Retile(ctx context.Context, req RetileRequest) (RetileReport, error) {
	tilingOutput, err := f.ReadTilingOutput(ctx)
	if err != nil {
		return RetileReport{}, fmt.Errorf("kwinplace: read the tiling screen: %w", err)
	}
	held, taken, err := acquireWithin(ctx, f.lockPath, f.lockWait, f.acquireLock)
	if err != nil {
		return RetileReport{}, fmt.Errorf("kwinplace: placement lock: %w", err)
	}
	if !taken {
		return RetileReport{}, fmt.Errorf("kwinplace: placement lock %s not free within %s of waiting", f.lockPath, f.lockWait)
	}
	defer func() {
		if err := held.Release(); err != nil {
			logger.Warn("kwinplace: placement lock not released", "path", f.lockPath, "error", err)
		}
	}()

	snap, _, err := f.Query(ctx, tilingOutput, f.now(), defaultScriptLockWait)
	if err != nil {
		return RetileReport{}, err
	}
	if !snap.PerDesktopTiling {
		return RetileReport{}, errors.New("kwinplace: retiling is unsupported: KWin does not tile each desktop on its own")
	}
	screen := slices.IndexFunc(snap.Outputs, func(o Output) bool { return o.Name == tilingOutput })
	if screen < 0 {
		return RetileReport{}, fmt.Errorf("kwinplace: KWin reports no output %q", tilingOutput)
	}
	capacity := Capacity(snap.Outputs[screen].Geometry, req.MinTileWidth, req.MinTileHeight)

	path := PlacesPath(req.BoltDir)
	grids, moving, unplaced := planRetile(snap, ReadPlaces(path), tilingOutput, capacity, req.Mode)
	report := RetileReport{Retiled: []RetiledWindow{}, InPlace: []RetiledWindow{}, NotPlaced: unplaced}
	var landed []homedWindow
	// Deferred after the lock's release, so it runs before it, however the applies end.
	defer func() {
		if err := recordRetiled(path, landed, tilingOutput); err != nil {
			logger.Warn("kwinplace: retiled windows not recorded", "error", err)
		}
	}()

	// applied holds the grids KWin built, and stranded the windows that did not get to their home.
	var applied []homeGrid
	var stranded []homedWindow
	for _, grid := range grids {
		if grid.inPlace {
			report.InPlace = append(report.InPlace, retiledWindows(grid)...)
			continue
		}
		applyReport, err := f.Apply(ctx, grid.order)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return report, ctxErr
			}
			for _, member := range grid.members {
				report.NotPlaced = append(report.NotPlaced, UnplacedWindow{Worktree: member.worktree, Reason: err.Error()})
			}
			stranded = append(stranded, grid.members...)
			continue
		}
		applied = append(applied, grid)
		for _, member := range grid.members {
			if !landedOn(applyReport, member.window.InternalID, grid.desktop.ID) {
				report.NotPlaced = append(report.NotPlaced, UnplacedWindow{Worktree: member.worktree, Reason: reasonNotLanded})
				stranded = append(stranded, member)
				continue
			}
			report.Retiled = append(report.Retiled, RetiledWindow{Worktree: member.worktree, Desktop: grid.desktop.Name})
			landed = append(landed, member)
		}
	}

	if len(stranded) > 0 && len(applied) > 0 {
		if err := f.putBack(ctx, snap, tilingOutput, capacity, applied, stranded, moving, report.NotPlaced); err != nil {
			return report, err
		}
	}
	return report, nil
}

// putBack builds again each applied grid whose desktop a stranded window still sits on, with that window in it.
// It adds to the window's reason in unplaced when the window gets no zone back.
// The only error it returns is that of an ended ctx.
// snap is the one the grids were planned from, and moving holds the internal ids of the windows they hold.
func (f Flow) putBack(ctx context.Context, snap Snapshot, tilingOutput string, capacity Grid, applied []homeGrid, stranded []homedWindow, moving map[string]bool, unplaced []UnplacedWindow) error {
	// A failed apply may have moved its windows all the same, so where they sit is read again.
	fresh, _, err := f.Query(ctx, tilingOutput, f.now(), defaultScriptLockWait)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		logger.Warn("kwinplace: windows not read again, taking them to sit where they did", "error", err)
	default:
		snap = fresh
	}

	givenBack := map[string]bool{}
	for _, window := range stranded {
		delete(moving, window.window.InternalID)
		givenBack[window.window.InternalID] = true
	}
	for _, grid := range applied {
		var back []homedWindow
		for _, window := range stranded {
			if slices.ContainsFunc(snap.Windows, func(w Window) bool {
				return w.InternalID == window.window.InternalID && sitsOn(w, grid.desktop.ID, tilingOutput)
			}) {
				back = append(back, window)
			}
		}
		if len(back) == 0 {
			continue
		}
		// Never nil: the apply script reads a list.
		moved := []string{}
		for _, member := range grid.members {
			if moving[member.window.InternalID] {
				moved = append(moved, member.window.InternalID)
			}
		}
		order := gridOrder(snap, grid.desktop.ID, tilingOutput, capacity, moved, moving, givenBack)
		applyReport, err := f.Apply(ctx, order)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for _, window := range back {
			id := window.window.InternalID
			var more string
			switch {
			case err != nil:
				more = "; the grid of the desktop it sits on was not built again around it: " + err.Error()
			case !slices.Contains(order.Zones, id):
				more = "; the desktop it sits on has no zone left for it"
			case !slices.ContainsFunc(applyReport.Windows, func(w AppliedWindow) bool { return w.InternalID == id && w.Tiled }):
				more = "; KWin did not tile it again where it sits"
			default:
				continue
			}
			i := slices.IndexFunc(unplaced, func(u UnplacedWindow) bool { return u.Worktree == window.worktree })
			unplaced[i].Reason += more
		}
	}
	return nil
}

// landedOn reports whether the apply report shows the window id on desktop alone.
func landedOn(report ApplyReport, id, desktop string) bool {
	return slices.ContainsFunc(report.Windows, func(w AppliedWindow) bool {
		return w.InternalID == id && slices.Equal(w.Desktops, []string{desktop})
	})
}

// planRetile returns, in desktop order, the grid of each home desktop that has a window of the hub to hold.
// It also returns the internal ids of the windows those grids hold.
// Its last result lists the open windows of the hub that are left where they are.
func planRetile(snap Snapshot, places Places, tilingOutput string, capacity Grid, mode func(string) (string, error)) ([]homeGrid, map[string]bool, []UnplacedWindow) {
	unplaced := []UnplacedWindow{}
	byHome := map[string][]homedWindow{}
	// moving holds the internal id of every window of the hub that a grid holds.
	moving := map[string]bool{}
	for _, worktree := range slices.Sorted(maps.Keys(places)) {
		entry := places[worktree]
		window, open := windowIn(snap, entry.PID, entry.Title)
		if !open {
			continue
		}
		if reason := leftReason(snap, entry, window, mode); reason != "" {
			unplaced = append(unplaced, UnplacedWindow{Worktree: worktree, Reason: reason})
			continue
		}
		byHome[entry.Home] = append(byHome[entry.Home], homedWindow{worktree: worktree, entry: entry, window: window})
		moving[window.InternalID] = true
	}

	desktops := slices.SortedFunc(slices.Values(snap.Desktops), func(a, b Desktop) int { return cmp.Compare(a.Order, b.Order) })
	for _, members := range byHome {
		slices.SortFunc(members, func(a, b homedWindow) int {
			return cmp.Or(cmp.Compare(a.entry.OrderKey, b.entry.OrderKey), cmp.Compare(a.worktree, b.worktree))
		})
	}
	// A window that its full home leaves out stays where it sits and takes room there,
	// so the homes are trimmed until none changes.
	for trimmed := true; trimmed; {
		trimmed = false
		for _, desktop := range desktops {
			members := byHome[desktop.ID]
			room := max(0, capacity.Rows*capacity.Cols-len(otherWindows(snap, desktop.ID, tilingOutput, moving)))
			if len(members) <= room {
				continue
			}
			kept, left := trimToRoom(members, room, desktop.ID, tilingOutput)
			byHome[desktop.ID] = kept
			for _, member := range left {
				delete(moving, member.window.InternalID)
				unplaced = append(unplaced, UnplacedWindow{Worktree: member.worktree, Reason: reasonFull})
			}
			trimmed = true
		}
	}

	var grids []homeGrid
	for _, desktop := range desktops {
		members := byHome[desktop.ID]
		if len(members) == 0 {
			continue
		}
		var moved []string
		for _, member := range members {
			moved = append(moved, member.window.InternalID)
		}
		order := gridOrder(snap, desktop.ID, tilingOutput, capacity, moved, moving, nil)
		grids = append(grids, homeGrid{
			desktop: desktop,
			order:   order,
			members: members,
			inPlace: gridStands(snap.Tiles[desktop.ID], order.Zones, members, desktop.ID, tilingOutput),
		})
	}
	return grids, moving, unplaced
}

// trimToRoom splits the members of a home, in zone order, into the room of them it keeps and the rest.
// Members already on the home are kept first, and the kept ones stay in zone order.
func trimToRoom(members []homedWindow, room int, desktop, tilingOutput string) (kept, left []homedWindow) {
	var sitting, coming []homedWindow
	for _, member := range members {
		if sitsOn(member.window, desktop, tilingOutput) {
			sitting = append(sitting, member)
		} else {
			coming = append(coming, member)
		}
	}
	left = append(sitting, coming...)[room:]
	for _, member := range members {
		if !slices.ContainsFunc(left, func(l homedWindow) bool { return l.worktree == member.worktree }) {
			kept = append(kept, member)
		}
	}
	return kept, left
}

// sitsOn reports whether the window is on the tiling output and on desktop alone.
func sitsOn(window Window, desktop, tilingOutput string) bool {
	return window.Output == tilingOutput && slices.Equal(window.Desktops, []string{desktop})
}

// otherWindows returns the windows that take up room on desktop beside the ones moving names.
func otherWindows(snap Snapshot, desktop, tilingOutput string, moving map[string]bool) []Window {
	var others []Window
	for _, window := range snap.Windows {
		if !moving[window.InternalID] && !window.Minimized && sitsOn(window, desktop, tilingOutput) {
			others = append(others, window)
		}
	}
	return others
}

// gridOrder returns the apply of desktop's grid: the moved windows first, then its other windows as they stand.
// The other windows givenBack names come last.
// The grid has no more zones than capacity, so windows past it get none.
func gridOrder(snap Snapshot, desktop, tilingOutput string, capacity Grid, moved []string, moving, givenBack map[string]bool) ApplyOrder {
	zones := slices.Clone(moved)
	var last []string
	for _, id := range CurrentOrder(snap.Tiles[desktop], otherWindows(snap, desktop, tilingOutput, moving)) {
		if givenBack[id] {
			last = append(last, id)
			continue
		}
		zones = append(zones, id)
	}
	zones = append(zones, last...)
	zones = zones[:min(len(zones), capacity.Rows*capacity.Cols)]
	grid := GridFor(len(zones), capacity)
	for len(zones) < grid.Rows*grid.Cols {
		zones = append(zones, "")
	}
	return ApplyOrder{
		TilingOutput: tilingOutput,
		Desktop:      desktop,
		Rows:         grid.Rows,
		Cols:         grid.Cols,
		Zones:        zones,
		Moved:        moved,
	}
}

// leftReason returns why the open window of entry stays where it is, "" when it goes back to its home.
func leftReason(snap Snapshot, entry Entry, window Window, mode func(string) (string, error)) string {
	if !entry.Settled() {
		return reasonPlacing
	}
	inForce, err := mode(entry.Worktree)
	switch {
	case err != nil:
		return err.Error()
	case inForce == beamsetting.WindowPlacementOff:
		return reasonOff
	case window.Minimized:
		return reasonMinimized
	case entry.Home == "":
		return reasonNoHome
	case !snap.hasDesktop(entry.Home):
		return reasonHomeGone
	}
	return ""
}

// gridStands reports whether tiles already hold zones one window to a zone, with every member on desktop alone.
// A one by one grid has no leaf zone to read, so it never stands.
func gridStands(tiles []Tile, zones []string, members []homedWindow, desktop, tilingOutput string) bool {
	if len(zones) < 2 || len(tiles) != len(zones) {
		return false
	}
	for _, member := range members {
		if !sitsOn(member.window, desktop, tilingOutput) {
			return false
		}
	}
	for i, id := range zones {
		if id == "" {
			if len(tiles[i].Windows) != 0 {
				return false
			}
			continue
		}
		if !slices.Equal(tiles[i].Windows, []string{id}) {
			return false
		}
	}
	return true
}

func retiledWindows(grid homeGrid) []RetiledWindow {
	windows := make([]RetiledWindow, 0, len(grid.members))
	for _, member := range grid.members {
		windows = append(windows, RetiledWindow{Worktree: member.worktree, Desktop: grid.desktop.Name})
	}
	return windows
}

// recordRetiled records each window put on its home as last seen there, on the tiling output.
// An entry that has since taken a newer window is left unchanged.
func recordRetiled(path string, landed []homedWindow, tilingOutput string) error {
	if len(landed) == 0 {
		return nil
	}
	return writePlaces(path, func(places Places) Places {
		for _, member := range landed {
			entry, found := places[member.worktree]
			if !found || entry.PID != member.entry.PID {
				continue
			}
			entry.LastDesktop, entry.Output = member.entry.Home, tilingOutput
			places[member.worktree] = entry
		}
		return places
	})
}
