//go:build !windows

package fslink

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateDirLink establishes a directory link — a symlink — from link to target. It
// calls prepareLink to refuse clobbering and create parent directories, then creates
// the symlink with os.Symlink, storing the target verbatim (not absolutized). On this
// platform symlinks target both files and directories, but the cross-platform contract
// is directory-only (Windows junctions cannot target files); a future CreateFileLink is
// reserved for file links.
func CreateDirLink(link, target string) error {
	if err := prepareLink(link); err != nil {
		return err
	}

	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}

	return nil
}

// IsLink reports whether path is a symlink. It returns (false, nil) if path does not
// exist, (false, err) on stat errors, and (true/false, nil) when the path exists and
// can be checked. A symlink is detected via the os.ModeSymlink bit in the file mode.
func IsLink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return (info.Mode()&os.ModeSymlink != 0), nil
}

// PointsTo returns the resolved absolute target of a symlink via filepath.EvalSymlinks.
// Returns an error if link is not a symlink or if the target does not exist.
func PointsTo(link string) (string, error) {
	// Verify it's actually a symlink
	isLink, err := IsLink(link)
	if err != nil {
		return "", err
	}
	if !isLink {
		return "", fmt.Errorf("PointsTo: %s is not a link", link)
	}

	absTarget, err := filepath.EvalSymlinks(link)
	if err != nil {
		return "", fmt.Errorf("filepath.EvalSymlinks(%s): %w", link, err)
	}

	return absTarget, nil
}
