//go:build integration

// mergein_integration_test.go covers MergeIn's scenario matrix against a real hubforge pair: both
// sides clean, a warp-side conflict (the only side MergeIn can now conflict on), the already-up-to-date
// fast paths, worktree-resolved conflicts via MergeContinue, MergeAbort, and the never-squashes
// merge-commit shape.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// newMergePairFixture builds a real hubforge pair anchored at anchor and the *fabricengine.Fabric
// handle over its prime warp/weft worktrees (via lyxcwd.ResolveWorktree + fabricengine.Open), plus
// two closures for building divergent commits directly on the prime warp and weft repos
// (gitkit.MustRun): commitOnWarpBranch/commitOnWeftBranch each check out (creating if absent) the
// named branch off whatever is currently checked out, write filename with content, commit msg, and
// switch back — leaving the pair's own checkout on its original branch when the closure returns, so
// every scenario builds its divergence without disturbing the checkout MergeIn itself will run
// against.
// Exported within the test package (capitalized helpers alongside it) for reuse by batches 4-5.
func newMergePairFixture(t *testing.T, anchor string) (h *hubforge.Hub, f *fabricengine.Fabric, commitOnWarpBranch, commitOnWeftBranch func(branch, filename, content, msg string), commitOnWarpCurrent, commitOnWeftCurrent func(filename, content, msg string)) {
	t.Helper()

	h = hubforge.NewHub(t, anchor)
	l, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
	}
	f, err = fabricengine.Open(l)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	warpDir, weftDir := h.PrimeWorktree(), h.PrimeWeft()
	commitOnWarpBranch = func(branch, filename, content, msg string) {
		commitOnBranch(t, warpDir, branch, filename, content, msg)
	}
	commitOnWeftBranch = func(branch, filename, content, msg string) {
		commitOnBranch(t, weftDir, branch, filename, content, msg)
	}
	commitOnWarpCurrent = func(filename, content, msg string) {
		commitOnCurrentBranch(t, warpDir, filename, content, msg)
	}
	commitOnWeftCurrent = func(filename, content, msg string) {
		commitOnCurrentBranch(t, weftDir, filename, content, msg)
	}
	return h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent
}

// commitOnBranch checks out branch in dir — creating it off whatever is currently checked out when
// it does not exist yet — writes filename with content, commits msg, then switches back to whatever
// branch was checked out before, so building a divergent branch never leaves dir mid-switch.
func commitOnBranch(t *testing.T, dir, branch, filename, content, msg string) {
	t.Helper()

	current := currentBranchName(t, dir)
	if branchExistsLocally(t, dir, branch) {
		gitkit.MustRun(t, dir, "git", "checkout", "-q", branch)
	} else {
		gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", branch)
	}

	commitOnCurrentBranch(t, dir, filename, content, msg)

	gitkit.MustRun(t, dir, "git", "checkout", "-q", current)
}

// branchAtCurrentHEAD creates branch in dir pointing at whatever is currently checked out, without
// checking it out or adding any commit — the already-up-to-date fixture shape.
func branchAtCurrentHEAD(t *testing.T, dir, branch string) {
	t.Helper()
	gitkit.MustRun(t, dir, "git", "branch", branch)
}

// commitOnCurrentBranch writes filename with content in dir, stages it, and commits msg on whatever
// branch is currently checked out.
func commitOnCurrentBranch(t *testing.T, dir, filename, content, msg string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", filename, err)
	}
	gitkit.MustRun(t, dir, "git", "add", filename)
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", msg)
}

// currentBranchName (dir's currently checked-out branch name) is livestate_verbs_test.go's own
// helper, reused unqualified since both files share package fabricengine_test.

