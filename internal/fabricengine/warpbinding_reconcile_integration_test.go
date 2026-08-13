//go:build integration

// warpbinding_reconcile_integration_test.go proves Topology.Reconcile's once-per-hub warp-URL
// binding backfill (reconcile.go's reconcileWarpBinding): the recorded/present/diverged/skipped/
// deferred outcome taxonomy, that the check runs exactly once per Reconcile call regardless of how
// many pairs a hub has, and that a divergent or transport-only-differing record is reported but never
// overwritten. It also proves Unwire leaves a recorded binding untouched.
//
// Every test needs a hub built through fabricengine.CloneHub's own backfill path directly — the
// backfill reads the warp side's real "origin" remote as CloneHub itself sets it up, not as
// hubforge.NewHub's own pre-wired template would — so this file builds its own newClonedHubFixture
// helper on top of CloneHub and clone_adopt_test.go's makeBareRemote instead of taking a ready-made
// hub from hubforge.
//
// Package fabricengine_test to isolate real end-to-end git fixtures from the package's internal unit
// tests; shares the single TestMain in testmain_test.go with every other file in this package.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// clonedHubFixture bundles the pieces newClonedHubFixture builds: the resolved clone geometry, the
// prime warp worktree's layout, a ready-to-use Topology, and the two bare remote paths the tests need
// to author expected binding content and to sever/restore the warp side's origin.
type clonedHubFixture struct {
	Result   fabricengine.CloneResult
	Layout   *lyxcwd.Location
	Topology *fabricengine.Topology
	WarpBare string
	WeftBare string
}

// newClonedHubFixture builds a genuinely remote-backed hub via fabricengine.CloneHub against two bare
// fixtures, seeds the repo-wide fabric config, and resolves a ready-to-use Topology and Location.
// ForceBootstrap is set because the bare weft fixture is an ordinary seeded remote standing in for a
// weft, not a repo that has ever been one, so it carries no .lyx-anchor of its own.
//
// The fixture leaves the board worktree CLEAN: CloneHub writes the anchor marker (and this helper's
// own seedRepoWideFabricConfig writes the repo-wide config) but neither is committed — CloneHub never
// commits, only the CLI's Bolt.Commit does — so the board is dirty the moment CloneHub returns. A
// local stage-all commit through Bolt (no push) closes that gap; every "recorded"-outcome test below
// depends on starting from a clean board, or reconcileWarpBinding's dirty-board guard would defer the
// write instead.
func newClonedHubFixture(t *testing.T) clonedHubFixture {
	t.Helper()

	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "warpbind-warp")
	weftBare := makeBareRemote(t, fixtures, "warpbind-weft")

	cloneParent := t.TempDir()
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL:        filepath.ToSlash(weftBare),
		WarpURL:        filepath.ToSlash(warpBare),
		Subpath:        ".",
		ForceBootstrap: true,
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	seedRepoWideFabricConfig(t, res.HubPath)

	l, err := lyxcwd.Resolve(res.PrimeCwd)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", res.PrimeCwd, err)
	}

	cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(res.HubPath))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	top := fabricengine.NewTopology(cfg)

	if _, _, err := fabricengine.NewBolt(res.BoardDir).Commit("test fixture: record anchor + repo-wide config", fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("commit fixture board state: %v", err)
	}

	return clonedHubFixture{
		Result:   res,
		Layout:   l,
		Topology: top,
		WarpBare: warpBare,
		WeftBare: weftBare,
	}
}

// unbindHub removes the recorded warp-binding file from boardDir's worktree and commits that deletion
// through a Bolt, producing the "unbound hub that predates the binding" state explicitly rather than
// assuming it. commitFileOnBranch is deliberately not reused here: it commits through a throwaway
// scratch clone pushed to the bare remote and never touches an existing local worktree, so it cannot
// leave boardDir itself clean.
func unbindHub(t *testing.T, boardDir string) {
	t.Helper()

	path := filepath.Join(boardDir, fabricengine.WarpBindingFileName)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove warp binding record at %s: %v", path, err)
	}
	if _, _, err := fabricengine.NewBolt(boardDir).Commit("test fixture: unbind hub", fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("commit unbind at %s: %v", boardDir, err)
	}
}

