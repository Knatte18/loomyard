//go:build integration

// pull_integration_test.go — the end-to-end integration matrix for
// Fabric.Pull: clean fast-forward, warp history rewrite detection and
// reconcile (single-back, multi-back, no-surviving-anchor, empty-index),
// idempotency after a reconcile, PATTERN-residue identification, the
// double-conflict abort, and the weft-first partial-failure contract.
// Package fabricengine_test. Reuses export_test.go's fixture shims
// (NewPlainWarpRepoForTest, CommitWarpForTest, CurrentSHAForTest,
// NewFabricForTest, WriteWeftConfigContentForTest) and, unqualified,
// coalesce_integration_test.go's addWarpBareRemote/commitPlain, since both
// files share package fabricengine_test — plus hubforge.NewHub for the weft
// side, whose upstream tracking lets PullWeft's ff-pull no-op cleanly in
// every test that does not deliberately diverge weft.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
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
	weftFixture = hubforge.NewHub(t, ".")
	f = fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeWeft())

	for i := 0; i < n; i++ {
		warpSHA := fabricengine.CommitWarpForTest(t, warpPath, fmt.Sprintf("warp change %d", i))
		fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeWeft(), fmt.Sprintf("weft change %d", i))
		weftSHA, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
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
	gitkit.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, clone, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, clone, "git", "reset", "--hard", resetToSHA)
	commitPlain(t, clone, "rewritten.txt", "rewritten history")
	gitkit.MustRun(t, clone, "git", "push", "--force", "origin", "main")
	return fabricengine.CurrentSHAForTest(t, clone)
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

// TestPull_DetectsDriftUnreachableUnprunedObject asserts that Fabric.Pull detects a warp history
// rewrite via ancestry, not object-existence: after fetch, the rebased-away commit's object still
// resolves (git fetch never prunes) yet Pull still classifies the pull as a rewrite and reconciles
// — guarding against any regression to SHAExists-style detection.
func TestPull_DetectsDriftUnreachableUnprunedObject(t *testing.T) {
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
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, warpSHAs, weftSHAs := buildReconcileFixture(t, fixturesDir, 2)

	weftHEADBefore := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())
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

	weftHEADAfter := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())
	if weftHEADAfter != result.ReanchorWeftSHA {
		t.Errorf("weft HEAD after Pull = %q; want the reported re-anchor SHA %q", weftHEADAfter, result.ReanchorWeftSHA)
	}
	if got := revListCountBetween(t, weftFixture.PrimeWeft(), weftHEADBefore+".."+weftHEADAfter); got != 1 {
		t.Errorf("commits added on top of pre-existing weft history = %d; want exactly 1 (the re-anchor commit)", got)
	}
}

