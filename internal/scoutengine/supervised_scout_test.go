//go:build scout

// supervised_scout_test.go holds this package's two subtests that need a
// real, already-installed gopls on $PATH to prove a real bind/log-file
// behavior — split out of supervised_test.go because, unlike that file's
// other three subtests, these two cannot run offline. Manual invocation
// only via `go test -tags scout ./...`, no runtime skip-gate (the build tag
// is the only gate).

package scoutengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestEnsureSupervised_StaleSocketCleanupAllowsRebind verifies stale sockets are cleaned up before
// rebind.
func TestEnsureSupervised_StaleSocketCleanupAllowsRebind(t *testing.T) {
	worktreeRoot := t.TempDir()
	const lang = "go"
	statePath := DaemonStateFile(worktreeRoot, lang)
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

// TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr is the regression test for the fd-leak
// this task fixed: a DetachBreakaway'd daemon whose Stderr is wired to the spawning process's own
// os.Stderr inherits that process's fds, including any pipe it happens to be part of.
// A reader on the other end of that pipe exiting before the daemon does (e.g.
// a caller piped through "| head") leaves the daemon holding a write end nobody reads — the
// daemon's next stderr write raises SIGPIPE and kills it, silently defeating DetachBreakaway's
// entire point (reproduced live: a spawn behind "2>&1 | tail -5" hung until "tail" was killed, at
// which point the daemon itself crashed on its next log line).
// Proving the daemon's diagnostics land in its own log file — never in this test process's
// os.Stderr — is what a caller-fd-independence regression test can assert deterministically;
// SIGPIPE-on-a-dead-reader is not something this suite reproduces directly.
func TestEnsureSupervised_DaemonLogsToOwnFileNotCallersStderr(t *testing.T) {
	worktreeRoot := t.TempDir()
	const lang = "go"
	statePath := DaemonStateFile(worktreeRoot, lang)
	logPath := filepath.Join(filepath.Dir(statePath), "daemon.log")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := ensureSupervised(ctx, []string{"gopls"}, lang, worktreeRoot, worktreeRoot, 30*time.Second)
	if err != nil {
		t.Fatalf("ensureSupervised() returned unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("ensureSupervised() returned a nil client with a nil error")
	}
	t.Cleanup(func() {
		if state, found, _ := readDaemonState(statePath); found {
			if p, err := os.FindProcess(state.PID); err == nil {
				_ = p.Kill()
			}
		}
	})

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("os.Stat(%s) failed: %v; want the daemon's own log file to exist beside its state file", logPath, err)
	}
	if info.Size() == 0 {
		// gopls always logs at least its own "listening on" banner on
		// startup — an empty file means nothing was ever routed there,
		// exactly the symptom of stderr going to the wrong fd instead.
		t.Errorf("daemon log file %s is empty; want gopls's startup diagnostics to have been written to it", logPath)
	}
}
