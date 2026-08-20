//go:build integration

// mergein_integration_test.go covers MergeIn's scenario matrix against a real hubforge pair: both
// sides clean, a conflict on either side alone (asserting the two are byte-identical in shape — the
// single most important test in the task), both sides conflicting, a fast-forward racing a conflict,
// the already-up-to-date fast paths, worktree-resolved conflicts via MergeContinue, MergeAbort, and
// the never-squashes merge-commit shape.

package fabricengine_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// mergeResultShape is the structural comparison mergein_integration_test.go's byte-identical-shape
// scenario compares two MergeResults through: everything except the conflicted paths' own spellings
// and the mutation record's own SHAs.
type mergeResultShape struct {
	AlreadyUpToDate bool
	ConflictsLen    int
	Committed       bool
	MutationKinds   []fabricengine.Kind
	MutationTargets []string
}

func shapeOf(res fabricengine.MergeResult) mergeResultShape {
	entries := res.Mutated().Entries()
	kinds := make([]fabricengine.Kind, len(entries))
	targets := make([]string, len(entries))
	for i, e := range entries {
		kinds[i] = e.Kind
		targets[i] = e.Target
	}
	return mergeResultShape{
		AlreadyUpToDate: res.AlreadyUpToDate,
		ConflictsLen:    len(res.Conflicts),
		Committed:       res.Committed,
		MutationKinds:   kinds,
		MutationTargets: targets,
	}
}

// jsonKeys returns the sorted top-level key set of v's JSON marshalling, for the shape assertion's
// structural (not byte-for-byte) comparison.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%+v): %v", v, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", data, err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// TestMergeIn_WarpConflictsWeftClean_And_WeftConflictsWarpClean covers a warp-only conflict, a
// weft-only conflict, and asserts the two are byte-identical in shape — the single most important
// test in the task.
func TestMergeIn_WarpConflictsWeftClean_And_WeftConflictsWarpClean(t *testing.T) {
	// Warp conflicts, weft clean.
	hWarpConflict, fWarpConflict, _, commitOnWeftBranch1, _, _ := newMergePairFixture(t, ".")
	setupConflictingDivergence(t, hWarpConflict.PrimeWorktree(), "feature", "conflict.txt")
	commitOnWeftBranch1("feature-weft", "clean-weft.txt", "clean\n", "weft: clean branch")

	resWarpConflict, errWarpConflict := fWarpConflict.MergeIn("feature")
	if errWarpConflict != nil {
		t.Fatalf("MergeIn(feature) [warp conflict] error = %v", errWarpConflict)
	}
	if len(resWarpConflict.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) [warp conflict] Conflicts is empty; want at least one path")
	}
	if resWarpConflict.Committed {
		t.Error("MergeIn(feature) [warp conflict] Committed = true; want false")
	}
	if inProgress, err := fWarpConflict.MergeInProgress(); err != nil || !inProgress {
		t.Errorf("MergeInProgress() [warp conflict] = (%v, %v); want (true, nil)", inProgress, err)
	}

	// Weft conflicts, warp clean.
	hWeftConflict, fWeftConflict, commitOnWarpBranch2, _, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch2("feature", "clean-warp.txt", "clean\n", "warp: clean branch")
	setupConflictingDivergence(t, hWeftConflict.PrimeWeft(), "feature-weft", "_lyx/conflict.txt")

	resWeftConflict, errWeftConflict := fWeftConflict.MergeIn("feature")
	if errWeftConflict != nil {
		t.Fatalf("MergeIn(feature) [weft conflict] error = %v", errWeftConflict)
	}
	if len(resWeftConflict.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) [weft conflict] Conflicts is empty; want at least one path")
	}
	if resWeftConflict.Committed {
		t.Error("MergeIn(feature) [weft conflict] Committed = true; want false")
	}
	if inProgress, err := fWeftConflict.MergeInProgress(); err != nil || !inProgress {
		t.Errorf("MergeInProgress() [weft conflict] = (%v, %v); want (true, nil)", inProgress, err)
	}

	// The byte-identical-shape assertion.
	if (errWarpConflict == nil) != (errWeftConflict == nil) {
		t.Fatalf("error values differ: warp-conflict err = %v, weft-conflict err = %v", errWarpConflict, errWeftConflict)
	}
	shapeWarp, shapeWeft := shapeOf(resWarpConflict), shapeOf(resWeftConflict)
	if shapeWarp.AlreadyUpToDate != shapeWeft.AlreadyUpToDate || shapeWarp.Committed != shapeWeft.Committed || shapeWarp.ConflictsLen != shapeWeft.ConflictsLen {
		t.Errorf("mismatched top-level shape: warp-conflict = %+v, weft-conflict = %+v", shapeWarp, shapeWeft)
	}
	if len(shapeWarp.MutationKinds) != len(shapeWeft.MutationKinds) {
		t.Fatalf("mismatched mutation-kind count: warp-conflict = %v, weft-conflict = %v", shapeWarp.MutationKinds, shapeWeft.MutationKinds)
	}
	for i := range shapeWarp.MutationKinds {
		if shapeWarp.MutationKinds[i] != shapeWeft.MutationKinds[i] {
			t.Errorf("mutation kind[%d]: warp-conflict = %q, weft-conflict = %q; want identical sequences", i, shapeWarp.MutationKinds[i], shapeWeft.MutationKinds[i])
		}
	}
	if len(shapeWarp.MutationTargets) != len(shapeWeft.MutationTargets) {
		t.Fatalf("mismatched mutation-target count: warp-conflict = %v, weft-conflict = %v", shapeWarp.MutationTargets, shapeWeft.MutationTargets)
	}
	for i := range shapeWarp.MutationTargets {
		if shapeWarp.MutationTargets[i] != shapeWeft.MutationTargets[i] {
			t.Errorf("mutation target[%d]: warp-conflict = %q, weft-conflict = %q; want identical sequences (same fixture layout both sides)", i, shapeWarp.MutationTargets[i], shapeWeft.MutationTargets[i])
		}
	}

	keysWarp, keysWeft := jsonKeys(t, resWarpConflict), jsonKeys(t, resWeftConflict)
	if len(keysWarp) != len(keysWeft) {
		t.Fatalf("JSON key sets differ: warp-conflict = %v, weft-conflict = %v", keysWarp, keysWeft)
	}
	for i := range keysWarp {
		if keysWarp[i] != keysWeft[i] {
			t.Errorf("JSON key[%d]: warp-conflict = %q, weft-conflict = %q", i, keysWarp[i], keysWeft[i])
		}
	}
}

