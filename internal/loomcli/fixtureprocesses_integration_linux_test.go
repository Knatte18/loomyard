// fixtureprocesses_integration_linux_test.go pins how the smoke fixtures end the processes left under their temp dirs.
// It reads the process table through /proc, so only Linux compiles it.

package loomcli

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWaitForFixtureProcessesToExit_KillsAProcessThatOutlivesItsPatience asserts the fixture cleanup ends a process.
// The process runs under the test's temp dir and does not exit within the patience.
// It stands for a detached `lym board sync` that is still running after its verb returned.
// It must be dead once the test returns, or it outlives the package's run and fails it.
//
//testtiming:keep pins that the fixture cleanup SIGKILLs a process left under the test's temp root
func TestWaitForFixtureProcessesToExit_KillsAProcessThatOutlivesItsPatience(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	done := make(chan error, 1)
	var cmd *exec.Cmd
	t.Run("fixture", func(t *testing.T) {
		waitForFixtureProcessesToExit(t, 100*time.Millisecond)
		cmd = exec.Command("sleep", "60")
		cmd.Dir = t.TempDir()
		if err := cmd.Start(); err != nil {
			t.Fatalf("start sleep: %v", err)
		}
		go func() { done <- cmd.Wait() }()
	})
	if cmd == nil || cmd.Process == nil {
		t.Fatal("the fixture subtest started no process")
	}

	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("sleep under the fixture's temp dir ended with %v; want it killed", err)
		}
		status := exitErr.Sys().(syscall.WaitStatus)
		if !status.Signaled() || status.Signal() != syscall.SIGKILL {
			t.Errorf("sleep under the fixture's temp dir ended with status %v; want killed by SIGKILL", status)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("sleep under the fixture's temp dir still runs after the fixture's cleanup; want it killed")
	}
}
