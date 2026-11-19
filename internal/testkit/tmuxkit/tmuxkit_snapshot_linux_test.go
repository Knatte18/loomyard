// tmuxkit_snapshot_linux_test.go holds the tier-1 test of the process snapshot.
// The snapshot reads /proc, so the test runs only on Linux.

package tmuxkit

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSnapshotProcesses_NamesAChildWorkingInTheDirectoryAndLeavesOutThisBinary reads the live process table.
// A child started in the directory is named by pid and argv, and this test binary is never one of the others.
func TestSnapshotProcesses_NamesAChildWorkingInTheDirectoryAndLeavesOutThisBinary(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	dir := t.TempDir()
	child := exec.Command("sleep", "60")
	child.Dir = dir
	if err := child.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	at := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	got := snapshotProcesses(dir, at)

	if !got.attributed || !got.at.Equal(at) {
		t.Errorf("snapshotProcesses(%q) attributed = %v, at = %v; want true, %v", dir, got.attributed, got.at, at)
	}
	if !got.ownInDir {
		t.Errorf("snapshotProcesses(%q) ownInDir = false; want true for a child of this binary working there", dir)
	}
	want := []string{fmt.Sprintf("pid %d %q", child.Process.Pid, []string{"sleep", "60"})}
	if !slices.Equal(got.inDir, want) {
		t.Errorf("snapshotProcesses(%q) inDir = %q; want %q", dir, got.inDir, want)
	}
	self := fmt.Sprintf("pid %d ", os.Getpid())
	for _, other := range got.others {
		if strings.HasPrefix(other, self) {
			t.Errorf("snapshotProcesses(%q) others = %q; want this binary left out", dir, got.others)
		}
	}
}
