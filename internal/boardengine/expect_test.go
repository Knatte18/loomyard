// expect_test.go tests the writes of a board that expects an entry revision.

package boardengine_test

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// viewedEntry is the entry slug as board's view shows it now.
func viewedEntry(t *testing.T, board *boardengine.Board, slug string) boardengine.ViewEntry {
	t.Helper()
	view, err := board.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	return viewEntry(t, view, slug)
}

func retitle(board *boardengine.Board, slug string) error {
	_, err := board.UpsertTaskReporting(map[string]any{"slug": slug, "title": "Retitled", "labels": bugLabels})
	return err
}

// requireStale returns the stale refusal err must be.
func requireStale(t *testing.T, err error) *boardengine.StaleEntryError {
	t.Helper()
	var stale *boardengine.StaleEntryError
	if !errors.As(err, &stale) {
		t.Fatalf("error = %v; want a *StaleEntryError", err)
	}
	return stale
}

func TestExpectingRevision(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, board *boardengine.Board, dir string)
	}{
		{"CurrentRevisionIsSaved", func(t *testing.T, board *boardengine.Board, _ string) {
			if err := retitle(board.Expecting("a", viewedEntry(t, board, "a").Revision), "a"); err != nil {
				t.Fatalf("edit: %v", err)
			}
			if got := viewedEntry(t, board, "a").Title; got != "Retitled" {
				t.Errorf("title = %q; want Retitled", got)
			}
		}},
		{"RevisionOlderByAStatusChangeIsRefusedAndWritesNothing", func(t *testing.T, board *boardengine.Board, dir string) {
			old := viewedEntry(t, board, "a").Revision
			if _, err := board.SetStatusReleasingProject("a", nil); err != nil {
				t.Fatalf("release: %v", err)
			}
			current := viewedEntry(t, board, "a").Revision
			before := dataFiles(t, dir)
			stale := requireStale(t, retitle(board.Expecting("a", old), "a"))
			if stale.Removed || stale.Create || stale.Fresh == nil || stale.Fresh.Revision != current {
				t.Errorf("refusal = %+v; want the current entry", stale)
			}
			assertSameFiles(t, before, dataFiles(t, dir))
		}},
		{"RemovedEntryIsRefusedWithNoEntry", func(t *testing.T, board *boardengine.Board, _ string) {
			old := viewedEntry(t, board, "a").Revision
			if err := board.RemoveTask("a"); err != nil {
				t.Fatalf("remove: %v", err)
			}
			if stale := requireStale(t, retitle(board.Expecting("a", old), "a")); !stale.Removed || stale.Fresh != nil {
				t.Errorf("refusal = %+v; want removed with no entry", stale)
			}
		}},
		{"CreateOfAnExistingSlugIsRefusedWithTheEntry", func(t *testing.T, board *boardengine.Board, _ string) {
			stale := requireStale(t, retitle(board.Expecting("a", ""), "a"))
			if !stale.Create || stale.Fresh == nil || stale.Fresh.Title != "A" {
				t.Errorf("refusal = %+v; want the existing entry", stale)
			}
		}},
		{"CreateOfANewSlugIsSaved", func(t *testing.T, board *boardengine.Board, _ string) {
			if err := retitle(board.Expecting("b", ""), "b"); err != nil {
				t.Fatalf("create: %v", err)
			}
			if got := viewedEntry(t, board, "b").Title; got != "Retitled" {
				t.Errorf("title = %q; want Retitled", got)
			}
		}},
		{"RevisionOfAnUnmigratedEntryIsAccepted", func(t *testing.T, board *boardengine.Board, dir string) {
			seedBoardJSON(t, dir, `{"id":0,"slug":"a","title":"A","kind":"task","labels":["bug"],"depends_on":[],"brief":"","body":""}`)
			if err := retitle(board.Expecting("a", viewedEntry(t, board, "a").Revision), "a"); err != nil {
				t.Errorf("edit: %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			board := boardengine.New(boardengine.Config{Path: dir, Readme: "Home.md", DesignPrefix: "design-", Types: testTypes, Labels: testLabels, SkipGit: true})
			// A task, so the seed is a draft that a release changes.
			if _, err := board.UpsertTask(map[string]any{"slug": "a", "title": "A", "kind": boardengine.KindTask, "labels": bugLabels}); err != nil {
				t.Fatalf("seed: %v", err)
			}
			tt.run(t, board, dir)
		})
	}
}
