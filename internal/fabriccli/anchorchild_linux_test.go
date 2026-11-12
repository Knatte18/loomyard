//go:build linux

// anchorchild_linux_test.go pins which live children of a dobby the anchor verb counts as a pass in flight.
// A child registered as a push, board sync or coverage-map upkeep pass is spared, and any other child counts.

package fabriccli_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// childOfFreshParent starts a shell that holds one `sleep` child, and returns the shell's pid and the child's.
// The shell stands in for a dobby, so no other process of the test binary is among its children.
// Cleanup ends both.
func childOfFreshParent(t *testing.T) (parentPID, childPID int) {
	t.Helper()
	parent := exec.Command("sh", "-c", "sleep 60 & wait")
	if err := parent.Start(); err != nil {
		t.Fatalf("start the stand-in dobby: %v", err)
	}
	t.Cleanup(func() {
		_ = parent.Process.Kill()
		_ = parent.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for childPID == 0 {
		stats, err := proc.ListStats()
		if err != nil {
			t.Fatalf("ListStats() = %v; want nil", err)
		}
		for _, stat := range stats {
			if stat.PPID == parent.Process.Pid && !stat.Zombie {
				childPID = stat.PID
			}
		}
		if childPID == 0 {
			if time.Now().After(deadline) {
				t.Fatal("the stand-in dobby started no child within 10s")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.Cleanup(func() {
		if child, err := os.FindProcess(childPID); err == nil {
			_ = child.Kill()
		}
	})
	return parent.Process.Pid, childPID
}

func TestLiveUnsparedChild(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// role is the role the child is registered under, empty for a child registered nowhere.
		role      string
		wantFound bool
	}{
		{name: "a child registered as the upkeep pass is spared", role: runproc.RoleTestMap},
		{name: "a child registered as a push is spared", role: runproc.RolePush},
		{name: "a child registered as a board sync is spared", role: runproc.RoleSync},
		{name: "a child registered under another role counts", role: runproc.RoleBatten, wantFound: true},
		{name: "a child registered nowhere counts", wantFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runProcDir := t.TempDir()
			parentPID, childPID := childOfFreshParent(t)
			if tt.role != "" {
				if err := runproc.Register(runProcDir, runproc.HubOwner, runproc.ChildRecord(childPID, tt.role, []string{"sleep", "60"})); err != nil {
					t.Fatalf("Register() = %v; want nil", err)
				}
			}

			got, found, err := fabriccli.LiveUnsparedChildForTest(runProcDir, []int{parentPID})
			if err != nil {
				t.Fatalf("LiveUnsparedChild() = _, _, %v; want nil", err)
			}
			if found != tt.wantFound {
				t.Fatalf("LiveUnsparedChild() found = %v; want %v", found, tt.wantFound)
			}
			if tt.wantFound && got != childPID {
				t.Errorf("LiveUnsparedChild() = %d; want the child %d", got, childPID)
			}
		})
	}
}
