// watcher_owner_test.go drives the watcher's owner check.
// It scripts the owner read and records the end and the strand stop.

package orchengine

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWatcher_AnAdoptThatLiftsLetsTheNextTickStopTheStrand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	e := newWatchEnv(t)
	held := true
	var stops []string
	e.w.WatchOwner(OwnerWatch{
		Read: func() (string, bool, error) { return "there", true, nil },
		Stop: func(guid string) error {
			stops = append(stops, guid)
			return nil
		},
		UnderAdopt: func() bool { return held },
	})

	done, err := e.w.Tick()
	if err != nil || done || len(stops) != 0 {
		t.Fatalf("Tick() under the adopt = %v, %v with stops %v; want not done, no error, no stop", done, err, stops)
	}
	held = false
	done, err = e.w.Tick()

	if err != nil || !done || !slices.Equal(stops, []string{"s1"}) {
		t.Errorf("Tick() after the adopt lifts = %v, %v with stops %v; want done, no error, the strand stopped once", done, err, stops)
	}
}

func TestWatcher_EndsTheProcessesAndStopsTheOrchOfABeamOwnedElsewhere(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		owner      string
		elsewhere  bool
		underAdopt bool
		readErr    error
		endErr     error
		stopErr    error
		wantDone   bool
		wantErr    bool
		wantCalls  []string
		wantExit   string
		wantNoExit bool
	}{
		{name: "OwnerNamingAnotherMachineEndsTheProcessesThenStopsTheStrandAndEndsTheWatcher", owner: "there", elsewhere: true, wantDone: true, wantCalls: []string{"end", "stop s1"}, wantExit: "owned by machine there"},
		{name: "OwnerNamingAnotherMachineUnderAnAdoptInFlightKeepsEverything", owner: "there", elsewhere: true, underAdopt: true, wantNoExit: true},
		{name: "OwnerNamingThisMachineGoesOn", owner: "here", wantNoExit: true},
		{name: "NoOwnerRecordedGoesOn", wantNoExit: true},
		{name: "FailingReadGoesOnWithNoEndOrStop", readErr: errors.New("board unreadable"), wantNoExit: true},
		{name: "FailingEndStillStopsTheStrandAndEndsTheWatcherWithTheOwnerExit", owner: "there", elsewhere: true, endErr: errors.New("survivors"), wantDone: true, wantCalls: []string{"end", "stop s1"}, wantExit: "owned by machine there"},
		{name: "FailingEndAndStopReturnTheErrorAndKeepTheWatcherRunning", owner: "there", elsewhere: true, endErr: errors.New("survivors"), stopErr: errors.New("reed busy"), wantErr: true, wantCalls: []string{"end", "stop s1"}, wantNoExit: true},
		{name: "FailingStopReturnsTheErrorAndKeepsTheWatcherRunning", owner: "there", elsewhere: true, stopErr: errors.New("reed busy"), wantErr: true, wantCalls: []string{"end", "stop s1"}, wantNoExit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newWatchEnv(t)
			var calls []string
			e.w.WatchOwner(OwnerWatch{
				Read: func() (string, bool, error) { return tt.owner, tt.elsewhere, tt.readErr },
				End: func() error {
					calls = append(calls, "end")
					return tt.endErr
				},
				Stop: func(guid string) error {
					calls = append(calls, "stop "+guid)
					return tt.stopErr
				},
				UnderAdopt: func() bool { return tt.underAdopt },
			})

			done, err := e.w.Tick()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Tick() error = %v; want error %v", err, tt.wantErr)
			}
			if done != tt.wantDone {
				t.Errorf("Tick() done = %v; want %v", done, tt.wantDone)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("End and Stop calls = %v; want %v", calls, tt.wantCalls)
			}
			st, loadErr := LoadState(e.paths)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if tt.wantNoExit && st.WatcherExit != "" {
				t.Errorf("WatcherExit = %q; want none recorded", st.WatcherExit)
			}
			if !tt.wantNoExit && st.WatcherExit != tt.wantExit {
				t.Errorf("WatcherExit = %q; want %q", st.WatcherExit, tt.wantExit)
			}
		})
	}
}
