// donerun_test.go pins dobby's done-run clear over a fake clear seam and a temp state record.

package dobbyengine

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestClearDoneRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "beam-merge-in"
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
		wantCalled  bool
		wantCleared bool
	}{
		{name: "DoneRunIsCleared", state: runStateDone, clearResult: true, wantCalled: true, wantCleared: true},
		{name: "DoneRunWithACountedLandingIsClearedAndLeavesTheCount", state: runStateDone, landed: true, counted: true, clearResult: true, wantCalled: true, wantCleared: true},
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
				return nil
			})
			if err != nil {
				t.Fatalf("UpdateState() = %v; want nil", err)
			}

			d.clearDoneRuns(time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))

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
		})
	}
}
