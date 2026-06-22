//go:build integration

// git_test.go covers the git command helpers exposed by this package.

package git_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/git"
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