// TestPull_IdentifiesPatternResidue seeds a synthetic _lyx/PATTERN.md weft commit (plus a
// non-PATTERN-path weft commit) after the anchor point and asserts PatternResidue names exactly
// the PATTERN-path-touching commit, not the others (including the pre-existing, already-synced
// content commit).
// It also pins the pathspec's narrow scope: a commit touching only
// _lyx/config/fabric.yaml must never appear, while a commit touching
// _lyx/pattern/<detail>.md must appear, proving both PathspecFile and
// PathspecDir are wired.
func TestPull_IdentifiesPatternResidue(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

	lyxDir := filepath.Join(weftFixture.PrimeWeft(), lyxdirs.LyxDirName)
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", lyxDir, err)
	}
	if err := os.WriteFile(filepath.Join(lyxDir, "PATTERN.md"), []byte("pattern content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "commit", "-q", "-m", "pattern residue commit")
	patternCommitSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())

	if err := os.WriteFile(filepath.Join(weftFixture.PrimeWeft(), "unrelated.txt"), []byte("unrelated"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "commit", "-q", "-m", "unrelated residue commit")

	configDir := filepath.Join(lyxDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "fabric.yaml"), []byte("junctions: []\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "commit", "-q", "-m", "config residue commit")

	patternDetailDir := filepath.Join(lyxDir, "pattern")
	if err := os.MkdirAll(patternDetailDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", patternDetailDir, err)
	}
	if err := os.WriteFile(filepath.Join(patternDetailDir, "detail.md"), []byte("detail content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "commit", "-q", "-m", "pattern detail residue commit")
	patternDetailCommitSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true")
	}
	if len(result.PatternResidue) != 2 {
		t.Fatalf("Pull() PatternResidue = %+v; want exactly two entries", result.PatternResidue)
	}

	residueSHAs := make(map[string]fabricengine.PatternResidueEntry, len(result.PatternResidue))
	for _, entry := range result.PatternResidue {
		residueSHAs[entry.WeftSHA] = entry
	}

	if _, ok := residueSHAs[patternCommitSHA]; !ok {
		t.Errorf("PatternResidue = %+v; want an entry for the PATTERN.md commit %q", result.PatternResidue, patternCommitSHA)
	} else {
		wantPath := pattern.PathspecFile
		found := false
		for _, p := range residueSHAs[patternCommitSHA].Paths {
			if p == wantPath {
				found = true
			}
		}
		if !found {
			t.Errorf("PatternResidue entry for %q Paths = %v; want it to contain %q", patternCommitSHA, residueSHAs[patternCommitSHA].Paths, wantPath)
		}
	}

	if _, ok := residueSHAs[patternDetailCommitSHA]; !ok {
		t.Errorf("PatternResidue = %+v; want an entry for the %s commit %q", result.PatternResidue, pattern.PathspecDir, patternDetailCommitSHA)
	} else {
		wantPath := pattern.PathspecDir + "/detail.md"
		found := false
		for _, p := range residueSHAs[patternDetailCommitSHA].Paths {
			if p == wantPath {
				found = true
			}
		}
		if !found {
			t.Errorf("PatternResidue entry for %q Paths = %v; want it to contain %q", patternDetailCommitSHA, residueSHAs[patternDetailCommitSHA].Paths, wantPath)
		}
	}
}

// TestPull_AbortsOnUnpushedPlusDiverged covers the double-conflict abort: local warp has an
// unpushed commit AND the remote diverged.
// Fabric.Pull must return fabricengine.ErrWarpDivergedUnpushed and mutate neither repo.
func TestPull_AbortsOnUnpushedPlusDiverged(t *testing.T) {
	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWarpHEAD := fabricengine.CommitWarpForTest(t, warpPath, "local unpushed change")
	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	_, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrWarpDivergedUnpushed) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, fabricengine.ErrWarpDivergedUnpushed)", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft()); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_NoSurvivingAnchorAborts covers a rewrite so thorough that no recorded correspondence
// entry survives at all: Fabric.Pull must return fabricengine.ErrNoSurvivingAnchor and mutate neither repo.
func TestPull_NoSurvivingAnchorAborts(t *testing.T) {
	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, initWarpSHA, _, _ := buildReconcileFixture(t, fixturesDir, 2)

	preWarpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)
	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, initWarpSHA)

	_, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrNoSurvivingAnchor) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, fabricengine.ErrNoSurvivingAnchor)", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft()); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_CleanFastForwardAdvancesWarp covers a plain fast-forward remote: warp's local branch
// must actually move to the fetched tip, with no rewrite detected and weft history untouched — a
// regression guard against a fetch-only no-op.
func TestPull_CleanFastForwardAdvancesWarp(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())

	clone := filepath.Join(fixturesDir, "warp-clone-ff")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, clone, "git", "config", "user.name", "Test")
	ffSHA := commitPlain(t, clone, "ff-file.txt", "ff change")
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
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft()); got != preWeftHEAD {
		t.Errorf("weft HEAD after Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_NoWeftUpstreamIsACleanNoOp guards the freshly-bootstrapped-hub case: the suffixed weft
// primary is created locally at clone time and gains an upstream only when the first push lands, so
// until then Pull's weft step must skip as a vacuous success — not surface git's "no tracking
// information" exit — and the warp side must still be processed.
func TestPull_NoWeftUpstreamIsACleanNoOp(t *testing.T) {
	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	bareDir := addWarpBareRemote(t, fixturesDir, warpPath)
	gitkit.MustRun(t, warpPath, "git", "push", "origin", "main")

	// A weft repo whose branch has no upstream at all — the post-bootstrap state before any push.
	weftPath := t.TempDir()
	gitkit.MustRun(t, weftPath, "git", "init", "-q", "-b", "main-weft")
	gitkit.MustRun(t, weftPath, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, weftPath, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(weftPath, "seed.txt"), []byte("weft"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftPath, "git", "add", ".")
	gitkit.MustRun(t, weftPath, "git", "commit", "-q", "-m", "init")

	f := fabricengine.NewFabricForTest(t, warpPath, weftPath)

	// Advance the warp remote so the warp half has real work to do.
	clone := filepath.Join(fixturesDir, "warp-clone-noupstream")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, clone, "git", "config", "user.name", "Test")
	ffSHA := commitPlain(t, clone, "ff-file.txt", "ff change")
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
	fixturesDir := t.TempDir()
	f, warpPath, bareDir, _, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWarpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)

	clone := filepath.Join(fixturesDir, "warp-clone-dirty-ff")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, clone, "git", "config", "user.name", "Test")
	commitPlain(t, clone, "ff-file.txt", "ff change")
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
	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	bareDir := addWarpBareRemote(t, fixturesDir, warpPath)
	initWarpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftFixture := hubforge.NewHub(t, ".")
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeWeft())

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

