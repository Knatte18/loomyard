// merge_target_integration_test.go covers Merge's target-pair scenario matrix against a real hubforge pair.
// It adds a second "target"/"target-weft" pair alongside the fixture's source pair.
// The target's *fabricengine.Fabric handle opens on its own worktree.
// The working directory stays on the source pair throughout, so the scenarios run on the path-anchored API.
// Reuses newMergePairFixture and its sibling helpers from mergein_integration_test.go.
// Both files share package fabricengine_test.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newMergeTargetFixture builds a hubforge pair at anchor, the source pair.
// It adds a second pair, "target"/"target-weft", and opens a *fabricengine.Fabric handle on its worktree.
// It returns h for path lookups on both pairs and target as the handle Merge runs on.
// It also returns the two closures that commit onto the source branches in the prime worktrees.
func newMergeTargetFixture(t *testing.T, anchor string) (h *hubforge.Hub, target *fabricengine.Fabric, commitOnSourceWarp, commitOnSourceWeft func(branch, filename, content, msg string)) {
	t.Helper()

	h, _, commitOnSourceWarp, commitOnSourceWeft, _, _ = newMergePairFixture(t, anchor)
	hubforge.AddPair(t, h, "target")

	l, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("target"))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s): %v", h.PairCodeWorktree("target"), err)
	}
	target, err = fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	return h, target, commitOnSourceWarp, commitOnSourceWeft
}

// advanceRemoteBranch commits filename and content on branch in a throwaway clone and pushes it to bareRemote.
// It moves origin's copy of branch without touching the local ref any worktree has checked out.
// That is the state Merge's pre-merge sync fetch must observe.
func advanceRemoteBranch(t *testing.T, bareRemote, branch, filename, content, msg string) {
	t.Helper()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "clone", "-q", "-b", branch, bareRemote, ".")
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.CommitFile(t, dir, filename, content, msg)
	gitkit.MustRun(t, dir, "git", "push", "-q", "origin", branch)
}

// seedSourceAndTarget seeds the source pair's "feature"/"feature-weft" branches with one clean commit
// each — the plain, no-conflict divergence most of this file's scenarios need.
func seedSourceAndTarget(t *testing.T, commitOnSourceWarp, commitOnSourceWeft func(branch, filename, content, msg string)) {
	t.Helper()
	commitOnSourceWarp("feature", "warp-feature.txt", "warp feature\n", "warp: add feature")
	commitOnSourceWeft("feature-weft", "weft-feature.txt", "weft feature\n", "weft: add feature")
}

