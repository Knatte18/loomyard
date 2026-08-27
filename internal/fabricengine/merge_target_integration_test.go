//go:build integration

// merge_target_integration_test.go covers Merge's target-pair scenario matrix against a real
// hubforge pair: a second "target"/"target-weft" pair added alongside the fixture's own source pair,
// with a *fabricengine.Fabric handle opened on the target's own worktree via lyxcwd.ResolveWorktree +
// fabricengine.Open — driven while cwd stays on the source pair, proving the path-anchored,
// cwd-gate-free Go API the handle decision requires.
// Reuses newMergePairFixture and its sibling helpers from mergein_integration_test.go, since both
// files share package fabricengine_test.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// newMergeTargetFixture builds a real hubforge pair (the source/task pair, at anchor) plus a second
// pair "target"/"target-weft" (hubforge.AddPair) — Merge's target — and opens a *fabricengine.Fabric
// handle on the target pair's own worktree via lyxcwd.ResolveWorktree + fabricengine.Open, entirely
// independent of the source pair's own handle newMergePairFixture already opened.
// It returns h (for path lookups on both pairs), target (the Merge-under-test handle), and the two
// closures newMergePairFixture already builds for diverging the source pair's branches directly on
// the prime warp/weft worktrees.
func newMergeTargetFixture(t *testing.T, anchor string) (h *hubforge.Hub, target *fabricengine.Fabric, commitOnSourceWarp, commitOnSourceWeft func(branch, filename, content, msg string)) {
	t.Helper()

	h, _, commitOnSourceWarp, commitOnSourceWeft, _, _ = newMergePairFixture(t, anchor)
	hubforge.AddPair(t, h, "target")

	l, err := lyxcwd.ResolveWorktree(h.PairWarpWorktree("target"))
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", h.PairWarpWorktree("target"), err)
	}
	target, err = fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	return h, target, commitOnSourceWarp, commitOnSourceWeft
}

// gitParentCount returns the number of parents rev has in dir, via `git log -1 --format=%P`.
func gitParentCount(t *testing.T, dir, rev string) int {
	t.Helper()

	cmd := exec.Command("git", "log", "-1", "--format=%P", rev)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%P %s in %s: %v", rev, dir, err)
	}
	parents := strings.Fields(strings.TrimSpace(string(out)))
	return len(parents)
}

// gitCommitSubject returns rev's commit subject line in dir, via `git log -1 --format=%s`.
func gitCommitSubject(t *testing.T, dir, rev string) string {
	t.Helper()

	cmd := exec.Command("git", "log", "-1", "--format=%s", rev)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%s %s in %s: %v", rev, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// advanceRemoteBranch commits filename/content on branch and pushes it to bareRemote from a
// throwaway clone, entirely independent of any of the fixture's own worktrees — advancing origin's
// copy of branch without ever touching the local ref any worktree has checked out, the shape Merge's
// pre-merge sync step (a fetch the guard stage never runs) needs to observe.
func advanceRemoteBranch(t *testing.T, bareRemote, branch, filename, content, msg string) {
	t.Helper()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "clone", "-q", "-b", branch, bareRemote, ".")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	commitOnCurrentBranch(t, dir, filename, content, msg)
	gitkit.MustRun(t, dir, "git", "push", "-q", "origin", branch)
}

// mutationKinds extracts the ordered Kind sequence from res's mutation record, for structural
// (never-SHA, never-path) shape comparisons across two fixtures with different hub roots.
func mutationKinds(res fabricengine.MergeResult) []fabricengine.Kind {
	entries := res.Mutated().Entries()
	kinds := make([]fabricengine.Kind, len(entries))
	for i, e := range entries {
		kinds[i] = e.Kind
	}
	return kinds
}

// seedSourceAndTarget seeds the source pair's "feature"/"feature-weft" branches with one clean commit
// each — the plain, no-conflict divergence most of this file's scenarios need.
func seedSourceAndTarget(t *testing.T, commitOnSourceWarp, commitOnSourceWeft func(branch, filename, content, msg string)) {
	t.Helper()
	commitOnSourceWarp("feature", "warp-feature.txt", "warp feature\n", "warp: add feature")
	commitOnSourceWeft("feature-weft", "weft-feature.txt", "weft feature\n", "weft: add feature")
}