// TestPull_WeftDivergedAndWarpFetchFails_PartialError forces the weft ff-pull to fail (a local weft
// commit diverging from a remote-advanced upstream) combined with a warp-side failure (warpPath
// carries no configured remote, so f.warp.Fetch() itself fails). Since the weft arm is now
// non-fatal, Pull still attempts the warp side, and the combined failure must surface as a
// *PartialPullError whose WeftPulled is false and whose Error() names both failures rather than
// claiming the weft pull succeeded — this is the rewrite of the pre-non-fatal-weft test that used
// to assert the opposite disposition (an immediate error with warp never touched).
func TestPull_WeftDivergedAndWarpFetchFails_PartialError(t *testing.T) {
	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.NewHub(t, ".")
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeWeft())

	// A real hub's weft primary checks out the suffixed branch (fabricengine.WeftBranchName("main")),
	// never bare "main", and that suffixed branch carries no upstream at all until something pushes
	// it -- CloneAndWire never does, since a fresh hub's primary is local-only until the first real
	// push. Establish that upstream explicitly here, or PullWeft finds nothing to track and
	// weftHasUpstream() reports false, skipping the pull as a vacuous success instead of the genuine
	// divergence this test needs.
	weftBranch := fabricengine.WeftBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.WeftBare, cloneB)
	gitkit.MustRun(t, cloneB, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, cloneB, "git", "config", "user.name", "Test")
	commitPlain(t, cloneB, "from-clone-b.txt", "b")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	// Diverge local weft too, so `git pull --ff-only` cannot fast-forward.
	commitPlain(t, weftFixture.PrimeWeft(), "local-only.txt", "local weft change")

	// warpPath has no configured remote at all -- f.warp.Fetch() fails, giving this test its
	// warp-side failure alongside the weft-side one.
	result, err := f.Pull(fabricengine.SyncOptions{})
	var partialErr *fabricengine.PartialPullError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Pull() error = %v (%T); want a *fabricengine.PartialPullError", err, err)
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

// TestPull_WeftDivergedWarpAdvancesCleanly covers the ordinary shape the non-fatal weft arm exists
// for: a weft that has genuinely diverged from its own upstream (git pull --ff-only hard-refuses)
// while warp's own remote has simply advanced, with no rewrite involved at all. Pull must still
// fetch and advance warp, report WeftPulled false, return a nil error, and leave weft HEAD exactly
// where it was before the call.
func TestPull_WeftDivergedWarpAdvancesCleanly(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	weftBranch := fabricengine.WeftBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-diverge-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.WeftBare, cloneB)
	gitkit.MustRun(t, cloneB, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, cloneB, "git", "config", "user.name", "Test")
	commitPlain(t, cloneB, "from-clone-b.txt", "b")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	// Diverge local weft too, so `git pull --ff-only` cannot fast-forward.
	commitPlain(t, weftFixture.PrimeWeft(), "local-only.txt", "local weft change")
	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())

	// Advance warp's remote with a clean, non-rewritten commit -- no history rewrite is involved.
	clone := filepath.Join(fixturesDir, "warp-clone-weft-diverge")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, clone, "git", "config", "user.name", "Test")
	ffSHA := commitPlain(t, clone, "ff-file.txt", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v; want nil (a weft-side failure is non-fatal)", err)
	}
	if result.WeftPulled {
		t.Errorf("Pull() WeftPulled = true; want false (the weft failed to fast-forward)")
	}
	if !result.WarpAdvanced || result.NewWarpHEAD != ffSHA {
		t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q despite the weft-side failure", result.WarpAdvanced, result.NewWarpHEAD, ffSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft()); got != preWeftHEAD {
		t.Errorf("weft HEAD after Pull() = %q; want unchanged %q (the failed weft pull must not move weft HEAD)", got, preWeftHEAD)
	}
}