// TestMerge_DirtyTargetHalts covers Merge's dirty-target guard.
// A dirty warp halts before mutating anything: the SHA is unchanged, no record is written,
// and the reason is the fixed "worktree dirty".
// A dirty weft does not halt the merge:
// the weft is not a merge participant, so its dirtiness cannot affect a warp-only merge's correctness;
// Merge lands the warp-side merge as it would against a clean weft,
// and the dirty weft file stays as the test left it (uncommitted, untouched, HEAD unmoved).
//
//testtiming:keep the operator's uncommitted weft edit left untouched by a warp-only Merge; coverage of its blocks by other tests does not show an assertion of this
func TestMerge_DirtyTargetHalts(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h1, target1, commitOnSourceWarp1, commitOnSourceWeft1 := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp1, commitOnSourceWeft1)
	targetWarpPath1 := h1.PairCodeWorktree("target")
	gitkit.CommitFile(t, targetWarpPath1, "tracked.txt", "v1\n", "target: seed tracked")
	warpBefore1 := fabricengine.CurrentSHAForTest(t, targetWarpPath1)
	weftBefore1 := fabricengine.CurrentSHAForTest(t, h1.PairRecordsSibling("target"))
	if err := os.WriteFile(filepath.Join(targetWarpPath1, "tracked.txt"), []byte("v2 (uncommitted)\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tracked.txt): %v", err)
	}
	_, errWarpDirty := target1.Merge("feature", fabricengine.MergeOptions{})

	var guardErrWarp *fabricengine.MergeGuardError
	if !errors.As(errWarpDirty, &guardErrWarp) {
		t.Fatalf("Merge(feature) [warp dirty] error = %v (%T); want *MergeGuardError", errWarpDirty, errWarpDirty)
	}
	if len(guardErrWarp.Reasons) != 1 || guardErrWarp.Reasons[0] != "worktree dirty" {
		t.Errorf("warp-dirty guard reasons = %v; want exactly [\"worktree dirty\"]", guardErrWarp.Reasons)
	}
	assertGuardDirt(t, guardErrWarp, targetWarpPath1, []string{"tracked.txt"})

	if got := fabricengine.CurrentSHAForTest(t, targetWarpPath1); got != warpBefore1 {
		t.Errorf("[warp dirty] target warp HEAD changed to %q; want unchanged %q", got, warpBefore1)
	}
	if got := fabricengine.CurrentSHAForTest(t, h1.PairRecordsSibling("target")); got != weftBefore1 {
		t.Errorf("[warp dirty] target weft HEAD changed to %q; want unchanged %q", got, weftBefore1)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(target1); err != nil || exists {
		t.Errorf("[warp dirty] MergeRecordExistsForTest() = (%v, %v); want (false, nil)", exists, err)
	}

	h2, target2, commitOnSourceWarp2, commitOnSourceWeft2 := newMergeTargetFixture(t, ".")
	targetWarpPath2 := h2.PairCodeWorktree("target")
	// A target-side warp commit makes merging "feature" a genuine (non-fast-forward) merge that fabricates a commit.
	// That shape is needed to assert Committed true below.
	gitkit.CommitFile(t, targetWarpPath2, "warp-progress.txt", "target progress\n", "target: progress")
	seedSourceAndTarget(t, commitOnSourceWarp2, commitOnSourceWeft2)
	targetWeftPath2 := h2.PairRecordsSibling("target")
	gitkit.CommitFile(t, targetWeftPath2, "_lym/tracked.txt", "v1\n", "target: seed tracked weft")
	warpBefore2 := fabricengine.CurrentSHAForTest(t, targetWarpPath2)
	weftBefore2 := fabricengine.CurrentSHAForTest(t, targetWeftPath2)
	if err := os.WriteFile(filepath.Join(targetWeftPath2, "_lym", "tracked.txt"), []byte("v2 (uncommitted)\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(_lym/tracked.txt): %v", err)
	}
	res, errWeftDirty := target2.Merge("feature", fabricengine.MergeOptions{})
	if errWeftDirty != nil {
		t.Fatalf("Merge(feature) [weft dirty] error = %v; want nil — a dirty weft must no longer halt a merge the warp alone completes", errWeftDirty)
	}
	if !res.Committed {
		t.Errorf("Merge(feature) [weft dirty].Committed = false; want true")
	}
	if got := fabricengine.CurrentSHAForTest(t, targetWarpPath2); got == warpBefore2 {
		t.Errorf("[weft dirty] target warp HEAD = %q; want it to have moved off %q", got, warpBefore2)
	}
	if got := fabricengine.CurrentSHAForTest(t, targetWeftPath2); got != weftBefore2 {
		t.Errorf("[weft dirty] target weft HEAD changed to %q; want unchanged %q — Merge never touches the weft", got, weftBefore2)
	}
	if got := gitkit.GitStatusPorcelain(t, targetWeftPath2); !strings.Contains(got, "_lym/tracked.txt") {
		t.Errorf("[weft dirty] weft git status --porcelain = %q; want it to still mention the uncommitted _lym/tracked.txt", got)
	}
}

// TestMerge_StaleTargetSyncsBeforeMerging covers a target behind its own upstream:
// Merge's pre-merge sync step fetches and fast-forwards the warp side (KindRepoAdvanced recorded) before merging,
// landing both the remote advance and the merge's own content.
// The weft side is not synced, since it is not a merge participant;
// a weft-side remote advance is left unfetched.
//
//testtiming:keep the repo_advanced mutation record entry from the pre-merge sync fast-forward; coverage of its blocks by other tests does not show an assertion of this
func TestMerge_StaleTargetSyncsBeforeMerging(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	advanceRemoteBranch(t, h.CodeBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("Merge(feature).Committed = false; want true")
	}

	foundAdvance := false
	for _, e := range res.Mutated().Entries() {
		if e.Kind == fabricengine.KindRepoAdvanced {
			foundAdvance = true
		}
	}
	if !foundAdvance {
		t.Errorf("Merge(feature).Mutated() = %v; want at least one repo_advanced entry from the pre-merge sync step", res.Mutated().Entries())
	}

	if _, err := os.Stat(filepath.Join(h.PairCodeWorktree("target"), "remote-warp.txt")); err != nil {
		t.Errorf("remote-warp.txt missing after Merge: %v; want it fetched and fast-forwarded before merging", err)
	}
}

