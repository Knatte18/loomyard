// anomaly_test.go is the TDD driver for DetectCrashResume and ClassifyHalt: table tests over
// hand-built observations and hand-built states and reasons. It is untagged (Tier 1): no spawn,
// no git, no filesystem I/O -- the classifier is pure.

package loomengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestDetectCrashResume(t *testing.T) {
	tests := []struct {
		name  string
		entry EntryObservation
		want  bool
	}{
		{
			name: "RunningWithNonEmptyHistory_IsCrashResume",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateRunning,
				HistoryLength: 3,
			},
			want: true,
		},
		{
			name: "RunningWithEmptyHistory_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateRunning,
				HistoryLength: 0,
			},
			want: false,
		},
		{
			name: "Paused_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StatePaused,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "Blocked_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateBlocked,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "Failed_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateFailed,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "RunningWithHistory_LockHeld_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   true,
				State:         shedengine.StateRunning,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "NotObserved_None",
			entry: EntryObservation{
				Observed:      false,
				RunLockHeld:   false,
				State:         shedengine.StateRunning,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			// A completed `lyx loom step` leaves exactly the crash signature -- state running, no
			// lock held, live history -- so the clean-handoff flag is the one thing separating an
			// operator's step-to-driver handoff from a mid-run driver death.
			name: "RunningWithHistory_CleanStepHandoff_None",
			entry: EntryObservation{
				Observed:         true,
				RunLockHeld:      false,
				State:            shedengine.StateRunning,
				HistoryLength:    3,
				CleanStepHandoff: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectCrashResume(tt.entry); got != tt.want {
				t.Errorf("DetectCrashResume(%+v) = %v; want %v", tt.entry, got, tt.want)
			}
		})
	}
}

func TestClassifyHalt(t *testing.T) {
	tests := []struct {
		name     string
		state    shedengine.State
		reason   string
		wantKind AnomalyKind
		wantOK   bool
	}{
		{
			name:     "BlockedGenericFallback_Escalation",
			state:    shedengine.StateBlocked,
			reason:   shedengine.ReasonNoOnStuckTarget,
			wantKind: AnomalyEscalation,
			wantOK:   true,
		},
		{
			name:     "BlockedBudgetExhausted",
			state:    shedengine.StateBlocked,
			reason:   shedengine.ReasonBounceBudgetExhausted,
			wantKind: AnomalyBudgetExhausted,
			wantOK:   true,
		},
		{
			name:     "BlockedBudgetExhaustedWithWayForward",
			state:    shedengine.StateBlocked,
			reason:   shedengine.ReasonBounceBudgetExhausted + ` for Plan-Write; way forward: "lyx shed goto --to Plan-Write" gives row "Plan-Write" a fresh budget`,
			wantKind: AnomalyBudgetExhausted,
			wantOK:   true,
		},
		{
			name:     "BlockedProducerReason_Escalation",
			state:    shedengine.StateBlocked,
			reason:   "pull request review is still pending: https://github.com/o/r/pull/1",
			wantKind: AnomalyEscalation,
			wantOK:   true,
		},
		{
			name:     "FailedArbitraryError_ProducerHardFailure",
			state:    shedengine.StateFailed,
			reason:   "arbitrary failure text",
			wantKind: AnomalyProducerFailure,
			wantOK:   true,
		},
		{
			name:   "Running_None",
			state:  shedengine.StateRunning,
			reason: "",
		},
		{
			name:   "Paused_None",
			state:  shedengine.StatePaused,
			reason: "",
		},
		{
			name:   "Awaiting_None",
			state:  shedengine.StateAwaiting,
			reason: "waiting for PR review",
		},
		{
			name:   "Done_None",
			state:  shedengine.StateDone,
			reason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ok := ClassifyHalt(tt.state, tt.reason)
			if kind != tt.wantKind || ok != tt.wantOK {
				t.Errorf("ClassifyHalt(%q, %q) = (%q, %v); want (%q, %v)", tt.state, tt.reason, kind, ok, tt.wantKind, tt.wantOK)
			}
		})
	}
}
