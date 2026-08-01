//go:build integration

// pull_integration_test.go — the end-to-end integration matrix for
// Fabric.Pull: clean fast-forward, warp history rewrite detection and
// reconcile (single-back, multi-back, no-surviving-anchor, empty-index),
// idempotency after a reconcile, PATTERN-residue identification, the
// double-conflict abort, and the weft-first partial-failure contract.
// Reuses this package's existing fixture helpers — newPlainWarpRepo,
// commitWarp, currentSHA, newFabric (index_integration_test.go);
// addWarpBareRemote, commitPlain, bareBranchSHA (coalesce_integration_test.go);
// writeWeftConfigContent (syncweft_integration_test.go) — plus
// lyxtest.CopyWeft for the weft side, whose upstream tracking lets PullWeft's
// ff-pull no-op cleanly in every test that does not deliberately diverge weft.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// buildReconcileFixture builds a warp+weft pair with a bare warp remote and n
// synced warp<->weft correspondences (via Fabric.CommitWeft, exactly like
// staleCorrespondenceFixture in syncweft_integration_test.go), pushing warp's
// history to the bare remote once at the end. It returns the Fabric handle,
// both worktree paths, the bare remote's path, the warp repo's very first
// (pre-loop) commit SHA — the root a caller can rewrite back to for a
// no-surviving-anchor scenario — and the recorded warp/weft SHA pairs in
// commit order.
func buildReconcileFixture(t *testing.T, fixturesDir string, n int) (f *Fabric, warpPath, bareDir string, weftFixture lyxtest.WeftFixture, initWarpSHA string, warpSHAs, weftSHAs []string) {
	t.Helper()

	warpPath = newPlainWarpRepo(t)
	bareDir = addWarpBareRemote(t, fixturesDir, warpPath)
	initWarpSHA = currentSHA(t, warpPath)
	weftFixture = lyxtest.CopyWeft(t)
	f = newFabric(t, warpPath, weftFixture.WeftPath)

	for i := 0; i < n; i++ {
		warpSHA := commitWarp(t, warpPath, fmt.Sprintf("warp change %d", i))
		writeWeftConfigContent(t, weftFixture.WeftPath, fmt.Sprintf("weft change %d", i))
		weftSHA, committed, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
		if err != nil {
			t.Fatalf("CommitWeft() round %d error = %v", i, err)
		}
		if !committed {
			t.Fatalf("CommitWeft() round %d committed = false; want true", i)
		}
		warpSHAs = append(warpSHAs, warpSHA)
		weftSHAs = append(weftSHAs, weftSHA)
	}

	lyxtest.MustRun(t, warpPath, "git", "push", "origin", "main")
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
	lyxtest.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	lyxtest.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, clone, "git", "config", "user.name", "Test")
	lyxtest.MustRun(t, clone, "git", "reset", "--hard", resetToSHA)
	commitPlain(t, clone, "rewritten.txt", "rewritten history")
	lyxtest.MustRun(t, clone, "git", "push", "--force", "origin", "main")
	return currentSHA(t, clone)
}

// revListCountBetween returns `git rev-list --count <rangeArg>` in repoPath —
// used to assert exactly how many commits separate two points, e.g. that a
// reconcile added exactly one new weft commit on top of pre-existing history.
func revListCountBetween(t *testing.T, repoPath, rangeArg string) int {
	t.Helper()

	cmd := exec.Command("git", "rev-list", "--count", rangeArg)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list --count %s in %s: %v", rangeArg, repoPath, err)
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n); err != nil {
		t.Fatalf("parse rev-list --count output %q: %v", out, err)
	}
	return n
}

// TestPull_DetectsDriftUnreachableUnprunedObject asserts that Fabric.Pull
// detects a warp history rewrite via ancestry, not object-existence: after
// fetch, the rebased-away commit's object still resolves (git fetch never
// prunes) yet Pull still classifies the pull as a rewrite and reconciles —
// guarding against any regression to SHAExists-style detection.
func TestPull_DetectsDriftUnreachableUnprunedObject(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

	newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(SyncOptions{})
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
	if !f.Warp.SHAExists(warpSHAs[1]) {
		t.Errorf("SHAExists(%q) = false after fetch; want true (fetch never prunes)", warpSHAs[1])
	}
	isAncestor, err := f.Warp.IsAncestor(warpSHAs[1], newTip)
	if err != nil {
		t.Fatalf("IsAncestor(%q, %q) error = %v", warpSHAs[1], newTip, err)
	}
	if isAncestor {
		t.Fatalf("IsAncestor(%q, %q) = true; want false (test setup requires it rewritten away)", warpSHAs[1], newTip)
	}
}

