//go:build tier1

// pull_integration_test.go — the end-to-end integration matrix for
// Fabric.Pull: clean fast-forward, warp history rewrite detection and
// reconcile (single-back, multi-back, no-surviving-anchor, empty-index),
// idempotency after a reconcile, the
// double-conflict abort, and the weft-first partial-failure contract.
// Package fabricengine_test. Reuses export_test.go's fixture shims
// (NewPlainWarpRepoForTest, CommitWarpForTest, CurrentSHAForTest,
// NewFabricForTest, WriteWeftConfigContentForTest) and, unqualified,
// coalesce_integration_test.go's addWarpBareRemote, since both
// files share package fabricengine_test — plus hubforge.NewHub for the weft
// side, whose upstream tracking lets PullWeft's ff-pull no-op cleanly in
// every test that does not deliberately diverge weft.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// buildReconcileFixture builds a warp+weft pair with a bare warp remote and n
// synced warp<->weft correspondences, returning the Fabric handle, both worktree
// paths, the bare remote's path, the initial warp SHA (the root for no-surviving-anchor),
// and the recorded warp/weft SHA pairs in commit order.
func buildReconcileFixture(t *testing.T, fixturesDir string, n int) (f *fabricengine.Fabric, warpPath, bareDir string, weftFixture *hubforge.Hub, initWarpSHA string, warpSHAs, weftSHAs []string) {
	t.Helper()

	warpPath = fabricengine.NewPlainWarpRepoForTest(t)
	bareDir = addWarpBareRemote(t, fixturesDir, warpPath)
	initWarpSHA = fabricengine.CurrentSHAForTest(t, warpPath)
	weftFixture = hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f = fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	for i := 0; i < n; i++ {
		warpSHA := fabricengine.CommitWarpForTest(t, warpPath, fmt.Sprintf("warp change %d", i))
		fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), fmt.Sprintf("weft change %d", i))
		weftSHA, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("commitWeft() round %d error = %v", i, err)
		}
		if !committed {
			t.Fatalf("commitWeft() round %d committed = false; want true", i)
		}
		warpSHAs = append(warpSHAs, warpSHA)
		weftSHAs = append(weftSHAs, weftSHA)
	}

	gitkit.MustRun(t, warpPath, "git", "push", "origin", "main")
	return f, warpPath, bareDir, weftFixture, initWarpSHA, warpSHAs, weftSHAs
}

// rewriteWarpRemoteHistory simulates an upstream rebase/force-push: it clones
// bareDir fresh, resets that clone to resetToSHA, commits one new, distinct
// commit, and force-pushes — rewriting the bare remote's main branch to a
// history that diverges from resetToSHA rather than descending from whatever
// main pointed at before. Returns the new remote tip SHA.
func rewriteWarpRemoteHistory(t *testing.T, fixturesDir, bareDir, resetToSHA string) string {
	t.Helper()

	clone := filepath.Join(fixturesDir, "warp-clone-rewrite")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	gitkit.MustRun(t, clone, "git", "reset", "--hard", resetToSHA)
	gitkit.CommitFile(t, clone, "rewritten.txt", "rewritten history", "rewritten history")
	gitkit.MustRun(t, clone, "git", "push", "--force", "origin", "main")
	return fabricengine.CurrentSHAForTest(t, clone)
}

// TestPull_DetectsDriftUnreachableUnprunedObject asserts that Fabric.Pull detects a warp history
// rewrite via ancestry, not object-existence: after fetch, the rebased-away commit's object still
// resolves (git fetch never prunes) yet Pull still classifies the pull as a rewrite and reconciles
// — guarding against any regression to SHAExists-style detection.
func TestPull_DetectsDriftUnreachableUnprunedObject(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

	newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.RewriteDetected {
		t.Errorf("Pull() RewriteDetected = false; want true")
	}
	if !result.Reconciled {
		t.Errorf("Pull() Reconciled = false; want true")
	}
	if result.AnchorWarpSHA != warpSHAs[0] {
		t.Errorf("Pull() AnchorWarpSHA = %q; want %q", result.AnchorWarpSHA, warpSHAs[0])
	}

	// The rebased-away commit's object still exists — fetch never prunes —
	// yet it is not an ancestor of the new tip. Detection must key off the
	// latter, never the former.
	if !fabricengine.WarpForTest(f).SHAExists(warpSHAs[1]) {
		t.Errorf("SHAExists(%q) = false after fetch; want true (fetch never prunes)", warpSHAs[1])
	}
	isAncestor, err := fabricengine.WarpForTest(f).IsAncestor(warpSHAs[1], newTip)
	if err != nil {
		t.Fatalf("IsAncestor(%q, %q) error = %v", warpSHAs[1], newTip, err)
	}
	if isAncestor {
		t.Fatalf("IsAncestor(%q, %q) = true; want false (test setup requires it rewritten away)", warpSHAs[1], newTip)
	}
}

