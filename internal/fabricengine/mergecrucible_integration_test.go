// mergecrucible_integration_test.go holds the regression scenarios for merge defects, run against real git repositories.
// Each test's doc comment states the defect it pins.
// The file reuses newMergePairFixture and its sibling helpers from mergein_integration_test.go.
// It also reuses openFreshFabric from mergein_recovery_integration_test.go.

package fabricengine_test

import (
	"encoding/json"
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// assertSoleGuardReason fails unless err is a *fabricengine.MergeGuardError carrying exactly the reason want.
func assertSoleGuardReason(t *testing.T, label string, err error, want string) {
	t.Helper()

	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("%s error = %v (%T); want *fabricengine.MergeGuardError", label, err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != want {
		t.Errorf("%s guard reasons = %v; want exactly [%q]", label, guardErr.Reasons, want)
	}
}

// TestMergeCrucible_DetachedHeadRefused pins that a merge refuses while the warp checkout has a detached HEAD.
// Without that refusal, MergeIn reported success and finalized the weft merge commit.
// It also deleted its record.
// The warp merge commit was reachable from no ref and was lost at the next checkout.
func TestMergeCrucible_DetachedHeadRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "--detach", "HEAD")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	_, err := f.MergeIn("feature")
	assertSoleGuardReason(t, "MergeIn(feature)", err, "checkout is not on a branch")

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpBefore {
		t.Errorf("warp HEAD = %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q", got, weftBefore)
	}

	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after a refused merge; want false — a guard refusal must write no record")
	}
}

// TestMergeCrucible_WeftDetachedDoesNotRefuse pins that a detached weft HEAD does not block MergeIn.
// The weft is not a merge participant, so MergeIn completes and leaves the weft HEAD where it was.
func TestMergeCrucible_WeftDetachedDoesNotRefuse(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

	gitkit.MustRun(t, h.PrimeRecords(), "git", "checkout", "-q", "--detach", "HEAD")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	_, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v; want nil — a detached weft HEAD must no longer refuse a merge the warp alone completes", err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got == warpBefore {
		t.Errorf("warp HEAD = %q; want it to have moved off %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q — MergeIn never touches the weft", got, weftBefore)
	}
}

// TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides pins that MergeContinue refuses.
// A record whose attempt never reached one side is refused before anything lands.
// The record is byte-for-byte what a kill between the two MergeStart calls leaves:
// merge.go persists WarpOutcome only after the warp MergeStart returns, so WeftOutcome is still empty then.
// Without the guard, MergeContinue committed the warp side, then failed on a weft side that was never started.
// It returned an error telling the operator to run "lym fabric merge --continue" again, which could never succeed.
// It also left the pair out of correspondence.
// MergeAbort recovers the same record.
func TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")
	// Diverge the target branches so the reconstructed attempt stages rather than fast-forwards:
	// a fast-forward moves HEAD, and the crash window this test reconstructs is the staged one.
	commitOnWarpCurrent("target.txt", "target\n", "target: warp")
	commitOnWeftCurrent("target.txt", "target\n", "target: weft")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	// The warp side of the attempt ran; the weft side never did.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", "feature")
	if err := fabricengine.SaveMergeStateForTest(f, fabricengine.MergeStateForTest{
		Verb:        "merge-in",
		Source:      "feature",
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpOutcome: "staged",
		StartedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("SaveMergeStateForTest: %v", err)
	}

	resumed := openFreshFabric(t, h.PrimeWorktree())
	_, err := resumed.MergeContinue("")
	assertSoleGuardReason(t, "MergeContinue on an attempt that never reached both sides", err,
		"merge attempt did not reach both sides")

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD = %q after the refused MergeContinue; want unchanged %q — the refusal must land nothing", got, warpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftStart {
		t.Errorf("weft HEAD = %q after the refused MergeContinue; want unchanged %q", got, weftStart)
	}

	// The record survives the refusal, and MergeAbort is the working recovery.
	if _, err := openFreshFabric(t, h.PrimeWorktree()).MergeAbort(); err != nil {
		t.Fatalf("MergeAbort after a refused MergeContinue: %v", err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD = %q after MergeAbort; want %q", got, warpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftStart {
		t.Errorf("weft HEAD = %q after MergeAbort; want %q", got, weftStart)
	}
	inProgress, err := openFreshFabric(t, h.PrimeWorktree()).MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after MergeAbort; want false")
	}
}

