// tacklerconsume_test.go — untagged Tier-1 tests for consuming a tackler's resume verdict, over a real status file in a temp dir.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
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
	resumeEnvelope := loomengine.TacklerEnvelope{Verdict: loomengine.TacklerVerdictResume, Reason: "safe"}
	tests := []struct {
		name string
		// envelope is written as envelope.json unless nil.
		envelope *loomengine.TacklerEnvelope
		applied  bool
		// mutate changes the status file after the attempt recorded its stop key and pause conditions.
		mutate      func(st *shedengine.Status)
		wantErr     string
		wantProduce string
	}{
		{name: "missing envelope", wantErr: "unusable: the tackler call left no envelope"},
		{
			name:     "hand-back envelope",
			envelope: &loomengine.TacklerEnvelope{Verdict: loomengine.TacklerVerdictHandBack, Reason: "r", Question: "q"}, wantErr: "not a resume",
		},
		{
			name:     "resume envelope carrying a cause",
			envelope: &loomengine.TacklerEnvelope{Verdict: loomengine.TacklerVerdictResume, Cause: arbiterCausePrecondition}, wantErr: "not a resume",
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
		{name: "resume without a goto leaves the status as it was", envelope: &resumeEnvelope, wantProduce: "Plan-Bouncer"},
		{
			name:     "resume with a goto moves the run",
			envelope: &loomengine.TacklerEnvelope{Verdict: loomengine.TacklerVerdictResume, Reason: "safe", Goto: "Plan-Write"}, wantProduce: "Plan-Write",
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
			if err := loomengine.WriteTacklerAttempt(attemptDir, loomengine.TacklerAttempt{Number: 1, StopKey: loomengine.StopKeyOf(baseStatus)}); err != nil {
				t.Fatal(err)
			}
			if tt.envelope != nil {
				if err := loomengine.WriteTacklerEnvelope(loomengine.TacklerEnvelopePath(attemptDir), *tt.envelope); err != nil {
					t.Fatal(err)
				}
			}
			if tt.applied {
				if err := loomengine.MarkTacklerApplied(attemptDir); err != nil {
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

			err := consumeTacklerAttempt(attemptDir, shedengine.RelaunchRequest{StatusPath: statusPath, LockPath: runLock, StatusLockPath: statusLock, Producers: producers, RunID: "self"})

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), attemptDir) || !strings.Contains(err.Error(), "way forward:") {
					t.Fatalf("consume error = %v; want it to contain %q, the attempt directory and a way forward", err, tt.wantErr)
				}
				if applied, _ := loomengine.TacklerApplied(attemptDir); applied != tt.applied {
					t.Errorf("applied mark = %v after a refusal; want %v, as before the call", applied, tt.applied)
				}
				got, _, readErr := state.ReadJSON[shedengine.Status](statusPath, statusLock)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if got.CurrentProducer != "Plan-Bouncer" {
					t.Errorf("current producer = %q after a refusal; want the run unmoved", got.CurrentProducer)
				}
				return
			}

			if err != nil {
				t.Fatalf("consume error = %v; want nil", err)
			}
			if applied, _ := loomengine.TacklerApplied(attemptDir); !applied {
				t.Error("applied mark is absent after a consumed attempt; want it written")
			}
			got, _, readErr := state.ReadJSON[shedengine.Status](statusPath, statusLock)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if got.CurrentProducer != tt.wantProduce {
				t.Errorf("current producer = %q; want %q", got.CurrentProducer, tt.wantProduce)
			}
		})
	}
}