// TestPull_ReanchorsSingleCommitBack covers a rewrite that orphans only the single newest
// correspondence entry: the anchor resolves to the one directly before it.
func TestPull_ReanchorsSingleCommitBack(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 3)

	newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[1])

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true")
	}
	if result.AnchorWarpSHA != warpSHAs[1] {
		t.Errorf("Pull() AnchorWarpSHA = %q; want %q", result.AnchorWarpSHA, warpSHAs[1])
	}
	if result.AnchorWeftSHA != weftSHAs[1] {
		t.Errorf("Pull() AnchorWeftSHA = %q; want %q", result.AnchorWeftSHA, weftSHAs[1])
	}
	if result.NewWarpHEAD != newTip {
		t.Errorf("Pull() NewWarpHEAD = %q; want %q", result.NewWarpHEAD, newTip)
	}
}

// TestPull_ReanchorsMultiCommitBack covers a rewrite that orphans several correspondence entries at
// once: the anchor resolves to the nearest older one that still survives, several steps back.
func TestPull_ReanchorsMultiCommitBack(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 4)

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true")
	}
	if result.AnchorWarpSHA != warpSHAs[0] {
		t.Errorf("Pull() AnchorWarpSHA = %q; want %q", result.AnchorWarpSHA, warpSHAs[0])
	}
	if result.AnchorWeftSHA != weftSHAs[0] {
		t.Errorf("Pull() AnchorWeftSHA = %q; want %q", result.AnchorWeftSHA, weftSHAs[0])
	}
}

// TestPull_IdempotentAfterReconcile asserts that a second, immediate Fabric.Pull call against an
// already-reconciled pair reports no further rewrite or reconcile: the new re-anchor commit's own
// Warp-SHA trailer makes detection idempotent.
func TestPull_IdempotentAfterReconcile(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	first, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("first Pull() error = %v", err)
	}
	if !first.Reconciled {
		t.Fatalf("first Pull() Reconciled = false; want true")
	}

	second, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("second Pull() error = %v", err)
	}
	if second.RewriteDetected {
		t.Errorf("second Pull() RewriteDetected = true; want false (idempotent)")
	}
	if second.Reconciled {
		t.Errorf("second Pull() Reconciled = true; want false (idempotent)")
	}
}

// TestPull_LeavesWeftHistoryUntouched asserts that a reconcile adds exactly one new weft commit
// (the re-anchor commit) on top of pre-existing weft history, without altering any commit that was
// already there.
func TestPull_LeavesWeftHistoryUntouched(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 2)

	weftHEADBefore := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())
	if weftHEADBefore != weftSHAs[len(weftSHAs)-1] {
		t.Fatalf("weft HEAD before Pull = %q; want the last synced weft SHA %q", weftHEADBefore, weftSHAs[len(weftSHAs)-1])
	}

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true")
	}

	weftHEADAfter := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())
	if weftHEADAfter != result.ReanchorWeftSHA {
		t.Errorf("weft HEAD after Pull = %q; want the reported re-anchor SHA %q", weftHEADAfter, result.ReanchorWeftSHA)
	}
	if got := gitkit.RevListCount(t, weftFixture.PrimeRecords(), weftHEADBefore+".."+weftHEADAfter); got != 1 {
		t.Errorf("commits added on top of pre-existing weft history = %d; want exactly 1 (the re-anchor commit)", got)
	}
}

// TestPull_AbortsOnUnpushedPlusDiverged covers the double-conflict abort: local warp has a commit no value of the tracking ref's reflog reaches AND the remote diverged, judged after the fetch.
// Fabric.Pull must return fabricengine.ErrWarpDivergedUnpushed and mutate neither repo.
func TestPull_AbortsOnUnpushedPlusDiverged(t *testing.T) {
	t.Parallel()
	boardkit.InstallReplay(t)

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWarpHEAD := fabricengine.CommitWarpForTest(t, warpPath, "local unpushed change")
	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	_, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrWarpDivergedUnpushed) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, fabricengine.ErrWarpDivergedUnpushed)", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_NoSurvivingAnchorAborts covers a rewrite so thorough that no recorded correspondence
// entry survives at all: Fabric.Pull must return fabricengine.ErrNoSurvivingAnchor and mutate neither repo.
func TestPull_NoSurvivingAnchorAborts(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, initWarpSHA, _, _ := buildReconcileFixture(t, fixturesDir, 2)

	preWarpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)
	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, initWarpSHA)

	_, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrNoSurvivingAnchor) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, fabricengine.ErrNoSurvivingAnchor)", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_CleanFastForwardAdvancesWarp covers a plain fast-forward remote: warp's local branch
