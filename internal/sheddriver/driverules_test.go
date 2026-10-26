//go:build tier0

// driverules_test.go — tier 0 tests for the Go driver's rule table and its stop parse.

package sheddriver

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// TestDecideDrive pins every row of the rule table, in order, with one row per stop class and the neighbours that decide between adjacent rows.
func TestDecideDrive(t *testing.T) {
	t.Parallel()

	blockedStop := Stop{OK: true, State: "blocked", LoopState: "blocked", CurrentProducer: "Plan-Write", HistoryLength: 3}
	errorStop := func(kind, transient string) Stop {
		return Stop{State: "failed", LoopState: "failed", Kind: kind, Transient: transient, CurrentProducer: "Plan-Write", HistoryLength: 3}
	}
	interruptedStop := func(policy string) Stop {
		stop := errorStop(shedverbs.KindInterrupted, "")
		stop.InterruptPolicy = policy
		return stop
	}
	awaitingStop := func(notice string) Stop {
		return Stop{OK: true, State: "awaiting", LoopState: "awaiting", ParentNotice: notice}
	}
	budgetHaltReason := shedengine.ReasonBounceBudgetExhausted + " for Plan-Write; way forward: lyx shed resume"
	budgetHaltStop := Stop{OK: true, State: "blocked", LoopState: "blocked", Reason: budgetHaltReason, CurrentProducer: "Plan-Write", HistoryLength: 3}
	preconditionHaltReason := shedengine.ReasonPreconditionUnmet + ": the source worktree has uncommitted tracked changes; way forward: commit or revert them"
	preconditionHaltStop := Stop{OK: true, State: "blocked", LoopState: "blocked", Reason: preconditionHaltReason, CurrentProducer: "Worktree-Create", HistoryLength: 3}
	landingHaltStop := Stop{OK: true, State: "blocked", LoopState: "blocked", CurrentProducer: "Finalize", HistoryLength: 3, HandBack: true}
	restepRecord := func(key StopKey) SettleRecord {
		return SettleRecord{StopKey: key, Action: shedrun.SettleRestep}
	}

	tests := []struct {
		name  string
		stop  Stop
		facts driveFacts
		// records, when set, derive the facts' RepairCount through repairCount.
		records          []SettleRecord
		want             driveAction
		wantFrictionNote bool
	}{
		{name: "done", stop: Stop{OK: true, State: "done", LoopState: "done"}, want: driveActionDone},
		{name: "busy kind", stop: errorStop(shedverbs.KindBusy, ""), want: driveActionBusyExit},
		{name: "ownership kind", stop: errorStop(shedverbs.KindOwnership, ""), want: driveActionHandBack},
		{name: "error with no kind", stop: errorStop("", ""), want: driveActionHandBack},
		{name: "step paused with an empty reason carries a friction note", stop: Stop{OK: true, State: "paused", LoopState: "paused"}, want: driveActionHandBack, wantFrictionNote: true},
		{name: "step paused with a reason carries none", stop: Stop{OK: true, State: "paused", LoopState: "paused", Reason: "pause requested"}, want: driveActionHandBack},
		{name: "status-derived paused stop with a pause reason carries none", stop: stopFromStatus(shedengine.Status{State: shedengine.StatePaused, PauseReason: "pause requested by operator"}, nil), want: driveActionHandBack},
		{name: "status-derived paused stop without a pause reason carries a friction note", stop: stopFromStatus(shedengine.Status{State: shedengine.StatePaused}, nil), want: driveActionHandBack, wantFrictionNote: true},
		{name: "marked landing halt hands back", stop: landingHaltStop, want: driveActionHandBack},
		{name: "marked landing halt on a changed build with an escalation pending still hands back", stop: landingHaltStop, facts: driveFacts{BuildChanged: true, EscalationPending: true}, want: driveActionHandBack},
		{name: "unmarked reroute-cap halt goes to the tackler", stop: Stop{OK: true, State: "blocked", LoopState: "blocked", Reason: shedengine.ReasonRerouteCapReached + " for Webster", CurrentProducer: "Webster", HistoryLength: 3}, want: driveActionTackler},
		{name: "budget halt from the envelope hands back", stop: budgetHaltStop, want: driveActionHandBack},
		{name: "budget halt from the status hands back", stop: stopFromStatus(shedengine.Status{State: shedengine.StateBlocked, Error: budgetHaltReason}, nil), want: driveActionHandBack},
		{name: "budget halt on a changed build still hands back", stop: budgetHaltStop, facts: driveFacts{BuildChanged: true}, want: driveActionHandBack},
		{name: "budget halt after a tackler resume returned still hands back", stop: budgetHaltStop, facts: driveFacts{ResumeReturned: true}, want: driveActionHandBack},
		{name: "precondition halt from the envelope hands back", stop: preconditionHaltStop, want: driveActionHandBack},
		{name: "precondition halt from the status hands back", stop: stopFromStatus(shedengine.Status{State: shedengine.StateBlocked, Error: preconditionHaltReason}, nil), want: driveActionHandBack},
		{name: "precondition halt on a changed build still hands back", stop: preconditionHaltStop, facts: driveFacts{BuildChanged: true}, want: driveActionHandBack},
		{name: "blocked stop containing the precondition prefix past its start goes to the tackler", stop: Stop{OK: true, State: "blocked", LoopState: "blocked", Reason: "halted: " + preconditionHaltReason, CurrentProducer: "Plan-Write", HistoryLength: 3}, want: driveActionTackler},
		{name: "awaiting with a pending escalation", stop: awaitingStop("a notice"), facts: driveFacts{EscalationPending: true}, want: driveActionArbiter},
		{name: "awaiting with a parent notice", stop: awaitingStop("a notice"), want: driveActionRelayHandBack},
		{name: "awaiting with nothing to relay", stop: awaitingStop(""), want: driveActionHandBack},
		{name: "blocked stop goes to the tackler", stop: blockedStop, want: driveActionTackler},
		{name: "binary-eligible stop on a changed build hands over", stop: blockedStop, facts: driveFacts{BuildChanged: true}, want: driveActionHandOver},
		{name: "producer error on a changed build hands over", stop: errorStop(shedverbs.KindProducer, ""), facts: driveFacts{BuildChanged: true}, want: driveActionHandOver},
		{name: "hand-over already made for this build goes on to the next row", stop: blockedStop, facts: driveFacts{BuildChanged: true, HandedOver: true}, want: driveActionTackler},
		{name: "changed build at a stop no build could change", stop: awaitingStop(""), facts: driveFacts{BuildChanged: true}, want: driveActionHandBack},
		{name: "tackler resume returned to the same stop goes to the arbiter", stop: blockedStop, facts: driveFacts{ResumeReturned: true}, want: driveActionArbiter},
		{name: "blocked stop at a row the arbiter judges first goes to the arbiter", stop: blockedStop, facts: driveFacts{ArbiterFirst: true}, want: driveActionArbiter},
		{name: "blocked stop the arbiter judges first on a changed build hands over", stop: blockedStop, facts: driveFacts{ArbiterFirst: true, BuildChanged: true}, want: driveActionHandOver},
		{name: "landing halt at a row the arbiter judges first still hands back", stop: landingHaltStop, facts: driveFacts{ArbiterFirst: true}, want: driveActionHandBack},
		{name: "budget halt at a row the arbiter judges first still hands back", stop: budgetHaltStop, facts: driveFacts{ArbiterFirst: true}, want: driveActionHandBack},
		{name: "precondition halt at a row the arbiter judges first still hands back", stop: preconditionHaltStop, facts: driveFacts{ArbiterFirst: true}, want: driveActionHandBack},
		{name: "transient error goes to the tackler", stop: errorStop(shedverbs.KindProducer, "network"), want: driveActionTackler},
		{name: "interrupted under reinvoke re-steps", stop: interruptedStop(shedverbs.InterruptPolicyReinvoke), want: driveActionRestep},
		{name: "second reinvoke failure re-steps", stop: interruptedStop(shedverbs.InterruptPolicyReinvoke), facts: driveFacts{RepairCount: 1}, want: driveActionRestep},
		{name: "third reinvoke failure goes to the tackler", stop: interruptedStop(shedverbs.InterruptPolicyReinvoke), facts: driveFacts{RepairCount: driveRepairCap}, want: driveActionTackler},
		{
			name: "repair count is the restep records of the stop key only", stop: interruptedStop(shedverbs.InterruptPolicyReinvoke),
			records: []SettleRecord{
				restepRecord(interruptedStop("").Key()),
				restepRecord(interruptedStop("").Key()),
				restepRecord(StopKey{CurrentProducer: "Other", HistoryLength: 1, State: "failed"}),
				{StopKey: interruptedStop("").Key(), Action: shedrun.SettleHandBack},
			},
			want: driveActionTackler,
		},
		{name: "interrupted under handback hands back", stop: interruptedStop(shedverbs.InterruptPolicyHandback), want: driveActionHandBack},
		{name: "interrupted with an empty policy hands back", stop: interruptedStop(""), want: driveActionHandBack},
		{name: "producer error goes to the tackler", stop: errorStop(shedverbs.KindProducer, ""), want: driveActionTackler},
		{name: "bootstrap error goes to the tackler", stop: errorStop(shedverbs.KindBootstrap, ""), want: driveActionTackler},
		{name: "unseeded error goes to the tackler", stop: errorStop(shedverbs.KindUnseeded, ""), want: driveActionTackler},
		{name: "a stop no row names hands back", stop: Stop{OK: true, State: "running", LoopState: "running"}, want: driveActionHandBack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			facts := tt.facts
			if tt.records != nil {
				facts.RepairCount = repairCount(tt.records, tt.stop.Key())
			}

			got := decideDrive(tt.stop, facts)

			if got.Action != tt.want || got.FrictionNote != tt.wantFrictionNote {
				t.Errorf("decideDrive() = action %q, friction note %v; want %q, %v", got.Action, got.FrictionNote, tt.want, tt.wantFrictionNote)
			}
			if got.Rule == "" {
				t.Error("decideDrive() carries no rule text for the stop report")
			}
		})
	}
}

