//go:build !linux

// tmuxkit_snapshot_other_test.go holds the tier-1 test of the process snapshot off Linux, where no process table is read.

package tmuxkit

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSnapshotProcesses_OffLinuxIsUnattributedAndKeepsTheTime(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	at := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	got := snapshotProcesses(t.TempDir(), at)

	if got.attributed || !got.at.Equal(at) || len(got.others) != 0 || len(got.inDir) != 0 || got.ownInDir {
		t.Errorf("snapshotProcesses off Linux = %+v; want only at = %v", got, at)
	}
}