// must actually move to the fetched tip, with no rewrite detected and weft history untouched — a
// regression guard against a fetch-only no-op.
func TestPull_CleanFastForwardAdvancesWarp(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	clone := filepath.Join(fixturesDir, "warp-clone-ff")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	ffSHA := gitkit.CommitFile(t, clone, "ff-file.txt", "ff change", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.WarpAdvanced {
		t.Errorf("Pull() WarpAdvanced = false; want true")
	}
	if result.NewWarpHEAD != ffSHA {
		t.Errorf("Pull() NewWarpHEAD = %q; want %q", result.NewWarpHEAD, ffSHA)
	}
	if result.RewriteDetected {
		t.Errorf("Pull() RewriteDetected = true; want false (clean fast-forward)")
	}
	if result.Reconciled {
		t.Errorf("Pull() Reconciled = true; want false (clean fast-forward)")
	}

	if got := fabricengine.CurrentSHAForTest(t, fabricengine.WarpPathForTest(f)); got != ffSHA {
		t.Errorf("warp HEAD after Pull() = %q; want it advanced to %q", got, ffSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != preWeftHEAD {
		t.Errorf("weft HEAD after Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_NoWeftUpstreamIsACleanNoOp guards the freshly-bootstrapped-hub case: the suffixed weft
// primary is created locally at clone time and gains an upstream only when the first push lands, so
// until then Pull's weft step must skip as a vacuous success — not surface git's "no tracking
// information" exit — and the warp side must still be processed.
func TestPull_NoWeftUpstreamIsACleanNoOp(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	bareDir := addWarpBareRemote(t, fixturesDir, warpPath)
	gitkit.MustRun(t, warpPath, "git", "push", "origin", "main")

	// A weft repo whose branch has no upstream at all — the post-bootstrap state before any push.
	weftPath := t.TempDir()
	gitkit.Git(t, weftPath, "init", "-q", "-b", "main-weft")
	gitkit.Git(t, weftPath, "config", "user.email", "test@test.com")
	gitkit.Git(t, weftPath, "config", "user.name", "Test")
	gitkit.CommitFile(t, weftPath, "seed.txt", "weft", "init")

	f := fabricengine.NewFabricForTest(t, warpPath, weftPath)

	// Advance the warp remote so the warp half has real work to do.
	clone := filepath.Join(fixturesDir, "warp-clone-noupstream")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	ffSHA := gitkit.CommitFile(t, clone, "ff-file.txt", "ff change", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v; want a clean no-op weft skip", err)
	}
	if !result.WeftPulled {
		t.Errorf("Pull() WeftPulled = false; want true (vacuous no-op success)")
	}
	if !result.WarpAdvanced || result.NewWarpHEAD != ffSHA {
		t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q", result.WarpAdvanced, result.NewWarpHEAD, ffSHA)
	}
}

// TestPull_StaleIndexRebuiltBeforeAnchorWalk guards the false fabricengine.ErrNoSurvivingAnchor a stale
// correspondence index produced: a re-cloned hub's per-pair index can be empty (or missing older
// entries) while the adopted weft trailer history — the sole source of truth — still carries a
// surviving anchor.
// Pull must rebuild the index from trailers before the anchor walk and reconcile, never abort.
func TestPull_StaleIndexRebuiltBeforeAnchorWalk(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 2)

	// Simulate the re-cloned hub: the trailer history stays, the local index cache does not.
	indexPath, err := fabricengine.CorrIndexPathForTest(f)
	if err != nil {
		t.Fatalf("corrIndexPath: %v", err)
	}
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove correspondence index: %v", err)
	}

	// Rewrite upstream so warpSHAs[1] dies but warpSHAs[0] survives as the nearest anchor.
	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v; want a reconcile via the rebuilt index, not an abort", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true — the surviving trailer anchor was not found")
	}
	if result.AnchorWarpSHA != warpSHAs[0] {
		t.Errorf("Pull() AnchorWarpSHA = %q; want the surviving %q", result.AnchorWarpSHA, warpSHAs[0])
	}
	if result.AnchorWeftSHA != weftSHAs[0] {
		t.Errorf("Pull() AnchorWeftSHA = %q; want %q", result.AnchorWeftSHA, weftSHAs[0])
	}
}

// TestPull_DirtyWarpRefusesBeforeMovingWarp guards the data-loss hole where Pull's ResetHard
// discarded uncommitted tracked warp changes on a routine fast-forward: with a modified tracked file
// in the warp worktree and an advanced remote, Pull must return fabricengine.ErrWarpDirty, leave warp HEAD
// unmoved, and leave the modification intact on disk.
func TestPull_DirtyWarpRefusesBeforeMovingWarp(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, _, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWarpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)

	clone := filepath.Join(fixturesDir, "warp-clone-dirty-ff")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	gitkit.CommitFile(t, clone, "ff-file.txt", "ff change", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	// Dirty a TRACKED warp file — the exact state ResetHard would destroy.
	dirtyFile := filepath.Join(warpPath, "README")
	const dirtyContent = "uncommitted local edit that must survive pull"
	if err := os.WriteFile(dirtyFile, []byte(dirtyContent), 0o644); err != nil {
		t.Fatalf("dirty tracked file: %v", err)
	}

	result, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrWarpDirty) {
		t.Fatalf("Pull() error = %v; want fabricengine.ErrWarpDirty", err)
	}
	if result.WarpAdvanced {
		t.Errorf("Pull() WarpAdvanced = true; want false (refused before moving warp)")
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after refused Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	data, readErr := os.ReadFile(dirtyFile)
	if readErr != nil {
		t.Fatalf("read dirtied file after Pull(): %v", readErr)
	}
	if string(data) != dirtyContent {
		t.Errorf("uncommitted warp edit was destroyed by Pull(): got %q; want %q", data, dirtyContent)
	}
}

// TestPull_EmptyIndexNoDrift covers a non-fast-forward remote with an empty correspondence index
// (warp commits that were never synced to weft at all): warp must still advance, with no reconcile
// commit written.
func TestPull_EmptyIndexNoDrift(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	bareDir := addWarpBareRemote(t, fixturesDir, warpPath)
	initWarpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// Warp commits happen, but nothing is ever synced to weft — the
	// correspondence index stays empty.
	fabricengine.CommitWarpForTest(t, warpPath, "warp change never synced 1")
	fabricengine.CommitWarpForTest(t, warpPath, "warp change never synced 2")
	gitkit.MustRun(t, warpPath, "git", "push", "origin", "main")

	newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, initWarpSHA)

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.RewriteDetected {
		t.Errorf("Pull() RewriteDetected = false; want true")
	}
	if result.Reconciled {
		t.Errorf("Pull() Reconciled = true; want false (empty index)")
	}
	if !result.WarpAdvanced {
		t.Errorf("Pull() WarpAdvanced = false; want true")
	}
	if result.NewWarpHEAD != newTip {
		t.Errorf("Pull() NewWarpHEAD = %q; want %q", result.NewWarpHEAD, newTip)
	}
}

// TestPull_WeftDivergedAndWarpFetchFails_PartialError forces a weft replay stop combined with a warp-side failure.
// The stop is a local weft commit and a remote-advanced upstream commit changing the same path, and the warp failure is warpPath carrying no configured remote, so f.code.Fetch() itself fails.
// The weft stop never stalls the warp side, and the combined failure must surface as both a *fabricengine.WeftStopError and a *PartialPullError whose WeftPulled is false.
// Its Error() names both failures rather than claiming the weft pull succeeded.
func TestPull_WeftDivergedAndWarpFetchFails_PartialError(t *testing.T) {
	t.Parallel()
	boardkit.InstallReplay(t)

	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// A real hub's weft primary checks out the suffixed branch (fabricengine.RecordsBranchName("main")), never bare "main", and that suffixed branch carries no upstream at all until something pushes
	// it -- CloneAndWire never does, since a fresh hub's primary is local-only until the first real
	// push.
	// Establish that upstream explicitly here, or PullWeft finds nothing to track and
	// weftHasUpstream() reports false, skipping the pull as a vacuous success instead of the genuine
	// divergence this test needs.
	weftBranch := fabricengine.RecordsBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.RecordsBare, cloneB)
	gitkit.Git(t, cloneB, "config", "user.email", "test@test.com")
	gitkit.Git(t, cloneB, "config", "user.name", "Test")
	gitkit.CommitFile(t, cloneB, "status.json", "from clone b", "b")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	// Diverge local weft too, on the same path, so the replay of the local commit conflicts.
	gitkit.CommitFile(t, weftFixture.PrimeRecords(), "status.json", "local weft change", "local weft change")

	// warpPath has no configured remote at all -- f.code.Fetch() fails, giving this test its
	// warp-side failure alongside the weft-side one.
	result, err := f.Pull(fabricengine.SyncOptions{})
	var stopErr *fabricengine.WeftStopError
	if !errors.As(err, &stopErr) {
		t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError among the joined errors", err, err)
	}
	var partialErr *fabricengine.PartialPullError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Pull() error = %v (%T); want a *fabricengine.PartialPullError among the joined errors", err, err)
	}
	if partialErr.WeftPulled {
		t.Errorf("PartialPullError.WeftPulled = true; want false (the weft pull failed to fast-forward)")
	}
	if result.WeftPulled {
		t.Errorf("Pull() result.WeftPulled = true; want false")
	}
	if strings.Contains(partialErr.Error(), "weft pull succeeded") {
		t.Errorf("PartialPullError.Error() = %q; must not claim the weft pull succeeded", partialErr.Error())
	}
}