// TestMergeCrucible_ResultFlagsDescribeWhatHappened checks that MergeResult.Committed and
// MergeResult.AlreadyUpToDate describe what the call did to the pair, not which return statement it reached.
// A merge that fast-forwards both sides reports no commit, since the record has no merge_committed entry.
// The loser of two concurrent MergeIn calls must report what a strictly sequential second call reports:
// {already_up_to_date:true, committed:false}, and not a committed call with no mutations.
// The second subtest is that sequential control.
func TestMergeCrucible_ResultFlagsDescribeWhatHappened(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("FastForwardBothSidesFabricatesNoCommit", func(t *testing.T) {
		h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
		commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
		commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

		res, err := f.MergeIn("feature")
		if err != nil {
			t.Fatalf("MergeIn(feature) error = %v", err)
		}
		if res.Committed {
			t.Error("MergeIn(feature).Committed = true; want false — both sides fast-forwarded, so no conclude-commit exists")
		}
		if res.AlreadyUpToDate {
			t.Error("MergeIn(feature).AlreadyUpToDate = true; want false — both sides advanced")
		}
		// The pair did advance, so the flags report "no commit", not "no merge".
		if !fileExistsInWorktree(t, h.PrimeWorktree(), "feature.txt") {
			t.Error("feature.txt missing from the warp worktree; want the fast-forward to have landed")
		}
	})

	t.Run("SecondCallReportsAlreadyUpToDateNotCommitted", func(t *testing.T) {
		_, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")
		commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
		commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")
		commitOnWarpCurrent("target.txt", "target\n", "target: warp")
		commitOnWeftCurrent("target.txt", "target\n", "target: weft")

		first, err := f.MergeIn("feature")
		if err != nil {
			t.Fatalf("first MergeIn(feature) error = %v", err)
		}
		if !first.Committed {
			t.Error("first MergeIn(feature).Committed = false; want true — both sides needed a conclude-commit")
		}

		second, err := f.MergeIn("feature")
		if err != nil {
			t.Fatalf("second MergeIn(feature) error = %v", err)
		}
		if !second.AlreadyUpToDate {
			t.Error("second MergeIn(feature).AlreadyUpToDate = false; want true")
		}
		if second.Committed {
			t.Error("second MergeIn(feature).Committed = true; want false — nothing was committed")
		}
	})
}

// fileExistsInWorktree reports whether name exists at the root of the worktree at dir.
func fileExistsInWorktree(t *testing.T, dir, name string) bool {
	t.Helper()

	_, err := os.Stat(filepath.Join(dir, name))
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat %s in %s: %v", name, dir, err)
	return false
}

// TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming checks one refusal of Topology.Remove.
// Remove must refuse a pair whose branches another pair's merge is resolving against.
// Checking only whether the pair being removed is mid-merge misses this case; the other pair's merge counts too:
// with the prime pair mid-merge on merge-in <slug>, removing <slug> must not delete branch <slug>-weft:
// the source work would then be reachable only from the remote if the operator aborted.
// Once the merge is aborted the same Remove must succeed,
// so the guard closes a window rather than blocking the pair forever.
func TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "merge-crucible-source"
	hubforge.AddPair(t, h, slug)

	sourceWarpDir := h.PairCodeWorktree(slug)
	sourceWeftDir := h.PairRecordsSibling(slug)
	sourceBranch := gitkit.CurrentBranch(t, sourceWarpDir)

	// The divergence conflicts on the warp side only:
	// a weft-root conflict would be unmappable and self-abort the whole attempt.
	// So MergeIn on the prime leaves a live record naming sourceBranch rather than concluding immediately.
	gitkit.CommitFile(t, sourceWarpDir, "conflict.txt", "source side\n", "source: warp conflict")
	gitkit.CommitFile(t, sourceWeftDir, "source-only.txt", "source weft\n", "source: weft advance")
	gitkit.CommitFile(t, h.PrimeWorktree(), "conflict.txt", "prime side\n", "prime: warp conflict")
	gitkit.CommitFile(t, h.PrimeRecords(), "prime-only.txt", "prime weft\n", "prime: weft advance")

	primeLocation, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
	}
	prime, err := fabricengine.Open(primeLocation)
	if err != nil {
		t.Fatalf("fabricengine.Open(prime): %v", err)
	}

	res, err := prime.MergeIn(sourceBranch)
	if err != nil {
		t.Fatalf("MergeIn(%s) on the prime pair error = %v; want a conflict result", sourceBranch, err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("MergeIn(%s).Conflicts is empty; the fixture must leave a live merge record", sourceBranch)
	}

	_, err = h.Topology.Remove(primeLocation, slug, false, false)
	var refused *fabricengine.ErrMergeInProgress
	if !errors.As(err, &refused) {
		t.Fatalf("Remove(%s) while the prime pair is mid-merge on its branches: error = %v (%T); want *ErrMergeInProgress", slug, err, err)
	}
	if !fileExistsInWorktree(t, sourceWarpDir, "conflict.txt") {
		t.Errorf("source warp worktree %s was torn down by the refused Remove", sourceWarpDir)
	}
	if !gitkit.BranchExists(t, h.PrimeRecords(), fabricengine.RecordsBranchName(sourceBranch)) {
		t.Errorf("weft branch %q was deleted by the refused Remove; want it intact", fabricengine.RecordsBranchName(sourceBranch))
	}

	// force answers dirtiness only, never a live merge record.
	if _, err := h.Topology.Remove(primeLocation, slug, true, false); !errors.As(err, &refused) {
		t.Fatalf("Remove(%s, force=true): error = %v (%T); want *ErrMergeInProgress even with force", slug, err, err)
	}

	// Once the merge is aborted the window is closed and the same Remove must succeed.
	if _, err := prime.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort: %v", err)
	}
	if _, err := h.Topology.Remove(primeLocation, slug, true, false); err != nil {
		t.Fatalf("Remove(%s) after MergeAbort: %v; want success — the guard must close a window, not block the pair forever", slug, err)
	}
}

