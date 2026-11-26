// loop_arming_integration_test.go runs the loop's arming refusal over a real hub.
// It drives a lym built by internal/testkit/lymbin over a loom-seeded run in a real hub, from the external test package because internal/hubforge reaches back into this package.

package shedverbs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/spf13/cobra"
)

// loomRunFixture is a real hub with one pair whose tapestry run is seeded and reads running.
// Its config is reconciled for the built lym and its hub-wide landing config is then gone.
type loomRunFixture struct {
	t        *testing.T
	loc      *lymcwd.Location
	worktree string
	lym      string
}

func newLoomRunFixture(t *testing.T) *loomRunFixture {
	t.Helper()
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "loop-task")
	worktree := h.PairCodeWorktree("loop-task")
	loc, err := lymcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", worktree, err)
	}
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin: found=%v err=%v", found, err)
	}
	seed := shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverGo, Params: map[string]string{"parent": recorded.ParentBranch}}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed: %v", err)
	}
	f := &loomRunFixture{t: t, loc: loc, worktree: worktree, lym: lymbin.Build(t)}
	f.resetRunning()
	// A verb that loads the loom config first reconciles a hub whose build stamp is stale.
	// That reconcile reseeds a missing config file.
	// The step would then arm and start a real agent, so the hub is stamped for this binary before the file goes.
	// The pushes are skipped because a detached push would outlive the test.
	skipPush := []string{"FABRIC_SKIP_PUSH=1", "BOARD_SKIP_PUSH=1"}
	if out, code, err := lymbin.RunWithEnv(f.lym, worktree, skipPush, time.Minute, "loom", "status"); err != nil || code != 0 {
		t.Fatalf("lym loom status = exit %d, %v; want the hub reconciled: %s", code, err, out)
	}
	landing := configengine.ConfigFile(h.BoardDir(), "landing")
	if err := os.Remove(landing); err != nil {
		t.Fatalf("remove the hub landing config: %v", err)
	}
	return f
}

// killStepChild kills the process tree of the step child the loop's pid record names, if any.
func (f *loomRunFixture) killStepChild() {
	record, found, err := shedverbs.ReadLoopPIDRecord(shedrun.LoopPIDFile(f.loc, shedrun.SelfRunID))
	if err == nil && found && record.Child.PID != 0 {
		_, _ = proc.KillTree(record.Child)
	}
}

// stuckStepEvidence renders the newest captured step stderr and the run's status file.
func (f *loomRunFixture) stuckStepEvidence() string {
	var newest string
	var newestAt time.Time
	paths, _ := filepath.Glob(shedrun.StepStderr(f.loc, shedrun.SelfRunID, "*"))
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(newestAt) {
			newest, newestAt = path, info.ModTime()
		}
	}
	stderr, _ := os.ReadFile(newest)
	status, err := os.ReadFile(shedrun.StatusFile(f.loc, shedrun.SelfRunID))
	return fmt.Sprintf("step stderr %q:\n%s\nstatus file (read error %v):\n%s", newest, stderr, err, status)
}

// runBuilt runs the built binary in the pair's worktree and returns its standard output and exit code.
// A run past the step bound is killed with its step child and fails t.
func (f *loomRunFixture) runBuilt(t *testing.T, args ...string) ([]byte, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), shedverbs.LoopStepBoundForTest)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.lym, args...)
	cmd.Dir = f.worktree
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Cancel = func() error {
		f.killStepChild()
		return cmd.Process.Kill()
	}
	out, err := cmd.Output()
	if ctx.Err() != nil {
		t.Fatalf("lym %v did not end within %s; its stderr:\n%s\n%s", args, shedverbs.LoopStepBoundForTest, stderr.String(), f.stuckStepEvidence())
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out, exitErr.ExitCode()
	}
	if err != nil {
		t.Fatalf("lym %v: %v", args, err)
	}
	return out, 0
}

