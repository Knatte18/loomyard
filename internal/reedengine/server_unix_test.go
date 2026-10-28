//go:build tier0 && !windows

// server_unix_test.go holds the server checks that rest on directory mode bits, which do not gate traversal on Windows.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestValidateToldWorktreeRootLive_UnreadableParentIsNotTheSentinel provokes a real non-fs.ErrNotExist stat failure — EACCES on the parent directory — and asserts it refuses without matching the sentinel, per the only-proven-gone-carries-the-sentinel contract.
// Skipped when running as root, since root ignores the permission bit entirely.
func TestValidateToldWorktreeRootLive_UnreadableParentIsNotTheSentinel(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bit")
	}

	parent := t.TempDir()
	worktreeRoot := filepath.Join(parent, "worktree")
	if err := os.Mkdir(worktreeRoot, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() {
		// Restore the mode so t.TempDir's own cleanup can traverse and remove parent.
		if err := os.Chmod(parent, 0o755); err != nil {
			t.Fatalf("restore parent mode: %v", err)
		}
	})

	err := validateToldWorktreeRootLive(Geometry{WorktreeRoot: worktreeRoot})
	if err == nil {
		t.Fatal("validateToldWorktreeRootLive with an unreadable parent = nil; want an error")
	}
	if errors.Is(err, errWorktreeRootGone) {
		t.Errorf("validateToldWorktreeRootLive with an unreadable parent matched errWorktreeRootGone; want a plain non-sentinel refusal")
	}
}
