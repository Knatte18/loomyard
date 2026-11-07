// devbin.go derives where dev and test builds of the lym binary are installed and resolved from.
// The location is a `.dev-bin` directory at the repository root.
// Consumers import this package instead of deriving the root or directory name, so they all resolve the same binary.

// Package devbin locates the repository root and the `.dev-bin` directory for dev and test builds of lym.
// It holds the derivation in one place, so every caller computes the same path.
package devbin

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// RepoRoot returns the repository root directory.
// It takes the root from this source file's absolute path when the build records one.
// A `-trimpath` build, which every gate run makes, records a module-relative path instead.
// RepoRoot then falls back to the nearest directory holding go.mod at or above the working directory.
// Both `go run` from the repository root and a test binary in its package directory start inside the module.
func RepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate devbin source file")
	}
	if filepath.IsAbs(thisFile) {
		return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..")), nil
	}
	cwd, err := lymcwd.Getwd()
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

// BinPath returns the full path to the dev lym binary: `lym` or `lym.exe` on Windows.
func BinPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	name := "lym"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name), nil
}
