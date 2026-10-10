//go:build integration

// loop_arming_integration_test.go runs the loop's arming refusal over a real hub.
// It drives a lyx built by internal/testkit/lyxbin over a loom-seeded run in a real hub, from the external test package because internal/hubforge reaches back into this package.

package shedverbs_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/spf13/cobra"
)

// loomRunFixture is a real hub with one pair whose loom run is seeded and reads running, and whose hub-wide landing config is gone.
type loomRunFixture struct {
	t        *testing.T
	loc      *lyxcwd.Location
	worktree string
	lyx      string
}

func newLoomRunFixture(t *testing.T) *loomRunFixture {
	t.Helper()
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "loop-task")
	worktree := h.PairCodeWorktree("loop-task")
	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin: found=%v err=%v", found, err)
	}
	seed := shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo, Params: map[string]string{"parent": recorded.ParentBranch}}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed: %v", err)
	}
	landing := configengine.ConfigFile(h.BoardDir(), "landing")
	if err := os.Remove(landing); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove the hub landing config: %v", err)
	}
	f := &loomRunFixture{t: t, loc: loc, worktree: worktree, lyx: lyxbin.Build(t)}
	f.resetRunning()
	return f
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
	st, found, err := state.ReadJSONStrict[shedengine.Status](shedrun.StatusFile(f.loc, shedrun.SelfRunID), shedrun.StatusLock(f.loc, shedrun.SelfRunID))
	if err != nil || !found {
		f.t.Fatalf("read the status file = %v, found %v; want it", err, found)
	}
	return st
}

// runLyx runs the built binary in the pair's worktree and decodes the last line it printed as the envelope.
func (f *loomRunFixture) runLyx(args ...string) (map[string]any, int) {
	f.t.Helper()
	cmd := exec.Command(f.lyx, args...)
	cmd.Dir = f.worktree
	out, err := cmd.Output()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		f.t.Fatalf("lyx %v: %v", args, err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var env map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &env); err != nil {
		f.t.Fatalf("decode the envelope of lyx %v from %q: %v", args, out, err)
	}
	return env, code
}

// childLoopSpec arms a loop whose children are real `lyx shed step` processes over the fixture's run.
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
			Executable: f.lyx,
		},
	}
}

func TestLoopIntegration_ArmingRefusalStopsAsBootstrap(t *testing.T) {
	f := newLoomRunFixture(t)
	var childLoopEnv map[string]any
	tests := []struct {
		name string
		run  func() (map[string]any, int)
	}{
		{
			name: "a child step refuses while arming",
			run: func() (map[string]any, int) {
				t.Chdir(f.worktree)
				root := &cobra.Command{Use: "lyx"}
				shed := &cobra.Command{Use: "shed"}
				shed.AddCommand(shedverbs.StepCmdForTest(f.childLoopSpec()))
				root.AddCommand(shed)
				var out bytes.Buffer
				code := clihelp.Execute(root, &out, []string{"shed", "step", shedrun.SelfRunID, "--" + shedverbs.UntilStopFlag, "--" + shedverbs.LoopDetachedFlag + "=loop-int-1"})
				var env map[string]any
				if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &env); err != nil {
					t.Fatalf("decode the loop's envelope from %q: %v", out.String(), err)
				}
				childLoopEnv = env
				return env, code
			},
		},
		{
			name: "the driver's own invocation of shed step",
			run: func() (map[string]any, int) {
				return f.runLyx("shed", "step", shedrun.SelfRunID, "--"+shedverbs.UntilStopFlag)
			},
		},
		{
			name: "the loom surface",
			run:  func() (map[string]any, int) { return f.runLyx("loom", "step", "--"+shedverbs.UntilStopFlag) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.resetRunning()

			env, code := tt.run()

			loop := shedverbs.LoopOfForTest(t, env)
			shedverbs.AssertLoopKeysForTest(t, loop, false)
			if code != 1 || env["kind"] != shedverbs.KindBootstrap || loop["stop"] != string(shedverbs.LoopStopError) {
				t.Errorf("exit/kind/stop = %d/%v/%v; want 1/%s/%s", code, env["kind"], loop["stop"], shedverbs.KindBootstrap, shedverbs.LoopStopError)
			}
			if !strings.Contains(env["error"].(string), "lyx config reconcile --apply") {
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
		if err != nil || !strings.Contains(string(data), "lyx config reconcile --apply") {
			t.Errorf("trace file %q = %q, %v; want it holding the refusal", traceFile, data, err)
		}
	})
}
