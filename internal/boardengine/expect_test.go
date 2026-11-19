// expect_test.go unit-tests the entry revision check in expect.go through the board's exported writes.

package boardengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newExpectingBoard returns a board over a temp directory holding one draft task, alpha, with git skipped.
// The entry is a task, not the note an upsert makes by default, so a release changes its status.
func newExpectingBoard(t *testing.T) (*boardengine.Board, string) {
	t.Helper()
	boardPath := t.TempDir()
	board := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
	if _, err := board.UpsertTask(map[string]any{"slug": "alpha", "title": "Alpha", "kind": "task", "labels": bugLabels}); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	return board, boardPath
}

// entryRevision returns the revision Board.View gives the entry slug.
func entryRevision(t *testing.T, board *boardengine.Board, slug string) string {
	t.Helper()
	view, err := board.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	for _, entry := range view.Entries {
		if entry.Slug == slug {
			return entry.Revision
		}
	}
	t.Fatalf("View holds no entry %q", slug)
	return ""
}

// requireStale returns the *StaleEntryError err wraps, failing the test when it is another error or none.
func requireStale(t *testing.T, err error, wantMessage string) *boardengine.StaleEntryError {
	t.Helper()
	var stale *boardengine.StaleEntryError
	if !errors.As(err, &stale) {
		t.Fatalf("err = %v; want a *StaleEntryError", err)
	}
	if !strings.Contains(stale.Error(), wantMessage) || !strings.Contains(stale.Error(), "way forward: ") {
		t.Errorf("err = %q; want it to hold %q and a way forward", stale.Error(), wantMessage)
	}
	return stale
}

// TestExpectingRevision pins the revision check of a board made by Expecting, one row per branch:
// a current revision is saved, a stale or removed entry and an existing slug are refused with nothing written,
// and a revision read from a board the load migrations change is accepted.
func TestExpectingRevision(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	edit := map[string]any{"slug": "alpha", "title": "Changed"}

	t.Run("EditWithTheCurrentRevisionIsSaved", func(t *testing.T) {
		t.Parallel()
		board, _ := newExpectingBoard(t)
		revision := entryRevision(t, board, "alpha")
		if _, err := board.Expecting("alpha", revision).UpsertTaskReporting(edit); err != nil {
			t.Fatalf("UpsertTaskReporting: %v", err)
		}
		if task, _, _ := board.GetTask("alpha"); task.Title != "Changed" {
			t.Errorf("title = %q; want the saved edit", task.Title)
		}
	})

	t.Run("EditWithARevisionOlderByAStatusChangeIsRefusedAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		board, boardPath := newExpectingBoard(t)
		stale := entryRevision(t, board, "alpha")
		if _, err := board.SetStatusReleasingProject("alpha", nil); err != nil {
			t.Fatalf("release alpha: %v", err)
		}
		before := dataFiles(t, boardPath)
		_, err := board.Expecting("alpha", stale).UpsertTaskReporting(edit)
		refusal := requireStale(t, err, `entry "alpha" changed since the page read it`)
		if current := entryRevision(t, board, "alpha"); refusal.Fresh == nil || refusal.Fresh.Revision != current {
			t.Errorf("Fresh = %+v; want the current entry with revision %q", refusal.Fresh, current)
		}
		assertSameFiles(t, before, dataFiles(t, boardPath))
	})

	t.Run("RevisionReadFromAMigratedBoardIsAccepted", func(t *testing.T) {
		t.Parallel()
		boardPath := t.TempDir()
		seed := `{"version":1,"entries":[{"id":1,"slug":"alpha","title":"Alpha","kind":"task","labels":["bug"]}]}`
		if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(seed), 0o644); err != nil {
			t.Fatalf("seed board.json: %v", err)
		}
		board := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
		revision := entryRevision(t, board, "alpha")
		if _, err := board.Expecting("alpha", revision).UpsertTaskReporting(edit); err != nil {
			t.Errorf("UpsertTaskReporting with the migrated revision: %v", err)
		}
	})

	t.Run("EditOfARemovedEntryIsRefusedWithNoFreshEntry", func(t *testing.T) {
		t.Parallel()
		board, boardPath := newExpectingBoard(t)
		revision := entryRevision(t, board, "alpha")
		if err := board.RemoveTask("alpha"); err != nil {
			t.Fatalf("RemoveTask: %v", err)
		}
		before := dataFiles(t, boardPath)
		_, err := board.Expecting("alpha", revision).UpsertTaskReporting(edit)
		if refusal := requireStale(t, err, `entry "alpha" was removed since the page read it`); refusal.Fresh != nil {
			t.Errorf("Fresh = %+v; want nil for a removed entry", refusal.Fresh)
		}
		assertSameFiles(t, before, dataFiles(t, boardPath))
	})

	t.Run("CreateOfAnExistingSlugIsRefusedWithTheExistingEntry", func(t *testing.T) {
		t.Parallel()
		board, boardPath := newExpectingBoard(t)
		before := dataFiles(t, boardPath)
		_, err := board.Expecting("alpha", "").UpsertTaskReporting(map[string]any{"slug": "alpha", "title": "Again", "labels": bugLabels})
		refusal := requireStale(t, err, `entry "alpha" already exists`)
		if want := entryRevision(t, board, "alpha"); refusal.Fresh == nil || refusal.Fresh.Revision != want {
			t.Errorf("Fresh = %+v; want the existing entry with revision %q", refusal.Fresh, want)
		}
		assertSameFiles(t, before, dataFiles(t, boardPath))
	})

	t.Run("CreateOfAProjectSlugIsRefusedAsTakenNotAsStale", func(t *testing.T) {
		t.Parallel()
		board, boardPath := newExpectingBoard(t)
		if _, err := board.UpsertTask(map[string]any{"slug": "grouping", "title": "Grouping", "kind": "project", "labels": bugLabels}); err != nil {
			t.Fatalf("seed grouping: %v", err)
		}
		before := dataFiles(t, boardPath)
		_, err := board.Expecting("grouping", "").UpsertTaskReporting(map[string]any{"slug": "grouping", "title": "Again", "labels": bugLabels})
		var stale *boardengine.StaleEntryError
		if err == nil || errors.As(err, &stale) {
			t.Fatalf("err = %v; want a plain refusal, not a *StaleEntryError", err)
		}
		if !strings.Contains(err.Error(), `slug "grouping" is taken by a project`) || !strings.Contains(err.Error(), "way forward: ") {
			t.Errorf("err = %q; want it to name the project and a way forward", err)
		}
		assertSameFiles(t, before, dataFiles(t, boardPath))
	})

	t.Run("CreateOfANewSlugIsSaved", func(t *testing.T) {
		t.Parallel()
		board, _ := newExpectingBoard(t)
		if _, err := board.Expecting("beta", "").UpsertTaskReporting(map[string]any{"slug": "beta", "title": "Beta", "labels": bugLabels}); err != nil {
			t.Fatalf("UpsertTaskReporting: %v", err)
		}
		if _, found, _ := board.GetTask("beta"); !found {
			t.Errorf("beta not saved")
		}
	})
}
