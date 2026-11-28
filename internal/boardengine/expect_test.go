// expect_test.go covers the revision check of expect.go through the board's exported writes.

package boardengine_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// boardTree returns the content of every file under dir, by relative path.
func boardTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(path, dir)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

// revisionInView is the revision Board.View gives the entry slug.
func revisionInView(t *testing.T, b *boardengine.Board, slug string) string {
	t.Helper()
	view, err := b.View()
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	entry, found := viewEntries(view)[slug]
	if !found {
		t.Fatalf("View holds no entry %q", slug)
	}
	return entry.Revision
}

func TestExpectingRevision(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	edit := func(b *boardengine.Board, slug, revision string) error {
		_, err := b.Expecting(slug, revision).UpsertTaskReporting(map[string]any{"slug": slug, "title": "Edited"})
		return err
	}
	create := func(b *boardengine.Board, slug string) error {
		_, err := b.Expecting(slug, "").UpsertTaskReporting(map[string]any{"slug": slug, "title": "Edited", "kind": "task", "labels": bugLabels})
		return err
	}
	tests := []struct {
		name string
		// seed, when set, is the board.json the board starts from; otherwise the board starts with the task "a".
		seed string
		// write reads the entry's revision, changes the board as another writer would and then writes against it.
		write func(t *testing.T, b *boardengine.Board, dir string) (error, map[string]string)
		// wantRefusal is empty for a write that is saved.
		wantRefusal string
		wantFresh   bool
	}{
		{
			name: "CurrentRevisionIsSaved",
			write: func(t *testing.T, b *boardengine.Board, dir string) (error, map[string]string) {
				return edit(b, "a", revisionInView(t, b, "a")), nil
			},
		},
		{
			name: "OlderByStatusAloneIsRefused",
			write: func(t *testing.T, b *boardengine.Board, dir string) (error, map[string]string) {
				older := revisionInView(t, b, "a")
				if _, err := b.SetStatusReleasingProject("a", nil); err != nil {
					t.Fatalf("release: %v", err)
				}
				before := boardTree(t, dir)
				return edit(b, "a", older), before
			},
			wantRefusal: `entry "a" changed since the page read it`,
			wantFresh:   true,
		},
		{
			name: "MigratedEntryRevisionIsAccepted",
			seed: `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"task","labels":["bug"]}]}`,
			write: func(t *testing.T, b *boardengine.Board, dir string) (error, map[string]string) {
				return edit(b, "a", revisionInView(t, b, "a")), nil
			},
		},
		{
			name: "RemovedEntryIsRefused",
			write: func(t *testing.T, b *boardengine.Board, dir string) (error, map[string]string) {
				older := revisionInView(t, b, "a")
				if err := b.RemoveTask("a"); err != nil {
					t.Fatalf("remove: %v", err)
				}
				before := boardTree(t, dir)
				return edit(b, "a", older), before
			},
			wantRefusal: `entry "a" was removed since the page read it`,
		},
		{
			name: "CreateOfAnExistingSlugIsRefused",
			write: func(t *testing.T, b *boardengine.Board, dir string) (error, map[string]string) {
				before := boardTree(t, dir)
				return create(b, "a"), before
			},
			wantRefusal: `entry "a" already exists`,
			wantFresh:   true,
		},
		{
			name: "CreateOfANewSlugIsSaved",
			write: func(t *testing.T, b *boardengine.Board, dir string) (error, map[string]string) {
				return create(b, "new"), nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			b := boardengine.New(boardengine.Config{Path: dir, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
			if tt.seed != "" {
				if err := os.WriteFile(filepath.Join(dir, "board.json"), []byte(tt.seed), 0o644); err != nil {
					t.Fatalf("seed board.json: %v", err)
				}
			} else if _, err := b.UpsertTask(map[string]any{"slug": "a", "title": "A", "kind": "task", "labels": bugLabels}); err != nil {
				t.Fatalf("seed: %v", err)
			}

			err, before := tt.write(t, b, dir)

			if tt.wantRefusal == "" {
				if err != nil {
					t.Fatalf("write = %v; want it saved", err)
				}
				saved := 0
				tasks, err := b.ListTasksFull()
				if err != nil {
					t.Fatalf("ListTasksFull: %v", err)
				}
				for _, task := range tasks {
					if task.Title == "Edited" {
						saved++
					}
				}
				if saved != 1 {
					t.Errorf("entries titled Edited = %d; want 1", saved)
				}
				return
			}
			var stale *boardengine.StaleEntryError
			if !errors.As(err, &stale) || !strings.Contains(err.Error(), tt.wantRefusal) {
				t.Fatalf("write = %v; want a *StaleEntryError saying %q", err, tt.wantRefusal)
			}
			if tt.wantFresh {
				if stale.Fresh == nil || stale.Fresh.Revision != revisionInView(t, b, "a") {
					t.Errorf("Fresh = %+v; want the entry at its current revision", stale.Fresh)
				}
			} else if stale.Fresh != nil {
				t.Errorf("Fresh = %+v; want none", stale.Fresh)
			}
			if after := boardTree(t, dir); !maps.Equal(before, after) {
				t.Errorf("board directory changed by a refused write")
			}
		})
	}
}
