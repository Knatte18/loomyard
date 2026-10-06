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

// TestRunGit pins RunGit's contract: a successful command returns its stdout with exit 0, a non-zero exit is reported through the exit code and stderr with a nil error, and an exec-level failure (a cwd that does not exist) returns exit -1 with blanked stdout and stderr.
// The non-zero row also shows the cwd parameter is respected: run in the package directory instead, `git status` would succeed.
func TestRunGit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		dir        string
		wantErr    bool
		wantExit   func(int) bool
		wantStdout bool
		wantStderr bool
	}{
		{
			name:       "success",
			args:       []string{"--version"},
			dir:        ".",
			wantExit:   func(code int) bool { return code == 0 },
			wantStdout: true,
		},
		{
			name:       "non-zero exit",
			args:       []string{"status"},
			dir:        t.TempDir(),
			wantExit:   func(code int) bool { return code > 0 },
			wantStderr: true,
		},
		{
			name:     "exec failure",
			args:     []string{"status"},
			dir:      filepath.Join(t.TempDir(), "does-not-exist"),
			wantErr:  true,
			wantExit: func(code int) bool { return code == -1 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, exitCode, err := gitexec.RunGit(tt.args, tt.dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RunGit error = %v; wantErr %v", err, tt.wantErr)
			}
			if !tt.wantExit(exitCode) {
				t.Errorf("unexpected exit code %d", exitCode)
			}
			if (stdout != "") != tt.wantStdout {
				t.Errorf("stdout = %q; non-empty want %v", stdout, tt.wantStdout)
			}
			if (stderr != "") != tt.wantStderr {
				t.Errorf("stderr = %q; non-empty want %v", stderr, tt.wantStderr)
			}
		})
	}
}

// TestRun pins Run's contract: a successful command returns its stdout with a nil error; a non-zero exit is recoverable via errors.As as *gitexec.GitError carrying the exit code, the args and dir it was given and non-empty stderr; and an exec-level failure — a cwd that does not exist — returns a non-nil error that errors.As does NOT match as *gitexec.GitError, the distinction every errors.As recovery site depends on.
func TestRun(t *testing.T) {
	t.Parallel()

	nonZeroArgs := []string{"log", "--format=stdout-marker", "-1"}
	nonZeroDir := t.TempDir()
	tests := []struct {
		name        string
		args        []string
		dir         string
		wantGitErr  bool
		wantAnyErr  bool
		wantStdout  bool
		wantDetails bool
	}{
		{name: "success", args: []string{"--version"}, dir: ".", wantStdout: true},
		{name: "non-zero exit", args: nonZeroArgs, dir: nonZeroDir, wantGitErr: true, wantAnyErr: true, wantDetails: true},
		{name: "exec failure", args: []string{"status"}, dir: filepath.Join(t.TempDir(), "does-not-exist"), wantAnyErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, err := gitexec.Run(tt.args, tt.dir)
			if (err != nil) != tt.wantAnyErr {
				t.Fatalf("Run error = %v; wantErr %v", err, tt.wantAnyErr)
			}
			if (stdout != "") != tt.wantStdout {
				t.Errorf("stdout = %q; non-empty want %v", stdout, tt.wantStdout)
			}
			var gitErr *gitexec.GitError
			if errors.As(err, &gitErr) != tt.wantGitErr {
				t.Fatalf("errors.As(*GitError) on %T: %v; want %v", err, err, tt.wantGitErr)
			}
			if !tt.wantDetails {
				return
			}
			if gitErr.ExitCode == 0 {
				t.Errorf("expected a non-zero exit code, got %d", gitErr.ExitCode)
			}
			if !reflect.DeepEqual(gitErr.Args, tt.args) {
				t.Errorf("GitError.Args = %v; want %v", gitErr.Args, tt.args)
			}
			if gitErr.Dir != tt.dir {
				t.Errorf("GitError.Dir = %q; want %q", gitErr.Dir, tt.dir)
			}
			if gitErr.Stderr == "" {
				t.Error("expected non-empty GitError.Stderr")
			}
		})
	}
}

// TestRun_StdoutOnError tests that stdout is still returned alongside a *GitError, using a command that writes to stdout and then exits non-zero:
// `git diff --exit-code` prints the diff to stdout and exits 1 when the working tree differs from the last commit.
//
//testtiming:keep pins that stdout is returned alongside a *GitError, which the covering tests discard
func TestRun_StdoutOnError(t *testing.T) {
	t.Parallel()

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
