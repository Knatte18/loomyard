// config_test.go pins how beam.yaml loads.
// It covers the template defaults, a worktree or local layer's override, the refusal of a value out of range,
// and the retired keys a layer no reconcile has rewritten still holds.

package dobbyengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeBeamLayer writes document as beam.yaml under baseDir's _lym/config.
func writeBeamLayer(t *testing.T, baseDir, document string) {
	t.Helper()
	path := configengine.ConfigFile(baseDir, "beam")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// defaultsWith returns the template's resolved defaults with change applied.
func defaultsWith(change func(*Config)) Config {
	cfg := Config{
		MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30,
		TaskHeadroom: 500, AdoptFixTries: 3,
	}
	change(&cfg)
	return cfg
}

func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// worktree and local are the documents of the worktree and local layers, each written when set.
		worktree, local string
		want            Config
		wantErr         []string
	}{
		{name: "the template defaults", want: defaultsWith(func(*Config) {})},
		{name: "a worktree layer's max_runs of 0 starts none", worktree: "max_runs: 0\n", want: defaultsWith(func(c *Config) { c.MaxRuns = 0 })},
		{name: "a worktree layer's pull_request of merge-in loads", worktree: "pull_request: merge-in\n", want: defaultsWith(func(*Config) {})},
		{name: "a local layer's pull_request of merge-in loads", local: "pull_request: merge-in\n", want: defaultsWith(func(*Config) {})},
		{name: "a worktree layer's land_notice of on loads", worktree: "land_notice: \"on\"\n", want: defaultsWith(func(*Config) {})},
		{name: "a local layer's land_notice of on loads", local: "land_notice: \"on\"\n", want: defaultsWith(func(*Config) {})},
		{name: "a retired pull_request of required is refused naming the key and the way forward", worktree: "pull_request: required\n", wantErr: []string{"pull_request", "required", "lym config beam --set pull_request="}},
		{name: "a pull_request of review loads", worktree: "pull_request: review\n", want: defaultsWith(func(*Config) {})},
		{name: "a pull_request of trail loads", local: "pull_request: trail\n", want: defaultsWith(func(*Config) {})},
		{name: "a renamed pull_request of on is refused naming the key and the way forward", worktree: "pull_request: \"on\"\n", wantErr: []string{"pull_request", "on", "lym config beam --set pull_request="}},
		{name: "a renamed pull_request of merge is refused naming the key and the way forward", local: "pull_request: merge\n", wantErr: []string{"pull_request", "merge", "lym config beam --set pull_request="}},
		{name: "an unknown pull_request is refused naming the key and the way forward", worktree: "pull_request: maybe\n", wantErr: []string{"pull_request", "maybe", "lym config beam --set pull_request="}},
		{name: "a retired land_notice of all is refused naming the key and the way forward", worktree: "land_notice: all\n", wantErr: []string{"land_notice", "all", "lym config beam --set land_notice="}},
		{name: "a retired land_notice of per-task is refused naming the key and the way forward", local: "land_notice: per-task\n", wantErr: []string{"land_notice", "per-task", "lym config beam --set land_notice="}},
		{name: "a non-boolean autonomy is refused naming the key and the way forward", worktree: "autonomy: maybe\n", wantErr: []string{"autonomy", "true or false", "lym config beam --set autonomy="}},
		{name: "a negative board_tidy_every is refused naming the key", worktree: "board_tidy_every: -1\n", wantErr: []string{"board_tidy_every", "lym config beam --set"}},
		{name: "a negative task_headroom is refused naming the key", worktree: "task_headroom: -1\n", wantErr: []string{"task_headroom", "lym config beam --set"}},
		{name: "a worktree layer's adopt_fix_tries resolves as set", worktree: "adopt_fix_tries: 1\n", want: defaultsWith(func(c *Config) { c.AdoptFixTries = 1 })},
		{name: "a negative adopt_fix_tries is refused naming the key", worktree: "adopt_fix_tries: -1\n", wantErr: []string{"adopt_fix_tries", "lym config beam --set"}},
		{name: "a worktree layer's retired test_map_upkeep loads and changes nothing", worktree: "test_map_upkeep: false\n", want: defaultsWith(func(*Config) {})},
		{name: "a local layer's retired test_map_every of 0 loads and changes nothing", local: "test_map_every: 0\n", want: defaultsWith(func(*Config) {})},
		{name: "a retired test_map_every other than 0 and a retired test_map_batch load and change nothing", worktree: "test_map_every: 3\ntest_map_batch: 0\n", want: defaultsWith(func(*Config) {})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board, anchor := t.TempDir(), t.TempDir()
			writeBeamLayer(t, board, ConfigTemplate())
			// An anchor with no _lym beside a told board dir has its worktree and local layers skipped,
			// so a local-only case still writes an empty worktree layer to mark the anchor.
			if tt.worktree != "" || tt.local != "" {
				writeBeamLayer(t, anchor, tt.worktree)
			}
			if tt.local != "" {
				path := configengine.LocalFile(anchor, "beam")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte(tt.local), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
				}
			}

			got, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("LoadConfig() = _, nil; want an error containing %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("LoadConfig() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}
