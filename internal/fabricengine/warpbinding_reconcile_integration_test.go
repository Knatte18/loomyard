// warpbinding_reconcile_integration_test.go covers the warp-URL binding backfill of Topology.Reconcile:
// the recorded, present, diverged, skipped and deferred outcomes of reconcileWarpBinding in reconcile.go.
// The check runs exactly once per Reconcile call, however many pairs the hub has.
// A divergent or transport-only-differing record is reported but never overwritten.
// Unwire leaves a recorded binding untouched.
//
// Every test needs a hub built through CloneHub's own backfill path.
// The backfill reads the warp side's real "origin" remote as CloneHub sets it up.
// hubforge.NewHub's pre-wired template does not set it up that way.
// This file therefore builds its own newClonedHubFixture on top of CloneHub.
// It reuses makeBareRemote from clone_adopt_test.go.
//
// Package fabricengine_test isolates real end-to-end git fixtures from the package's internal unit tests.
// It shares the single TestMain in testmain_test.go with every other file in this package.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// clonedHubFixture bundles a cloned test hub: its clone result, its layout, and a ready-to-use Topology.
// The two bare remote paths let tests author expected binding content and sever and restore the warp origin.
type clonedHubFixture struct {
	Result      fabricengine.CloneResult
	Layout      *lymcwd.Location
	Topology    *fabricengine.Topology
	CodeBare    string
	RecordsBare string
}

// newClonedHubFixture builds a remote-backed hub through fabricengine.CloneHub against two bare fixtures.
// It seeds the repo-wide fabric config and resolves a ready-to-use Topology and Location.
// ForceBootstrap is set because the bare weft fixture is a seeded remote, not a repo that has been a weft;
// it therefore carries no .lym-anchor of its own.
//
// The fixture leaves the board worktree clean only after a local commit.
// CloneHub writes the anchor marker and seedRepoWideFabricConfig writes the repo-wide config; neither is committed.
// CloneHub never commits; only the CLI's Bolt.Commit does, so the board is dirty when CloneHub returns.
// A local stage-all commit through Bolt, with no push, closes that gap.
// Every "recorded"-outcome test below depends on that clean board:
// without it, reconcileWarpBinding's dirty-board guard defers the write instead.
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
		Shortname:      "tst",
	})
	if err != nil {
		t.Fatalf("CloneHub() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })

	seedRepoWideFabricConfig(t, res.HubPath)

	l, err := lymcwd.Resolve(res.PrimeCwd)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", res.PrimeCwd, err)
	}

	cfg, err := fabricengine.LoadConfig(configengine.Dirs{Board: fabricengine.BoardDir(res.HubPath)})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	top := fabricengine.NewTopology(cfg)

	if _, _, err := fabricengine.NewBolt(res.BoardDir).Commit("test fixture: record anchor + repo-wide config", fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("commit fixture board state: %v", err)
	}

	return clonedHubFixture{
		Result:      res,
		Layout:      l,
		Topology:    top,
		CodeBare:    warpBare,
		RecordsBare: weftBare,
	}
}

// unbindHub removes the recorded warp-binding file from boardDir's worktree and commits that deletion through a Bolt.
// That produces the unbound hub that predates the binding, explicitly rather than by assumption.
// The commit runs on boardDir itself, which leaves the board clean for the test.
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

// writeAndCommitBinding overwrites boardDir's recorded warp-binding file with url and commits the change through a Bolt.
// Tests use it to author a record that differs from the warp origin in a chosen way:
// normalized-equal, divergent, or transport-identity-equal.
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

// TestReconcile_BacksFillsBindingOnce asserts that an unbound wired hub gains the record on its first Reconcile:
// the outcome is "recorded", the board root holds the file with the warp origin URL,
// and no pair result carries a binding detail, because the binding is a repo-wide fact.
func TestReconcile_BacksFillsBindingOnce(t *testing.T) {
	tiergate.Require(t, 1)
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
	if got, want := strings.TrimSpace(string(data)), filepath.ToSlash(fixture.CodeBare); got != want {
		t.Errorf("binding content = %q; want %q (the warp origin URL)", got, want)
	}

	for _, pair := range result.Pairs {
		if strings.Contains(strings.ToLower(pair.Detail), "warp binding") {
			t.Errorf("pair %+v carries a warp-binding detail; want the binding reported only repo-wide", pair)
		}
	}
}

