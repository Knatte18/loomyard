//go:build tier1

// wiring_darn_integration_test.go drives commitDarnDiscussion over a real fabric pair,
// so the board entry snapshot is committed with the discussion directory, and a failed snapshot leaves the directory uncommitted.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestCommitDarnDiscussion_Real covers the darn CommitDiscussion seam's order: the snapshot is written before the commit and lands in it,
// and a board that holds no entry for the task fails the seam with the discussion directory left uncommitted.
// It sets the board skip variables, which are process-global, so it does not call t.Parallel.
func TestCommitDarnDiscussion_Real(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Setenv("BOARD_SKIP_PUSH", "1")

	newPair := func(t *testing.T, slug string) (*hubforge.Hub, *lyxcwd.Location) {
		t.Helper()
		hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		boardConfig := configengine.ConfigFile(hub.BoardDir(), "board")
		if err := os.MkdirAll(filepath.Dir(boardConfig), 0o755); err != nil {
			t.Fatalf("mkdir board config dir: %v", err)
		}
		if err := os.WriteFile(boardConfig, []byte(boardengine.ConfigTemplate()), 0o644); err != nil {
			t.Fatalf("write board config: %v", err)
		}
		hubforge.AddPair(t, hub, slug)
		location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree error = %v; want nil", err)
		}
		writeTestFile(t, loomengine.DiscussionDecisionRecord(location), "# Record\n")
		return hub, location
	}

	t.Run("the snapshot is committed with the discussion directory", func(t *testing.T) {
		const slug = "snapshotcommit"
		hub, location := newPair(t, slug)
		board, err := boardengine.OpenHub(location.HubPath)
		if err != nil {
			t.Fatalf("OpenHub() error = %v; want nil", err)
		}
		if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": "Snapshot commit", "kind": "task", "labels": []string{"bug"}, "brief": "Commit it.", "body": "## Idea\n\nCommit it."}); err != nil {
			t.Fatalf("UpsertTask() error = %v; want nil", err)
		}

		recordsSibling := hub.PairRecordsSibling(slug)
		before := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD")
		if err := commitDarnDiscussion(location); err != nil {
			t.Fatalf("commitDarnDiscussion() = %v; want nil", err)
		}
		if after := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD"); atoiOrFail(t, after) != atoiOrFail(t, before)+1 {
			t.Fatalf("records commit count %s -> %s; want exactly one commit", strings.TrimSpace(before), strings.TrimSpace(after))
		}
		changed := gitkit.Git(t, recordsSibling, "show", "--no-renames", "--name-only", "--format=", "HEAD")
		for _, want := range []string{"board-entry.md", "decision-record.md"} {
			if !strings.Contains(changed, want) {
				t.Errorf("commit changed %q; want it to hold %s", changed, want)
			}
		}
		if status := gitkit.Git(t, recordsSibling, "status", "--porcelain"); strings.Contains(status, "board-entry.md") {
			t.Errorf("git status = %q; want the snapshot committed", status)
		}
	})

	t.Run("a board without the task's entry leaves the discussion directory uncommitted", func(t *testing.T) {
		const slug = "snapshotmissing"
		hub, location := newPair(t, slug)

		recordsSibling := hub.PairRecordsSibling(slug)
		before := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD")
		if err := commitDarnDiscussion(location); err == nil || !strings.Contains(err.Error(), "lyx board upsert") {
			t.Errorf("commitDarnDiscussion() = %v; want a refusal naming lyx board upsert", err)
		}
		if after := gitkit.Git(t, recordsSibling, "rev-list", "--count", "HEAD"); after != before {
			t.Errorf("records commit count %s -> %s; want no commit", strings.TrimSpace(before), strings.TrimSpace(after))
		}
		if status := gitkit.Git(t, recordsSibling, "status", "--porcelain"); !strings.Contains(status, "?? "+filepath.ToSlash(loomengine.DiscussionDirRel())+"/") {
			t.Errorf("git status = %q; want the discussion directory left untracked", status)
		}
	})
}
