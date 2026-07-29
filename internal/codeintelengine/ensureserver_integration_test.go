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

package codeintelengine

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"testing"
	"time"
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
