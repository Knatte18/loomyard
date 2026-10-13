// drive_test.go — untagged Tier-1 tests for the Go driver's settle loop, over fake seams and a temp worktree.

package loomcli

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

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/buildvcs"
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
	blockedKey := loomengine.ArbiterStopKey{CurrentProducer: "Build", HistoryLength: 2, State: "blocked"}
	blockedStatus := shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Build", History: []shedengine.HistoryEntry{
		{Producer: "Plan", Outcome: shedengine.Done, At: driveTestEpoch.Format(time.RFC3339)},
		{Producer: "Build", Outcome: shedengine.Stuck, At: driveTestEpoch.Add(time.Second).Format(time.RFC3339)},
	}}
	awaitingStatus := shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: "Review", History: blockedStatus.History}
	tacklerOutcome := func(verdict string, attempt int) loomengine.TacklerEnvelope {
		dir := fmt.Sprintf("tackler/%d", attempt)
		return loomengine.TacklerEnvelope{Verdict: verdict, Reason: "because", Question: "what now?", Attempt: attempt, AttemptDir: dir, VerdictPath: dir + "/verdict.yaml", CaseBriefPath: dir + "/case-brief.md"}
	}
	arbiterOutcome := func(verdict string, attempt int) loomengine.ArbiterEnvelope {
		dir := fmt.Sprintf("arbiter/%d", attempt)
		return loomengine.ArbiterEnvelope{Verdict: verdict, Reason: "because", Question: "which way?", Attempt: attempt, AttemptDir: dir, VerdictPath: dir + "/verdict.yaml"}
	}
	repairSeed := func(key loomengine.ArbiterStopKey) []loomengine.SettleRecord {
		record := loomengine.SettleRecord{StopKey: key, Settler: loomengine.SettlerGoRule, Action: loomengine.SettleRestep}
		return []loomengine.SettleRecord{record, record}
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
		// statusSeq, when set, is what the nth status read after the first step returns, the last entry repeating.
		statusSeq []shedengine.Status
		// initialStatus is what the status read before the first step returns; without it the run has no status file.
		initialStatus *shedengine.Status
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

		// The agent seams: what Tackler and Arbiter return, and what ConsumeTackler refuses with.
		tacklerReturns loomengine.TacklerEnvelope
		tacklerErr     error
		arbiterReturns loomengine.ArbiterEnvelope
		arbiterErr     error
		consumeErr     error
		// currentSnap and attemptSnap are the worktree snapshot now and the one the tackler attempt recorded at spawn.
		currentSnap, attemptSnap loomengine.WorktreeSnapshot
		// open names the attempts OpenAttempts lists for openKey; restartAttempt also writes the attempt record and stop envelope the tackler directory holds.
		openKey        *loomengine.ArbiterStopKey
		openTackler    string
		openArbiter    string
		openApplied    bool
		restartAttempt bool

		// The hand-over seams: whether the last step ran another build than the on-disk one, the uncommitted paths, and what the successor spawn fails with.
		buildChanged bool
		dirty        []string
		spawnErr     error

		wantCalls    []string
		wantActions  []string
		wantSettlers []string
		// wantReportHas maps a stop report's number, from 1, to substrings it holds.
		wantReportHas map[int][]string
		wantReports   int
		wantParked    bool
		wantNotifies  int
		wantHandBack  bool
		// wantOrder, when set, is the order of the parent notice, the lock release and the successor spawn.
		wantOrder []string
		// wantBuild, when set, is the build the last settle record names.
		wantBuild string
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
			name:  "unwatched verify halt with a matching record goes to its target once, and a second such halt parks",
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
			name: "a stale failure record from an earlier halt drives no goto and reaches the tackler", steps: []func(*testing.T) []byte{verifyHalt}, statuses: []shedengine.Status{verifyHaltStatus()},
			record: &staleRecord, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictHandBack, 1),
			wantCalls:   []string{"step", "tackler"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantReportHas: map[int][]string{2: {"what now?", "tackler attempt 1: verdict hand-back"}},
		},
		{
			name:  "a transient stop calls the tackler once, and its resume is consumed then re-stepped",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictResume, 1),
			wantCalls:   []string{"step", "tackler", "consume:tackler/1", "step", "commit-records"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleRestep}, wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerTackler}, wantReports: 1,
		},
		{
			name:  "a third reinvoke failure reaches the tackler, and a hand-back parks with its question",
			steps: []func(*testing.T) []byte{interrupted}, settleSeed: repairSeed(loomengine.ArbiterStopKey{CurrentProducer: "Build", HistoryLength: 2, State: "running"}),
			tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictHandBack, 1),
			wantCalls:      []string{"step", "tackler"},
			wantActions:    []string{loomengine.SettleRestep, loomengine.SettleRestep, loomengine.SettleSpawn, loomengine.SettleHandBack},
			wantSettlers:   []string{loomengine.SettlerGoRule, loomengine.SettlerGoRule, loomengine.SettlerTackler, loomengine.SettlerParent},
			wantReports:    2, wantParked: true, wantHandBack: true, wantReportHas: map[int][]string{2: {"what now?"}},
		},
		{
			name:  "a tackler escalate calls the arbiter with the case brief, and the arbiter's resume steps with its attempt number",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictEscalate, 1), arbiterReturns: arbiterOutcome(loomengine.ArbiterVerdictResume, 1),
			wantCalls:    []string{"step", "tackler", "arbiter:tackler/1/case-brief.md", "step:1", "commit-records"},
			wantActions:  []string{loomengine.SettleSpawn, loomengine.SettleSpawn, loomengine.SettleRestep},
			wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerArbiter, loomengine.SettlerArbiter}, wantReports: 2,
		},
		{
			name:  "an escalate at an unseeded error hands back when the arbiter refuses it as not eligible",
			steps: []func(*testing.T) []byte{unseededFailure}, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictEscalate, 1), arbiterErr: errors.New("arbiter: the stop is not eligible"),
			wantCalls:    []string{"step", "tackler", "arbiter:tackler/1/case-brief.md"},
			wantActions:  []string{loomengine.SettleSpawn, loomengine.SettleHandBack},
			wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerParent}, wantReports: 3, wantParked: true, wantHandBack: true,
			wantReportHas: map[int][]string{3: {"the arbiter refused the stop: arbiter: the stop is not eligible"}},
		},
		{
			name:  "tackler_max_spawns 0 hands back with no agent started and no spawn record",
			steps: []func(*testing.T) []byte{transientFailure}, tacklerErr: fmt.Errorf("%w: tackler_max_spawns: 0 turns the tackler off", errTacklerUnavailable),
			wantCalls: []string{"step", "tackler"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 2, wantParked: true, wantHandBack: true,
			wantReportHas: map[int][]string{2: {"tackler_max_spawns: 0"}},
		},
		{
			name:  "an arbiter off refusal at an awaiting stop with a notice still relays it",
			steps: []func(*testing.T) []byte{awaiting("review is ready")}, escalation: true, orchRecorded: true, arbiterErr: errors.New("arbiter: arbiter_max_spawns: 0 turns the arbiter off"),
			wantCalls: []string{"step", "arbiter:"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 2, wantParked: true, wantNotifies: 1, wantHandBack: true,
		},
		{
			name:  "a resume whose consume is refused as moved is acted on as a new stop",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictResume, 1), consumeErr: errors.New("the run moved since the tackler spawned"),
			statusSeq:   []shedengine.Status{driveStatusFor(t, transientFailure(t)), driveStatusFor(t, transientFailure(t)), gotoMoved},
			wantCalls:   []string{"step", "tackler", "consume:tackler/1", "step", "commit-records"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleMoved}, wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerGoRule}, wantReports: 1,
		},
		{
			name:  "an orch goto during the tackler wait discards the verdict, notes the change and re-steps",
			steps: []func(*testing.T) []byte{transientFailure, done}, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictResume, 1),
			statusSeq:   []shedengine.Status{driveStatusFor(t, transientFailure(t)), gotoMoved},
			wantCalls:   []string{"step", "tackler", "step", "commit-records"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleMoved}, wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerGoRule}, wantReports: 1,
			wantReportHas: map[int][]string{1: {"The run moved while the tackler worked"}},
		},
		{
			name:  "a changed worktree with the stop key unchanged hands back as tampered",
			steps: []func(*testing.T) []byte{transientFailure}, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictResume, 1),
			currentSnap: loomengine.WorktreeSnapshot{Head: "b"}, attemptSnap: loomengine.WorktreeSnapshot{Head: "a"},
			wantCalls:   []string{"step", "tackler"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantReportHas: map[int][]string{2: {"cause tampered"}},
		},
		{
			name:  "a stop key returning after an applied tackler resume goes to the arbiter",
			steps: []func(*testing.T) []byte{blocked}, openKey: &blockedKey, openTackler: "tackler/1", openApplied: true, arbiterReturns: arbiterOutcome(loomengine.ArbiterVerdictHandOn, 1),
			wantCalls:   []string{"step", "arbiter:"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerArbiter, loomengine.SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true, wantReportHas: map[int][]string{2: {"which way?"}},
		},
		{
			name:  "a status moved to an awaiting stop with a pending escalation calls the arbiter with a readable envelope",
			steps: []func(*testing.T) []byte{blocked}, statusSeq: []shedengine.Status{awaitingStatus}, escalation: true, arbiterReturns: arbiterOutcome(loomengine.ArbiterVerdictHandOn, 1),
			wantCalls:   []string{"step", "arbiter:"},
			wantActions: []string{loomengine.SettleMoved, loomengine.SettleSpawn, loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerGoRule, loomengine.SettlerArbiter, loomengine.SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true,
		},
		{
			name:  "an awaiting stop with a pending escalation calls the arbiter, and a hand-on parks with the notice relayed",
			steps: []func(*testing.T) []byte{awaiting("review is ready")}, escalation: true, orchRecorded: true, arbiterReturns: arbiterOutcome(loomengine.ArbiterVerdictHandOn, 1),
			wantCalls:   []string{"step", "arbiter:"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerArbiter, loomengine.SettlerParent},
			wantReports: 2, wantParked: true, wantNotifies: 2, wantHandBack: true,
		},
		{
			name:  "a halted run with an open tackler attempt calls the tackler before any step and writes no spawn record for it",
			steps: []func(*testing.T) []byte{}, initialStatus: &blockedStatus, restartAttempt: true, openKey: &blockedKey, openTackler: "tackler/1",
			tacklerReturns: func() loomengine.TacklerEnvelope {
				envelope := tacklerOutcome(loomengine.TacklerVerdictHandBack, 1)
				envelope.AttemptDir = "reuse"
				return envelope
			}(),
			wantCalls: []string{"tackler"}, wantActions: []string{loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerParent}, wantReports: 2, wantParked: true, wantHandBack: true,
		},
		{
			name:  "a halted run with an open applied tackler attempt goes to the arbiter, never consumes it, and writes no spawn record for a reused arbiter attempt",
			steps: []func(*testing.T) []byte{}, initialStatus: &blockedStatus, restartAttempt: true, openKey: &blockedKey, openTackler: "tackler/1", openArbiter: "arbiter/1", openApplied: true,
			arbiterReturns: arbiterOutcome(loomengine.ArbiterVerdictHandOn, 1),
			wantCalls:      []string{"arbiter:"}, wantActions: []string{loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerParent}, wantReports: 2, wantParked: true, wantHandBack: true,
		},
		{
			name:  "an attempt a later hand-back closed steps first, and the tackler judges only the next stop",
			steps: []func(*testing.T) []byte{blockedLater}, initialStatus: &blockedStatus, restartAttempt: true, openKey: &blockedKey, openTackler: "tackler/1",
			settleSeed: []loomengine.SettleRecord{
				{StopKey: blockedKey, Settler: loomengine.SettlerTackler, Action: loomengine.SettleSpawn, At: driveTestEpoch},
				{StopKey: blockedKey, Settler: loomengine.SettlerParent, Action: loomengine.SettleHandBack, At: driveTestEpoch.Add(time.Minute)},
			},
			tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictHandBack, 2),
			wantCalls:      []string{"step", "tackler"},
			wantActions:    []string{loomengine.SettleSpawn, loomengine.SettleHandBack, loomengine.SettleSpawn, loomengine.SettleHandBack},
			wantSettlers:   []string{loomengine.SettlerTackler, loomengine.SettlerParent, loomengine.SettlerTackler, loomengine.SettlerParent}, wantReports: 2, wantParked: true, wantHandBack: true,
		},
		{
			name: "a pause covering the slug starts no tackler until the hold returns", steps: []func(*testing.T) []byte{transientFailure},
			pauseCovered: []bool{true}, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictHandBack, 1),
			wantCalls:   []string{"step", "hold", "tackler"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerTackler, loomengine.SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true,
		},
		{
			name: "a pause covering the slug starts no arbiter until the hold returns", steps: []func(*testing.T) []byte{awaiting("")}, escalation: true,
			pauseCovered: []bool{true}, arbiterReturns: arbiterOutcome(loomengine.ArbiterVerdictHandOn, 1),
			wantCalls:   []string{"step", "hold", "arbiter:"},
			wantActions: []string{loomengine.SettleSpawn, loomengine.SettleHandBack}, wantSettlers: []string{loomengine.SettlerArbiter, loomengine.SettlerParent},
			wantReports: 2, wantParked: true, wantHandBack: true,
		},
		{
			name:  "a changed build on a clean tree records the hand-over, parks, notifies before releasing the lock, spawns and exits",
			steps: []func(*testing.T) []byte{blocked}, buildChanged: true, orchRecorded: true,
			wantCalls: []string{"step", "release", "spawn"}, wantOrder: []string{"notify", "release", "spawn"},
			wantActions: []string{loomengine.SettleHandOver}, wantSettlers: []string{loomengine.SettlerGoRule}, wantBuild: "newbuild1234",
			wantReports: 1, wantParked: true, wantNotifies: 1,
		},
		{
			name:  "the same stop key and build a second time does not hand over again and goes to the tackler",
			steps: []func(*testing.T) []byte{blocked}, buildChanged: true, tacklerReturns: tacklerOutcome(loomengine.TacklerVerdictHandBack, 1),
			settleSeed:  []loomengine.SettleRecord{{StopKey: blockedKey, Settler: loomengine.SettlerGoRule, Action: loomengine.SettleHandOver, Build: "newbuild1234"}},
			wantCalls:   []string{"step", "tackler"},
			wantActions: []string{loomengine.SettleHandOver, loomengine.SettleSpawn, loomengine.SettleHandBack},
			wantReports: 2, wantParked: true, wantHandBack: true,
		},
		{
			name: "a dirty tree hands back naming its paths and spawns nothing", steps: []func(*testing.T) []byte{blocked}, buildChanged: true, dirty: []string{"a.go", "b.go"},
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
			wantReportHas: map[int][]string{1: {"a.go, b.go"}},
		},
		{
			name: "a failing successor spawn leaves the report and the park marker on disk", steps: []func(*testing.T) []byte{blocked}, buildChanged: true, spawnErr: errors.New("no such binary"),
			wantCalls: []string{"step", "release", "spawn"}, wantActions: []string{loomengine.SettleHandOver}, wantBuild: "newbuild1234", wantReports: 1, wantParked: true,
		},
		{
			name: "an awaiting stop is not binary-eligible and takes its usual hand-back", steps: []func(*testing.T) []byte{awaiting("")}, buildChanged: true,
			wantCalls: []string{"step"}, wantActions: []string{loomengine.SettleHandBack}, wantReports: 1, wantParked: true, wantHandBack: true,
		},
		{
			name: "a pause covering the slug holds the hand-over until the hold returns", steps: []func(*testing.T) []byte{blocked}, buildChanged: true, pauseCovered: []bool{true},
			wantCalls: []string{"step", "hold", "release", "spawn"}, wantActions: []string{loomengine.SettleHandOver}, wantBuild: "newbuild1234", wantReports: 1, wantParked: true,
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
				attempt := loomengine.TacklerAttempt{Number: 1, StopKey: blockedKey, EnvelopePath: envelopePath}
				if err := loomengine.WriteTacklerAttempt(openTackler, attempt); err != nil {
					t.Fatalf("write the open attempt: %v", err)
				}
			}

			deps := goDriverDeps{
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
					MarkerPath: markerPath,
					Notify: func(string) error {
						notified++
						order = append(order, "notify")
						return nil
					},
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
				Tackler: func(stop driveStop, reportPath string) (loomengine.TacklerEnvelope, error) {
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
				Arbiter: func(envelopePath, reportPath, caseBriefPath string) (loomengine.ArbiterEnvelope, error) {
					calls = append(calls, "arbiter:"+caseBriefPath)
					if _, err := os.ReadFile(envelopePath); err != nil {
						t.Errorf("the arbiter was handed an unreadable stop envelope: %v", err)
					}
					return tt.arbiterReturns, tt.arbiterErr
				},
				Snapshot:        func() (loomengine.WorktreeSnapshot, error) { return tt.currentSnap, nil },
				AttemptSnapshot: func(string) (loomengine.WorktreeSnapshot, error) { return tt.attemptSnap, nil },
				OpenAttempts: func(key loomengine.ArbiterStopKey) (string, string, bool, error) {
					if tt.openKey == nil || key != *tt.openKey {
						return "", "", false, nil
					}
					return openTackler, tt.openArbiter, tt.openApplied, nil
				},
				OnDiskBuild:  func() (buildvcs.Identity, error) { return buildvcs.Identity{Revision: "newbuild1234567"}, nil },
				BuildChanged: func(buildvcs.Identity) bool { return tt.buildChanged },
				DirtyPaths:   func() ([]string, error) { return tt.dirty, nil },
				ReleaseLock: func() error {
					calls = append(calls, "release")
					order = append(order, "release")
					return nil
				},
				SpawnSuccessor: func() error {
					calls = append(calls, "spawn")
					order = append(order, "spawn")
					return tt.spawnErr
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
