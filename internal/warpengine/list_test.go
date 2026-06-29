//go:build integration

// list_test.go covers the git worktree list porcelain parser.

package warpengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// TestList covers the porcelain parser: a fresh repo yields exactly the main
// worktree, and additional worktrees appear after it with Main=false.
func TestList(t *testing.T) {
	t.Parallel()

	// TwoWorktrees first checks the single-worktree state (pre-add) then adds a
	// second worktree and verifies the expanded list. Both assertions run
	// sequentially on the same fixture to save one fixture build.
	t.Run("TwoWorktrees", func(t *testing.T) {
		t.Parallel()

		f := lyxtest.CopyHostHub(t)
		w := warpengine.New(warpengine.Config{})

		// Pre-add: single-worktree assertions (ported from the former SingleWorktree subtest).
		entries, err := w.List(f.Hub)
		if err != nil {
			t.Fatalf("pre-add List() error = %v; want nil", err)
		}
		if len(entries) != 1 {
			t.Fatalf("pre-add List() len = %d; want 1", len(entries))
		}
		e := entries[0]
		if !e.Main {
			t.Errorf("pre-add entries[0].Main = false; want true")
		}
		if e.Branch != "main" {
			t.Errorf("pre-add entries[0].Branch = %q; want %q", e.Branch, "main")
		}
		if e.Head == "" {
			t.Error(`pre-add entries[0].Head = ""; want non-empty`)
		}

		// Add a second worktree so the post-add check can verify list expansion.
		wtPath := filepath.Join(filepath.Dir(f.Hub), "wt1")
		lyxtest.MustRun(t, f.Hub, "git", "worktree", "add", wtPath)

		// Post-add: two-worktree assertions.
		entries, err = w.List(f.Hub)
		if err != nil {
			t.Fatalf("post-add List() error = %v; want nil", err)
		}
		if len(entries) != 2 {
			t.Fatalf("post-add List() len = %d; want 2", len(entries))
		}
		if !entries[0].Main {
			t.Errorf("entries[0].Main = false; want true")
		}
		// git may emit forward slashes on all platforms; normalize before comparing.
		gotPath := filepath.FromSlash(entries[0].Path)
		if gotPath != f.Hub {
			t.Errorf("entries[0].Path = %q; want %q", gotPath, f.Hub)
		}
		if entries[1].Main {
			t.Errorf("entries[1].Main = true; want false")
		}
	})
}