// TestMergeCrucible_ConflictsIsEmptyNeverNil checks that MergeResult.Conflicts is never nil on any verb's return.
// A nil slice marshals as "conflicts": null, which a consumer does not read as an empty list.
// The check is on the marshalled JSON, since null versus [] is the property a consumer depends on.
func TestMergeCrucible_ConflictsIsEmptyNeverNil(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	assertConflictsMarshalsAsArray := func(t *testing.T, label string, res fabricengine.MergeResult) {
		t.Helper()
		if res.Conflicts == nil {
			t.Errorf("%s.Conflicts is nil; want an empty non-nil slice", label)
		}
		data, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("json.Marshal(%s): %v", label, err)
		}
		if !strings.Contains(string(data), `"conflicts":[]`) {
			t.Errorf("%s marshalled to %s; want it to carry \"conflicts\":[] rather than null", label, data)
		}
	}

	t.Run("MergeContinue", func(t *testing.T) {
		h, f := mergeCrucibleWarpConflictFixture(t)
		resolveWarpConflict(t, h.PrimeWorktree(), "conflict.txt")
		res, err := f.MergeContinue("")
		if err != nil {
			t.Fatalf("MergeContinue: %v", err)
		}
		assertConflictsMarshalsAsArray(t, "MergeContinue", res)
	})

	t.Run("MergeAbort", func(t *testing.T) {
		_, f := mergeCrucibleWarpConflictFixture(t)
		res, err := f.MergeAbort()
		if err != nil {
			t.Fatalf("MergeAbort: %v", err)
		}
		assertConflictsMarshalsAsArray(t, "MergeAbort", res)
	})

	// The error returns are not reached by the two success subtests above.
	// Every verb's error returns are zero MergeResults.
	// A zero MergeResult marshals with "conflicts": null.
	// Each subtest below picks a refusal that needs no mid-merge fixture.
	// What is under test is the return shape rather than the refusal.
	t.Run("MergeInGuardRefusal", func(t *testing.T) {
		_, f, _, _, _, _ := newMergePairFixture(t, ".")
		res, err := f.MergeIn("no-such-branch")
		if err == nil {
			t.Fatal("MergeIn(no-such-branch) error = nil; want a guard refusal — this subtest is about the error return's shape")
		}
		assertConflictsMarshalsAsArray(t, "MergeIn error return", res)
	})

	t.Run("MergeGuardRefusal", func(t *testing.T) {
		_, f, _, _, _, _ := newMergePairFixture(t, ".")
		res, err := f.Merge("no-such-branch", fabricengine.MergeOptions{})
		if err == nil {
			t.Fatal("Merge(no-such-branch) error = nil; want a guard refusal")
		}
		assertConflictsMarshalsAsArray(t, "Merge error return", res)
	})

	t.Run("MergeContinueNoMergeInProgress", func(t *testing.T) {
		_, f, _, _, _, _ := newMergePairFixture(t, ".")
		res, err := f.MergeContinue("")
		if !errors.As(err, new(*fabricengine.ErrNoMergeInProgress)) {
			t.Fatalf("MergeContinue() error = %v (%T); want *fabricengine.ErrNoMergeInProgress", err, err)
		}
		assertConflictsMarshalsAsArray(t, "MergeContinue error return", res)
	})

	t.Run("MergeAbortNoMergeInProgress", func(t *testing.T) {
		_, f, _, _, _, _ := newMergePairFixture(t, ".")
		res, err := f.MergeAbort()
		if !errors.As(err, new(*fabricengine.ErrNoMergeInProgress)) {
			t.Fatalf("MergeAbort() error = %v (%T); want *fabricengine.ErrNoMergeInProgress", err, err)
		}
		assertConflictsMarshalsAsArray(t, "MergeAbort error return", res)
	})
}

// mergeCrucibleWarpConflictFixture builds a pair left mid-merge by a real MergeIn that conflicted on
// the warp side only.
// The weft side diverges cleanly.
// A conflict on the weft root is unmappable and aborts the whole attempt before any record survives.
func mergeCrucibleWarpConflictFixture(t *testing.T) (*hubforge.Hub, *fabricengine.Fabric) {
	t.Helper()

	hub, fabric, onWarpBranch, onWeftBranch, onWarpCurrent, onWeftCurrent := newMergePairFixture(t, ".")
	onWarpBranch("feature", "conflict.txt", "feature side\n", "feature: warp conflict")
	onWeftBranch("feature-weft", "weft-only.txt", "weft feature\n", "feature: weft advance")
	onWarpCurrent("conflict.txt", "target side\n", "target: warp conflict")
	onWeftCurrent("target-only.txt", "weft target\n", "target: weft advance")

	res, err := fabric.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature): %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature).Conflicts is empty; the fixture must leave the pair mid-merge")
	}
	return hub, fabric
}

// resolveWarpConflict writes a resolution for name in the warp worktree at dir and stages it.
// Staging it lets MergeContinue's unresolved-conflicts guard pass.
func resolveWarpConflict(t *testing.T, dir, name string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolution for %s in %s: %v", name, dir, err)
	}
	gitkit.MustRun(t, dir, "git", "add", name)
}