// TestMergeIn_BothSidesConflict covers both sides conflicting: one flat, lexically sorted list
// containing paths from both sides, in the unified namespace, with no duplicates.
func TestMergeIn_BothSidesConflict(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "warp-conflict.txt")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "_lyx/weft-conflict.txt")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) != 2 {
		t.Fatalf("MergeIn(feature).Conflicts = %v; want exactly 2 entries", res.Conflicts)
	}
	if !sort.StringsAreSorted(res.Conflicts) {
		t.Errorf("MergeIn(feature).Conflicts = %v; want lexically sorted", res.Conflicts)
	}
	seen := map[string]bool{}
	for _, p := range res.Conflicts {
		if seen[p] {
			t.Errorf("MergeIn(feature).Conflicts = %v; want no duplicates, found repeat %q", res.Conflicts, p)
		}
		seen[p] = true
	}
	if !seen["warp-conflict.txt"] || !seen["_lyx/weft-conflict.txt"] {
		t.Errorf("MergeIn(feature).Conflicts = %v; want both warp-conflict.txt and _lyx/weft-conflict.txt", res.Conflicts)
	}
}

// TestMergeIn_NonASCIIConflictPaths_ReportedRawNotQuotedNotUnmergeable pins the core.quotepath
// regression: a conflict on a path outside git's default ASCII quoting set — on either side — must
// surface as the raw worktree-relative path. Before ConflictedFiles passed `-z`, git handed back
// the C-quoted rendering (`"\303\244..."`, quotes included): the warp side's reported path was a
// literal that exists nowhere in the worktree, and the weft side's quoted form failed
// weftPathVisible's prefix test, so a mappable in-tree conflict self-aborted the whole merge as
// *ErrUnmergeableState.
func TestMergeIn_NonASCIIConflictPaths_ReportedRawNotQuotedNotUnmergeable(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "ä-warp.txt")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "_lyx/ä-weft.txt")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v; want nil — a non-ASCII in-tree conflict is mappable, never *ErrUnmergeableState", err)
	}
	want := []string{"_lyx/ä-weft.txt", "ä-warp.txt"}
	if len(res.Conflicts) != len(want) || res.Conflicts[0] != want[0] || res.Conflicts[1] != want[1] {
		t.Errorf("MergeIn(feature).Conflicts = %q; want %q — raw path bytes, not git's C-quoted rendering", res.Conflicts, want)
	}
}

// TestMergeIn_OneSideFastForwardsOtherConflicts_AbortRestoresFastForwardedSide covers the B1 case:
// one side fast-forwards while the other conflicts, conflicts are reported, and MergeAbort returns
// the fast-forwarded side to its recorded pre-merge SHA.
func TestMergeIn_OneSideFastForwardsOtherConflicts_AbortRestoresFastForwardedSide(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupCleanFastForward(t, h.PrimeWorktree(), "feature", "ff.txt")
	setupConflictingDivergence(t, h.PrimeWeft(), "feature-weft", "_lyx/conflict.txt")

	// Captured after the divergence above lands, since those are the actual pre-merge SHAs MergeIn's
	// own record captures and MergeAbort must restore.
	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) Conflicts is empty; want the weft-side conflict reported")
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got == warpStartSHA {
		t.Fatalf("warp HEAD after conflicted MergeIn = %q (unchanged); want it to have fast-forwarded before abort restores it", got)
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
}

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
