// mergein_integration_test.go covers MergeIn's scenarios against a real hubforge pair:
// the mixed already-up-to-date merge, MergeContinue's refusal on unresolved conflicts, and MergeInAt's given tip.
// It also holds the fixtures and layers the other merge integration tests share.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newMergePairFixture builds a real hubforge pair anchored at anchor.
// It returns the pair's Hub and the Fabric opened over its prime warp and weft worktrees.
// The commitOnWarpBranch and commitOnWeftBranch closures commit on a named branch and switch back,
// so the checkout MergeIn runs against stays on its original branch.
func newMergePairFixture(t *testing.T, anchor string) (h *hubforge.Hub, f *fabricengine.Fabric, commitOnWarpBranch, commitOnWeftBranch func(branch, filename, content, msg string), commitOnWarpCurrent, commitOnWeftCurrent func(filename, content, msg string)) {
	t.Helper()
	return mergePairFixtureOf(t, hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor}))
}

// newMergeLayerFixture copies layer, a "." hub template whose build already made a merge scenario's commits.
// It returns the copy's Hub and the Fabric opened over its prime warp and weft worktrees.
// The commitOnWarpBranch and commitOnWeftBranch closures commit on a named branch and switch back,
// so the checkout MergeIn runs against stays on its original branch.
func newMergeLayerFixture(t *testing.T, layer *hubforge.Registration) (h *hubforge.Hub, f *fabricengine.Fabric, commitOnWarpBranch, commitOnWeftBranch func(branch, filename, content, msg string), commitOnWarpCurrent, commitOnWeftCurrent func(filename, content, msg string)) {
	t.Helper()
	return mergePairFixtureOf(t, hubforge.CopyRegistered(t, layer))
}

// The merge layers: "." hubs whose prime pair carries a merge scenario's branch commits.
// Each holds a start state that at least three tests share.
var (
	// conflictLayer's warp branch "feature" conflicts with the current branch on conflict.txt,
	// and its weft branch "feature-weft" is at the weft's HEAD.
	conflictLayer = hubforge.Registration{
		Key:   "fabricengine-merge-conflict",
		Base:  hubforge.Shape{Anchor: "."},
		Build: conflictingFeature("conflict.txt"),
	}
	// clashLayer is conflictLayer's scenario with the conflict on clash.txt.
	clashLayer = hubforge.Registration{
		Key:   "fabricengine-merge-clash",
		Base:  hubforge.Shape{Anchor: "."},
		Build: conflictingFeature("clash.txt"),
	}
	// nonFastForwardLayer's warp branch "feature" adds warp-branch.txt, the current branch has since added warp-current.txt,
	// and its weft branch "feature-weft" is at the weft's HEAD.
	nonFastForwardLayer = hubforge.Registration{
		Key:   "fabricengine-merge-nonff",
		Base:  hubforge.Shape{Anchor: "."},
		Build: nonFastForwardFeature("warp-branch.txt", "warp-current.txt"),
	}
	// featureBothSidesLayer has "feature" and "feature-weft" each one commit adding feature.txt past the current branches.
	featureBothSidesLayer = hubforge.Registration{
		Key:  "fabricengine-merge-feature-both-sides",
		Base: hubforge.Shape{Anchor: "."},
		Build: func(tb testing.TB, h *hubforge.Hub) {
			tb.Helper()
			commitOnBranch(tb, h.PrimeWorktree(), "feature", "feature.txt", "feature\n", "feature: warp")
			commitOnBranch(tb, h.PrimeRecords(), "feature-weft", "feature.txt", "feature\n", "feature: weft")
		},
	}
	// cleanMergeableLayer has "feature" and "feature-weft" each one commit past the current branches.
	// Both merge without conflict.
	cleanMergeableLayer = hubforge.Registration{
		Key:  "fabricengine-merge-clean",
		Base: hubforge.Shape{Anchor: "."},
		Build: func(tb testing.TB, h *hubforge.Hub) {
			tb.Helper()
			commitOnBranch(tb, h.PrimeWorktree(), "feature", "clean-warp.txt", "clean\n", "warp: clean branch")
			commitOnBranch(tb, h.PrimeRecords(), "feature-weft", "_lym/clean-weft.txt", "clean\n", "weft: clean branch")
		},
	}
)