// TestParseDriveStop covers the loop envelope's decode: a halted success, an error carrying kind and transient, an interrupted loop object, and bytes that do not decode.
// Each decoded row also pins the stop key, which falls back to the top-level state when the loop object names none.
func TestParseDriveStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		envelope string
		want     Stop
		wantKey  StopKey
		wantErr  bool
	}{
		{
			name:     "halted success envelope",
			envelope: `{"ok":true,"state":"awaiting","reason":"review","parent_notice":"look","friction":"ok","envelope_path":"/e.json","loop":{"stop":"halted","current_producer":"Plan-Bouncer","history_length":4,"state":"awaiting"}}`,
			want: Stop{
				OK: true, State: "awaiting", Reason: "review", ParentNotice: "look", Friction: "ok", EnvelopePath: "/e.json",
				Stop: "halted", CurrentProducer: "Plan-Bouncer", HistoryLength: 4, LoopState: "awaiting",
			},
			wantKey: StopKey{CurrentProducer: "Plan-Bouncer", HistoryLength: 4, State: "awaiting"},
		},
		{
			name:     "error envelope carrying kind and transient",
			envelope: `{"ok":false,"error":"boom","kind":"producer","transient":"network","loop":{"stop":"error","current_producer":"Plan-Write","history_length":2,"state":"failed"}}`,
			want: Stop{
				Error: "boom", Kind: "producer", Transient: "network",
				Stop: "error", CurrentProducer: "Plan-Write", HistoryLength: 2, LoopState: "failed",
			},
			wantKey: StopKey{CurrentProducer: "Plan-Write", HistoryLength: 2, State: "failed"},
		},
		{
			name:     "interrupted loop object with its trace paths",
			envelope: `{"ok":false,"error":"interrupted","kind":"interrupted","state":"failed","loop":{"stop":"interrupted","current_producer":"Plan-Write","history_length":2,"interrupt_policy":"reinvoke","status_moved":true,"trace_copy":"/t.log","stderr_path":"/s.log"}}`,
			want: Stop{
				State: "failed", Error: "interrupted", Kind: "interrupted",
				Stop: "interrupted", CurrentProducer: "Plan-Write", HistoryLength: 2, InterruptPolicy: "reinvoke", StatusMoved: true, TraceCopy: "/t.log", StderrPath: "/s.log",
			},
			wantKey: StopKey{CurrentProducer: "Plan-Write", HistoryLength: 2, State: "failed"},
		},
		{
			name:     "marked landing halt carries the hand_back key",
			envelope: `{"ok":true,"state":"blocked","loop":{"stop":"halted","current_producer":"Finalize","history_length":5,"state":"blocked","hand_back":true}}`,
			want: Stop{
				OK: true, State: "blocked",
				Stop: "halted", CurrentProducer: "Finalize", HistoryLength: 5, LoopState: "blocked", HandBack: true,
			},
			wantKey: StopKey{CurrentProducer: "Finalize", HistoryLength: 5, State: "blocked"},
		},
		{
			name:     "yielded step carries the outcome",
			envelope: `{"ok":true,"outcome":"yield","state":"running","loop":{"stop":"halted","current_producer":"Run-Shed","history_length":3,"state":"running"}}`,
			want: Stop{
				OK: true, Outcome: "yield", State: "running",
				Stop: "halted", CurrentProducer: "Run-Shed", HistoryLength: 3, LoopState: "running",
			},
			wantKey: StopKey{CurrentProducer: "Run-Shed", HistoryLength: 3, State: "running"},
		},
		{name: "undecodable bytes are refused", envelope: `{"ok":`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDriveStop([]byte(tt.envelope))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDriveStop(%q) error = nil; want a refusal", tt.envelope)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDriveStop() error = %v; want nil", err)
			}
			if string(got.Raw) != tt.envelope || got.FromStatus {
				t.Errorf("parseDriveStop() raw = %q, from status = %v; want the envelope verbatim and false", got.Raw, got.FromStatus)
			}
			got.Raw = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDriveStop() = %+v; want %+v", got, tt.want)
			}
			if got.Key() != tt.wantKey {
				t.Errorf("key() = %+v; want %+v", got.Key(), tt.wantKey)
			}
		})
	}
}
