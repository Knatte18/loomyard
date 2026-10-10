//go:build integration

// gitexec_test.go covers the git command helpers exposed by this package.

package gitexec_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/proc"
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
		name       string
		args       []string
		dir        string
		stdin      string
		viaStdin   bool
		wantGitErr bool
		wantAnyErr bool
		wantStdout bool
		// wantExact, when set, is the exact stdout.
		wantExact   string
		wantDetails bool
	}{
		{name: "success", args: []string{"--version"}, dir: ".", wantStdout: true},
		{name: "non-zero exit", args: nonZeroArgs, dir: nonZeroDir, wantGitErr: true, wantAnyErr: true, wantDetails: true},
		{name: "exec failure", args: []string{"status"}, dir: filepath.Join(t.TempDir(), "does-not-exist"), wantAnyErr: true},
		{name: "stdin reaches git", args: []string{"hash-object", "--stdin"}, dir: nonZeroDir, stdin: "hello\n", viaStdin: true, wantStdout: true, wantExact: "ce013625030ba8dba906f756967f9e9ca394464a\n"},
		{name: "stdin non-zero exit", args: nonZeroArgs, dir: nonZeroDir, stdin: "ignored\n", viaStdin: true, wantGitErr: true, wantAnyErr: true, wantDetails: true},
		{name: "empty stdin behaves as Run", args: []string{"--version"}, dir: ".", viaStdin: true, wantStdout: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout string
			var err error
			if tt.viaStdin {
				stdout, err = gitexec.RunStdin(tt.args, tt.dir, tt.stdin)
			} else {
				stdout, err = gitexec.Run(tt.args, tt.dir)
			}
			if (err != nil) != tt.wantAnyErr {
				t.Fatalf("Run error = %v; wantErr %v", err, tt.wantAnyErr)
			}
			if (stdout != "") != tt.wantStdout {
				t.Errorf("stdout = %q; non-empty want %v", stdout, tt.wantStdout)
			}
			if tt.wantExact != "" && stdout != tt.wantExact {
				t.Errorf("stdout = %q; want %q", stdout, tt.wantExact)
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

	diffArgs := []string{"diff", "--exit-code", "--", "a.txt"}
	forms := map[string]func() (string, error){
		"Run":      func() (string, error) { return gitexec.Run(diffArgs, tempDir) },
		"RunStdin": func() (string, error) { return gitexec.RunStdin(diffArgs, tempDir, "ignored\n") },
	}
	for name, form := range forms {
		stdout, err := form()
		if err == nil {
			t.Fatalf("%s: expected a non-nil error for a non-empty diff with --exit-code", name)
		}

		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) {
			t.Fatalf("%s: expected errors.As to recover *gitexec.GitError, got %T: %v", name, err, err)
		}
		if stdout == "" {
			t.Fatalf("%s: expected non-empty stdout containing the diff output", name)
		}
	}
}

// TestRun_RemoteBounds drives the bounds on a remote git command through git's ext:: transport, a remote that accepts and never answers, with no network.
// Its rows run serially because they set the remote deadline and the kill reporter, which are process-global state.
// The ext:: command is a shell script, so the test is Unix-only.
func TestRun_RemoteBounds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the ext:: remote is a shell script")
	}

	const deadline = time.Second
	gitexec.SetRemoteDeadlineForTest(t, deadline)

	var mu sync.Mutex
	type kill struct {
		args     []string
		groupErr error
	}
	var kills []kill
	gitexec.SetKillReporter(func(args []string, pid int, groupErr error) {
		mu.Lock()
		defer mu.Unlock()
		kills = append(kills, kill{args: args, groupErr: groupErr})
	})
	t.Cleanup(func() { gitexec.SetKillReporter(nil) })

	scriptDir := t.TempDir()
	pidFile := filepath.Join(scriptDir, "pid")
	hang := filepath.Join(scriptDir, "hang.sh")
	printEnv := filepath.Join(scriptDir, "env.sh")
	for path, body := range map[string]string{
		hang:     "#!/bin/sh\necho $$ > " + pidFile + "\nexec sleep 300\n",
		printEnv: "#!/bin/sh\nenv >&2\nexit 1\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	repoDir := t.TempDir()

	if !t.Run("a hung remote command is killed at the deadline", func(t *testing.T) {
		args := []string{"-c", "protocol.ext.allow=always", "ls-remote", "ext::" + hang}
		start := time.Now()
		_, err := gitexec.Run(args, repoDir)
		if elapsed := time.Since(start); elapsed > 10*deadline {
			t.Errorf("Run took %v; want it to return shortly after the %v deadline", elapsed, deadline)
		}

		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) || gitErr.Timeout != deadline {
			t.Fatalf("Run error = %v; want a *GitError with Timeout %v", err, deadline)
		}
		if !gitexec.IsTransportFailure(err) {
			t.Errorf("IsTransportFailure(%v) = false; want true for a timed-out remote command", err)
		}

		mu.Lock()
		recorded := append([]kill(nil), kills...)
		mu.Unlock()
		if len(recorded) != 1 || !reflect.DeepEqual(recorded[0].args, args) || recorded[0].groupErr != nil {
			t.Errorf("kill reports = %v; want one report for %v with a nil group error", recorded, args)
		}

		pidText, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatalf("read the ext command's pid: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
		if err != nil {
			t.Fatalf("parse the ext command's pid %q: %v", pidText, err)
		}
		// The killed child is a zombie until it is reaped, so poll for it to disappear.
		for end := time.Now().Add(10 * time.Second); proc.IsAlive(pid) && time.Now().Before(end); {
			time.Sleep(20 * time.Millisecond)
		}
		if proc.IsAlive(pid) {
			t.Errorf("the ext command (pid %d) outlived the process-group kill", pid)
		}
	}) {
		return
	}

	if !t.Run("a remote command gets the low-speed environment", func(t *testing.T) {
		_, err := gitexec.Run([]string{"-c", "protocol.ext.allow=always", "ls-remote", "ext::" + printEnv}, repoDir)
		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) {
			t.Fatalf("Run error = %v; want a *GitError from the failing ext command", err)
		}
		for _, want := range []string{"GIT_HTTP_LOW_SPEED_LIMIT=1000", "GIT_HTTP_LOW_SPEED_TIME=60"} {
			if !strings.Contains(gitErr.Stderr, want) {
				t.Errorf("remote command environment lacks %s; stderr: %s", want, gitErr.Stderr)
			}
		}
	}) {
		return
	}

	t.Run("a local command is neither bounded nor given the low-speed environment", func(t *testing.T) {
		stdout, err := gitexec.Run([]string{"-c", "alias.slowenv=!sleep 2 && env", "slowenv"}, repoDir)
		if err != nil {
			t.Fatalf("Run error = %v; want a local command to run past the remote deadline", err)
		}
		if strings.Contains(stdout, "GIT_HTTP_LOW_SPEED") {
			t.Errorf("local command environment holds a low-speed variable; stdout: %s", stdout)
		}
	})
}