// divergedWeftFixture is a reconcile fixture whose weft branch both this machine and origin moved, and whose warp remote advanced by one commit.
type divergedWeftFixture struct {
	f           *fabricengine.Fabric
	warpPath    string
	weftPath    string
	weftBranch  string
	originSHA   string
	localSHA    string
	warpFFSHA   string
	weftHeadWas string
}

// removeMarker, as a marker argument of newDivergedWeftFixture, commits the removal of the weft's anchor marker.
const removeMarker = "<remove the marker>"

// commitWeftSide commits rel with content, and then the anchor marker as marker says, in the weft checkout at dir.
// An empty rel commits no file and an empty marker leaves the marker alone; removeMarker deletes it and any other value is written.
// It returns the checkout's HEAD.
func commitWeftSide(t *testing.T, dir, rel, content, marker, message string) string {
	t.Helper()

	if rel != "" {
		gitkit.CommitFile(t, dir, rel, content, message)
	}
	switch marker {
	case "":
	case removeMarker:
		gitkit.Git(t, dir, "rm", "--quiet", lymcwd.AnchorFileName)
		gitkit.Git(t, dir, "commit", "--quiet", "-m", message+": drop the marker")
	default:
		gitkit.CommitFile(t, dir, lymcwd.AnchorFileName, marker+"\n", message+": set the marker")
	}
	return gitkit.RevParse(t, dir, "HEAD")
}