// writeAndCommitBinding overwrites boardDir's recorded warp-binding file with url and commits the
// change through a Bolt, used to author a record that intentionally differs from the warp side's
// actual origin (normalized-equal, genuinely divergent, or transport-identity-equal).
func writeAndCommitBinding(t *testing.T, boardDir, url string) {
	t.Helper()

	path := filepath.Join(boardDir, fabricengine.WarpBindingFileName)
	if err := os.WriteFile(path, []byte(url+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if _, _, err := fabricengine.NewBolt(boardDir).Commit("test fixture: rewrite warp binding", fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("commit rewritten binding at %s: %v", boardDir, err)
	}
}

// TestReconcile_BacksFillsBindingOnce asserts that an unbound wired hub gains the record on its first
// Reconcile: the outcome is "recorded", the file exists at the board root holding the warp origin URL,
// and no pair result carries a binding-related detail — the binding is a repo-wide fact, never
// reported per-pair.
func TestReconcile_BacksFillsBindingOnce(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)
	unbindHub(t, fixture.Result.BoardDir)

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomeRecorded {
		t.Fatalf("WarpBinding = %q (detail %q); want %q", result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomeRecorded)
	}

	bindingPath := filepath.Join(fixture.Result.BoardDir, fabricengine.WarpBindingFileName)
	data, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}
	if got, want := strings.TrimSpace(string(data)), filepath.ToSlash(fixture.WarpBare); got != want {
		t.Errorf("binding content = %q; want %q (the warp origin URL)", got, want)
	}

	for _, pair := range result.Pairs {
		if strings.Contains(strings.ToLower(pair.Detail), "warp binding") {
			t.Errorf("pair %+v carries a warp-binding detail; want the binding reported only repo-wide", pair)
		}
	}
}

// TestReconcile_BacksFillsOnceOnMultiWorktreeHub asserts that with several warp worktrees present,
// the backfill still writes the record exactly once and reports it exactly once at the repo-wide
// level, leaving the per-pair results untouched.
func TestReconcile_BacksFillsOnceOnMultiWorktreeHub(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)
	unbindHub(t, fixture.Result.BoardDir)

	if _, err := fixture.Topology.Add(fixture.Layout, "second-pair", fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(second-pair): %v", err)
	}

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomeRecorded {
		t.Fatalf("WarpBinding = %q (detail %q); want %q", result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomeRecorded)
	}
	if len(result.Pairs) < 2 {
		t.Fatalf("len(Pairs) = %d; want at least 2 (multi-worktree hub)", len(result.Pairs))
	}
	for _, pair := range result.Pairs {
		if strings.Contains(strings.ToLower(pair.Detail), "warp binding") {
			t.Errorf("pair %+v carries a warp-binding detail; want the binding reported only repo-wide", pair)
		}
	}
}

// TestReconcile_NormalizedRecordReportsPresent asserts that a record differing from the warp origin
// only by a trailing ".git" normalizes equal and reports "present", not "diverged".
func TestReconcile_NormalizedRecordReportsPresent(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)

	origin := filepath.ToSlash(fixture.WarpBare)
	trimmed := strings.TrimSuffix(origin, ".git")
	if trimmed == origin {
		t.Fatalf("fixture warp URL %q has no trailing .git to trim", origin)
	}
	writeAndCommitBinding(t, fixture.Result.BoardDir, trimmed)

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomePresent {
		t.Errorf("WarpBinding = %q (detail %q); want %q", result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomePresent)
	}
}

