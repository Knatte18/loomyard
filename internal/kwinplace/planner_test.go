// planner_test.go pins the grid sizes, the parent anchor, the current order and the planned placement.

package kwinplace

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCapacityAndGrid(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	screens := []struct {
		width, height float64
		minW, minH    int
		want          Grid
	}{
		{2560, 1440, 500, 600, Grid{Rows: 2, Cols: 5}},
		{1536, 960, 500, 600, Grid{Rows: 1, Cols: 3}},
		{2560, 1440, 1, 1, Grid{Rows: 2, Cols: 5}},
		{400, 300, 500, 600, Grid{Rows: 1, Cols: 1}},
	}
	for _, s := range screens {
		if got := Capacity(Rect{Width: s.width, Height: s.height}, s.minW, s.minH); got != s.want {
			t.Errorf("Capacity(%vx%v at %dx%d) = %+v; want %+v", s.width, s.height, s.minW, s.minH, got, s.want)
		}
	}
	for windows, want := range map[int]Grid{3: {Rows: 1, Cols: 3}, 6: {Rows: 2, Cols: 3}, 7: {Rows: 2, Cols: 4}} {
		if got := GridFor(windows, Grid{Rows: 2, Cols: 5}); got != want {
			t.Errorf("GridFor(%d) = %+v; want %+v", windows, got, want)
		}
	}
}

func TestAnchor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	parent := Entry{Worktree: "parent", PID: 5, Title: "P", SettledBy: 5, LastDesktop: "d2"}
	live := Window{PID: 5, ResourceClass: konsoleResourceClass, Caption: "P — Konsole", Desktops: []string{"d4"}}
	unsettledParent := parent
	unsettledParent.SettledBy = 0
	tests := []struct {
		name    string
		windows []Window
		places  Places
		want    string
		known   bool
	}{
		{"LiveWindow", []Window{live}, Places{"parent": parent}, "d4", true},
		{"NoLiveWindow", nil, Places{"parent": parent}, "d2", true},
		{"Unsettled", []Window{live}, Places{"parent": unsettledParent}, "", false},
		{"NoEntry", []Window{live}, Places{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, known := Anchor(Snapshot{Windows: tt.windows}, tt.places, "parent")
			if got != tt.want || known != tt.known {
				t.Errorf("Anchor() = %q, %v; want %q, %v", got, known, tt.want, tt.known)
			}
		})
	}
}

// zone returns a 100 by 100 zone at x holding the named windows, with a padding of 4.
func zone(x float64, windows ...string) Tile {
	return Tile{AbsoluteGeometry: Rect{X: x, Width: 100, Height: 100}, Padding: 4, Windows: windows}
}

func TestCurrentOrder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	window := func(id string, stacking int) Window { return Window{InternalID: id, Stacking: stacking} }
	startingAt := func(id string, stacking int, x float64) Window {
		return Window{InternalID: id, Stacking: stacking, Frame: Rect{X: x, Y: 4}}
	}
	tests := []struct {
		name    string
		tiles   []Tile
		windows []Window
		want    []string
	}{
		{"TiledInZoneOrder", []Tile{zone(0, "b"), zone(100, "a")}, []Window{window("a", 0), window("b", 1)}, []string{"b", "a"}},
		{
			"UntiledAtAZoneCornerCountsInThatZone",
			[]Tile{zone(0), zone(100, "b")},
			[]Window{window("b", 0), startingAt("a", 1, 4)},
			[]string{"a", "b"},
		},
		{
			"UntiledInNoZoneGoLastByStacking",
			[]Tile{zone(0, "c")},
			[]Window{startingAt("b", 2, 50), startingAt("a", 1, 60), window("c", 3)},
			[]string{"c", "a", "b"},
		},
		{"NoZonesOrdersByStacking", nil, []Window{window("b", 2), window("a", 1)}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := CurrentOrder(tt.tiles, tt.windows); !slices.Equal(got, tt.want) {
				t.Errorf("CurrentOrder() = %q; want %q", got, tt.want)
			}
		})
	}
}

// planWindow is a window of a planned desktop, its process id being n and its title "T<n>".
func planWindow(n int, desktops ...string) Window {
	return Window{
		InternalID:    "w" + strconv.Itoa(n),
		PID:           n,
		ResourceClass: konsoleResourceClass,
		Caption:       "T" + strconv.Itoa(n) + "!",
		Desktops:      desktops,
		Output:        "DP",
		Stacking:      n,
	}
}

func homed(n int, home string, key int) Entry {
	return Entry{Worktree: "t" + strconv.Itoa(n), PID: n, Title: "T" + strconv.Itoa(n) + "!", SettledBy: n, Home: home, OrderKey: key}
}