// TestPull_ReanchorsSingleCommitBack covers a rewrite that orphans only the
// single newest correspondence entry: the anchor resolves to the one
// directly before it.
func TestPull_ReanchorsSingleCommitBack(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 3)

	newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[1])

	result, err := f.Pull(SyncOptions{})
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

// TestPull_ReanchorsMultiCommitBack covers a rewrite that orphans several
// correspondence entries at once: the anchor resolves to the nearest older
// one that still survives, several steps back.
func TestPull_ReanchorsMultiCommitBack(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 4)

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(SyncOptions{})
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

// TestPull_IdempotentAfterReconcile asserts that a second, immediate
// Fabric.Pull call against an already-reconciled pair reports no further
// rewrite or reconcile: the new re-anchor commit's own Warp-SHA trailer makes
// detection idempotent.
func TestPull_IdempotentAfterReconcile(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, _, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	first, err := f.Pull(SyncOptions{})
	if err != nil {
		t.Fatalf("first Pull() error = %v", err)
	}
	if !first.Reconciled {
		t.Fatalf("first Pull() Reconciled = false; want true")
	}

	second, err := f.Pull(SyncOptions{})
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

// TestPull_LeavesWeftHistoryUntouched asserts that a reconcile adds exactly
// one new weft commit (the re-anchor commit) on top of pre-existing weft
// history, without altering any commit that was already there.
func TestPull_LeavesWeftHistoryUntouched(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 2)

	weftHEADBefore := currentSHA(t, weftFixture.WeftPath)
	if weftHEADBefore != weftSHAs[len(weftSHAs)-1] {
		t.Fatalf("weft HEAD before Pull = %q; want the last synced weft SHA %q", weftHEADBefore, weftSHAs[len(weftSHAs)-1])
	}

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true")
	}

	weftHEADAfter := currentSHA(t, weftFixture.WeftPath)
	if weftHEADAfter != result.ReanchorWeftSHA {
		t.Errorf("weft HEAD after Pull = %q; want the reported re-anchor SHA %q", weftHEADAfter, result.ReanchorWeftSHA)
	}
	if got := revListCountBetween(t, weftFixture.WeftPath, weftHEADBefore+".."+weftHEADAfter); got != 1 {
		t.Errorf("commits added on top of pre-existing weft history = %d; want exactly 1 (the re-anchor commit)", got)
	}
}

