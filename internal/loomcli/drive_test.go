// drive_test.go — untagged Tier-1 tests for the Go driver's settle loop, over fake seams and a temp worktree.

package loomcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// driveTestEpoch is the time the scripted history and the failure record are laid out from.
var driveTestEpoch = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

// driveEnvelopeJSON renders the envelope a step prints for a stop with the given top-level fields and loop object.
func driveEnvelopeJSON(t *testing.T, fields map[string]any, loop map[string]any) []byte {
	t.Helper()
	if loop != nil {
		fields["loop"] = loop
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	return raw
}

// driveStatusFor derives the status an unmoved run shows at an envelope's stop.
func driveStatusFor(t *testing.T, raw []byte) shedengine.Status {
	t.Helper()
	stop, err := parseDriveStop(raw)
	if err != nil {
		t.Fatalf("parse envelope: %v", err)
	}
	status := shedengine.Status{State: shedengine.State(stop.key().State), CurrentProducer: stop.CurrentProducer}
	for i := 0; i < stop.HistoryLength; i++ {
		status.History = append(status.History, shedengine.HistoryEntry{Producer: "Row", Outcome: shedengine.Done, At: driveTestEpoch.Add(time.Duration(i) * time.Second).Format(time.RFC3339)})
	}
	return status
}

// verifyHaltStatus is the status of a run whose Publish row halted on a verify failure at HEAD "abc".
func verifyHaltStatus() shedengine.Status {
	return shedengine.Status{
		State:           shedengine.StateBlocked,
		CurrentProducer: loomshed.NamePublish,
		History: []shedengine.HistoryEntry{
			{Producer: "Plan", Outcome: shedengine.Done, At: driveTestEpoch.Format(time.RFC3339)},
			{Producer: loomshed.NamePublish, Outcome: shedengine.Stuck, At: driveTestEpoch.Add(10 * time.Second).Format(time.RFC3339)},
		},
	}
}

// TestGoDriver_Settle drives the loop over scripted envelopes and statuses and pins, per row, the seams it calls in order, the settle records it appends and what a hand-back leaves on disk.
func TestGoDriver_Settle(t *testing.T) {
	t.Parallel()

	blocked := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "blocked"}, map[string]any{"state": "blocked", "current_producer": "Build", "history_length": 2})
	}
	done := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "done"}, map[string]any{"state": "done", "current_producer": "Build", "history_length": 3})
	}
	doneFrictionFailed := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "done", "friction": "failed"}, map[string]any{"state": "done", "current_producer": "Build", "history_length": 3})
	}
	paused := func(reason string) func(t *testing.T) []byte {
		return func(t *testing.T) []byte {
			return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "paused", "reason": reason}, map[string]any{"state": "paused", "current_producer": "Build", "history_length": 2})
		}
	}
	awaiting := func(notice string) func(t *testing.T) []byte {
		return func(t *testing.T) []byte {
			return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "awaiting", "parent_notice": notice}, map[string]any{"state": "awaiting", "current_producer": "Review", "history_length": 2})
		}
	}
	busy := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": false, "kind": "busy", "error": "the run is held by pid 42"}, nil)
	}
	interrupted := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": false, "kind": "interrupted", "error": "the step ended without an envelope"},
			map[string]any{"state": "running", "current_producer": "Build", "history_length": 2, "interrupt_policy": "reinvoke"})
	}
	verifyHalt := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "blocked"}, map[string]any{"state": "blocked", "current_producer": loomshed.NamePublish, "history_length": 2})
	}
	verifyRecord := battenshed.ChildPublishFailure{Row: loomshed.NamePublish, Head: "abc", Target: "Build", WrittenAt: driveTestEpoch.Add(5 * time.Second).Format(time.RFC3339)}
	staleRecord := verifyRecord
	staleRecord.WrittenAt = driveTestEpoch.Add(-time.Hour).Format(time.RFC3339)
	gotoMoved := shedengine.Status{
		State:           shedengine.StatePaused,
		CurrentProducer: "Build",
		History: []shedengine.HistoryEntry{
			{Producer: "Plan", Outcome: shedengine.Done, At: driveTestEpoch.Format(time.RFC3339)},
			{Producer: "Build", Outcome: shedengine.Stuck, At: driveTestEpoch.Add(time.Second).Format(time.RFC3339)},
			{Producer: "Build", Outcome: shedengine.OutcomeGoto, At: driveTestEpoch.Add(2 * time.Second).Format(time.RFC3339)},
		},
	}

	tests := []struct {
		name  string
		steps []func(t *testing.T) []byte
		// statuses, when set, is the status the run shows after each step; otherwise the status the step's envelope describes.
		statuses []shedengine.Status
		// settleSeed holds the settle records an earlier process left.
		settleSeed   []loomengine.SettleRecord
		runLockHeld  bool
		escalation   bool
		record       *battenshed.ChildPublishFailure
		watch        battenWatch
		watchAlive   bool
		battenSpent  bool
		pauseCovered []bool
		orchRecorded bool

		wantCalls     []string
		wantActions   []string
		wantReports   int
		wantParked    bool
		wantNotifies  int
		wantHandBack  bool
		wantStepCalls int
	}{
		{name: "done commits and exits with no park", steps: []func(*testing.T) []byte{done}, wantCalls: []string{"step", "commit-records"}},
		{
			name: "done with friction failed writes its report, notifies and parks nothing", steps: []func(*testing.T) []byte{doneFrictionFailed},
			orchRecorded: true, wantCalls: []string{"step", "commit-records"}, wantReports: 1, wantNotifies: 1,
		},
		{
			name: "busy exits with a busy-exit record and parks nothing", steps: []func(*testing.T) []byte{busy}, runLockHeld: true,
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleBusyExit},
		},
		{
			name: "paused with an empty reason writes the friction note and parks", steps: []func(*testing.T) []byte{paused("")},
			wantCalls: []string{"step", "note"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "paused with a reason writes no note", steps: []func(*testing.T) []byte{paused("pause requested")},
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "awaiting with no notice parks", steps: []func(*testing.T) []byte{awaiting("")},
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "awaiting with a notice relays it and parks", steps: []func(*testing.T) []byte{awaiting("review is ready")}, orchRecorded: true,
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantNotifies: 1, wantHandBack: true,
		},
		{
			name: "unwatched verify halt with a matching record goes to its target once, and a second such halt parks",
			steps: []func(*testing.T) []byte{verifyHalt, verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus(), verifyHaltStatus()}, record: &verifyRecord,
			wantCalls:   []string{"step", "goto:Build", "step"},
			wantActions: []string{loomengine.SettleVerifyRetry, loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "watched verify halt parks with no goto", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			record: &verifyRecord, watch: battenWatch{PID: 7}, watchAlive: true, orchRecorded: true,
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantNotifies: 1, wantHandBack: true,
		},
		{
			name: "verify halt whose batten marker shows the retry spent parks with no goto", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			record: &verifyRecord, watch: battenWatch{PID: 7, ScratchDir: "/scratch", Producer: "Run-Shed"}, battenSpent: true,
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "verify halt with a retry record from an earlier process parks with no goto", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			record:     &verifyRecord,
			settleSeed: []loomengine.SettleRecord{{StopKey: loomengine.ArbiterStopKey{CurrentProducer: loomshed.NamePublish, HistoryLength: 1, State: "blocked"}, Settler: loomengine.SettlerGoRule, Action: loomengine.SettleVerifyRetry}},
			wantCalls:  []string{"step"}, wantActions: []string{loomengine.SettleVerifyRetry, loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "a stale failure record from an earlier halt drives no goto", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			record:    &staleRecord,
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "interrupted under reinvoke re-steps and writes restep records", steps: []func(*testing.T) []byte{interrupted, interrupted, done},
			wantCalls: []string{"step", "step", "step", "commit-records"}, wantActions: []string{loomengine.SettleRestep, loomengine.SettleRestep},
		},
		{
			name: "a status showing a goto entry re-steps with no note and no park", steps: []func(*testing.T) []byte{blocked, done},
			statuses:  []shedengine.Status{gotoMoved, driveStatusFor(t, done(t))},
			wantCalls: []string{"step", "step", "commit-records"}, wantActions: []string{loomengine.SettleMoved},
		},
		{
			name: "a status showing running exits with a running-exit record", steps: []func(*testing.T) []byte{blocked},
			statuses:  []shedengine.Status{{State: shedengine.StateRunning, CurrentProducer: "Build"}},
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleRunningExit},
		},
		{
			name: "a run lock held by another process exits with a running-exit record", steps: []func(*testing.T) []byte{blocked}, runLockHeld: true,
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleRunningExit},
		},
		{
			name: "a pause covering the slug holds the re-step until it lifts", steps: []func(*testing.T) []byte{interrupted, done}, pauseCovered: []bool{true},
			wantCalls: []string{"step", "hold", "step", "commit-records"}, wantActions: []string{loomengine.SettleRestep},
		},
		{
			name: "a pause covering an unwatched verify halt holds the goto", steps: []func(*testing.T) []byte{verifyHalt, done},
			statuses: []shedengine.Status{verifyHaltStatus(), driveStatusFor(t, done(t))}, record: &verifyRecord, pauseCovered: []bool{true},
			wantCalls: []string{"step", "hold", "goto:Build", "step", "commit-records"}, wantActions: []string{loomengine.SettleVerifyRetry},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			settlePath := filepath.Join(root, "friction", "settle-records.jsonl")
			for _, record := range tt.settleSeed {
				if err := loomengine.AppendSettleRecord(settlePath, record); err != nil {
					t.Fatalf("seed settle record: %v", err)
				}
			}
			markerPath := filepath.Join(root, "scratch", "driver-parked")
			var calls []string
			var notified int
			stepsTaken := 0
			pauseAnswers := slices.Clone(tt.pauseCovered)
			reports := 0

			deps := goDriverDeps{
				RunID: "run-1",
				Step: func(ctx context.Context, attempt string) ([]byte, error) {
					if stepsTaken >= len(tt.steps) {
						t.Fatalf("Step called %d times; the row scripts %d", stepsTaken+1, len(tt.steps))
					}
					calls = append(calls, "step")
					raw := tt.steps[stepsTaken](t)
					stepsTaken++
					return raw, nil
				},
				Goto: func(ctx context.Context, target string) error {
					calls = append(calls, "goto:"+target)
					return nil
				},
				ReadStatus: func() (shedengine.Status, bool, error) {
					if len(tt.statuses) > 0 {
						return tt.statuses[min(stepsTaken, len(tt.statuses))-1], true, nil
					}
					return driveStatusFor(t, tt.steps[stepsTaken-1](t)), true, nil
				},
				RunLockHeld: func() (bool, error) { return tt.runLockHeld, nil },
				SaveEnvelope: func(raw []byte) (string, error) {
					path := filepath.Join(root, "drive-reports", fmt.Sprintf("envelope-%d.json", len(raw)))
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						return "", err
					}
					return path, os.WriteFile(path, raw, 0o644)
				},
				SettlePath:        settlePath,
				EscalationPending: func(string) (bool, error) { return tt.escalation, nil },
				VerifyRecord: func() (battenshed.ChildPublishFailure, bool, error) {
					if tt.record == nil {
						return battenshed.ChildPublishFailure{}, false, nil
					}
					return *tt.record, true, nil
				},
				Head:             func() (string, error) { return "abc", nil },
				VerifyRows:       []string{loomshed.NameMergeIn, loomshed.NamePublish},
				Watch:            func() (battenWatch, bool) { return tt.watch, tt.watchAlive },
				BattenRetrySpent: func(battenWatch) (bool, error) { return tt.battenSpent, nil },
				Pause: func(time.Time) (pauserecord.Coverage, error) {
					if len(pauseAnswers) == 0 {
						return pauserecord.Coverage{}, nil
					}
					covered := pauseAnswers[0]
					pauseAnswers = pauseAnswers[1:]
					return pauserecord.Coverage{Covered: covered}, nil
				},
				PauseHold: func(context.Context) error {
					calls = append(calls, "hold")
					return nil
				},
				NewReportPath: func() string {
					reports++
					return filepath.Join(root, "drive-reports", fmt.Sprintf("report-%d.md", reports))
				},
				Park: driveParkDeps{
					Commit: commitStatusDeps{
						MergeActive: func() (bool, error) { return false, nil },
						Commit:      func(string) error { return nil },
						Push:        func() error { return nil },
					},
					MarkerPath:   markerPath,
					Notify:       func(string) error { notified++; return nil },
					OrchRecorded: func() (bool, error) { return tt.orchRecorded, nil },
					Now:          func() time.Time { return driveTestEpoch },
				},
				CommitRecords: func() error {
					calls = append(calls, "commit-records")
					return nil
				},
				WriteFrictionNote: func(driveStop) error {
					calls = append(calls, "note")
					return nil
				},
				Now: func() time.Time { return driveTestEpoch },
			}

			if err := (&goDriver{deps: deps}).run(context.Background()); err != nil {
				t.Fatalf("run() = %v; want nil", err)
			}

			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("seam calls = %q; want %q", calls, tt.wantCalls)
			}
			records, err := loomengine.ReadSettleRecords(settlePath)
			if err != nil {
				t.Fatalf("ReadSettleRecords: %v", err)
			}
			var gotActions []string
			for _, record := range records {
				gotActions = append(gotActions, record.Action)
			}
			if !slices.Equal(gotActions, tt.wantActions) {
				t.Errorf("settle actions = %q; want %q", gotActions, tt.wantActions)
			}
			if tt.wantHandBack {
				last := records[len(records)-1]
				if last.Action != loomengine.SettleHandBack || last.Settler != loomengine.SettlerParent {
					t.Errorf("last settle record = %+v; want a hand-back with settler %q", last, loomengine.SettlerParent)
				}
			}
			if reports != tt.wantReports {
				t.Errorf("stop reports taken = %d; want %d", reports, tt.wantReports)
			}
			for i := 1; i <= tt.wantReports; i++ {
				if _, err := os.Stat(filepath.Join(root, "drive-reports", fmt.Sprintf("report-%d.md", i))); err != nil {
					t.Errorf("stop report %d is not on disk: %v", i, err)
				}
			}
			_, markerErr := os.Stat(markerPath)
			if parked := markerErr == nil; parked != tt.wantParked {
				t.Errorf("park marker present = %t; want %t (%v)", parked, tt.wantParked, markerErr)
			}
			if tt.wantParked {
				marker, err := os.ReadFile(markerPath)
				if err != nil || string(marker) != filepath.Join(root, "drive-reports", fmt.Sprintf("report-%d.md", reports)) {
					t.Errorf("park marker = %q (%v); want the last stop report's path", marker, err)
				}
			}
			if notified != tt.wantNotifies {
				t.Errorf("Notify reached %d times; want %d", notified, tt.wantNotifies)
			}
		})
	}
}
