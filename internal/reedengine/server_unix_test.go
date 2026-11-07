//go:build !windows

// server_unix_test.go holds the server checks that rest on directory mode bits, which do not gate traversal on Windows.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestValidateToldWorktreeRootLive_UnreadableParentIsNotTheSentinel makes the parent directory unreadable,
// so the stat fails with EACCES rather than fs.ErrNotExist.
// It asserts the refusal does not match the sentinel: only a path proven gone carries errWorktreeRootGone.
// Skipped when running as root, since root ignores the permission bit entirely.
func TestValidateToldWorktreeRootLive_UnreadableParentIsNotTheSentinel(t *testing.T) {
	tiergate.Require(t, 0)
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
