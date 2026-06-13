package worktree_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestListSingleWorktree tests that List returns exactly one entry for a fresh repo.
func TestListSingleWorktree(t *testing.T) {
	hub := newTestRepo(t)

	w := worktree.New(worktree.Config{})
	entries, err := w.List(hub)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if !entry.Main {
		t.Errorf("expected Main=true, got %v", entry.Main)
	}

	if entry.Branch != "main" {
		t.Errorf("expected Branch %q, got %q", "main", entry.Branch)
	}

	if entry.Head == "" {
		t.Errorf("expected non-empty Head")
	}
}

// TestListTwoWorktrees tests that List returns two entries in correct order.
func TestListTwoWorktrees(t *testing.T) {
	hub := newTestRepo(t)

	// Create a second worktree
	wt2Path := filepath.Join(filepath.Dir(hub), "wt2")
	mustRun(t, hub, "git", "worktree", "add", wt2Path)

	w := worktree.New(worktree.Config{})
	entries, err := w.List(hub)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// First entry should be main
	if !entries[0].Main {
		t.Errorf("expected entries[0].Main=true, got %v", entries[0].Main)
	}

	// Verify the first entry is the main checkout (its path matches the hub)
	if entries[0].Path != hub {
		t.Errorf("expected entries[0].Path=%q, got %q", hub, entries[0].Path)
	}

	// Second entry should not be main
	if entries[1].Main {
		t.Errorf("expected entries[1].Main=false, got %v", entries[1].Main)
	}
}
