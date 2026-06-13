package worktree_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestRemoveHappyPath tests removing a clean worktree.
func TestRemoveHappyPath(t *testing.T) {
	hub := newTestRepo(t)
	slug := "task1"

	// Create a worktree
	targetPath := filepath.Join(filepath.Dir(hub), slug)
	mustRun(t, hub, "git", "worktree", "add", targetPath)

	// Verify it exists
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("worktree should exist before removal: %v", err)
	}

	w := worktree.New(worktree.Config{})
	result, err := w.Remove(hub, slug, false)

	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if result.LinksRemoved != 0 {
		t.Errorf("expected LinksRemoved=0, got %d", result.LinksRemoved)
	}

	// Verify the directory no longer exists
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("worktree should not exist after removal")
	}
}

// TestRemoveDirtyWithoutForce tests that Remove fails on dirty worktree without force.
func TestRemoveDirtyWithoutForce(t *testing.T) {
	hub := newTestRepo(t)
	slug := "task1"

	// Create a worktree
	targetPath := filepath.Join(filepath.Dir(hub), slug)
	mustRun(t, hub, "git", "worktree", "add", targetPath)

	// Create an untracked file in the worktree
	untrackedFile := filepath.Join(targetPath, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
		t.Fatalf("failed to create untracked file: %v", err)
	}

	w := worktree.New(worktree.Config{})
	result, err := w.Remove(hub, slug, false)

	if err == nil {
		t.Fatalf("expected error for dirty worktree, got nil")
	}

	// Verify the directory still exists
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("worktree should still exist after failed removal: %v", err)
	}

	_ = result // silence unused warning
}

// TestRemoveDirtyWithForce tests that Remove succeeds with force despite dirty worktree.
func TestRemoveDirtyWithForce(t *testing.T) {
	hub := newTestRepo(t)
	slug := "task1"

	// Create a worktree
	targetPath := filepath.Join(filepath.Dir(hub), slug)
	mustRun(t, hub, "git", "worktree", "add", targetPath)

	// Create an untracked file in the worktree
	untrackedFile := filepath.Join(targetPath, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
		t.Fatalf("failed to create untracked file: %v", err)
	}

	w := worktree.New(worktree.Config{})
	result, err := w.Remove(hub, slug, true)

	if err != nil {
		t.Fatalf("Remove with force failed: %v", err)
	}

	if result.LinksRemoved < 0 {
		t.Errorf("expected LinksRemoved >= 0, got %d", result.LinksRemoved)
	}

	// Verify the directory no longer exists
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("worktree should not exist after removal with force")
	}
}

// TestRemoveNonexistentSlug tests that Remove fails for a non-existent slug.
func TestRemoveNonexistentSlug(t *testing.T) {
	hub := newTestRepo(t)

	w := worktree.New(worktree.Config{})
	result, err := w.Remove(hub, "ghost", false)

	if err == nil {
		t.Fatalf("expected error for non-existent slug, got nil")
	}

	_ = result // silence unused warning
}

// TestRemoveWithSymlink tests that LinksRemoved > 0 when a symlink is present.
// This test gracefully skips on platforms where symlinks are not supported.
func TestRemoveWithSymlink(t *testing.T) {
	hub := newTestRepo(t)
	slug := "task1"

	// Create a worktree
	targetPath := filepath.Join(filepath.Dir(hub), slug)
	mustRun(t, hub, "git", "worktree", "add", targetPath)

	// Attempt to create a symlink inside the worktree
	symlinkPath := filepath.Join(targetPath, "my-symlink")
	targetFile := filepath.Join(targetPath, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target"), 0644); err != nil {
		t.Fatalf("failed to create symlink target: %v", err)
	}

	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Skipf("symlinks not permitted on this platform: %v", err)
	}

	w := worktree.New(worktree.Config{})
	result, err := w.Remove(hub, slug, false)

	if err != nil {
		t.Fatalf("Remove with symlink failed: %v", err)
	}

	if result.LinksRemoved < 1 {
		t.Errorf("expected LinksRemoved >= 1 when symlink present, got %d", result.LinksRemoved)
	}

	// Verify the directory no longer exists
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("worktree should not exist after removal")
	}
}