// branchExistsLocally reports whether branch already exists as a local ref in dir.
func branchExistsLocally(t *testing.T, dir, branch string) bool {
	t.Helper()

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// setupConflictingDivergence seeds filename on dir's current branch, branches off, diverges the
// branch's copy, then diverges the current branch's own copy again — so merging branch into the
// current branch conflicts on filename.
func setupConflictingDivergence(t *testing.T, dir, branch, filename string) {
	t.Helper()

	commitOnCurrentBranch(t, dir, filename, "seed content\n", "seed "+filename)
	commitOnBranch(t, dir, branch, filename, "branch content\n", "diverge "+filename+" on "+branch)
	commitOnCurrentBranch(t, dir, filename, "current content\n", "diverge "+filename+" on current")
}

// setupCleanFastForward creates branch off dir's current HEAD with one new-file commit — a
// fast-forward merge target, since the current branch never advances past the branch point.
func setupCleanFastForward(t *testing.T, dir, branch, filename string) {
	t.Helper()
	commitOnBranch(t, dir, branch, filename, "clean content\n", "clean "+filename+" on "+branch)
}

// setupCleanNonFastForward creates branch off dir's current HEAD with a new-file commit, then
// advances the current branch with its own unrelated commit — a genuine (non-fast-forward) merge
// target.
func setupCleanNonFastForward(t *testing.T, dir, branch, branchFile, currentFile string) {
	t.Helper()
	commitOnBranch(t, dir, branch, branchFile, "clean branch content\n", "clean "+branchFile+" on "+branch)
	commitOnCurrentBranch(t, dir, currentFile, "current progress\n", "progress current past "+branch)
}

// TestMergeIn_BothSidesClean covers the both-sides-clean scenario: Committed true, both sides
// concluded, correspondence recorded, the record deleted, and MergeInProgress false afterward.
// Both target branches carry a divergent commit of their own, so the merge genuinely needs a
// conclude-commit on each side rather than fast-forwarding. That divergence is load-bearing: without
// it neither side commits anything, and the Committed assertion below passes for the wrong reason —
// which is exactly how this test used to false-green while MergeIn hardcoded Committed true.
func TestMergeIn_BothSidesClean(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")

	commitOnWarpBranch("feature", "warp-feature.txt", "warp feature\n", "warp: add feature")
	commitOnWeftBranch("feature-weft", "weft-feature.txt", "weft feature\n", "weft: add feature")
	commitOnWarpCurrent("warp-target.txt", "warp target\n", "warp: diverge target")
	commitOnWeftCurrent("weft-target.txt", "weft target\n", "weft: diverge target")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if !res.Committed {
		t.Errorf("MergeIn(feature).Committed = false; want true")
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("MergeIn(feature).Conflicts = %v; want empty", res.Conflicts)
	}

	warpHEAD := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftSHA, err := f.WeftSHAForWarpSHA(warpHEAD)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA(%s) error = %v; want the merge's correspondence recorded", warpHEAD, err)
	}
	if weftSHA == "" {
		t.Error("WeftSHAForWarpSHA returned empty weft SHA")
	}

	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after clean MergeIn = (%v, %v); want (false, nil)", exists, err)
	}
	if inProgress, err := f.MergeInProgress(); err != nil || inProgress {
		t.Errorf("MergeInProgress() after clean MergeIn = (%v, %v); want (false, nil)", inProgress, err)
	}
}

// TestMergeIn_WarpConflicts covers a warp conflict — the only side MergeIn can now conflict on.
// This test used to also cover a weft-only conflict and assert the two produced byte-identical
// result shapes; the weft arm is deleted, with no replacement, since MergeIn no longer merges the
// weft side at all, so a weft-side conflict is not a shape it can produce any more — see the
// merge-drops-weft task.
func TestMergeIn_WarpConflicts(t *testing.T) {
	h, f, _, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	commitOnWeftBranch("feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) [warp conflict] error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) [warp conflict] Conflicts is empty; want at least one path")
	}
	if res.Committed {
		t.Error("MergeIn(feature) [warp conflict] Committed = true; want false")
	}
	if inProgress, err := f.MergeInProgress(); err != nil || !inProgress {
		t.Errorf("MergeInProgress() [warp conflict] = (%v, %v); want (true, nil)", inProgress, err)
	}
}

