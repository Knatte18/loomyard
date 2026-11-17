//go:build !windows

// pluginhook_integration_test.go runs RunPluginHook against real `sh` commands.
// It pins the output, the failure and timeout errors, and that a timeout ends a child the hook's shell forked.

package claudeengine

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pluginHookDeadline is the generous bound a test puts on a call that must return, far past any load.
const pluginHookDeadline = 60 * time.Second

// TestRunPluginHook pins the real hook runner.
// A timed-out command returns long before its own sleep ends, and the group kill takes the forked child with it.
func TestRunPluginHook(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name       string
		command    string
		stdin      string
		timeout    time.Duration
		wantStdout string
		// wantErr is a text the error holds; empty wants none.
		wantErr string
		// killsForkedChild also checks that the child the shell forked is gone after the call.
		killsForkedChild bool
	}{
		{
			name: "ReturnsItsStdinAndPluginRoot", command: `cat; printf '%s' "$CLAUDE_PLUGIN_ROOT"`, stdin: "payload",
			timeout: 30 * time.Second, wantStdout: "payload/plugin",
		},
		{name: "NonZeroExitNamesItsStderr", command: `echo boom >&2; exit 3`, timeout: 30 * time.Second, wantErr: "boom"},
		{name: "TimeoutEndsAForkedSleep", command: `sleep 120; true`, timeout: 100 * time.Millisecond, wantErr: "timed out"},
		{
			name: "TimeoutKillsTheForkedChild", command: `sleep 120 & echo $! > "$PIDFILE"; wait`, timeout: 5 * time.Second,
			wantErr: "timed out", killsForkedChild: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			run := shuttleengine.HookRun{
				Command: tt.command, Dir: t.TempDir(), Stdin: []byte(tt.stdin), Timeout: tt.timeout,
				Env: []string{"CLAUDE_PLUGIN_ROOT=/plugin", "PIDFILE=" + pidFile},
			}
			type result struct {
				stdout []byte
				err    error
			}
			done := make(chan result, 1)
			go func() {
				stdout, err := RunPluginHook(run)
				done <- result{stdout, err}
			}()
			var got result
			select {
			case got = <-done:
			case <-time.After(pluginHookDeadline):
				t.Fatalf("RunPluginHook(%q) did not return within %s", tt.command, pluginHookDeadline)
			}

			if tt.wantErr == "" {
				if got.err != nil || string(got.stdout) != tt.wantStdout {
					t.Fatalf("RunPluginHook(%q) = %q, %v; want %q and no error", tt.command, got.stdout, got.err, tt.wantStdout)
				}
			} else if got.err == nil || !strings.Contains(got.err.Error(), tt.wantErr) {
				t.Fatalf("RunPluginHook(%q) error = %v; want one holding %q", tt.command, got.err, tt.wantErr)
			}
			if tt.killsForkedChild {
				requireProcessGone(t, pidFile)
			}
		})
	}
}

// requireProcessGone reads a pid from pidFile and waits for that process to be gone.
func requireProcessGone(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read the forked child's pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("forked child's pid %q: %v", data, err)
	}
	deadline := time.After(pluginHookDeadline)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("forked child %d outlived the timeout's group kill", pid)
		case <-tick.C:
		}
	}
}
