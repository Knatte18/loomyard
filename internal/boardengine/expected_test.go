// expected_test.go covers the writes held to an entry revision, and their refusals.

package boardengine_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// boardFiles returns the bytes of every file below dir, keyed by path.
func boardFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	return files
}

// viewedRevision returns the revision the board's view gives the entry slug.
func viewedRevision(t *testing.T, board *boardengine.Board, slug string) string {
	t.Helper()
	view, err := board.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	return viewEntryBySlug(t, view, slug).Revision
}

// TestExpectingRevision pins the revision check of the write section through the board's exported writes.
func TestExpectingRevision(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// seeded returns a board holding the task "alpha", and its directory.
	seeded := func(t *testing.T) (*boardengine.Board, string) {
		t.Helper()
		dir := t.TempDir()
		board := boardengine.New(boardengine.Config{Path: dir, Readme: "README.md", DesignPrefix: "design-", Types: testTypes, Labels: testLabels, SkipGit: true})
		if _, err := board.UpsertTask(map[string]any{"slug": "alpha", "title": "Alpha", "kind": boardengine.KindTask, "labels": bugLabels}); err != nil {
			t.Fatalf("seed alpha: %v", err)
		}
		return board, dir
	}

	tests := []struct {
		name string
		// prepare changes the seeded board and returns the slug and revision the write expects.
		prepare func(t *testing.T, board *boardengine.Board) (slug, revision string)
		// refusal is empty for a write that is saved.
		refusal     string
		wantRemoved bool
		wantFresh   bool
	}{
		{
			name: "CurrentRevisionIsSaved",
			prepare: func(t *testing.T, board *boardengine.Board) (string, string) {
				return "alpha", viewedRevision(t, board, "alpha")
			},
		},
		{
			name: "RevisionOlderByAStatusChangeIsRefused",
			prepare: func(t *testing.T, board *boardengine.Board) (string, string) {
				revision := viewedRevision(t, board, "alpha")
				if _, err := board.SetStatusReleasingProject("alpha", nil); err != nil {
					t.Fatalf("release alpha: %v", err)
				}
				return "alpha", revision
			},
			refusal:   `entry "alpha" changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again`,
			wantFresh: true,
		},
		{
			name: "RemovedEntryIsRefused",
			prepare: func(t *testing.T, board *boardengine.Board) (string, string) {
				revision := viewedRevision(t, board, "alpha")
				if err := board.RemoveTask("alpha"); err != nil {
					t.Fatalf("remove alpha: %v", err)
				}
				return "alpha", revision
			},
			refusal:     `entry "alpha" was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now`,
			wantRemoved: true,
		},
		{
			name: "CreateOfAnExistingSlugIsRefused",
			prepare: func(*testing.T, *boardengine.Board) (string, string) {
				return "alpha", ""
			},
			refusal:   `entry "alpha" already exists, so nothing was written: way forward: pick another slug, or edit the existing entry`,
			wantFresh: true,
		},
		{
			name: "CreateOfANewSlugIsSaved",
			prepare: func(*testing.T, *boardengine.Board) (string, string) {
				return "beta", ""
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board, dir := seeded(t)
			slug, revision := tt.prepare(t, board)
			before := boardFiles(t, dir)

			_, err := board.Expecting(slug, revision).UpsertTaskReporting(map[string]any{"slug": slug, "title": "Edited", "labels": bugLabels})

			if tt.refusal == "" {
				if err != nil {
					t.Fatalf("UpsertTaskReporting: %v", err)
				}
				if task, _, err := board.GetTask(slug); err != nil || task.Title != "Edited" {
					t.Errorf("GetTask(%s) title = %q, %v; want Edited", slug, task.Title, err)
				}
				return
			}
			var stale *boardengine.StaleEntryError
			if !errors.As(err, &stale) {
				t.Fatalf("UpsertTaskReporting error = %v; want a *StaleEntryError", err)
			}
			if err.Error() != tt.refusal || stale.Removed != tt.wantRemoved {
				t.Errorf("refusal = %q, removed %v; want %q, %v", err, stale.Removed, tt.refusal, tt.wantRemoved)
			}
			if !tt.wantFresh {
				if stale.Fresh != nil {
					t.Errorf("Fresh = %+v; want nil", stale.Fresh)
				}
			} else if want := viewedRevision(t, board, slug); stale.Fresh == nil || stale.Fresh.Revision != want {
				t.Errorf("Fresh = %+v; want the entry at revision %s", stale.Fresh, want)
			}
			if after := boardFiles(t, dir); !maps.Equal(before, after) {
				t.Errorf("the refused write changed the board directory:\nbefore %q\nafter %q", before, after)
			}
		})
	}

	t.Run("MigratedEntryRevisionIsAccepted", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		data := []byte(`{"version":1,"entries":[{"id":1,"handle":"T1","slug":"loose","title":"Loose","kind":"task","labels":["bug"],"issues":[],"depends_on":[],"isolated":false,"brief":"","body":""}]}`)
		if err := os.WriteFile(filepath.Join(dir, "board.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		board := boardengine.New(boardengine.Config{Path: dir, Readme: "README.md", DesignPrefix: "design-", Types: testTypes, Labels: testLabels, SkipGit: true})
		revision := viewedRevision(t, board, "loose")
		if _, err := board.Expecting("loose", revision).UpsertTaskReporting(map[string]any{"slug": "loose", "title": "Edited"}); err != nil {
			t.Errorf("UpsertTaskReporting at the viewed revision: %v", err)
		}
	})
}
