//go:build tier1

// lymbin_integration_linux_test.go holds the lymbin group-kill test, which asserts liveness through /proc.

package lymbin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

// TestRun_KillsTheGroupOnTimeout ends the run only once the sleeper has recorded its pids,
// so the time a scanner holds the freshly built helper before it runs never counts against the timeout.
func TestRun_KillsTheGroupOnTimeout(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(t.TempDir(), "pids")
	t.Setenv(helperEnv, helperRoleSleeper)
	t.Setenv(helperPidFileEnv, pidFile)
	ctx, cancel := context.WithCancelCause(context.Background())
	t.Cleanup(func() { cancel(nil) })
	go cancelOnceFileExists(ctx, cancel, pidFile, 2*time.Minute)

	out, code, err := runUntilDone(ctx, self, t.TempDir(), nil)

	if !errors.Is(err, ErrTimeout) || code != -1 {
		t.Fatalf("runUntilDone = (%q, %d, %v); want ErrTimeout and exit code -1", out, code, err)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the sleeper never recorded its pids: %v", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		t.Fatalf("pid file = %q; want the sleeper's pid and its child's", raw)
	}
	for _, field := range fields {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for proc.IsAlive(pid) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if proc.IsAlive(pid) {
			t.Errorf("pid %d survived the timeout", pid)
		}
	}
}
