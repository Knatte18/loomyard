// isalive_test.go covers IsAlive, defined identically-named on both platforms, so this file needs
// no //go:build tag.
// It is allowed under this package's Test Tier Purity Invariant allowlist entry ("process control
// is the package's subject — its tests must spawn"): sub-test (2) below spawns a short-lived
// exec.Command child to obtain a confirmed-dead PID fixture.

package proc

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// TestIsAlive_CurrentProcessIsAlive asserts IsAlive reports true for the test process's own PID,
// which is always alive for the duration of the test.
func TestIsAlive_CurrentProcessIsAlive(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Errorf("IsAlive(%d) = false; want true (own process)", os.Getpid())
	}
}

// TestIsAlive_ExitedProcessIsNotAlive spawns a short-lived child, waits for it to exit, and asserts
// IsAlive reports false for its now-dead PID.
func TestIsAlive_ExitedProcessIsNotAlive(t *testing.T) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit", "0")
	} else {
		cmd = exec.Command("true")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() failed: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("cmd.Wait() failed: %v", err)
	}

	if IsAlive(pid) {
		t.Errorf("IsAlive(%d) = true; want false (process has exited)", pid)
	}
}
