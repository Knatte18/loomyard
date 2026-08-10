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

// RawTarget returns the literal, one-hop target recorded in link's own symlink data — what
// os.Symlink(target, link) actually wrote — without resolving link's target further, and without
// requiring that target (or anything past it) to exist.
// This is the ownership-check primitive PointsTo cannot serve: PointsTo fully resolves the chain, so
// it fails outright when a later segment is gone (the legitimate case of a stale-pair prune, where
// the warp side a wired junction chains through no longer exists), and it silently walks past a
// target that is itself a further symlink (a fabric junction wired at a path whose own immediate
// target is another fabric junction, e.g. a portal pointing at a warp `_lyx` that itself points at
// weft) — comparing that fully-resolved end state against the ONE HOP a wiring call actually recorded
// is a mismatch by construction, not a drift.
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