// resetRunning writes the run's status file as a run no step has finished.
func (f *loomRunFixture) resetRunning() {
	f.t.Helper()
	status := shedengine.Status{CurrentProducer: "Discussion-Write", State: shedengine.StateRunning, History: []shedengine.HistoryEntry{}}
	lockPath := shedrun.StatusLock(f.loc, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		f.t.Fatalf("create the status lock directory: %v", err)
	}
	if err := state.WriteJSON(shedrun.StatusFile(f.loc, shedrun.SelfRunID), lockPath, status); err != nil {
		f.t.Fatalf("write the running status file: %v", err)
	}
}

func (f *loomRunFixture) status() shedengine.Status {
	f.t.Helper()
	st, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(f.loc, shedrun.SelfRunID), shedrun.StatusLock(f.loc, shedrun.SelfRunID))
	if err != nil || !found {
		f.t.Fatalf("read the status file = %v, found %v; want it", err, found)
	}
	return st
}

// runLym runs the built binary in the pair's worktree and decodes the last line it printed as the envelope.
func (f *loomRunFixture) runLym(t *testing.T, args ...string) (map[string]any, int) {
	t.Helper()
	out, code := f.runBuilt(t, args...)
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var env map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &env); err != nil {
		t.Fatalf("decode the envelope of lym %v from %q: %v", args, out, err)
	}
	return env, code
}

// childLoopSpec arms a loop whose children are real `lym shed step` processes over the fixture's run.
func (f *loomRunFixture) childLoopSpec() *shedverbs.Spec {
	loc, run := f.loc, shedrun.SelfRunID
	return &shedverbs.Spec{
		StatusPath:     shedrun.StatusFile(loc, run),
		LockPath:       shedrun.RunLock(loc, run),
		StatusLockPath: shedrun.StatusLock(loc, run),
		RunID:          shedrun.ResolveRunID(loc, run),
		StepsDir:       shedrun.StepsDir(loc, run),
		Loop: shedverbs.LoopSpec{
			LockPath:     shedrun.LoopLock(loc, run),
			PIDPath:      shedrun.LoopPIDFile(loc, run),
			LogPath:      shedrun.LoopLog(loc, run),
			EnvelopePath: shedrun.LoopEnvelope(loc, run),
			JobName:      shedrun.LoopJobName(loc, run),
			Delivered:    func(loopID string) string { return shedrun.LoopDelivered(loc, run, loopID) },
			StopFiles: func(traceID string) (string, string) {
				return shedrun.StepTraceCopy(loc, run, traceID), shedrun.StepStderr(loc, run, traceID)
			},
			TraceFiles: func(traceID string) ([]string, error) { return logger.TraceFilesFor(logger.TraceDir(), traceID) },
			Executable: f.lym,
		},
	}
}

