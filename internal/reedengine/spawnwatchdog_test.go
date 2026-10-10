// spawnwatchdog_test.go pins SpawnWatchdog's two no-spawn early returns, spawning no subprocess and
// driving no live tmux at all — per the Test Tier Purity Invariant an untagged test file spawns
// nothing. Reaching the end of each case below is the assertion: either call would re-exec
// os.Executable() and recurse the whole suite if the early return were lost, which is precisely
// what PATTERN-spawn-observability's "never re-exec
// os.Executable() under go test" clause bars. No case in this file may ever call SpawnWatchdog
// with suppress false and a non-empty hub path.

package reedengine

import (
	"reflect"
	"testing"
)

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

func TestSpawnWatchdogEnv_DropsLogFileOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		environ []string
		want    []string
	}{
		{
			name:    "DropsOnlyTheExactKey",
			environ: []string{"PATH=/bin", "LYX_LOG_FILE=/tmp/x.log", "LYX_LOG_LEVEL=debug", "LYX_LOG_FILE_EXTRA=keep", "HOME=/home/u"},
			want:    []string{"PATH=/bin", "LYX_LOG_LEVEL=debug", "LYX_LOG_FILE_EXTRA=keep", "HOME=/home/u"},
		},
		{
			name:    "AbsentKeyReturnedAsIs",
			environ: []string{"PATH=/bin", "LYX_LOG_LEVEL=debug"},
			want:    []string{"PATH=/bin", "LYX_LOG_LEVEL=debug"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spawnWatchdogEnv(tt.environ)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("spawnWatchdogEnv(%q) = %q, want %q", tt.environ, got, tt.want)
			}
		})
	}
}
