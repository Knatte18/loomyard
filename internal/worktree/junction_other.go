//go:build !windows

package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

// createJunction creates a symlink from link to target on non-Windows platforms.
//
// The parent directory of the link is created if needed.
// If the link path already exists, returns an error (refuses to clobber).
// Returns an error if creation fails.
func createJunction(link, target string) error {
	// Check if link already exists
	if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("symlink already exists — remove it first: %s", link)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", link, err)
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", link, err)
	}

	// Create symlink
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}

	return nil
}