// TestPull_HealthyPairBothSidesPullCleanly pins the ordinary path against a regression that simply
// stops pulling the weft: with a healthy weft upstream and an advanced warp remote, both sides must
// still report success and the weft HEAD must actually advance.
func TestPull_HealthyPairBothSidesPullCleanly(t *testing.T) {
	fixturesDir := t.TempDir()
	f, _, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	weftBranch := fabricengine.WeftBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-healthy-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.WeftBare, cloneB)
	gitkit.MustRun(t, cloneB, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, cloneB, "git", "config", "user.name", "Test")
	weftFFSHA := commitPlain(t, cloneB, "from-clone-b.txt", "b")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	clone := filepath.Join(fixturesDir, "warp-clone-healthy")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.MustRun(t, clone, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, clone, "git", "config", "user.name", "Test")
	warpFFSHA := commitPlain(t, clone, "ff-file.txt", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.WeftPulled {
		t.Errorf("Pull() WeftPulled = false; want true (a clean fast-forward on both sides)")
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft()); got != weftFFSHA {
		t.Errorf("weft HEAD after Pull() = %q; want it advanced to %q", got, weftFFSHA)
	}
	if !result.WarpAdvanced || result.NewWarpHEAD != warpFFSHA {
		t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q", result.WarpAdvanced, result.NewWarpHEAD, warpFFSHA)
	}
}

// TestPull_IdentifiesPatternResidueUnderSubpathAnchor is the subpath-anchored counterpart to
// TestPull_IdentifiesPatternResidue: on a hub anchored at "backend", PATTERN content lives at
// backend/_lyx/PATTERN.md and never at the weft worktree root, so a root-relative residue pathspec
// reported an empty residue — telling a caller "nothing needs review" about exactly the commit that
// does.
func TestPull_IdentifiesPatternResidueUnderSubpathAnchor(t *testing.T) {
	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

	// Record a subpath anchor for this pair the same way a real hub does: the marker at the hub's
	// board root, which lyxcwd.ResolveWorktree reads back for AnchorRel.
	const anchor = "backend"
	boardDir := fabricengine.BoardDir(filepath.Dir(warpPath))
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", boardDir, err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, lyxcwd.AnchorFileName), []byte(anchor+"\n"), 0o644); err != nil {
		t.Fatalf("write anchor marker: %v", err)
	}

	anchoredLyxDir := filepath.Join(weftFixture.PrimeWeft(), anchor, lyxdirs.LyxDirName)
	if err := os.MkdirAll(anchoredLyxDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", anchoredLyxDir, err)
	}
	if err := os.WriteFile(filepath.Join(anchoredLyxDir, "PATTERN.md"), []byte("anchored pattern content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "add", "-A")
	gitkit.MustRun(t, weftFixture.PrimeWeft(), "git", "commit", "-q", "-m", "anchored pattern residue commit")
	anchoredPatternSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeWeft())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Reconciled {
		t.Fatalf("Pull() Reconciled = false; want true")
	}

	var found *fabricengine.PatternResidueEntry
	for i := range result.PatternResidue {
		if result.PatternResidue[i].WeftSHA == anchoredPatternSHA {
			found = &result.PatternResidue[i]
		}
	}
	if found == nil {
		t.Fatalf("Pull() PatternResidue = %+v; want an entry for the anchored PATTERN.md commit %q",
			result.PatternResidue, anchoredPatternSHA)
	}
	wantPath := anchor + "/" + pattern.PathspecFile
	if !slices.Contains(found.Paths, wantPath) {
		t.Errorf("PatternResidue entry Paths = %v; want it to contain %q", found.Paths, wantPath)
	}
}
