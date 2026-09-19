// spawnwatchdog_test.go pins SpawnWatchdog's two no-spawn early returns, spawning no subprocess and
// driving no live tmux at all — per the Test Tier Purity Invariant an untagged test file spawns
// nothing. Reaching the end of either test below is the assertion: either call would re-exec
// os.Executable() and recurse the whole suite if the early return were lost, which is precisely
// what CONSTRAINTS.md's Live-Substrate Spawn Observability invariant's "never re-exec
// os.Executable() under go test" clause bars. No test in this file may ever call SpawnWatchdog
// with suppress false and a non-empty hub path.

package reedengine

import "testing"

func TestSpawnWatchdog_SuppressedReturnsWithoutSpawning(t *testing.T) {
	SpawnWatchdog(t.TempDir(), "tmux", true)
}

func TestSpawnWatchdog_EmptyHubPathReturnsWithoutSpawning(t *testing.T) {
	SpawnWatchdog("", "tmux", false)
}
