package loomcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestConsumeArbiterAttempt covers the re-launch hook over a temp status file and attempt directory with the real shedengine.Relaunch.
// Rows with a refusal assert its message and the absent applied mark; success rows assert the applied mark and where the run stands; the numeric rows drive the hook's own parse of the attempt number.
func TestConsumeArbiterAttempt(t *testing.T) {
	t.Parallel()

	const blocked = "Plan-Write"
	baseStatus := shedengine.Status{
		CurrentProducer: "Plan-Bouncer",
		State:           shedengine.StateBlocked,
		History:         []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Done}, {Producer: "Plan-Bouncer", Outcome: shedengine.Done}},
	}
	resumeEnvelope := loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictResume, Reason: "safe"}
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
		// attemptArg drives loomConsumeAttempt with this argument instead of consumeArbiterAttempt.
		attemptArg  string
		wantErr     string
		wantProduce string
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
			envelope:    &loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictHandOn, Cause: arbiterCausePrecondition, Reason: "r", Question: "q"},
			verdictYAML: arbiterResumeVerdict, wantErr: "not a resume",
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
			name: "plain pause", envelope: &resumeEnvelope,
			mutate:  func(st *shedengine.Status) { st.PauseRequested = true },
			wantErr: "paused since the arbiter spawned",
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
		{name: "resume without a target", envelope: &resumeEnvelope, wantProduce: "Plan-Bouncer"},
		{
			name:     "resume with a backward target",
			envelope: &loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictResume, Reason: "safe", Goto: blocked}, wantProduce: blocked,
		},
		{name: "a plain pause written after the call survives", envelope: &resumeEnvelope, wantProduce: "Plan-Bouncer", pauseAfterCall: true},
		{name: "attempt number 0", attemptArg: "0", wantErr: "not a positive integer"},
		{name: "attempt number -1", attemptArg: "-1", wantErr: "not a positive integer"},
		{name: "attempt number x", attemptArg: "x", wantErr: "not a positive integer"},
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
			attempt := loomengine.ArbiterAttempt{Number: 1, StopKey: loomengine.StopKeyOf(baseStatus)}
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
				writeArbiterVerdictFile(t, attemptDir, tt.verdictYAML)
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
			request := shedengine.RelaunchRequest{StatusPath: statusPath, LockPath: runLock, StatusLockPath: statusLock, Producers: producers, RunID: "self"}

			var err error
			if tt.attemptArg != "" {
				c := &loomCLI{
					location:  &lyxcwd.Location{HubPath: root, WorktreeName: "pair", AnchorRel: "."},
					runID:     shedrun.SelfRunID,
					shedPaths: shedbuild.ShedPaths{StatusPath: statusPath, LockPath: runLock, StatusLockPath: statusLock},
					routing:   shedengine.Routing{Producers: producers},
				}
				err = c.loomConsumeAttempt(context.Background(), tt.attemptArg)
			} else {
				err = consumeArbiterAttempt(attemptDir, request)
			}

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("consume error = %v; want it to contain %q", err, tt.wantErr)
				}
				if tt.attemptArg == "" && !strings.Contains(err.Error(), loomengine.ArbiterVerdictPath(attemptDir)) {
					t.Errorf("consume error = %v; want it to name the verdict path", err)
				}
				if applied, _ := loomengine.ArbiterApplied(attemptDir); applied != tt.applied {
					t.Errorf("applied mark = %v after a refusal; want %v, as before the call", applied, tt.applied)
				}
				return
			}

			if err != nil {
				t.Fatalf("consume error = %v; want nil", err)
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
			if tt.pauseAfterCall && !got.PauseRequested {
				t.Error("pause_requested = false after a pause written following the call; want it set for the next step")
			}
		})
	}
}