// TestReconcile_BacksFillsOnceOnMultiWorktreeHub asserts that with several warp worktrees present,
// the backfill still writes the record exactly once and reports it exactly once at the repo-wide level,
// and it leaves the per-pair results untouched.
func TestReconcile_BacksFillsOnceOnMultiWorktreeHub(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestReconcile_NormalizedRecordReportsPresent asserts that a trailing ".git" difference is normalized away:
// a record that differs from the warp origin only by that suffix reports "present", not "diverged".
func TestReconcile_NormalizedRecordReportsPresent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixture := newClonedHubFixture(t)

	origin := filepath.ToSlash(fixture.CodeBare)
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

// TestReconcile_DivergentRecordIsLeftUntouched asserts that a differing record is not overwritten:
// it reports "diverged" with both URLs named in the detail, and Reconcile still returns a nil error.
func TestReconcile_DivergentRecordIsLeftUntouched(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixture := newClonedHubFixture(t)

	origin := filepath.ToSlash(fixture.CodeBare)
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

// TestReconcile_TransportOnlyDifferenceIsAdvisory asserts that a transport-only difference is advisory:
// the record reports "diverged" with a detail that calls the difference transport-only and advisory.
// The warp origin is a local path with no scheme, so normalizeWarpURL applies no case-folding to it.
// Flipping its case gives two spellings that differ under normalizeWarpURL and stay "diverged".
// They collapse to one string under warpURLTransportIdentity, which lowercases unconditionally.
// This has the same shape as a real scheme-versus-scp-form transport swap.
func TestReconcile_TransportOnlyDifferenceIsAdvisory(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixture := newClonedHubFixture(t)

	origin := filepath.ToSlash(fixture.CodeBare)
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

// TestReconcile_DirtyBoardDefersWrite asserts that an uncommitted unrelated file defers the write:
// the outcome is "deferred" and no binding file is written.
// The unrelated file is neither committed nor pushed: it stays untracked, and the board's HEAD is unchanged.
func TestReconcile_DirtyBoardDefersWrite(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestReconcile_NoWarpOriginIsSkipped asserts that without a warp origin remote the outcome is "skipped":
// the detail is empty, and Reconcile still returns a nil error.
func TestReconcile_NoWarpOriginIsSkipped(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestReconcile_UnpushedRetryPushesOnPresent covers the engine half of the unpushed-retry path:
// once a record is committed and matches the warp origin, Reconcile reports "present" with an empty detail,
// and a second pass creates no further commit.
// Topology.Reconcile never pushes; the CLI handler (fabriccli.runReconcile) does.
// This test therefore drives Topology.Reconcile directly and asserts only the engine's verdict.
// The commit-then-push retry is covered at the CLI seam.
func TestReconcile_UnpushedRetryPushesOnPresent(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestUnwire_LeavesWarpBindingInPlace asserts that unwire on a bound hub leaves the record on the weft side untouched.
// Unwire already leaves the anchor marker and the repo-wide config in place the same way.
// No production change is needed; the test stops a future unwire edit from deleting the record.
func TestUnwire_LeavesWarpBindingInPlace(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixture := newClonedHubFixture(t)

	bindingPath := filepath.Join(fixture.Result.BoardDir, fabricengine.WarpBindingFileName)
	before, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}

	if _, err := fabricengine.Unwire(fixture.Result.PrimeCwd, true); err != nil {
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

// TestReconcile_BoardLockArtifactDoesNotDeferBackfill is the counterpart to TestReconcile_DirtyBoardDefersWrite.
// A lock file at the board root is machine-local scratch, not content a stage-all commit would sweep up.
// It must therefore not defer the once-per-hub binding backfill.
func TestReconcile_BoardLockArtifactDoesNotDeferBackfill(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixture := newClonedHubFixture(t)
	unbindHub(t, fixture.Result.BoardDir)

	for _, name := range []string{"tasks.json.swaplock", "board.write.lock"} {
		lockPath, err := fabricengine.BoltLockPath(fixture.Result.BoardDir, name)
		if err != nil {
			t.Fatalf("lock path of %s: %v", name, err)
		}
		if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
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