// TestMerge_CleanSquash covers the clean squash-merge scenario: Committed true, a single new
// one-parent commit on both sides, correspondence recorded for the target pair, and the record
// deleted.
func TestMerge_CleanSquash(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	res, err := target.Merge("feature", fabricengine.MergeOptions{Squash: true})
	if err != nil {
		t.Fatalf("Merge(feature, squash) error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("Merge(feature, squash).Committed = false; want true")
	}

	targetWarpPath, targetWeftPath := h.PairWarpWorktree("target"), h.PairWeftSibling("target")
	warpHEAD := fabricengine.CurrentSHAForTest(t, targetWarpPath)
	weftHEAD := fabricengine.CurrentSHAForTest(t, targetWeftPath)

	if got := gitParentCount(t, targetWarpPath, warpHEAD); got != 1 {
		t.Errorf("target warp HEAD %s has %d parents; want exactly 1 (a squash commit)", warpHEAD, got)
	}
	if got := gitParentCount(t, targetWeftPath, weftHEAD); got != 1 {
		t.Errorf("target weft HEAD %s has %d parents; want exactly 1 (a squash commit)", weftHEAD, got)
	}

	weftSHA, err := target.WeftSHAForWarpSHA(warpHEAD)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA(%s) error = %v; want the target pair's correspondence recorded", warpHEAD, err)
	}
	if weftSHA != weftHEAD {
		t.Errorf("correspondence for %s = %q; want %q", warpHEAD, weftSHA, weftHEAD)
	}

	if exists, err := fabricengine.MergeRecordExistsForTest(target); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after clean squash Merge = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMerge_CleanNonSquash covers the clean non-squash merge scenario: a real merge commit with two
// parents on the warp side, plain `git merge` semantics preserved, and the weft side left byte-for-byte
// unmoved — Merge is no longer a weft-side merge participant.
func TestMerge_CleanNonSquash(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")

	targetWarpPath, targetWeftPath := h.PairWarpWorktree("target"), h.PairWeftSibling("target")
	// A target-side commit on each checkout, so merging "feature" is a genuine (non-fast-forward)
	// merge on the warp side rather than a plain pointer advance. The weft-side commit only proves the
	// weft checkout stays put across the merge.
	commitOnCurrentBranch(t, targetWarpPath, "target-progress.txt", "target progress\n", "target: progress")
	commitOnCurrentBranch(t, targetWeftPath, "_lyx/target-progress.txt", "target progress\n", "target: progress weft")
	weftBefore := fabricengine.CurrentSHAForTest(t, targetWeftPath)

	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("Merge(feature).Committed = false; want true")
	}

	warpHEAD := fabricengine.CurrentSHAForTest(t, targetWarpPath)
	if got := gitParentCount(t, targetWarpPath, warpHEAD); got != 2 {
		t.Errorf("target warp HEAD %s has %d parents; want exactly 2 (a real merge commit)", warpHEAD, got)
	}
	if got := fabricengine.CurrentSHAForTest(t, targetWeftPath); got != weftBefore {
		t.Errorf("target weft HEAD = %q; want unchanged %q — the weft is not a merge participant", got, weftBefore)
	}
}

// TestMerge_MessagePrecedence covers Merge's Message option: empty uses git's own prepared message,
// set is used verbatim on the warp side — the only side Merge concludes.
func TestMerge_MessagePrecedence(t *testing.T) {
	t.Run("empty uses git's prepared message", func(t *testing.T) {
		h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
		seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

		res, err := target.Merge("feature", fabricengine.MergeOptions{Squash: true})
		if err != nil {
			t.Fatalf("Merge(feature, squash) error = %v", err)
		}
		if !res.Committed {
			t.Fatalf("Merge(feature, squash).Committed = false; want true")
		}

		subj := gitCommitSubject(t, h.PairWarpWorktree("target"), "HEAD")
		if !strings.Contains(strings.ToLower(subj), "squash") {
			t.Errorf("warp commit subject = %q; want git's own prepared squash message", subj)
		}
	})

	t.Run("set is used verbatim on the warp side", func(t *testing.T) {
		h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
		seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)
		weftBefore := fabricengine.CurrentSHAForTest(t, h.PairWeftSibling("target"))

		const msg = "custom target-pair merge message"
		res, err := target.Merge("feature", fabricengine.MergeOptions{Squash: true, Message: msg})
		if err != nil {
			t.Fatalf("Merge(feature, squash, message) error = %v", err)
		}
		if !res.Committed {
			t.Fatalf("Merge(feature, squash, message).Committed = false; want true")
		}

		if got := gitCommitSubject(t, h.PairWarpWorktree("target"), "HEAD"); got != msg {
			t.Errorf("warp commit subject = %q; want verbatim %q", got, msg)
		}
		// The weft is not a merge participant, so the message option never reaches it — the weft HEAD
		// stays exactly where it was.
		if got := fabricengine.CurrentSHAForTest(t, h.PairWeftSibling("target")); got != weftBefore {
			t.Errorf("target weft HEAD = %q; want unchanged %q", got, weftBefore)
		}
	})
}

