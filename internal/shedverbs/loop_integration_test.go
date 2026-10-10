//go:build integration

// loop_integration_test.go runs the loop against real processes.
// The arming-refusal test drives a lyx built by internal/testkit/lyxbin over a loom-seeded run in a real hub;
// the child-tree test starts this test binary as a waiter, a detached loop and a step child, to see which of them the others outlive.

package shedverbs

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/spf13/cobra"
)

// helperDirEnv names the directory a helper-role process works in; its presence is what makes the test binary run as a helper.
const helperDirEnv = "SHEDVERBS_HELPER_DIR"

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
func (f *loomRunFixture) childLoopSpec() *Spec {
	loc, run := f.loc, shedrun.SelfRunID
	return &Spec{
		StatusPath:     shedrun.StatusFile(loc, run),
		LockPath:       shedrun.RunLock(loc, run),
		StatusLockPath: shedrun.StatusLock(loc, run),
		RunID:          shedrun.ResolveRunID(loc, run),
		StepsDir:       shedrun.StepsDir(loc, run),
		Loop: LoopSpec{
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
				shed.AddCommand(stepCmd(stepTexts(), f.childLoopSpec()))
				root.AddCommand(shed)
				var out bytes.Buffer
				code := clihelp.Execute(root, &out, []string{"shed", "step", shedrun.SelfRunID, "--" + UntilStopFlag, "--" + LoopDetachedFlag + "=loop-int-1"})
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
			run:  func() (map[string]any, int) { return f.runLyx("shed", "step", shedrun.SelfRunID, "--"+UntilStopFlag) },
		},
		{
			name: "the loom surface",
			run:  func() (map[string]any, int) { return f.runLyx("loom", "step", "--"+UntilStopFlag) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.resetRunning()

			env, code := tt.run()

			loop := loopOf(t, env)
			assertLoopKeys(t, loop, false)
			if code != 1 || env["kind"] != KindBootstrap || loop["stop"] != string(LoopStopError) {
				t.Errorf("exit/kind/stop = %d/%v/%v; want 1/%s/%s", code, env["kind"], loop["stop"], KindBootstrap, LoopStopError)
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
		loop := loopOf(t, childLoopEnv)
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

// helperSpec is the spec the helper-role processes arm, anchored in dir.
func helperSpec(dir, executable string) *Spec {
	steps := filepath.Join(dir, "steps")
	return &Spec{
		StatusPath:     filepath.Join(dir, "status.json"),
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: filepath.Join(dir, "status.lock"),
		RunID:          "run-1",
		StepsDir:       steps,
		Loop: LoopSpec{
			LockPath:     filepath.Join(steps, "loop.lock"),
			PIDPath:      filepath.Join(steps, "loop.pid"),
			LogPath:      filepath.Join(steps, "loop.log"),
			EnvelopePath: filepath.Join(steps, "loop-envelope.json"),
			Delivered:    func(loopID string) string { return filepath.Join(steps, "loop-envelope."+loopID+".delivered.json") },
			Executable:   executable,
		},
	}
}

// runHelperRole runs this binary as one of the processes of TestLoopIntegration_ChildTreeDiesWithLoop and reports whether it was started as one.
// A command line carrying the detached flag is the loop, `step` alone is a step child, and anything else is the waiter.
func runHelperRole() (int, bool) {
	dir := os.Getenv(helperDirEnv)
	if dir == "" {
		return 0, false
	}
	args := os.Args[1:]
	switch {
	case slices.ContainsFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "--"+LoopDetachedFlag) }):
		return executeHelperStep(dir, args), true
	case len(args) == 1 && args[0] == "step":
		return runHelperChild(dir), true
	}
	return executeHelperStep(dir, []string{"step", "--" + UntilStopFlag}), true
}

// executeHelperStep runs the step command over the helper spec as the waiter or the loop, and returns its exit code.
func executeHelperStep(dir string, args []string) int {
	root := &cobra.Command{Use: "helper"}
	root.AddCommand(stepCmd(stepTexts(), helperSpec(dir, os.Args[0])))
	return clihelp.Execute(root, os.Stdout, args)
}

// runHelperChild is a step child that starts a grandchild, records its pid and never ends on its own.
func runHelperChild(dir string) int {
	grandchild := exec.Command("sleep", "300")
	if err := grandchild.Start(); err != nil {
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, "grandchild.pid"), []byte(strconv.Itoa(grandchild.Process.Pid)), 0o644); err != nil {
		return 1
	}
	time.Sleep(10 * time.Minute)
	return 0
}

// startHelperWaiter starts this binary as the waiter over dir and returns it with the buffer its output lands in.
func startHelperWaiter(t *testing.T, dir string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperDirEnv+"="+dir)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the helper waiter: %v", err)
	}
	return cmd, &out
}