// conflictingFeature returns a layer build for a conflict on filename.
// The build makes the warp branch "feature" conflict with the current branch on that file.
// It branches "feature-weft" at the weft's HEAD.
func conflictingFeature(filename string) func(tb testing.TB, h *hubforge.Hub) {
	return func(tb testing.TB, h *hubforge.Hub) {
		tb.Helper()
		setupConflictingDivergence(tb, h.PrimeWorktree(), "feature", filename)
		branchAtCurrentHEAD(tb, h.PrimeRecords(), "feature-weft")
	}
}

// nonFastForwardFeature returns a layer build whose warp branch "feature" adds branchFile.
// The current branch advances past the branch point by adding currentFile.
// It branches "feature-weft" at the weft's HEAD.
func nonFastForwardFeature(branchFile, currentFile string) func(tb testing.TB, h *hubforge.Hub) {
	return func(tb testing.TB, h *hubforge.Hub) {
		tb.Helper()
		setupCleanNonFastForward(tb, h.PrimeWorktree(), "feature", branchFile, currentFile)
		branchAtCurrentHEAD(tb, h.PrimeRecords(), "feature-weft")
	}
}

// mergePairFixtureOf returns h, the Fabric opened over its prime warp and weft worktrees,
// and the commit closures over them.
func mergePairFixtureOf(t *testing.T, h *hubforge.Hub) (_ *hubforge.Hub, f *fabricengine.Fabric, commitOnWarpBranch, commitOnWeftBranch func(branch, filename, content, msg string), commitOnWarpCurrent, commitOnWeftCurrent func(filename, content, msg string)) {
	t.Helper()

	f = hubforge.OpenFabric(t, h)

	warpDir, weftDir := h.PrimeWorktree(), h.PrimeRecords()
	commitOnWarpBranch = func(branch, filename, content, msg string) {
		commitOnBranch(t, warpDir, branch, filename, content, msg)
	}
	commitOnWeftBranch = func(branch, filename, content, msg string) {
		commitOnBranch(t, weftDir, branch, filename, content, msg)
	}
	commitOnWarpCurrent = func(filename, content, msg string) {
		gitkit.CommitFile(t, warpDir, filename, content, msg)
	}
	commitOnWeftCurrent = func(filename, content, msg string) {
		gitkit.CommitFile(t, weftDir, filename, content, msg)
	}
	return h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent
}

// commitOnBranch checks out branch in dir, creating it off the current checkout when it does not exist yet.
// It writes filename with content, commits msg, and switches back to the previous branch.
func commitOnBranch(tb testing.TB, dir, branch, filename, content, msg string) {
	tb.Helper()

	current := gitkit.CurrentBranch(tb, dir)
	if gitkit.BranchExists(tb, dir, branch) {
		gitkit.MustRun(tb, dir, "git", "checkout", "-q", branch)
	} else {
		gitkit.MustRun(tb, dir, "git", "checkout", "-q", "-b", branch)
	}

	gitkit.CommitFile(tb, dir, filename, content, msg)

	gitkit.MustRun(tb, dir, "git", "checkout", "-q", current)
}

// branchAtCurrentHEAD creates branch in dir at the current HEAD, without checking it out or adding a commit.
// Merging it reports already up to date.
func branchAtCurrentHEAD(tb testing.TB, dir, branch string) {
	tb.Helper()
	gitkit.MustRun(tb, dir, "git", "branch", branch)
}

// setupConflictingDivergence commits filename on dir's current branch.
// It then writes different content to filename on branch and on the current branch.
// Merging branch into the current branch then conflicts on filename.
func setupConflictingDivergence(tb testing.TB, dir, branch, filename string) {
	tb.Helper()

	gitkit.CommitFile(tb, dir, filename, "seed content\n", "seed "+filename)
	commitOnBranch(tb, dir, branch, filename, "branch content\n", "diverge "+filename+" on "+branch)
	gitkit.CommitFile(tb, dir, filename, "current content\n", "diverge "+filename+" on current")
}

// setupCleanFastForward creates branch off dir's current HEAD with one commit adding filename.
// The current branch never advances past the branch point, so merging branch fast-forwards.
func setupCleanFastForward(t *testing.T, dir, branch, filename string) {
	t.Helper()
	commitOnBranch(t, dir, branch, filename, "clean content\n", "clean "+filename+" on "+branch)
}

