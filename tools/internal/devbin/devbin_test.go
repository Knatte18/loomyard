// devbin_test.go covers the RepoRoot/Dir/BinPath derivation helpers.
// All tests are Go-native and Tier-1 pure: they only inspect derived paths and the local filesystem
// layout of this checkout, with no process spawns.

package devbin

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestDevBinPaths_DeriveFromRepoRoot verifies RepoRoot returns the root directory with this
// package's source beneath it, Dir returns RepoRoot + ".dev-bin", and BinPath returns the
// platform's binary name inside Dir.
func TestDevBinPaths_DeriveFromRepoRoot(t *testing.T) {
	t.Parallel()

	root, err := RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot() error: %v", err)
	}

	wantSelfDir := filepath.Join(root, "tools", "internal", "devbin")
	gotSelfDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("filepath.Abs(.): %v", err)
	}
	if wantSelfDir != gotSelfDir {
		t.Errorf("RepoRoot() = %q; tools/internal/devbin beneath it = %q, want %q", root, wantSelfDir, gotSelfDir)
	}

	wantDir := filepath.Join(root, ".dev-bin")
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if dir != wantDir {
		t.Errorf("Dir() = %q; want %q", dir, wantDir)
	}

	name := "lyx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	wantBin := filepath.Join(dir, name)
	got, err := BinPath()
	if err != nil {
		t.Fatalf("BinPath() error: %v", err)
	}
	if got != wantBin {
		t.Errorf("BinPath() = %q; want %q", got, wantBin)
	}
}