// TestMerge_DivergedTargetRefuses covers a diverged target:
// local commits past an already fetched, moved-elsewhere upstream.
// It expects Merge to refuse with *MergeGuardError carrying "branch not synced to upstream" and to mutate nothing.
func TestMerge_DivergedTargetRefuses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	targetWarpPath := h.PairCodeWorktree("target")
	advanceRemoteBranch(t, h.CodeBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")
	// Fetch so the guard's own read-only @{u} check sees the remote advance (the guard stage never
	// fetches on its own), then diverge locally so neither direction is an ancestor of the other.
	gitkit.MustRun(t, targetWarpPath, "git", "fetch", "-q", "origin")
	gitkit.CommitFile(t, targetWarpPath, "local-warp.txt", "local content\n", "target: local progress")

	warpBefore := fabricengine.CurrentSHAForTest(t, targetWarpPath)
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PairRecordsSibling("target"))

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("Merge(feature) error = %v (%T); want *MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "branch not synced to upstream" {
		t.Errorf("Merge(feature) guard reasons = %v; want exactly [\"branch not synced to upstream\"]", guardErr.Reasons)
	}

	if got := fabricengine.CurrentSHAForTest(t, targetWarpPath); got != warpBefore {
		t.Errorf("target warp HEAD changed to %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PairRecordsSibling("target")); got != weftBefore {
		t.Errorf("target weft HEAD changed to %q; want unchanged %q", got, weftBefore)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("Merge(feature).Mutated() = %v; want empty (the guard stage is strictly read-only)", res.Mutated().Entries())
	}
}