// TestReconcile_DivergentRecordIsLeftUntouched asserts that a genuinely differing record is not
// overwritten, reports "diverged" with both URLs named in the detail, and Reconcile still returns a
// nil error.
func TestReconcile_DivergentRecordIsLeftUntouched(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)

	origin := filepath.ToSlash(fixture.WarpBare)
	divergent := origin + "-not-the-same-repo"
	writeAndCommitBinding(t, fixture.Result.BoardDir, divergent)

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomeDiverged {
		t.Fatalf("WarpBinding = %q (detail %q); want %q", result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomeDiverged)
	}
	if !strings.Contains(result.WarpBindingDetail, divergent) || !strings.Contains(result.WarpBindingDetail, origin) {
		t.Errorf("WarpBindingDetail = %q; want both %q and %q named", result.WarpBindingDetail, divergent, origin)
	}

	bindingPath := filepath.Join(fixture.Result.BoardDir, fabricengine.WarpBindingFileName)
	data, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}
	if got := strings.TrimSpace(string(data)); got != divergent {
		t.Errorf("binding content = %q; want unchanged %q", got, divergent)
	}
}

// TestReconcile_TransportOnlyDifferenceIsAdvisory asserts that a record spelling the same repo over a
// different transport reports "diverged" with a detail that says the difference is transport-only and
// advisory. The fixture's warp origin is a local path with no scheme, so normalizeWarpURL applies no
// case-folding to it; flipping its case produces two spellings that differ under normalizeWarpURL
// (still "diverged") but collapse to the same string under warpURLTransportIdentity, which lowercases
// unconditionally — the same shape a real scheme-vs-scp-form transport swap produces.
func TestReconcile_TransportOnlyDifferenceIsAdvisory(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)

	origin := filepath.ToSlash(fixture.WarpBare)
	caseDiffered := strings.ToUpper(origin)
	if caseDiffered == origin {
		t.Fatalf("fixture warp URL %q has no letters to case-flip", origin)
	}
	writeAndCommitBinding(t, fixture.Result.BoardDir, caseDiffered)

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomeDiverged {
		t.Fatalf("WarpBinding = %q (detail %q); want %q", result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomeDiverged)
	}
	lowerDetail := strings.ToLower(result.WarpBindingDetail)
	if !strings.Contains(lowerDetail, "transport") || !strings.Contains(lowerDetail, "advisory") {
		t.Errorf("WarpBindingDetail = %q; want it to name the difference as transport-only and advisory", result.WarpBindingDetail)
	}
}

// TestReconcile_DirtyBoardDefersWrite asserts that with an unrelated uncommitted file in the board
// worktree, the outcome is "deferred", no binding file is written, and the unrelated file is neither
// committed nor pushed: it remains untracked and the board's HEAD is unchanged.
func TestReconcile_DirtyBoardDefersWrite(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)
	unbindHub(t, fixture.Result.BoardDir)

	headBefore := gitOutput(t, fixture.Result.BoardDir, "rev-parse", "HEAD")

	unrelated := filepath.Join(fixture.Result.BoardDir, "unrelated-scratch.txt")
	if err := os.WriteFile(unrelated, []byte("scratch"), 0o644); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomeDeferred {
		t.Fatalf("WarpBinding = %q (detail %q); want %q", result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomeDeferred)
	}

	bindingPath := filepath.Join(fixture.Result.BoardDir, fabricengine.WarpBindingFileName)
	if _, statErr := os.Stat(bindingPath); statErr == nil {
		t.Errorf("binding file %s was written; want the write deferred", bindingPath)
	}

	if tracked := gitOutput(t, fixture.Result.BoardDir, "ls-files", "--", "unrelated-scratch.txt"); tracked != "" {
		t.Errorf("unrelated file was committed (tracked as %q); want it left untracked", tracked)
	}
	if headAfter := gitOutput(t, fixture.Result.BoardDir, "rev-parse", "HEAD"); headAfter != headBefore {
		t.Errorf("board HEAD changed from %s to %s; want unchanged (no commit or push attempted)", headBefore, headAfter)
	}
}

// TestReconcile_NoWarpOriginIsSkipped asserts that after removing the warp side's origin remote, the
// outcome is "skipped" with an empty detail and Reconcile still returns a nil error.
func TestReconcile_NoWarpOriginIsSkipped(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)

	gitkit.MustRun(t, fixture.Layout.WorktreePath(), "git", "remote", "remove", "origin")

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomeSkipped {
		t.Errorf("WarpBinding = %q; want %q", result.WarpBinding, fabricengine.WarpBindingOutcomeSkipped)
	}
	if result.WarpBindingDetail != "" {
		t.Errorf("WarpBindingDetail = %q; want empty", result.WarpBindingDetail)
	}
}

