// isalive_test.go covers IsAlive, which has the same name and signature on both platforms.
// The file therefore carries no platform constraint.
// The "exited child" row spawns a short-lived child to obtain a confirmed-dead PID fixture.
// The package's Test Tier Purity allowlist admits that, since process control is the package's subject.

package proc

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// exitedChildPID spawns a short-lived child, waits for it to exit and returns its now-dead PID.
func exitedChildPID(t *testing.T) int {
	t.Helper()

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
	return pid
}

// TestIsAlive asserts IsAlive reports true for the test process's own PID, which lives for the whole test.
// It also asserts false for the PID of a child that has exited.
func TestIsAlive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		pid  func(t *testing.T) int
		want bool
	}{
		{"own process", func(*testing.T) int { return os.Getpid() }, true},
		{"exited child", exitedChildPID, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pid := tt.pid(t)
			if got := IsAlive(pid); got != tt.want {
				t.Errorf("IsAlive(%d) = %v; want %v", pid, got, tt.want)
			}
		})
	}
}