// TestMerge_DirtyTargetHalts covers Merge's dirty-target guard: a dirty warp halts before mutating
// anything (SHA unchanged, no record) with the fixed "worktree dirty" reason. A dirty weft no longer
// halts anything — the weft is not a merge participant, so its dirtiness cannot affect a warp-only
// merge's correctness — and Merge proceeds to land the warp-side merge exactly as it would against a
// clean weft, leaving the dirty weft file exactly as the test left it (uncommitted, untouched, HEAD
// unmoved).
func TestMerge_DirtyTargetHalts(t *testing.T) {
	h1, target1, commitOnSourceWarp1, commitOnSourceWeft1 := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp1, commitOnSourceWeft1)
	targetWarpPath1 := h1.PairWarpWorktree("target")
	commitOnCurrentBranch(t, targetWarpPath1, "tracked.txt", "v1\n", "target: seed tracked")
	warpBefore1 := fabricengine.CurrentSHAForTest(t, targetWarpPath1)
	weftBefore1 := fabricengine.CurrentSHAForTest(t, h1.PairWeftSibling("target"))
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

	if got := fabricengine.CurrentSHAForTest(t, targetWarpPath1); got != warpBefore1 {
		t.Errorf("[warp dirty] target warp HEAD changed to %q; want unchanged %q", got, warpBefore1)
	}
	if got := fabricengine.CurrentSHAForTest(t, h1.PairWeftSibling("target")); got != weftBefore1 {
		t.Errorf("[warp dirty] target weft HEAD changed to %q; want unchanged %q", got, weftBefore1)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(target1); err != nil || exists {
		t.Errorf("[warp dirty] MergeRecordExistsForTest() = (%v, %v); want (false, nil)", exists, err)
	}

	h2, target2, commitOnSourceWarp2, commitOnSourceWeft2 := newMergeTargetFixture(t, ".")
	targetWarpPath2 := h2.PairWarpWorktree("target")
	// A target-side warp commit, so merging "feature" is a genuine (non-fast-forward) merge that
	// fabricates a commit — the shape needed to assert Committed true below, mirroring
	// TestMerge_CleanNonSquash's own reason for the same seed commit.
	commitOnCurrentBranch(t, targetWarpPath2, "warp-progress.txt", "target progress\n", "target: progress")
	seedSourceAndTarget(t, commitOnSourceWarp2, commitOnSourceWeft2)
	targetWeftPath2 := h2.PairWeftSibling("target")
	commitOnCurrentBranch(t, targetWeftPath2, "_lyx/tracked.txt", "v1\n", "target: seed tracked weft")
	warpBefore2 := fabricengine.CurrentSHAForTest(t, targetWarpPath2)
	weftBefore2 := fabricengine.CurrentSHAForTest(t, targetWeftPath2)
	if err := os.WriteFile(filepath.Join(targetWeftPath2, "_lyx", "tracked.txt"), []byte("v2 (uncommitted)\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(_lyx/tracked.txt): %v", err)
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
	if got := gitkit.GitStatusPorcelain(t, targetWeftPath2); !strings.Contains(got, "_lyx/tracked.txt") {
		t.Errorf("[weft dirty] weft git status --porcelain = %q; want it to still mention the uncommitted _lyx/tracked.txt", got)
	}
}

// TestMerge_StaleTargetSyncsBeforeMerging covers a target behind its own upstream: Merge's pre-merge
// sync step fetches and fast-forwards the warp side (KindRepoAdvanced recorded) before merging,
// landing both the remote advance and the merge's own content. The weft side is not synced — it is
// not a merge participant — so a weft-side remote advance is left unfetched.
func TestMerge_StaleTargetSyncsBeforeMerging(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	advanceRemoteBranch(t, h.WarpBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")

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

	if _, err := os.Stat(filepath.Join(h.PairWarpWorktree("target"), "remote-warp.txt")); err != nil {
		t.Errorf("remote-warp.txt missing after Merge: %v; want it fetched and fast-forwarded before merging", err)
	}
}

// TestMerge_DivergedTargetRefuses covers a genuinely diverged target — local commits past an already
// fetched, moved-elsewhere upstream — refusing with *MergeGuardError carrying "branch not synced to
// upstream" and mutating nothing.
func TestMerge_DivergedTargetRefuses(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	targetWarpPath := h.PairWarpWorktree("target")
	advanceRemoteBranch(t, h.WarpBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")
	// Fetch so the guard's own read-only @{u} check sees the remote advance (the guard stage never
	// fetches on its own), then diverge locally so neither direction is an ancestor of the other.
	gitkit.MustRun(t, targetWarpPath, "git", "fetch", "-q", "origin")
	commitOnCurrentBranch(t, targetWarpPath, "local-warp.txt", "local content\n", "target: local progress")

	warpBefore := fabricengine.CurrentSHAForTest(t, targetWarpPath)
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PairWeftSibling("target"))

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
	if got := fabricengine.CurrentSHAForTest(t, h.PairWeftSibling("target")); got != weftBefore {
		t.Errorf("target weft HEAD changed to %q; want unchanged %q", got, weftBefore)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("Merge(feature).Mutated() = %v; want empty (the guard stage is strictly read-only)", res.Mutated().Entries())
	}
}

// TestMerge_NoUpstreamSidePassesVacuously covers a target side with no configured upstream: the sync
// guard passes vacuously for that side, and the result is indistinguishable (same top-level shape,
// same mutation-kind sequence) from the with-upstream clean case.
func TestMerge_NoUpstreamSidePassesVacuously(t *testing.T) {
	h1, target1, commitOnSourceWarp1, commitOnSourceWeft1 := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp1, commitOnSourceWeft1)
	resWithUpstream, err := target1.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) [with upstream] error = %v", err)
	}

	h2, target2, commitOnSourceWarp2, commitOnSourceWeft2 := newMergeTargetFixture(t, ".")
	gitkit.MustRun(t, h2.PairWeftSibling("target"), "git", "branch", "--unset-upstream")
	seedSourceAndTarget(t, commitOnSourceWarp2, commitOnSourceWeft2)
	resNoUpstream, err := target2.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) [weft no upstream] error = %v", err)
	}

	_ = h1
	if resWithUpstream.Committed != resNoUpstream.Committed {
		t.Errorf("Committed differs: with-upstream = %v, no-upstream = %v", resWithUpstream.Committed, resNoUpstream.Committed)
	}
	if resWithUpstream.AlreadyUpToDate != resNoUpstream.AlreadyUpToDate {
		t.Errorf("AlreadyUpToDate differs: with-upstream = %v, no-upstream = %v", resWithUpstream.AlreadyUpToDate, resNoUpstream.AlreadyUpToDate)
	}
	if len(resWithUpstream.Conflicts) != len(resNoUpstream.Conflicts) {
		t.Errorf("Conflicts length differs: with-upstream = %d, no-upstream = %d", len(resWithUpstream.Conflicts), len(resNoUpstream.Conflicts))
	}

	kindsWith, kindsNo := mutationKinds(resWithUpstream), mutationKinds(resNoUpstream)
	if len(kindsWith) != len(kindsNo) {
		t.Fatalf("mutation-kind count differs: with-upstream = %v, no-upstream = %v", kindsWith, kindsNo)
	}
	for i := range kindsWith {
		if kindsWith[i] != kindsNo[i] {
			t.Errorf("mutation kind[%d]: with-upstream = %q, no-upstream = %q; want identical sequences", i, kindsWith[i], kindsNo[i])
		}
	}
}