// newDivergedWeftFixture commits originRel on the weft's origin from a second clone and localRel on this machine's weft, each with its own content.
// originMarker and localMarker, when set, change the anchor marker in a commit of their side after the file's; removeMarker deletes it.
// An empty originRel and originMarker mean origin gains no commit, and an empty localRel and localMarker mean the weft gains none.
// It then advances warp's remote with a clean commit, so a Pull has both arms to run.
func newDivergedWeftFixture(t *testing.T, originRel, originContent, originMarker, localRel, localContent, localMarker string) divergedWeftFixture {
	t.Helper()
	boardkit.InstallReplay(t)

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)
	weftPath := weftFixture.PrimeRecords()

	weftBranch := fabricengine.RecordsBranchName("main")
	gitkit.MustRun(t, weftPath, "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-diverge-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.RecordsBare, cloneB)
	gitkit.Git(t, cloneB, "config", "user.email", "test@test.com")
	gitkit.Git(t, cloneB, "config", "user.name", "Test")
	originSHA := commitWeftSide(t, cloneB, originRel, originContent, originMarker, "origin weft change")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	// Diverge local weft too, so `git pull --ff-only` cannot fast-forward.
	localSHA := commitWeftSide(t, weftPath, localRel, localContent, localMarker, "local weft change")

	// Advance warp's remote with a clean, non-rewritten commit -- no history rewrite is involved.
	clone := filepath.Join(fixturesDir, "warp-clone-weft-diverge")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	warpFFSHA := gitkit.CommitFile(t, clone, "ff-file.txt", "ff change", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	return divergedWeftFixture{
		f: f, warpPath: warpPath, weftPath: weftPath, weftBranch: gitkit.CurrentBranch(t, weftPath),
		originSHA: originSHA, localSHA: localSHA, warpFFSHA: warpFFSHA, weftHeadWas: localSHA,
	}
}

// TestPull_WeftDivergedWarpAdvancesCleanly covers a weft that both this machine and origin moved while warp's own remote simply advanced.
// The weft's local commit replays onto origin's and the pull succeeds.
// A replay conflict and an uncommitted change on a replayed path each return a *WeftStopError after the warp arm ran, leaving the weft where it was.
func TestPull_WeftDivergedWarpAdvancesCleanly(t *testing.T) {
	t.Parallel()

	t.Run("A local weft commit replays onto origin's and the pull succeeds", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil (the local weft commit replays)", err)
		}
		if !result.WeftPulled {
			t.Errorf("Pull() WeftPulled = false; want true (the weft was brought level)")
		}
		if !result.WarpAdvanced || result.NewWarpHEAD != fx.warpFFSHA {
			t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q", result.WarpAdvanced, result.NewWarpHEAD, fx.warpFFSHA)
		}
		if got := gitkit.RevParse(t, fx.weftPath, "HEAD~1"); got != fx.originSHA {
			t.Errorf("weft HEAD~1 after Pull() = %q; want origin's commit %q under the replayed one", got, fx.originSHA)
		}
		for _, rel := range []string{"from-clone-b.txt", "local-only.txt"} {
			if _, err := os.Stat(filepath.Join(fx.weftPath, rel)); err != nil {
				t.Errorf("weft after Pull() lacks %s: %v", rel, err)
			}
		}
	})

	t.Run("A both-sides change returns a weft stop after warp advanced", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "status.json", "from origin", "", "status.json", "from this machine", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError", err, err)
		}
		if !errors.Is(err, fabricengine.ErrReplayConflict) {
			t.Errorf("Pull() error = %v; want errors.Is(err, ErrReplayConflict)", err)
		}
		if !result.WarpAdvanced || result.NewWarpHEAD != fx.warpFFSHA {
			t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q before the weft stop is returned", result.WarpAdvanced, result.NewWarpHEAD, fx.warpFFSHA)
		}
		if result.WeftPulled {
			t.Errorf("Pull() WeftPulled = true; want false beside a weft stop")
		}
		pushVerb := "`lym fabric push` in " + fx.warpPath
		wantLine := fabricengine.RecordsConflictWayForwardForTest(fx.weftPath, fx.weftBranch, []string{"status.json"}, []string{fx.localSHA}, pushVerb)
		if !strings.Contains(err.Error(), wantLine) {
			t.Errorf("Pull() error = %q; want it to carry the way forward %q", err.Error(), wantLine)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
		}
	})

	t.Run("An uncommitted change on a replayed path returns a weft stop naming it", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
		dirtyFile := filepath.Join(fx.weftPath, "local-only.txt")
		if err := os.WriteFile(dirtyFile, []byte("uncommitted edit"), 0o644); err != nil {
			t.Fatalf("dirty %s: %v", dirtyFile, err)
		}

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError", err, err)
		}
		if !errors.Is(err, fabricengine.ErrReplayDirty) {
			t.Errorf("Pull() error = %v; want errors.Is(err, ErrReplayDirty)", err)
		}
		if len(stop.Paths) != 1 || stop.Paths[0] != "local-only.txt" || !strings.Contains(err.Error(), "local-only.txt") {
			t.Errorf("WeftStopError paths = %v, message %q; want local-only.txt named", stop.Paths, err.Error())
		}
		if !result.WarpAdvanced {
			t.Errorf("Pull() WarpAdvanced = false; want the warp arm to run beside the weft stop")
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
		}
		data, readErr := os.ReadFile(dirtyFile)
		if readErr != nil || string(data) != "uncommitted edit" {
			t.Errorf("uncommitted weft edit after the stop = %q, %v; want it kept", data, readErr)
		}
	})

	t.Run("A marker change on either side stops the pull before warp or the weft moves", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			// fixture builds the diverged weft with the marker changes of the row.
			fixture func(t *testing.T) divergedWeftFixture
			// wantSet is the anchor the way forward must name.
			wantSet string
		}{
			{
				"OriginChangesTheMarkerAndTheWeftHasNoLocalCommit",
				func(t *testing.T) divergedWeftFixture {
					return newDivergedWeftFixture(t, "from-clone-b.txt", "b", "sub", "", "", "")
				},
				"sub",
			},
			{
				"BothSidesChangeItDifferently",
				func(t *testing.T) divergedWeftFixture {
					return newDivergedWeftFixture(t, "from-clone-b.txt", "b", "sub", "local-only.txt", "local", "other")
				},
				"other",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx := tt.fixture(t)
				warpHeadWas := fabricengine.CurrentSHAForTest(t, fx.warpPath)

				result, err := fx.f.Pull(fabricengine.SyncOptions{})

				var stop *fabricengine.WeftStopError
				if !errors.As(err, &stop) || !errors.Is(err, fabricengine.ErrAnchorChange) {
					t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError wrapping ErrAnchorChange", err, err)
				}
				for _, want := range []string{"lym fabric anchor", "--set " + tt.wantSet} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Pull() error = %q; want it to name %q", err.Error(), want)
					}
				}
				if result.WarpAdvanced || result.WeftPulled {
					t.Errorf("Pull() WarpAdvanced=%v WeftPulled=%v; want neither side moved", result.WarpAdvanced, result.WeftPulled)
				}
				if got := fabricengine.CurrentSHAForTest(t, fx.warpPath); got != warpHeadWas {
					t.Errorf("warp HEAD after the stop = %q; want unchanged %q", got, warpHeadWas)
				}
				if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
					t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
				}
			})
		}
	})

	t.Run("Markerless commits whose content sits at the local marker replay with no stop", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", removeMarker, "local-only.txt", "local weft change", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil, the markerless tip compares equal by its content's anchor", err)
		}
		if !result.WeftPulled {
			t.Errorf("Pull() WeftPulled = false; want the weft brought level")
		}
	})

	t.Run("A weft ahead of origin with a re-anchor not yet pushed is no stop", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "", "", "", "", "", "sub")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil, origin has nothing the weft lacks", err)
		}
		if !result.WeftPulled || !result.WarpAdvanced {
			t.Errorf("Pull() WeftPulled=%v WarpAdvanced=%v; want both arms run", result.WeftPulled, result.WarpAdvanced)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after Pull() = %q; want the unpushed re-anchor %q kept", got, fx.weftHeadWas)
		}
	})

	t.Run("An empty own marker stops the pull naming the marker file", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
		warpHeadWas := fabricengine.CurrentSHAForTest(t, fx.warpPath)
		if err := os.WriteFile(filepath.Join(fx.weftPath, lymcwd.AnchorFileName), []byte("\n"), 0o644); err != nil {
			t.Fatalf("truncate the marker: %v", err)
		}

		result, err := fx.f.Pull(fabricengine.SyncOptions{})

		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) || !errors.Is(err, fabricengine.ErrAnchorChange) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError wrapping ErrAnchorChange", err, err)
		}
		if !strings.Contains(err.Error(), lymcwd.AnchorFileName) {
			t.Errorf("Pull() error = %q; want it to name %s", err.Error(), lymcwd.AnchorFileName)
		}
		if result.WarpAdvanced {
			t.Errorf("Pull() WarpAdvanced = true; want warp left alone")
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.warpPath); got != warpHeadWas {
			t.Errorf("warp HEAD after the stop = %q; want unchanged %q", got, warpHeadWas)
		}
	})

	t.Run("A move record on the weft stops the pull before warp or the weft moves", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
		warpHeadWas := fabricengine.CurrentSHAForTest(t, fx.warpPath)
		writeMoveRecord(t, fx.weftPath, fabricengine.MoveRecord{A: ".", B: "x", Kind: fabricengine.MoveKindMove, PID: 4242})

		result, err := fx.f.Pull(fabricengine.SyncOptions{})

		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) || !errors.Is(err, fabricengine.ErrAnchorChange) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError wrapping ErrAnchorChange", err, err)
		}
		if !strings.Contains(stop.WayForward, "lym fabric anchor") || !strings.Contains(stop.WayForward, "--set x") {
			t.Errorf("WeftStopError way forward = %q; want the move record's `lym fabric anchor` re-run", stop.WayForward)
		}
		if result.WarpAdvanced || result.WeftPulled {
			t.Errorf("Pull() WarpAdvanced=%v WeftPulled=%v; want neither side moved", result.WarpAdvanced, result.WeftPulled)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.warpPath); got != warpHeadWas {
			t.Errorf("warp HEAD after the stop = %q; want unchanged %q", got, warpHeadWas)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
		}
	})
}

