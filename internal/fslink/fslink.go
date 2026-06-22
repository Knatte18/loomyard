// Package fslink provides a unified cross-platform link primitive that abstracts the
// differences between Windows junctions and POSIX symlinks. On Windows, links are
// junctions (mount-point reparse points) created via direct reparse-point syscalls
// (golang.org/x/sys/windows), requiring no special privileges. On non-Windows
// platforms, links are symlinks created via os.Symlink.
//
// Public API:
//   - CreateDirLink(link, target string) error: Create a directory link pointing to
//     target (a junction on Windows, a symlink elsewhere). Refuses to clobber existing
//     paths and creates missing parent directories. File links are not yet supported;
//     a future CreateFileLink is reserved for that (Windows file symlinks need elevated
//     privileges, which junctions do not).
//   - Remove(link string) error: Idempotent removal of a link; returns nil if the link
//     does not exist, otherwise returns wrapped errors.
//   - IsLink(path string) (bool, error): Reports whether path is a link. Returns
//     (false, nil) if path does not exist, (false, err) on stat errors, and (true/false, nil)
//     for successful checks.
//   - PointsTo(link string) (string, error): Returns the resolved absolute target of a
//     link via filepath.EvalSymlinks. No \??\ prefix in the result. Errors if link is
//     not a link or if the target does not exist.
//   - RemoveLinksIn(dir string) (int, error): Scans the immediate children of dir,
//     removes each link found, and returns the count. Regular files and real directories
//     are left untouched. Returns (0, err) if dir does not exist.
package fslink

import (
	"fmt"
	"os"
	"path/filepath"
)

// Remove idempotently deletes a link. It returns nil if the link does not exist
// (os.IsNotExist), wraps other errors with context "remove link %s: %w", and
// removes only the link entry itself, never recursing into the target.
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

// RemoveLinksIn scans the immediate children of dir and removes any links found.
// It returns the count of removed links and the first error encountered. Regular
// files and real subdirectories are left untouched. If dir does not exist or
// cannot be read, returns (0, err) from the os.ReadDir failure.
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

// prepareLink is an unexported helper that enforces the refuse-to-clobber and
// parent-mkdir guards. It returns an error if link already exists, if lstat fails
// for a reason other than os.IsNotExist, or if parent directory creation fails.
func prepareLink(link string) error {
	// Refuse to clobber existing paths
	if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("link already exists — remove it first: %s", link)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", link, err)
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", link, err)
	}

	return nil
}
