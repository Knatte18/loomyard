//go:build tier0

package shedarbiter

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestConsumeAttempt covers the re-launch consumption over a temp status file and attempt directory with the real shedengine.Relaunch.
// Rows with a refusal assert its message and the absent applied mark; success rows assert the applied mark and where the run stands.
func TestConsumeAttempt(t *testing.T) {
	t.Parallel()

	const blocked = "Plan-Write"
	baseStatus := shedengine.Status{
		CurrentProducer: "Plan-Bouncer",
		State:           shedengine.StateBlocked,
		History:         []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Done}, {Producer: "Plan-Bouncer", Outcome: shedengine.Done}},
	}
	resumeEnvelope := loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictResume, Reason: "safe"}
	gotoPlanWriteEnvelope := loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictResume, Reason: "safe", Goto: blocked}
	// verdictInEffect leaves the run as the verdict's own goto to Plan-Write would.
	verdictInEffect := func(st *shedengine.Status) {
		st.History = append(st.History, shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto})
		st.CurrentProducer = "Plan-Write"
		st.State = shedengine.StatePaused
	}
	tests := []struct {
		name string
		// envelope is written as envelope.json unless nil; rawEnvelope overrides it with raw bytes.
		envelope    *loomengine.ArbiterEnvelope
		rawEnvelope string
		// verdictYAML is written as the agent's verdict.yaml when non-empty.
		verdictYAML string
		applied     bool
		// mutate changes the status file after the attempt recorded its stop key and pause conditions.
		mutate func(st *shedengine.Status)
		// spawn changes the attempt record before it is written.
		spawn func(a *loomengine.ArbiterAttempt)
		// guard is the base request's goto guard.
		guard       shedengine.GotoGuard
		wantErr     string
		wantProduce string
		// wantHistory is the history length asserted on a success row when non-zero.
		wantHistory int
		// pauseAfterCall requests a plain pause once the call returned and expects it to survive.
		pauseAfterCall bool
	}{
		{name: "missing envelope", wantErr: "unusable: the arbiter verb left no envelope"},
		{name: "unparseable envelope", rawEnvelope: "{", wantErr: "unusable"},
		{
			name:     "hand-on envelope",
			envelope: &loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictHandOn, Reason: "r", Question: "q"}, wantErr: "not a resume",
		},
		{
			name:        "verdict file says resume under a precondition hand-on envelope",
			envelope:    &loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictHandOn, Cause: sheddriver.CausePrecondition, Reason: "r", Question: "q"},
			verdictYAML: resumeVerdict, wantErr: "not a resume",
		},
		{name: "already applied", envelope: &resumeEnvelope, applied: true, wantErr: "already applied"},
		{
			name: "stop key moved by a goto", envelope: &resumeEnvelope,
			mutate: func(st *shedengine.Status) {
				st.History = append(st.History, shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto})
			},
			wantErr: "the run moved since the arbiter spawned",
		},
		{
			name: "a plain pause left by an older binary is carried through", envelope: &resumeEnvelope,
			mutate:      func(st *shedengine.Status) { st.PauseRequested = true },
			wantProduce: "Plan-Bouncer",
		},
		{
			name: "pause before a row", envelope: &resumeEnvelope,
			mutate:  func(st *shedengine.Status) { st.PauseBefore = "Plan-Bouncer" },
			wantErr: "paused since the arbiter spawned",
		},
		{
			name: "pause after a row", envelope: &resumeEnvelope,
			mutate:  func(st *shedengine.Status) { st.PauseAfter = "Plan-Write" },
			wantErr: "paused since the arbiter spawned",
		},
		{
			name: "a refusing guard refuses the goto", envelope: &gotoPlanWriteEnvelope,
			guard:   func(string) (string, error) { return "the Plan-Review verdict is settled", nil },
			wantErr: "the Plan-Review verdict is settled",
		},
		{name: "resume without a target", envelope: &resumeEnvelope, wantProduce: "Plan-Bouncer"},
		{
			name:     "resume with a backward target",
			envelope: &loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictResume, Reason: "safe", Goto: blocked}, wantProduce: blocked,
		},
		{
			name: "verdict already in effect is consumed with no second goto", envelope: &gotoPlanWriteEnvelope,
			mutate: verdictInEffect, wantProduce: "Plan-Write", wantHistory: 3,
		},
		{
			name: "a pause the goto cleared does not refuse the in-effect admit", envelope: &gotoPlanWriteEnvelope,
			spawn:  func(a *loomengine.ArbiterAttempt) { a.PauseBefore = "Plan-Bouncer" },
			mutate: verdictInEffect, wantProduce: "Plan-Write", wantHistory: 3,
		},
		{
			name: "a pause added since the spawn refuses the in-effect admit", envelope: &gotoPlanWriteEnvelope,
			mutate: func(st *shedengine.Status) {
				verdictInEffect(st)
				st.PauseAfter = "Plan-Write"
			},
			wantErr: "paused since the arbiter spawned",
		},
		{
			name: "an unmoved run whose pause was cleared by hand is refused", envelope: &resumeEnvelope,
			spawn:   func(a *loomengine.ArbiterAttempt) { a.PauseBefore = "Plan-Bouncer" },
			wantErr: "paused since the arbiter spawned",
		},
		{
			name: "moved by a goto to another row is refused", envelope: &gotoPlanWriteEnvelope,
			mutate: func(st *shedengine.Status) {
				st.History = append(st.History, shedengine.HistoryEntry{Producer: "Plan-Bouncer", Outcome: shedengine.OutcomeGoto})
				st.State = shedengine.StatePaused
			},
			wantErr: "the run moved",
		},
		{
			name: "moved by a goto and then stepped is refused", envelope: &gotoPlanWriteEnvelope,
			mutate: func(st *shedengine.Status) {
				st.History = append(st.History,
					shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto},
					shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.Stuck},
					shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto},
				)
				st.CurrentProducer = "Plan-Write"
				st.State = shedengine.StatePaused
			},
			wantErr: "the run moved",
		},
		{name: "a plain pause written after the call survives", envelope: &resumeEnvelope, wantProduce: "Plan-Bouncer", pauseAfterCall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			statusPath := filepath.Join(root, "status.json")
			statusLock := filepath.Join(root, "scratch", "status.lock")
			runLock := filepath.Join(root, "scratch", "run.lock")
			attemptDir := filepath.Join(root, "arbiter", "attempt-1")

			if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := state.WriteJSON(statusPath, statusLock, baseStatus); err != nil {
				t.Fatal(err)
			}
			attempt := loomengine.ArbiterAttempt{Number: 1, StopKey: sheddriver.StopKeyOf(baseStatus)}
			if tt.spawn != nil {
				tt.spawn(&attempt)
			}
			if err := loomengine.WriteArbiterAttempt(attemptDir, attempt); err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.rawEnvelope != "":
				if err := os.WriteFile(loomengine.ArbiterEnvelopePath(attemptDir), []byte(tt.rawEnvelope), 0o644); err != nil {
					t.Fatal(err)
				}
			case tt.envelope != nil:
				if err := loomengine.WriteArbiterEnvelope(loomengine.ArbiterEnvelopePath(attemptDir), *tt.envelope); err != nil {
					t.Fatal(err)
				}
			}
			if tt.verdictYAML != "" {
				writeVerdictFile(t, attemptDir, tt.verdictYAML)
			}
			if tt.applied {
				if err := loomengine.MarkArbiterApplied(attemptDir); err != nil {
					t.Fatal(err)
				}
			}
			if tt.mutate != nil {
				mutated := baseStatus
				tt.mutate(&mutated)
				if err := state.WriteJSON(statusPath, statusLock, mutated); err != nil {
					t.Fatal(err)
				}
			}
			producers := []shedengine.ProducerDef{{Name: "Plan-Write"}, {Name: "Plan-Bouncer"}, {Name: "Publish"}}
			request := shedengine.RelaunchRequest{StatusPath: statusPath, LockPath: runLock, StatusLockPath: statusLock, Producers: producers, RunID: "self", Guard: tt.guard}
			before, _, readErr := state.ReadJSON[shedengine.Status](statusPath, statusLock)
			if readErr != nil {
				t.Fatal(readErr)
			}

			err := ConsumeAttempt(attemptDir, request)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ConsumeAttempt() error = %v; want it to contain %q", err, tt.wantErr)
				}
				// A guard's refusal comes from Shed with the guard's own way forward, not the attempt's.
				if tt.guard == nil && !strings.Contains(err.Error(), loomengine.ArbiterVerdictPath(attemptDir)) {
					t.Errorf("ConsumeAttempt() error = %v; want it to name the verdict path", err)
				}
				if applied, _ := loomengine.ArbiterApplied(attemptDir); applied != tt.applied {
					t.Errorf("applied mark = %v after a refusal; want %v, as before the call", applied, tt.applied)
				}
				if after, _, _ := state.ReadJSON[shedengine.Status](statusPath, statusLock); !reflect.DeepEqual(after, before) {
					t.Errorf("status after a refusal = %+v; want it unchanged from %+v", after, before)
				}
				return
			}

			if err != nil {
				t.Fatalf("ConsumeAttempt() error = %v; want nil", err)
			}
			if applied, _ := loomengine.ArbiterApplied(attemptDir); !applied {
				t.Error("applied mark is absent after a consumed attempt; want it written")
			}
			if tt.pauseAfterCall {
				err := state.UpdateJSON(statusPath, statusLock, func(cur shedengine.Status, _ bool) (shedengine.Status, error) {
					cur.PauseRequested = true
					return cur, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			got, _, readErr := state.ReadJSON[shedengine.Status](statusPath, statusLock)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if got.CurrentProducer != tt.wantProduce {
				t.Errorf("current producer = %q; want %q", got.CurrentProducer, tt.wantProduce)
			}
			if tt.wantHistory != 0 && len(got.History) != tt.wantHistory {
				t.Errorf("history length = %d; want %d", len(got.History), tt.wantHistory)
			}
			if tt.pauseAfterCall && !got.PauseRequested {
				t.Error("pause_requested = false after a pause written following the call; want it set for the next step")
			}
		})
	}
}
