// snapshot.go declares what a query of KWin reports, what an apply is told and what it reports back.

package kwinplace

import (
	"slices"
	"strings"
)

// konsoleResourceClass is the resource class KWin reports for a Konsole window on Wayland, Konsole's app_id.
const konsoleResourceClass = "org.kde.konsole"

// Rect is a rectangle in KWin's logical pixels.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Desktop is one virtual desktop, Order being its position among them from 0.
type Desktop struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
}

// Output is one screen KWin drives.
type Output struct {
	Name     string `json:"name"`
	Geometry Rect   `json:"geometry"`
}

// Tile is one leaf zone of a desktop's tiling grid, with the internal ids of the windows it holds.
type Tile struct {
	AbsoluteGeometry Rect     `json:"absoluteGeometry"`
	Padding          float64  `json:"padding"`
	Windows          []string `json:"windows"`
}

// Window is one normal window KWin manages.
// Desktops holds the ids of the desktops it is on, and Stacking its index in the stacking order, bottom first.
type Window struct {
	InternalID    string   `json:"internalId"`
	PID           int      `json:"pid"`
	ResourceClass string   `json:"resourceClass"`
	Caption       string   `json:"caption"`
	Desktops      []string `json:"desktops"`
	Output        string   `json:"output"`
	Minimized     bool     `json:"minimized"`
	Stacking      int      `json:"stacking"`
	Frame         Rect     `json:"frameGeometry"`
}

// Matches reports whether the window is the Konsole window of process pid whose caption holds title.
func (w Window) Matches(pid int, title string) bool {
	return w.PID == pid && w.ResourceClass == konsoleResourceClass && strings.Contains(w.Caption, title)
}

// Snapshot is KWin's state as one query saw it.
// Tiles maps a desktop id to the leaf zones of that desktop's grid on the tiling output, in reading order.
type Snapshot struct {
	CurrentDesktop   string            `json:"currentDesktop"`
	Desktops         []Desktop         `json:"desktops"`
	Outputs          []Output          `json:"outputs"`
	PerDesktopTiling bool              `json:"perDesktopTiling"`
	Windows          []Window          `json:"windows"`
	Tiles            map[string][]Tile `json:"tiles"`
}

func (s Snapshot) hasDesktop(id string) bool {
	return slices.ContainsFunc(s.Desktops, func(d Desktop) bool { return d.ID == id })
}

func zoneHolding(tiles []Tile, windowID string) (int, bool) {
	for i, tile := range tiles {
		if slices.Contains(tile.Windows, windowID) {
			return i, true
		}
	}
	return 0, false
}

// ApplyOrder tells an apply what to build: a Rows by Cols grid on the target Desktop of TilingOutput.
// Zones names the window of each zone in reading order, "" for an empty one.
// Placed names the window to move onto that desktop and output.
type ApplyOrder struct {
	TilingOutput string   `json:"tilingOutput"`
	Desktop      string   `json:"desktop"`
	Rows         int      `json:"rows"`
	Cols         int      `json:"cols"`
	Zones        []string `json:"zones"`
	Placed       string   `json:"placed"`
}

// AppliedWindow is one window as an apply reports it after building the grid.
// Zone is the index of the target desktop's zone that holds it, -1 for none.
// Tiled reports whether any tile of any desktop holds it.
// Frame may lag the window's place when read right after the apply.
type AppliedWindow struct {
	InternalID string   `json:"internalId"`
	Desktops   []string `json:"desktops"`
	Zone       int      `json:"zone"`
	Tiled      bool     `json:"tiled"`
	Frame      Rect     `json:"frameGeometry"`
}

// ApplyReport is what an apply reports: the target desktop's usable area, its zones and the windows.
type ApplyReport struct {
	Area    Rect
	Tiles   []Tile
	Windows []AppliedWindow
}
