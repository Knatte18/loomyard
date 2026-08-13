//go:build integration

// gitexec_test.go covers the git command helpers exposed by this package.

package gitexec_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// TestRunGit_Success tests basic git command execution
func TestRunGit_Success(t *testing.T) {
	stdout, _, exitCode, err := gitexec.RunGit([]string{"--version"}, ".")
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
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status"}, tempDir)
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
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"init"}, tempDir)
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("git init exited with code %d: %s", exitCode, stderr)
	}
	_ = stdout

	// Run rev-parse in the same temp directory to verify it's a git repo
	stdout, stderr, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--absolute-git-dir"}, tempDir)
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

// TestRunGit_ExecFailure pins RunGit's unchanged shape on an exec-level
// failure: exit code -1 and blanked stdout and stderr, even though runCore
// now backs both RunGit and Run.
func TestRunGit_ExecFailure(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")

	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status"}, missingDir)
	if err == nil {
		t.Fatal("expected a non-nil error for a nonexistent cwd")
	}
	if exitCode != -1 {
		t.Fatalf("expected exit code -1, got %d", exitCode)
	}
	if stdout != "" {
		t.Fatalf("expected blanked stdout, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected blanked stderr, got %q", stderr)
	}
}

// TestRun_Success tests that a successful command returns its stdout with a
// nil error.
func TestRun_Success(t *testing.T) {
	stdout, err := gitexec.Run([]string{"--version"}, ".")
	if err != nil {
		t.Fatalf("Run failed with error: %v", err)
	}
	if stdout == "" {
		t.Fatal("expected non-empty stdout")
	}
}

// TestRun_NonZeroExit tests that a non-zero exit is recoverable via
// errors.As as *gitexec.GitError, carrying the exit code, the args and dir
// it was given, and non-empty stderr — and that stdout is still returned
// alongside that error.
func TestRun_NonZeroExit(t *testing.T) {
	tempDir := t.TempDir()
	args := []string{"log", "--format=stdout-marker", "-1"}

	stdout, err := gitexec.Run(args, tempDir)
	if err == nil {
		t.Fatal("expected a non-nil error for a command run outside a git repo")
	}

	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("expected errors.As to recover *gitexec.GitError, got %T: %v", err, err)
	}
	if gitErr.ExitCode == 0 {
		t.Fatalf("expected a non-zero exit code, got %d", gitErr.ExitCode)
	}
	if !reflect.DeepEqual(gitErr.Args, args) {
		t.Errorf("GitError.Args = %v; want %v", gitErr.Args, args)
	}
	if gitErr.Dir != tempDir {
		t.Errorf("GitError.Dir = %q; want %q", gitErr.Dir, tempDir)
	}
	if gitErr.Stderr == "" {
		t.Error("expected non-empty GitError.Stderr")
	}
	_ = stdout // git may emit nothing to stdout on this failure; presence isn't asserted here
}

// TestRun_StdoutOnError tests that stdout is still returned alongside a
// *GitError, using a command that writes to stdout and then exits non-zero:
// `git diff --exit-code` prints the diff to stdout and exits 1 when the
// working tree differs from the last commit.
func TestRun_StdoutOnError(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "a.txt")

	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, tempDir); err != nil || exitCode != 0 {
		t.Fatalf("git init failed: exitCode=%d err=%v", exitCode, err)
	}
	if err := os.WriteFile(filePath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("failed to seed a.txt: %v", err)
	}
	if _, _, exitCode, err := gitexec.RunGit([]string{"add", "a.txt"}, tempDir); err != nil || exitCode != 0 {
		t.Fatalf("git add failed: exitCode=%d err=%v", exitCode, err)
	}
	if _, _, exitCode, err := gitexec.RunGit([]string{"commit", "-m", "seed"}, tempDir); err != nil || exitCode != 0 {
		t.Fatalf("git commit failed: exitCode=%d err=%v", exitCode, err)
	}
	if err := os.WriteFile(filePath, []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("failed to modify a.txt: %v", err)
	}

	stdout, err := gitexec.Run([]string{"diff", "--exit-code", "--", "a.txt"}, tempDir)
	if err == nil {
		t.Fatal("expected a non-nil error for a non-empty diff with --exit-code")
	}

	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("expected errors.As to recover *gitexec.GitError, got %T: %v", err, err)
	}
	if stdout == "" {
		t.Fatal("expected non-empty stdout containing the diff output")
	}
}

// TestRun_ExecFailure tests that an exec-level failure — a cwd that does
// not exist — returns a non-nil error that errors.As does NOT match as
// *gitexec.GitError. This is the load-bearing distinction every
// errors.As recovery site across batches 2 through 7 depends on.
func TestRun_ExecFailure(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := gitexec.Run([]string{"status"}, missingDir)
	if err == nil {
		t.Fatal("expected a non-nil error for a nonexistent cwd")
	}

	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) {
		t.Fatal("expected errors.As NOT to recover *gitexec.GitError for an exec-level failure")
	}
}
