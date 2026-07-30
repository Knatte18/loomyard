//go:build integration

// ensureserver_integration_test.go exercises ensureNative against a real,
// network-installed gopls, mirroring refs_integration_test.go's and
// toolchain_integration_test.go's //go:build integration-tagged,
// t.Skip(builtins()["go"].InstallHint)-gated style: it is excluded from the
// plain `go test` verify (the Test Tier Purity Invariant) and run
// separately with `-tags integration`. Even though ensureNative itself
// ignores $PATH and resolves its own toolchain-managed binary, the skip
// gate here is about whether this machine can plausibly run a real gopls at
// all (network + `go install` capability), which exec.LookPath("gopls") is
// a reasonable, cheap proxy for reusing rather than inventing a second
// capability probe. This test only spawns gopls, never git, so no
// TestMain/lyxtest.HermeticGitEnv is required per the Hermetic Git Test
// Environment Invariant.
//
// It also now covers ensureServer's supervised dispatch: since the
// engine-supervised-flip batch, ensureServer routes Go through
// ensureSupervised first, falling back to ensureNative (still directly
// exercised above) only on failure. TestEnsureServer_Integration_
// SupervisedDispatch below drives ensureServer itself end to end and
// asserts it lands on the supervised strategy and reuses one daemon across
// calls, complementing supervised_integration_test.go's direct
// ensureSupervised coverage.

package traceengine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// errNoWorkspaceSymbolCandidates reports that a workspace/symbol query
// returned zero candidates when the shared-daemon regression test below
// requires at least one to compare across both connections.
var errNoWorkspaceSymbolCandidates = errors.New("workspace/symbol returned zero candidates; want at least one")

// TestEnsureNative_Integration proves the toolchain-resolve -> argv-build ->
// spawn -> initialize -> probe chain ensureNative wires together works end
// to end against a real gopls, not just its individually-mocked pieces
// (unit-tested separately in ensureserver_test.go and
// toolchain_integration_test.go).
func TestEnsureNative_Integration(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip(builtins()["go"].InstallHint)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := ensureNative(ctx, "go", builtins()["go"], repoRoot(t), 30*time.Second)
	if err != nil {
		t.Fatalf("ensureNative() returned unexpected error: %v", err)
	}
	defer client.kill()

	if client == nil {
		t.Fatal("ensureNative() returned a nil client with a nil error")
	}
	if client.closed {
		t.Error("ensureNative() returned a client with closed = true; want an open, ready-to-use connection")
	}
}

// TestEnsureNative_Integration_SharedDaemonWireCompatibility ports the exact
// empirical procedure _mill/discussion.md's
// native-strategy-wire-compatibility decision already ran manually, as an
// automated regression pin rather than a one-off manual check: two
// independent ensureNative calls, spawned a moment apart, each rooted at
// the same target directory. Both use -remote=auto under the hood, so both
// end up talking to the same shared gopls daemon. Each connection queries
// workspace/symbol for the literal, well-known, unique, package-level
// symbol "Resolve" (the same hubgeometry.Resolve symbol
// refs_integration_test.go's own findFuncPosition helper locates for its
// own test). A merely-nonempty result from each connection would not, by
// itself, be discriminating — two independent, unconnected gopls instances
// would each resolve the query correctly in isolation too. What only a
// genuinely shared index guarantees is that both connections resolve the
// query to the exact same location on every run, which is the assertion
// this test actually pins.
func TestEnsureNative_Integration_SharedDaemonWireCompatibility(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip(builtins()["go"].InstallHint)
	}

	root := repoRoot(t)
	entry := builtins()["go"]

	type result struct {
		location string
		err      error
	}
	results := make([]result, 2)

	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			client, err := ensureNative(ctx, "go", entry, root, 30*time.Second)
			if err != nil {
				results[i] = result{err: err}
				return
			}
			defer client.kill()

			symbolCtx, symbolCancel := context.WithTimeout(ctx, 30*time.Second)
			defer symbolCancel()
			candidates, err := client.workspaceSymbol(symbolCtx, "Resolve")
			if err != nil {
				results[i] = result{err: err}
				return
			}
			if len(candidates) == 0 {
				results[i] = result{err: errNoWorkspaceSymbolCandidates}
				return
			}
			results[i] = result{location: formatLocation(candidates[0].Location)}
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			t.Fatalf("connection %d: workspace/symbol(\"Resolve\") failed: %v", i, r.err)
		}
	}
	if results[0].location != results[1].location {
		t.Errorf("connection 0 resolved %q, connection 1 resolved %q; want identical first-candidate locations, proving both connections share one gopls daemon's index", results[0].location, results[1].location)
	}
}

