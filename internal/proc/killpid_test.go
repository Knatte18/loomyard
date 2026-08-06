// killpid_test.go covers KillPID, defined identically-named on both platforms, so this file needs
// no //go:build tag — it is the untagged shared home for KillPID, mirroring how isalive_test.go is
// the untagged shared home for IsAlive.
// It is allowed under this package's Test Tier Purity Invariant allowlist entry ("process control
// is the package's subject — its tests must spawn"): both sub-tests below spawn a real child
// process, one held alive to prove KillPID actually terminates it, one waited on immediately to
// obtain a confirmed-dead PID fixture.

package proc

import (
	"os/exec"
	"runtime"
	"testing"
)

// TestKillPID_KillsLiveProcess spawns a child that blocks for several seconds, calls KillPID on its
// PID, and asserts both that KillPID reports no error and that the child actually terminated
// (cmd.Wait returns a non-nil error, reflecting the forced termination rather than a normal exit).
func TestKillPID_KillsLiveProcess(t *testing.T) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "ping", "-n", "6", "127.0.0.1")
	} else {
		cmd = exec.Command("sleep", "5")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() failed: %v", err)
	}
	pid := cmd.Process.Pid

	if err := KillPID(pid); err != nil {
		t.Errorf("KillPID(%d) = %v; want nil (live process)", pid, err)
	}

	// A killed process's Wait must report an error (non-zero/signaled exit),
	// distinguishing an actual forced termination from the 5-second sleep
	// simply having already elapsed on its own.
	if err := cmd.Wait(); err == nil {
		t.Errorf("cmd.Wait() = nil; want non-nil (process was force-killed, not a clean exit)")
	}
}

// TestKillPID_DeadPIDReturnsError spawns a short-lived child, waits for it to exit to obtain a
// confirmed-dead PID, and asserts KillPID returns a non-nil error without panicking.
// This matches the PID-reuse trust KillPID documents: a dead PID has no live process to terminate,
// so the platform call underneath (FindProcess/Kill) must surface that as an error rather than
// succeeding silently.
func TestKillPID_DeadPIDReturnsError(t *testing.T) {
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

	if err := KillPID(pid); err == nil {
		t.Errorf("KillPID(%d) = nil; want non-nil error (process has exited)", pid)
	}
}
