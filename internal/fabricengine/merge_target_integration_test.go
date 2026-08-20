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
// parents on both sides, plain `git merge` semantics preserved.
func TestMerge_CleanNonSquash(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")

	targetWarpPath, targetWeftPath := h.PairWarpWorktree("target"), h.PairWeftSibling("target")
	// A target-side commit on each checkout, so merging "feature"/"feature-weft" is a genuine
	// (non-fast-forward) merge on both sides rather than a plain pointer advance.
	commitOnCurrentBranch(t, targetWarpPath, "target-progress.txt", "target progress\n", "target: progress")
	commitOnCurrentBranch(t, targetWeftPath, "_lyx/target-progress.txt", "target progress\n", "target: progress weft")

	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	res, err := target.Merge("feature", fabricengine.MergeOptions{})
	if err != nil {
		t.Fatalf("Merge(feature) error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("Merge(feature).Committed = false; want true")
	}

	warpHEAD := fabricengine.CurrentSHAForTest(t, targetWarpPath)
	weftHEAD := fabricengine.CurrentSHAForTest(t, targetWeftPath)
	if got := gitParentCount(t, targetWarpPath, warpHEAD); got != 2 {
		t.Errorf("target warp HEAD %s has %d parents; want exactly 2 (a real merge commit)", warpHEAD, got)
	}
	if got := gitParentCount(t, targetWeftPath, weftHEAD); got != 2 {
		t.Errorf("target weft HEAD %s has %d parents; want exactly 2 (a real merge commit)", weftHEAD, got)
	}
}

// TestMerge_MessagePrecedence covers Merge's Message option: empty uses git's own prepared message,
// set is used verbatim on both sides.
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

	t.Run("set is used verbatim on both sides", func(t *testing.T) {
		h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
		seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

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
		if got := gitCommitSubject(t, h.PairWeftSibling("target"), "HEAD"); got != msg {
			t.Errorf("weft commit subject = %q; want verbatim %q", got, msg)
		}
	})
}

