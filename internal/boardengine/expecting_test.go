// expecting_test.go checks that a write made with an expected entry revision goes through only while it holds.

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

// expectingBoard returns a board over a new temp directory holding the task "one", and that directory.
func expectingBoard(t *testing.T) (*boardengine.Board, string) {
	t.Helper()
	dir := t.TempDir()
	board := boardengine.New(boardengine.Config{Path: dir, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
	if _, err := board.UpsertTaskReporting(map[string]any{"slug": "one", "title": "One", "kind": "task", "labels": []any{"bug"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return board, dir
}

// viewRevision returns the revision the board's view gives the entry slug.
func viewRevision(t *testing.T, board *boardengine.Board, slug string) string {
	t.Helper()
	view, err := board.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	return entryOf(t, view, slug).Revision
}

// directoryContent returns the bytes of every file under dir by its relative path.
func directoryContent(t *testing.T, dir string) map[string]string {
	t.Helper()
	content := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		content[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return content
}

// retitle upserts a new title for the entry slug through board.
func retitle(board *boardengine.Board, slug, title string) error {
	_, err := board.UpsertTaskReporting(map[string]any{"slug": slug, "title": title})
	return err
}

func TestExpectingRevision(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("CurrentRevisionSaves", func(t *testing.T) {
		t.Parallel()
		board, _ := expectingBoard(t)
		if err := retitle(board.Expecting("one", viewRevision(t, board, "one")), "one", "Edited"); err != nil {
			t.Fatalf("edit: %v", err)
		}
		if got, _, err := board.GetTask("one"); err != nil || got.Title != "Edited" {
			t.Errorf("title = %q, %v; want Edited", got.Title, err)
		}
	})

	t.Run("MigratedEntryRevisionSaves", func(t *testing.T) {
		t.Parallel()
		board, dir := expectingBoard(t)
		raw := `{"version":1,"entries":[{"id":1,"handle":"T1","slug":"unmarked","title":"U","kind":"task","labels":["bug"]}]}`
		if err := os.WriteFile(filepath.Join(dir, "board.json"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := retitle(board.Expecting("unmarked", viewRevision(t, board, "unmarked")), "unmarked", "Edited"); err != nil {
			t.Errorf("edit: %v", err)
		}
	})

	t.Run("NewSlugCreates", func(t *testing.T) {
		t.Parallel()
		board, _ := expectingBoard(t)
		if _, err := board.Expecting("two", "").UpsertTaskReporting(map[string]any{"slug": "two", "title": "Two", "kind": "task", "labels": []any{"bug"}}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, found, err := board.GetTask("two"); err != nil || !found {
			t.Errorf("GetTask(two) = %v, %v; want it saved", found, err)
		}
	})

	refusals := []struct {
		name string
		// change moves the board on after the revision was read.
		change func(board *boardengine.Board) error
		// expect returns the revision the write expects, given the one read before change.
		expect      func(read string) string
		wantRemoved bool
		wantMessage string
	}{
		{
			name:        "OlderByAnEdit",
			change:      func(board *boardengine.Board) error { return retitle(board, "one", "Moved") },
			expect:      func(read string) string { return read },
			wantMessage: `entry "one" changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again`,
		},
		{
			name: "OlderByAStatusAlone",
			change: func(board *boardengine.Board) error {
				_, err := board.SetStatusReleasingProject("one", nil)
				return err
			},
			expect:      func(read string) string { return read },
			wantMessage: `entry "one" changed since the page read it, so nothing was written: way forward: compare the fresh entry and send the edit again`,
		},
		{
			name:        "Removed",
			change:      func(board *boardengine.Board) error { return board.RemoveTask("one") },
			expect:      func(read string) string { return read },
			wantRemoved: true,
			wantMessage: `entry "one" was removed since the page read it, so nothing was written: way forward: reload the board and act on what it holds now`,
		},
		{
			name:        "CreateOfAnExistingSlug",
			change:      func(*boardengine.Board) error { return nil },
			expect:      func(string) string { return "" },
			wantMessage: `entry "one" already exists, so nothing was written: way forward: pick another slug, or edit the existing entry`,
		},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board, dir := expectingBoard(t)
			read := viewRevision(t, board, "one")
			if err := tt.change(board); err != nil {
				t.Fatalf("change: %v", err)
			}
			before := directoryContent(t, dir)

			err := retitle(board.Expecting("one", tt.expect(read)), "one", "Refused")

			var stale *boardengine.StaleEntryError
			if !errors.As(err, &stale) {
				t.Fatalf("write error = %v; want a *StaleEntryError", err)
			}
			if err.Error() != tt.wantMessage {
				t.Errorf("message = %q; want %q", err.Error(), tt.wantMessage)
			}
			if tt.wantRemoved {
				if stale.Fresh != nil {
					t.Errorf("Fresh = %+v; want nil", stale.Fresh)
				}
			} else if stale.Fresh == nil || stale.Fresh.Revision != viewRevision(t, board, "one") {
				t.Errorf("Fresh = %+v; want the current entry", stale.Fresh)
			}
			if after := directoryContent(t, dir); !maps.Equal(before, after) {
				t.Errorf("board directory changed by a refused write")
			}
		})
	}
}
