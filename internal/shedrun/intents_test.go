// intents_test.go pins the writers of the batten record's cancel and pause intents.

package shedrun

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedIntentStatus writes a running status file carrying an unknown top-level key for run runID.
func seedIntentStatus(t *testing.T, l *lymcwd.Location, runID string) {
	t.Helper()
	seed := map[string]any{"state": "running", "unknown_key": "kept"}
	if err := state.WriteJSON(StatusFile(l, runID), StatusLock(l, runID), seed); err != nil {
		t.Fatalf("seed status file: %v", err)
	}
}

// readIntentStatus returns the run's status file decoded, and its raw top-level members.
func readIntentStatus(t *testing.T, l *lymcwd.Location, runID string) (shedengine.Status, map[string]json.RawMessage) {
	t.Helper()
	data, err := os.ReadFile(StatusFile(l, runID))
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	st, err := shedengine.ParseStatusTolerant(data)
	if err != nil {
		t.Fatalf("parse status file: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal status file: %v", err)
	}
	return st, raw
}

func TestRunIntents(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const runID = "slug"
	cancel := shedengine.CancelIntent{StartedAt: "2026-11-15T10:00:00Z", Machine: "neve", Remove: true}
	finish := shedengine.CancelFinish{EndedAt: "2026-11-15T10:01:00Z", Remove: true}
	keepCancel := shedengine.CancelIntent{StartedAt: "2026-11-15T11:00:00Z", Machine: "other"}
	pause := shedengine.PauseIntent{By: "orch", At: "2026-11-15T09:00:00Z"}

	t.Run("AbsentFileWritesNothingAndReportsFalse", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		writers := []struct {
			name  string
			write func() (bool, error)
		}{
			{"WriteCancelIntent", func() (bool, error) { return WriteCancelIntent(l, runID, cancel) }},
			{"FinishCancel", func() (bool, error) { return FinishCancel(l, runID, finish) }},
			{"WritePauseIntent", func() (bool, error) { return WritePauseIntent(l, runID, pause) }},
			{"ClearPauseIntent", func() (bool, error) { return ClearPauseIntent(l, runID) }},
		}
		for _, tt := range writers {
			if found, err := tt.write(); err != nil || found {
				t.Errorf("%s(absent) = %v, %v; want false, nil", tt.name, found, err)
			}
			if _, err := os.Stat(StatusFile(l, runID)); !os.IsNotExist(err) {
				t.Errorf("%s(absent) left a status file, stat error %v", tt.name, err)
			}
		}
	})

	t.Run("CancelThenFinishLeavesCancelledWithBothRecorded", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		seedIntentStatus(t, l, runID)
		if found, err := WriteCancelIntent(l, runID, cancel); err != nil || !found {
			t.Fatalf("WriteCancelIntent() = %v, %v; want true, nil", found, err)
		}
		if found, err := FinishCancel(l, runID, finish); err != nil || !found {
			t.Fatalf("FinishCancel() = %v, %v; want true, nil", found, err)
		}
		got, _ := readIntentStatus(t, l, runID)
		wantCancel := cancel
		wantCancel.Finish = &finish
		if got.State != shedengine.StateCancelled || !reflect.DeepEqual(got.Cancel, &wantCancel) {
			t.Errorf("status = state %q, cancel %+v; want state %q, cancel %+v", got.State, got.Cancel, shedengine.StateCancelled, &wantCancel)
		}
	})

	t.Run("FinishWithoutIntentErrorsAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		seedIntentStatus(t, l, runID)
		if _, err := FinishCancel(l, runID, finish); err == nil {
			t.Fatal("FinishCancel() with no cancel intent = nil; want an error")
		}
		if got, _ := readIntentStatus(t, l, runID); got.State != shedengine.StateRunning || got.Cancel != nil {
			t.Errorf("status after refused finish = state %q, cancel %+v; want running and no cancel", got.State, got.Cancel)
		}
	})

	t.Run("IntentOverFinishedOneReplacesItWhole", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		seedIntentStatus(t, l, runID)
		for _, step := range []func() (bool, error){
			func() (bool, error) { return WriteCancelIntent(l, runID, cancel) },
			func() (bool, error) { return FinishCancel(l, runID, finish) },
			func() (bool, error) { return WriteCancelIntent(l, runID, keepCancel) },
		} {
			if found, err := step(); err != nil || !found {
				t.Fatalf("step = %v, %v; want true, nil", found, err)
			}
		}
		if got, _ := readIntentStatus(t, l, runID); !reflect.DeepEqual(got.Cancel, &keepCancel) {
			t.Errorf("Cancel = %+v; want %+v with no finish", got.Cancel, &keepCancel)
		}
	})

	t.Run("PauseIntentWrittenThenCleared", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		seedIntentStatus(t, l, runID)
		if found, err := WritePauseIntent(l, runID, pause); err != nil || !found {
			t.Fatalf("WritePauseIntent() = %v, %v; want true, nil", found, err)
		}
		if got, _ := readIntentStatus(t, l, runID); !reflect.DeepEqual(got.PauseIntent, &pause) {
			t.Errorf("PauseIntent = %+v; want %+v", got.PauseIntent, &pause)
		}
		for range 2 {
			if found, err := ClearPauseIntent(l, runID); err != nil || !found {
				t.Fatalf("ClearPauseIntent() = %v, %v; want true, nil, even when no intent stands", found, err)
			}
			if got, _ := readIntentStatus(t, l, runID); got.PauseIntent != nil {
				t.Errorf("PauseIntent after clear = %+v; want nil", got.PauseIntent)
			}
		}
	})

	t.Run("UnknownTopLevelKeySurvivesEveryWriter", func(t *testing.T) {
		t.Parallel()
		l := syntheticLocation(t)
		seedIntentStatus(t, l, runID)
		for _, step := range []func() (bool, error){
			func() (bool, error) { return WriteCancelIntent(l, runID, cancel) },
			func() (bool, error) { return FinishCancel(l, runID, finish) },
			func() (bool, error) { return WritePauseIntent(l, runID, pause) },
			func() (bool, error) { return ClearPauseIntent(l, runID) },
		} {
			if found, err := step(); err != nil || !found {
				t.Fatalf("step = %v, %v; want true, nil", found, err)
			}
			if _, raw := readIntentStatus(t, l, runID); string(raw["unknown_key"]) != `"kept"` {
				t.Errorf("unknown_key = %s after a writer; want %q", raw["unknown_key"], `"kept"`)
			}
		}
	})
}
