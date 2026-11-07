// checkout_index_refresh_test.go covers the refresh of the pair's per-worktree correspondence index on switch.
// Entries recorded on the previous branch must not survive the switch.
// Their commits still exist on the other branch's refs, so the SHAExists staleness check passes them.
// Left in place, WeftSHAForWarpSHA would serve a weft SHA that the current branch's trailer history never produces.
// Fabric.Diff would then graft the current branches onto the other branch's history through weftAnchorForWarpSHA.
//
// The package's TestMain lives in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckout_RefreshesCorrespondenceIndex records a correspondence on the primary pair's original branch.
// It then switches with Checkout to a divergent branch pair whose weft history predates that record.
// The lookup must then miss with ErrNoCorrespondence rather than serve the other branch's weft SHA.
func TestCheckout_RefreshesCorrespondenceIndex(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	top := h.Topology

	// Healthy junction so Checkout's wiring step succeeds.
	slug := filepath.Base(l.WorktreePath())
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}

	// Create the divergent target before the commit below.
	// Its weft history then never contains the correspondence-carrying commit recorded on the original branch.
	const targetBranch = "index-refresh-target"
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", targetBranch)
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "branch", fabricengine.RecordsBranchName(targetBranch))

	f := hubforge.OpenFabric(t, h)

	// Record one correspondence on the original branch via a real scoped commit.
	if err := os.WriteFile(filepath.Join(fabricengine.RecordsWorktree(l), "_lym", "config.yaml"), []byte("index refresh probe"), 0o644); err != nil {
		t.Fatalf("write weft config: %v", err)
	}
	warpSHA, err := fabricengine.WarpForTest(f).CurrentSHA()
	if err != nil {
		t.Fatalf("warp CurrentSHA: %v", err)
	}
	result, err := f.Commit([]string{"_lym"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil || !result.WeftCommitted {
		t.Fatalf("Commit = (%+v, %v); want a real weft commit", result, err)
	}
	weftSHA := result.WeftSHA

	// Sanity: the lookup answers while the recording branch is still current.
	if got, err := f.WeftSHAForWarpSHA(warpSHA); err != nil || got != weftSHA {
		t.Fatalf("pre-switch WeftSHAForWarpSHA(%s) = (%q, %v); want (%q, nil)", warpSHA, got, err, weftSHA)
	}

	// Coordinated switch to the divergent pair.
	if _, err := top.Checkout(l, targetBranch); err != nil {
		t.Fatalf("Checkout(%q): %v", targetBranch, err)
	}
	if got := gitkit.CurrentBranch(t, fabricengine.RecordsWorktree(l)); got != fabricengine.RecordsBranchName(targetBranch) {
		t.Fatalf("weft branch after Checkout = %q; want %q", got, fabricengine.RecordsBranchName(targetBranch))
	}

	// The now-current branch's trailer history predates the recorded correspondence.
	// The lookup must therefore miss rather than serve the other branch's weft SHA.
	got, err := f.WeftSHAForWarpSHA(warpSHA)
	if err == nil {
		t.Fatalf("post-switch WeftSHAForWarpSHA(%s) = (%q, nil); want ErrNoCorrespondence — a cross-branch answer served from the stale cache", warpSHA, got)
	}
	if !errors.Is(err, fabricengine.ErrNoCorrespondence) {
		t.Errorf("post-switch WeftSHAForWarpSHA(%s) error = %v; want ErrNoCorrespondence", warpSHA, err)
	}
}
