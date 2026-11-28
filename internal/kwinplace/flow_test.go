// flow_test.go pins the placement flow's waits, lock, records and command over hand-built snapshots and a fake clock.

package kwinplace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestMode(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		perTask, machine, want string
	}{
		{"", "same", "same"},
		{"next", "off", "next"},
		{"off", "same", "off"},
	}
	for _, tt := range tests {
		if got := Mode(tt.perTask, tt.machine); got != tt.want {
			t.Errorf("Mode(%q, %q) = %q; want %q", tt.perTask, tt.machine, got, tt.want)
		}
	}
}

// parentName is the worktree the placed window of a harness is placed from.
const parentName = "t9"

type queryCall struct {
	at     time.Time
	after  time.Time
	within time.Duration
}

// flowHarness drives one flow over a fake clock that only its sleeps, queries and lock acquires move.
// The placed window is number 1 of worktree t1, and windows lists what every query reports.
type flowHarness struct {
	t     *testing.T
	flow  Flow
	req   Request
	path  string
	start time.Time
	now   time.Time

	windows  []Window
	desktops []Desktop
	current  string
	tiles    map[string][]Tile
	tiled    bool
	// worktrees is what the request's worktree list answers.
	worktrees []string
	// afterErr is what the after-place call returns.
	afterErr error

	queryTakes time.Duration
	// onQuery runs before the query numbered n, from 1, is answered.
	onQuery func(n int)
	// onSleep runs after the sleep numbered n, from 1.
	onSleep func(n int)
	// report answers an apply; its error fails it.
	report  func(order ApplyOrder) (ApplyReport, error)
	onAfter func(ctx context.Context)

	events  []string
	queries []queryCall
	sleeps  []time.Duration
	applies []ApplyOrder
	afters  []context.Context
}

func newFlowHarness(t *testing.T) *flowHarness {
	t.Helper()
	boltDir := t.TempDir()
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	h := &flowHarness{
		t:        t,
		path:     PlacesPath(boltDir),
		start:    start,
		now:      start,
		windows:  []Window{planWindow(1, "d3")},
		desktops: []Desktop{{ID: "d1", Name: "B0", Order: 0}, {ID: "d2", Name: "B1", Order: 1}, {ID: "d3", Name: "B2", Order: 2}},
		current:  "d1",
		tiled:    true,

		worktrees: []string{"t1", "t2", "t3", "t4", parentName},
		req: Request{
			BoltDir:       boltDir,
			Worktree:      "t1",
			Title:         "T1!",
			PID:           1,
			Parent:        parentName,
			Mode:          "same",
			MinTileWidth:  500,
			MinTileHeight: 600,
		},
	}
	h.req.Worktrees = func() ([]string, error) { return h.worktrees, nil }
	h.flow = Flow{
		Query: func(_ context.Context, _ string, after time.Time, within time.Duration) (Snapshot, time.Time, error) {
			h.events = append(h.events, "query")
			h.queries = append(h.queries, queryCall{at: h.now, after: after, within: within})
			if h.onQuery != nil {
				h.onQuery(len(h.queries))
			}
			h.now = h.now.Add(h.queryTakes)
			return Snapshot{
				CurrentDesktop:   h.current,
				Desktops:         h.desktops,
				Outputs:          []Output{{Name: "DP", Geometry: Rect{Width: 1536, Height: 960}}},
				PerDesktopTiling: h.tiled,
				Windows:          slices.Clone(h.windows),
				Tiles:            h.tiles,
			}, h.now, nil
		},
		Apply: func(_ context.Context, order ApplyOrder) (ApplyReport, error) {
			h.events = append(h.events, "apply")
			h.applies = append(h.applies, order)
			if h.report != nil {
				return h.report(order)
			}
			return ApplyReport{Windows: []AppliedWindow{{InternalID: order.Placed, Desktops: []string{order.Desktop}}}}, nil
		},
		ReadTilingOutput: func(context.Context) (string, error) { return "DP", nil },
		afterPlace: func(ctx context.Context) error {
			h.events = append(h.events, "after")
			h.afters = append(h.afters, ctx)
			if h.onAfter != nil {
				h.onAfter(ctx)
			}
			return h.afterErr
		},
		now: func() time.Time { return h.now },
		sleep: func(_ context.Context, d time.Duration) {
			h.sleeps = append(h.sleeps, d)
			h.now = h.now.Add(d)
			if h.onSleep != nil {
				h.onSleep(len(h.sleeps))
			}
		},
		lockPath:    filepath.Join(t.TempDir(), placementLockFileName),
		acquireLock: lock.AcquireWriteLock,
		windowWait:  defaultWindowWait,
		parentWait:  defaultParentWait,
		lockWait:    defaultLockWait,
	}
	return h
}

