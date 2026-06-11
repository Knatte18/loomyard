package git_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/git"
)

// TestRunGit_Success tests basic git command execution
func TestRunGit_Success(t *testing.T) {
	stdout, _, exitCode, err := git.RunGit([]string{"--version"}, ".")
	if err != nil {
		t.Fatalf("RunGit failed with error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if stdout == "" {
		t.Fatal("expected non-empty stdout")
	}
}

// TestRunGit_NonZeroExit tests handling of non-zero exit codes
func TestRunGit_NonZeroExit(t *testing.T) {
	tempDir := t.TempDir()
	stdout, stderr, exitCode, err := git.RunGit([]string{"status"}, tempDir)
	if err != nil {
		t.Fatalf("RunGit should not return error for non-zero exit: %v", err)
	}
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code, got %d", exitCode)
	}
	if stderr == "" {
		t.Fatal("expected non-empty stderr for non-git directory")
	}
	_ = stdout // unused but captured
}

// TestRunGit_Cwd tests that the cwd parameter is respected
func TestRunGit_Cwd(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize a git repo in the temp directory
	stdout, stderr, exitCode, err := git.RunGit([]string{"init"}, tempDir)
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("git init exited with code %d: %s", exitCode, stderr)
	}
	_ = stdout

	// Run rev-parse in the same temp directory to verify it's a git repo
	stdout, stderr, exitCode, err = git.RunGit([]string{"rev-parse", "--absolute-git-dir"}, tempDir)
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("git rev-parse exited with code %d: %s", exitCode, stderr)
	}
	if stdout == "" {
		t.Fatal("expected non-empty stdout from git rev-parse")
	}
}

// TestFindRoot_InGitRepo tests that FindRoot returns a non-empty path in a fresh git repo
func TestFindRoot_InGitRepo(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize a git repo in the temp directory
	stdout, stderr, exitCode, err := git.RunGit([]string{"init"}, tempDir)
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("git init exited with code %d: %s", exitCode, stderr)
	}
	_ = stdout

	// Call FindRoot
	root, err := git.FindRoot(tempDir)
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	if root == "" {
		t.Fatal("expected non-empty root path")
	}

	// Verify the result matches or is symlink-equivalent to tempDir
	// Use suffix match to handle platform differences in temp dir resolution
	if !pathsMatch(tempDir, root) {
		t.Errorf("root %q does not match or resolve to tempDir %q", root, tempDir)
	}
}

// TestFindRoot_NotInGitRepo tests that FindRoot returns an error in a non-git directory
func TestFindRoot_NotInGitRepo(t *testing.T) {
	tempDir := t.TempDir()

	// Call FindRoot without initializing a git repo
	root, err := git.FindRoot(tempDir)

	if err == nil {
		t.Fatalf("expected error, got nil; root: %q", root)
	}

	if root != "" {
		t.Errorf("expected empty path on error, got %q", root)
	}
}

// pathsMatch checks if two paths refer to the same location, handling symlinks and platform differences
func pathsMatch(a, b string) bool {
	evalA, errA := filepath.EvalSymlinks(a)
	evalB, errB := filepath.EvalSymlinks(b)

	if errA == nil && errB == nil {
		return filepath.Clean(evalA) == filepath.Clean(evalB)
	}

	// Fallback to suffix match for platform compatibility
	return filepath.Clean(a) == filepath.Clean(b)
}
