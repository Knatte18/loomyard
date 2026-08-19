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
