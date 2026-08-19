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
	"errors"
	"os"
	"path/filepath"
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

// TestMergeCrucible_DetachedHeadRefused pins finding F2: a merge verb must refuse while either
// checkout has HEAD pointing straight at a commit rather than at a branch.
// Without the guard, MergeIn reported full success on a detached warp HEAD, landed a warp merge
// commit no ref reaches, landed the weft merge commit permanently, and deleted its own record — so
// the warp half vanished at the next checkout with the weft half already final and no longer
// abortable.
// The table drives both sides, since the guard is aggregated and must fire whichever side is
// detached.
func TestMergeCrucible_DetachedHeadRefused(t *testing.T) {
	tests := []struct {
		name       string
		detachWeft bool
	}{
		{name: "WarpDetached", detachWeft: false},
		{name: "WeftDetached", detachWeft: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
			commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
			commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

			detachDir := h.PrimeWorktree()
			if tt.detachWeft {
				detachDir = h.PrimeWeft()
			}
			gitkit.MustRun(t, detachDir, "git", "checkout", "-q", "--detach", "HEAD")

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
		})
	}
}

// TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides pins finding F1: a record whose
// attempt never reached one side must refuse MergeContinue outright, before anything lands.
// The reconstructed state is byte-for-byte what a kill between the two MergeStart calls leaves —
// merge.go persists WarpOutcome only after the warp MergeStart returns, so WeftOutcome is still
// empty at that instant. Without the guard, MergeContinue committed the warp side, then failed
// concluding a weft side that was never started, returned "run MergeContinue again" (an instruction
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
