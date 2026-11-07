// Package fslink provides a unified cross-platform link primitive.
// It abstracts the differences between Windows junctions and POSIX symlinks.
// On Windows, links are junctions (mount-point reparse points) created with direct reparse-point syscalls.
// Those syscalls come from golang.org/x/sys/windows, and creating a junction needs no special privileges.
// On non-Windows platforms, links are symlinks created via os.Symlink.
//
// Public API:
//   - CreateDirLink(link, target string) error: Create a directory link pointing to target.
//     On Windows it is a junction, elsewhere a symlink.
//     It refuses to clobber existing paths and creates missing parent directories.
//   - CreateFileLink(link, target string) error: Create a symlink to the file target.
//     Off Windows it refuses to clobber existing paths and creates missing parent directories.
//     On Windows it refuses before touching the filesystem: a file symlink there needs elevated privileges,
//     and a junction links only a directory.
//   - Remove(link string) error: Idempotent removal of a link.
//     It returns nil if the link does not exist, and wrapped errors otherwise.
//   - IsLink(path string) (bool, error): Reports whether path is a link.
//     It returns (false, nil) if path does not exist, and (false, err) on stat errors.
//     Otherwise it returns (true or false, nil).
//   - PointsTo(link string) (string, error): Returns the resolved absolute target of a link.
//     It resolves the chain with filepath.EvalSymlinks; the result has no \??\ prefix.
//     It errors if link is not a link or if the target does not exist.
//   - RemoveLinksIn(dir string) (int, error): Scans the immediate children of dir and removes each link found.
//     It returns the count of links removed.
//     Regular files and real directories are left untouched.
//     It returns (0, err) if dir does not exist.
//
// The tests run on Linux, so only the symlink path is executed.
// The Windows junction path is reasoned about rather than run.
package fslink

import (
	"fmt"
	"os"
	"path/filepath"
)

// Remove idempotently deletes a link, returning nil if absent.
// It wraps errors with context and removes only the link entry, never the target.
func Remove(link string) error {
	err := os.Remove(link)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("remove link %s: %w", link, err)
}

// RemoveLinksIn scans dir's immediate children, removes links found, and returns the count.
// Regular files and directories are left untouched.
func RemoveLinksIn(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		isLink, err := IsLink(fullPath)
		if err != nil {
			return count, err
		}
		if isLink {
			if err := Remove(fullPath); err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}

// prepareLink enforces refuse-to-clobber and parent-mkdir guards.
func prepareLink(link string) error {
	if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("link already exists — remove it first: %s", link)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", link, err)
	}

	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", link, err)
	}

	return nil
}
