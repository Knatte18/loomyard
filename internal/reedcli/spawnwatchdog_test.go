// spawnwatchdog_test.go pins that ensureWatchdogSpawned reaches the reedengine seam and that a
// call under a test binary re-execs nothing, plus the daemon's lock-path target — spawning no
// subprocess and driving no live tmux at all, per the Test Tier Purity Invariant. SpawnWatchdog's
// own no-spawn-early-return mechanism is pinned in internal/reedengine/spawnwatchdog_test.go, not
// re-tested here; the daemon's live spawn/lock behaviour is card 41's integration suite.

package reedcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestEnsureWatchdogSpawned_SuppressedReturnsWithoutSpawning pins that ensureWatchdogSpawned
// reaches reedengine.SpawnWatchdog with suppress true and re-execs nothing under a test binary. A
// no-op call: if this spawned anything, it would re-exec this test binary, which would hang or
// recurse the whole suite. Reaching the end of this test at all is the assertion.
func TestEnsureWatchdogSpawned_SuppressedReturnsWithoutSpawning(t *testing.T) {
	c := &reedCLI{
		eng:                   reedengine.New(reedengine.Config{}, reedengine.Geometry{}),
		hubPath:               t.TempDir(),
		suppressWatchdogSpawn: true,
	}
	c.ensureWatchdogSpawned()
}

// TestEnsureWatchdogSpawned_EmptyHubPathReturnsWithoutSpawning pins that ensureWatchdogSpawned
// reaches reedengine.SpawnWatchdog with an empty hub path and re-execs nothing under a test
// binary, exercising SpawnWatchdog's other early return through the same delegating call.
func TestEnsureWatchdogSpawned_EmptyHubPathReturnsWithoutSpawning(t *testing.T) {
	c := &reedCLI{
		eng:                   reedengine.New(reedengine.Config{}, reedengine.Geometry{}),
		hubPath:               "",
		suppressWatchdogSpawn: false,
	}
	c.ensureWatchdogSpawned()
}

func TestWatchdogLockPath_IsHubScratchDirLockFile(t *testing.T) {
	hub := t.TempDir()
	want := filepath.Join(fabricengine.HubScratchDir(hub), watchdogLockFileName)
	got := filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock")
	if got != want {
		t.Errorf("watchdog lock path = %q; want %q", got, want)
	}
}