// TestMerge_ConflictSelfAborts covers a warp-side merge that would conflict: it self-aborts,
// restoring the target pair's warp SHA and worktree exactly.
// It returns *ErrMergeInRequired with Source set and leaves no record.
// Merge merges only the warp side, so a weft-side conflict cannot occur.
func TestMerge_ConflictSelfAborts(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hWarp, targetWarp, commitOnSourceWarp1, commitOnSourceWeft1 := newMergeTargetFixture(t, ".")
	gitkit.CommitFile(t, hWarp.PairCodeWorktree("target"), "conflict.txt", "target content\n", "target: seed conflict.txt")
	commitOnSourceWarp1("feature", "conflict.txt", "feature content\n", "feature: diverge conflict.txt")
	commitOnSourceWeft1("feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")

	warpBeforeW := fabricengine.CurrentSHAForTest(t, hWarp.PairCodeWorktree("target"))
	weftBeforeW := fabricengine.CurrentSHAForTest(t, hWarp.PairRecordsSibling("target"))

	_, errWarpConflict := targetWarp.Merge("feature", fabricengine.MergeOptions{})
	var reqWarp *fabricengine.ErrMergeInRequired
	if !errors.As(errWarpConflict, &reqWarp) {
		t.Fatalf("Merge(feature) [warp conflict] error = %v (%T); want *ErrMergeInRequired", errWarpConflict, errWarpConflict)
	}
	if reqWarp.Source != "feature" {
		t.Errorf("[warp conflict] ErrMergeInRequired.Source = %q; want %q", reqWarp.Source, "feature")
	}
	if got := fabricengine.CurrentSHAForTest(t, hWarp.PairCodeWorktree("target")); got != warpBeforeW {
		t.Errorf("[warp conflict] target warp HEAD = %q; want restored pre-merge SHA %q", got, warpBeforeW)
	}
	if got := fabricengine.CurrentSHAForTest(t, hWarp.PairRecordsSibling("target")); got != weftBeforeW {
		t.Errorf("[warp conflict] target weft HEAD = %q; want restored pre-merge SHA %q", got, weftBeforeW)
	}
	if out := gitkit.GitStatusPorcelain(t, hWarp.PairCodeWorktree("target")); out != "" {
		t.Errorf("[warp conflict] target warp git status --porcelain = %q; want clean", out)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(targetWarp); err != nil || exists {
		t.Errorf("[warp conflict] MergeRecordExistsForTest() = (%v, %v); want (false, nil)", exists, err)
	}
	if inProgress, err := targetWarp.MergeInProgress(); err != nil || inProgress {
		t.Errorf("[warp conflict] MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
	}
}

// TestMerge_BothSidesAlreadyUpToDate covers the degenerate no-op: AlreadyUpToDate true, no record
// written (and, by extension, no lock taken).
func TestMerge_BothSidesAlreadyUpToDate(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, _, _ := newMergeTargetFixture(t, ".")

	branchAtCurrentHEAD(t, h.PrimeWorktree(), "feature")
	branchAtCurrentHEAD(t, h.PrimeRecords(), "feature-weft")

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v", err)
	}
	if !res.AlreadyUpToDate {
		t.Errorf("Merge(feature).AlreadyUpToDate = false; want true")
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("Merge(feature).Mutated().Len() = %d; want 0 (empty mutation record)", res.Mutated().Len())
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(target); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after already-up-to-date Merge = (%v, %v); want (false, nil)", exists, err)
	}
}

// divergeSideWithoutFetching creates the unfetched-divergence shape on one side of the target pair:
// someone else advances the side's own upstream branch on the bare remote;
// the side then makes a local commit without fetching,
// so its remote-tracking ref still names a commit that is an ancestor of its own HEAD.
// It asserts that shape with t.Fatal rather than assuming it.
// A fixture that fetched by accident would see a plain divergence and prove nothing about the post-fetch check.
func divergeSideWithoutFetching(t *testing.T, worktree, bareRemote, branch, remoteFile, localFile string) {
	t.Helper()

	advanceRemoteBranch(t, bareRemote, branch, remoteFile, "remote content\n", "origin: advance "+branch)
	gitkit.CommitFile(t, worktree, localFile, "local content\n", "local progress on "+branch)

	head := gitkit.RevParse(t, worktree, "HEAD")
	tracking := gitkit.RevParse(t, worktree, "origin/"+branch)
	if tracking == head {
		t.Fatalf("origin/%s in %s equals HEAD (%s); want a stale remote-tracking ref — this fixture must NOT have fetched", branch, worktree, head)
	}
	if !gitkit.IsAncestor(t, worktree, tracking, head) {
		t.Fatalf("origin/%s (%s) in %s is not an ancestor of HEAD (%s); want the stale-tracking shape where the pre-lock guard classifies this side as merely ahead and passes", branch, tracking, worktree, head)
	}
	if remoteTip := gitkit.RevParse(t, bareRemote, branch); remoteTip == tracking || remoteTip == head {
		t.Fatalf("real %s tip %s equals the stale tracking ref or HEAD in %s; want a genuine divergence", branch, remoteTip, worktree)
	}
}

// assertMergeRefusedAsNotSynced asserts res/err is exactly the not-synced guard refusal and that
// neither side of the target pair moved.
func assertMergeRefusedAsNotSynced(t *testing.T, h *hubforge.Hub, res fabricengine.MergeResult, err error, warpBefore, weftBefore string) {
	t.Helper()

	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("Merge(feature) = (committed %v, error %v (%T)); want *MergeGuardError", res.Committed, err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "branch not synced to upstream" {
		t.Errorf("Merge(feature) guard reasons = %v; want exactly [\"branch not synced to upstream\"]", guardErr.Reasons)
	}
	if res.Committed {
		t.Errorf("Merge(feature).Committed = true; want false — a refused merge lands nothing")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PairCodeWorktree("target")); got != warpBefore {
		t.Errorf("target warp HEAD = %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PairRecordsSibling("target")); got != weftBefore {
		t.Errorf("target weft HEAD = %q; want unchanged %q", got, weftBefore)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(openFreshFabric(t, h.PairCodeWorktree("target"))); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMerge_UnfetchedDivergedTargetRefuses covers a warp target whose upstream diverged on the remote.
// This checkout has not fetched the divergence.
// The pre-lock guard resolves the upstream before anything has fetched, so the divergence reads as merely ahead.
// Merge then fetches, and the post-fetch check must refuse; without it, Merge commits over the divergence.
func TestMerge_UnfetchedDivergedTargetRefuses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	divergeSideWithoutFetching(t, h.PairCodeWorktree("target"), h.CodeBare, "target", "remote-warp.txt", "local-warp.txt")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PairCodeWorktree("target"))
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PairRecordsSibling("target"))

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	assertMergeRefusedAsNotSynced(t, h, res, err, warpBefore, weftBefore)
}

// TestMerge_FetchedBehindTargetIsSyncedNotRefused pins the behind arm of `sideNotSyncedToUpstream`:
// a target whose upstream advance is fetched, so HEAD is strictly behind it, must count as synced.
// Merge's own pre-merge sync step advances it, and refusing it would make every stale target unmergeable.
func TestMerge_FetchedBehindTargetIsSyncedNotRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	targetWarpPath := h.PairCodeWorktree("target")
	advanceRemoteBranch(t, h.CodeBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")
	gitkit.MustRun(t, targetWarpPath, "git", "fetch", "-q", "origin")

	// Precondition: strictly behind — HEAD is an ancestor of the fetched upstream and not equal to it,
	// so the predicate must reach its behind arm rather than its equality or ahead arm.
	head := gitkit.RevParse(t, targetWarpPath, "HEAD")
	tracking := gitkit.RevParse(t, targetWarpPath, "origin/target")
	if head == tracking {
		t.Fatalf("HEAD and origin/target are both %s; want HEAD strictly behind a fetched upstream", head)
	}
	if !gitkit.IsAncestor(t, targetWarpPath, head, tracking) {
		t.Fatalf("HEAD (%s) is not an ancestor of origin/target (%s); want the strictly-behind shape", head, tracking)
	}

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v; want a behind-but-not-diverged target to pass the sync guard and be fast-forwarded", err)
	}
	if !res.Committed {
		t.Errorf("Merge(feature).Committed = false; want true")
	}
	if _, err := os.Stat(filepath.Join(targetWarpPath, "remote-warp.txt")); err != nil {
		t.Errorf("remote-warp.txt missing after Merge: %v; want the pre-merge sync step to have fast-forwarded it in", err)
	}
}
