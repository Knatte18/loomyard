// drive_test.go — untagged Tier-1 tests for the shed driver's settle loop, over fake seams and a temp worktree.

package sheddriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// driveTestEpoch is the time the scripted history is laid out from.
var driveTestEpoch = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

// verifyTestRow is the row a verify halt blocks at in these tests.
const verifyTestRow = "Publish"

// arbiterTestRole is the role name the scripted arbiter outcomes carry.
const arbiterTestRole = "arbiter"

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
	status := shedengine.Status{State: shedengine.State(stop.Key().State), CurrentProducer: stop.CurrentProducer}
	for i := 0; i < stop.HistoryLength; i++ {
		status.History = append(status.History, shedengine.HistoryEntry{Producer: "Row", Outcome: shedengine.Done, At: driveTestEpoch.Add(time.Duration(i) * time.Second).Format(time.RFC3339)})
	}
	return status
}

// verifyHaltStatus is the status of a run whose verify row halted.
func verifyHaltStatus() shedengine.Status {
	return shedengine.Status{
		State:           shedengine.StateBlocked,
		CurrentProducer: verifyTestRow,
		History: []shedengine.HistoryEntry{
			{Producer: "Plan", Outcome: shedengine.Done, At: driveTestEpoch.Format(time.RFC3339)},
			{Producer: verifyTestRow, Outcome: shedengine.Stuck, At: driveTestEpoch.Add(10 * time.Second).Format(time.RFC3339)},
		},
	}
}

// wiredExecutable is the lyx path the settle tests resolve at wiring time; a hand-over must spawn its successor from it.
const wiredExecutable = "/wired/bin/lyx"

