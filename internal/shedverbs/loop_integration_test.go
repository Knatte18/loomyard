//go:build tier1

// loop_integration_test.go holds the helper roles the loop tests start this test binary as: a waiter, a detached loop and a step child.

package shedverbs

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
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

// runHelperChild is a step child that starts a grandchild, records its pid and the trace id it runs under, and never ends on its own.
func runHelperChild(dir string) int {
	if err := os.WriteFile(filepath.Join(dir, "trace.id"), []byte(os.Getenv(logger.TraceIDEnv)), 0o644); err != nil {
		return 1
	}
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
	if state, ok := tmuxkit.ProcState(pid); ok {
		return state == "Z"
	}
	return !proc.IsAlive(pid)
}
