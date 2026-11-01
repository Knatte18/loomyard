//go:build tier1 && !windows

// run_tied_integration_test.go holds the tests of RunTied: the process-group record it reports and the parent-death tie of its shell.

package verifyrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

// waitForPidFile polls for a pid file a process writes and returns the pid it holds.
func waitForPidFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && convErr == nil {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no pid written to %s", path)
	return 0
}

// waitForGone fails the test unless pid is gone within a deadline.
func waitForGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d still alive", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunTied(t *testing.T) {
	t.Parallel()

	t.Run("StartedReceivesTheShellGroupAndTheExitCodeIsTheCommands", func(t *testing.T) {
		t.Parallel()
		var got proc.TreeRecord
		var calls int
		code, err := RunTied(context.Background(), "exit 7", t.TempDir(), nil, &bytes.Buffer{}, func(record proc.TreeRecord) {
			got = record
			calls++
		})
		if err != nil || code != 7 {
			t.Fatalf("RunTied = (%d, %v); want (7, nil)", code, err)
		}
		if calls != 1 || got.PID <= 0 || got.PGID != got.PID || got.StartTime == "" {
			t.Errorf("started got %d calls, record %+v; want one call with PGID == PID and a start time", calls, got)
		}
	})

	t.Run("KillTreeOnTheRecordEndsABackgroundedDescendant", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		pidFile := filepath.Join(dir, "descendant.pid")
		recorded := make(chan proc.TreeRecord, 1)
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = RunTied(context.Background(), "sleep 30 & echo $! > "+pidFile+"; wait", dir, nil, &bytes.Buffer{}, func(record proc.TreeRecord) {
				recorded <- record
			})
		}()
		record := <-recorded
		descendant := waitForPidFile(t, pidFile)

		killed, err := proc.KillTree(record)
		if err != nil || !killed {
			t.Fatalf("KillTree(%+v) = (%v, %v); want (true, nil)", record, killed, err)
		}
		waitForGone(t, descendant)
		<-done
	})

	t.Run("KillingTheCallingProcessEndsTheShell", func(t *testing.T) {
		t.Parallel()
		pidFile := filepath.Join(t.TempDir(), "shell.pid")
		helper := exec.Command(os.Args[0])
		helper.Env = append(os.Environ(), helperRoleEnv+"="+roleRunTied, helperPidFileEnv+"="+pidFile)
		if err := helper.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = helper.Process.Kill(); _ = helper.Wait() })
		shell := waitForPidFile(t, pidFile)

		if err := helper.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		waitForGone(t, shell)
	})
}