// TestPull_IdentifiesPatternResidue seeds a synthetic _pattern/PATTERN.md
// weft commit (plus a non-_pattern weft commit) after the anchor point and
// asserts PatternResidue names exactly the _pattern-touching commit, not the
// others (including the pre-existing, already-synced content commit).
func TestPull_IdentifiesPatternResidue(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

	patternDir := filepath.Join(weftFixture.WeftPath, hubgeometry.PatternDirName)
	if err := os.MkdirAll(patternDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", patternDir, err)
	}
	if err := os.WriteFile(filepath.Join(patternDir, "PATTERN.md"), []byte("pattern content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "add", "-A")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "commit", "-q", "-m", "pattern residue commit")
	patternCommitSHA := currentSHA(t, weftFixture.WeftPath)

	if err := os.WriteFile(filepath.Join(weftFixture.WeftPath, "unrelated.txt"), []byte("unrelated"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "add", "-A")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "commit", "-q", "-m", "unrelated residue commit")

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true")
	}
	if len(result.PatternResidue) != 1 {
		t.Fatalf("Pull() PatternResidue = %+v; want exactly one entry", result.PatternResidue)
	}

	entry := result.PatternResidue[0]
	if entry.WeftSHA != patternCommitSHA {
		t.Errorf("PatternResidue[0].WeftSHA = %q; want %q", entry.WeftSHA, patternCommitSHA)
	}
	wantPath := hubgeometry.PatternDirName + "/PATTERN.md"
	found := false
	for _, p := range entry.Paths {
		if p == wantPath {
			found = true
		}
	}
	if !found {
		t.Errorf("PatternResidue[0].Paths = %v; want it to contain %q", entry.Paths, wantPath)
	}
}

// TestPull_AbortsOnUnpushedPlusDiverged covers the double-conflict abort:
// local warp has an unpushed commit AND the remote diverged. Fabric.Pull must
// return ErrWarpDivergedUnpushed and mutate neither repo.
func TestPull_AbortsOnUnpushedPlusDiverged(t *testing.T) {
	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWarpHEAD := commitWarp(t, warpPath, "local unpushed change")
	preWeftHEAD := currentSHA(t, weftFixture.WeftPath)

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	_, err := f.Pull(SyncOptions{})
	if !errors.Is(err, ErrWarpDivergedUnpushed) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, ErrWarpDivergedUnpushed)", err)
	}

	if got := currentSHA(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := currentSHA(t, weftFixture.WeftPath); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_NoSurvivingAnchorAborts covers a rewrite so thorough that no
// recorded correspondence entry survives at all: Fabric.Pull must return
// ErrNoSurvivingAnchor and mutate neither repo.
func TestPull_NoSurvivingAnchorAborts(t *testing.T) {
	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, initWarpSHA, _, _ := buildReconcileFixture(t, fixturesDir, 2)

	preWarpHEAD := currentSHA(t, warpPath)
	preWeftHEAD := currentSHA(t, weftFixture.WeftPath)

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, initWarpSHA)

	_, err := f.Pull(SyncOptions{})
	if !errors.Is(err, ErrNoSurvivingAnchor) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, ErrNoSurvivingAnchor)", err)
	}

	if got := currentSHA(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := currentSHA(t, weftFixture.WeftPath); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_CleanFastForwardAdvancesWarp covers a plain fast-forward remote:
// warp's local branch must actually move to the fetched tip, with no rewrite
// detected and weft history untouched — a regression guard against a
// fetch-only no-op.
func TestPull_CleanFastForwardAdvancesWarp(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWeftHEAD := currentSHA(t, weftFixture.WeftPath)

	clone := filepath.Join(fixturesDir, "warp-clone-ff")
	lyxtest.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	lyxtest.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, clone, "git", "config", "user.name", "Test")
	ffSHA := commitPlain(t, clone, "ff-file.txt", "ff change")
	lyxtest.MustRun(t, clone, "git", "push")

	result, err := f.Pull(SyncOptions{})
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

	if got := currentSHA(t, f.warpPath); got != ffSHA {
		t.Errorf("warp HEAD after Pull() = %q; want it advanced to %q", got, ffSHA)
	}
	if got := currentSHA(t, weftFixture.WeftPath); got != preWeftHEAD {
		t.Errorf("weft HEAD after Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_EmptyIndexNoDrift covers a non-fast-forward remote with an empty
// correspondence index (warp commits that were never synced to weft at all):
// warp must still advance, with no reconcile commit written.
func TestPull_EmptyIndexNoDrift(t *testing.T) {
	fixturesDir := t.TempDir()
	warpPath := newPlainWarpRepo(t)
	bareDir := addWarpBareRemote(t, fixturesDir, warpPath)
	initWarpSHA := currentSHA(t, warpPath)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	// Warp commits happen, but nothing is ever synced to weft — the
	// correspondence index stays empty.
	commitWarp(t, warpPath, "warp change never synced 1")
	commitWarp(t, warpPath, "warp change never synced 2")
	lyxtest.MustRun(t, warpPath, "git", "push", "origin", "main")

	newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, initWarpSHA)

	result, err := f.Pull(SyncOptions{})
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

// TestPull_WeftPullFailsWarpUntouched forces the weft ff-pull to fail (a
// local weft commit diverging from a remote-advanced upstream) and asserts
// warp is never fetched/reset, the error surfaces, and warp HEAD is
// unchanged.
func TestPull_WeftPullFailsWarpUntouched(t *testing.T) {
	fixturesDir := t.TempDir()
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	preWarpHEAD := currentSHA(t, warpPath)

	cloneB := filepath.Join(fixturesDir, "weft-cloneB")
	lyxtest.MustRun(t, fixturesDir, "git", "clone", "-q", weftFixture.Bare, cloneB)
	lyxtest.MustRun(t, cloneB, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, cloneB, "git", "config", "user.name", "Test")
	commitPlain(t, cloneB, "from-clone-b.txt", "b")
	lyxtest.MustRun(t, cloneB, "git", "push", "-q")

	// Diverge local weft too, so `git pull --ff-only` cannot fast-forward.
	commitPlain(t, weftFixture.WeftPath, "local-only.txt", "local weft change")

	result, err := f.Pull(SyncOptions{})
	if err == nil {
		t.Fatalf("Pull() error = nil; want an error (weft pull should fail to fast-forward)")
	}
	if result.WeftPulled {
		t.Errorf("Pull() result.WeftPulled = true; want false (a weft-side failure must report the zero result)")
	}

	if got := currentSHA(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after failed Pull() = %q; want unchanged %q (warp must never be touched)", got, preWarpHEAD)
	}
}
