//go:build tier0

// watcher_owner_test.go drives the watcher's owner check over a scripted owner read and a recording strand stop.

package orchengine

import (
	"errors"
	"slices"
	"testing"
)

func TestWatcher_StopsTheOrchOfABeamOwnedElsewhere(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		owner      string
		elsewhere  bool
		readErr    error
		stopErr    error
		wantDone   bool
		wantErr    bool
		wantStops  []string
		wantExit   string
		wantNoExit bool
	}{
		{name: "OwnerNamingAnotherMachineStopsTheStrandOnceAndEndsTheWatcher", owner: "there", elsewhere: true, wantDone: true, wantStops: []string{"s1"}, wantExit: "owned by machine there"},
		{name: "OwnerNamingThisMachineGoesOn", owner: "here", wantNoExit: true},
		{name: "NoOwnerRecordedGoesOn", wantNoExit: true},
		{name: "FailingReadGoesOnWithNoStop", readErr: errors.New("board unreadable"), wantNoExit: true},
		{name: "FailingStopReturnsTheErrorAndKeepsTheWatcherRunning", owner: "there", elsewhere: true, stopErr: errors.New("reed busy"), wantErr: true, wantStops: []string{"s1"}, wantNoExit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newWatchEnv(t)
			var stops []string
			e.w.WatchOwner(OwnerWatch{
				Read: func() (string, bool, error) { return tt.owner, tt.elsewhere, tt.readErr },
				Stop: func(guid string) error {
					stops = append(stops, guid)
					return tt.stopErr
				},
			})

			done, err := e.w.Tick()

			if (err != nil) != tt.wantErr {
				t.Fatalf("Tick() error = %v; want error %v", err, tt.wantErr)
			}
			if done != tt.wantDone {
				t.Errorf("Tick() done = %v; want %v", done, tt.wantDone)
			}
			if !slices.Equal(stops, tt.wantStops) {
				t.Errorf("Stop calls = %v; want %v", stops, tt.wantStops)
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
