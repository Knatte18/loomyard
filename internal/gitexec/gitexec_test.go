//go:build integration

// gitexec_test.go covers the git command helpers exposed by this package.

package gitexec_test

import (
	"context"
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
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
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

		got, err := gitexec.Run([]string{"-c", "alias.lyxenv=!env", "lyxenv"}, t.TempDir())
		if err != nil {
			t.Fatalf("Run(lyxenv) error = %v", err)
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
// Its rows run serially because they set the remote deadline, the stall bound, the kill reporter, the pause network gate and the process environment, which are process-global state.
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

	if !t.Run("a remote command waits while a pause covers the process, a local one does not", func(t *testing.T) {
		marker := filepath.Join(scriptDir, "ran")
		touch := filepath.Join(scriptDir, "touch.sh")
		if err := os.WriteFile(touch, []byte("#!/bin/sh\ntouch "+marker+"\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", touch, err)
		}
		recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
		setLive := func(entries ...pauserecord.Entry) {
			t.Helper()
			if _, err := pauserecord.Update(recordPath, time.Now(), func(r *pauserecord.Record) error {
				r.Live = entries
				return nil
			}); err != nil {
				t.Fatalf("update the pause record: %v", err)
			}
		}
		setLive(pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: time.Now()})

		wakes := make(chan struct{}, 100)
		restore := pauserecord.InstallGate(pauserecord.Gate{
			Geometry: pauserecord.Geometry{RecordPath: recordPath},
			Notify:   func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				wakes <- struct{}{}
				time.Sleep(20 * time.Millisecond)
				return nil
			},
		})
		t.Cleanup(restore)

		finished := make(chan struct{})
		go func() {
			defer close(finished)
			// The ext:: command exits without speaking the protocol, so the call's own error is not the assertion.
			_, _ = gitexec.Run([]string{"-c", "protocol.ext.allow=always", "ls-remote", "ext::" + touch}, repoDir)
		}()

		select {
		case <-wakes:
		case <-time.After(10 * time.Second):
			t.Fatal("the held remote command never woke its wait")
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("marker stat error = %v; want the remote command not to have run while the pause covers the process", err)
		}

		setLive()
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Fatal("the remote command did not run after the pause was resumed")
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("marker stat error = %v; want the remote command to have run after the resume", err)
		}

		setLive(pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: time.Now()})
		for len(wakes) > 0 {
			<-wakes
		}
		if _, err := gitexec.Run([]string{"-c", "alias.ok=!true", "ok"}, repoDir); err != nil {
			t.Fatalf("local command under a live pause: %v", err)
		}
		if len(wakes) != 0 {
			t.Fatalf("a local command woke the pause wait %d times; want none", len(wakes))
		}
	}) {
		return
	}

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

	if !t.Run("RunWithin bounds a remote command by its own deadline, not the package's", func(t *testing.T) {
		own := deadline / 4
		_, err := gitexec.RunWithin([]string{"-c", "protocol.ext.allow=always", "ls-remote", "ext::" + hang}, repoDir, own)

		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) || gitErr.Timeout != own {
			t.Fatalf("RunWithin error = %v; want a *GitError with Timeout %v", err, own)
		}
		if !gitexec.IsTransportFailure(err) {
			t.Errorf("IsTransportFailure(%v) = false; want true for a timed-out remote command", err)
		}
	}) {
		return
	}

	// remoteAttempts runs a remote ls-remote against an ext:: command that counts its runs, writes reply and hangs, and returns the command's run count, the call's kill reports and its error.
	remoteAttempts := func(t *testing.T, name, reply string) (int, []kill, error) {
		t.Helper()
		countFile := filepath.Join(scriptDir, name+".count")
		script := filepath.Join(scriptDir, name+".sh")
		body := "#!/bin/sh\necho run >> " + countFile + "\nprintf '" + reply + "'\nexec sleep 300\n"
		if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
			t.Fatalf("write %s: %v", script, err)
		}
		mu.Lock()
		kills = nil
		mu.Unlock()

		_, err := gitexec.Run([]string{"-c", "protocol.ext.allow=always", "ls-remote", "ext::" + script}, repoDir)

		counted, readErr := os.ReadFile(countFile)
		if readErr != nil {
			t.Fatalf("read the ext command's run count: %v", readErr)
		}
		mu.Lock()
		defer mu.Unlock()
		return strings.Count(string(counted), "run\n"), append([]kill(nil), kills...), err
	}

	if !t.Run("a remote that never answers is killed at the stall bound and run once more", func(t *testing.T) {
		const stall = deadline / 5
		gitexec.SetStallBoundForTest(t, stall)
		logs := logcapture.Capture(t)

		runs, recorded, err := remoteAttempts(t, "silent", "")

		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) || !gitErr.Stalled || gitErr.Timeout != stall || gitErr.ExitCode != -1 {
			t.Fatalf("Run error = %v; want a stalled *GitError with Timeout %v and exit code -1", err, stall)
		}
		if !gitexec.IsTransportFailure(err) {
			t.Errorf("IsTransportFailure(%v) = false; want true for a stalled remote command", err)
		}
		if runs != 2 || len(recorded) != 2 {
			t.Errorf("runs = %d, kill reports = %d; want 2 of each, the call and its one retry", runs, len(recorded))
		}
		if got := strings.Count(logs.String(), "remote sent no answer, retrying once"); got != 1 {
			t.Errorf("retry warnings = %d; want 1; log:\n%s", got, logs.String())
		}

		runDir, dirErr := lyxtemp.Dir()
		if dirErr != nil {
			t.Fatalf("lyxtemp.Dir() error = %v", dirErr)
		}
		leftover, globErr := filepath.Glob(filepath.Join(runDir, "git-packet-trace-*"))
		if globErr != nil {
			t.Fatalf("glob the run directory: %v", globErr)
		}
		if len(leftover) != 0 {
			t.Errorf("trace files left after the call = %v; want none", leftover)
		}
	}) {
		return
	}

	if !t.Run("a deadline at or under the stall bound kills a silent remote once, at the deadline", func(t *testing.T) {
		gitexec.SetStallBoundForTest(t, deadline)

		runs, recorded, err := remoteAttempts(t, "unarmed", "")

		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) || gitErr.Stalled || gitErr.Timeout != deadline {
			t.Fatalf("Run error = %v; want a *GitError with Timeout %v that is not stalled", err, deadline)
		}
		if runs != 1 || len(recorded) != 1 {
			t.Errorf("runs = %d, kill reports = %d; want 1 of each", runs, len(recorded))
		}
	}) {
		return
	}

	if !t.Run("a remote that answers is not stalled, and is killed at the deadline without a retry", func(t *testing.T) {
		gitexec.SetStallBoundForTest(t, deadline/5)

		runs, recorded, err := remoteAttempts(t, "answering", "000eversion 2\\n")

		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) || gitErr.Stalled || gitErr.Timeout != deadline {
			t.Fatalf("Run error = %v; want a *GitError with Timeout %v that is not stalled", err, deadline)
		}
		if runs != 1 || len(recorded) != 1 {
			t.Errorf("runs = %d, kill reports = %d; want 1 of each", runs, len(recorded))
		}
	}) {
		return
	}

	if !t.Run("a remote command gets the low-speed environment and its own packet trace, over an operator's", func(t *testing.T) {
		const operatorTrace = "/operator/chosen/trace"
		t.Setenv("GIT_TRACE_PACKET", operatorTrace)
		_, err := gitexec.Run([]string{"-c", "protocol.ext.allow=always", "ls-remote", "ext::" + printEnv}, repoDir)
		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) {
			t.Fatalf("Run error = %v; want a *GitError from the failing ext command", err)
		}
		for _, want := range []string{"GIT_HTTP_LOW_SPEED_LIMIT=1000", "GIT_HTTP_LOW_SPEED_TIME=60", "GIT_OPTIONAL_LOCKS=0"} {
			if !strings.Contains(gitErr.Stderr, want) {
				t.Errorf("remote command environment lacks %s; stderr: %s", want, gitErr.Stderr)
			}
		}
		if !strings.Contains(gitErr.Stderr, "GIT_TRACE_PACKET=") {
			t.Errorf("remote command environment lacks GIT_TRACE_PACKET; stderr: %s", gitErr.Stderr)
		}
		if strings.Contains(gitErr.Stderr, operatorTrace) {
			t.Errorf("remote command environment keeps the operator's GIT_TRACE_PACKET; stderr: %s", gitErr.Stderr)
		}
	}) {
		return
	}

	t.Run("a local command is neither bounded nor given the low-speed environment or a packet trace", func(t *testing.T) {
		// t.Setenv registers the restore of the operator's value; the unset keeps an exported GIT_TRACE_PACKET out of the child.
		t.Setenv("GIT_TRACE_PACKET", "")
		if err := os.Unsetenv("GIT_TRACE_PACKET"); err != nil {
			t.Fatalf("unset GIT_TRACE_PACKET: %v", err)
		}
		stdout, err := gitexec.Run([]string{"-c", "alias.slowenv=!sleep 2 && env", "slowenv"}, repoDir)
		if err != nil {
			t.Fatalf("Run error = %v; want a local command to run past the remote deadline", err)
		}
		if strings.Contains(stdout, "GIT_HTTP_LOW_SPEED") {
			t.Errorf("local command environment holds a low-speed variable; stdout: %s", stdout)
		}
		if strings.Contains(stdout, "GIT_TRACE_PACKET") {
			t.Errorf("local command environment holds GIT_TRACE_PACKET; stdout: %s", stdout)
		}
	})
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
		_, _, exitCode, rawErr := gitexec.RunGit([]string{"--version"}, dir)
		checkErr := gitexec.CheckFloor()

		for name, err := range map[string]error{"Run": runErr, "RunStdin": stdinErr, "RunGit": rawErr, "CheckFloor": checkErr} {
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