// TestReconcile_UnpushedRetryPushesOnPresent covers the engine-only half of the unpushed-retry story:
// once a record is committed and matches the warp origin, Reconcile reports "present" with an empty
// detail, and a second Reconcile pass creates no further commit. Topology.Reconcile itself never
// pushes — that is the CLI handler's job (fabriccli.runReconcile) — so this test drives
// Topology.Reconcile directly and asserts only on the engine's verdict; the actual commit-then-push
// retry (a backfill whose push failed, then succeeding on the next pass) is covered at the CLI seam by
// TestRunCLI_ReconcileBacksFillsWarpBinding and TestRunCLI_ReconcileBackfillFailureIsNonFatal instead.
func TestReconcile_UnpushedRetryPushesOnPresent(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomePresent {
		t.Fatalf("WarpBinding = %q (detail %q); want %q", result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomePresent)
	}
	if result.WarpBindingDetail != "" {
		t.Errorf("WarpBindingDetail = %q; want empty", result.WarpBindingDetail)
	}

	headBefore := gitOutput(t, fixture.Result.BoardDir, "rev-parse", "HEAD")
	if _, err := fixture.Topology.Reconcile(fixture.Layout); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if headAfter := gitOutput(t, fixture.Result.BoardDir, "rev-parse", "HEAD"); headAfter != headBefore {
		t.Errorf("a second Reconcile pass changed HEAD from %s to %s; want no second commit", headBefore, headAfter)
	}
}

// TestUnwire_LeavesWarpBindingInPlace asserts that running unwire on a bound hub leaves the record on
// the weft side untouched, exactly as it already leaves the anchor marker and the repo-wide config.
// This requires no production change; it exists so a future unwire edit cannot quietly delete the
// record.
func TestUnwire_LeavesWarpBindingInPlace(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)

	bindingPath := filepath.Join(fixture.Result.BoardDir, fabricengine.WarpBindingFileName)
	before, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}

	if _, err := fabricengine.Unwire(fixture.Result.PrimeCwd); err != nil {
		t.Fatalf("Unwire: %v", err)
	}

	after, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s after Unwire: %v", bindingPath, err)
	}
	if string(before) != string(after) {
		t.Errorf("warp binding record changed after Unwire: before %q, after %q", before, after)
	}
}

// TestReconcile_BoardLockArtifactDoesNotDeferBackfill is the counterpart to
// TestReconcile_DirtyBoardDefersWrite: a module's own lock file at the board root is machine-local
// scratch, not unrelated content a stage-all commit would sweep up, so it must not defer the
// once-per-hub binding backfill.
// Before the *.lock/*.swaplock excludes, a single ordinary board read left tasks.json.swaplock at
// the board root permanently, and every subsequent reconcile on a hub predating the binding reported
// "deferred" forever.
func TestReconcile_BoardLockArtifactDoesNotDeferBackfill(t *testing.T) {
	t.Parallel()

	fixture := newClonedHubFixture(t)
	unbindHub(t, fixture.Result.BoardDir)

	for _, name := range []string{"tasks.json.swaplock", "board.write.lock"} {
		if err := os.WriteFile(filepath.Join(fixture.Result.BoardDir, name), nil, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result, err := fixture.Topology.Reconcile(fixture.Layout)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.WarpBinding != fabricengine.WarpBindingOutcomeRecorded {
		t.Fatalf("WarpBinding = %q (detail %q); want %q — a lock artifact must not defer the backfill",
			result.WarpBinding, result.WarpBindingDetail, fabricengine.WarpBindingOutcomeRecorded)
	}

	bindingPath := filepath.Join(fixture.Result.BoardDir, fabricengine.WarpBindingFileName)
	if _, statErr := os.Stat(bindingPath); statErr != nil {
		t.Errorf("binding file %s not written: %v", bindingPath, statErr)
	}
}
