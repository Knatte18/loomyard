// retile_test.go pins the retile's grids, report, records and lock over hand-built snapshots.

package kwinplace

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/google/go-cmp/cmp"
)

// retile runs the harness's flow over its hub, with mode answering each worktree's mode; nil answers same for all.
func (h *flowHarness) retile(mode func(worktree string) (string, error)) (RetileReport, error) {
	h.t.Helper()
	return h.retileWithin(context.Background(), mode)
}

// retileWithin is retile under ctx.
func (h *flowHarness) retileWithin(ctx context.Context, mode func(worktree string) (string, error)) (RetileReport, error) {
	h.t.Helper()
	if mode == nil {
		mode = func(string) (string, error) { return "same", nil }
	}
	return h.flow.Retile(ctx, RetileRequest{
		BoltDir:       h.req.BoltDir,
		MinTileWidth:  h.req.MinTileWidth,
		MinTileHeight: h.req.MinTileHeight,
		Mode:          mode,
	})
}

// wantReport checks the retile's report and that it ran the applies want names, in order.
func (h *flowHarness) wantReport(got RetileReport, err error, want RetileReport, applies []ApplyOrder) {
	h.t.Helper()
	if err != nil {
		h.t.Fatalf("Retile() error = %v", err)
	}
	for _, list := range []*[]RetiledWindow{&want.Retiled, &want.InPlace} {
		if *list == nil {
			*list = []RetiledWindow{}
		}
	}
	if want.NotPlaced == nil {
		want.NotPlaced = []UnplacedWindow{}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		h.t.Errorf("Retile() report mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(applies, h.applies); diff != "" {
		h.t.Errorf("Retile() applies mismatch (-want +got):\n%s", diff)
	}
}

func TestFlow_Retile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	run := func(name string, body func(t *testing.T, h *flowHarness)) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body(t, newFlowHarness(t))
		})
	}

	run("ADriftedWindowGoesBackToItsZone", func(t *testing.T, h *flowHarness) {
		// Window 1 has left its home d2 for d1; window 100 is no window of the hub.
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d2"), planWindow(100, "d2")}
		h.seed(homed(1, "d2", 1))
		h.seed(homed(2, "d2", 0))
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B1"}, {"t1", "B1"}}},
			[]ApplyOrder{{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 3, Zones: []string{"w2", "w1", "w100"}, Moved: []string{"w2", "w1"}}})
		if entry := ReadPlaces(h.path)["t1"]; entry.LastDesktop != "d2" || entry.Output != "DP" || entry.Home != "d2" || entry.OrderKey != 1 {
			t.Errorf("entry = %+v; want last seen on d2 on DP, still homed on d2 under key 1", entry)
		}
	})

	run("AStandingGridIsLeftUntouched", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d2"), planWindow(2, "d2")}
		h.tiles = map[string][]Tile{"d2": {{Windows: []string{"w2"}}, {Windows: []string{"w1"}}}}
		h.seed(homed(1, "d2", 1))
		h.seed(homed(2, "d2", 0))
		got, err := h.retile(nil)
		h.wantReport(got, err, RetileReport{InPlace: []RetiledWindow{{"t2", "B1"}, {"t1", "B1"}}}, nil)
	})

	run("AGridOutOfOrderIsBuiltAgain", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d2"), planWindow(2, "d2")}
		h.tiles = map[string][]Tile{"d2": {{Windows: []string{"w1"}}, {Windows: []string{"w2"}}}}
		h.seed(homed(1, "d2", 1))
		h.seed(homed(2, "d2", 0))
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B1"}, {"t1", "B1"}}},
			[]ApplyOrder{{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 2, Zones: []string{"w2", "w1"}, Moved: []string{"w2", "w1"}}})
	})

	run("AWindowOnAnotherHomeLeavesThatDesktopsGrid", func(t *testing.T, h *flowHarness) {
		// Window 1 sits on d2 but is homed on d3, so d2's grid holds window 2 alone.
		h.windows = []Window{planWindow(1, "d2"), planWindow(2, "d2")}
		h.seed(homed(1, "d3", 0))
		h.seed(homed(2, "d2", 0))
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B1"}, {"t1", "B2"}}},
			[]ApplyOrder{
				{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 1, Zones: []string{"w2"}, Moved: []string{"w2"}},
				{TilingOutput: "DP", Desktop: "d3", Rows: 1, Cols: 1, Zones: []string{"w1"}, Moved: []string{"w1"}},
			})
	})

	run("WindowsLeftWhereTheyAreAreNamed", func(t *testing.T, h *flowHarness) {
		minimized := planWindow(3, "d1")
		minimized.Minimized = true
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d1"), minimized, planWindow(4, "d1"), planWindow(5, "d1"), planWindow(6, "d1")}
		h.seed(Entry{Worktree: "t1", PID: 1, Title: "T1!", Home: "d2"})
		h.seed(homed(2, "d2", 0))
		h.seed(homed(3, "d2", 1))
		h.seed(homed(4, "", 0))
		h.seed(homed(5, "d9", 0))
		h.seed(homed(6, "d2", 2))
		// Worktree t7's window is not open, so the report leaves it out.
		h.seed(homed(7, "d2", 3))
		got, err := h.retile(func(worktree string) (string, error) {
			switch worktree {
			case "t2":
				return "off", nil
			case "t6":
				return "", errors.New("mode unreadable")
			}
			return "same", nil
		})
		h.wantReport(got, err, RetileReport{NotPlaced: []UnplacedWindow{
			{"t1", reasonPlacing},
			{"t2", reasonOff},
			{"t3", reasonMinimized},
			{"t4", reasonNoHome},
			{"t5", reasonHomeGone},
			{"t6", "mode unreadable"},
		}}, nil)
	})

	run("AFullHomeTakesTheWindowsThatFit", func(t *testing.T, h *flowHarness) {
		// The screen takes three zones, and two of d2's hold windows of no hub.
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d1"), planWindow(100, "d2"), planWindow(101, "d2")}
		h.seed(homed(1, "d2", 0))
		h.seed(homed(2, "d2", 1))
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t1", "B1"}}, NotPlaced: []UnplacedWindow{{"t2", reasonFull}}},
			[]ApplyOrder{{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 3, Zones: []string{"w1", "w100", "w101"}, Moved: []string{"w1"}}})
	})

	run("AFailedApplyLeavesOnlyItsDesktopsWindows", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d1")}
		h.seed(homed(1, "d2", 0))
		h.seed(homed(2, "d3", 0))
		h.report = func(order ApplyOrder) (ApplyReport, error) {
			if order.Desktop == "d2" {
				return ApplyReport{}, errors.New("apply broke")
			}
			return movedReport(order)
		}
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B2"}}, NotPlaced: []UnplacedWindow{{"t1", "apply broke"}}},
			[]ApplyOrder{
				{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 1, Zones: []string{"w1"}, Moved: []string{"w1"}},
				{TilingOutput: "DP", Desktop: "d3", Rows: 1, Cols: 1, Zones: []string{"w2"}, Moved: []string{"w2"}},
			})
		places := ReadPlaces(h.path)
		if failed, moved := places["t1"].LastDesktop, places["t2"].LastDesktop; failed != "" || moved != "d3" {
			t.Errorf("last desktops of t1, t2 = %q, %q; want none for the failed apply and d3", failed, moved)
		}
	})

	run("AWindowTrimmedFromAFullHomeStaysInItsDesktopsGrid", func(t *testing.T, h *flowHarness) {
		// d2 is full of windows of no hub; window 1 sits on d1, which gets a grid for window 2.
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d1"), planWindow(100, "d2"), planWindow(101, "d2"), planWindow(102, "d2")}
		h.seed(homed(1, "d2", 0))
		h.seed(homed(2, "d1", 0))
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B0"}}, NotPlaced: []UnplacedWindow{{"t1", reasonFull}}},
			[]ApplyOrder{{TilingOutput: "DP", Desktop: "d1", Rows: 1, Cols: 2, Zones: []string{"w2", "w1"}, Moved: []string{"w2"}}})
	})

	run("ATrimmedWindowTakesRoomWhereItSits", func(t *testing.T, h *flowHarness) {
		// d1 has room for window 2 only until window 1, trimmed from the full d2, counts there.
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d3"), planWindow(100, "d1"), planWindow(101, "d1"),
			planWindow(102, "d2"), planWindow(103, "d2"), planWindow(104, "d2")}
		h.seed(homed(1, "d2", 0))
		h.seed(homed(2, "d1", 0))
		got, err := h.retile(nil)
		h.wantReport(got, err, RetileReport{NotPlaced: []UnplacedWindow{{"t1", reasonFull}, {"t2", reasonFull}}}, nil)
	})

	run("AFullHomeKeepsTheWindowAlreadyOnIt", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d2"), planWindow(100, "d2"), planWindow(101, "d2")}
		h.seed(homed(1, "d2", 0))
		h.seed(homed(2, "d2", 1))
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B1"}}, NotPlaced: []UnplacedWindow{{"t1", reasonFull}}},
			[]ApplyOrder{{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 3, Zones: []string{"w2", "w100", "w101"}, Moved: []string{"w2"}}})
	})

	// Window 1 sits on d2 and is homed on d3, where it does not get to; window 2 is at home on d2.
	strandings := []struct {
		name string
		// d3 answers the apply of window 1's home.
		d3 func(ApplyOrder) (ApplyReport, error)
		// rebuild answers the second apply on d2; nil tiles every window of its zones.
		rebuild func(ApplyOrder) (ApplyReport, error)
		// queryFails fails every query after the first.
		queryFails bool
		wantReason string
	}{
		{
			name:       "AWindowOfAFailedApplyIsPutBackWhereItSits",
			d3:         func(ApplyOrder) (ApplyReport, error) { return ApplyReport{}, errors.New("apply broke") },
			wantReason: "apply broke",
		},
		{
			name:       "AWindowKWinKeptOffItsHomeIsPutBackWhereItSits",
			d3:         func(ApplyOrder) (ApplyReport, error) { return ApplyReport{}, nil },
			wantReason: reasonNotLanded,
		},
		{
			name:       "AFailedPutBackIsNamed",
			d3:         func(ApplyOrder) (ApplyReport, error) { return ApplyReport{}, errors.New("apply broke") },
			rebuild:    func(ApplyOrder) (ApplyReport, error) { return ApplyReport{}, errors.New("rebuild broke") },
			wantReason: "apply broke; the grid of the desktop it sits on was not built again around it: rebuild broke",
		},
		{
			name:       "APutBackKWinDidNotTileIsNamed",
			d3:         func(ApplyOrder) (ApplyReport, error) { return ApplyReport{}, errors.New("apply broke") },
			rebuild:    movedReport,
			wantReason: "apply broke; KWin did not tile it again where it sits",
		},
		{
			name:       "AnUnreadSecondQueryPutsBackByTheFirst",
			d3:         func(ApplyOrder) (ApplyReport, error) { return ApplyReport{}, errors.New("apply broke") },
			queryFails: true,
			wantReason: "apply broke",
		},
	}
	for _, tt := range strandings {
		run(tt.name, func(t *testing.T, h *flowHarness) {
			h.windows = []Window{planWindow(1, "d2"), planWindow(2, "d2")}
			h.seed(homed(1, "d3", 0))
			h.seed(homed(2, "d2", 0))
			h.report = func(order ApplyOrder) (ApplyReport, error) {
				if order.Desktop == "d3" {
					return tt.d3(order)
				}
				if len(h.applies) > 1 && tt.rebuild != nil {
					return tt.rebuild(order)
				}
				return underAPanel(order)
			}
			if tt.queryFails {
				query := h.flow.Query
				h.flow.Query = func(ctx context.Context, tilingOutput string, after time.Time, within time.Duration) (Snapshot, time.Time, error) {
					if len(h.queries) > 0 {
						return Snapshot{}, time.Time{}, errors.New("query broke")
					}
					return query(ctx, tilingOutput, after, within)
				}
			}
			got, err := h.retile(nil)
			h.wantReport(got, err,
				RetileReport{Retiled: []RetiledWindow{{"t2", "B1"}}, NotPlaced: []UnplacedWindow{{"t1", tt.wantReason}}},
				[]ApplyOrder{
					{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 1, Zones: []string{"w2"}, Moved: []string{"w2"}},
					{TilingOutput: "DP", Desktop: "d3", Rows: 1, Cols: 1, Zones: []string{"w1"}, Moved: []string{"w1"}},
					{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 2, Zones: []string{"w2", "w1"}, Moved: []string{"w2"}},
				})
			if last := ReadPlaces(h.path)["t1"].LastDesktop; last != "" {
				t.Errorf("last desktop of t1 = %q; want none", last)
			}
		})
	}

	run("APutBackBuildsNoMoreZonesThanTheScreenTakes", func(t *testing.T, h *flowHarness) {
		// Window 2 takes the zone window 1 was to leave on d2.
		h.windows = []Window{planWindow(1, "d2"), planWindow(2, "d1"), planWindow(100, "d2"), planWindow(101, "d2")}
		h.seed(homed(1, "d3", 0))
		h.seed(homed(2, "d2", 0))
		h.report = func(order ApplyOrder) (ApplyReport, error) {
			if order.Desktop == "d3" {
				return ApplyReport{}, errors.New("apply broke")
			}
			return movedReport(order)
		}
		got, err := h.retile(nil)
		// Window 1 stacks below the others, and still it is the one left without a zone.
		full := ApplyOrder{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 3, Zones: []string{"w2", "w100", "w101"}, Moved: []string{"w2"}}
		h.wantReport(got, err,
			RetileReport{
				Retiled:   []RetiledWindow{{"t2", "B1"}},
				NotPlaced: []UnplacedWindow{{"t1", "apply broke; the desktop it sits on has no zone left for it"}},
			},
			[]ApplyOrder{
				full,
				{TilingOutput: "DP", Desktop: "d3", Rows: 1, Cols: 1, Zones: []string{"w1"}, Moved: []string{"w1"}},
				full,
			})
	})

	run("AWindowAFailedApplyMovedAllTheSameIsNotPutBack", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d2"), planWindow(2, "d2")}
		h.seed(homed(1, "d3", 0))
		h.seed(homed(2, "d2", 0))
		h.report = func(order ApplyOrder) (ApplyReport, error) {
			if order.Desktop == "d3" {
				h.windows[0] = planWindow(1, "d3")
				return ApplyReport{}, errors.New("apply broke")
			}
			return movedReport(order)
		}
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B1"}}, NotPlaced: []UnplacedWindow{{"t1", "apply broke"}}},
			[]ApplyOrder{
				{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 1, Zones: []string{"w2"}, Moved: []string{"w2"}},
				{TilingOutput: "DP", Desktop: "d3", Rows: 1, Cols: 1, Zones: []string{"w1"}, Moved: []string{"w1"}},
			})
	})

	run("AnEndedContextKeepsWhatWasDone", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d1")}
		h.seed(homed(1, "d2", 0))
		h.seed(homed(2, "d3", 0))
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		h.report = func(order ApplyOrder) (ApplyReport, error) {
			if order.Desktop == "d3" {
				cancel()
				return ApplyReport{}, ctx.Err()
			}
			return movedReport(order)
		}
		got, err := h.retileWithin(ctx, nil)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Retile() error = %v; want %v", err, context.Canceled)
		}
		want := RetileReport{Retiled: []RetiledWindow{{"t1", "B1"}}, InPlace: []RetiledWindow{}, NotPlaced: []UnplacedWindow{}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("Retile() report mismatch (-want +got):\n%s", diff)
		}
		if last := ReadPlaces(h.path)["t1"].LastDesktop; last != "d2" {
			t.Errorf("last desktop of t1 = %q; want d2", last)
		}
	})

	run("AMemberOnAnotherScreenBuildsTheGridAgain", func(t *testing.T, h *flowHarness) {
		elsewhere := planWindow(1, "d2")
		elsewhere.Output = "HDMI"
		h.windows = []Window{elsewhere, planWindow(2, "d2")}
		h.tiles = map[string][]Tile{"d2": {{Windows: []string{"w2"}}, {Windows: []string{"w1"}}}}
		h.seed(homed(1, "d2", 1))
		h.seed(homed(2, "d2", 0))
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t2", "B1"}, {"t1", "B1"}}},
			[]ApplyOrder{{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 2, Zones: []string{"w2", "w1"}, Moved: []string{"w2", "w1"}}})
	})

	run("AnEntryThatTookANewerWindowIsNotRecorded", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d1")}
		h.seed(homed(1, "d2", 0))
		h.report = func(order ApplyOrder) (ApplyReport, error) {
			h.reopen("t1", 50)
			return movedReport(order)
		}
		if _, err := h.retile(nil); err != nil {
			t.Fatalf("Retile() error = %v", err)
		}
		if last := ReadPlaces(h.path)["t1"].LastDesktop; last != "" {
			t.Errorf("last desktop of t1 = %q; want it unchanged", last)
		}
	})

	run("AnApplyThatOmitsAWindowLeavesItUnplaced", func(t *testing.T, h *flowHarness) {
		h.windows = []Window{planWindow(1, "d1"), planWindow(2, "d1")}
		h.seed(homed(1, "d2", 0))
		h.seed(homed(2, "d2", 1))
		h.report = func(order ApplyOrder) (ApplyReport, error) {
			return ApplyReport{Windows: []AppliedWindow{{InternalID: "w1", Desktops: []string{order.Desktop}}}}, nil
		}
		got, err := h.retile(nil)
		h.wantReport(got, err,
			RetileReport{Retiled: []RetiledWindow{{"t1", "B1"}}, NotPlaced: []UnplacedWindow{{"t2", reasonNotLanded}}},
			[]ApplyOrder{{TilingOutput: "DP", Desktop: "d2", Rows: 1, Cols: 2, Zones: []string{"w1", "w2"}, Moved: []string{"w1", "w2"}}})
		if last := ReadPlaces(h.path)["t2"].LastDesktop; last != "" {
			t.Errorf("last desktop of t2 = %q; want none", last)
		}
	})

	run("AHeldLockMovesNothing", func(t *testing.T, h *flowHarness) {
		h.seed(homed(1, "d2", 0))
		holder, err := lock.AcquireWriteLock(h.flow.lockPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = holder.Release() })
		h.flow.lockWait = 50 * time.Millisecond
		want := fmt.Sprintf("kwinplace: placement lock %s not free within 50ms of waiting", h.flow.lockPath)
		if _, err := h.retile(nil); err == nil || err.Error() != want {
			t.Errorf("Retile() error = %v; want %q", err, want)
		}
		if len(h.queries) != 0 || len(h.applies) != 0 {
			t.Errorf("queries, applies = %d, %d; want none", len(h.queries), len(h.applies))
		}
	})

	run("NoPerDesktopTilingMovesNothing", func(t *testing.T, h *flowHarness) {
		h.seed(homed(1, "d2", 0))
		h.tiled = false
		want := "kwinplace: retiling is unsupported: KWin does not tile each desktop on its own"
		if _, err := h.retile(nil); err == nil || err.Error() != want {
			t.Errorf("Retile() error = %v; want %q", err, want)
		}
		if len(h.applies) != 0 {
			t.Errorf("applies = %d; want none", len(h.applies))
		}
	})
}