func TestPlan(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// fill returns windows numbered from first on one desktop.
	fill := func(desktop string, first, count int) []Window {
		var windows []Window
		for n := first; n < first+count; n++ {
			windows = append(windows, planWindow(n, desktop))
		}
		return windows
	}
	otherOutput := planWindow(2, "d1")
	otherOutput.Output = "HDMI"
	minimized := planWindow(3, "d1")
	minimized.Minimized = true

	// The screen takes one row of three, and the placed window is number 1, still on d3.
	tests := []struct {
		name     string
		windows  []Window
		places   []Entry
		mode     string
		prime    bool
		fallback bool
		anchor   string
		gone     bool

		target     string
		zones      []string
		overflowed bool
		homeless   bool
		warning    string
	}{
		{
			name:    "UncountedWindowsTakeNoRoom",
			windows: []Window{otherOutput, minimized, planWindow(4, "d1", "d2"), planWindow(5, "d1")},
			mode:    "same", anchor: "d1",
			target: "d1", zones: []string{"w5", "w1"},
		},
		{
			name:    "OneBelowCapacityIsNotFull",
			windows: fill("d1", 2, 2),
			mode:    "same", anchor: "d1",
			target: "d1", zones: []string{"w2", "w3", "w1"},
		},
		{
			name:    "KeyedWindowsEndInKeyOrder",
			windows: []Window{planWindow(2, "d1"), planWindow(3, "d1")},
			places:  []Entry{homed(1, "d1", 1), homed(2, "d1", 2), homed(3, "d1", 0)},
			mode:    "next", anchor: "d2",
			target: "d1", zones: []string{"w3", "w1", "w2"},
		},
		{
			name:    "UnhomedWindowsFollowInCurrentOrder",
			windows: []Window{planWindow(3, "d1"), planWindow(2, "d1")},
			places:  []Entry{homed(2, "d2", 0), homed(3, "d1", 5)},
			mode:    "same", anchor: "d1",
			target: "d1", zones: []string{"w3", "w2", "w1"},
		},
		{
			name:    "EqualKeysKeepTheCurrentOrder",
			windows: []Window{planWindow(3, "d1"), planWindow(2, "d1")},
			places:  []Entry{homed(2, "d1", 0), homed(3, "d1", 0)},
			mode:    "same", anchor: "d1",
			target: "d1", zones: []string{"w2", "w3", "w1"},
		},
		{
			name:    "NextPassesAFullDesktop",
			windows: fill("d2", 2, 3),
			mode:    "next", anchor: "d1",
			target: "d3", zones: []string{"w1"},
		},
		{
			name:    "PastTheLastDesktopIsNoTarget",
			windows: append(fill("d2", 2, 3), fill("d3", 5, 3)...),
			mode:    "same", anchor: "d2",
			warning: "every desktop from B1 is full",
		},
		{
			name: "NextFromTheLastDesktopIsNoTarget",
			mode: "next", anchor: "d3",
			warning: "every desktop from B2 is full",
		},
		{
			name:   "AnAnchorNamingNoDesktopStartsAtTheCurrentOne",
			mode:   "same",
			target: "d1", zones: []string{"w1"},
		},
		{
			name:   "HomeWinsOverTheMode",
			places: []Entry{homed(1, "d2", 0)},
			mode:   "next", anchor: "d1",
			target: "d2", zones: []string{"w1"},
		},
		{
			name:   "GoneHomeFallsToTheAnchor",
			places: []Entry{homed(1, "d9", 0)},
			mode:   "same", anchor: "d2", fallback: true,
			target: "d2", zones: []string{"w1"}, homeless: true,
		},
		{
			name:    "FullHomeOverflows",
			windows: fill("d1", 2, 3),
			places:  []Entry{homed(1, "d1", 0)},
			mode:    "same", anchor: "d3",
			target: "d2", zones: []string{"w1"}, overflowed: true,
		},
		{
			name:  "PrimeUnderNextStartsAtTheAnchor",
			mode:  "next",
			prime: true, anchor: "d2",
			target: "d2", zones: []string{"w1"},
		},
		{
			name:    "OtherDesktopsAreNotInTheOrder",
			windows: []Window{planWindow(2, "d1"), planWindow(3, "d2")},
			mode:    "same", anchor: "d2",
			target: "d2", zones: []string{"w3", "w1"},
		},
		{
			name: "AWindowKWinDoesNotReportIsNotPlanned",
			mode: "same", anchor: "d1", gone: true,
			warning: "not in KWin's report",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			places := Places{}
			for _, entry := range tt.places {
				places[entry.Worktree] = entry
			}
			windows := slices.Clone(tt.windows)
			if !tt.gone {
				windows = append(windows, planWindow(1, "d3"))
			}
			got := Plan(PlanInput{
				Snapshot: Snapshot{
					CurrentDesktop: "d1",
					Desktops:       []Desktop{{ID: "d1", Name: "B0", Order: 0}, {ID: "d2", Name: "B1", Order: 1}, {ID: "d3", Name: "B2", Order: 2}},
					Outputs:        []Output{{Name: "DP", Geometry: Rect{Width: 1536, Height: 960}}},
					Windows:        windows,
				},
				Places:         places,
				Mode:           tt.mode,
				MinTileWidth:   500,
				MinTileHeight:  600,
				TilingOutput:   "DP",
				Worktree:       "t1",
				PID:            1,
				Title:          "T1!",
				Prime:          tt.prime,
				Anchor:         tt.anchor,
				AnchorFallback: tt.fallback,
			})

			if tt.warning != "" {
				if got.Target != "" || !strings.Contains(got.Warning, tt.warning) {
					t.Errorf("Plan() = %+v; want no target and a warning holding %q", got, tt.warning)
				}
				return
			}
			want := Placement{
				Target:     tt.target,
				Placed:     "w1",
				Grid:       Grid{Rows: 1, Cols: len(tt.zones)},
				Zones:      tt.zones,
				Overflowed: tt.overflowed,
				Homeless:   tt.homeless,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Plan() = %+v; want %+v", got, want)
			}
		})
	}
}
