// failedstop_test.go covers WriteFailedStop: the failed write on a running file and every case that leaves the file alone.

package shedengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

func TestWriteFailedStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		seedState  State
		missing    bool
		holdLock   bool
		wantResult FailedStopResult
		wantErr    bool
	}{
		{name: "running file is written failed", seedState: StateRunning, wantResult: FailedStopResult{Written: true}},
		{name: "held run lock writes nothing", seedState: StateRunning, holdLock: true, wantResult: FailedStopResult{Busy: true}},
		{name: "paused file is left untouched", seedState: StatePaused, wantResult: FailedStopResult{NotRunning: true}},
		{name: "blocked file is left untouched", seedState: StateBlocked, wantResult: FailedStopResult{NotRunning: true}},
		{name: "missing file is an error", missing: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			shed, _, _ := gotoShed(t)
			seed := commonSeed("B")
			seed.State = tt.seedState
			seed.PauseRequested = true
			seed.PauseBefore = "A"
			seed.History = []HistoryEntry{{Producer: "A", Outcome: Done, At: "2026-08-15T09:00:00Z"}}
			seed.Activity = composeActivity("B", seed.History, tt.seedState, "", "")
			if !tt.missing {
				seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
			}
			if tt.holdLock {
				if err := os.MkdirAll(filepath.Dir(shed.LockPath), 0o755); err != nil {
					t.Fatal(err)
				}
				held, locked, err := lock.TryAcquireWriteLock(shed.LockPath)
				if err != nil || !locked {
					t.Fatalf("TryAcquireWriteLock = _, %v, %v; want acquired", locked, err)
				}
				defer held.Release()
			}

			got, err := WriteFailedStop(FailedStopRequest{
				StatusPath:     shed.StatusPath,
				LockPath:       shed.LockPath,
				StatusLockPath: shed.StatusLockPath,
				Error:          "step child exited 2",
				Transient:      "network",
				WayForward:     "re-run lyx shed step",
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("WriteFailedStop error = %v; want error %v", err, tt.wantErr)
			}
			if got != tt.wantResult {
				t.Errorf("result = %+v; want %+v", got, tt.wantResult)
			}
			if tt.wantErr {
				return
			}

			file := readStatus(t, shed.StatusPath, shed.StatusLockPath)
			if !tt.wantResult.Written {
				if !reflect.DeepEqual(file, seed) {
					t.Errorf("status file changed: %+v -> %+v", seed, file)
				}
				return
			}
			if file.State != StateFailed || file.Error != "step child exited 2; way forward: re-run lyx shed step" || file.Transient != "network" {
				t.Errorf("State, Error, Transient = %q, %q, %q; want failed with the error, its way forward and the class", file.State, file.Error, file.Transient)
			}
			if !reflect.DeepEqual(file.History, seed.History) || file.CurrentProducer != "B" || !file.PauseRequested || file.PauseBefore != "A" {
				t.Errorf("history, current_producer, pause_requested, pause_before = %+v, %q, %v, %q; want all untouched", file.History, file.CurrentProducer, file.PauseRequested, file.PauseBefore)
			}
		})
	}
}
