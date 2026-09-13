// stephandoff_test.go is the untagged suite for the step clean-handoff marker (crucible round 2,
// R2-F1): recordStepHandoff and consumeStepHandoffMatch directly, and observeEntry's consumption of
// the marker into EntryObservation.CleanStepHandoff. Everything runs against t.TempDir() paths via
// the production state primitives -- no tmux, no git, no real run.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// stepHandoffPaths returns a marker path and lock path rooted in a fresh temp dir.
func stepHandoffPaths(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "step-handoff.json"), filepath.Join(dir, "step-handoff.json.lock")
}

func TestConsumeStepHandoffMatch(t *testing.T) {
	tests := []struct {
		name            string
		record          bool
		recordedHistory int
		recordedState   shedengine.State
		observedHistory int
		observedState   shedengine.State
		want            bool
	}{
		{
			name:   "Match_Suppresses",
			record: true, recordedHistory: 3, recordedState: shedengine.StateRunning,
			observedHistory: 3, observedState: shedengine.StateRunning,
			want: true,
		},
		{
			// A driver that resumed from the handoff and appended history before dying must be
			// reported as a crash, so a grown history mismatches.
			name:   "HistoryGrew_NoMatch",
			record: true, recordedHistory: 3, recordedState: shedengine.StateRunning,
			observedHistory: 4, observedState: shedengine.StateRunning,
			want: false,
		},
		{
			name:   "StateDiffers_NoMatch",
			record: true, recordedHistory: 3, recordedState: shedengine.StateRunning,
			observedHistory: 3, observedState: shedengine.StateBlocked,
			want: false,
		},
		{
			name:            "AbsentMarker_NoMatch",
			record:          false,
			observedHistory: 3, observedState: shedengine.StateRunning,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markerPath, lockPath := stepHandoffPaths(t)
			if tt.record {
				recordStepHandoff(markerPath, lockPath, tt.recordedHistory, tt.recordedState)
				if _, err := os.Stat(markerPath); err != nil {
					t.Fatalf("recordStepHandoff left no marker at %s: %v", markerPath, err)
				}
			}

			got := consumeStepHandoffMatch(markerPath, lockPath, tt.observedHistory, tt.observedState)
			if got != tt.want {
				t.Errorf("consumeStepHandoffMatch(recorded %v/%d/%s, observed %d/%s) = %v; want %v",
					tt.record, tt.recordedHistory, tt.recordedState, tt.observedHistory, tt.observedState, got, tt.want)
			}

			// The marker is a one-shot voucher: consumed (deleted) on every read, match or not,
			// so a driver crash after a consumed handoff is never suppressed by a stale marker.
			if tt.record {
				if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
					t.Errorf("marker still present after consume (stat err=%v); want it deleted", err)
				}
			}
		})
	}
}

// TestObserveEntry_ConsumesStepHandoffIntoObservation asserts the wiring: a marker matching the
// persisted status yields CleanStepHandoff true, and -- the one-shot property -- a second identical
// observation yields false, because the first consumed the marker.
func TestObserveEntry_ConsumesStepHandoffIntoObservation(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	runLockPath := filepath.Join(dir, "run.lock")
	markerPath := filepath.Join(dir, "step-handoff.json")
	markerLockPath := filepath.Join(dir, "step-handoff.json.lock")

	writeSelfreportStatus(t, statusPath, statusLockPath, shedengine.Status{
		CurrentProducer: "Plan-Write",
		State:           shedengine.StateRunning,
		History: []shedengine.HistoryEntry{
			{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-01-01T00:00:00Z"},
			{Producer: "Loom-Preflight", Outcome: shedengine.Done, At: "2026-01-01T00:00:01Z"},
		},
		Product: productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"}),
	})
	recordStepHandoff(markerPath, markerLockPath, 2, shedengine.StateRunning)

	first := observeEntry(true, runLockPath, statusPath, statusLockPath, markerPath, markerLockPath)
	if !first.Observed {
		t.Fatalf("observeEntry first call: Observed = false; want true")
	}
	if !first.CleanStepHandoff {
		t.Errorf("observeEntry first call: CleanStepHandoff = false; want true -- a matching marker must suppress the crash signature")
	}

	second := observeEntry(true, runLockPath, statusPath, statusLockPath, markerPath, markerLockPath)
	if second.CleanStepHandoff {
		t.Errorf("observeEntry second call: CleanStepHandoff = true; want false -- the marker is one-shot and the first call consumed it")
	}
}