// TestMergeIn_BothSidesConflict used to cover both sides conflicting simultaneously, asserting one
// flat, lexically sorted, deduplicated list spanning both sides. That scenario has no warp-side
// analogue and is deleted: MergeIn no longer merges the weft side at all, so the weft half can never
// conflict, and "both sides conflict" is not a reachable shape any more — see the merge-drops-weft
// task. TestMergeIn_WarpConflicts covers the warp side's own conflict-reporting behavior.

// TestMergeIn_NonASCIIConflictPaths_ReportedRawNotQuotedNotUnmergeable pins the core.quotepath
// regression on the warp side: a conflict on a path outside git's default ASCII quoting set must
// surface as the raw worktree-relative path. Before ConflictedFiles passed `-z`, git handed back
// the C-quoted rendering (`"\303\244..."`, quotes included), so the reported path was a literal that
// exists nowhere in the worktree.
// This test used to also seed a weft-side non-ASCII conflict, since weftPathVisible's own prefix test
// was a second, independent place the quoted form could fail. That arm is deleted along with the
// weft's ability to conflict at all — see the merge-drops-weft task.
func TestMergeIn_NonASCIIConflictPaths_ReportedRawNotQuotedNotUnmergeable(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "ä-warp.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v; want nil — a non-ASCII in-tree conflict is mappable, never *ErrUnmergeableState", err)
	}
	want := []string{"ä-warp.txt"}
	if len(res.Conflicts) != len(want) || res.Conflicts[0] != want[0] {
		t.Errorf("MergeIn(feature).Conflicts = %q; want %q — raw path bytes, not git's C-quoted rendering", res.Conflicts, want)
	}
}

// TestMergeIn_OneSideFastForwardsOtherConflicts_AbortRestoresFastForwardedSide used to cover the B1
// case: the warp side fast-forwards while the weft side conflicts, and MergeAbort restores the
// fast-forwarded warp side to its recorded pre-merge SHA. That shape has no warp-only analogue and is
// deleted: with the weft no longer a merge participant, there is no "other side" left that can
// conflict while the warp side fast-forwards — a fast-forwarding warp side means the whole call
// concludes cleanly, never conflicted. MergeAbort's own restore-a-fast-forwarded-side behavior is
// covered structurally by TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded's fixtures and
// by mergestate_test.go's own outcome-classification tests — see the merge-drops-weft task.