// TestEnsureServer_Integration_SupervisedDispatch proves ensureServer's live
// dispatch decision end to end against a real gopls, not just the
// mocked-transport unit coverage in ensureserver_test.go: since Go's
// registry entry now dispatches to the supervised strategy first, a call
// through ensureServer (not ensureSupervised directly, which
// supervised_integration_test.go already covers) must come back with
// connKindSupervised, must have recorded a live daemon state file, and a
// second call against the same worktreeRoot must reuse that same daemon
// (identical PID, stable Address) rather than spawning a second one —
// mirroring TestEnsureSupervised_Integration's own reuse assertions, but
// through the exact entry point production code calls.
func TestEnsureServer_Integration_SupervisedDispatch(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip(builtins()["go"].InstallHint)
	}

	root := repoRoot(t)
	// A fresh temp worktreeRoot per test run keeps this test's daemon
	// isolated from any other integration test's own supervised daemon,
	// matching TestEnsureSupervised_Integration's own isolation.
	worktreeRoot := t.TempDir()
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	statePath := layout.TraceDaemonStateFile("go")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, kind, err := ensureServer(ctx, "go", builtins()["go"], root, worktreeRoot, 30*time.Second)
	if err != nil {
		t.Fatalf("ensureServer() returned unexpected error: %v", err)
	}
	// Per the connKindSupervised teardown rule, a supervised connection must
	// never be close()'d or kill()'d — the daemon it dials into is meant to
	// outlive this call. The state-file PID read below is what this test
	// reaps instead, exactly like supervised_integration_test.go's own
	// killRecordedDaemon.
	t.Cleanup(func() { killRecordedDaemon(t, statePath) })

	if kind != connKindSupervised {
		t.Errorf("ensureServer() connKind = %v; want connKindSupervised (native is now the fallback path only)", kind)
	}
	if client == nil {
		t.Fatal("ensureServer() returned a nil client with a nil error")
	}

	state, found, err := readDaemonState(statePath)
	if err != nil {
		t.Fatalf("readDaemonState() failed: %v", err)
	}
	if !found {
		t.Fatal("readDaemonState() found = false after ensureServer() succeeded; want true")
	}
	if _, err := os.FindProcess(state.PID); err != nil {
		t.Fatalf("os.FindProcess(%d) failed: %v", state.PID, err)
	}

	// A second call against the same worktreeRoot/lang must reuse the
	// existing daemon rather than spawning a second one.
	if _, _, err := ensureServer(ctx, "go", builtins()["go"], root, worktreeRoot, 30*time.Second); err != nil {
		t.Fatalf("ensureServer() second call returned unexpected error: %v", err)
	}

	state2, found, err := readDaemonState(statePath)
	if err != nil {
		t.Fatalf("readDaemonState() failed: %v", err)
	}
	if !found {
		t.Fatal("readDaemonState() found = false after the second ensureServer() call; want true")
	}
	if state2.PID != state.PID {
		t.Errorf("state.PID after the second ensureServer() call = %d; want unchanged %d (reuse, not a second spawn)", state2.PID, state.PID)
	}
	if state2.Address != state.Address {
		t.Errorf("state.Address after the second ensureServer() call = %q; want unchanged %q", state2.Address, state.Address)
	}
}
