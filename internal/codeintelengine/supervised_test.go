// supervised_test.go covers ensureSupervised's state-file/lock/retry-
// exhaustion decision logic, mostly offline (no gopls spawn) — the
// readDaemonState+daemonStale combination itself is already covered by
// daemonstate_test.go and is not duplicated here. Its second sub-test is the
// exception: it needs a real bind attempt to prove the stale-socket cleanup
// step actually avoids EADDRINUSE, not just in a mocked scenario, so it is
// skip-gated on exec.LookPath("gopls") via t.Skip rather than build-tag-
// gated like supervised_integration_test.go — it still runs as part of a
// plain `go test` whenever gopls happens to be on $PATH.
//
// Both sub-tests below spawn a real, short-lived child process (one to
// obtain a confirmed-alive PID fixture for the retry-exhaustion path, one a
// real gopls for the stale-socket-cleanup bind proof), tripping the Test
// Tier Purity Invariant guard; this file is allowlisted at the file level in
// cmd/lyx/tierpurity_test.go's allowedSpawners map.

package codeintelengine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
)

// spawnAndHoldSubprocess starts a child process that blocks for several
// seconds and registers a t.Cleanup to kill and reap it, returning its PID
// while it is still alive. Unlike daemonstate_test.go's
// spawnAndWaitForDeadPID (which needs a confirmed-dead PID), this fixture
// needs a PID that stays alive for the sub-test's own short duration, so the
// child process here is deliberately held open rather than waited on
// immediately.
func spawnAndHoldSubprocess(t *testing.T) int {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "ping", "-n", "6", "127.0.0.1")
	} else {
		cmd = exec.Command("sleep", "5")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

// TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout asserts
// that when ensureSupervised can neither reuse a recorded daemon (its
// address is unreachable) nor win the spawn lock (this test holds it for
// the sub-test's own duration), it returns within its timeout bound rather
// than hanging, with an error satisfying errors.Is(err,
// ErrServerSpawnTimeoutSentinel) — proving the bounded-retry contract
// rather than indefinite blocking.
func TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout(t *testing.T) {
	worktreeRoot := t.TempDir()
	const lang = "go"
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	statePath := layout.CodeintelDaemonStateFile(lang)
	lockPath := layout.CodeintelDaemonLock(lang)
	socketPath := filepath.Join(filepath.Dir(statePath), "daemon.sock")

	// Record a state that reads as healthy (a confirmed-alive PID, the
	// current protocol version) but whose recorded address nothing is
	// listening on, so every dial-and-finalize attempt in step 1 fails.
	pid := spawnAndHoldSubprocess(t)
	if err := writeDaemonState(statePath, daemonState{
		PID:             pid,
		Address:         "unix;" + socketPath,
		ProtocolVersion: supervisedProtocolVersion,
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("writeDaemonState() failed: %v", err)
	}

	// Pre-acquire the spawn lock ourselves and hold it for the sub-test's
	// duration, so ensureSupervised can never win it either — every retry
	// iteration must fall into the "lock not acquired" branch.
	fileLock, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("lock.AcquireWriteLock() failed: %v", err)
	}
	t.Cleanup(func() { _ = fileLock.Release() })

	const timeout = 300 * time.Millisecond
	start := time.Now()
	// command is never reached: this call can neither dial (unreachable
	// address) nor win the lock (held above), so it must exhaust its retry
	// budget before ever attempting to spawn anything.
	_, err = ensureSupervised(context.Background(), []string{"lyx-codeintel-should-never-spawn"}, lang, worktreeRoot, worktreeRoot, timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("ensureSupervised() with an unreachable daemon and a held lock returned nil error; want ErrServerSpawnTimeout")
	}
	if !errors.Is(err, ErrServerSpawnTimeoutSentinel) {
		t.Errorf("ensureSupervised() err = %v; want errors.Is(err, ErrServerSpawnTimeoutSentinel)", err)
	}
	// Generous upper bound: the retry loop must return once its deadline
	// passes, not hang indefinitely. A few seconds is ample margin over the
	// 300ms timeout and the loop's own 100ms poll interval.
	if elapsed > 5*time.Second {
		t.Errorf("ensureSupervised() took %s to return after a %s timeout; want it bounded by the timeout, not hanging", elapsed, timeout)
	}
}

// TestEnsureSupervised_StaleSocketCleanupAllowsRebind asserts that a
// leftover non-socket regular file at the deterministic socketPath (e.g.
// left behind by a daemon that crashed without cleaning up after itself)
// does not block a fresh spawn's bind. This exercises ensureSupervised's
// unconditional os.Remove(socketPath) cleanup against a real gopls listen
// socket, not a mocked scenario — only a real bind attempt can prove
// EADDRINUSE was actually avoided, which is the trade-off for needing gopls
// on $PATH to run at all.
func TestEnsureSupervised_StaleSocketCleanupAllowsRebind(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip(builtins()["go"].InstallHint)
	}

	worktreeRoot := t.TempDir()
	const lang = "go"
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	statePath := layout.CodeintelDaemonStateFile(lang)
	socketPath := filepath.Join(filepath.Dir(statePath), "daemon.sock")

	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%s) failed: %v", filepath.Dir(socketPath), err)
	}
	// A leftover empty regular file, not a real socket — exactly what
	// ensureSupervised's unconditional os.Remove(socketPath) must clear
	// before the fresh gopls process binds, or the bind fails with
	// EADDRINUSE.
	if err := os.WriteFile(socketPath, nil, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%s) failed: %v", socketPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := ensureSupervised(ctx, []string{"gopls"}, lang, worktreeRoot, worktreeRoot, 30*time.Second)
	if err != nil {
		t.Fatalf("ensureSupervised() returned unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("ensureSupervised() returned a nil client with a nil error")
	}
	// The daemon is meant to outlive this call; per the connKindSupervised
	// teardown rule a real caller must never close()/kill() it, but a test
	// must still reap the process it spawned so repeated test runs don't
	// accumulate stray gopls processes.
	t.Cleanup(func() {
		if state, found, _ := readDaemonState(statePath); found {
			if p, err := os.FindProcess(state.PID); err == nil {
				_ = p.Kill()
			}
		}
	})

	state, found, err := readDaemonState(statePath)
	if err != nil {
		t.Fatalf("readDaemonState() failed: %v", err)
	}
	if !found {
		t.Fatal("readDaemonState() found = false after ensureSupervised() succeeded; want true")
	}
	if want := "unix;" + socketPath; state.Address != want {
		t.Errorf("state.Address = %q; want %q (the deterministic socket path, successfully rebound after cleanup)", state.Address, want)
	}
}