// seed writes one entry to the places file.
func (h *flowHarness) seed(entry Entry) {
	h.t.Helper()
	if err := writePlaces(h.path, func(places Places) Places {
		places[entry.Worktree] = entry
		return places
	}); err != nil {
		h.t.Fatal(err)
	}
}

// parentOn records the parent's window as placed on desktop.
func (h *flowHarness) parentOn(desktop string) {
	h.seed(Entry{Worktree: parentName, PID: 9, Title: "T9!", SettledBy: 9, LastDesktop: desktop})
}

// reopen records a new window of worktree, which leaves its entry unsettled.
func (h *flowHarness) reopen(worktree string, pid int) {
	h.t.Helper()
	if err := RecordOpen(h.path, worktree, pid, "T!", h.now); err != nil {
		h.t.Fatal(err)
	}
}

func (h *flowHarness) place() error {
	return h.flow.Place(context.Background(), h.req)
}

// wantEnded checks that the flow settled the window's entry, applied applies times and ran its command once.
func (h *flowHarness) wantEnded(applies int) {
	h.t.Helper()
	if entry := ReadPlaces(h.path)["t1"]; !entry.Settled() {
		h.t.Errorf("entry = %+v; want it settled", entry)
	}
	if len(h.applies) != applies || len(h.afters) != 1 {
		h.t.Errorf("applies, after-place calls = %d, %d; want %d, 1", len(h.applies), len(h.afters), applies)
	}
}

func (h *flowHarness) wantTarget(desktop string) {
	h.t.Helper()
	if len(h.applies) != 1 || h.applies[0].Desktop != desktop {
		h.t.Errorf("applies = %+v; want one on %q", h.applies, desktop)
	}
}

// underAPanel answers an apply on a screen with a panel:
// every zone runs under the panel, and each window stays in its tile, its frame ending above it.
func underAPanel(order ApplyOrder) (ApplyReport, error) {
	report := ApplyReport{Area: Rect{Width: 1536, Height: 920}}
	for i, id := range order.Zones {
		x := float64(i) * 512
		report.Tiles = append(report.Tiles, Tile{AbsoluteGeometry: Rect{X: x, Width: 512, Height: 960}, Padding: 4, Windows: []string{id}})
		report.Windows = append(report.Windows, AppliedWindow{
			InternalID: id, Desktops: []string{order.Desktop}, Zone: i, Tiled: true,
			Frame: Rect{X: x + 4, Y: 4, Width: 504, Height: 912},
		})
	}
	return report, nil
}

// seedSitting records window n as settled on desktop d2, its home, under order key key.
func (h *flowHarness) seedSitting(n, key int) {
	h.t.Helper()
	entry := homed(n, "d2", key)
	entry.LastDesktop = "d2"
	h.seed(entry)
}

// wantKeys checks the order key of each named worktree.
func (h *flowHarness) wantKeys(want map[string]int) {
	h.t.Helper()
	places := ReadPlaces(h.path)
	for worktree, key := range want {
		if got := places[worktree].OrderKey; got != key {
			h.t.Errorf("order key of %s = %d; want %d", worktree, got, key)
		}
	}
}

func wantWarning(t *testing.T, err error, holding string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), holding) {
		t.Errorf("Place() error = %v; want one holding %q", err, holding)
	}
}