// TestMerge_ConflictSelfAborts covers a warp-side merge that would conflict: it self-aborts,
// restoring the target pair's warp SHA and worktree exactly, returning *ErrMergeInRequired with
// Source set and leaving no record.
// This test used to also cover a weft-side conflict, asserting the returned error was byte-identical
// regardless of which side conflicted. That sub-scenario has no warp-side analogue and is deleted
// here: Merge no longer merges the weft side at all, so a weft-side conflict is not a shape Merge can
// produce any more — see the merge-drops-weft task.
func TestMerge_ConflictSelfAborts(t *testing.T) {
	hWarp, targetWarp, commitOnSourceWarp1, commitOnSourceWeft1 := newMergeTargetFixture(t, ".")
	commitOnCurrentBranch(t, hWarp.PairWarpWorktree("target"), "conflict.txt", "target content\n", "target: seed conflict.txt")
	commitOnSourceWarp1("feature", "conflict.txt", "feature content\n", "feature: diverge conflict.txt")
	commitOnSourceWeft1("feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")

	warpBeforeW := fabricengine.CurrentSHAForTest(t, hWarp.PairWarpWorktree("target"))
	weftBeforeW := fabricengine.CurrentSHAForTest(t, hWarp.PairWeftSibling("target"))

	_, errWarpConflict := targetWarp.Merge("feature", fabricengine.MergeOptions{})
	var reqWarp *fabricengine.ErrMergeInRequired
	if !errors.As(errWarpConflict, &reqWarp) {
		t.Fatalf("Merge(feature) [warp conflict] error = %v (%T); want *ErrMergeInRequired", errWarpConflict, errWarpConflict)
	}
	if reqWarp.Source != "feature" {
		t.Errorf("[warp conflict] ErrMergeInRequired.Source = %q; want %q", reqWarp.Source, "feature")
	}
	if got := fabricengine.CurrentSHAForTest(t, hWarp.PairWarpWorktree("target")); got != warpBeforeW {
		t.Errorf("[warp conflict] target warp HEAD = %q; want restored pre-merge SHA %q", got, warpBeforeW)
	}
	if got := fabricengine.CurrentSHAForTest(t, hWarp.PairWeftSibling("target")); got != weftBeforeW {
		t.Errorf("[warp conflict] target weft HEAD = %q; want restored pre-merge SHA %q", got, weftBeforeW)
	}
	if out := gitkit.GitStatusPorcelain(t, hWarp.PairWarpWorktree("target")); out != "" {
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
	h, target, _, _ := newMergeTargetFixture(t, ".")

	branchAtCurrentHEAD(t, h.PrimeWorktree(), "feature")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

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

// TestMerge_CrashRecovery covers Merge's crash-recovery role: a record manufactured over a staged
// target reports MergeInProgress true on a fresh handle, MergeAbort restores the target pair exactly,
// and MergeContinue concludes a crashed-after-clean-staging merge.
func TestMerge_CrashRecovery(t *testing.T) {
	t.Run("MergeInProgress true and MergeAbort restores", func(t *testing.T) {
		h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
		targetWarpPath, targetWeftPath := h.PairWarpWorktree("target"), h.PairWeftSibling("target")
		commitOnCurrentBranch(t, targetWarpPath, "target-progress.txt", "v1\n", "target: progress")
		commitOnCurrentBranch(t, targetWeftPath, "_lyx/target-progress.txt", "v1\n", "target: progress weft")
		seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

		warpStart := fabricengine.CurrentSHAForTest(t, targetWarpPath)
		weftStart := fabricengine.CurrentSHAForTest(t, targetWeftPath)

		warpRepo := fabricengine.WarpForTest(target)
		weftRepo := fabricengine.WeftForTest(target)
		warpSourceSHA, err := warpRepo.ResolveSHA("feature")
		if err != nil {
			t.Fatalf("ResolveSHA(feature) error = %v", err)
		}
		weftSourceSHA, err := weftRepo.ResolveSHA("feature-weft")
		if err != nil {
			t.Fatalf("ResolveSHA(feature-weft) error = %v", err)
		}
		if _, err := warpRepo.MergeStart(warpSourceSHA, false); err != nil {
			t.Fatalf("warp MergeStart(%s) error = %v", warpSourceSHA, err)
		}
		if _, err := weftRepo.MergeStart(weftSourceSHA, false); err != nil {
			t.Fatalf("weft MergeStart(%s) error = %v", weftSourceSHA, err)
		}

		if err := fabricengine.SaveMergeStateForTest(target, fabricengine.MergeStateForTest{
			Verb:        "merge",
			Source:      "feature",
			Squash:      false,
			WarpStart:   warpStart,
			WeftStart:   weftStart,
			WarpOutcome: "staged",
			WeftOutcome: "staged",
			StartedAt:   time.Now(),
		}); err != nil {
			t.Fatalf("SaveMergeStateForTest() error = %v", err)
		}

		fresh := openFreshFabric(t, targetWarpPath)
		if inProgress, err := fresh.MergeInProgress(); err != nil || !inProgress {
			t.Errorf("fresh handle MergeInProgress() = (%v, %v); want (true, nil)", inProgress, err)
		}

		if _, err := fresh.MergeAbort(); err != nil {
			t.Fatalf("fresh handle MergeAbort() error = %v", err)
		}

		if got := fabricengine.CurrentSHAForTest(t, targetWarpPath); got != warpStart {
			t.Errorf("target warp HEAD after fresh-handle MergeAbort = %q; want %q", got, warpStart)
		}
		if got := fabricengine.CurrentSHAForTest(t, targetWeftPath); got != weftStart {
			t.Errorf("target weft HEAD after fresh-handle MergeAbort = %q; want %q", got, weftStart)
		}
		if inProgress, err := fresh.MergeInProgress(); err != nil || inProgress {
			t.Errorf("fresh handle MergeInProgress() after MergeAbort = (%v, %v); want (false, nil)", inProgress, err)
		}
	})

	t.Run("MergeContinue concludes a crashed-after-clean-staging merge", func(t *testing.T) {
		h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
		targetWarpPath, targetWeftPath := h.PairWarpWorktree("target"), h.PairWeftSibling("target")
		commitOnCurrentBranch(t, targetWarpPath, "target-progress.txt", "v1\n", "target: progress")
		commitOnCurrentBranch(t, targetWeftPath, "_lyx/target-progress.txt", "v1\n", "target: progress weft")
		seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

		warpStart := fabricengine.CurrentSHAForTest(t, targetWarpPath)
		weftStart := fabricengine.CurrentSHAForTest(t, targetWeftPath)

		warpRepo := fabricengine.WarpForTest(target)
		weftRepo := fabricengine.WeftForTest(target)
		warpSourceSHA, err := warpRepo.ResolveSHA("feature")
		if err != nil {
			t.Fatalf("ResolveSHA(feature) error = %v", err)
		}
		weftSourceSHA, err := weftRepo.ResolveSHA("feature-weft")
		if err != nil {
			t.Fatalf("ResolveSHA(feature-weft) error = %v", err)
		}
		if _, err := warpRepo.MergeStart(warpSourceSHA, false); err != nil {
			t.Fatalf("warp MergeStart(%s) error = %v", warpSourceSHA, err)
		}
		if _, err := weftRepo.MergeStart(weftSourceSHA, false); err != nil {
			t.Fatalf("weft MergeStart(%s) error = %v", weftSourceSHA, err)
		}

		if err := fabricengine.SaveMergeStateForTest(target, fabricengine.MergeStateForTest{
			Verb:        "merge",
			Source:      "feature",
			Squash:      false,
			WarpStart:   warpStart,
			WeftStart:   weftStart,
			WarpOutcome: "staged",
			WeftOutcome: "staged",
			StartedAt:   time.Now(),
		}); err != nil {
			t.Fatalf("SaveMergeStateForTest() error = %v", err)
		}

		fresh := openFreshFabric(t, targetWarpPath)
		res, err := fresh.MergeContinue("")
		if err != nil {
			t.Fatalf("fresh handle MergeContinue() error = %v", err)
		}
		if !res.Committed {
			t.Errorf("fresh handle MergeContinue().Committed = false; want true")
		}
		if exists, err := fabricengine.MergeRecordExistsForTest(target); err != nil || exists {
			t.Errorf("MergeRecordExistsForTest() after recovery = (%v, %v); want (false, nil)", exists, err)
		}
	})
}

// TestMerge_PreMergeSyncRunsInsideTheWriteLock pins the one mutation Merge performs before its merge
// record exists: the pre-merge sync step.
// The sync fast-forwards the warp checkout, and it runs at a point where no merge record has been
// written yet — so mergeBlocksMutation reports false and the sibling weft-mutating verbs
// (Commit/Pull/Checkout/Remove) do not refuse. The weft write lock is therefore the only thing that
// can serialize it against a concurrent Commit writing the same weft index.
// The assertion is behavioural rather than structural: with the lock externally held, Merge must not
// have advanced the warp side by the time a sibling would have finished its own write, and must
// complete once the lock is released.
func TestMerge_PreMergeSyncRunsInsideTheWriteLock(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	advanceRemoteBranch(t, h.WarpBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")

	syncedWarpFile := filepath.Join(h.PairWarpWorktree("target"), "remote-warp.txt")
	if _, err := os.Stat(syncedWarpFile); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) before Merge = %v; want not-exist — the fixture must be genuinely stale for the sync step to have anything to do", syncedWarpFile, err)
	}

	lockPath := fabricengine.WeftWriteLockPathForTest(t, target)
	externalLock, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%q) error = %v", lockPath, err)
	}

	done := make(chan error, 1)
	go func() {
		_, mergeErr := target.Merge("feature", fabricengine.MergeOptions{})
		done <- mergeErr
	}()

	select {
	case <-done:
		_ = externalLock.Release()
		t.Fatal("Merge() completed while the weft write lock was externally held; want it to block before its pre-merge sync step")
	case <-time.After(150 * time.Millisecond):
		// Still blocked, as expected.
	}
	if _, err := os.Stat(syncedWarpFile); !os.IsNotExist(err) {
		_ = externalLock.Release()
		t.Fatalf("Stat(%s) while the lock was held = %v; want not-exist — the pre-merge sync mutated the warp checkout outside the lock", syncedWarpFile, err)
	}

	if err := externalLock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	select {
	case mergeErr := <-done:
		if mergeErr != nil {
			t.Fatalf("Merge(feature) error = %v", mergeErr)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Merge() did not complete after the external lock was released")
	}

	if _, err := os.Stat(syncedWarpFile); err != nil {
		t.Errorf("Stat(%s) after Merge = %v; want the sync step to have landed it once the lock was free", syncedWarpFile, err)
	}
}

