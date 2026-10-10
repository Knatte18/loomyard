//go:build integration

// loop_integration_test.go runs the loop against real processes.
// The child-tree test starts this test binary as a waiter, a detached loop and a step child, to see which of them the others outlive.

package shedverbs

import (
	"bytes"
	"encoding/json"
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
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// helperDirEnv names the directory a helper-role process works in; its presence is what makes the test binary run as a helper.
const helperDirEnv = "SHEDVERBS_HELPER_DIR"

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
