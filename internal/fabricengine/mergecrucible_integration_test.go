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
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
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