// gitIsAncestor reports whether ancestor is an ancestor of descendant in dir, via
// `git merge-base --is-ancestor`. It is the independent read the sync-guard tests assert their
// fixture's ancestry with, so a fixture that silently stopped producing the shape under test fails
// on its own precondition instead of passing vacuously.
func gitIsAncestor(t *testing.T, dir, ancestor, descendant string) bool {
	t.Helper()

	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("git merge-base --is-ancestor %s %s in %s: %v", ancestor, descendant, dir, err)
		}
		return false
	}
	return true
}

// gitRevParse resolves rev to a SHA in dir.
func gitRevParse(t *testing.T, dir, rev string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", rev)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s in %s: %v", rev, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// divergeSideWithoutFetching creates the unfetched-divergence shape on one side of the target pair:
// someone else advances the side's own upstream branch on the bare remote, and the side then makes a
// local commit WITHOUT fetching, so its remote-tracking ref still names a commit that is an ancestor
// of its own HEAD.
// It asserts that shape with t.Fatal rather than assuming it, because the whole point of the tests
// below is that the pre-lock guard cannot see this divergence: a fixture that accidentally fetched
// would classify as a plain divergence, take the pre-lock guard's own path, and prove nothing about
// the post-fetch arm.
func divergeSideWithoutFetching(t *testing.T, worktree, bareRemote, branch, remoteFile, localFile string) {
	t.Helper()

	advanceRemoteBranch(t, bareRemote, branch, remoteFile, "remote content\n", "origin: advance "+branch)
	commitOnCurrentBranch(t, worktree, localFile, "local content\n", "local progress on "+branch)

	head := gitRevParse(t, worktree, "HEAD")
	tracking := gitRevParse(t, worktree, "origin/"+branch)
	if tracking == head {
		t.Fatalf("origin/%s in %s equals HEAD (%s); want a stale remote-tracking ref — this fixture must NOT have fetched", branch, worktree, head)
	}
	if !gitIsAncestor(t, worktree, tracking, head) {
		t.Fatalf("origin/%s (%s) in %s is not an ancestor of HEAD (%s); want the stale-tracking shape where the pre-lock guard classifies this side as merely ahead and passes", branch, tracking, worktree, head)
	}
	if gitIsAncestor(t, worktree, head, gitRevParse(t, bareRemote, branch)) {
		t.Fatalf("HEAD in %s is an ancestor of the real %s tip; want a genuine divergence", worktree, branch)
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
	if got := fabricengine.CurrentSHAForTest(t, h.PairWarpWorktree("target")); got != warpBefore {
		t.Errorf("target warp HEAD = %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PairWeftSibling("target")); got != weftBefore {
		t.Errorf("target weft HEAD = %q; want unchanged %q", got, weftBefore)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(openFreshFabric(t, h.PairWarpWorktree("target"))); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after the refusal = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMerge_UnfetchedDivergedTargetRefuses covers the divergence Merge's own pre-lock guard stage is
// structurally too early to see, on the warp side.
// syncedToUpstreamReason resolves @{u} before anything in the call has fetched, so a divergence
// created by someone else's push that this checkout has not fetched yet reads as "merely ahead" and
// the guard passes. Merge then fetches twice (resolveMergeSources, then the pre-merge sync step) and
// used to merge straight over the now-visible divergence, returning ok with committed:true against a
// target that `rev-list --left-right --count HEAD...@{u}` reports as genuinely diverged.
// The post-fetch arm in syncSideBeforeMerge is what closes it, and this test is the only thing that
// reaches that arm: TestMerge_DivergedTargetRefuses hand-fetches first and therefore exercises the
// pre-lock guard instead.
func TestMerge_UnfetchedDivergedTargetRefuses(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	divergeSideWithoutFetching(t, h.PairWarpWorktree("target"), h.WarpBare, "target", "remote-warp.txt", "local-warp.txt")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PairWarpWorktree("target"))
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PairWeftSibling("target"))

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	assertMergeRefusedAsNotSynced(t, h, res, err, warpBefore, weftBefore)
}

// TestMerge_UnfetchedDivergedWeftDoesNotRefuse used to be TestMerge_UnfetchedDivergedTargetRefuses'
// weft-side twin, pinning syncSideBeforeMerge's post-fetch divergence check on the weft side. Batch
// merge-drops-weft deleted Merge's weft-side sync call entirely (Merge no longer synchronizes or
// merges the weft side at all), so that post-fetch weft arm no longer existed to pin, and this test
// was deleted rather than weakened, leaving only the still-present syncedToUpstreamReason guard's
// weft half (TestMerge_FetchedDivergedWeftRefuses) as coverage of the weft's remaining power to
// block a merge.
// This batch (weft-guards-drop) removes that remaining power too: syncedToUpstreamReason now
// evaluates the warp side alone. There is no longer a weft-side divergence check anywhere in Merge's
// guard stage, fetched or unfetched, so this name is not resurrected as a separate test — the single
// TestMerge_FetchedDivergedWeftDoesNotRefuse test below covers the (now unified) non-refusing
// behavior for both the fetched and unfetched shapes, since neither reaches a weft-aware guard arm
// anymore.

// TestMerge_FetchedDivergedWeftDoesNotRefuse used to be TestMerge_FetchedDivergedWeftRefuses,
// pinning syncedToUpstreamReason's WEFT half: with the divergence already fetched before the call,
// the pre-lock guard used to be what refused.
// This batch drops that weft half. A weft diverged from its own upstream can no longer refuse a
// merge the warp alone completes, in either the pre-lock guard or the post-fetch sync layer (the
// latter no longer even touches the weft) — per-transition status pushes warn and continue on a
// rejected push, which makes a locally-diverged weft a routine, expected state.
//
// The fixture still pairs that diverged weft with a warp side deliberately left BEHIND its own
// upstream: that pairing is what gives the test its teeth rather than being scene-setting. Merge's
// own pre-merge sync step is expected to fast-forward the behind warp side and land the merge, so
// the assertions below pin exactly that: no error, Committed true, and the warp HEAD moved off its
// pre-merge SHA while the weft HEAD — never touched by Merge — stays exactly where it started.
func TestMerge_FetchedDivergedWeftDoesNotRefuse(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	targetWarpPath := h.PairWarpWorktree("target")
	targetWeftPath := h.PairWeftSibling("target")

	advanceRemoteBranch(t, h.WeftBare, "target-weft", "remote-weft.txt", "remote content\n", "origin: advance target-weft")
	gitkit.MustRun(t, targetWeftPath, "git", "fetch", "-q", "origin")
	commitOnCurrentBranch(t, targetWeftPath, "local-weft.txt", "local content\n", "target-weft: local progress")

	// The warp side is left strictly behind its upstream: legal on its own (the sync step exists to
	// fast-forward exactly this).
	advanceRemoteBranch(t, h.WarpBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")

	// Precondition: the weft side is genuinely diverged with the divergence already visible to the
	// pre-lock guard — the shape that used to make only the weft half of that guard refuse.
	head := gitRevParse(t, targetWeftPath, "HEAD")
	tracking := gitRevParse(t, targetWeftPath, "origin/target-weft")
	if gitIsAncestor(t, targetWeftPath, head, tracking) || gitIsAncestor(t, targetWeftPath, tracking, head) {
		t.Fatalf("weft HEAD (%s) and origin/target-weft (%s) are ancestor-related; want a genuine divergence visible to the pre-lock guard", head, tracking)
	}

	warpBefore := fabricengine.CurrentSHAForTest(t, targetWarpPath)
	weftBefore := fabricengine.CurrentSHAForTest(t, targetWeftPath)

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v; want nil — a diverged weft must no longer refuse a merge the warp alone completes", err)
	}
	if !res.Committed {
		t.Errorf("Merge(feature).Committed = false; want true")
	}
	if got := fabricengine.CurrentSHAForTest(t, targetWarpPath); got == warpBefore {
		t.Errorf("target warp HEAD = %q; want it to have moved off %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, targetWeftPath); got != weftBefore {
		t.Errorf("target weft HEAD = %q; want unchanged %q — Merge never touches the weft", got, weftBefore)
	}
}

// TestMerge_FetchedBehindTargetIsSyncedNotRefused pins the behind-passes clause of
// sideNotSyncedToUpstream, which nothing reached before: TestMerge_StaleTargetSyncsBeforeMerging's
// target has NOT fetched, so at guard time its @{u} still equals its own HEAD and the predicate
// returns on the equality arm long before the behind arm.
// Here the advance is fetched first, so the guard genuinely sees HEAD strictly behind upstream and
// must classify it as synced — Merge's own pre-merge sync step is what advances it. Turning that
// clause into a refusal makes a merely-stale target unmergeable, which is the whole scenario the sync
// step exists for.
func TestMerge_FetchedBehindTargetIsSyncedNotRefused(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	targetWarpPath := h.PairWarpWorktree("target")
	advanceRemoteBranch(t, h.WarpBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")
	gitkit.MustRun(t, targetWarpPath, "git", "fetch", "-q", "origin")

	// Precondition: strictly behind — HEAD is an ancestor of the fetched upstream and not equal to it,
	// so the predicate must reach its behind arm rather than its equality or ahead arm.
	head := gitRevParse(t, targetWarpPath, "HEAD")
	tracking := gitRevParse(t, targetWarpPath, "origin/target")
	if head == tracking {
		t.Fatalf("HEAD and origin/target are both %s; want HEAD strictly behind a fetched upstream", head)
	}
	if !gitIsAncestor(t, targetWarpPath, head, tracking) {
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
