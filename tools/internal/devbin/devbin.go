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
	"os"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// RepoRoot returns the repository root directory, derived from this source file's location via runtime.Caller.
// A `-trimpath` build, which every gate run makes, records that location module-relative.
// RepoRoot then answers the nearest directory holding go.mod at or above the working directory, since `go run` from the repository root and a test binary in its package directory both start inside the module.
func RepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate devbin source file")
	}
	if filepath.IsAbs(thisFile) {
		return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..")), nil
	}
	cwd, err := lyxcwd.Getwd()
	if err != nil {
		return "", fmt.Errorf("devbin: read the working directory: %w", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("devbin: a -trimpath build and no go.mod at or above %s", cwd)
		}
	}
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
