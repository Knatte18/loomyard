//go:build tier1 && !windows

// gitexec_unix_test.go holds the gitexec tests whose ext:: remote is a shell script.

package gitexec_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestRun_RemoteBounds drives the bounds on a remote git command through git's ext:: transport, a remote that accepts and never answers, with no network.
// Its rows run serially because they set the remote deadline, the stall bound, the kill reporter, the pause network gate and the process environment, which are process-global state.
func TestRun_RemoteBounds(t *testing.T) {
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

		runDir, dirErr := lymtemp.Dir()
		if dirErr != nil {
			t.Fatalf("lymtemp.Dir() error = %v", dirErr)
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