func TestLoopIntegration_ArmingRefusalStopsAsBootstrap(t *testing.T) {
	tiergate.Require(t, 1)
	f := newLoomRunFixture(t)
	var childLoopEnv map[string]any
	// runKeeperRefusal runs the built binary as the loop's keeper of loopID over the verb in verbArgs.
	// The keeper's arming refuses, and it prints nothing.
	// The refusal is written as the loop envelope under the loop id, for the waiter that spawned the keeper to print.
	// It returns that envelope and the keeper's exit code.
	// It removes the envelope file so the next row meets no undelivered envelope.
	runKeeperRefusal := func(t *testing.T, loopID string, verbArgs ...string) (map[string]any, int) {
		t.Helper()
		args := append(verbArgs, "--"+shedverbs.UntilStopFlag, "--"+shedverbs.LoopKeeperFlag+"="+loopID)
		out, code := f.runBuilt(t, args...)
		if len(bytes.TrimSpace(out)) != 0 {
			t.Errorf("the keeper over lym %v printed %q; want nothing, the refusal going to the loop envelope file", args, out)
		}
		path := shedrun.LoopEnvelope(f.loc, shedrun.SelfRunID)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the loop envelope file: %v; want the keeper's refusal in it", err)
		}
		_ = os.Remove(path)
		var file struct {
			LoopID   string         `json:"loop_id"`
			Envelope map[string]any `json:"envelope"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			t.Fatalf("decode the loop envelope file %q: %v", data, err)
		}
		if file.LoopID != loopID {
			t.Errorf("loop envelope file loop id = %q; want the keeper's, %q", file.LoopID, loopID)
		}
		return file.Envelope, code
	}
	tests := []struct {
		name string
		run  func(t *testing.T) (map[string]any, int)
	}{
		{
			name: "a child step refuses while arming",
			run: func(sub *testing.T) (map[string]any, int) {
				// The parent changes directory, so the trace file check after the rows still runs in the worktree.
				t.Chdir(f.worktree)
				root := &cobra.Command{Use: "lym"}
				shed := &cobra.Command{Use: "shed"}
				shed.AddCommand(shedverbs.StepCmdForTest(f.childLoopSpec()))
				root.AddCommand(shed)
				var out bytes.Buffer
				// The loop runs in this process, so the bound kills its step child, which ends the loop.
				bound := time.AfterFunc(shedverbs.LoopStepBoundForTest, f.killStepChild)
				code := clihelp.Execute(root, &out, []string{"shed", "step", shedrun.SelfRunID, "--" + shedverbs.UntilStopFlag, "--" + shedverbs.LoopDetachedFlag + "=loop-int-1"})
				if !bound.Stop() {
					sub.Fatalf("the step child did not end within %s; the loop printed %q\n%s", shedverbs.LoopStepBoundForTest, out.String(), f.stuckStepEvidence())
				}
				var env map[string]any
				if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &env); err != nil {
					sub.Fatalf("decode the loop's envelope from %q: %v", out.String(), err)
				}
				childLoopEnv = env
				return env, code
			},
		},
		{
			name: "the loop's keeper refuses while arming",
			run: func(t *testing.T) (map[string]any, int) {
				return runKeeperRefusal(t, "loop-int-2", "shed", "step", shedrun.SelfRunID)
			},
		},
		{
			name: "the loom surface's keeper refuses while arming",
			run:  func(t *testing.T) (map[string]any, int) { return runKeeperRefusal(t, "loop-int-3", "loom", "step") },
		},
		{
			name: "the driver's own invocation of shed step",
			run: func(t *testing.T) (map[string]any, int) {
				return f.runLym(t, "shed", "step", shedrun.SelfRunID, "--"+shedverbs.UntilStopFlag)
			},
		},
		{
			name: "the loom surface",
			run: func(t *testing.T) (map[string]any, int) {
				return f.runLym(t, "loom", "step", "--"+shedverbs.UntilStopFlag)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.resetRunning()

			env, code := tt.run(t)

			loop := shedverbs.LoopOfForTest(t, env)
			shedverbs.AssertLoopKeysForTest(t, loop, false)
			if code != 1 || env["kind"] != shedverbs.KindBootstrap || loop["stop"] != string(shedverbs.LoopStopError) {
				t.Errorf("exit/kind/stop = %d/%v/%v; want 1/%s/%s", code, env["kind"], loop["stop"], shedverbs.KindBootstrap, shedverbs.LoopStopError)
			}
			if !strings.Contains(env["error"].(string), "lym config reconcile --apply") {
				t.Errorf("error = %q; want the hub-config way forward", env["error"])
			}
			if st := f.status(); st.State != shedengine.StateFailed {
				t.Errorf("status state = %s; want failed", st.State)
			}
		})
	}

	t.Run("the child's captured stderr and trace carry its own id", func(t *testing.T) {
		loop := shedverbs.LoopOfForTest(t, childLoopEnv)
		stderrPath, _ := loop["stderr_path"].(string)
		childID := strings.TrimSuffix(filepath.Base(stderrPath), ".stderr.log")
		traceFile, _ := childLoopEnv["trace_file"].(string)
		if _, err := os.Stat(stderrPath); err != nil {
			t.Errorf("captured stderr file %q: %v; want it on disk", stderrPath, err)
		}
		if childID == "" || childID == logger.TraceID() || !strings.Contains(traceFile, childID) {
			t.Errorf("child id %q, loop id %q, trace file %q; want a trace file named by the child's own id", childID, logger.TraceID(), traceFile)
		}
		data, err := os.ReadFile(traceFile)
		if err != nil || !strings.Contains(string(data), "lym config reconcile --apply") {
			t.Errorf("trace file %q = %q, %v; want it holding the refusal", traceFile, data, err)
		}
	})
}