// mergeHeadPresentInCheckout reports whether dir's checkout holds a live MERGE_HEAD.
// It probes with plain git rather than through the engine,
// so a test can assert what git believes independently of what fabric's record says.
func mergeHeadPresentInCheckout(t *testing.T, dir string) bool {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned pins that a merge which stages nothing is still concluded.
// Fixture: on both sides the source branch and the current branch reach the same content independently.
// Neither source is an ancestor of its side's HEAD, yet merging it stages nothing and moves no HEAD.
// Abandoning such a merge would leave git's MERGE_HEAD live while fabric drops its own record,
// and every merge verb would then refuse the pair with *ErrForeignMergeState.
// The MERGE_HEAD check is the assertion that catches this:
// a verb that returns without error must leave no git-level merge state on either side.
func TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")

	// On the warp side, the only side MergeIn merges, the branch and the trunk reach the same content independently.
	// The weft side is seeded identically only to prove it stays untouched.
	commitOnWarpBranch("feature", "shared.txt", "same change\n", "feature: warp same change")
	commitOnWarpCurrent("shared.txt", "same change\n", "target: warp same change, reached independently")
	commitOnWeftBranch("feature-weft", "shared-weft.txt", "same change\n", "feature: weft same change")
	commitOnWeftCurrent("shared-weft.txt", "same change\n", "target: weft same change, reached independently")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature): %v", err)
	}

	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) {
		t.Error("MERGE_HEAD is live in the warp checkout after MergeIn returned without error; fabric abandoned a merge it started")
	}
	if mergeHeadPresentInCheckout(t, h.PrimeRecords()) {
		t.Error("MERGE_HEAD is live in the weft checkout after MergeIn returned without error; fabric abandoned a merge it started")
	}

	if res.AlreadyUpToDate {
		t.Error("MergeIn(feature).AlreadyUpToDate = true; the warp source is not an ancestor of warp HEAD, so this is a real merge")
	}
	if !res.Committed {
		t.Error("MergeIn(feature).Committed = false; a real merge on the warp side must land its conclude-commit")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got == warpBefore {
		t.Errorf("warp HEAD = %q, unchanged; want the conclude-commit to have landed", got)
	}
	// The weft is not a merge participant, so its HEAD never moves.
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q — the weft is not a merge participant", got, weftBefore)
	}

	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after a completed MergeIn; want false")
	}

	// The pair must still be usable.
	// A MERGE_HEAD left live would make every later merge verb refuse with *ErrForeignMergeState.
	if _, err := f.MergeAbort(); !errors.As(err, new(*fabricengine.ErrNoMergeInProgress)) {
		t.Errorf("MergeAbort() after a completed MergeIn error = %v (%T); want *fabricengine.ErrNoMergeInProgress — a *ErrForeignMergeState here means fabric left state it will not clean up", err, err)
	}
}

// TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded pins that MergeAbort refuses to reset
// a landed conclude-commit.
// MergeAbort restores the warp side from its recorded pre-merge SHA,
// so an abort against a half-concluded attempt would discard a conclude-commit that has landed.
// With force: true the reset succeeds and drops the landed commit with no warning.
// That commit may carry the operator's hand-written conflict resolutions.
// Two arms, because the record is not always honest about what landed:
//   - Recorded: the warp conclude lands and WarpCommitted is set in the record.
//     The call then crashes before RecordCorrespondence and deleteMergeState run.
//     A real MergeContinue has no independent second side to fail on,
//     so SaveMergeStateForTest creates this crash point directly.
//   - Invisible: the operator concludes the warp side by hand with plain git.
//     Warp HEAD moves past its recorded start while warp_committed stays empty.
//     concludeMergeSides leaves this shape when CurrentSHA or the record re-save fails after `git commit` succeeds,
//     and a guard keyed only on the recorded SHA would sail straight through it.
//
// Both must refuse, and the landed commit must survive; MergeContinue must then still finish.
func TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// landWarpConclude drives the warp side's conclude-commit into place and reports the SHA it landed.
		// It leaves the pair mid-merge with the record still live.
		landWarpConclude func(t *testing.T, h *hubforge.Hub, f *fabricengine.Fabric) string
	}{
		{
			name: "RecordedConcludeSHA",
			landWarpConclude: func(t *testing.T, h *hubforge.Hub, f *fabricengine.Fabric) string {
				t.Helper()
				st, found, err := fabricengine.LoadMergeStateForTest(f)
				if err != nil || !found {
					t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
				}
				gitkit.Git(t, h.PrimeWorktree(), "commit", "--no-edit")
				sha := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
				st.WarpCommitted = sha
				if err := fabricengine.SaveMergeStateForTest(f, st); err != nil {
					t.Fatalf("SaveMergeStateForTest() error = %v", err)
				}
				return sha
			},
		},
		{
			name: "InvisibleConcludeTheRecordNeverLearnedAbout",
			landWarpConclude: func(t *testing.T, h *hubforge.Hub, f *fabricengine.Fabric) string {
				t.Helper()
				gitkit.Git(t, h.PrimeWorktree(), "commit", "--no-edit")
				return fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f := mergeCrucibleWarpConflictFixture(t)
			resolveWarpConflict(t, h.PrimeWorktree(), "conflict.txt")

			warpStart := readMergeRecordWarpStart(t, h)
			landedSHA := tt.landWarpConclude(t, h, f)
			if landedSHA == warpStart {
				t.Fatalf("fixture broken: warp HEAD = %q is still its recorded pre-merge SHA; no conclude landed", landedSHA)
			}

			res, err := f.MergeAbort()
			assertSoleGuardReason(t, "MergeAbort()", err, "merge conclude already landed")
			if res.Mutated().Len() != 0 {
				t.Errorf("MergeAbort() mutations = %v; want none — a refusal must not touch either checkout", res.Mutated().Entries())
			}
			if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != landedSHA {
				t.Errorf("warp HEAD = %q after the refused abort; want the landed conclude-commit %q still in place", got, landedSHA)
			}
			inProgress, err := f.MergeInProgress()
			if err != nil {
				t.Fatalf("MergeInProgress: %v", err)
			}
			if !inProgress {
				t.Error("MergeInProgress() = false after a refused abort; the record must survive so MergeContinue can still finish")
			}
		})
	}
}

