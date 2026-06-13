package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveLinksIgnoresRegularFilesAndDirs tests that removeLinks leaves
// regular files and real subdirectories untouched.
func TestRemoveLinksIgnoresRegularFilesAndDirs(t *testing.T) {
	tempDir := t.TempDir()

	// Create a regular file
	regularFile := filepath.Join(tempDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create regular file: %v", err)
	}

	// Create a real subdirectory
	realDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	// Call removeLinks
	count, err := removeLinks(tempDir)

	// Should return 0 count and no error
	if err != nil {
		t.Fatalf("removeLinks failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}

	// Verify the regular file and directory still exist
	if _, err := os.Stat(regularFile); err != nil {
		t.Fatalf("regular file was removed: %v", err)
	}
	if _, err := os.Stat(realDir); err != nil {
		t.Fatalf("real directory was removed: %v", err)
	}
}

// TestRemoveLinksRemovesSymlinks tests that removeLinks removes symlinks
// and returns the correct count.
func TestRemoveLinksRemovesSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	// Create symlink targets
	target1 := filepath.Join(tempDir, "target1.txt")
	if err := os.WriteFile(target1, []byte("target1"), 0644); err != nil {
		t.Fatalf("failed to create target1: %v", err)
	}

	target2 := filepath.Join(tempDir, "target2.txt")
	if err := os.WriteFile(target2, []byte("target2"), 0644); err != nil {
		t.Fatalf("failed to create target2: %v", err)
	}

	// Create symlinks to these targets
	link1 := filepath.Join(tempDir, "link1")
	if err := os.Symlink(target1, link1); err != nil {
		t.Skipf("symlinks not permitted on this platform: %v", err)
	}

	link2 := filepath.Join(tempDir, "link2")
	if err := os.Symlink(target2, link2); err != nil {
		t.Skipf("symlinks not permitted on this platform: %v", err)
	}

	// Create a regular file for control
	regularFile := filepath.Join(tempDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("regular"), 0644); err != nil {
		t.Fatalf("failed to create regular file: %v", err)
	}

	// Call removeLinks
	count, err := removeLinks(tempDir)

	// Should return 2 count and no error
	if err != nil {
		t.Fatalf("removeLinks failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	// Verify the links are gone
	if _, err := os.Lstat(link1); err == nil {
		t.Fatalf("link1 still exists")
	}
	if _, err := os.Lstat(link2); err == nil {
		t.Fatalf("link2 still exists")
	}

	// Verify the targets still exist
	if _, err := os.Stat(target1); err != nil {
		t.Fatalf("target1 was removed: %v", err)
	}
	if _, err := os.Stat(target2); err != nil {
		t.Fatalf("target2 was removed: %v", err)
	}

	// Verify the regular file still exists
	if _, err := os.Stat(regularFile); err != nil {
		t.Fatalf("regular file was removed: %v", err)
	}
}

// TestRemoveLinksNonexistentDir tests that removeLinks handles a nonexistent
// directory gracefully.
func TestRemoveLinksNonexistentDir(t *testing.T) {
	nonexistentDir := "/nonexistent/dir/that/does/not/exist"

	count, err := removeLinks(nonexistentDir)

	// Should return 0 count and an error
	if err == nil {
		t.Fatalf("expected error for nonexistent directory, got nil")
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}
