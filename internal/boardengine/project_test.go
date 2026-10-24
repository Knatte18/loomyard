//go:build tier0

// project_test.go — the project kind's write rules (project.go): what a write may set on a project and its members, through a Board built from a board.yaml.

package boardengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
)

// newProjectBoard returns a board whose config LoadConfig reads from a board.yaml with the bug and enhancement type labels and the Home.md output, under t.TempDir().
// configure, when non-nil, edits the loaded config before the board is built, so a test sets the seams it needs.
func newProjectBoard(t *testing.T, configure func(cfg *boardengine.Config)) *boardengine.Board {
	t.Helper()
	configDir := writeBoardConfig(t, "readme: Home.md\ndesign_prefix: proposal-\ntypes:\n  bug: Broken\n  enhancement: New\nlabels:\n  undecided: Not decided\n")
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: configDir}, "board")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v; want nil", err)
	}
	cfg.Path = t.TempDir()
	cfg.SkipGit = true
	cfg.Clock = func() time.Time { return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC) }
	if configure != nil {
		configure(&cfg)
	}
	return boardengine.New(cfg)
}

// seedBoardJSON writes a board.json holding entries into the board directory of b's config, which only a stored state can hold when no upsert key reaches it.
func seedBoardJSON(t *testing.T, boardPath string, entries ...string) {
	t.Helper()
	content := `{"version":1,"entries":[` + strings.Join(entries, ",") + `]}`
	if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("seed board.json: %v", err)
	}
}

// TestProjectRules pins that a write setting or changing a task's depends_on to a project is refused naming the project's task members,
// and that a stored dependency on a project takes any write that leaves it unchanged.
func TestProjectRules(t *testing.T) {
	t.Parallel()
	var boardPath string
	b := newProjectBoard(t, func(cfg *boardengine.Config) { boardPath = cfg.Path })
	seedBoardJSON(t, boardPath,
		`{"id":0,"slug":"proj","title":"Proj","kind":"project","labels":["bug"]}`,
		`{"id":1,"slug":"member","title":"Member","kind":"task","labels":["bug"],"project":"proj"}`,
		`{"id":2,"slug":"stale","title":"Stale","kind":"task","labels":["bug"],"depends_on":["proj"],"status":"done"}`,
	)

	writes := []struct {
		name  string
		write func() error
	}{
		{"an upsert of a new task", func() error {
			_, err := b.UpsertTask(map[string]any{"slug": "fresh", "kind": "task", "labels": bugLabels, "depends_on": []string{"proj"}})
			return err
		}},
		{"set-deps", func() error {
			_, err := b.SetDeps("member", []string{"proj"})
			return err
		}},
		{"a batch", func() error {
			_, err := b.UpsertTasksBatch([]map[string]any{{"slug": "fresh", "kind": "task", "labels": bugLabels, "depends_on": []string{"proj"}}})
			return err
		}},
	}
	for _, tt := range writes {
		err := tt.write()
		if err == nil || !strings.Contains(err.Error(), `"proj"`) || !strings.Contains(err.Error(), "member") || !strings.Contains(err.Error(), "set-deps") {
			t.Errorf("%s: error = %v; want a refusal naming proj, its member and set-deps", tt.name, err)
		}
	}
	if got, _, err := b.GetTask("fresh"); err != nil || got.Slug != "" {
		t.Errorf("GetTask(fresh) = %+v, %v; want no entry, since every refused write left the board as it was", got, err)
	}

	edited, err := b.UpsertTask(map[string]any{"slug": "stale", "body": "a later edit"})
	if err != nil || edited.Body != "a later edit" {
		t.Errorf("UpsertTask(stale) = %+v, %v; want the body edit to land, since its depends_on on proj is unchanged", edited, err)
	}
}
