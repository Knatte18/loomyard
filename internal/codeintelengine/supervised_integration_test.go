//go:build integration

// supervised_integration_test.go is the central "prove supervised against a
// plain gopls" deliverable: it drives ensureSupervised directly (never
// through ensureServer's dispatch, which has no live path to it in V1 per
// the "EnsureServer has exactly one live dispatch arm" Shared Decision)
// against a real, held-open gopls daemon, proving the spawn-race lock,
// state-file recording, dial-or-reconnect, and staleness-triggered-restart
// behavior end to end before any language that actually needs the strategy
// exists. //go:build integration-tagged and therefore excluded from the
// plain `go test` verify (the Test Tier Purity Invariant); it is run
// separately with `-tags integration` on a machine with gopls installed,
// alongside refs_integration_test.go and
// ensureserver_integration_test.go.

package codeintelengine

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// killRecordedDaemon reads the daemon PID from the state file at statePath
// and kills-then-waits it, ignoring a missing state file or an
// already-dead process. Wait (not just Kill) matters here: the daemon is
// this test process's own child (ensureSupervised's spawn is never
// cmd.Wait()'d in production, since the daemon is meant to outlive the
// call), so without reaping it the killed process lingers as a zombie whose
// PID would still pass a signal-0 liveness check — Wait ensures the
// respawn sub-test's daemonStale check sees a genuinely gone PID, and
// doubles as best-effort cleanup so repeated test runs don't accumulate
// stray gopls processes.
func killRecordedDaemon(t *testing.T, statePath string) {
	t.Helper()
	state, found, err := readDaemonState(statePath)
	if err != nil || !found {
		return
	}
	process, err := os.FindProcess(state.PID)
	if err != nil {
		return
	}
	_ = process.Kill()
	_, _ = process.Wait()
}

// TestEnsureSupervised_Integration drives ensureSupervised three times
// against a real gopls, in the same worktreeRoot/lang throughout: (1) a
// first call with no daemon yet, which must spawn one and hand back a
// client that actually answers a real workspace/symbol query; (2) a second
// call, which must reconnect to the same daemon (unchanged PID) rather than
// spawning a second one; (3) a third call after the daemon is killed out
// from under it, which must detect the dead PID via daemonStale, respawn,
// and land on the same deterministic socket address despite the PID
// changing — the proof that the stale-socket-cleanup-before-rebind logic
// (ensureSupervised step 4) actually avoids EADDRINUSE on a real bind.
func TestEnsureSupervised_Integration(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip(builtins()["go"].InstallHint)
	}

	const lang = "go"
	root := repoRoot(t)
	// A fresh temp worktreeRoot per test run keeps runs isolated from each
	// other and from any real worktree's own supervised daemon.
	worktreeRoot := t.TempDir()
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	statePath := layout.CodeintelDaemonStateFile(lang)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// (1) First call: no daemon exists yet, so this call must spawn one.
	client, err := ensureSupervised(ctx, []string{"gopls"}, lang, root, worktreeRoot, 30*time.Second)
	if err != nil {
		t.Fatalf("ensureSupervised() first call returned unexpected error: %v", err)
	}
	t.Cleanup(func() { killRecordedDaemon(t, statePath) })

	state, found, err := readDaemonState(statePath)
	if err != nil {
		t.Fatalf("readDaemonState() failed: %v", err)
	}
	if !found {
		t.Fatal("readDaemonState() found = false after the first ensureSupervised() call; want true")
	}
	if state.ProtocolVersion != supervisedProtocolVersion {
		t.Errorf("state.ProtocolVersion = %q; want %q", state.ProtocolVersion, supervisedProtocolVersion)
	}
	firstPID := state.PID

	// Prove the returned client actually works end to end, not merely that
	// initialize/probe succeeded: issue a real workspace/symbol query
	// against this module's own source.
	symbols, err := client.workspaceSymbol(ctx, "Resolve")
	if err != nil {
		t.Fatalf("workspaceSymbol(%q) returned unexpected error: %v", "Resolve", err)
	}
	if len(symbols) == 0 {
		t.Error("workspaceSymbol(\"Resolve\") returned zero results against this module's own source; want at least one match")
	}

	// (2) Second call, same worktreeRoot/lang: must reconnect to the
	// existing daemon rather than spawning a second one.
	if _, err := ensureSupervised(ctx, []string{"gopls"}, lang, root, worktreeRoot, 30*time.Second); err != nil {
		t.Fatalf("ensureSupervised() second call returned unexpected error: %v", err)
	}

	state2, found, err := readDaemonState(statePath)
	if err != nil {
		t.Fatalf("readDaemonState() failed: %v", err)
	}
	if !found {
		t.Fatal("readDaemonState() found = false after the second ensureSupervised() call; want true")
	}
	if state2.PID != firstPID {
		t.Errorf("state.PID after the second ensureSupervised() call = %d; want unchanged %d (reconnect, not a second spawn)", state2.PID, firstPID)
	}
	if state2.Address != state.Address {
		t.Errorf("state.Address after the second ensureSupervised() call = %q; want unchanged %q", state2.Address, state.Address)
	}

	// (3) Kill the daemon directly, then call a third time: ensureSupervised
	// must detect the dead PID via daemonStale, respawn, and record a new
	// PID at the same deterministic socket address.
	killRecordedDaemon(t, statePath)

	if _, err := ensureSupervised(ctx, []string{"gopls"}, lang, root, worktreeRoot, 30*time.Second); err != nil {
		t.Fatalf("ensureSupervised() third call (after killing the daemon) returned unexpected error: %v", err)
	}

	state3, found, err := readDaemonState(statePath)
	if err != nil {
		t.Fatalf("readDaemonState() failed: %v", err)
	}
	if !found {
		t.Fatal("readDaemonState() found = false after the third ensureSupervised() call; want true")
	}
	if state3.PID == firstPID {
		t.Errorf("state.PID after killing the daemon and calling ensureSupervised() again = %d; want a new PID (respawn), not the killed one", state3.PID)
	}
	if state3.Address != state.Address {
		t.Errorf("state.Address after respawn = %q; want unchanged %q (the deterministic socket path, avoiding EADDRINUSE via stale-socket cleanup)", state3.Address, state.Address)
	}
}