// setupCleanNonFastForward creates branch off dir's current HEAD with a commit adding branchFile.
// It then advances the current branch with a commit adding currentFile, so merging branch is not a fast-forward.
func setupCleanNonFastForward(tb testing.TB, dir, branch, branchFile, currentFile string) {
	tb.Helper()
	commitOnBranch(tb, dir, branch, branchFile, "clean branch content\n", "clean "+branchFile+" on "+branch)
	gitkit.CommitFile(tb, dir, currentFile, "current progress\n", "progress current past "+branch)
}

// With the weft no longer a merge participant, a fast-forwarding warp side means the whole call
// concludes cleanly and is never conflicted.

// TestMergeIn_OneSideAlreadyUpToDate_OtherMerges covers a mixed already-up-to-date merge.
// The merge concludes and correspondence pairs the new SHA with the unchanged one.
// No empty commit is fabricated on the no-op side.
// The merging side fast-forwards (`setupCleanFastForward`), so no conclude commit is fabricated there either.
// Committed is false: the pair advanced without any merge commit existing.
// AlreadyUpToDate is likewise false: one side did move.
//
//testtiming:keep no merge_staged or merge_committed mutation record entry targeting the already-up-to-date weft side; coverage of its blocks by other tests does not show an assertion of this
func TestMergeIn_OneSideAlreadyUpToDate_OtherMerges(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	setupCleanFastForward(t, h.PrimeWorktree(), "feature", "warp-ff.txt")
	branchAtCurrentHEAD(t, h.PrimeRecords(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if res.Committed {
		t.Errorf("MergeIn(feature).Committed = true; want false — the merging side fast-forwarded, so no conclude-commit was fabricated on either side")
	}
	if res.AlreadyUpToDate {
		t.Error("MergeIn(feature).AlreadyUpToDate = true; want false — one side did advance")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got == weftStartSHA {
		t.Error("warp HEAD did not move; want the fast-forward advance")
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftStartSHA {
		t.Errorf("weft HEAD after MergeIn = %q; want unchanged %q (already up to date)", got, weftStartSHA)
	}

	warpHEAD := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftSHA, err := f.WeftSHAForWarpSHA(warpHEAD)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA(%s) error = %v", warpHEAD, err)
	}
	if weftSHA != weftStartSHA {
		t.Errorf("correspondence for new warp HEAD = %q; want it paired with the unchanged weft SHA %q", weftSHA, weftStartSHA)
	}

	for _, e := range res.Mutated().Entries() {
		if e.Kind == fabricengine.KindMergeStaged && e.Target == hubRelForTest(t, f, h.PrimeRecords()) {
			t.Errorf("mutation record carries a merge_staged entry for the already-up-to-date weft side: %+v", e)
		}
		if e.Kind == fabricengine.KindMergeCommitted && e.Target == hubRelForTest(t, f, h.PrimeRecords()) {
			t.Errorf("mutation record carries a merge_committed entry for the already-up-to-date weft side: %+v", e)
		}
	}
}

// hubRelForTest returns target as a slash-separated path relative to the hub directory.
// A test uses it to compare a raw path with a recorded mutation target without repeating the conversion.
func hubRelForTest(t *testing.T, f *fabricengine.Fabric, target string) string {
	t.Helper()
	rel, err := filepath.Rel(filepath.Dir(fabricengine.WarpPathForTest(f)), target)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	return filepath.ToSlash(rel)
}

// TestMergeContinue_UnresolvedConflictsRefuse covers MergeContinue's refusal while conflict markers remain:
// a *MergeGuardError whose sole reason is the fixed "unresolved conflicts remain".
func TestMergeContinue_UnresolvedConflictsRefuse(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	_, f, _, _, _, _ := newMergeLayerFixture(t, &conflictLayer)

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	_, err := f.MergeContinue("")
	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("MergeContinue(\"\") error = %v (%T); want *MergeGuardError", err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != "unresolved conflicts remain" {
		t.Errorf("MergeContinue(\"\") guard reasons = %v; want exactly [\"unresolved conflicts remain\"]", guardErr.Reasons)
	}
}

// TestMergeInAt_MergesTheGivenTipNotTheLocalOne covers MergeInAt's code-side SHA argument.
// A tip behind the local branch merges only that tip.
// A given tip that conflicts parks a merge-in of the named source, recording the given SHA.
// A SHA naming no commit is refused by the source-not-found guard, with nothing merged.
func TestMergeInAt_MergesTheGivenTipNotTheLocalOne(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("TipBehindLocalBranch", func(t *testing.T) {
		t.Parallel()

		h, f, commitOnWarpBranch, _, _, _ := newMergePairFixture(t, ".")
		warpDir := h.PrimeWorktree()

		commitOnWarpBranch("feature", "upstream.txt", "upstream\n", "warp: upstream commit")
		gitkit.MustRun(t, warpDir, "git", "push", "-q", "origin", "feature")
		upstreamTip := gitkit.RevParse(t, warpDir, "origin/feature")
		commitOnWarpBranch("feature", "local-only.txt", "local only\n", "warp: local-only commit")

		res, err := f.MergeInAt("feature", upstreamTip)
		if err != nil {
			t.Fatalf("MergeInAt(feature, %s) error = %v", upstreamTip, err)
		}
		if len(res.Conflicts) != 0 {
			t.Errorf("MergeInAt(feature, %s).Conflicts = %v; want empty", upstreamTip, res.Conflicts)
		}
		if _, err := os.Stat(filepath.Join(warpDir, "upstream.txt")); err != nil {
			t.Errorf("upstream.txt after MergeInAt: %v; want the told tip's file merged in", err)
		}
		if _, err := os.Stat(filepath.Join(warpDir, "local-only.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("local-only.txt after MergeInAt: err = %v; want it absent, the local-only commit is not the told tip", err)
		}
	})

	t.Run("ConflictParksTheToldTip", func(t *testing.T) {
		t.Parallel()

		h, f, _, _, _, _ := newMergePairFixture(t, ".")
		warpDir := h.PrimeWorktree()
		setupConflictingDivergence(t, warpDir, "feature", "conflict.txt")
		tip := gitkit.RevParse(t, warpDir, "feature")

		res, err := f.MergeInAt("feature", tip)
		if err != nil {
			t.Fatalf("MergeInAt(feature, %s) error = %v", tip, err)
		}
		if len(res.Conflicts) == 0 {
			t.Fatalf("MergeInAt(feature, %s).Conflicts is empty; want the conflicted path", tip)
		}

		state, err := f.MidMerge()
		if err != nil {
			t.Fatalf("MidMerge() error = %v", err)
		}
		if state.Kind != fabricengine.MidMergeParked || state.Verb != fabricengine.MergeVerbMergeIn {
			t.Errorf("MidMerge() = kind %v verb %q; want a parked %q", state.Kind, state.Verb, fabricengine.MergeVerbMergeIn)
		}
		if state.Source != "feature" || state.SourceSHA != tip {
			t.Errorf("MidMerge() source = %q at %q; want %q at %q", state.Source, state.SourceSHA, "feature", tip)
		}
	})

	t.Run("UnknownTipIsSourceNotFound", func(t *testing.T) {
		t.Parallel()

		h, f, commitOnWarpBranch, _, _, _ := newMergePairFixture(t, ".")
		warpDir := h.PrimeWorktree()
		commitOnWarpBranch("feature", "feature.txt", "feature\n", "warp: feature commit")
		headBefore := gitkit.RevParse(t, warpDir, "HEAD")

		_, err := f.MergeInAt("feature", "0123456789abcdef0123456789abcdef01234567")
		var guardErr *fabricengine.MergeGuardError
		if !errors.As(err, &guardErr) {
			t.Fatalf("MergeInAt(feature, unknown SHA) error = %v (%T); want *fabricengine.MergeGuardError", err, err)
		}
		if !slices.Contains(guardErr.Reasons, "source branch not found") {
			t.Errorf("MergeInAt(feature, unknown SHA) reasons = %v; want source branch not found", guardErr.Reasons)
		}
		if headAfter := gitkit.RevParse(t, warpDir, "HEAD"); headAfter != headBefore {
			t.Errorf("HEAD after the refused MergeInAt = %s; want %s unchanged", headAfter, headBefore)
		}
		if inProgress, err := f.MergeInProgress(); err != nil || inProgress {
			t.Errorf("MergeInProgress() after the refused MergeInAt = (%v, %v); want (false, nil)", inProgress, err)
		}
	})
}
