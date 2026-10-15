// tacklerconsume_test.go — untagged Tier-1 tests for consuming a tackler's resume verdict, over a real status file in a temp dir.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestConsumeTacklerAttempt covers the consume path over a temp status file and attempt directory with the real shedengine.Relaunch.
// Refusal rows assert the message, which names the attempt directory, and the applied mark as it was before the call; success rows assert the mark and where the run stands.
func TestConsumeTacklerAttempt(t *testing.T) {
	t.Parallel()

	baseStatus := shedengine.Status{
		CurrentProducer: "Plan-Bouncer",
		State:           shedengine.StateBlocked,
		History:         []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Done}, {Producer: "Plan-Bouncer", Outcome: shedengine.Done}},
	}
	resumeEnvelope := sheddriver.TacklerEnvelope{Verdict: sheddriver.TacklerVerdictResume, Reason: "safe"}
	gotoPlanWriteEnvelope := sheddriver.TacklerEnvelope{Verdict: sheddriver.TacklerVerdictResume, Reason: "safe", Goto: "Plan-Write"}
	// verdictInEffect leaves the run as the verdict's own goto to Plan-Write would.
	verdictInEffect := func(st *shedengine.Status) {
		st.History = append(st.History, shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto})
		st.CurrentProducer = "Plan-Write"
		st.State = shedengine.StatePaused
	}
	tests := []struct {
		name string
		// envelope is written as envelope.json unless nil.
		envelope *sheddriver.TacklerEnvelope
		applied  bool
		// mutate changes the status file after the attempt recorded its stop key and pause conditions.
		mutate      func(st *shedengine.Status)
		wantErr     string
		wantProduce string
		// wantHistory is the history length asserted on a success row when non-zero.
		wantHistory int
	}{
		{name: "missing envelope", wantErr: "unusable: the tackler call left no envelope"},
		{
			name:     "hand-back envelope",
			envelope: &sheddriver.TacklerEnvelope{Verdict: sheddriver.TacklerVerdictHandBack, Reason: "r", Question: "q"}, wantErr: "not a resume",
		},
		{
			name:     "resume envelope carrying a cause",
			envelope: &sheddriver.TacklerEnvelope{Verdict: sheddriver.TacklerVerdictResume, Cause: arbiterCausePrecondition}, wantErr: "not a resume",
		},
		{name: "already applied", envelope: &resumeEnvelope, applied: true, wantErr: "already applied"},
		{
			name: "stop key moved since the spawn", envelope: &resumeEnvelope,
			mutate: func(st *shedengine.Status) {
				st.History = append(st.History, shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto})
			},
			wantErr: "the run moved since the tackler spawned",
		},
		{
			name: "pause condition added since the spawn", envelope: &resumeEnvelope,
			mutate:  func(st *shedengine.Status) { st.PauseBefore = "Plan-Bouncer" },
			wantErr: "paused since the tackler spawned",
		},
		{
			name: "verdict already in effect is consumed with no second goto", envelope: &gotoPlanWriteEnvelope,
			mutate: verdictInEffect, wantProduce: "Plan-Write", wantHistory: 3,
		},
		{
			name: "a pause added since the spawn refuses the in-effect admit", envelope: &gotoPlanWriteEnvelope,
			mutate: func(st *shedengine.Status) {
				verdictInEffect(st)
				st.PauseAfter = "Plan-Write"
			},
			wantErr: "paused since the tackler spawned",
		},
		{
			name: "moved by a goto to another row is refused", envelope: &gotoPlanWriteEnvelope,
			mutate: func(st *shedengine.Status) {
				st.History = append(st.History, shedengine.HistoryEntry{Producer: "Plan-Bouncer", Outcome: shedengine.OutcomeGoto})
				st.State = shedengine.StatePaused
			},
			wantErr: "the run moved since the tackler spawned",
		},
		{name: "resume without a goto leaves the status as it was", envelope: &resumeEnvelope, wantProduce: "Plan-Bouncer"},
		{
			name:     "resume with a goto moves the run",
			envelope: &sheddriver.TacklerEnvelope{Verdict: sheddriver.TacklerVerdictResume, Reason: "safe", Goto: "Plan-Write"}, wantProduce: "Plan-Write",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			statusPath := filepath.Join(root, "status.json")
			statusLock := filepath.Join(root, "scratch", "status.lock")
			runLock := filepath.Join(root, "scratch", "run.lock")
			attemptDir := filepath.Join(root, "tackler", "attempt-1")

			if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := state.WriteJSON(statusPath, statusLock, baseStatus); err != nil {
				t.Fatal(err)
			}
			if err := sheddriver.WriteTacklerAttempt(attemptDir, sheddriver.TacklerAttempt{Number: 1, StopKey: sheddriver.StopKeyOf(baseStatus)}); err != nil {
				t.Fatal(err)
			}
			if tt.envelope != nil {
				if err := sheddriver.WriteTacklerEnvelope(sheddriver.TacklerEnvelopePath(attemptDir), *tt.envelope); err != nil {
					t.Fatal(err)
				}
			}
			if tt.applied {
				if err := sheddriver.MarkTacklerApplied(attemptDir); err != nil {
					t.Fatal(err)
				}
			}
			written := baseStatus
			if tt.mutate != nil {
				tt.mutate(&written)
				if err := state.WriteJSON(statusPath, statusLock, written); err != nil {
					t.Fatal(err)
				}
			}
			producers := []shedengine.ProducerDef{{Name: "Plan-Write"}, {Name: "Plan-Bouncer"}, {Name: "Publish"}}

			err := consumeTacklerAttempt(attemptDir, shedengine.RelaunchRequest{StatusPath: statusPath, LockPath: runLock, StatusLockPath: statusLock, Producers: producers, RunID: "self"})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), attemptDir) || !strings.Contains(err.Error(), "way forward:") {
					t.Fatalf("consume error = %v; want it to contain %q, the attempt directory and a way forward", err, tt.wantErr)
				}
				if applied, _ := sheddriver.TacklerApplied(attemptDir); applied != tt.applied {
					t.Errorf("applied mark = %v after a refusal; want %v, as before the call", applied, tt.applied)
				}
				got, _, readErr := state.ReadJSON[shedengine.Status](statusPath, statusLock)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if got.CurrentProducer != written.CurrentProducer {
					t.Errorf("current producer = %q after a refusal; want the run unmoved at %q", got.CurrentProducer, written.CurrentProducer)
				}
				return
			}

			if err != nil {
				t.Fatalf("consume error = %v; want nil", err)
			}
			if applied, _ := sheddriver.TacklerApplied(attemptDir); !applied {
				t.Error("applied mark is absent after a consumed attempt; want it written")
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
		})
	}
}
