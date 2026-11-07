package fslink

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateDirLink establishes a symlink from link to target.
// It refuses to clobber an existing path and creates parent directories.
func CreateDirLink(link, target string) error {
	if err := prepareLink(link); err != nil {
		return err
	}

	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}

	return nil
}

// CreateFileLink establishes a symlink from link to the file target.
// It refuses to clobber an existing path and creates parent directories.
// A program that writes the file in place through the link writes target.
// A program that removes the file by name removes only the link.
func CreateFileLink(link, target string) error {
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

// RawTarget returns the literal target recorded in link's own symlink data, one hop only.
// That is the path os.Symlink wrote, returned without resolving it further and without requiring it to exist.
// Ownership checks need this one hop: a fully resolved path differs from it whenever the target is a link,
// and it fails outright when a later segment is missing.
// Returns an error if link is not a symlink.
func RawTarget(link string) (string, error) {
	isLink, err := IsLink(link)
	if err != nil {
		return "", err
	}
	if !isLink {
		return "", fmt.Errorf("RawTarget: %s is not a link", link)
	}

	rawTarget, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("os.Readlink(%s): %w", link, err)
	}

	return rawTarget, nil
}
