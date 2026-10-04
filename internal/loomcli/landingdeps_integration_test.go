//go:build integration

// landingdeps_integration_test.go drives landingDeps' MarkTaskDone closure against a real hub built
// by internal/hubforge, so the board config load, the hub-board path and the status write all run
// for real rather than through a stub.

package loomcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// markDoneFixture builds a hub with one pair and returns landingDeps' MarkTaskDone closure for that
// pair, plus the hub-board handle and the pair's slug.
func markDoneFixture(t *testing.T) (markDone func() error, board *boardengine.Board, slug string) {
	t.Helper()
	markDone, board, slug, _ = markDoneFixtureDir(t)
	return markDone, board, slug
}

// markDoneFixtureDir is markDoneFixture that also returns the hub board directory.
func markDoneFixtureDir(t *testing.T) (markDone func() error, board *boardengine.Board, slug, boardDir string) {
	t.Helper()
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Setenv("BOARD_SKIP_PUSH", "1")

	hub := hubforge.NewHub(t, ".")
	slug = "markdone"
	hubforge.AddPair(t, hub, slug)

	location, err := lyxcwd.ResolveWorktree(hub.PairWarpWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}

	deps := landingDeps(location, websterengine.Geometry{}, "task", "https://example.com/o.git", "main",
		true, func() error { return nil }, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{})

	bc, err := boardengine.LoadConfig(location.AnchorPath(), "board")
	if err != nil {
		t.Fatalf("LoadConfig error = %v; want nil", err)
	}
	bc.Path = fabricengine.BoardDir(location.HubPath)
	if _, err := os.Stat(bc.Path); err != nil {
		t.Fatalf("hub board dir %s: %v", bc.Path, err)
	}
	return deps.MarkTaskDone, boardengine.New(boardengine.ApplySkipEnv(bc)), slug, bc.Path
}

func TestLandingDeps_MarkTaskDone_SetsStatusDone(t *testing.T) {
	markDone, board, slug := markDoneFixture(t)

	if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": "Mark done", "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("UpsertTask error = %v; want nil", err)
	}
	if err := markDone(); err != nil {
		t.Fatalf("MarkTaskDone() error = %v; want nil", err)
	}

	task, found, err := board.GetTask(slug)
	if err != nil || !found {
		t.Fatalf("GetTask(%q) = found %v, err %v; want found", slug, found, err)
	}
	if task.Status == nil || *task.Status != "done" {
		t.Errorf("task status = %v; want done", task.Status)
	}
}

func TestLandingDeps_MarkTaskDone_UnknownSlugIsError(t *testing.T) {
	markDone, _, _ := markDoneFixture(t)

	if err := markDone(); err == nil {
		t.Error("MarkTaskDone() error = nil for a slug with no board task; want an error")
	}
}

// TestLandingDeps_MarkTaskDone_MigratesLegacyTasksJSON seeds the hub board with a legacy tasks.json holding the pair's slug and checks MarkTaskDone leaves that entry done in board.json.
func TestLandingDeps_MarkTaskDone_MigratesLegacyTasksJSON(t *testing.T) {
	markDone, _, slug, boardDir := markDoneFixtureDir(t)

	legacy := `[{"id":0,"slug":"` + slug + `","title":"Mark done","depends_on":[],"isolated":false,"deferred":false,"brief":"","body":""}]`
	if err := os.WriteFile(filepath.Join(boardDir, "tasks.json"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write tasks.json: %v", err)
	}
	if err := markDone(); err != nil {
		t.Fatalf("MarkTaskDone() error = %v; want nil", err)
	}

	raw, err := os.ReadFile(filepath.Join(boardDir, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	var file struct {
		Entries []struct {
			Slug   string  `json:"slug"`
			Status *string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal board.json: %v", err)
	}
	if len(file.Entries) != 1 || file.Entries[0].Slug != slug || file.Entries[0].Status == nil || *file.Entries[0].Status != "done" {
		t.Errorf("board.json entries = %+v; want %q done", file.Entries, slug)
	}
}
