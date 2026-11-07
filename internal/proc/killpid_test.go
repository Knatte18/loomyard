// killpid_test.go covers KillPID, which has the same name and signature on both platforms.
// The file therefore carries no platform constraint.
// Its sub-tests spawn real children.
// One holds its child alive to prove KillPID terminates it.
// The other waits on its child to obtain a confirmed-dead PID fixture.
// The package's Test Tier Purity allowlist admits this, since process control is the package's subject.

package proc

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestKillPID_KillsLiveProcess spawns a child that blocks for several seconds.
// It calls KillPID on the child's PID and asserts KillPID reports no error.
// It also asserts the child actually terminated.
// cmd.Wait returns a non-nil error for that forced termination, not a normal exit.
func TestKillPID_KillsLiveProcess(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

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

	// A killed process's Wait must report an error (a non-zero or signaled exit).
	// That separates a forced termination from the sleep elapsing on its own.
	if err := cmd.Wait(); err == nil {
		t.Errorf("cmd.Wait() = nil; want non-nil (process was force-killed, not a clean exit)")
	}
}

// TestKillPID_DeadPIDReturnsError takes the PID of a child that has exited.
// It asserts that KillPID returns a non-nil error without panicking.
// A dead PID has no live process to terminate.
// The platform call (FindProcess/Kill) must surface that as an error rather than succeed silently.
func TestKillPID_DeadPIDReturnsError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pid := exitedChildPID(t)
	if err := KillPID(pid); err == nil {
		t.Errorf("KillPID(%d) = nil; want non-nil error (process has exited)", pid)
	}
}
