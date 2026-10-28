//go:build tier2

// tmuxkit_integration_linux_test.go holds the tmuxkit integration tests that find processes through /proc, which only Linux has.

package tmuxkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/proc"
)

// TestSweep_KillsServersAndPaneTreesUnderItsDirectoryOnly pins the sweep's reach: the server under its directory goes, with a pane child that ignores the hangup `kill-server` sends, and a server elsewhere stays.
func TestSweep_KillsServersAndPaneTreesUnderItsDirectoryOnly(t *testing.T) {
	tmux := requireTmux(t)
	dir, err := lymtemp.MkdirTemp(dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// A sleep length unique to this process names the pane child among every process on the host.
	payload := []string{"sleep", strconv.Itoa(1_000_000 + os.Getpid())}
	immune := fmt.Sprintf("trap '' HUP; %s & wait", strings.Join(payload, " "))
	start := exec.Command(tmux, "-L", "swept", "new-session", "-d", "-s", "kit", "sh", "-c", immune)
	start.Env = append(os.Environ(), "TMUX_TMPDIR="+dir)
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start server: %v\n%s", err, out)
	}
	has := exec.Command(tmux, "-L", "swept", "has-session")
	has.Env = start.Env
	if err := has.Run(); err != nil {
		t.Fatalf("server not running before the sweep: %v", err)
	}
	runsPayload := func(pid int) bool {
		argv, ok := ProcArgv(pid)
		return ok && slices.Equal(argv, payload)
	}
	child := 0
	for deadline := time.Now().Add(5 * time.Second); child == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		for _, pid := range Pids() {
			if runsPayload(pid) {
				child = pid
				t.Cleanup(func() { _ = proc.KillPID(pid) })
			}
		}
	}
	if child == 0 {
		t.Fatalf("pane child %q never started", payload)
	}

	other := Socket(t, tmux)
	if out, err := exec.Command(tmux, "-L", other, "new-session", "-d", "-s", "kit").CombinedOutput(); err != nil {
		t.Fatalf("start second server: %v\n%s", err, out)
	}

	sweep(dir, os.Getuid())

	after := exec.Command(tmux, "-L", "swept", "has-session")
	after.Env = start.Env
	if after.Run() == nil {
		t.Error("server under the swept directory survived the sweep")
	}
	// A killed process lingers until the kernel delivers the signal, so its exit is awaited briefly.
	for deadline := time.Now().Add(2 * time.Second); runsPayload(child); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Errorf("pane child %d ignoring the hangup survived the sweep", child)
			break
		}
	}
	if !hasServer(tmux, other) {
		t.Error("the sweep killed a server outside its own directory")
	}
	if _, err := os.Lstat(filepath.Join(socketDir(dir, os.Getuid()), "swept")); !os.IsNotExist(err) {
		t.Errorf("socket file under the swept directory survived the sweep: %v", err)
	}
	if _, err := os.Lstat(socketPath(other)); err != nil {
		t.Errorf("the sweep removed a socket file outside its own directory: %v", err)
	}
}

func TestAfterRun_LeftoverProcesses(t *testing.T) {
	t.Parallel()
	dir, err := lymtemp.MkdirTemp(dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	start := func(args ...string) *exec.Cmd {
		cmd := exec.Command("sh", args...)
		cmd.Dir = dir
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %v: %v", args, err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		return cmd
	}
	stray := start("-c", "sleep 60")
	watchdog := start("-c", "sleep 60", "x", "reed", "watchdog")

	var out strings.Builder
	code := afterRun(&out, dir, 0, 100*time.Millisecond, func() {}, func() error { return os.RemoveAll(dir) })

	if code != 1 {
		t.Errorf("afterRun code = %d; want 1 for a stray process", code)
	}
	if want := fmt.Sprintf("tmuxkit: process %d left running by the package in %s: %q\n", stray.Process.Pid, dir, []string{"sh", "-c", "sleep 60"}); !strings.Contains(out.String(), want) {
		t.Errorf("output %q does not name the stray process by pid, cwd and argv (want %q)", out.String(), want)
	}
	if strings.Contains(out.String(), fmt.Sprintf("process %d left running", watchdog.Process.Pid)) {
		t.Errorf("output %q names the watchdog process, which must not fail the package", out.String())
	}
	for name, cmd := range map[string]*exec.Cmd{"stray": stray, "watchdog": watchdog} {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s process exited cleanly; want it killed by afterRun", name)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("%s process still running after afterRun", name)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory survived afterRun: %v", err)
	}

	for _, tt := range []struct {
		argv []string
		want bool
	}{
		{[]string{"lym", "reed", "watchdog", "--hub", "/x"}, true},
		{[]string{"sh", "-c", "sleep 60", "x", "reed", "watchdog"}, true},
		{[]string{"lym", "reed", "up"}, false},
		{[]string{"watchdog", "reed"}, false},
		{[]string{"reed"}, false},
		{nil, false},
	} {
		if got := IsWatchdog(tt.argv); got != tt.want {
			t.Errorf("IsWatchdog(%q) = %v; want %v", tt.argv, got, tt.want)
		}
	}
}
