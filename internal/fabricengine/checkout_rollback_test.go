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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheckout_JunctionFailureRollsBackBothSides wires a healthy primary pair.
// It then turns the warp _lym into a real directory, so the junction wiring fails after the weft has switched.
// It asserts that Checkout errors and that both sides stay on their original branches, never a half-switched pair.
func TestCheckout_JunctionFailureRollsBackBothSides(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	top := h.Topology

	const targetBranch = "checkout-rollback-target"

	// Set up a healthy primary junction and the target branch on both sides.
	// Steps 3 (warp switch) and 4 (weft switch) must succeed, so the failure is isolated to step 5 (junction wiring).
	// Checkout derives the slug the same way (filepath.Base of the worktree root).
	slug := filepath.Base(l.WorktreePath())
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", targetBranch)
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "branch", fabricengine.RecordsBranchName(targetBranch))

	originalWarpBranch := gitkit.CurrentBranch(t, l.WorktreePath())
	originalWeftBranch := gitkit.CurrentBranch(t, fabricengine.RecordsWorktree(l))

	// Corrupt the warp _lym into a real directory: seedLymJunction refuses a real (non-link) _lym.
	// So step 5 fails after step 4 has moved the weft.
	warpLym := fabricengine.WarpLymLinkHere(l)
	if err := os.Remove(warpLym); err != nil {
		t.Fatalf("remove warp junction to corrupt it: %v", err)
	}
	if err := os.MkdirAll(warpLym, 0o755); err != nil {
		t.Fatalf("create real _lym dir: %v", err)
	}

	res, err := top.Checkout(l, targetBranch)
	if err == nil {
		t.Fatalf("Checkout(%q) error = nil; want a junction-wiring failure (res=%+v)", targetBranch, res)
	}

	// Both sides return to their originals.
	// The warp must not roll back while the weft stays on the new branch.
	if got := gitkit.CurrentBranch(t, l.WorktreePath()); got != originalWarpBranch {
		t.Errorf("warp branch after failed Checkout = %q; want %q (original)", got, originalWarpBranch)
	}
	if got := gitkit.CurrentBranch(t, fabricengine.RecordsWorktree(l)); got != originalWeftBranch {
		t.Errorf("weft branch after failed Checkout = %q; want %q (original) — half-switched pair", got, originalWeftBranch)
	}

	// The target weft branch pre-existed this Checkout, so the rollback must leave it in place.
	if !gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), fabricengine.RecordsBranchName(targetBranch)) {
		t.Errorf("pre-existing weft branch %q deleted by rollback; want it untouched", fabricengine.RecordsBranchName(targetBranch))
	}
}

// TestCheckout_JunctionFailureDeletesForkedWeftBranch covers the fork path of a failed Checkout.
// The target has no weft branch yet, so Checkout's step 4 forks one.
// When step 5 then fails, the rollback must restore both sides and delete the branch this Checkout forked,
// or the failed operation leaves an orphan weft branch behind.
func TestCheckout_JunctionFailureDeletesForkedWeftBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	top := h.Topology

	const targetBranch = "checkout-rollback-forked"

	slug := filepath.Base(l.WorktreePath())
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
		t.Fatalf("setup WireJunctions: %v", err)
	}
	// Only the warp branch exists: the weft side must be forked by Checkout.
	gitkit.MustRun(t, l.WorktreePath(), "git", "branch", targetBranch)

	originalWarpBranch := gitkit.CurrentBranch(t, l.WorktreePath())
	originalWeftBranch := gitkit.CurrentBranch(t, fabricengine.RecordsWorktree(l))

	// Corrupt the warp _lym into a real directory so step 5 fails after the fork.
	warpLym := fabricengine.WarpLymLinkHere(l)
	if err := os.Remove(warpLym); err != nil {
		t.Fatalf("remove warp junction to corrupt it: %v", err)
	}
	if err := os.MkdirAll(warpLym, 0o755); err != nil {
		t.Fatalf("create real _lym dir: %v", err)
	}

	res, err := top.Checkout(l, targetBranch)
	if err == nil {
		t.Fatalf("Checkout(%q) error = nil; want a junction-wiring failure (res=%+v)", targetBranch, res)
	}

	if got := gitkit.CurrentBranch(t, l.WorktreePath()); got != originalWarpBranch {
		t.Errorf("warp branch after failed Checkout = %q; want %q (original)", got, originalWarpBranch)
	}
	if got := gitkit.CurrentBranch(t, fabricengine.RecordsWorktree(l)); got != originalWeftBranch {
		t.Errorf("weft branch after failed Checkout = %q; want %q (original) — half-switched pair", got, originalWeftBranch)
	}

	// The branch step 4 forked must be gone: the rolled-back Checkout tears down
	// exactly what it created.
	forked := fabricengine.RecordsBranchName(targetBranch)
	if gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), forked) {
		t.Errorf("forked weft branch %q survived the rollback; want it deleted (orphan branch stranded by fabric's own failed operation)", forked)
	}
}

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

// TestCheckout_WarpSwitchFailureCarriesGitStderr proves that a failing warp switch reports git's own explanation.
// The usual cause is a branch already checked out in another worktree.
// A bare exit code such as "git exit 128" gives the operator nothing to act on.
func TestCheckout_WarpSwitchFailureCarriesGitStderr(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: t.Setenv("FABRIC_SKIP_PUSH") sets a process-global variable.
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	const slug = "checkout-stderr"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	warpLayout, err := lymcwd.Resolve(fabricengine.WorktreePath(l, slug))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(warp): %v", err)
	}

	// The prime worktree holds primeBranch, so switching the task worktree onto it must fail.
	primeBranch := gitkit.CurrentBranch(t, l.WorktreePath())
	_, err = topology.Checkout(warpLayout, primeBranch)
	if err == nil {
		t.Fatalf("Checkout(%q) = nil error; want a refusal (the branch is checked out elsewhere)", primeBranch)
	}
	if !strings.Contains(err.Error(), primeBranch) || !strings.Contains(err.Error(), "worktree") {
		t.Errorf("Checkout error = %q; want it to carry git's own explanation naming the branch and the other worktree", err.Error())
	}
}
