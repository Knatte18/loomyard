// config_test.go pins how beam.yaml loads.
// It covers the template defaults, a worktree layer's override, and the refusal of a negative value.

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

func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		worktree string
		want     Config
		wantErr  []string
	}{
		{name: "the template defaults", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "on", LandNotice: "per-task", AdoptFixTries: 3}},
		{name: "a worktree layer's max_runs of 0 starts none", worktree: "max_runs: 0\n", want: Config{MaxRuns: 0, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "on", LandNotice: "per-task", AdoptFixTries: 3}},
		{name: "a worktree layer's pull_request of required resolves as set", worktree: "pull_request: required\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "required", LandNotice: "per-task", AdoptFixTries: 3}},
		{name: "a worktree layer's pull_request of off resolves as set", worktree: "pull_request: \"off\"\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "off", LandNotice: "per-task", AdoptFixTries: 3}},
		{name: "a worktree layer's pull_request of merge resolves as set", worktree: "pull_request: merge\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "merge", LandNotice: "per-task", AdoptFixTries: 3}},
		{name: "an unknown pull_request is refused naming the key and the way forward", worktree: "pull_request: maybe\n", wantErr: []string{"pull_request", "must be on, off, required or merge", "lym config beam --set pull_request="}},
		{name: "a worktree layer's land_notice of all resolves as set", worktree: "land_notice: all\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "on", LandNotice: "all", AdoptFixTries: 3}},
		{name: "a worktree layer's land_notice of off resolves as set", worktree: "land_notice: \"off\"\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "on", LandNotice: "off", AdoptFixTries: 3}},
		{name: "an unknown land_notice is refused naming the key and the way forward", worktree: "land_notice: some\n", wantErr: []string{"land_notice", "must be all, per-task or off", "lym config beam --set land_notice="}},
		{name: "a negative board_tidy_every is refused naming the key", worktree: "board_tidy_every: -1\n", wantErr: []string{"board_tidy_every", "lym config beam --set"}},
		{name: "a negative task_headroom is refused naming the key", worktree: "task_headroom: -1\n", wantErr: []string{"task_headroom", "lym config beam --set"}},
		{name: "a worktree layer's adopt_fix_tries resolves as set", worktree: "adopt_fix_tries: 1\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "on", LandNotice: "per-task", AdoptFixTries: 1}},
		{name: "a negative adopt_fix_tries is refused naming the key", worktree: "adopt_fix_tries: -1\n", wantErr: []string{"adopt_fix_tries", "lym config beam --set"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board, anchor := t.TempDir(), t.TempDir()
			writeBeamLayer(t, board, ConfigTemplate())
			if tt.worktree != "" {
				writeBeamLayer(t, anchor, tt.worktree)
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
