// supervised_test.go now covers exactly ensureSupervised's state-file/lock/ retry-exhaustion
// decision logic across its three remaining subtests —
// TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout,
// TestEnsureSupervised_UncontendedLockWithUndialableHealthyStateReturnsErrServerSpawnTimeout,
// TestEnsureSupervised_WedgedEscalationReuseReleasesLock — entirely offline (no gopls spawn);
// the two previously gopls-gated subtests (TestEnsureSupervised_StaleSocketCleanupAllowsRebind,
// TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr) have moved to the new //go:build
// scout-tagged supervised_scout_test.go in this same package.
//
// Each remaining subtest spawns one real short-lived child process via spawnAndHoldSubprocess only
// as a PID-liveness fixture (allowlisted in cmd/lyx/tierpurity_test.go's allowedSpawners map),
// never gopls itself.

package scoutengine

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/proc"
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

// TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout asserts that when
// ensureSupervised can neither reuse a recorded daemon (its address is unreachable) nor win the
// spawn lock (this test holds it for the sub-test's own duration), it returns within its timeout
// bound rather than hanging, with an error satisfying errors.Is(err, ErrServerSpawnTimeoutSentinel)
// — proving the bounded-retry contract rather than indefinite blocking.
func TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout(t *testing.T) {
	worktreeRoot := t.TempDir()
	const lang = "go"
	statePath := DaemonStateFile(worktreeRoot, lang)
	lockPath := DaemonLock(worktreeRoot, lang)
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
	_, err = ensureSupervised(context.Background(), []string{"lyx-scout-should-never-spawn"}, lang, worktreeRoot, worktreeRoot, timeout)
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

// TestEnsureSupervised_UncontendedLockWithUndialableHealthyStateReturnsErrServerSpawnTimeout
// asserts that ensureSupervised is still bounded by its deadline when the spawn lock is never
// contended at all — the "wedged daemon" scenario the function's own doc comment names: a recorded
// state that reads healthy (alive PID, current protocol version) but whose address is never
// actually dialable.
// This sub-test now drives the wedged-daemon re-dial-under-lock escalation (card 11): step 1's dial
// fails against the non-stale state, the uncontended lock is acquired immediately, and the
// escalation's own re-dial-under-lock retry (~500ms worst case) is what actually consumes this
// test's 300ms deadline — the deadline guard placed immediately after that retry is what returns
// ErrServerSpawnTimeout here, before ever attempting proc.KillPID against the held fixture PID.
// Unlike TestEnsureSupervised_RetryExhaustionReturnsErrServerSpawnTimeout (which pre-holds the lock
// so every retry falls into the "lock not acquired" branch, step 2), this sub-test never touches
// the lock itself, so it reaches the escalation branch on its very first iteration.
func TestEnsureSupervised_UncontendedLockWithUndialableHealthyStateReturnsErrServerSpawnTimeout(t *testing.T) {
	worktreeRoot := t.TempDir()
	const lang = "go"
	statePath := DaemonStateFile(worktreeRoot, lang)
	lockPath := DaemonLock(worktreeRoot, lang)
	socketPath := filepath.Join(filepath.Dir(statePath), "daemon.sock")

	// Record a state that reads as healthy (a confirmed-alive PID, the
	// current protocol version) but whose recorded address nothing is
	// listening on, so every dial-and-finalize attempt in step 1 fails —
	// and the lock is left free, so the escalation acquires it immediately
	// rather than falling into step 2's !acquired branch.
	pid := spawnAndHoldSubprocess(t)
	if err := writeDaemonState(statePath, daemonState{
		PID:             pid,
		Address:         "unix;" + socketPath,
		ProtocolVersion: supervisedProtocolVersion,
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("writeDaemonState() failed: %v", err)
	}

	const timeout = 300 * time.Millisecond
	start := time.Now()
	// command is never reached: this call always finds a healthy-reading
	// but undialable state, so it must exhaust its retry budget without
	// ever attempting to spawn anything.
	_, err := ensureSupervised(context.Background(), []string{"lyx-scout-should-never-spawn"}, lang, worktreeRoot, worktreeRoot, timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("ensureSupervised() with an uncontended lock and an undialable healthy state returned nil error; want ErrServerSpawnTimeout")
	}
	if !errors.Is(err, ErrServerSpawnTimeoutSentinel) {
		t.Errorf("ensureSupervised() err = %v; want errors.Is(err, ErrServerSpawnTimeoutSentinel)", err)
	}
	// Generous upper bound: before the fix, this path had no deadline check
	// at all and would spin indefinitely rather than return here.
	if elapsed > 5*time.Second {
		t.Errorf("ensureSupervised() took %s to return after a %s timeout; want it bounded by the timeout, not hanging", elapsed, timeout)
	}

	// The deadline-timeout return inside the escalation must release the
	// spawn lock before returning, exactly like every other return path —
	// a follow-up TryAcquireWriteLock succeeding proves it did.
	freshLock, acquired, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("lock.TryAcquireWriteLock() after ensureSupervised() timeout failed: %v", err)
	}
	if !acquired {
		t.Error("lock.TryAcquireWriteLock() after ensureSupervised()'s deadline-timeout return acquired = false; want true (lock released)")
	} else {
		_ = freshLock.Release()
	}

	// The deadline guard fires before proc.KillPID is ever attempted, so the
	// held fixture PID must still be alive.
	if !proc.IsAlive(pid) {
		t.Error("ensureSupervised()'s deadline-timeout escalation killed the held fixture PID; want the deadline guard to fire first, before any KillPID")
	}
}

// TestEnsureSupervised_WedgedEscalationReuseReleasesLock asserts the wedged-daemon escalation's
// reuse-success return path (card 11): step 1's dial-then-finalizeConnection fails against a
// non-stale state (the fake server answers the first accepted connection's initialize but fails its
// workspace/symbol probe), triggering the escalation, which acquires the uncontended spawn lock and
// re-dials the same recorded address under the lock — this time succeeding (the fake server answers
// the second accepted connection's initialize and probe both successfully), so ensureSupervised
// must reuse that connection rather than concluding the daemon is wedged.
// This proves both that the reuse-success path frees the spawn lock and that it never calls
// proc.KillPID against the held fixture PID.
func TestEnsureSupervised_WedgedEscalationReuseReleasesLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Mirrors lspclient_test.go's own unix-socket skip: this is a
		// scoping choice (TCP is covered elsewhere via newLSPClientDial's
		// address-form flexibility), not an oversight.
		t.Skip("unix sockets not exercised on windows here")
	}

	worktreeRoot := t.TempDir()
	const lang = "go"
	statePath := DaemonStateFile(worktreeRoot, lang)
	lockPath := DaemonLock(worktreeRoot, lang)
	socketPath := filepath.Join(filepath.Dir(statePath), "daemon.sock")

	// A non-stale state (alive PID, current protocol version) naming the
	// socket the fake server below binds — should the escalation ever
	// wrongly take the wedged branch, proc.KillPID would hit this held
	// fixture PID, never the test process itself.
	pid := spawnAndHoldSubprocess(t)
	if err := writeDaemonState(statePath, daemonState{
		PID:             pid,
		Address:         "unix;" + socketPath,
		ProtocolVersion: supervisedProtocolVersion,
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("writeDaemonState() failed: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%s) failed: %v", filepath.Dir(socketPath), err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Listen(unix, %s) failed: %v", socketPath, err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)

		// First accepted connection: this is step 1's dial-then-
		// finalizeConnection attempt. Answer initialize, then fail the
		// follow-up workspace/symbol probe with an LSP error response, so
		// finalizeConnection fails and ensureSupervised escalates.
		conn1, err := listener.Accept()
		if err != nil {
			t.Errorf("listener.Accept() (1st conn) failed: %v", err)
			return
		}
		server1 := newFakeServer(conn1)
		req, ok := server1.readMessage(t)
		if !ok {
			conn1.Close()
			return
		}
		if req.Method != "initialize" {
			t.Errorf("fakeServer (1st conn): got request method %q; want %q", req.Method, "initialize")
			conn1.Close()
			return
		}
		if !server1.respond(t, req.ID, map[string]any{"capabilities": map[string]any{}}) {
			conn1.Close()
			return
		}
		server1.readMessage(t) // initialized notification
		probeReq, ok := server1.readMessage(t)
		if !ok {
			conn1.Close()
			return
		}
		if probeReq.Method != "workspace/symbol" {
			t.Errorf("fakeServer (1st conn): got request method %q; want %q", probeReq.Method, "workspace/symbol")
			conn1.Close()
			return
		}
		server1.writeMessage(t, map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(probeReq.ID),
			"error":   map[string]any{"code": -32603, "message": "boom"},
		})
		conn1.Close()

		// Second accepted connection: the escalation's own under-lock
		// re-dial. Answer both initialize and the probe successfully, so
		// reconnectUnderLock reports healthy and ensureSupervised reuses
		// this connection instead of killing and respawning.
		conn2, err := listener.Accept()
		if err != nil {
			t.Errorf("listener.Accept() (2nd conn) failed: %v", err)
			return
		}
		defer conn2.Close()
		server2 := newFakeServer(conn2)
		req2, ok := server2.readMessage(t)
		if !ok {
			return
		}
		if req2.Method != "initialize" {
			t.Errorf("fakeServer (2nd conn): got request method %q; want %q", req2.Method, "initialize")
			return
		}
		if !server2.respond(t, req2.ID, map[string]any{"capabilities": map[string]any{}}) {
			return
		}
		server2.readMessage(t) // initialized notification
		probeReq2, ok := server2.readMessage(t)
		if !ok {
			return
		}
		if probeReq2.Method != "workspace/symbol" {
			t.Errorf("fakeServer (2nd conn): got request method %q; want %q", probeReq2.Method, "workspace/symbol")
			return
		}
		server2.respond(t, probeReq2.ID, []map[string]any{})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// command is never reached: the escalation reuses the second
	// connection, so no respawn is ever attempted.
	client, err := ensureSupervised(ctx, []string{"lyx-scout-should-never-spawn"}, lang, worktreeRoot, worktreeRoot, 5*time.Second)
	<-done
	if err != nil {
		t.Fatalf("ensureSupervised() returned unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("ensureSupervised() returned a nil client with a nil error")
	}
	// kill() only tears down this test's own dialed connection to the fake
	// server above; it has no bearing on the held fixture PID.
	defer client.kill()

	if !proc.IsAlive(pid) {
		t.Error("ensureSupervised()'s reuse-success escalation killed the held fixture PID; want it left alive (the daemon was never actually wedged)")
	}

	// The reuse-success return path must release the spawn lock before
	// returning, exactly like every other return path.
	freshLock, acquired, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("lock.TryAcquireWriteLock() after ensureSupervised()'s reuse-success return failed: %v", err)
	}
	if !acquired {
		t.Error("lock.TryAcquireWriteLock() after ensureSupervised()'s reuse-success return acquired = false; want true (lock released)")
	} else {
		_ = freshLock.Release()
	}
}
