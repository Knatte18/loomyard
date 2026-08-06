package fslink

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateDirLink establishes a symlink from link to target, refusing to clobber and creating parent
// directories.
func CreateDirLink(link, target string) error {
	if err := prepareLink(link); err != nil {
		return err
	}

	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}

	return nil
}

// IsLink reports whether path is a symlink, returning (false, nil) if absent.
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

// PointsTo returns the resolved absolute target of a symlink.
// Returns an error if link is not a symlink or if the target does not exist.
func PointsTo(link string) (string, error) {
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
