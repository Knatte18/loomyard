// config_test.go pins how beam.yaml loads and how its retired test_map_every migrates.
// It covers the template defaults, a worktree or local layer's override, the refusal of a value out of range,
// and the retired keys a layer no reconcile has rewritten still holds.

package dobbyengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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
		MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TestMapUpkeep: true,
		TaskHeadroom: 500, PullRequest: "on", LandNotice: "per-task", AdoptFixTries: 3,
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
		{name: "a worktree layer's pull_request of required resolves as set", worktree: "pull_request: required\n", want: defaultsWith(func(c *Config) { c.PullRequest = "required" })},
		{name: "a worktree layer's pull_request of off resolves as set", worktree: "pull_request: \"off\"\n", want: defaultsWith(func(c *Config) { c.PullRequest = "off" })},
		{name: "a worktree layer's pull_request of merge resolves as set", worktree: "pull_request: merge\n", want: defaultsWith(func(c *Config) { c.PullRequest = "merge" })},
		{name: "an unknown pull_request is refused naming the key and the way forward", worktree: "pull_request: maybe\n", wantErr: []string{"pull_request", "must be on, off, required or merge", "lym config beam --set pull_request="}},
		{name: "a worktree layer's land_notice of all resolves as set", worktree: "land_notice: all\n", want: defaultsWith(func(c *Config) { c.LandNotice = "all" })},
		{name: "a worktree layer's land_notice of off resolves as set", worktree: "land_notice: \"off\"\n", want: defaultsWith(func(c *Config) { c.LandNotice = "off" })},
		{name: "an unknown land_notice is refused naming the key and the way forward", worktree: "land_notice: some\n", wantErr: []string{"land_notice", "must be all, per-task or off", "lym config beam --set land_notice="}},
		{name: "a negative board_tidy_every is refused naming the key", worktree: "board_tidy_every: -1\n", wantErr: []string{"board_tidy_every", "lym config beam --set"}},
		{name: "a negative task_headroom is refused naming the key", worktree: "task_headroom: -1\n", wantErr: []string{"task_headroom", "lym config beam --set"}},
		{name: "a worktree layer's adopt_fix_tries resolves as set", worktree: "adopt_fix_tries: 1\n", want: defaultsWith(func(c *Config) { c.AdoptFixTries = 1 })},
		{name: "a negative adopt_fix_tries is refused naming the key", worktree: "adopt_fix_tries: -1\n", wantErr: []string{"adopt_fix_tries", "lym config beam --set"}},
		{name: "a worktree layer's test_map_upkeep of false resolves as set", worktree: "test_map_upkeep: false\n", want: defaultsWith(func(c *Config) { c.TestMapUpkeep = false })},
		{name: "an unreconciled worktree layer's test_map_every of 0 loads as test_map_upkeep false", worktree: "test_map_every: 0\n", want: defaultsWith(func(c *Config) { c.TestMapUpkeep = false })},
		{name: "an unreconciled local layer's test_map_every of 0 loads as test_map_upkeep false", local: "test_map_every: 0\n", want: defaultsWith(func(c *Config) { c.TestMapUpkeep = false })},
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

func TestMigrateConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		existing  string
		want      string
		wantLines []string
		// wantInvalid expects an error marked configengine.ErrInvalid instead of a document.
		wantInvalid bool
	}{
		{
			name:      "test_map_every 0 becomes test_map_upkeep false in its place, without the retired key's comment",
			existing:  "max_runs: 3\ntest_map_every: 0 # landings between refreshes\nland_notice: all\n",
			want:      "max_runs: 3\ntest_map_upkeep: false\nland_notice: all\n",
			wantLines: []string{"test_map_every: 0 became test_map_upkeep: false"},
		},
		{
			name:      "any other test_map_every is removed and test_map_batch is left to the retired-key prune",
			existing:  "test_map_every: 5\ntest_map_batch: 2\nmax_runs: 2\n",
			want:      "test_map_batch: 2\nmax_runs: 2\n",
			wantLines: []string{"test_map_every: retired, removed"},
		},
		{
			name:      "a document that sets test_map_upkeep keeps it, and its test_map_every 0 is removed",
			existing:  "test_map_upkeep: true\ntest_map_every: 0\n",
			want:      "test_map_upkeep: true\n",
			wantLines: []string{"test_map_every: retired, removed"},
		},
		{name: "a document without test_map_every is returned byte for byte", existing: "max_runs:   2 # kept\n", want: "max_runs:   2 # kept\n"},
		{name: "an empty document is returned unchanged", existing: "", want: ""},
		{name: "a root that is no mapping is refused as invalid", existing: "- test_map_every\n", wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := MigrateConfig([]byte(tt.existing))
			if tt.wantInvalid {
				if !errors.Is(err, configengine.ErrInvalid) {
					t.Errorf("MigrateConfig(%q) error = %v; want one marked ErrInvalid", tt.existing, err)
				}
				return
			}
			if err != nil || string(got) != tt.want || !slices.Equal(lines, tt.wantLines) {
				t.Errorf("MigrateConfig(%q) = %q, %q, %v; want %q, %q, nil", tt.existing, got, lines, err, tt.want, tt.wantLines)
			}
		})
	}
}
