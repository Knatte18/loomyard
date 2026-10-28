//go:build tier1

// gitexec_test.go covers the git command helpers exposed by this package.

package gitexec_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/toolfloor"
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
// One row sets GIT_DIR and GIT_WORK_TREE to a second repository and asserts the child still answers the directory it was run in, because git children never inherit those two variables.
// Another sets GIT_OPTIONAL_LOCKS=1 and asserts the child sees it as 0 alone, so a read-only command never takes the index lock.
// Those rows touch process-global state, the environment, so the test does not call t.Parallel; its table rows do.
func TestRun(t *testing.T) {

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

	t.Run("ignores GIT_DIR and GIT_WORK_TREE", func(t *testing.T) {
		first, second := t.TempDir(), t.TempDir()
		for _, dir := range []string{first, second} {
			if _, err := gitexec.Run([]string{"init", "-q"}, dir); err != nil {
				t.Fatalf("git init in %s error = %v", dir, err)
			}
		}
		t.Setenv("GIT_DIR", filepath.Join(second, ".git"))
		t.Setenv("GIT_WORK_TREE", second)

		got, err := gitexec.Run([]string{"rev-parse", "--absolute-git-dir"}, first)
		if err != nil {
			t.Fatalf("Run(rev-parse --absolute-git-dir) error = %v", err)
		}
		wantDir, err := filepath.EvalSymlinks(filepath.Join(first, ".git"))
		if err != nil {
			t.Fatalf("EvalSymlinks error = %v", err)
		}
		gotDir, err := filepath.EvalSymlinks(strings.TrimSpace(got))
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) error = %v", got, err)
		}
		if gotDir != wantDir {
			t.Errorf("git dir = %q; want %q, the directory the command ran in", gotDir, wantDir)
		}
	})

	t.Run("turns optional locks off", func(t *testing.T) {
		t.Setenv("GIT_OPTIONAL_LOCKS", "1")

		got, err := gitexec.Run([]string{"-c", "alias.lymenv=!env", "lymenv"}, t.TempDir())
		if err != nil {
			t.Fatalf("Run(lymenv) error = %v", err)
		}
		var values []string
		for _, line := range strings.Split(got, "\n") {
			if value, found := strings.CutPrefix(strings.TrimRight(line, "\r"), "GIT_OPTIONAL_LOCKS="); found {
				values = append(values, value)
			}
		}
		if !reflect.DeepEqual(values, []string{"0"}) {
			t.Errorf("GIT_OPTIONAL_LOCKS values in the child environment = %v; want [0]", values)
		}
	})

	t.Run("RunWithEnv reaches the child's environment", func(t *testing.T) {
		got, err := gitexec.RunWithEnv([]string{"var", "GIT_AUTHOR_IDENT"}, t.TempDir(), []string{"GIT_AUTHOR_NAME=Env Author", "GIT_AUTHOR_EMAIL=env@example.com"})
		if err != nil {
			t.Fatalf("RunWithEnv(var GIT_AUTHOR_IDENT) error = %v", err)
		}
		if !strings.HasPrefix(got, "Env Author <env@example.com>") {
			t.Errorf("RunWithEnv(var GIT_AUTHOR_IDENT) = %q; want it to start with the author set through extraEnv", got)
		}
	})
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
		"RunWithEnv": func() (string, error) {
			return gitexec.RunWithEnv(diffArgs, tempDir, []string{"GIT_AUTHOR_NAME=Env Author"})
		},
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

// TestCheckFloor pins the once-per-process git floor check through every entry point.
// It is serial: it replaces the process-global version probe and floor cache, which no parallel test may see.
func TestCheckFloor(t *testing.T) {
	// countingProbe scripts one answer per call, repeating the last, and counts the calls.
	type answer struct {
		line string
		err  error
	}
	countingProbe := func(answers ...answer) (func() (string, error), *int) {
		calls := 0
		return func() (string, error) {
			a := answers[min(calls, len(answers)-1)]
			calls++
			return a.line, a.err
		}, &calls
	}

	t.Run("a below-floor git refuses every entry point", func(t *testing.T) {
		probe, calls := countingProbe(answer{line: "git version 2.42.0"})
		gitexec.SetVersionProbeForTest(t, probe)
		dir := t.TempDir()

		_, runErr := gitexec.Run([]string{"--version"}, dir)
		_, stdinErr := gitexec.RunStdin([]string{"--version"}, dir, "")
		_, envErr := gitexec.RunWithEnv([]string{"--version"}, dir, []string{"GIT_AUTHOR_NAME=Env Author"})
		_, _, exitCode, rawErr := gitexec.RunGit([]string{"--version"}, dir)
		checkErr := gitexec.CheckFloor()

		for name, err := range map[string]error{"Run": runErr, "RunStdin": stdinErr, "RunWithEnv": envErr, "RunGit": rawErr, "CheckFloor": checkErr} {
			var floorErr *toolfloor.FloorError
			if !errors.As(err, &floorErr) {
				t.Errorf("%s error = %v; want a *toolfloor.FloorError", name, err)
			}
			var gitErr *gitexec.GitError
			if errors.As(err, &gitErr) {
				t.Errorf("%s error = %v; want no *GitError", name, err)
			}
		}
		if exitCode != -1 {
			t.Errorf("RunGit exit code = %d; want -1", exitCode)
		}
		if *calls != 1 {
			t.Errorf("probe ran %d times; want 1", *calls)
		}
	})

	t.Run("an at-floor git is probed once", func(t *testing.T) {
		probe, calls := countingProbe(answer{line: "git version 2.43.0"})
		gitexec.SetVersionProbeForTest(t, probe)
		dir := t.TempDir()

		for range 2 {
			if _, err := gitexec.Run([]string{"--version"}, dir); err != nil {
				t.Fatalf("Run(--version) error = %v; want nil", err)
			}
		}
		if *calls != 1 {
			t.Errorf("probe ran %d times; want 1", *calls)
		}
	})

	t.Run("a probe that could not run is retried", func(t *testing.T) {
		execErr := errors.New("git not found")
		probe, calls := countingProbe(answer{err: execErr}, answer{line: "git version 2.43.0"})
		gitexec.SetVersionProbeForTest(t, probe)

		if err := gitexec.CheckFloor(); !errors.Is(err, execErr) {
			t.Errorf("first CheckFloor() = %v; want %v", err, execErr)
		}
		if err := gitexec.CheckFloor(); err != nil {
			t.Errorf("second CheckFloor() = %v; want nil", err)
		}
		if *calls != 2 {
			t.Errorf("probe ran %d times; want 2", *calls)
		}
	})

	t.Run("an unparseable line is admitted with a warning", func(t *testing.T) {
		probe, calls := countingProbe(answer{line: "git version unknown"})
		gitexec.SetVersionProbeForTest(t, probe)
		logs := logcapture.Capture(t)
		dir := t.TempDir()

		for range 2 {
			if _, err := gitexec.Run([]string{"--version"}, dir); err != nil {
				t.Fatalf("Run(--version) error = %v; want nil", err)
			}
		}
		if *calls != 1 {
			t.Errorf("probe ran %d times; want 1", *calls)
		}
		if got := strings.Count(logs.String(), "git version unknown"); got != 1 {
			t.Errorf("log names the line %d times; want 1; log:\n%s", got, logs.String())
		}
	})
}
