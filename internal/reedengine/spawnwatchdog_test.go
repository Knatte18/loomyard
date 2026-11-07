// spawnwatchdog_test.go pins that SpawnWatchdog's two no-spawn early returns spawn no subprocess and drive no live tmux.
// Reaching the end of each case is the assertion:
// if an early return were lost, the call would re-exec os.Executable() and recurse the whole suite.
// An untagged test file spawns nothing, per the Test Tier Purity Invariant.
// No case here may call SpawnWatchdog with suppress false and a non-empty hub path.

package reedengine

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSpawnWatchdog_ReturnsWithoutSpawning(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		environ []string
		want    []string
	}{
		{
			name:    "DropsOnlyTheExactKey",
			environ: []string{"PATH=/bin", "LYM_LOG_FILE=/tmp/x.log", "LYM_LOG_LEVEL=debug", "LYM_LOG_FILE_EXTRA=keep", "HOME=/home/u"},
			want:    []string{"PATH=/bin", "LYM_LOG_LEVEL=debug", "LYM_LOG_FILE_EXTRA=keep", "HOME=/home/u"},
		},
		{
			name:    "AbsentKeyReturnedAsIs",
			environ: []string{"PATH=/bin", "LYM_LOG_LEVEL=debug"},
			want:    []string{"PATH=/bin", "LYM_LOG_LEVEL=debug"},
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
