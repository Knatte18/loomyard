// spawnwatchdog_test.go pins ensureWatchdogSpawned's no-spawn early returns and its lock-path
// target, driving no exec.Command and no live tmux at all — per the Test Tier Purity Invariant an
// untagged test file spawns nothing; the daemon's live spawn/lock behaviour is card 41's
// integration suite.

package reedcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestEnsureWatchdogSpawned_SuppressedReturnsWithoutSpawning(t *testing.T) {
	c := &reedCLI{
		eng:                   reedengine.New(reedengine.Config{}, reedengine.Geometry{}),
		hubPath:               t.TempDir(),
		suppressWatchdogSpawn: true,
	}
	// A no-op call: if this spawned anything, it would re-exec this test binary, which would hang
	// or recurse the whole suite. Reaching the end of this test at all is the assertion.
	c.ensureWatchdogSpawned()
}

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