// TestGoDriver_Settle drives the loop over scripted envelopes and statuses and pins, per row, the seams it calls in order, the settle records it appends, how the run ended and what a hand-back leaves on disk.
// The no-group rows leave one optional seam group nil and pin that the stop meets the later rows unchanged.
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
		return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "blocked"}, map[string]any{"state": "blocked", "current_producer": verifyTestRow, "history_length": 2})
	}
	transientFailure := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": false, "kind": "producer", "transient": "api", "error": "the API timed out"},
			map[string]any{"state": "failed", "current_producer": "Build", "history_length": 2})
	}
	unseededFailure := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": false, "kind": "unseeded", "error": "no seed"},
			map[string]any{"state": "failed", "current_producer": "Build", "history_length": 2})
	}
	blockedLater := func(t *testing.T) []byte {
		return driveEnvelopeJSON(t, map[string]any{"ok": true, "state": "blocked"}, map[string]any{"state": "blocked", "current_producer": "Build", "history_length": 3})
	}
	blockedKey := StopKey{CurrentProducer: "Build", HistoryLength: 2, State: "blocked"}
	awaitingKey := StopKey{CurrentProducer: "Review", HistoryLength: 2, State: "awaiting"}
	blockedStatus := shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Build", History: []shedengine.HistoryEntry{
		{Producer: "Plan", Outcome: shedengine.Done, At: driveTestEpoch.Format(time.RFC3339)},
		{Producer: "Build", Outcome: shedengine.Stuck, At: driveTestEpoch.Add(time.Second).Format(time.RFC3339)},
	}}
	awaitingStatus := shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: "Review", History: blockedStatus.History}
	tacklerOutcome := func(verdict string, attempt int) TacklerEnvelope {
		dir := fmt.Sprintf("tackler/%d", attempt)
		return TacklerEnvelope{Verdict: verdict, Reason: "because", Question: "what now?", Attempt: attempt, AttemptDir: dir, VerdictPath: dir + "/verdict.yaml", CaseBriefPath: dir + "/case-brief.md"}
	}
	arbiterOutcome := func(verdict string, attempt int) ArbiterOutcome {
		dir := fmt.Sprintf("arbiter/%d", attempt)
		return ArbiterOutcome{Resume: verdict == "resume", Verdict: verdict, Role: arbiterTestRole, Reason: "because", Question: "which way?", Attempt: attempt, AttemptDir: dir, VerdictPath: dir + "/verdict.yaml"}
	}
	repairSeed := func(key StopKey) []SettleRecord {
		record := SettleRecord{StopKey: key, Settler: SettlerGoRule, Action: SettleRestep}
		return []SettleRecord{record, record}
	}
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
		// statusSeq, when set, is what the nth status read after the first step returns, the last entry repeating.
		statusSeq []shedengine.Status
		// initialStatus is what the status read before the first step returns; without it the run has no status file.
		initialStatus *shedengine.Status
		// settleSeed holds the settle records an earlier process left.
		settleSeed  []SettleRecord
		runLockHeld bool
		escalation  bool
		// haltTarget, when set, is the row the verify seam's Halt reports as matching the blocked stop.
		haltTarget string
		// watched is what the Watched seam reports; retrySpentElsewhere is what the verify seam's RetrySpentElsewhere reports.
		watched             bool
		retrySpentElsewhere bool
		pauseCovered        []bool
		orchRecorded        bool

		// The agent seams: what Tackler and Arbiter return, and what ConsumeTackler refuses with.
		tacklerReturns TacklerEnvelope
		tacklerErr     error
		arbiterReturns ArbiterOutcome
		arbiterErr     error
		consumeErr     error
		// currentSnap and attemptSnap are the worktree snapshot now and the one the tackler attempt recorded at spawn.
		currentSnap, attemptSnap WorktreeSnapshot
		// open names the attempts OpenAttempts lists for openKey; restartAttempt also writes the attempt record and stop envelope the tackler directory holds, and restartArbiter writes the stop envelope the arbiter seam's AttemptEnvelope returns.
		openKey        *StopKey
		openTackler    string
		openArbiter    string
		openApplied    bool
		restartAttempt bool
		restartArbiter bool

		// The hand-over seams: whether the last step ran another build than the on-disk one, the uncommitted paths, and what the successor spawn fails with.
		buildChanged bool
		dirty        []string
		spawnErr     error

		// Each no-group field leaves that optional group nil for the row.
		noArbiter, noVerify, noHandOver, noTamper, noCommit bool

		wantCalls    []string
		wantActions  []string
		wantSettlers []string
		// wantReportHas maps a stop report's number, from 1, to substrings it holds.
		wantReportHas map[int][]string
		wantReports   int
		wantParked    bool
		wantNotifies  int
		wantHandBack  bool
		wantExit      Exit
		// wantOrder, when set, is the order of the parent notice, the lock release and the successor spawn.
		wantOrder []string
		// wantBuild, when set, is the build the last settle record names.
		wantBuild string
	}{
		{name: "done commits and exits with no park", steps: []func(*testing.T) []byte{done}, wantCalls: []string{"step", "commit-records"}, wantExit: ExitDone},
		{
			name: "done with friction failed writes its report, notifies and parks nothing", steps: []func(*testing.T) []byte{doneFrictionFailed},
			orchRecorded: true, wantCalls: []string{"step", "commit-records"}, wantReports: 1, wantNotifies: 1, wantExit: ExitDone,
		},
		{
			name: "busy exits with a busy-exit record and parks nothing", steps: []func(*testing.T) []byte{busy}, runLockHeld: true,
			wantCalls: []string{"step"}, wantActions: []string{SettleBusyExit}, wantExit: ExitBusy,
		},
		{
			name: "paused with an empty reason writes the friction note and parks", steps: []func(*testing.T) []byte{paused("")},
			wantCalls: []string{"step", "note"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "paused with a reason writes no note", steps: []func(*testing.T) []byte{paused("pause requested")},
			wantCalls: []string{"step"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "awaiting with no notice parks", steps: []func(*testing.T) []byte{awaiting("")},
			wantCalls: []string{"step"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "awaiting with a notice relays it and parks", steps: []func(*testing.T) []byte{awaiting("review is ready")}, orchRecorded: true,
			wantCalls: []string{"step"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantNotifies: 1, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "unwatched verify halt with a matching record goes to its target once, and a second such halt parks",
			steps: []func(*testing.T) []byte{verifyHalt, verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus(), verifyHaltStatus()}, haltTarget: "Build",
			wantCalls:   []string{"step", "goto:Build", "step"},
			wantActions: []string{SettleVerifyRetry, SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "watched verify halt parks with no goto", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			haltTarget: "Build", watched: true, orchRecorded: true,
			wantCalls: []string{"step"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantNotifies: 1, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "verify halt whose retry another actor spent parks with no goto", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			haltTarget: "Build", retrySpentElsewhere: true,
			wantCalls: []string{"step"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "verify halt with a retry record from an earlier process parks with no goto", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			haltTarget: "Build",
			settleSeed: []SettleRecord{{StopKey: StopKey{CurrentProducer: verifyTestRow, HistoryLength: 1, State: "blocked"}, Settler: SettlerGoRule, Action: SettleVerifyRetry}},
			wantCalls:  []string{"step"}, wantActions: []string{SettleVerifyRetry, SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "a verify halt that matches no record drives no goto and reaches the tackler", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			tacklerReturns: tacklerOutcome(TacklerVerdictHandBack, 1),
			wantCalls:      []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions:    []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerTackler, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
			wantReportHas: map[int][]string{2: {"what now?", "tackler attempt 1: verdict hand-back", "Friction directory: /loom-friction"}},
		},
		{
			name:  "a transient stop calls the tackler once, and its resume is consumed then re-stepped",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(TacklerVerdictResume, 1),
			wantCalls:   []string{"step", "tackler", "snapshot", "snapshot", "consume:tackler/1", "step", "commit-records"},
			wantActions: []string{SettleSpawn, SettleRestep}, wantSettlers: []string{SettlerTackler, SettlerTackler}, wantReports: 1, wantExit: ExitDone,
		},
		{
			name:  "a third reinvoke failure reaches the tackler, and a hand-back parks with its question",
			steps: []func(*testing.T) []byte{interrupted}, settleSeed: repairSeed(StopKey{CurrentProducer: "Build", HistoryLength: 2, State: "running"}),
			tacklerReturns: tacklerOutcome(TacklerVerdictHandBack, 1),
			wantCalls:      []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions:    []string{SettleRestep, SettleRestep, SettleSpawn, SettleHandBack},
			wantSettlers:   []string{SettlerGoRule, SettlerGoRule, SettlerTackler, SettlerParent},
			wantReports:    2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack, wantReportHas: map[int][]string{2: {"what now?"}},
		},
		{
			name:  "a tackler escalate calls the arbiter with the case brief, and the arbiter's resume steps with its attempt number",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(TacklerVerdictEscalate, 1), arbiterReturns: arbiterOutcome("resume", 1),
			wantCalls:    []string{"step", "tackler", "snapshot", "snapshot", "arbiter:tackler/1/case-brief.md", "step:1", "commit-records"},
			wantActions:  []string{SettleSpawn, SettleSpawn, SettleRestep},
			wantSettlers: []string{SettlerTackler, SettlerArbiter, SettlerArbiter}, wantReports: 2, wantExit: ExitDone,
		},
		{
			name:  "an escalate at an unseeded error hands back when the arbiter refuses it as not eligible",
			steps: []func(*testing.T) []byte{unseededFailure}, tacklerReturns: tacklerOutcome(TacklerVerdictEscalate, 1), arbiterErr: errors.New("arbiter: the stop is not eligible"),
			wantCalls:    []string{"step", "tackler", "snapshot", "snapshot", "arbiter:tackler/1/case-brief.md"},
			wantActions:  []string{SettleSpawn, SettleHandBack},
			wantSettlers: []string{SettlerTackler, SettlerParent}, wantReports: 3, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
			wantReportHas: map[int][]string{3: {"the arbiter refused the stop: arbiter: the stop is not eligible"}},
		},
		{
			name:  "tackler_max_spawns 0 hands back with no agent started and no spawn record",
			steps: []func(*testing.T) []byte{transientFailure}, tacklerErr: fmt.Errorf("%w: tackler_max_spawns: 0 turns the tackler off", ErrTacklerUnavailable),
			wantCalls: []string{"step", "tackler"}, wantActions: []string{SettleHandBack}, wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
			wantReportHas: map[int][]string{2: {"tackler_max_spawns: 0"}},
		},
		{
			name:  "an arbiter off refusal at an awaiting stop with a notice still relays it",
			steps: []func(*testing.T) []byte{awaiting("review is ready")}, escalation: true, orchRecorded: true, arbiterErr: errors.New("arbiter: arbiter_max_spawns: 0 turns the arbiter off"),
			wantCalls: []string{"step", "arbiter:"}, wantActions: []string{SettleHandBack}, wantReports: 2, wantParked: true, wantNotifies: 1, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "a resume whose consume is refused as moved is acted on as a new stop",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(TacklerVerdictResume, 1), consumeErr: errors.New("the run moved since the tackler spawned"),
			statusSeq:   []shedengine.Status{driveStatusFor(t, transientFailure(t)), driveStatusFor(t, transientFailure(t)), gotoMoved},
			wantCalls:   []string{"step", "tackler", "snapshot", "snapshot", "consume:tackler/1", "step", "commit-records"},
			wantActions: []string{SettleSpawn, SettleMoved}, wantSettlers: []string{SettlerTackler, SettlerGoRule}, wantReports: 1, wantExit: ExitDone,
		},
		{
			name:  "an orch goto during the tackler wait discards the verdict, notes the change and re-steps",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(TacklerVerdictResume, 1),
			statusSeq:   []shedengine.Status{driveStatusFor(t, transientFailure(t)), gotoMoved},
			wantCalls:   []string{"step", "tackler", "step", "commit-records"},
			wantActions: []string{SettleSpawn, SettleMoved}, wantSettlers: []string{SettlerTackler, SettlerGoRule}, wantReports: 1, wantExit: ExitDone,
			wantReportHas: map[int][]string{1: {"The run moved while the tackler worked"}},
		},
		{
			name:  "a changed worktree with the stop key unchanged hands back as tampered",
			steps: []func(*testing.T) []byte{transientFailure}, tacklerReturns: tacklerOutcome(TacklerVerdictResume, 1),
			currentSnap: WorktreeSnapshot{Head: "b"}, attemptSnap: WorktreeSnapshot{Head: "a"},
			wantCalls:   []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerTackler, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack, wantReportHas: map[int][]string{2: {"cause tampered"}},
		},
		{
			name:  "a stop key returning after an applied tackler resume goes to the arbiter",
			steps: []func(*testing.T) []byte{blocked}, openKey: &blockedKey, openTackler: "tackler/1", openApplied: true, arbiterReturns: arbiterOutcome("hand-on", 1),
			wantCalls:   []string{"step", "arbiter:"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerArbiter, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack, wantReportHas: map[int][]string{2: {"which way?", "arbiter attempt 1: verdict hand-on"}},
		},
		{
			name:  "a status moved to an awaiting stop with a pending escalation calls the arbiter with a readable envelope",
			steps: []func(*testing.T) []byte{blocked}, statusSeq: []shedengine.Status{awaitingStatus}, escalation: true, arbiterReturns: arbiterOutcome("hand-on", 1),
			wantCalls:   []string{"step", "arbiter:"},
			wantActions: []string{SettleMoved, SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerGoRule, SettlerArbiter, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "an awaiting stop with a pending escalation calls the arbiter, and a hand-on parks with the notice relayed",
			steps: []func(*testing.T) []byte{awaiting("review is ready")}, escalation: true, orchRecorded: true, arbiterReturns: arbiterOutcome("hand-on", 1),
			wantCalls:   []string{"step", "arbiter:"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerArbiter, SettlerParent},
			wantReports: 2, wantParked: true, wantNotifies: 2, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "a halted run with an open tackler attempt calls the tackler before any step and writes no spawn record for it",
			steps: []func(*testing.T) []byte{}, initialStatus: &blockedStatus, restartAttempt: true, openKey: &blockedKey, openTackler: "tackler/1",
			tacklerReturns: func() TacklerEnvelope {
				envelope := tacklerOutcome(TacklerVerdictHandBack, 1)
				envelope.AttemptDir = "reuse"
				return envelope
			}(),
			wantCalls: []string{"tackler", "snapshot", "snapshot"}, wantActions: []string{SettleHandBack}, wantSettlers: []string{SettlerParent}, wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "a halted run with an open applied tackler attempt goes to the arbiter, never consumes it, and writes no spawn record for a reused arbiter attempt",
			steps: []func(*testing.T) []byte{}, initialStatus: &blockedStatus, restartAttempt: true, openKey: &blockedKey, openTackler: "tackler/1", openArbiter: "arbiter/1", openApplied: true,
			arbiterReturns: arbiterOutcome("hand-on", 1),
			wantCalls:      []string{"arbiter:"}, wantActions: []string{SettleHandBack}, wantSettlers: []string{SettlerParent}, wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "a halted run with an open arbiter attempt and no tackler attempt reads the arbiter's stop envelope and calls the arbiter before any step",
			steps: []func(*testing.T) []byte{}, initialStatus: &awaitingStatus, restartArbiter: true, openKey: &awaitingKey, openArbiter: "arbiter/1", escalation: true,
			arbiterReturns: arbiterOutcome("hand-on", 1),
			wantCalls:      []string{"attempt-envelope", "arbiter:"}, wantActions: []string{SettleHandBack}, wantSettlers: []string{SettlerParent}, wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "an attempt a later hand-back closed steps first, and the tackler judges only the next stop",
			steps: []func(*testing.T) []byte{blockedLater}, initialStatus: &blockedStatus, restartAttempt: true, openKey: &blockedKey, openTackler: "tackler/1",
			settleSeed: []SettleRecord{
				{StopKey: blockedKey, Settler: SettlerTackler, Action: SettleSpawn, At: driveTestEpoch},
				{StopKey: blockedKey, Settler: SettlerParent, Action: SettleHandBack, At: driveTestEpoch.Add(time.Minute)},
			},
			tacklerReturns: tacklerOutcome(TacklerVerdictHandBack, 2),
			wantCalls:      []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions:    []string{SettleSpawn, SettleHandBack, SettleSpawn, SettleHandBack},
			wantSettlers:   []string{SettlerTackler, SettlerParent, SettlerTackler, SettlerParent}, wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "a pause covering the slug starts no tackler until the hold returns", steps: []func(*testing.T) []byte{transientFailure},
			pauseCovered: []bool{true}, tacklerReturns: tacklerOutcome(TacklerVerdictHandBack, 1),
			wantCalls:   []string{"step", "hold", "tackler", "snapshot", "snapshot"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerTackler, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "a pause covering the slug starts no arbiter until the hold returns", steps: []func(*testing.T) []byte{awaiting("")}, escalation: true,
			pauseCovered: []bool{true}, arbiterReturns: arbiterOutcome("hand-on", 1),
			wantCalls:   []string{"step", "hold", "arbiter:"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerArbiter, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "a changed build on a clean tree records the hand-over, parks, notifies before releasing the lock, spawns and exits",
			steps: []func(*testing.T) []byte{blocked}, buildChanged: true, orchRecorded: true,
			wantCalls: []string{"step", "release", "spawn:" + wiredExecutable}, wantOrder: []string{"notify", "release", "spawn"},
			wantActions: []string{SettleHandOver}, wantSettlers: []string{SettlerGoRule}, wantBuild: "newbuild1234",
			wantReports: 1, wantParked: true, wantNotifies: 1, wantExit: ExitHandOver,
		},
		{
			name:  "the same stop key and build a second time does not hand over again and goes to the tackler",
			steps: []func(*testing.T) []byte{blocked}, buildChanged: true, tacklerReturns: tacklerOutcome(TacklerVerdictHandBack, 1),
			settleSeed:  []SettleRecord{{StopKey: blockedKey, Settler: SettlerGoRule, Action: SettleHandOver, Build: "newbuild1234"}},
			wantCalls:   []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions: []string{SettleHandOver, SettleSpawn, SettleHandBack},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "a dirty tree hands back naming its paths and spawns nothing", steps: []func(*testing.T) []byte{blocked}, buildChanged: true, dirty: []string{"a.go", "b.go"},
			wantCalls: []string{"step"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
			wantReportHas: map[int][]string{1: {"a.go, b.go"}},
		},
		{
			name: "a failing successor spawn leaves the report and the park marker on disk", steps: []func(*testing.T) []byte{blocked}, buildChanged: true, spawnErr: errors.New("no such binary"),
			wantCalls: []string{"step", "release", "spawn:" + wiredExecutable}, wantActions: []string{SettleHandOver}, wantBuild: "newbuild1234", wantReports: 1, wantParked: true, wantExit: ExitHandOver,
		},
		{
			name: "an awaiting stop is not binary-eligible and takes its usual hand-back", steps: []func(*testing.T) []byte{awaiting("")}, buildChanged: true,
			wantCalls: []string{"step"}, wantActions: []string{SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name: "a pause covering the slug holds the hand-over until the hold returns", steps: []func(*testing.T) []byte{blocked}, buildChanged: true, pauseCovered: []bool{true},
			wantCalls: []string{"step", "hold", "release", "spawn:" + wiredExecutable}, wantActions: []string{SettleHandOver}, wantBuild: "newbuild1234", wantReports: 1, wantParked: true, wantExit: ExitHandOver,
		},
		{
			name: "interrupted under reinvoke re-steps and writes restep records", steps: []func(*testing.T) []byte{interrupted, interrupted, done},
			wantCalls: []string{"step", "step", "step", "commit-records"}, wantActions: []string{SettleRestep, SettleRestep}, wantExit: ExitDone,
		},
		{
			name: "a status showing a goto entry re-steps with no note and no park", steps: []func(*testing.T) []byte{blocked, done},
			statuses:  []shedengine.Status{gotoMoved, driveStatusFor(t, done(t))},
			wantCalls: []string{"step", "step", "commit-records"}, wantActions: []string{SettleMoved}, wantExit: ExitDone,
		},
		{
			name: "a status showing running exits with a running-exit record", steps: []func(*testing.T) []byte{blocked},
			statuses:  []shedengine.Status{{State: shedengine.StateRunning, CurrentProducer: "Build"}},
			wantCalls: []string{"step"}, wantActions: []string{SettleRunningExit}, wantExit: ExitRunningElsewhere,
		},
		{
			name: "a run lock held by another process exits with a running-exit record", steps: []func(*testing.T) []byte{blocked}, runLockHeld: true,
			wantCalls: []string{"step"}, wantActions: []string{SettleRunningExit}, wantExit: ExitRunningElsewhere,
		},
		{
			name: "a pause covering the slug holds the re-step until it lifts", steps: []func(*testing.T) []byte{interrupted, done}, pauseCovered: []bool{true},
			wantCalls: []string{"step", "hold", "step", "commit-records"}, wantActions: []string{SettleRestep}, wantExit: ExitDone,
		},
		{
			name: "a pause covering an unwatched verify halt holds the goto", steps: []func(*testing.T) []byte{verifyHalt, done},
			statuses: []shedengine.Status{verifyHaltStatus(), driveStatusFor(t, done(t))}, haltTarget: "Build", pauseCovered: []bool{true},
			wantCalls: []string{"step", "hold", "goto:Build", "step", "commit-records"}, wantActions: []string{SettleVerifyRetry}, wantExit: ExitDone,
		},
		{
			name:  "no arbiter: a returned tackler resume hands back with no arbiter call",
			steps: []func(*testing.T) []byte{blocked}, openKey: &blockedKey, openTackler: "tackler/1", openApplied: true, noArbiter: true,
			wantCalls:   []string{"step"},
			wantActions: []string{SettleHandBack}, wantSettlers: []string{SettlerParent},
			wantReports: 1, wantParked: true, wantHandBack: true, wantExit: ExitHandBack, wantReportHas: map[int][]string{1: {"no arbiter is wired: hand back"}},
		},
		{
			name:  "no arbiter: a tackler escalate hands back and the stop report's question names the case brief",
			steps: []func(*testing.T) []byte{transientFailure}, tacklerReturns: tacklerOutcome(TacklerVerdictEscalate, 1), noArbiter: true,
			wantCalls:   []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerTackler, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
			wantReportHas: map[int][]string{2: {"no stronger agent is wired; read its case brief at tackler/1/case-brief.md"}},
		},
		{
			name:  "no hand-over seam: a blocked stop whose last step ran another build goes to the tackler with no successor spawned",
			steps: []func(*testing.T) []byte{blocked}, buildChanged: true, noHandOver: true, tacklerReturns: tacklerOutcome(TacklerVerdictHandBack, 1),
			wantCalls:   []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerTackler, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "no verify seam: a blocked stop goes to the tackler, never a goto",
			steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()}, haltTarget: "Build", noVerify: true, tacklerReturns: tacklerOutcome(TacklerVerdictHandBack, 1),
			wantCalls:   []string{"step", "tackler", "snapshot", "snapshot"},
			wantActions: []string{SettleSpawn, SettleHandBack}, wantSettlers: []string{SettlerTackler, SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantExit: ExitHandBack,
		},
		{
			name:  "no tamper seam: a tackler resume is consumed with no snapshot taken and no tampered cause",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(TacklerVerdictResume, 1), noTamper: true,
			currentSnap: WorktreeSnapshot{Head: "b"}, attemptSnap: WorktreeSnapshot{Head: "a"},
			wantCalls:   []string{"step", "tackler", "consume:tackler/1", "step", "commit-records"},
			wantActions: []string{SettleSpawn, SettleRestep}, wantSettlers: []string{SettlerTackler, SettlerTackler}, wantReports: 1, wantExit: ExitDone,
		},
		{
			name:      "no commit seam: a done run exits with no commit",
			steps:     []func(*testing.T) []byte{done},
			noCommit:  true,
			wantCalls: []string{"step"}, wantExit: ExitDone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			settlePath := filepath.Join(root, "run", "settle-records.jsonl")
			for _, record := range tt.settleSeed {
				if err := AppendSettleRecord(settlePath, record); err != nil {
					t.Fatalf("seed settle record: %v", err)
				}
			}
			markerPath := filepath.Join(root, "scratch", "driver-parked")
			var calls, order []string
			var notified int
			stepsTaken := 0
			statusReads := 0
			pauseAnswers := slices.Clone(tt.pauseCovered)
			reports := 0

			openTackler := tt.openTackler
			if tt.restartAttempt {
				openTackler = filepath.Join(root, "tackler", "1")
				envelopePath := filepath.Join(root, "restart-envelope.json")
				if err := os.WriteFile(envelopePath, blocked(t), 0o644); err != nil {
					t.Fatalf("write the open attempt's stop envelope: %v", err)
				}
				attempt := TacklerAttempt{Number: 1, StopKey: blockedKey, EnvelopePath: envelopePath}
				if err := WriteTacklerAttempt(openTackler, attempt); err != nil {
					t.Fatalf("write the open attempt: %v", err)
				}
			}
			arbiterEnvelopePath := ""
			if tt.restartArbiter {
				arbiterEnvelopePath = filepath.Join(root, "restart-arbiter-envelope.json")
				if err := os.WriteFile(arbiterEnvelopePath, awaiting("")(t), 0o644); err != nil {
					t.Fatalf("write the open arbiter attempt's stop envelope: %v", err)
				}
			}

			deps := Deps{
				RunID: "run-1",
				Step: func(ctx context.Context, attempt string) ([]byte, error) {
					if stepsTaken >= len(tt.steps) {
						t.Fatalf("Step called %d times; the row scripts %d", stepsTaken+1, len(tt.steps))
					}
					label := "step"
					if attempt != "" {
						label += ":" + attempt
					}
					calls = append(calls, label)
					raw := tt.steps[stepsTaken](t)
					stepsTaken++
					return raw, nil
				},
				Goto: func(ctx context.Context, target string) error {
					calls = append(calls, "goto:"+target)
					return nil
				},
				ReadStatus: func() (shedengine.Status, bool, error) {
					if stepsTaken == 0 {
						if tt.initialStatus == nil {
							return shedengine.Status{}, false, nil
						}
						return *tt.initialStatus, true, nil
					}
					if len(tt.statusSeq) > 0 {
						statusReads++
						return tt.statusSeq[min(statusReads, len(tt.statusSeq))-1], true, nil
					}
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
				SettlePath:  settlePath,
				FrictionDir: "/loom-friction",
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
				HandBack: HandBackDeps{
					Notify: func(Stop, string) (bool, error) {
						notified++
						order = append(order, "notify")
						return true, nil
					},
					OrchRecorded: func() (bool, error) { return tt.orchRecorded, nil },
					Park: func(reportPath string) error {
						if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
							return err
						}
						return os.WriteFile(markerPath, []byte(reportPath), 0o644)
					},
					Now: func() time.Time { return driveTestEpoch },
				},
				Tackler: func(stop Stop, reportPath string) (TacklerEnvelope, error) {
					calls = append(calls, "tackler")
					envelope := tt.tacklerReturns
					if envelope.AttemptDir == "reuse" {
						envelope.AttemptDir = openTackler
					}
					return envelope, tt.tacklerErr
				},
				ConsumeTackler: func(attemptDir string) error {
					calls = append(calls, "consume:"+attemptDir)
					return tt.consumeErr
				},
				OpenAttempts: func(key StopKey) (string, string, bool, error) {
					if tt.openKey == nil || key != *tt.openKey {
						return "", "", false, nil
					}
					return openTackler, tt.openArbiter, tt.openApplied, nil
				},
				Now: func() time.Time { return driveTestEpoch },
				CommitRecords: func() error {
					calls = append(calls, "commit-records")
					return nil
				},
				WriteFrictionNote: func(Stop) error {
					calls = append(calls, "note")
					return nil
				},
				Watched: func() bool { return tt.watched },
				Arbiter: &ArbiterSeam{
					EscalationPending: func(string) (bool, error) { return tt.escalation, nil },
					Judge: func(envelopePath, reportPath, caseBriefPath string) (ArbiterOutcome, error) {
						calls = append(calls, "arbiter:"+caseBriefPath)
						if _, err := os.ReadFile(envelopePath); err != nil {
							t.Errorf("the arbiter was handed an unreadable stop envelope: %v", err)
						}
						return tt.arbiterReturns, tt.arbiterErr
					},
					AttemptEnvelope: func(string) (string, error) {
						calls = append(calls, "attempt-envelope")
						return arbiterEnvelopePath, nil
					},
				},
				Verify: &VerifySeam{
					Halt: func() (bool, string, error) { return tt.haltTarget != "", tt.haltTarget, nil },
					RetrySpentElsewhere: func() (bool, error) {
						return tt.retrySpentElsewhere, nil
					},
				},
				HandOver: &HandOverSeam{
					OnDiskBuild:  func() (buildvcs.Identity, error) { return buildvcs.Identity{Revision: "newbuild1234567"}, nil },
					BuildChanged: func(buildvcs.Identity) bool { return tt.buildChanged },
					DirtyPaths:   func() ([]string, error) { return tt.dirty, nil },
					ReleaseLock: func() error {
						calls = append(calls, "release")
						order = append(order, "release")
						return nil
					},
					Executable: wiredExecutable,
					SpawnSuccessor: func(executable string) error {
						calls = append(calls, "spawn:"+executable)
						order = append(order, "spawn")
						return tt.spawnErr
					},
				},
				Tamper: &TamperSeam{
					Snapshot: func() (WorktreeSnapshot, error) {
						calls = append(calls, "snapshot")
						return tt.currentSnap, nil
					},
					AttemptSnapshot: func(string) (WorktreeSnapshot, error) {
						calls = append(calls, "snapshot")
						return tt.attemptSnap, nil
					},
				},
			}
			if tt.noArbiter {
				deps.Arbiter = nil
			}
			if tt.noVerify {
				deps.Verify = nil
			}
			if tt.noHandOver {
				deps.HandOver = nil
			}
			if tt.noTamper {
				deps.Tamper = nil
			}
			if tt.noCommit {
				deps.CommitRecords = nil
			}

			exit, err := Run(context.Background(), deps)
			if err != nil {
				t.Fatalf("Run() = %v; want nil", err)
			}

			if exit != tt.wantExit {
				t.Errorf("Run() exit = %q; want %q", exit, tt.wantExit)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("seam calls = %q; want %q", calls, tt.wantCalls)
			}
			records, err := ReadSettleRecords(settlePath)
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
			if tt.wantSettlers != nil {
				var gotSettlers []string
				for _, record := range records {
					gotSettlers = append(gotSettlers, record.Settler)
				}
				if !slices.Equal(gotSettlers, tt.wantSettlers) {
					t.Errorf("settle settlers = %q; want %q", gotSettlers, tt.wantSettlers)
				}
			}
			for number, substrings := range tt.wantReportHas {
				report, err := os.ReadFile(filepath.Join(root, "drive-reports", fmt.Sprintf("report-%d.md", number)))
				if err != nil {
					t.Errorf("stop report %d: %v", number, err)
					continue
				}
				for _, substring := range substrings {
					if !strings.Contains(string(report), substring) {
						t.Errorf("stop report %d = %q; want it to hold %q", number, report, substring)
					}
				}
			}
			if tt.wantOrder != nil && !slices.Equal(order, tt.wantOrder) {
				t.Errorf("notice, release and spawn order = %q; want %q", order, tt.wantOrder)
			}
			if tt.wantBuild != "" {
				if last := records[len(records)-1]; last.Build != tt.wantBuild {
					t.Errorf("last settle record build = %q; want %q", last.Build, tt.wantBuild)
				}
			}
			if tt.wantHandBack {
				last := records[len(records)-1]
				if last.Action != SettleHandBack || last.Settler != SettlerParent {
					t.Errorf("last settle record = %+v; want a hand-back with settler %q", last, SettlerParent)
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

// TestDriveVerb pins the driver lock around a driver: a held lock refuses at once with the recipe's refusal and touches nothing, a lock directory that cannot be created ends the recipe's retry clause, a free lock clears the stale files, runs once and prints the run's fields, and a run that released the lock early for a hand-over leaves it free with no double release.
func TestDriveVerb(t *testing.T) {
	t.Parallel()

	const refusal = "test: a driver already drives this run; way forward: transient, the live driver settles it"
	const retryClause = "re-run the test verb"

	tests := []struct {
		name         string
		holdLock     bool
		blockLockDir bool
		releaseEarly bool
		wantCode     int
		wantRuns     int
		// wantStale is true when the stale files must still exist afterwards.
		wantStale bool
		wantText  string
	}{
		{name: "a held lock refuses with the recipe's refusal, never runs and leaves the stale files", holdLock: true, wantCode: 1, wantRuns: 0, wantStale: true, wantText: refusal},
		{name: "a lock directory that cannot be created ends in the retry clause", blockLockDir: true, wantCode: 1, wantRuns: 0, wantStale: true, wantText: "way forward: transient, " + retryClause},
		{name: "a free lock removes the stale files, runs once, prints the run's fields and leaves the lock free", wantCode: 0, wantRuns: 1, wantText: `"drove":true`},
		{name: "a run that released early for a hand-over leaves the lock free with no double release", releaseEarly: true, wantCode: 0, wantRuns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			lockPath := filepath.Join(root, "scratch", "driver.lock")
			stale := []string{filepath.Join(root, "driver-parked"), filepath.Join(root, "resume-report")}
			for _, path := range stale {
				if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
					t.Fatalf("seed stale file: %v", err)
				}
			}
			if tt.blockLockDir {
				blocker := filepath.Join(root, "blocker")
				if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
					t.Fatalf("seed the blocking file: %v", err)
				}
				lockPath = filepath.Join(blocker, "driver.lock")
			}
			if tt.holdLock {
				if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
					t.Fatalf("create lock directory: %v", err)
				}
				held, acquired, err := lock.TryAcquireWriteLock(lockPath)
				if err != nil || !acquired {
					t.Fatalf("test could not take the driver lock: acquired %t, %v", acquired, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}

			runs := 0
			var out strings.Builder
			code := DriveVerb(&out, DriveLock{Path: lockPath, Stale: stale, Refusal: refusal, RetryClause: retryClause}, func(release func() error) (map[string]any, error) {
				runs++
				if tt.releaseEarly {
					if err := release(); err != nil {
						t.Errorf("early release = %v; want nil", err)
					}
				}
				return map[string]any{"drove": true}, nil
			})

			if code != tt.wantCode {
				t.Errorf("DriveVerb() = %d; want %d (%s)", code, tt.wantCode, out.String())
			}
			if runs != tt.wantRuns {
				t.Errorf("run called %d times; want %d", runs, tt.wantRuns)
			}
			if tt.wantText != "" && !strings.Contains(out.String(), tt.wantText) {
				t.Errorf("output = %q; want it to hold %q", out.String(), tt.wantText)
			}
			for _, path := range stale {
				_, err := os.Stat(path)
				if exists := err == nil; exists != tt.wantStale {
					t.Errorf("stale file %s exists = %t; want %t", path, exists, tt.wantStale)
				}
			}
			if !tt.holdLock && !tt.blockLockDir {
				probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
				if err != nil || !acquired {
					t.Fatalf("driver lock is still held after the verb: acquired %t, %v", acquired, err)
				}
				_ = probe.Release()
			}
		})
	}
}