// TestPull_WeftUpstreamUnreadableIsAnError covers a weft whose pull fails because origin is unreachable and whose tracking ref names an object the repository lacks, so its commits against head cannot be listed.
// A failed local read is an infrastructure error: Pull returns it rather than a nil error with WeftPulled false.
func TestPull_WeftUpstreamUnreadableIsAnError(t *testing.T) {
	t.Parallel()
	fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
	gitkit.Git(t, fx.weftPath, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-origin"))
	trackingRef := strings.TrimSpace(gitkit.Git(t, fx.weftPath, "rev-parse", "--path-format=absolute", "--git-path", "refs/remotes/origin/"+fx.weftBranch))
	if err := os.MkdirAll(filepath.Dir(trackingRef), 0o755); err != nil {
		t.Fatalf("make the tracking ref's directory: %v", err)
	}
	if err := os.WriteFile(trackingRef, []byte(strings.Repeat("a", 40)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", trackingRef, err)
	}

	result, err := fx.f.Pull(fabricengine.SyncOptions{})
	if err == nil || !strings.Contains(err.Error(), "against its upstream") {
		t.Fatalf("Pull() = %+v, %v; want an error that the weft's state against its upstream could not be read", result, err)
	}
	if result.WeftPulled {
		t.Errorf("Pull() WeftPulled = true; want false beside the error")
	}
}

// TestPull_ReflogJudgesUnpushedAndRewinds covers what the reflog tests decide that no other pull test reaches.
// A branch fetched but never pushed from this clone is not unpushed and takes the anchor walk.
// A task branch origin rewound is refused with ErrWarpRewound before and after its tracking ref was fetched.
// A weft whose only commits replay empty pulls clean.
func TestPull_ReflogJudgesUnpushedAndRewinds(t *testing.T) {
	t.Parallel()
	boardkit.InstallReplay(t)

	t.Run("A fetched, never-pushed branch with a rewritten head takes the anchor walk", func(t *testing.T) {
		t.Parallel()
		fixturesDir := t.TempDir()
		_, _, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

		clone := filepath.Join(fixturesDir, "warp-clone-never-pushed")
		gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
		f := fabricengine.NewFabricForTest(t, clone, weftFixture.PrimeRecords())
		// A clone records no reflog entry for its tracking ref, so the ref is recreated by a fetch, as in a checkout that fetched its branch and never pushed it.
		gitkit.MustRun(t, clone, "git", "update-ref", "-d", "refs/remotes/origin/main")
		gitkit.MustRun(t, clone, "git", "fetch")
		newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])
		gitkit.MustRun(t, clone, "git", "fetch")

		result, err := f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil (no local commit is unpushed)", err)
		}
		if !result.Reconciled || !result.WarpAdvanced || result.NewWarpHEAD != newTip {
			t.Errorf("Pull() Reconciled=%v WarpAdvanced=%v NewWarpHEAD=%q; want a reconcile to %q", result.Reconciled, result.WarpAdvanced, result.NewWarpHEAD, newTip)
		}
		if got := fabricengine.CurrentSHAForTest(t, clone); got != newTip {
			t.Errorf("warp HEAD after Pull() = %q; want origin's head %q", got, newTip)
		}
		if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != result.ReanchorWeftSHA {
			t.Errorf("weft HEAD after Pull() = %q; want the re-anchor commit %q", got, result.ReanchorWeftSHA)
		}
	})

	for _, fetchFirst := range []bool{false, true} {
		name := "A rewound task branch is refused before its tracking ref was fetched"
		if fetchFirst {
			name = "A rewound task branch is refused after its tracking ref was fetched"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, hub, "task")
			pairCode := hub.PairCodeWorktree("task")
			branch := gitkit.CurrentBranch(t, pairCode)
			gitkit.Git(t, pairCode, "branch", "--set-upstream-to=origin/"+branch)
			base := gitkit.RevParse(t, pairCode, "HEAD")
			first := gitkit.CommitFile(t, pairCode, "one.txt", "one", "one")
			second := gitkit.CommitFile(t, pairCode, "two.txt", "two", "two")
			gitkit.Git(t, pairCode, "push", "origin", branch)

			other := filepath.Join(t.TempDir(), "other")
			gitkit.Git(t, filepath.Dir(other), "clone", hub.CodeBare, other)
			gitkit.Git(t, other, "push", "--force-with-lease=refs/heads/"+branch+":"+second, "origin", base+":refs/heads/"+branch)
			if fetchFirst {
				gitkit.Git(t, pairCode, "fetch")
			}

			loc, err := lymcwd.ResolveWorktree(pairCode)
			if err != nil {
				t.Fatalf("ResolveWorktree(%s): %v", pairCode, err)
			}
			f, err := fabricengine.Open(loc)
			if err != nil {
				t.Fatalf("Open(%s): %v", pairCode, err)
			}
			weftHeadBefore := fabricengine.CurrentSHAForTest(t, hub.PairRecordsSibling("task"))

			_, err = f.Pull(fabricengine.SyncOptions{})
			if !errors.Is(err, fabricengine.ErrWarpRewound) {
				t.Fatalf("Pull() error = %v; want errors.Is(err, ErrWarpRewound)", err)
			}
			wantLine := fabricengine.RewoundWayForwardForTest(pairCode, branch, base, []string{second, first})
			if !strings.Contains(err.Error(), wantLine) {
				t.Errorf("Pull() error = %q; want it to carry the way forward %q", err.Error(), wantLine)
			}
			if got := fabricengine.CurrentSHAForTest(t, pairCode); got != second {
				t.Errorf("task branch HEAD after the refusal = %q; want unchanged %q", got, second)
			}
			if got := fabricengine.CurrentSHAForTest(t, hub.PairRecordsSibling("task")); got != weftHeadBefore {
				t.Errorf("weft HEAD after the refusal = %q; want unchanged %q", got, weftHeadBefore)
			}
		})
	}

	t.Run("A weft with only commits that replay empty pulls clean", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "same.txt", "same", "", "same.txt", "same", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil (the local commit replays empty)", err)
		}
		if !result.WeftPulled {
			t.Errorf("Pull() WeftPulled = false; want true")
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.originSHA {
			t.Errorf("weft HEAD after Pull() = %q; want origin's head %q", got, fx.originSHA)
		}
	})
}

