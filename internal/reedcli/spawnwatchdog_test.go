// spawnwatchdog_test.go pins that ensureWatchdogSpawned reaches the reedengine seam and that a
// call under a test binary re-execs nothing — spawning no subprocess and driving no live tmux at
// all, per the Test Tier Purity Invariant. SpawnWatchdog's own no-spawn-early-return mechanism is
// pinned in internal/reedengine/spawnwatchdog_test.go, not re-tested here; the daemon's live
// spawn/lock behaviour is card 41's integration suite.

package reedcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// A no-op call: if this spawned anything, it would re-exec this test binary, which would hang or
// recurse the whole suite. Reaching the end of this test at all is the assertion.
func TestEnsureWatchdogSpawned_SuppressedReturnsWithoutSpawning(t *testing.T) {
	c := &reedCLI{
		eng:                   reedengine.New(reedengine.Config{}, reedengine.Geometry{}),
		hubPath:               t.TempDir(),
		suppressWatchdogSpawn: true,
	}
	c.ensureWatchdogSpawned()
}
