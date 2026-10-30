//go:build tier0

// config_test.go pins beam.yaml's load: the template defaults, a worktree layer's override and the refusal of a negative value.

package dobbyengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
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
	t.Parallel()

	tests := []struct {
		name     string
		worktree string
		want     Config
		wantErr  []string
	}{
		{name: "the template defaults", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "on"}},
		{name: "a worktree layer's max_runs of 0 starts none", worktree: "max_runs: 0\n", want: Config{MaxRuns: 0, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "on"}},
		{name: "a worktree layer's pull_request of required resolves as set", worktree: "pull_request: required\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "required"}},
		{name: "a worktree layer's pull_request of off resolves as set", worktree: "pull_request: \"off\"\n", want: Config{MaxRuns: 3, BoardTidyEvery: 5, BoardTidyTimeoutMin: 30, TaskHeadroom: 500, PullRequest: "off"}},
		{name: "an unknown pull_request is refused naming the key and the way forward", worktree: "pull_request: maybe\n", wantErr: []string{"pull_request", "must be on, off or required", "lym config beam --set pull_request="}},
		{name: "a negative board_tidy_every is refused naming the key", worktree: "board_tidy_every: -1\n", wantErr: []string{"board_tidy_every", "lym config beam --set"}},
		{name: "a negative task_headroom is refused naming the key", worktree: "task_headroom: -1\n", wantErr: []string{"task_headroom", "lym config beam --set"}},
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
