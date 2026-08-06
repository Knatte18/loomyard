// devbin_test.go covers the RepoRoot/Dir/BinPath derivation helpers.
// All tests are Go-native and Tier-1 pure: they only inspect derived paths and the local filesystem
// layout of this checkout, with no process spawns.

package devbin

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestRepoRoot_LocatesModuleRoot verifies RepoRoot returns the correct root directory with this
// package's source beneath it.
func TestRepoRoot_LocatesModuleRoot(t *testing.T) {
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
}

// TestDir_JoinsRepoRootWithDevBin verifies Dir returns RepoRoot + ".dev-bin".
func TestDir_JoinsRepoRootWithDevBin(t *testing.T) {
	root, err := RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot() error: %v", err)
	}
	want := filepath.Join(root, ".dev-bin")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if got != want {
		t.Errorf("Dir() = %q; want %q", got, want)
	}
}

// TestBinPath_PlatformBinaryName verifies BinPath returns the correct name for the current
// platform.
func TestBinPath_PlatformBinaryName(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}

	name := "lyx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	want := filepath.Join(dir, name)

	got, err := BinPath()
	if err != nil {
		t.Fatalf("BinPath() error: %v", err)
	}
	if got != want {
		t.Errorf("BinPath() = %q; want %q", got, want)
	}
}
