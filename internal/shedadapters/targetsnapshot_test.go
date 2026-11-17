// targetsnapshot_test.go tests TargetSnapshot's ids, its overlay copies and its changed-path lists in both scopes.

package shedadapters

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeSnapshotFile writes content to rel under root, creating its directories.
func writeSnapshotFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v; want nil", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
	}
}

// newOverlaySnapshot returns an overlay snapshot over a temp worktree holding a.txt and dir/b.txt, with an empty cache.
func newOverlaySnapshot(t *testing.T, paths ...string) TargetSnapshot {
	t.Helper()
	root := t.TempDir()
	writeSnapshotFile(t, root, "a.txt", "a\n")
	writeSnapshotFile(t, root, "dir/b.txt", "b\n")
	if len(paths) == 0 {
		paths = []string{"a.txt", "dir"}
	}
	return TargetSnapshot{Scope: burlerengine.FixScopeOverlay, WorktreeRoot: root, Paths: paths, CacheDir: filepath.Join(t.TempDir(), "cache")}
}

// mustTake returns the id Take answers, failing the test on an error.
func mustTake(t *testing.T, s TargetSnapshot) string {
	t.Helper()
	id, err := s.Take()
	if err != nil {
		t.Fatalf("Take() = %v; want nil", err)
	}
	return id
}

// TestTargetSnapshot covers the overlay id, copy and changed-path list, and both source-scope answers.
func TestTargetSnapshot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("an overlay id follows the target bytes", func(t *testing.T) {
		t.Parallel()
		s := newOverlaySnapshot(t)
		first, err := s.ID()
		if err != nil {
			t.Fatalf("ID() = %v; want nil", err)
		}
		if again, _ := s.ID(); again != first {
			t.Errorf("ID() with no edit = %q; want %q", again, first)
		}
		writeSnapshotFile(t, s.WorktreeRoot, "dir/b.txt", "edited\n")
		if edited, _ := s.ID(); edited == first {
			t.Errorf("ID() after an edit = %q; want a new id", edited)
		}
	})

	t.Run("a missing target path is skipped", func(t *testing.T) {
		t.Parallel()
		with := newOverlaySnapshot(t, "a.txt", "gone.txt")
		without := newOverlaySnapshot(t, "a.txt")
		writeSnapshotFile(t, without.WorktreeRoot, "dir/b.txt", "b\n")
		withID, err := with.ID()
		if err != nil {
			t.Fatalf("ID() = %v; want nil", err)
		}
		withoutID, _ := without.ID()
		if withID != withoutID {
			t.Errorf("ID() with a missing path = %q; want %q, the id without it", withID, withoutID)
		}
	})

	t.Run("an overlay change list holds an edited, an added and a removed file", func(t *testing.T) {
		t.Parallel()
		s := newOverlaySnapshot(t)
		writeSnapshotFile(t, s.WorktreeRoot, "dir/keep.txt", "keep\n")
		id := mustTake(t, s)
		writeSnapshotFile(t, s.WorktreeRoot, "a.txt", "edited\n")
		writeSnapshotFile(t, s.WorktreeRoot, "dir/new.txt", "new\n")
		if err := os.Remove(filepath.Join(s.WorktreeRoot, "dir", "b.txt")); err != nil {
			t.Fatalf("Remove = %v; want nil", err)
		}

		got, ok := s.ChangedSince(id)

		if want := []string{"a.txt", "dir/b.txt", "dir/new.txt"}; !ok || !slices.Equal(got, want) {
			t.Errorf("ChangedSince(%q) = %v, %v; want %v, true", id, got, ok, want)
		}
	})

	t.Run("a second take keeps the first copy", func(t *testing.T) {
		t.Parallel()
		s := newOverlaySnapshot(t)
		id := mustTake(t, s)
		copyPath := filepath.Join(s.CacheDir, id, "a.txt")
		past := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(copyPath, past, past); err != nil {
			t.Fatalf("Chtimes = %v; want nil", err)
		}

		if again := mustTake(t, s); again != id {
			t.Fatalf("second Take() = %q; want %q", again, id)
		}

		info, err := os.Stat(copyPath)
		if err != nil || !info.ModTime().Equal(past) {
			t.Errorf("copy after the second Take = %v, %v; want the first copy's modification time %v", info, err, past)
		}
	})

	t.Run("a deleted cache gives false", func(t *testing.T) {
		t.Parallel()
		s := newOverlaySnapshot(t)
		id := mustTake(t, s)
		if err := os.RemoveAll(s.CacheDir); err != nil {
			t.Fatalf("RemoveAll = %v; want nil", err)
		}
		if got, ok := s.ChangedSince(id); ok {
			t.Errorf("ChangedSince(%q) = %v, true; want false", id, got)
		}
	})

	t.Run("source scope answers from the closures", func(t *testing.T) {
		t.Parallel()
		s := TargetSnapshot{
			Scope:        burlerengine.FixScopeSource,
			SourceTreeID: func() (string, error) { return "tree-now", nil },
			SourceChanges: func(fromID, toID string) ([]string, error) {
				return []string{fromID + ">" + toID}, nil
			},
		}
		if id, err := s.Take(); err != nil || id != "tree-now" {
			t.Errorf("Take() = %q, %v; want tree-now", id, err)
		}
		if got, ok := s.ChangedSince("tree-then"); !ok || !slices.Equal(got, []string{"tree-then>tree-now"}) {
			t.Errorf("ChangedSince() = %v, %v; want the closure's list, true", got, ok)
		}
	})

	t.Run("a failing source closure gives false", func(t *testing.T) {
		t.Parallel()
		s := TargetSnapshot{
			Scope:         burlerengine.FixScopeSource,
			SourceTreeID:  func() (string, error) { return "tree-now", nil },
			SourceChanges: func(string, string) ([]string, error) { return nil, errors.New("tree gone") },
		}
		if got, ok := s.ChangedSince("tree-then"); ok {
			t.Errorf("ChangedSince() = %v, true; want false", got)
		}
	})
}
