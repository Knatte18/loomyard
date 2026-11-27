// donerun_test.go pins dobby's done-run clear over a fake clear seam and a temp state record.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestClearDoneRuns_WaitsForTheLandingCountOfARunWhileATidyIsInFlight(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "beam-merge-in"
	h := newTidyHarness(t, 5)
	h.step()
	var calls []string
	h.d.deps.ClearDoneRun = func(slug string) (bool, error) {
		calls = append(calls, slug)
		return true, nil
	}
	h.d.flights = map[string]flight{}
	h.d.listedRuns = []RunView{{Slug: slug, State: runStateDone}}
	h.landed = []string{slug}
	if err := UpdateState(h.d.geom.StatePath, func(s *State) error {
		s.Tidy = &TidyState{}
		return nil
	}); err != nil {
		t.Fatalf("UpdateState() = %v; want nil", err)
	}

	if err := h.d.countLandings(h.now); err != nil {
		t.Fatalf("countLandings() = %v; want nil", err)
	}
	h.d.clearDoneRuns(h.now)
	if len(calls) != 0 {
		t.Fatalf("ClearDoneRun calls = %v; want none while the landing is uncounted", calls)
	}

	if err := UpdateState(h.d.geom.StatePath, func(s *State) error {
		s.Tidy = nil
		return nil
	}); err != nil {
		t.Fatalf("UpdateState() = %v; want nil", err)
	}
	if err := h.d.countLandings(h.now); err != nil {
		t.Fatalf("countLandings() = %v; want nil", err)
	}
	h.d.clearDoneRuns(h.now)
	if !slices.Equal(calls, []string{slug}) {
		t.Errorf("ClearDoneRun calls = %v; want the one call once the landing is counted", calls)
	}
}

func TestClearDoneRuns_LeavesARunWhosePairStandsForTheLeftoverTeardown(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "beam-merge-in"
	var calls []string
	d := &daemon{
		geom: Geometry{WorktreeName: "prime", StatePath: filepath.Join(t.TempDir(), StateFileName)},
		deps: Deps{ClearDoneRun: func(slug string) (bool, error) {
			calls = append(calls, slug)
			return true, nil
		}},
		flights:    map[string]flight{},
		listedRuns: []RunView{{Slug: slug, State: runStateDone, PairPresent: true}},
	}
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

	d.clearDoneRuns(now)
	if len(calls) != 0 {
		t.Fatalf("ClearDoneRun calls = %v; want none while the run's pair stands", calls)
	}

	d.listedRuns[0].PairPresent = false
	d.clearDoneRuns(now)
	if !slices.Equal(calls, []string{slug}) {
		t.Errorf("ClearDoneRun calls = %v; want the one call once the pair is gone", calls)
	}
}

func TestClearDoneRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "beam-merge-in"
	cleared := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		state string
		// driverLockHeld and inFlight mark a pass of the run as running.
		driverLockHeld bool
		inFlight       bool
		// remembered gives the run a landing-notice memory entry.
		remembered bool
		// landed lists the run in the tick's landing count, and counted marks that landing counted.
		landed  bool
		counted bool
		// clearResult and clearErr are what the clear seam returns.
		clearResult bool
		clearErr    error
		// watch seeds the run's run watch entry, none when nil.
		watch *RunWatchState
		// posts runs the run watch step, whose post succeeds, between a first clear and the second.
		posts       bool
		wantCalled  bool
		wantCleared bool
	}{
		{name: "DoneRunIsCleared", state: runStateDone, clearResult: true, watch: &RunWatchState{Level: RunWatchStops}, wantCalled: true, wantCleared: true},
		{name: "DoneRunWithACountedLandingIsClearedAndLeavesTheCount", state: runStateDone, landed: true, counted: true, clearResult: true, watch: &RunWatchState{Level: RunWatchStops}, wantCalled: true, wantCleared: true},
		{name: "DoneRunOwingAWatchLineIsNotCleared", state: runStateDone, clearResult: true, watch: &RunWatchState{Owed: []OwedWatchLine{{Line: "line", OwedAt: cleared}}}},
		{name: "DoneRunIsClearedOnceItsOwedWatchLinePosted", state: runStateDone, clearResult: true, watch: &RunWatchState{Level: RunWatchStops, Owed: []OwedWatchLine{{Line: "line", OwedAt: cleared}}}, posts: true, wantCalled: true, wantCleared: true},
		{name: "RunningRunIsNotCleared", state: runStateRunning},
		{name: "DoneRunWithItsDriverLockHeldIsNotCleared", state: runStateDone, driverLockHeld: true},
		{name: "DoneRunWithAPassInFlightIsNotCleared", state: runStateDone, inFlight: true},
		{name: "DoneRunWithALandingNoticeRememberedIsNotCleared", state: runStateDone, remembered: true},
		{name: "DoneRunWithAnUncountedLandingIsNotCleared", state: runStateDone, landed: true},
		{name: "DoneRunTheClearLeavesAloneKeepsItsCount", state: runStateDone, landed: true, counted: true, wantCalled: true},
		{name: "DoneRunWhoseClearFailsKeepsItsCount", state: runStateDone, landed: true, counted: true, clearErr: errors.New("records commit refused"), wantCalled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			var logged []string
			d := &daemon{
				geom: Geometry{WorktreeName: "prime", StatePath: filepath.Join(t.TempDir(), StateFileName)},
				deps: Deps{
					ClearDoneRun: func(slug string) (bool, error) {
						calls = append(calls, slug)
						return tt.clearResult, tt.clearErr
					},
					Log: func(_, line string, _ bool) {
						_, text, _ := strings.Cut(line, " ")
						logged = append(logged, text)
					},
				},
				flights:    map[string]flight{},
				listedRuns: []RunView{{Slug: slug, State: tt.state, DriverLockHeld: tt.driverLockHeld}, {Slug: "other", State: runStateRunning}},
			}
			if tt.inFlight {
				d.flights[slug] = flight{}
			}
			if tt.landed {
				d.landed = []string{slug}
			}
			err := UpdateState(d.geom.StatePath, func(s *State) error {
				s.LandNotices = map[string]LandNoticeState{}
				if tt.remembered {
					s.LandNotices[slug] = LandNoticeState{Requested: true}
				}
				s.CountedLandings = map[string]bool{"other": true, "gone": true}
				if tt.counted {
					s.CountedLandings[slug] = true
				}
				if tt.watch != nil {
					s.Watches = map[string]RunWatchState{slug: *tt.watch}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("UpdateState() = %v; want nil", err)
			}

			d.clearDoneRuns(cleared)
			if tt.posts {
				if len(calls) != 0 {
					t.Fatalf("ClearDoneRun calls = %v; want none while the watch line is owed", calls)
				}
				d.deps.Log = nil
				d.deps.RunWatch = RunWatchDeps{
					Level: func(string) (string, error) { return RunWatchStops, nil },
					Post:  func(context.Context, string, string) (bool, error) { return true, nil },
				}
				d.runWatch(context.Background(), cleared)
				d.deps.Log = func(_, line string, _ bool) {
					_, text, _ := strings.Cut(line, " ")
					logged = append(logged, text)
				}
				d.clearDoneRuns(cleared)
			}

			if called := slices.Equal(calls, []string{slug}); called != tt.wantCalled {
				t.Errorf("ClearDoneRun calls = %v; want called %t", calls, tt.wantCalled)
			}
			var wantLogged []string
			if tt.wantCleared {
				wantLogged = []string{"run " + slug + ": done run cleared"}
			}
			if !slices.Equal(logged, wantLogged) {
				t.Errorf("log lines = %q; want %q", logged, wantLogged)
			}
			after, _, err := ReadState(d.geom.StatePath)
			if err != nil {
				t.Fatalf("ReadState() = %v; want nil", err)
			}
			if counted := after.CountedLandings[slug]; counted != (tt.counted && !tt.wantCleared) {
				t.Errorf("CountedLandings[%s] = %t; want %t", slug, counted, tt.counted && !tt.wantCleared)
			}
			if !after.CountedLandings["other"] {
				t.Errorf("CountedLandings = %v; want the listed run's count kept", after.CountedLandings)
			}
			if after.CountedLandings["gone"] {
				t.Errorf("CountedLandings = %v; want the entry of a slug neither landed nor listed dropped", after.CountedLandings)
			}
			if _, kept := after.Watches[slug]; tt.wantCleared && kept {
				t.Errorf("Watches = %v; want the cleared run's entry gone", after.Watches)
			}
		})
	}

	t.Run("DoneRunIsNotClearedOnATickWhoseRunWatchCouldNotRecord", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("directory mode bits do not refuse the write here")
		}
		var calls []string
		recordDir := t.TempDir()
		d := &daemon{
			geom: Geometry{WorktreeName: "prime", StatePath: filepath.Join(recordDir, StateFileName)},
			deps: Deps{
				ClearDoneRun: func(slug string) (bool, error) {
					calls = append(calls, slug)
					return true, nil
				},
				RunWatch: RunWatchDeps{
					Level: func(string) (string, error) { return RunWatchStops, nil },
					Post:  func(context.Context, string, string) (bool, error) { return true, nil },
				},
			},
			flights:    map[string]flight{},
			listedRuns: []RunView{{Slug: slug, State: runStateDone}},
		}
		if err := UpdateState(d.geom.StatePath, func(s *State) error {
			s.Watches = map[string]RunWatchState{slug: {Level: RunWatchStops}}
			return nil
		}); err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}
		permkit.Chmod(t, recordDir, 0o555)

		d.runWatch(context.Background(), cleared)
		d.clearDoneRuns(cleared)

		// The clear reads the record itself, so an unreadable one would hold it for another reason.
		if _, _, err := ReadState(d.geom.StatePath); err != nil {
			t.Fatalf("ReadState() = %v; want the record readable", err)
		}
		if len(calls) != 0 {
			t.Errorf("ClearDoneRun calls = %v; want none while the watch cannot record", calls)
		}
	})

	t.Run("DoneRunWhoseRunWatchNeverReadsWaitsForItAndThenClears", func(t *testing.T) {
		t.Parallel()
		var calls, posted []string
		levelErr := errors.New("level unreadable")
		d := &daemon{
			geom: Geometry{WorktreeName: "prime", StatePath: filepath.Join(t.TempDir(), StateFileName)},
			deps: Deps{
				ClearDoneRun: func(slug string) (bool, error) {
					calls = append(calls, slug)
					return true, nil
				},
				RunWatch: RunWatchDeps{
					Level: func(string) (string, error) { return RunWatchStops, levelErr },
					Post: func(_ context.Context, _, line string) (bool, error) {
						posted = append(posted, line)
						return true, nil
					},
				},
			},
			flights:    map[string]flight{},
			listedRuns: []RunView{{Slug: slug, State: runStateDone, Run: runfsm.Landed, Producer: "Done", PastLand: true, Landed: true, StatusModified: cleared}},
		}
		if err := UpdateState(d.geom.StatePath, func(s *State) error {
			s.Watches = map[string]RunWatchState{slug: {}}
			return nil
		}); err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}

		for range 2 {
			d.runWatch(context.Background(), cleared)
			d.clearDoneRuns(cleared)
		}
		if len(calls) != 0 || len(posted) != 0 {
			t.Fatalf("ClearDoneRun calls = %v, lines = %q; want none while the level does not read", calls, posted)
		}

		levelErr = nil
		d.runWatch(context.Background(), cleared)
		d.clearDoneRuns(cleared)

		if want := []string{"[run watch] " + slug + ": landed; batten done at Done; child none; since 2026-11-15T12:00:00Z"}; !slices.Equal(posted, want) {
			t.Errorf("lines posted = %q; want %q", posted, want)
		}
		if !slices.Equal(calls, []string{slug}) {
			t.Errorf("ClearDoneRun calls = %v; want the one call once the landed line posted", calls)
		}
	})
}
