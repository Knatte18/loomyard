//go:build scout

// supervised_integration_test.go is the central "prove supervised against a
// plain gopls" deliverable: it drives ensureSupervised directly against a
// real, held-open gopls daemon, proving the spawn-race lock, state-file
// recording, dial-or-reconnect, and staleness-triggered-restart behavior end
// to end at the strategy-internals level, independent of whichever entry
// point dispatches to it. Since the engine-supervised-flip batch,
// ensureServer now dispatches Go to ensureSupervised as its live V1
// strategy — TestEnsureServer_Integration_SupervisedDispatch in
// ensureserver_integration_test.go additionally proves that dispatch path
// end to end through ensureServer itself. //go:build scout-tagged
// and therefore excluded from the plain `go test` verify (the Test Tier
// Purity Invariant); it is run separately with `-tags scout` on a
// machine with gopls installed, alongside refs_integration_test.go and
// ensureserver_integration_test.go.

package scoutengine

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// killRecordedDaemon kills the daemon PID recorded in the state file, ensuring cleanup.
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

// TestEnsureSupervised_Integration verifies spawn, reconnect, and respawn behavior end-to-end.
func TestEnsureSupervised_Integration(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip(builtins()["go"].InstallHint)
	}

	const lang = "go"
	root := repoRoot(t)
	// A fresh temp worktree per test run keeps runs isolated from each
	// other and from any real worktree's own supervised daemon.
	worktreeRoot := t.TempDir()
	statePath := DaemonStateFile(worktreeRoot, lang)

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

	// (2) Second call, same anchorRoot/lang: must reconnect to the
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