// TestMergeCrucible_AbortRefusesOnTheRecordedConcludeSHAAlone pins the first clause of sideConcludeMayHaveLanded:
// the record already carries this side's conclude SHA.
// No other test fails if that clause is removed, because every existing fixture also meets the second clause:
// an outcome of staged or conflicted, with HEAD moved off the recorded start.
// The test isolates the first clause by putting HEAD back.
// Here a half-concluded attempt had its warp conclude landed and recorded.
// The operator then resets that side to its recorded pre-merge SHA.
// HEAD no longer looks moved.
// Only the record's memory of the conclude stands between MergeAbort and a force reset.
// The abort must still refuse.
// The recorded SHA is evidence that a commit was made.
// After the reset, that commit is reachable from no branch.
// An abort that proceeded would discard it for good, along with any resolutions it carries.
// The record is written directly with SaveMergeStateForTest.
// A real MergeContinue call has no independent weft side left to fail on, and so it finishes one-sided.
func TestMergeCrucible_AbortRefusesOnTheRecordedConcludeSHAAlone(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f := mergeCrucibleWarpConflictFixture(t)
	resolveWarpConflict(t, h.PrimeWorktree(), "conflict.txt")

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	gitkit.Git(t, h.PrimeWorktree(), "commit", "--no-edit")
	st.WarpCommitted = fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if err := fabricengine.SaveMergeStateForTest(f, st); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	// The operator puts the landed side back, so the HEAD-moved clause can no longer fire.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "reset", "--hard", st.WarpStart)

	// Preconditions, asserted rather than assumed: HEAD is no longer off its recorded start,
	// so the second clause is false and the recorded SHA is the only evidence left.
	// The weft side never moved; it is not a merge participant.
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != st.WarpStart {
		t.Fatalf("warp HEAD = %q after the reset; want the recorded start %q, or the HEAD-moved clause refuses instead of the one under test", got, st.WarpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != st.WeftStart {
		t.Fatalf("weft HEAD = %q; want the recorded start %q", got, st.WeftStart)
	}

	res, err := f.MergeAbort()
	assertSoleGuardReason(t, "MergeAbort()", err, "merge conclude already landed")
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeAbort() mutations = %v; want none — a refusal must not touch either checkout", res.Mutated().Entries())
	}
	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if !inProgress {
		t.Error("MergeInProgress() = false after a refused abort; the record must survive so MergeContinue can still finish")
	}
}

// readMergeRecordWarpStart reads the pre-merge SHA that the live merge-state record holds for the warp side.
// The value read is the one MergeAbort would reset the warp side to.
func readMergeRecordWarpStart(t *testing.T, h *hubforge.Hub) string {
	t.Helper()

	path := filepath.Join(strings.TrimSpace(gitOutput(t, h.PrimeRecords(), "rev-parse", "--absolute-git-dir")), "fabric-merge.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read merge record %s: %v", path, err)
	}
	var record struct {
		WarpStart string `json:"warp_start"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("unmarshal merge record %s: %v", path, err)
	}
	if record.WarpStart == "" {
		t.Fatalf("merge record %s has an empty warp_start", path)
	}
	return record.WarpStart
}

// TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord pins the derivation of MergeResult.AlreadyUpToDate:
// it is read from mergeState.bothSidesAlreadyUpToDate.
// No other test fails if the field is hardwired to false.
// A plain second merge call never reads the field:
// the pre-lock already-up-to-date probe returns AlreadyUpToDate: true from its own return site,
// so the derived field is never read.
// Reaching the derived field needs both post-lock MergeStart outcomes to report up_to_date.
// The pre-lock probe must still say otherwise.
// A real race between two processes achieves that,
// but this test uses a deterministic, single-process route that needs no seam:
// a squash merge whose source is NOT an ancestor of HEAD on either side.
// IsAncestor is then false, so the pre-lock probe cannot early-return.
// The squash result tree equals HEAD's own tree.
// Then the squash stages nothing, moves no HEAD, writes no MERGE_HEAD, and classifies up_to_date on both sides.
// AlreadyUpToDate is then the correct report, and it can only come from the record.
func TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")

	// The same content reaches the branch and the trunk independently, on both sides, so each
	// source is a real non-ancestor whose merge result is nevertheless empty.
	commitOnWarpBranch("feature", "shared.txt", "same change\n", "feature: warp same change")
	commitOnWarpCurrent("shared.txt", "same change\n", "target: warp same change, reached independently")
	commitOnWeftBranch("feature-weft", "shared-weft.txt", "same change\n", "feature: weft same change")
	commitOnWeftCurrent("shared-weft.txt", "same change\n", "target: weft same change, reached independently")

	// The source must not be an ancestor of the target:
	// an ancestor source returns early from the pre-lock probe, so the derived AlreadyUpToDate field would never be read.
	if gitkit.IsAncestor(t, h.PrimeWorktree(), "feature", "HEAD") {
		t.Fatal("fixture broken: feature is an ancestor of the warp HEAD, so the pre-lock probe would short-circuit before the derived field is read")
	}
	if gitkit.IsAncestor(t, h.PrimeRecords(), "feature-weft", "HEAD") {
		t.Fatal("fixture broken: feature-weft is an ancestor of the weft HEAD, so the pre-lock probe would short-circuit before the derived field is read")
	}

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	res, err := f.Merge("feature", fabricengine.MergeOptions{Squash: true})
	if err != nil {
		t.Fatalf("Merge(feature, squash): %v", err)
	}

	if !res.AlreadyUpToDate {
		t.Error("Merge(feature, squash).AlreadyUpToDate = false; want true — both sides' post-lock MergeStart outcomes are up_to_date, which is what the derived field reads")
	}
	if res.Committed {
		t.Error("Merge(feature, squash).Committed = true; want false — a squash with an empty result has nothing to commit")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpBefore {
		t.Errorf("warp HEAD = %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q", got, weftBefore)
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) || mergeHeadPresentInCheckout(t, h.PrimeRecords()) {
		t.Error("MERGE_HEAD is live after a squash merge; squash never writes one, so the up_to_date classification must not have left git mid-merge")
	}

	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after the merge returned; want false")
	}
}

// TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource pins that Remove refuses a source pair:
// a linked pair's merge of that source is in progress.
//
// Merge records live at <weft gitdir>/fabric-merge.json.
// The weft gitdir is either the weft repo's own .git (the prime pair) or .git/worktrees/<name>/ (every other pair).
// The in-flight check scans both shapes; this test covers the linked one, where a merge normally runs.
//
// The fixture runs the merge from a linked pair and drives Remove from the prime's own location,
// so the refusal can only come from the hub-wide scan finding a record in a worktree gitdir.
// Both record locations are asserted with t.Fatal before Remove is called: the linked one must exist
// and the prime one must not, or the test would pass on the path it does not cover.
func TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const consumerSlug = "merge-crucible-consumer"
	const sourceSlug = "merge-crucible-linked-source"
	hubforge.AddPair(t, h, consumerSlug)
	hubforge.AddPair(t, h, sourceSlug)

	consumerWarpDir := h.PairCodeWorktree(consumerSlug)
	consumerWeftDir := h.PairRecordsSibling(consumerSlug)
	sourceWarpDir := h.PairCodeWorktree(sourceSlug)
	sourceWeftDir := h.PairRecordsSibling(sourceSlug)

	sourceBranch := gitkit.CurrentBranch(t, sourceWarpDir)

	// Warp-side-only conflict:
	// a weft-root conflict would be unmappable and self-abort the attempt instead of leaving a live record.
	gitkit.CommitFile(t, sourceWarpDir, "conflict.txt", "source side\n", "source: warp conflict")
	gitkit.CommitFile(t, sourceWeftDir, "source-only.txt", "source weft\n", "source: weft advance")
	gitkit.CommitFile(t, consumerWarpDir, "conflict.txt", "consumer side\n", "consumer: warp conflict")
	gitkit.CommitFile(t, consumerWeftDir, "consumer-only.txt", "consumer weft\n", "consumer: weft advance")

	consumer := openFreshFabric(t, consumerWarpDir)
	res, err := consumer.MergeIn(sourceBranch)
	if err != nil {
		t.Fatalf("MergeIn(%s) on the linked consumer pair error = %v; want a conflict result", sourceBranch, err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("MergeIn(%s).Conflicts is empty; the fixture must leave a live merge record", sourceBranch)
	}

	// Precondition: the record is in the linked shape, and the prime shape is empty.
	// Otherwise this test would cover the prime path instead of the linked one.
	linkedRecord := filepath.Join(h.PrimeRecords(), ".git", "worktrees", filepath.Base(consumerWeftDir), "fabric-merge.json")
	if _, err := os.Stat(linkedRecord); err != nil {
		t.Fatalf("Stat(%s) = %v; want the merge record in the linked pair's own weft gitdir", linkedRecord, err)
	}
	primeRecord := filepath.Join(h.PrimeRecords(), ".git", "fabric-merge.json")
	if _, err := os.Stat(primeRecord); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) = %v; want not-exist — a record in the prime shape would let this test pass without the linked glob", primeRecord, err)
	}

	primeLocation, err := lymcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
	}

	_, err = h.Topology.Remove(primeLocation, sourceSlug, false, false)
	var refused *fabricengine.ErrMergeInProgress
	if !errors.As(err, &refused) {
		t.Fatalf("Remove(%s) while a LINKED pair is mid-merge on its branches: error = %v (%T); want *ErrMergeInProgress", sourceSlug, err, err)
	}
	if !fileExistsInWorktree(t, sourceWarpDir, "conflict.txt") {
		t.Errorf("source warp worktree %s was torn down by the refused Remove", sourceWarpDir)
	}
	if !gitkit.BranchExists(t, h.PrimeRecords(), fabricengine.RecordsBranchName(sourceBranch)) {
		t.Errorf("weft branch %q was deleted by the refused Remove; want it intact", fabricengine.RecordsBranchName(sourceBranch))
	}

	// force answers dirtiness only, never a live merge record.
	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true, false); !errors.As(err, &refused) {
		t.Fatalf("Remove(%s, force=true): error = %v (%T); want *ErrMergeInProgress even with force", sourceSlug, err, err)
	}

	// Aborting the linked pair's merge closes the window, so Remove must then succeed.
	if _, err := consumer.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort on the linked consumer pair: %v", err)
	}
	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true, false); err != nil {
		t.Fatalf("Remove(%s) after MergeAbort: %v; want success — the guard must close a window, not block the pair forever", sourceSlug, err)
	}
}