// TestMerge_DirtyTargetHalts covers Merge's dirty-target guard: it halts before mutating anything on
// either side (SHAs unchanged, no record), and a dirty-warp-only pair and a dirty-weft-only pair both
// produce byte-identical *MergeGuardError values.
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

	h2, target2, commitOnSourceWarp2, commitOnSourceWeft2 := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp2, commitOnSourceWeft2)
	targetWeftPath2 := h2.PairWeftSibling("target")
	commitOnCurrentBranch(t, targetWeftPath2, "_lyx/tracked.txt", "v1\n", "target: seed tracked weft")
	warpBefore2 := fabricengine.CurrentSHAForTest(t, h2.PairWarpWorktree("target"))
	weftBefore2 := fabricengine.CurrentSHAForTest(t, targetWeftPath2)
	if err := os.WriteFile(filepath.Join(targetWeftPath2, "_lyx", "tracked.txt"), []byte("v2 (uncommitted)\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(_lyx/tracked.txt): %v", err)
	}
	_, errWeftDirty := target2.Merge("feature", fabricengine.MergeOptions{})

	var guardErrWarp, guardErrWeft *fabricengine.MergeGuardError
	if !errors.As(errWarpDirty, &guardErrWarp) {
		t.Fatalf("Merge(feature) [warp dirty] error = %v (%T); want *MergeGuardError", errWarpDirty, errWarpDirty)
	}
	if !errors.As(errWeftDirty, &guardErrWeft) {
		t.Fatalf("Merge(feature) [weft dirty] error = %v (%T); want *MergeGuardError", errWeftDirty, errWeftDirty)
	}
	if len(guardErrWarp.Reasons) != 1 || guardErrWarp.Reasons[0] != "worktree dirty" {
		t.Errorf("warp-dirty guard reasons = %v; want exactly [\"worktree dirty\"]", guardErrWarp.Reasons)
	}
	if errWarpDirty.Error() != errWeftDirty.Error() {
		t.Errorf("error values diverge by side: warp-dirty = %q, weft-dirty = %q; want byte-identical", errWarpDirty.Error(), errWeftDirty.Error())
	}

	if got := fabricengine.CurrentSHAForTest(t, targetWarpPath1); got != warpBefore1 {
		t.Errorf("[warp dirty] target warp HEAD changed to %q; want unchanged %q", got, warpBefore1)
	}
	if got := fabricengine.CurrentSHAForTest(t, h1.PairWeftSibling("target")); got != weftBefore1 {
		t.Errorf("[warp dirty] target weft HEAD changed to %q; want unchanged %q", got, weftBefore1)
	}
	if got := fabricengine.CurrentSHAForTest(t, h2.PairWarpWorktree("target")); got != warpBefore2 {
		t.Errorf("[weft dirty] target warp HEAD changed to %q; want unchanged %q", got, warpBefore2)
	}
	if got := fabricengine.CurrentSHAForTest(t, targetWeftPath2); got != weftBefore2 {
		t.Errorf("[weft dirty] target weft HEAD changed to %q; want unchanged %q", got, weftBefore2)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(target1); err != nil || exists {
		t.Errorf("[warp dirty] MergeRecordExistsForTest() = (%v, %v); want (false, nil)", exists, err)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(target2); err != nil || exists {
		t.Errorf("[weft dirty] MergeRecordExistsForTest() = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMerge_StaleTargetSyncsBeforeMerging covers a target behind its own upstream: Merge's pre-merge
// sync step fetches and fast-forwards it (KindRepoAdvanced recorded) before merging, landing both the
// remote advance and the merge's own content.
func TestMerge_StaleTargetSyncsBeforeMerging(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	advanceRemoteBranch(t, h.WarpBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")
	advanceRemoteBranch(t, h.WeftBare, "target-weft", "remote-weft.txt", "remote content\n", "origin: advance target-weft")

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
	if _, err := os.Stat(filepath.Join(h.PairWeftSibling("target"), "remote-weft.txt")); err != nil {
		t.Errorf("remote-weft.txt missing after Merge: %v; want it fetched and fast-forwarded before merging", err)
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

// TestMerge_ConflictSelfAborts covers a merge that would conflict: it self-aborts, restoring the
// target pair's SHAs and worktrees exactly, returning *ErrMergeInRequired with Source set and leaving
// no record — and the error value is identical whether the warp or the weft side would have
// conflicted.
func TestMerge_ConflictSelfAborts(t *testing.T) {
	// Warp conflicts, weft clean.
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

	// Weft conflicts, warp clean.
	hWeft, targetWeft, commitOnSourceWarp2, commitOnSourceWeft2 := newMergeTargetFixture(t, ".")
	commitOnSourceWarp2("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	commitOnCurrentBranch(t, hWeft.PairWeftSibling("target"), "_lyx/conflict.txt", "target content\n", "target: seed conflict.txt")
	commitOnSourceWeft2("feature-weft", "_lyx/conflict.txt", "feature content\n", "feature: diverge conflict.txt")

	warpBeforeF := fabricengine.CurrentSHAForTest(t, hWeft.PairWarpWorktree("target"))
	weftBeforeF := fabricengine.CurrentSHAForTest(t, hWeft.PairWeftSibling("target"))

	_, errWeftConflict := targetWeft.Merge("feature", fabricengine.MergeOptions{})
	var reqWeft *fabricengine.ErrMergeInRequired
	if !errors.As(errWeftConflict, &reqWeft) {
		t.Fatalf("Merge(feature) [weft conflict] error = %v (%T); want *ErrMergeInRequired", errWeftConflict, errWeftConflict)
	}
	if reqWeft.Source != "feature" {
		t.Errorf("[weft conflict] ErrMergeInRequired.Source = %q; want %q", reqWeft.Source, "feature")
	}
	if got := fabricengine.CurrentSHAForTest(t, hWeft.PairWarpWorktree("target")); got != warpBeforeF {
		t.Errorf("[weft conflict] target warp HEAD = %q; want restored pre-merge SHA %q", got, warpBeforeF)
	}
	if got := fabricengine.CurrentSHAForTest(t, hWeft.PairWeftSibling("target")); got != weftBeforeF {
		t.Errorf("[weft conflict] target weft HEAD = %q; want restored pre-merge SHA %q", got, weftBeforeF)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(targetWeft); err != nil || exists {
		t.Errorf("[weft conflict] MergeRecordExistsForTest() = (%v, %v); want (false, nil)", exists, err)
	}
	if inProgress, err := targetWeft.MergeInProgress(); err != nil || inProgress {
		t.Errorf("[weft conflict] MergeInProgress() = (%v, %v); want (false, nil)", inProgress, err)
	}

	// The byte-identical-error assertion: the value never discloses which side conflicted.
	if errWarpConflict.Error() != errWeftConflict.Error() {
		t.Errorf("error values differ by side: warp-conflict = %q, weft-conflict = %q; want identical", errWarpConflict.Error(), errWeftConflict.Error())
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
// The sync fast-forwards BOTH checkouts, and it runs at a point where no merge record has been
// written yet — so mergeBlocksMutation reports false and the sibling weft-mutating verbs
// (Commit/Pull/Checkout/Remove) do not refuse. The weft write lock is therefore the only thing that
// can serialize it, and acquiring the lock after the sync left `git merge --ff-only` running in the
// weft checkout alongside a concurrent Commit writing that same index.
// The assertion is behavioural rather than structural: with the lock externally held, Merge must not
// have advanced either side by the time a sibling would have finished its own write, and must
// complete once the lock is released.
func TestMerge_PreMergeSyncRunsInsideTheWriteLock(t *testing.T) {
	h, target, commitOnSourceWarp, commitOnSourceWeft := newMergeTargetFixture(t, ".")
	seedSourceAndTarget(t, commitOnSourceWarp, commitOnSourceWeft)

	advanceRemoteBranch(t, h.WarpBare, "target", "remote-warp.txt", "remote content\n", "origin: advance target")
	advanceRemoteBranch(t, h.WeftBare, "target-weft", "remote-weft.txt", "remote content\n", "origin: advance target-weft")

	syncedWeftFile := filepath.Join(h.PairWeftSibling("target"), "remote-weft.txt")
	if _, err := os.Stat(syncedWeftFile); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) before Merge = %v; want not-exist — the fixture must be genuinely stale for the sync step to have anything to do", syncedWeftFile, err)
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
	if _, err := os.Stat(syncedWeftFile); !os.IsNotExist(err) {
		_ = externalLock.Release()
		t.Fatalf("Stat(%s) while the lock was held = %v; want not-exist — the pre-merge sync mutated the weft checkout outside the lock", syncedWeftFile, err)
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

	if _, err := os.Stat(syncedWeftFile); err != nil {
		t.Errorf("Stat(%s) after Merge = %v; want the sync step to have landed it once the lock was free", syncedWeftFile, err)
	}
}
