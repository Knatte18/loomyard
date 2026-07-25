// devbin.go derives the single, non-hardcoded location where dev/test builds of
// the lyx binary are installed and resolved from: a `.dev-bin` directory sitting
// at the repository root. Every consumer (the deploy tool, the sandbox launcher)
// imports this package instead of deriving the repo root or the `.dev-bin`
// convention itself, so they can never disagree on where the dev binary lives.

// Package devbin locates the repository root and the derived `.dev-bin`
// directory used to install and resolve dev/test builds of lyx, keeping that
// derivation in exactly one place in the codebase.
package devbin

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// RepoRoot returns the repository root directory, derived from the location of
// this source file (tools/internal/devbin/devbin.go) via runtime.Caller. It
// never hardcodes a machine-specific path, so it works from any checkout or
// worktree regardless of the caller's current working directory.
func RepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate devbin source file")
	}
	// This file sits three levels below the repo root
	// (tools/internal/devbin/devbin.go), so walk up three directories.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..")), nil
}

// Dir returns the `.dev-bin` directory beneath the repository root, the
// single derived location where dev/test builds of lyx are installed and
// resolved from.
func Dir() (string, error) {
	root, err := RepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".dev-bin"), nil
}

// BinPath returns the full path to the dev lyx binary inside Dir(): `lyx` on
// most platforms, `lyx.exe` on Windows.
func BinPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	name := "lyx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name), nil
}