// waitUntil polls cond every 50ms and fails the test when it is still false after limit.
func waitUntil(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", limit, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// processEnded reports whether pid no longer runs; a zombie that nothing has reaped yet counts as ended.
func processEnded(pid int) bool {
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		stat := string(data)
		fields := strings.Fields(stat[strings.LastIndexByte(stat, ')')+1:])
		return len(fields) > 0 && fields[0] == "Z"
	}
	return !proc.IsAlive(pid)
}

func TestLoopIntegration_ChildTreeDiesWithLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the tree kill is the named job's on Windows")
	}
	dir := t.TempDir()
	spec := helperSpec(dir, "")
	status := shedengine.Status{CurrentProducer: "A", State: shedengine.StateRunning, History: []shedengine.HistoryEntry{}}
	if err := state.WriteJSON(spec.StatusPath, spec.StatusLockPath, status); err != nil {
		t.Fatalf("write the status file: %v", err)
	}

	waiter, _ := startHelperWaiter(t, dir)
	t.Cleanup(func() { _ = waiter.Process.Kill() })
	var record LoopPIDRecord
	var grandchild int
	waitUntil(t, 30*time.Second, "the loop's step child to start a grandchild", func() bool {
		found := false
		var err error
		record, found, err = ReadLoopPIDRecord(spec.Loop.PIDPath)
		data, readErr := os.ReadFile(filepath.Join(dir, "grandchild.pid"))
		if err != nil || !found || record.Child.PID == 0 || readErr != nil {
			return false
		}
		grandchild, err = strconv.Atoi(string(data))
		return err == nil
	})
	t.Cleanup(func() {
		_, _ = proc.KillTree(record.Child)
		_ = proc.KillPID(record.Loop.PID)
		_ = proc.KillPID(grandchild)
	})

	if err := waiter.Process.Kill(); err != nil {
		t.Fatalf("kill the waiter: %v", err)
	}
	_ = waiter.Wait()
	if loopLockFree(spec.Loop.LockPath) || processEnded(record.Loop.PID) || processEnded(record.Child.PID) || processEnded(grandchild) {
		t.Fatalf("after the waiter's death the loop, its child or the grandchild ended; want all three to outlive the waiter")
	}

	if err := proc.KillPID(record.Loop.PID); err != nil {
		t.Fatalf("kill the loop: %v", err)
	}
	waitUntil(t, 30*time.Second, "the step child to die with the loop", func() bool { return processEnded(record.Child.PID) })

	second, out := startHelperWaiter(t, dir)
	if err := second.Wait(); err == nil {
		t.Errorf("the second waiter exited 0; want the loop-exited stop's exit 1")
	}
	var env map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &env); err != nil {
		t.Fatalf("decode the second waiter's envelope from %q: %v", out.String(), err)
	}
	if env["kind"] != KindInterrupted || !strings.Contains(env["error"].(string), "(loop-exited)") {
		t.Errorf("second waiter kind/error = %v/%v; want the loop-exited stop", env["kind"], env["error"])
	}
	waitUntil(t, 30*time.Second, "the dead loop's step tree to be killed", func() bool { return processEnded(grandchild) })
	if st, found, err := state.ReadJSONStrict[shedengine.Status](spec.StatusPath, spec.StatusLockPath); err != nil || !found || st.State != shedengine.StateFailed {
		t.Errorf("status = %+v, %v, %v; want failed", st, found, err)
	}
}
