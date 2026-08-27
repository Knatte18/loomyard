//go:build integration

// mergecrucible_integration_test.go holds the regression scenarios the crucible review round
// opus-medium-r1 found against real git repositories — the ones the hermetic and pre-existing
// integration tiers both passed while the defect was live.
// Each test names the finding it pins in its own doc comment.
// Reuses newMergePairFixture and its sibling helpers from mergein_integration_test.go and
// openFreshFabric from mergein_recovery_integration_test.go, since all three files share
// package fabricengine_test.

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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// assertSoleGuardReason fails unless err is a *fabricengine.MergeGuardError carrying exactly the one
// reason want.
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

// TestMergeCrucible_DetachedHeadRefused pins finding F2 on the warp side: a merge verb must refuse
// while the warp checkout has HEAD pointing straight at a commit rather than at a branch.
// Without the guard, MergeIn reported full success on a detached warp HEAD, landed a warp merge
// commit no ref reaches, landed the weft merge commit permanently, and deleted its own record — so
// the warp half vanished at the next checkout with the weft half already final and no longer
// abortable.
// A detached WEFT HEAD no longer refuses: the weft is not a merge participant, so its head
// attachment cannot affect a warp-only merge's correctness — TestMergeCrucible_WeftDetachedDoesNotRefuse
// below covers that side instead, asserting the merge proceeds.
func TestMergeCrucible_DetachedHeadRefused(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "-q", "--detach", "HEAD")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	_, err := f.MergeIn("feature")
	assertSoleGuardReason(t, "MergeIn(feature)", err, "checkout is not on a branch")

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpBefore {
		t.Errorf("warp HEAD = %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
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

// TestMergeCrucible_WeftDetachedDoesNotRefuse used to be TestMergeCrucible_DetachedHeadRefused's
// WeftDetached table case, pinning detachedHeadReason's now-removed weft arm. detachedHeadReason
// evaluates the warp side alone now, so a detached weft HEAD no longer blocks MergeIn: the weft is
// not a merge participant, and its own detachment cannot affect a warp-only merge's correctness.
func TestMergeCrucible_WeftDetachedDoesNotRefuse(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

	gitkit.MustRun(t, h.PrimeWeft(), "git", "checkout", "-q", "--detach", "HEAD")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	_, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v; want nil — a detached weft HEAD must no longer refuse a merge the warp alone completes", err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got == warpBefore {
		t.Errorf("warp HEAD = %q; want it to have moved off %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q — MergeIn never touches the weft", got, weftBefore)
	}
}

// TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides pins finding F1: a record whose
// attempt never reached one side must refuse MergeContinue outright, before anything lands.
// The reconstructed state is byte-for-byte what a kill between the two MergeStart calls leaves —
// merge.go persists WarpOutcome only after the warp MergeStart returns, so WeftOutcome is still
// empty at that instant. Without the guard, MergeContinue committed the warp side, then failed
// concluding a weft side that was never started, returned "run \"lyx fabric merge --continue\" again"
// (an instruction
// that could never succeed), and left the pair out of correspondence.
// MergeAbort must still recover the same record — that is the whole point of refusing.
func TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")
	// Diverge the target branches so the reconstructed attempt STAGES rather than fast-forwards:
	// a fast-forward moves HEAD, and the crash window this test reconstructs is the staged one.
	commitOnWarpCurrent("target.txt", "target\n", "target: warp")
	commitOnWeftCurrent("target.txt", "target\n", "target: weft")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	// The warp side of the attempt really ran; the weft side never did.
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
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStart {
		t.Errorf("weft HEAD = %q after the refused MergeContinue; want unchanged %q", got, weftStart)
	}

	// The record survives the refusal, and MergeAbort is still the working recovery.
	if _, err := openFreshFabric(t, h.PrimeWorktree()).MergeAbort(); err != nil {
		t.Fatalf("MergeAbort after a refused MergeContinue: %v", err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD = %q after MergeAbort; want %q", got, warpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStart {
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

// TestMergeCrucible_ResultFlagsDescribeWhatHappened pins finding F3: MergeResult.Committed and
// MergeResult.AlreadyUpToDate must describe what the call did to the pair, not which return
// statement it reached.
// Both verbs used to hardcode Committed true on the both-sides-clean path, so a merge that
// fast-forwarded both sides reported committed with no merge_committed entry anywhere in the record,
// and the loser of two concurrent MergeIn calls reported
// {already_up_to_date:false, committed:true, mutations:[]} for a call that did nothing — where a
// strictly sequential run of the same two calls honestly reports {already_up_to_date:true,
// committed:false}. The second subtest is that sequential control, which is what the interleaved
// loser now also reports.
func TestMergeCrucible_ResultFlagsDescribeWhatHappened(t *testing.T) {
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
		// The pair really did advance, so the flags are reporting "no commit", not "no merge".
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

// TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming pins finding F5: Topology.Remove
// must refuse a pair whose branches some OTHER pair's merge is currently resolving against.
// The pre-existing guard asks only "is the pair being removed itself mid-merge", which is a
// different subject: with the prime pair mid-merge on merge-in <slug>, removing <slug> succeeded and
// deleted branch <slug>-weft out from under the live merge, leaving the source work reachable only
// from the remote if the operator then aborted. Once the merge is aborted the same Remove must
// succeed again, so the guard closes a window rather than blocking the pair forever.
func TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "merge-crucible-source"
	hubforge.AddPair(t, h, slug)

	sourceWarpDir := h.PairWarpWorktree(slug)
	sourceWeftDir := h.PairWeftSibling(slug)
	sourceBranch, err := readBranchForTest(t, sourceWarpDir)
	if err != nil {
		t.Fatalf("readBranchForTest(%s): %v", sourceWarpDir, err)
	}

	// Conflicting divergence on the warp side only — a weft-root conflict would be unmappable and
	// self-abort the whole attempt — so MergeIn on the prime leaves a live record naming sourceBranch
	// rather than concluding immediately.
	commitOnCurrentBranch(t, sourceWarpDir, "conflict.txt", "source side\n", "source: warp conflict")
	commitOnCurrentBranch(t, sourceWeftDir, "source-only.txt", "source weft\n", "source: weft advance")
	commitOnCurrentBranch(t, h.PrimeWorktree(), "conflict.txt", "prime side\n", "prime: warp conflict")
	commitOnCurrentBranch(t, h.PrimeWeft(), "prime-only.txt", "prime weft\n", "prime: weft advance")

	primeLocation, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
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

	_, err = h.Topology.Remove(primeLocation, slug, false)
	var refused *fabricengine.ErrMergeInProgress
	if !errors.As(err, &refused) {
		t.Fatalf("Remove(%s) while the prime pair is mid-merge on its branches: error = %v (%T); want *ErrMergeInProgress", slug, err, err)
	}
	if !fileExistsInWorktree(t, sourceWarpDir, "conflict.txt") {
		t.Errorf("source warp worktree %s was torn down by the refused Remove", sourceWarpDir)
	}
	if !branchExistsLocally(t, h.PrimeWeft(), fabricengine.WeftBranchName(sourceBranch)) {
		t.Errorf("weft branch %q was deleted by the refused Remove; want it intact", fabricengine.WeftBranchName(sourceBranch))
	}

	// force answers dirtiness only, never a live merge record.
	if _, err := h.Topology.Remove(primeLocation, slug, true); !errors.As(err, &refused) {
		t.Fatalf("Remove(%s, force=true): error = %v (%T); want *ErrMergeInProgress even with force", slug, err, err)
	}

	// Once the merge is aborted the window is closed and the same Remove must succeed.
	if _, err := prime.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort: %v", err)
	}
	if _, err := h.Topology.Remove(primeLocation, slug, true); err != nil {
		t.Fatalf("Remove(%s) after MergeAbort: %v; want success — the guard must close a window, not block the pair forever", slug, err)
	}
}

// TestMergeCrucible_ConflictsIsEmptyNeverNil pins finding F7: MergeResult.Conflicts is documented as
// empty-never-nil so a caller's JSON never sees "conflicts": null, and merge.go declares the
// mergeNoConflicts sentinel for exactly that — but MergeContinue and MergeAbort both returned a bare
// MergeResult, leaving the field nil on their success paths.
// The check is on the marshalled JSON, since null-vs-[] is the property that actually matters to a
// consumer.
func TestMergeCrucible_ConflictsIsEmptyNeverNil(t *testing.T) {
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

	// The error returns, which the two success subtests above do not reach. Every verb has roughly a
	// dozen of them, they all returned a zero MergeResult, and a zero MergeResult marshals with
	// "conflicts": null — so the property the type's godoc promises held on the paths a test happened
	// to name and nowhere else. Each arm below picks a refusal that needs no mid-merge fixture, so
	// what is under test is the return shape rather than the refusal.
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
// the warp side only — the weft side diverges cleanly, since a weft-root conflict would be
// unmappable and self-abort the whole attempt before any record survives.
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

// resolveWarpConflict writes a resolution for name in the warp worktree at dir and stages it, so
// MergeContinue's unresolved-conflicts guard passes.
func resolveWarpConflict(t *testing.T, dir, name string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolution for %s in %s: %v", name, dir, err)
	}
	gitkit.MustRun(t, dir, "git", "add", name)
}

// mergeHeadPresentInCheckout reports whether dir's checkout holds a live MERGE_HEAD, probed with
// plain git rather than through the engine, so a test can assert what git believes independently of
// what fabric's own record says.
func mergeHeadPresentInCheckout(t *testing.T, dir string) bool {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned pins crucible round opus-medium-r2's
// finding R1 at the pair level.
// Fixture: on both sides the source branch and the current branch reach the same content
// independently, so neither source is an ancestor of its side's HEAD, yet merging it stages nothing
// and moves no HEAD. Before the fix, gitrepo.MergeStart classified that as MergeAlreadyUpToDate, so
// MergeIn reported {ok, already_up_to_date: true, committed: false}, skipped the conclude on both
// sides, recorded the pre-merge SHAs as the pair's post-merge correspondence, and deleted its own
// merge-state record -- while git had a live MERGE_HEAD in both checkouts. The merge was silently
// lost, MergeInProgress() disagreed with git, and every merge verb (including MergeAbort, the
// recovery verb) then refused the pair with *ErrForeignMergeState, leaving only plain git as a way
// out.
// The assertion that catches the regression is the MERGE_HEAD pair: a verb that returns without
// error must never leave git-level merge state behind on either side.
func TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")

	// The same content reaches the branch and the trunk independently, on the warp side — the only
	// side MergeIn merges. The weft side is seeded identically only to prove it stays untouched.
	commitOnWarpBranch("feature", "shared.txt", "same change\n", "feature: warp same change")
	commitOnWarpCurrent("shared.txt", "same change\n", "target: warp same change, reached independently")
	commitOnWeftBranch("feature-weft", "shared-weft.txt", "same change\n", "feature: weft same change")
	commitOnWeftCurrent("shared-weft.txt", "same change\n", "target: weft same change, reached independently")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature): %v", err)
	}

	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) {
		t.Error("MERGE_HEAD is live in the warp checkout after MergeIn returned without error; fabric abandoned a merge it started")
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWeft()) {
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
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q — the weft is not a merge participant", got, weftBefore)
	}

	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after a completed MergeIn; want false")
	}

	// The pair must still be usable: before the fix, the abandoned MERGE_HEAD made every subsequent
	// merge verb refuse with *ErrForeignMergeState.
	if _, err := f.MergeAbort(); !errors.As(err, new(*fabricengine.ErrNoMergeInProgress)) {
		t.Errorf("MergeAbort() after a completed MergeIn error = %v (%T); want *fabricengine.ErrNoMergeInProgress — a *ErrForeignMergeState here means fabric left state it will not clean up", err, err)
	}
}

// TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded pins crucible round opus-medium-r2's
// finding R2: MergeAbort restores the warp side from its recorded pre-merge SHA, so an abort issued
// against a half-concluded attempt discarded a conclude-commit that had really landed -- in this
// flow, one carrying the operator's own hand-written conflict resolutions, reset away under
// force: true with an "ok" result and no warning.
// Two arms, because the record is not always honest about what landed:
//   - Recorded: the warp conclude lands and IS written into the record (WarpCommitted set), then the
//     call crashes before it can run RecordCorrespondence/deleteMergeState — the one window, now that
//     the weft is not a merge participant and concludeMergeSides has nothing left to conclude on that
//     side, where the record can know the conclude landed but the call as a whole never finished.
//     SaveMergeStateForTest manufactures that exact post-record-save, pre-delete crash point directly,
//     since a real MergeContinue call no longer has an independent second side to fail on and complete
//     one-sided otherwise.
//   - Invisible: the operator concludes the warp side by hand with plain git, so warp HEAD has moved
//     past its recorded start while warp_committed is still empty. concludeMergeSides leaves exactly
//     this shape whenever CurrentSHA or the record re-save fails after `git commit` succeeded, and a
//     guard keyed only on the recorded SHA would sail straight through it.
//
// Both must refuse, and the landed commit must survive; MergeContinue must then still finish.
func TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded(t *testing.T) {
	tests := []struct {
		name string
		// landWarpConclude drives the warp side's conclude-commit into place and reports the SHA it
		// landed, leaving the pair mid-merge with the record still live.
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
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
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
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
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

// TestMergeCrucible_AbortRefusesOnTheRecordedConcludeSHAAlone pins sideConcludeMayHaveLanded's FIRST
// clause -- "the record already carries this side's conclude SHA" -- which no other test can fail on.
// Deleting that clause left the whole integration suite green, because every existing fixture also
// satisfies the second clause (outcome staged/conflicted AND HEAD moved off the recorded start), so
// the HEAD read is always what actually refuses.
// The shape that isolates the first clause puts HEAD back: after a half-concluded attempt whose warp
// conclude landed and WAS recorded, the operator resets that side to the recorded pre-merge SHA.
// HEAD no longer looks moved, so only the record's own memory of the conclude stands between
// MergeAbort and a force reset. It must still refuse: the recorded SHA is evidence that a commit was
// made, and after the reset that commit is reachable from no branch, so an abort that proceeded would
// discard it for good along with any resolutions it carries.
// The record is manufactured directly via SaveMergeStateForTest, the same "record learned about a
// landed conclude, then the call never reached its own delete" shape
// TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded's RecordedConcludeSHA arm builds — a real
// MergeContinue call has no independent weft side left to fail on and finish one-sided.
func TestMergeCrucible_AbortRefusesOnTheRecordedConcludeSHAAlone(t *testing.T) {
	h, f := mergeCrucibleWarpConflictFixture(t)
	resolveWarpConflict(t, h.PrimeWorktree(), "conflict.txt")

	st, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil || !found {
		t.Fatalf("LoadMergeStateForTest() = (_, %v, %v); want found", found, err)
	}
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
	st.WarpCommitted = fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	if err := fabricengine.SaveMergeStateForTest(f, st); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	// The operator puts the landed side back, so the HEAD-moved clause can no longer fire.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "reset", "--hard", st.WarpStart)

	// Preconditions, asserted rather than assumed: HEAD is not off its recorded start any more, so the
	// second clause is false and the recorded SHA is the only evidence left. The weft side never moved
	// in the first place — it is not a merge participant.
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != st.WarpStart {
		t.Fatalf("warp HEAD = %q after the reset; want the recorded start %q, or the HEAD-moved clause refuses instead of the one under test", got, st.WarpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != st.WeftStart {
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

// readMergeRecordWarpStart reads the pre-merge SHA the live merge-state record holds for the warp
// side, straight off disk -- the value MergeAbort would reset that side to.
func readMergeRecordWarpStart(t *testing.T, h *hubforge.Hub) string {
	t.Helper()

	path := filepath.Join(strings.TrimSpace(gitOutput(t, h.PrimeWeft(), "rev-parse", "--absolute-git-dir")), "fabric-merge.json")
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

// isAncestorInCheckout reports whether ref is an ancestor of descendant in dir's checkout, probed
// with plain git so a test can state the precondition the engine's own pre-lock probe reads.
func isAncestorInCheckout(t *testing.T, dir, ref, descendant string) bool {
	t.Helper()

	cmd := exec.Command("git", "merge-base", "--is-ancestor", ref, descendant)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord closes crucible round opus-medium-r2's
// residual 1: mergeState.bothSidesAlreadyUpToDate, which MergeResult.AlreadyUpToDate is derived
// from, had no test coverage at all. Hardwiring it to false left the entire suite green.
// The pre-existing subtest that looks like it covers the field does not: a plain second merge call is
// caught by the pre-lock already-up-to-date probe, which returns a hardcoded AlreadyUpToDate: true
// from a different return site, so the derived field is never read.
// Reaching the derived field needs both post-lock MergeStart outcomes to come back up_to_date while
// the pre-lock probe said otherwise. A real race between two processes gets there, but the route
// this test uses is deterministic, single-process and needs no seam: a squash merge whose source is
// NOT an ancestor of HEAD on either side -- so IsAncestor is false and the pre-lock probe cannot
// early-return -- but whose squash result tree equals HEAD's own tree, so `git merge --squash`
// stages nothing, moves no HEAD, writes no MERGE_HEAD, and classifies up_to_date on both sides.
// Reporting AlreadyUpToDate there is the honest answer, and it can only have come from the record.
func TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")

	// The same content reaches the branch and the trunk independently, on both sides, so each
	// source is a real non-ancestor whose merge result is nevertheless empty.
	commitOnWarpBranch("feature", "shared.txt", "same change\n", "feature: warp same change")
	commitOnWarpCurrent("shared.txt", "same change\n", "target: warp same change, reached independently")
	commitOnWeftBranch("feature-weft", "shared-weft.txt", "same change\n", "feature: weft same change")
	commitOnWeftCurrent("shared-weft.txt", "same change\n", "target: weft same change, reached independently")

	// This is the whole point of the fixture: an ancestor source would be caught by the pre-lock
	// probe's hardcoded return and the derived field would never be read.
	if isAncestorInCheckout(t, h.PrimeWorktree(), "feature", "HEAD") {
		t.Fatal("fixture broken: feature is an ancestor of the warp HEAD, so the pre-lock probe would short-circuit before the derived field is read")
	}
	if isAncestorInCheckout(t, h.PrimeWeft(), "feature-weft", "HEAD") {
		t.Fatal("fixture broken: feature-weft is an ancestor of the weft HEAD, so the pre-lock probe would short-circuit before the derived field is read")
	}

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

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
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q", got, weftBefore)
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) || mergeHeadPresentInCheckout(t, h.PrimeWeft()) {
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

// TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource is the linked-worktree twin of
// TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming, and it covers the half of
// mergeSourceInFlight that actually matters in practice.
//
// Merge records live at <weft gitdir>/fabric-merge.json, and the weft gitdir has two shapes: the weft
// repo's own .git for the PRIME pair, and .git/worktrees/<name>/ for every other pair.
// mergeSourceInFlight globs both. Only the prime shape was covered, and that is the rarer one — a
// merge normally runs in a task pair, not in the prime worktree. Measured rather than assumed:
// pointing the linked glob at a filename that cannot exist left the whole suite green, while doing the
// same to the prime path was caught immediately.
//
// The fixture therefore runs the merge from a LINKED pair and drives Remove from the prime's own
// location, so the refusal can only come from the hub-wide scan finding a record in a worktree gitdir.
// Both record locations are asserted with t.Fatal before Remove is called: the linked one must exist
// and the prime one must not, or the test would pass on the path it is not trying to cover.
func TestMergeCrucible_RemoveRefusesWhenALinkedPairIsConsumingTheSource(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const consumerSlug = "merge-crucible-consumer"
	const sourceSlug = "merge-crucible-linked-source"
	hubforge.AddPair(t, h, consumerSlug)
	hubforge.AddPair(t, h, sourceSlug)

	consumerWarpDir := h.PairWarpWorktree(consumerSlug)
	consumerWeftDir := h.PairWeftSibling(consumerSlug)
	sourceWarpDir := h.PairWarpWorktree(sourceSlug)
	sourceWeftDir := h.PairWeftSibling(sourceSlug)

	sourceBranch, err := readBranchForTest(t, sourceWarpDir)
	if err != nil {
		t.Fatalf("readBranchForTest(%s): %v", sourceWarpDir, err)
	}

	// Warp-side-only conflicting divergence, matching the prime-pair test's own reasoning: a weft-root
	// conflict would be unmappable and self-abort the attempt instead of leaving a live record.
	commitOnCurrentBranch(t, sourceWarpDir, "conflict.txt", "source side\n", "source: warp conflict")
	commitOnCurrentBranch(t, sourceWeftDir, "source-only.txt", "source weft\n", "source: weft advance")
	commitOnCurrentBranch(t, consumerWarpDir, "conflict.txt", "consumer side\n", "consumer: warp conflict")
	commitOnCurrentBranch(t, consumerWeftDir, "consumer-only.txt", "consumer weft\n", "consumer: weft advance")

	consumer := openFreshFabric(t, consumerWarpDir)
	res, err := consumer.MergeIn(sourceBranch)
	if err != nil {
		t.Fatalf("MergeIn(%s) on the linked consumer pair error = %v; want a conflict result", sourceBranch, err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("MergeIn(%s).Conflicts is empty; the fixture must leave a live merge record", sourceBranch)
	}

	// Precondition: the record really is in the LINKED shape, and the prime shape is empty — otherwise
	// this test would be re-covering the path the sibling test already covers.
	linkedRecord := filepath.Join(h.PrimeWeft(), ".git", "worktrees", filepath.Base(consumerWeftDir), "fabric-merge.json")
	if _, err := os.Stat(linkedRecord); err != nil {
		t.Fatalf("Stat(%s) = %v; want the merge record in the linked pair's own weft gitdir", linkedRecord, err)
	}
	primeRecord := filepath.Join(h.PrimeWeft(), ".git", "fabric-merge.json")
	if _, err := os.Stat(primeRecord); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) = %v; want not-exist — a record in the prime shape would let this test pass without the linked glob", primeRecord, err)
	}

	primeLocation, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
	}

	_, err = h.Topology.Remove(primeLocation, sourceSlug, false)
	var refused *fabricengine.ErrMergeInProgress
	if !errors.As(err, &refused) {
		t.Fatalf("Remove(%s) while a LINKED pair is mid-merge on its branches: error = %v (%T); want *ErrMergeInProgress", sourceSlug, err, err)
	}
	if !fileExistsInWorktree(t, sourceWarpDir, "conflict.txt") {
		t.Errorf("source warp worktree %s was torn down by the refused Remove", sourceWarpDir)
	}
	if !branchExistsLocally(t, h.PrimeWeft(), fabricengine.WeftBranchName(sourceBranch)) {
		t.Errorf("weft branch %q was deleted by the refused Remove; want it intact", fabricengine.WeftBranchName(sourceBranch))
	}

	// force answers dirtiness only, never a live merge record — the same rule as the prime-pair case.
	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true); !errors.As(err, &refused) {
		t.Fatalf("Remove(%s, force=true): error = %v (%T); want *ErrMergeInProgress even with force", sourceSlug, err, err)
	}

	// Aborting the linked pair's merge closes the window, so the same Remove must then succeed.
	if _, err := consumer.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort on the linked consumer pair: %v", err)
	}
	if _, err := h.Topology.Remove(primeLocation, sourceSlug, true); err != nil {
		t.Fatalf("Remove(%s) after MergeAbort: %v; want success — the guard must close a window, not block the pair forever", sourceSlug, err)
	}
}
