// devbin.go derives the single, non-hardcoded location where dev/test builds of the lyx binary are
// installed and resolved from: a `.dev-bin` directory sitting at the repository root.
// Every consumer (the deploy tool, the sandbox launcher) imports this package instead of deriving
// the repo root or the `.dev-bin` convention itself, so they can never disagree on where the dev
// binary lives.

// Package devbin locates the repository root and the derived `.dev-bin` directory used to install
// and resolve dev/test builds of lyx, keeping that derivation in exactly one place in the codebase.
package devbin

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// RepoRoot returns the repository root directory, derived from this source file's location via
// runtime.Caller.
func RepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate devbin source file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..")), nil
}

// Dir returns the `.dev-bin` directory where dev/test builds are installed.
func Dir() (string, error) {
	root, err := RepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".dev-bin"), nil
}

// BinPath returns the full path to the dev lyx binary: `lyx` or `lyx.exe` on Windows.
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
