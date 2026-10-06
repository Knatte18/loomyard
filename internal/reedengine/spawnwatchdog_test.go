// spawnwatchdog_test.go pins SpawnWatchdog's two no-spawn early returns, spawning no subprocess and
// driving no live tmux at all — per the Test Tier Purity Invariant an untagged test file spawns
// nothing. Reaching the end of each case below is the assertion: either call would re-exec
// os.Executable() and recurse the whole suite if the early return were lost, which is precisely
// what PATTERN-spawn-observability's "never re-exec
// os.Executable() under go test" clause bars. No case in this file may ever call SpawnWatchdog
// with suppress false and a non-empty hub path.

package reedengine

import "testing"

func TestSpawnWatchdog_ReturnsWithoutSpawning(t *testing.T) {
	tests := []struct {
		name     string
		hubPath  string
		suppress bool
	}{
		{"Suppressed", t.TempDir(), true},
		{"EmptyHubPath", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SpawnWatchdog(tt.hubPath, "tmux", "shell", tt.suppress)
		})
	}
}
