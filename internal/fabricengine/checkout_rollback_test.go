// checkout_rollback_test.go covers the all-or-nothing contract of Checkout when junction wiring (step 5) fails.
// That failure comes after the weft branch has switched (step 4), so both sides must roll back.
// The tests make the warp _lym a real directory, which seedLymJunction refuses.
// Checkout rolls back both the warp and the weft.
//
// The package's single TestMain lives in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckout_FailureDeletesWeftBranchAdoptedFromOrigin covers a failed Checkout of an origin-only weft branch.
// It deletes the local branch Checkout created to track it, and leaves origin's copy untouched.
// Both sides return to their original branches.
// The failure comes after the adoption, at junction wiring or at the weft switch.
func TestCheckout_FailureDeletesWeftBranchAdoptedFromOrigin(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const markerRel = "origin-marker.txt"

	cases := []struct {
		name   string
		inject func(t *testing.T, l *lymcwd.Location)
	}{
		{"junction wiring fails", func(t *testing.T, l *lymcwd.Location) {
			warpLym := fabricengine.WarpLymLinkHere(l)
			if err := os.Remove(warpLym); err != nil {
				t.Fatalf("remove warp junction to corrupt it: %v", err)
			}
			if err := os.MkdirAll(warpLym, 0o755); err != nil {
				t.Fatalf("create real _lym dir: %v", err)
			}
		}},
		// An untracked file at the path the origin branch tracks passes the pre-switch tracked-dirty check.
		// Git therefore refuses the switch.
		{"weft switch fails", func(t *testing.T, l *lymcwd.Location) {
			path := filepath.Join(fabricengine.RecordsWorktree(l), markerRel)
			if err := os.WriteFile(path, []byte("untracked\n"), 0o644); err != nil {
				t.Fatalf("write untracked file: %v", err)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location

			const targetBranch = "checkout-rollback-origin"
			weftBranch := fabricengine.RecordsBranchName(targetBranch)

			slug := filepath.Base(l.WorktreePath())
			if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
				t.Fatalf("setup WireJunctions: %v", err)
			}
			// Only the warp branch exists locally: the weft branch is on origin alone.
			gitkit.MustRun(t, l.WorktreePath(), "git", "branch", targetBranch)
			clone := t.TempDir()
			mustGit(clone, "clone", "--quiet", h.RecordsBare, ".")
			mustGit(clone, "checkout", "--quiet", "-b", weftBranch)
			gitkit.CommitFile(t, clone, markerRel, "from origin\n", "origin-only weft branch")
			mustGit(clone, "push", "--quiet", "origin", weftBranch)
			originTip := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+weftBranch)

			originalWarpBranch := gitkit.CurrentBranch(t, l.WorktreePath())
			originalWeftBranch := gitkit.CurrentBranch(t, fabricengine.RecordsWorktree(l))

			tc.inject(t, l)

			res, err := h.Topology.Checkout(l, targetBranch)
			if err == nil {
				t.Fatalf("Checkout(%q) error = nil; want the injected failure (res=%+v)", targetBranch, res)
			}

			if got := gitkit.CurrentBranch(t, l.WorktreePath()); got != originalWarpBranch {
				t.Errorf("warp branch after failed Checkout = %q; want %q (original)", got, originalWarpBranch)
			}
			if got := gitkit.CurrentBranch(t, fabricengine.RecordsWorktree(l)); got != originalWeftBranch {
				t.Errorf("weft branch after failed Checkout = %q; want %q (original)", got, originalWeftBranch)
			}
			if gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), weftBranch) {
				t.Errorf("local weft branch %q adopted from origin survived the rollback; want it deleted", weftBranch)
			}
			if got := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+weftBranch); got != originTip {
				t.Errorf("origin %s = %s after the rollback; want %s (unchanged)", weftBranch, got, originTip)
			}
		})
	}
}
