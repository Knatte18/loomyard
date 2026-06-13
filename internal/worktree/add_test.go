package worktree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestAddHappyPath tests the happy path: repo with remote, Add succeeds.
func TestAddHappyPath(t *testing.T) {
	hub := newTestRepo(t)
	addRemote(t, hub)

	w := worktree.New(worktree.Config{})
	result, err := w.Add(hub, "my-task")

	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if result.Branch != "my-task" {
		t.Errorf("expected Branch %q, got %q", "my-task", result.Branch)
	}

	expectedPath := filepath.Join(filepath.Dir(hub), "my-task")
	if result.Path != expectedPath {
		t.Errorf("expected Path %q, got %q", expectedPath, result.Path)
	}

	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("worktree directory does not exist: %v", err)
	}

	if !result.Pushed {
		t.Errorf("expected Pushed=true, got %v", result.Pushed)
	}
}

// TestAddBranchPrefix tests that BranchPrefix is prepended to the slug.
func TestAddBranchPrefix(t *testing.T) {
	hub := newTestRepo(t)
	addRemote(t, hub)

	w := worktree.New(worktree.Config{BranchPrefix: "hanf/"})
	result, err := w.Add(hub, "my-task")

	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if result.Branch != "hanf/my-task" {
		t.Errorf("expected Branch %q, got %q", "hanf/my-task", result.Branch)
	}

	// But the slug is still just "my-task" (not prefixed in the path)
	expectedPath := filepath.Join(filepath.Dir(hub), "my-task")
	if result.Path != expectedPath {
		t.Errorf("expected Path %q, got %q", expectedPath, result.Path)
	}

	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("worktree directory does not exist: %v", err)
	}
}

// TestAddDirtySource tests that Add fails if the source has uncommitted changes.
func TestAddDirtySource(t *testing.T) {
	hub := newTestRepo(t)
	addRemote(t, hub)

	// Modify the tracked README file without committing
	readmeFile := filepath.Join(hub, "README")
	if err := os.WriteFile(readmeFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("failed to modify README: %v", err)
	}

	w := worktree.New(worktree.Config{})
	result, err := w.Add(hub, "my-task")

	if err == nil {
		t.Fatalf("expected error for dirty source, got nil")
	}

	// Verify no sibling worktree was created
	targetPath := filepath.Join(filepath.Dir(hub), "my-task")
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Errorf("worktree directory should not exist, but does")
	}

	// Verify the error message mentions uncommitted changes
	errMsg := err.Error()
	if errMsg != "source worktree has uncommitted changes" {
		t.Errorf("expected error message about uncommitted changes, got: %s", errMsg)
	}

	_ = result // silence unused warning
}

// TestAddBranchExists tests that Add fails if the branch already exists.
func TestAddBranchExists(t *testing.T) {
	hub := newTestRepo(t)
	addRemote(t, hub)

	// Create the branch first
	mustRun(t, hub, "git", "branch", "my-task")

	w := worktree.New(worktree.Config{})
	result, err := w.Add(hub, "my-task")

	if err == nil {
		t.Fatalf("expected error for existing branch, got nil")
	}

	// Verify the error message mentions the branch
	errMsg := err.Error()
	if errMsg != `branch "my-task" already exists` {
		t.Errorf("expected error about existing branch, got: %s", errMsg)
	}

	_ = result // silence unused warning
}

// TestAddTargetDirExists tests that Add fails if the target directory already exists.
func TestAddTargetDirExists(t *testing.T) {
	hub := newTestRepo(t)
	addRemote(t, hub)

	// Create the target directory first
	targetPath := filepath.Join(filepath.Dir(hub), "my-task")
	if err := os.Mkdir(targetPath, 0755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	w := worktree.New(worktree.Config{})
	result, err := w.Add(hub, "my-task")

	if err == nil {
		t.Fatalf("expected error for existing target directory, got nil")
	}

	// Verify the error message mentions the target directory
	errMsg := err.Error()
	if !strings.Contains(errMsg, "already exists") {
		t.Errorf("expected error about existing target directory, got: %s", errMsg)
	}

	_ = result // silence unused warning
}

// TestAddNoRemote tests that Add fails if no remote is configured.
func TestAddNoRemote(t *testing.T) {
	hub := newTestRepo(t)
	// Intentionally don't call addRemote

	w := worktree.New(worktree.Config{})
	result, err := w.Add(hub, "my-task")

	if err == nil {
		t.Fatalf("expected error for no remote, got nil")
	}

	// Verify the error message mentions remote
	errMsg := err.Error()
	if errMsg != "no remote configured" {
		t.Errorf("expected error about no remote, got: %s", errMsg)
	}

	// Verify no sibling worktree was created (precheck, so no dir should exist)
	targetPath := filepath.Join(filepath.Dir(hub), "my-task")
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Errorf("worktree directory should not exist, but does")
	}

	_ = result // silence unused warning
}