// TestPull_HealthyPairBothSidesPullCleanly pins the ordinary path against a regression that simply
// stops pulling the weft: with a healthy weft upstream and an advanced warp remote, both sides must
// still report success and the weft HEAD must actually advance.
func TestPull_HealthyPairBothSidesPullCleanly(t *testing.T) {
	t.Parallel()

	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	weftBranch := fabricengine.RecordsBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-healthy-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.RecordsBare, cloneB)
	gitkit.Git(t, cloneB, "config", "user.email", "test@test.com")
	gitkit.Git(t, cloneB, "config", "user.name", "Test")
	weftFFSHA := gitkit.CommitFile(t, cloneB, "from-clone-b.txt", "b", "b")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	clone := filepath.Join(fixturesDir, "warp-clone-healthy")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	warpFFSHA := gitkit.CommitFile(t, clone, "ff-file.txt", "ff change", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.WeftPulled {
		t.Errorf("Pull() WeftPulled = false; want true (a clean fast-forward on both sides)")
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != weftFFSHA {
		t.Errorf("weft HEAD after Pull() = %q; want it advanced to %q", got, weftFFSHA)
	}
	if !result.WarpAdvanced || result.NewWarpHEAD != warpFFSHA {
		t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q", result.WarpAdvanced, result.NewWarpHEAD, warpFFSHA)
	}
}