// TestMergeIn_OneSideAlreadyUpToDate_OtherMerges covers the mixed already-up-to-date case: the
// merge concludes, no empty commit is fabricated on the no-op side, and correspondence pairs the
// new SHA with the unchanged one.
// The merging side fast-forwards (setupCleanFastForward), so no conclude-commit is fabricated there
// either and Committed is false — the pair advanced without any merge commit existing. AlreadyUpToDate
// is likewise false: one side did move.
func TestMergeIn_OneSideAlreadyUpToDate_OtherMerges(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	setupCleanFastForward(t, h.PrimeWorktree(), "feature", "warp-ff.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

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

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStartSHA {
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
		if e.Kind == fabricengine.KindMergeStaged && e.Target == hubRelForTest(t, f, h.PrimeWeft()) {
			t.Errorf("mutation record carries a merge_staged entry for the already-up-to-date weft side: %+v", e)
		}
		if e.Kind == fabricengine.KindMergeCommitted && e.Target == hubRelForTest(t, f, h.PrimeWeft()) {
			t.Errorf("mutation record carries a merge_committed entry for the already-up-to-date weft side: %+v", e)
		}
	}
}

// hubRelForTest mirrors mutation.go's hub-relative Target conversion for target, so a test can
// compare a raw path against a Mutation entry's own Target without duplicating fabricengine's
// unexported conversion.
func hubRelForTest(t *testing.T, f *fabricengine.Fabric, target string) string {
	t.Helper()
	rel, err := filepath.Rel(filepath.Dir(fabricengine.WarpPathForTest(f)), target)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	return filepath.ToSlash(rel)
}

// TestMergeIn_BothSidesAlreadyUpToDate covers the degenerate no-op: AlreadyUpToDate true, an empty
// mutation record, and no merge-state record written.
func TestMergeIn_BothSidesAlreadyUpToDate(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	branchAtCurrentHEAD(t, h.PrimeWorktree(), "feature")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if !res.AlreadyUpToDate {
		t.Errorf("MergeIn(feature).AlreadyUpToDate = false; want true")
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeIn(feature).Mutated().Len() = %d; want 0 (empty mutation record)", res.Mutated().Len())
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after already-up-to-date MergeIn = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeContinue_ResolvedConflictsConclude covers resolving a conflict in the worktree (edit +
// git add) and running MergeContinue: both sides conclude, correspondence is recorded, and the
// record is deleted.
func TestMergeContinue_ResolvedConflictsConclude(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) produced no conflicts; want the seeded warp conflict")
	}

	resolvedPath := filepath.Join(h.PrimeWorktree(), "conflict.txt")
	if err := os.WriteFile(resolvedPath, []byte("resolved content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict.txt): %v", err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "conflict.txt")

	continued, err := f.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue(\"\") error = %v", err)
	}
	if !continued.Committed {
		t.Errorf("MergeContinue(\"\").Committed = false; want true")
	}

	warpHEAD := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if _, err := f.WeftSHAForWarpSHA(warpHEAD); err != nil {
		t.Errorf("WeftSHAForWarpSHA(%s) error = %v; want correspondence recorded", warpHEAD, err)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after MergeContinue = (%v, %v); want (false, nil)", exists, err)
	}
}

// TestMergeContinue_UnresolvedConflictsRefuse covers MergeContinue's refusal while conflict markers
// remain: a *MergeGuardError whose sole reason is the fixed "unresolved conflicts remain".
func TestMergeContinue_UnresolvedConflictsRefuse(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

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

// TestMergeAbort_AfterConflict covers MergeAbort after a conflicted MergeIn: both sides land at
// their exact pre-merge SHAs, worktrees are clean, the record is deleted, and MergeInProgress
// reports false.
func TestMergeAbort_AfterConflict(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	// Captured after the divergence commits above land, since those are the actual pre-merge SHAs
	// MergeIn's own record captures and MergeAbort must restore — not the SHAs before this fixture's
	// own setup ran.
	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	if _, err := f.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStartSHA {
		t.Errorf("warp HEAD after MergeAbort = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStartSHA {
		t.Errorf("weft HEAD after MergeAbort = %q; want restored pre-merge SHA %q", got, weftStartSHA)
	}
	if out := gitkit.GitStatusPorcelain(t, h.PrimeWorktree()); out != "" {
		t.Errorf("warp git status --porcelain after MergeAbort = %q; want clean", out)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("MergeRecordExistsForTest() after MergeAbort = (%v, %v); want (false, nil)", exists, err)
	}
	if inProgress, err := f.MergeInProgress(); err != nil || inProgress {
		t.Errorf("MergeInProgress() after MergeAbort = (%v, %v); want (false, nil)", inProgress, err)
	}
}

// TestMergeIn_NeverSquashes covers that a clean, non-fast-forward MergeIn lands a real merge commit
// on warp: two parents.
func TestMergeIn_NeverSquashes(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupCleanNonFastForward(t, h.PrimeWorktree(), "feature", "branch-file.txt", "current-file.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("MergeIn(feature).Committed = false; want true")
	}

	warpHEAD := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	cmd := exec.Command("git", "log", "-1", "--format=%P", warpHEAD)
	cmd.Dir = h.PrimeWorktree()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%P %s: %v", warpHEAD, err)
	}
	parents := strings.Fields(strings.TrimSpace(string(out)))
	if len(parents) != 2 {
		t.Errorf("warp HEAD %s has %d parents (%v); want exactly 2 (a real merge commit, never a squash)", warpHEAD, len(parents), parents)
	}
}