func TestFlow_Place(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	run := func(name string, body func(t *testing.T, h *flowHarness)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body(t, newFlowHarness(t))
		})
	}

	run("PlacesRecordsThenRunsTheCommand", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		var atCommand Entry
		h.onAfter = func(context.Context) { atCommand = ReadPlaces(h.path)["t1"] }
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantEnded(1)
		want := ApplyOrder{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 1, Zones: []string{"w1"}, Placed: "w1"}
		if got := h.applies[0]; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("apply order = %+v; want %+v", got, want)
		}
		if atCommand.Home != "d2" || !atCommand.Settled() {
			t.Errorf("entry at the command = %+v; want it settled with home d2", atCommand)
		}
		if want := []string{"query", "query", "apply", "after"}; !slices.Equal(h.events, want) {
			t.Errorf("events = %q; want %q", h.events, want)
		}
	})

	run("AWindowThatNeverAppears", func(t *testing.T, h *flowHarness) {
		h.windows = nil
		h.queryTakes = time.Second / 2
		wantWarning(t, h.place(), "did not appear")
		h.wantEnded(0)
		for i, call := range h.queries {
			after := h.start
			if i > 0 {
				after = h.queries[i-1].at.Add(h.queryTakes)
				if pause := call.at.Sub(after); pause < time.Second {
					t.Errorf("query %d started %s after the previous one ended; want at least 1s", i, pause)
				}
			}
			if want := h.start.Add(10 * time.Second).Sub(call.at); !call.after.Equal(after) || call.within != want {
				t.Errorf("query %d = after %v within %s; want after %v within %s", i, call.after, call.within, after, want)
			}
		}
		if len(h.queries) < 2 {
			t.Errorf("queries = %d; want the window queried repeatedly", len(h.queries))
		}
	})

	run("TheLockedQueryTakesNoOlderReport", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		h.flow.acquireLock = func(path string) (*lock.FileLock, error) {
			h.now = h.now.Add(5 * time.Second)
			return lock.AcquireWriteLock(path)
		}
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		want := queryCall{at: h.start.Add(5 * time.Second), after: h.start.Add(5 * time.Second), within: 30 * time.Second}
		if len(h.queries) != 2 || h.queries[1] != want {
			t.Errorf("queries = %+v; want a second one %+v", h.queries, want)
		}
	})

	run("ALockHeldPastTheBudget", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		holder, err := lock.AcquireWriteLock(h.flow.lockPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = holder.Release() })
		h.flow.lockWait = 50 * time.Millisecond
		wantWarning(t, h.place(), "placement lock")
		h.wantEnded(0)
		if got := ReadPlaces(h.path)["t1"].LastDesktop; got != "d3" {
			t.Errorf("last desktop = %q; want d3, where the window was seen", got)
		}
	})

	run("OneLockBudgetCoversEveryAcquire", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		acquires := 0
		h.flow.acquireLock = func(path string) (*lock.FileLock, error) {
			acquires++
			h.now = h.now.Add(defaultLockWait)
			return lock.AcquireWriteLock(path)
		}
		h.onQuery = func(n int) {
			if n == 2 {
				h.reopen(parentName, 19)
			}
		}
		h.onSleep = func(int) { h.parentOn("d2") }
		wantWarning(t, h.place(), "placement lock")
		h.wantEnded(0)
		if acquires != 1 {
			t.Errorf("acquires = %d; want 1, the budget being spent", acquires)
		}
	})

	run("AFailedApply", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		h.report = func(ApplyOrder) (ApplyReport, error) { return ApplyReport{}, errors.New("apply broke") }
		wantWarning(t, h.place(), "apply broke")
		h.wantEnded(1)
		if entry := ReadPlaces(h.path)["t1"]; entry.LastDesktop != "d3" || entry.Home != "" {
			t.Errorf("entry = %+v; want last desktop d3 and no home", entry)
		}
	})

	run("AnUnwritablePlacesFileStillRunsTheCommand", func(t *testing.T, h *flowHarness) {
		h.req.Prime, h.req.Parent = true, ""
		dir := filepath.Dir(h.path)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		permkit.Chmod(t, dir, 0o500)
		if err := h.place(); err == nil {
			t.Error("Place() error = nil; want the failed record")
		}
		if len(h.afters) != 1 {
			t.Errorf("after-place calls = %d; want 1", len(h.afters))
		}
	})

	run("NoRoomAnywhereReleasesTheLock", func(t *testing.T, h *flowHarness) {
		h.parentOn("d1")
		for i, desktop := range []string{"d1", "d2", "d3"} {
			for n := range 3 {
				h.windows = append(h.windows, planWindow(100+3*i+n, desktop))
			}
		}
		h.onAfter = func(context.Context) {
			held, free, err := lock.TryAcquireWriteLock(h.flow.lockPath)
			if err != nil || !free {
				t.Errorf("placement lock free = %v, %v; want it released before the command", free, err)
				return
			}
			_ = held.Release()
		}
		wantWarning(t, h.place(), "is full")
		h.wantEnded(0)
	})

	run("ReopeningUnderAPanelKeepsEveryKey", func(t *testing.T, h *flowHarness) {
		// Window 4 was homed on d2 and has since been moved to d1 by hand.
		h.windows = []Window{planWindow(1, "d3"), planWindow(2, "d2"), planWindow(3, "d2"), planWindow(4, "d1")}
		// The stacking order runs against the zone order, so only the report's tiles give the keys.
		for i, stacking := range []int{3, 2, 1} {
			h.windows[i].Stacking = stacking
		}
		for n, key := range []int{0, 1, 2, 3} {
			h.seedSitting(n+1, key)
		}
		h.report = underAPanel
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantKeys(map[string]int{"t1": 0, "t2": 1, "t3": 2})
		if moved := ReadPlaces(h.path)["t4"]; moved.Home != "d1" || moved.LastDesktop != "d1" {
			t.Errorf("entry of the window moved by hand = %+v; want it homed on d1", moved)
		}
	})

	run("AMinimizedWindowKeepsItsKey", func(t *testing.T, h *flowHarness) {
		minimized := planWindow(2, "d2")
		minimized.Minimized = true
		h.windows = []Window{planWindow(1, "d3"), minimized, planWindow(3, "d2")}
		h.seedSitting(1, 1)
		h.seedSitting(2, 0)
		h.seedSitting(3, 2)
		h.report = underAPanel
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantKeys(map[string]int{"t2": 0, "t1": 1, "t3": 2})
	})

	run("AWorktreeAddedDuringTheRunKeepsItsEntry", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		h.onQuery = func(n int) {
			if n == 2 {
				h.worktrees = append(h.worktrees, "t5")
				h.reopen("t5", 5)
			}
		}
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		if added := ReadPlaces(h.path)["t5"]; added.Title != "T!" {
			t.Errorf("entry of the added worktree = %+v; want it kept with its title", added)
		}
	})

	run("AParentWithNoDesktopAnchorsAsAFallback", func(t *testing.T, h *flowHarness) {
		h.current = "d2"
		h.seed(Entry{Worktree: parentName, PID: 9, Title: "T9!", SettledBy: 9})
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d2")
		if home := ReadPlaces(h.path)["t1"].Home; home != "" {
			t.Errorf("home = %q; want none", home)
		}
	})

	run("ACancelDuringAWaitSettlesAndSkipsTheCommand", func(t *testing.T, windowWait *flowHarness) {
		windowWait.windows = nil
		parentWait := newFlowHarness(t)
		parentWait.reopen(parentName, 9)
		for wait, h := range map[string]*flowHarness{"window": windowWait, "parent": parentWait} {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h.onSleep = func(int) { cancel() }
			if err := h.flow.Place(ctx, h.req); !errors.Is(err, context.Canceled) {
				t.Errorf("Place() cancelled in the %s wait = %v; want context.Canceled", wait, err)
			}
			entry := ReadPlaces(h.path)["t1"]
			if !entry.Settled() || len(h.sleeps) != 1 || len(h.applies) != 0 || len(h.afters) != 0 {
				t.Errorf("cancelled in the %s wait: entry = %+v, sleeps %v, applies %d, after-place calls %d; want settled, one sleep, none and none",
					wait, entry, h.sleeps, len(h.applies), len(h.afters))
			}
		}
	})

	run("TheCommandsErrorJoinsTheWarning", func(t *testing.T, h *flowHarness) {
		h.windows = nil
		h.afterErr = errors.New("command broke")
		err := h.place()
		wantWarning(t, err, "did not appear")
		if !errors.Is(err, h.afterErr) {
			t.Errorf("Place() error = %v; want it to wrap the command's error", err)
		}
	})

	run("OffRecordsTheLockedSnapshot", func(t *testing.T, h *flowHarness) {
		h.req.Mode = "off"
		h.onQuery = func(n int) {
			if n == 2 {
				h.windows = []Window{planWindow(1, "d2")}
			}
		}
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantEnded(0)
		entry := ReadPlaces(h.path)["t1"]
		if entry.LastDesktop != "d2" || entry.Home != "d2" || entry.Output != "DP" {
			t.Errorf("entry = %+v; want last desktop and home d2 on DP", entry)
		}
		if want := []string{"query", "query", "after"}; !slices.Equal(h.events, want) {
			t.Errorf("events = %q; want %q", h.events, want)
		}
	})

	run("OffKeepsTheHomeAnotherPlacementSet", func(t *testing.T, h *flowHarness) {
		h.req.Mode = "off"
		h.onQuery = func(n int) {
			if n != 1 {
				return
			}
			other := newFlowHarness(t)
			other.path, other.req.BoltDir = h.path, h.req.BoltDir
			other.req.Worktree, other.req.PID, other.req.Title = "t2", 2, "T2!"
			other.flow.lockPath = h.flow.lockPath
			other.windows = []Window{planWindow(2, "d1")}
			other.parentOn("d2")
			if err := other.place(); err != nil {
				t.Errorf("other Place() error = %v", err)
			}
			h.windows = []Window{planWindow(1, "d3"), planWindow(2, "d2")}
		}
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		if entry := ReadPlaces(h.path)["t2"]; entry.Home != "d2" || entry.LastDesktop != "d2" {
			t.Errorf("other entry = %+v; want home d2 as its apply set it", entry)
		}
	})

	run("AFailedProbeLeavesTheWindow", func(t *testing.T, h *flowHarness) {
		h.tiled = false
		wantWarning(t, h.place(), "unsupported")
		h.wantEnded(0)
	})

	run("NoTilingOutputLeavesTheWindow", func(t *testing.T, h *flowHarness) {
		h.flow.Query = func(context.Context, string, time.Time, time.Duration) (Snapshot, time.Time, error) {
			return Snapshot{}, time.Time{}, fmt.Errorf("%w: %q", ErrNoTilingOutput, "DP")
		}
		wantWarning(t, h.place(), `"DP"`)
		h.wantEnded(0)
	})

	run("AHomedWindowDoesNotWaitForItsParent", func(t *testing.T, h *flowHarness) {
		h.seed(homed(1, "d2", 0))
		h.reopen(parentName, 9)
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d2")
		if len(h.sleeps) != 0 {
			t.Errorf("sleeps = %v; want none", h.sleeps)
		}
	})

	run("AHomeGoneUnderTheLockWaitsForTheParent", func(t *testing.T, h *flowHarness) {
		h.seed(homed(1, "d3", 0))
		h.reopen(parentName, 9)
		h.onQuery = func(n int) {
			if n == 2 {
				h.desktops = h.desktops[:2]
				h.windows = []Window{planWindow(1, "d1")}
			}
		}
		h.onSleep = func(int) { h.parentOn("d2") }
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d2")
		if len(h.sleeps) != 1 {
			t.Errorf("sleeps = %v; want one wait for the parent", h.sleeps)
		}
		if home := ReadPlaces(h.path)["t1"].Home; home != "d2" {
			t.Errorf("home = %q; want d2 in place of the gone desktop", home)
		}
	})

	run("ThePrimeStartsAtTheCurrentDesktopWithoutWaiting", func(t *testing.T, h *flowHarness) {
		h.req.Prime, h.req.Parent, h.req.Mode, h.current = true, "", "next", "d2"
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d2")
		if len(h.sleeps) != 0 {
			t.Errorf("sleeps = %v; want none", h.sleeps)
		}
	})

	run("AParentSettlingDuringTheWaitAnchorsTheWindow", func(t *testing.T, h *flowHarness) {
		h.reopen(parentName, 9)
		h.onSleep = func(n int) {
			if n == 2 {
				h.parentOn("d3")
			}
		}
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d3")
	})

	run("AParentUnsettledUnderTheLockIsWaitedForAgain", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		h.onQuery = func(n int) {
			if n == 2 {
				h.reopen(parentName, 19)
			}
		}
		h.onSleep = func(int) { h.parentOn("d3") }
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d3")
	})

	run("EveryReturnSharesOneParentBound", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		h.onQuery = func(n int) {
			if n > 1 {
				h.reopen(parentName, 19)
			}
		}
		h.onSleep = func(int) { h.parentOn("d2") }
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d1")
		if elapsed := h.now.Sub(h.start); elapsed < defaultParentWait || elapsed > defaultParentWait+time.Second {
			t.Errorf("waited %s; want the one bound of %s", elapsed, defaultParentWait)
		}
		if home := ReadPlaces(h.path)["t1"].Home; home != "" {
			t.Errorf("home = %q; want none for a window anchored past the bound", home)
		}
	})

	run("NoParentEntryFallsBackToTheCurrentDesktop", func(t *testing.T, h *flowHarness) {
		h.current = "d2"
		h.onSleep = func(n int) {
			if n == 5 {
				h.reopen("t4", 4)
			}
		}
		if err := h.place(); err != nil {
			t.Fatalf("Place() error = %v", err)
		}
		h.wantTarget("d2")
		if home := ReadPlaces(h.path)["t1"].Home; home != "" {
			t.Errorf("home = %q; want none", home)
		}
		want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, time.Second, 2 * time.Second}
		if len(h.sleeps) < len(want) || !slices.Equal(h.sleeps[:len(want)], want) {
			t.Errorf("sleeps = %v; want them to start %v", h.sleeps, want)
		}
	})

	run("ACancelBeforeTheCommandSkipsIt", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.report = func(ApplyOrder) (ApplyReport, error) {
			cancel()
			return ApplyReport{}, ctx.Err()
		}
		if err := h.flow.Place(ctx, h.req); !errors.Is(err, context.Canceled) {
			t.Errorf("Place() error = %v; want context.Canceled", err)
		}
		if entry := ReadPlaces(h.path)["t1"]; !entry.Settled() || len(h.afters) != 0 {
			t.Errorf("entry, after-place calls = %+v, %d; want settled and none", entry, len(h.afters))
		}
	})

	run("ACancelDuringTheCommandReachesIt", func(t *testing.T, h *flowHarness) {
		h.parentOn("d2")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var inCommand error
		h.onAfter = func(commandCtx context.Context) {
			cancel()
			inCommand = commandCtx.Err()
		}
		if err := h.flow.Place(ctx, h.req); err != nil {
			t.Errorf("Place() error = %v", err)
		}
		if !errors.Is(inCommand, context.Canceled) {
			t.Errorf("command context error = %v; want context.Canceled", inCommand)
		}
	})

	t.Run("TwoFlowsApplyInTurn", func(t *testing.T) {
		t.Parallel()
		first, second := newFlowHarness(t), newFlowHarness(t)
		second.flow.lockPath = first.flow.lockPath
		for _, h := range []*flowHarness{first, second} {
			h.req.Prime, h.req.Parent = true, ""
		}
		var applying atomic.Int32
		inFirstApply := make(chan struct{})
		secondAtTheLock := make(chan struct{})
		apply := func(wait func()) func(ApplyOrder) (ApplyReport, error) {
			return func(ApplyOrder) (ApplyReport, error) {
				if applying.Add(1) != 1 {
					t.Error("two applies ran at once")
				}
				wait()
				applying.Add(-1)
				return ApplyReport{}, nil
			}
		}
		first.report = apply(func() {
			held, free, err := lock.TryAcquireWriteLock(first.flow.lockPath)
			if err != nil || free {
				t.Errorf("placement lock free during an apply = %v, %v; want it held", free, err)
			}
			if free {
				_ = held.Release()
			}
			close(inFirstApply)
			<-secondAtTheLock
		})
		second.report = apply(func() {})
		second.flow.acquireLock = func(path string) (*lock.FileLock, error) {
			close(secondAtTheLock)
			return lock.AcquireWriteLock(path)
		}

		firstDone := make(chan error)
		go func() { firstDone <- first.place() }()
		<-inFirstApply
		if err := second.place(); err != nil {
			t.Errorf("second Place() error = %v", err)
		}
		if err := <-firstDone; err != nil {
			t.Errorf("first Place() error = %v", err)
		}
		if len(first.applies) != 1 || len(second.applies) != 1 {
			t.Errorf("applies = %d, %d; want 1, 1", len(first.applies), len(second.applies))
		}
	})
}
