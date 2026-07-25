//go:build integration

// lifecycle_differential_test.go runs warpengine's and fabricengine's Add, Remove,
// Checkout, and List back to back against twin lyxtest fixtures and asserts
// equivalent end state, modulo the one deliberate delta this batch documents:
// fabric's weft branch is always the suffixed sibling of the host branch
// (WeftBranchName), never a mirrored name.
//
// Package fabricengine_test (external test package) because this file imports
// both warpengine and fabricengine; it shares the single TestMain declared in
// testmain_test.go (package fabricengine) per the test-package-discipline
// decision. The fixture-pair builder (buildDiffPair) and the small assertion
// helpers below are written for reuse by reconcile_differential_test.go
// (card 21).

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// diffPair holds one twin-fixture setup for a differential topology case: side A
// drives warpengine against WarpFixture, side B drives fabricengine against
// FabricFixture. Both fixtures start from the same lyxtest template and are
// seeded with their own module's config under the same BranchPrefix.
type diffPair struct {
	WarpFixture   lyxtest.PairedFixture
	FabricFixture lyxtest.PairedFixture
	Warp          *warpengine.Worktree
	Fabric        *fabricengine.Topology
}

// buildDiffPair builds one warp-side and one fabric-side lyxtest.CopyPairedLocal
// fixture, seeds each with its own module's config template under the given
// branchPrefix, and puts the fabric side's weft prime on
// fabricengine.WeftBranchName("main") — mirroring the post-CloneHub state (the
// parallel-build decision's "fabric's clone leaves the weft primary on
// main-weft") so fabric's Add/Checkout fork-from-parent logic has the ref it
// expects to already exist on a fresh fixture. Shared by every differential
// topology case in this file and reused by reconcile_differential_test.go
// (card 21).
func buildDiffPair(t *testing.T, branchPrefix string) diffPair {
	t.Helper()

	warpFixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, warpFixture.WeftPrime, map[string]string{
		"warp": warpengine.ConfigTemplate(),
	})

	fabricFixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fabricFixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})
	// Mirror CloneHub's post-clone state: the weft primary sits on the suffixed
	// sibling of the host's branch, not the host's bare branch name. Without this,
	// a fresh Add/Checkout fork on the fabric side would have no "main-weft" ref to
	// fork from.
	lyxtest.MustRun(t, fabricFixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	return diffPair{
		WarpFixture:   warpFixture,
		FabricFixture: fabricFixture,
		Warp:          warpengine.New(warpengine.Config{BranchPrefix: branchPrefix}),
		Fabric:        fabricengine.NewTopology(fabricengine.Config{BranchPrefix: branchPrefix}),
	}
}

// currentBranchOf returns the branch currently checked out at dir via
// git rev-parse --abbrev-ref HEAD, failing the test on any git error.
func currentBranchOf(t *testing.T, dir string) string {
	t.Helper()

	out, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, dir)
	if err != nil || exitCode != 0 {
		t.Fatalf("rev-parse --abbrev-ref HEAD in %s: err=%v exit=%d", dir, err, exitCode)
	}
	return strings.TrimSpace(out)
}

// branchExistsAt reports whether branch exists as a local ref in the repo at repoRoot.
func branchExistsAt(t *testing.T, repoRoot, branch string) bool {
	t.Helper()

	_, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, repoRoot)
	if err != nil {
		t.Fatalf("rev-parse --verify refs/heads/%s in %s: %v", branch, repoRoot, err)
	}
	return exitCode == 0
}

// assertAddEquivalent asserts that a warp Add and a fabric Add produced equivalent
// end state for the same slug: identical host branch/path-basename/pushed verdict,
// both worktrees present on disk, and a fabric weft branch that is exactly
// WeftBranchName of warp's weft branch.
func assertAddEquivalent(t *testing.T, dp diffPair, slug string, warpRes warpengine.AddResult, fabricRes fabricengine.AddResult) {
	t.Helper()

	if warpRes.Slug != fabricRes.Slug {
		t.Errorf("Slug: warp=%q fabric=%q; want equal", warpRes.Slug, fabricRes.Slug)
	}
	if warpRes.Branch != fabricRes.Branch {
		t.Errorf("Branch: warp=%q fabric=%q; want equal (host branch naming is unchanged by fabric)", warpRes.Branch, fabricRes.Branch)
	}
	if filepath.Base(warpRes.Path) != filepath.Base(fabricRes.Path) {
		t.Errorf("Path basename: warp=%q fabric=%q; want equal", filepath.Base(warpRes.Path), filepath.Base(fabricRes.Path))
	}
	if warpRes.Pushed != fabricRes.Pushed {
		t.Errorf("Pushed: warp=%v fabric=%v; want equal", warpRes.Pushed, fabricRes.Pushed)
	}

	if _, err := os.Stat(warpRes.Path); err != nil {
		t.Errorf("warp host worktree missing: %v", err)
	}
	if _, err := os.Stat(fabricRes.Path); err != nil {
		t.Errorf("fabric host worktree missing: %v", err)
	}

	warpWeftPath := dp.WarpFixture.Layout.WeftWorktreePath(slug)
	fabricWeftPath := dp.FabricFixture.Layout.WeftWorktreePath(slug)
	if _, err := os.Stat(warpWeftPath); err != nil {
		t.Errorf("warp weft worktree missing: %v", err)
	}
	if _, err := os.Stat(fabricWeftPath); err != nil {
		t.Errorf("fabric weft worktree missing: %v", err)
	}

	// The deliberate delta: fabric's weft branch is the suffixed sibling of warp's.
	warpWeftBranch := currentBranchOf(t, warpWeftPath)
	fabricWeftBranch := currentBranchOf(t, fabricWeftPath)
	if want := fabricengine.WeftBranchName(warpWeftBranch); fabricWeftBranch != want {
		t.Errorf("fabric weft branch = %q; want %q (fabricengine.WeftBranchName(%q))", fabricWeftBranch, want, warpWeftBranch)
	}
}

// TestAdd_DifferentialEquivalence covers the fresh-create and
// adopt-existing-weft-branch paths of Add, asserting equivalent end state
// between warpengine and fabricengine.
func TestAdd_DifferentialEquivalence(t *testing.T) {
	t.Run("Fresh", func(t *testing.T) {
		t.Parallel()

		const slug = "diff-add-fresh"
		dp := buildDiffPair(t, "")

		warpRes, err := dp.Warp.Add(dp.WarpFixture.Layout, slug, warpengine.AddOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("warpengine Add: %v", err)
		}
		fabricRes, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("fabricengine Add: %v", err)
		}

		assertAddEquivalent(t, dp, slug, warpRes, fabricRes)
	})

	t.Run("AdoptExistingWeftBranch", func(t *testing.T) {
		t.Parallel()

		const slug = "diff-add-adopt"
		dp := buildDiffPair(t, "")

		// Pre-create each side's weft branch under its own naming scheme so Add
		// routes through the adopt path rather than the create-and-fork path.
		lyxtest.MustRun(t, dp.WarpFixture.Layout.WeftRepoRoot(), "git", "branch", slug, "main")
		lyxtest.MustRun(
			t, dp.FabricFixture.Layout.WeftRepoRoot(),
			"git", "branch", fabricengine.WeftBranchName(slug), fabricengine.WeftBranchName("main"),
		)

		warpRes, err := dp.Warp.Add(dp.WarpFixture.Layout, slug, warpengine.AddOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("warpengine Add (adopt): %v", err)
		}
		fabricRes, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("fabricengine Add (adopt): %v", err)
		}

		assertAddEquivalent(t, dp, slug, warpRes, fabricRes)
	})
}

// assertZeroAddResidue asserts that no host worktree dir, host branch, weft
// worktree dir, or weft branch remain for slug/weftBranch under layout l — the
// rollback contract both Add implementations share.
func assertZeroAddResidue(t *testing.T, label string, l *hubgeometry.Layout, slug, weftBranch string) {
	t.Helper()

	target := l.WorktreePath(slug)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("%s: host worktree dir still exists at %s", label, target)
	}
	if branchExistsAt(t, l.WorktreeRoot, slug) {
		t.Errorf("%s: host branch %q still exists", label, slug)
	}

	weftTarget := l.WeftWorktreePath(slug)
	if _, err := os.Stat(weftTarget); !os.IsNotExist(err) {
		t.Errorf("%s: weft worktree dir still exists at %s", label, weftTarget)
	}
	if branchExistsAt(t, l.WeftRepoRoot(), weftBranch) {
		t.Errorf("%s: weft branch %q still exists", label, weftBranch)
	}
}

// TestAddRollback_DifferentialEquivalence injects the same post-creation failure
// (a blocker file at the portal location) on both sides and asserts both leave
// zero residue — the all-or-nothing rollback contract holds identically for warp
// and fabric.
func TestAddRollback_DifferentialEquivalence(t *testing.T) {
	t.Parallel()

	const slug = "diff-add-rollback"
	dp := buildDiffPair(t, "")

	for _, l := range []*hubgeometry.Layout{dp.WarpFixture.Layout, dp.FabricFixture.Layout} {
		portalLink := filepath.Join(l.PortalsDir(), slug)
		if err := os.MkdirAll(filepath.Dir(portalLink), 0o755); err != nil {
			t.Fatalf("mkdir portal parent: %v", err)
		}
		if err := os.WriteFile(portalLink, []byte("blocker"), 0o644); err != nil {
			t.Fatalf("create blocker: %v", err)
		}
	}

	if _, err := dp.Warp.Add(dp.WarpFixture.Layout, slug, warpengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("warpengine Add should have failed (portal blocker)")
	}
	if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true}); err == nil {
		t.Fatalf("fabricengine Add should have failed (portal blocker)")
	}

	assertZeroAddResidue(t, "warp", dp.WarpFixture.Layout, slug, slug)
	assertZeroAddResidue(t, "fabric", dp.FabricFixture.Layout, slug, fabricengine.WeftBranchName(slug))
}

// TestRemove_DifferentialEquivalence adds then removes the same slug on both
// sides and asserts equivalent LinksRemoved counts and equivalent full teardown
// (host and weft worktree directories both gone).
func TestRemove_DifferentialEquivalence(t *testing.T) {
	t.Parallel()

	const slug = "diff-remove"
	dp := buildDiffPair(t, "")

	if _, err := dp.Warp.Add(dp.WarpFixture.Layout, slug, warpengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup warpengine Add: %v", err)
	}
	if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup fabricengine Add: %v", err)
	}

	warpRes, err := dp.Warp.Remove(dp.WarpFixture.Layout, slug, false)
	if err != nil {
		t.Fatalf("warpengine Remove: %v", err)
	}
	fabricRes, err := dp.Fabric.Remove(dp.FabricFixture.Layout, slug, false)
	if err != nil {
		t.Fatalf("fabricengine Remove: %v", err)
	}

	if warpRes.LinksRemoved != fabricRes.LinksRemoved {
		t.Errorf("LinksRemoved: warp=%d fabric=%d; want equal", warpRes.LinksRemoved, fabricRes.LinksRemoved)
	}
	if filepath.Base(warpRes.Path) != filepath.Base(fabricRes.Path) {
		t.Errorf("Path basename: warp=%q fabric=%q; want equal", filepath.Base(warpRes.Path), filepath.Base(fabricRes.Path))
	}

	for _, target := range []string{warpRes.Path, dp.WarpFixture.Layout.WeftWorktreePath(slug)} {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("warp: %s still exists after Remove; want removed", target)
		}
	}
	for _, target := range []string{fabricRes.Path, dp.FabricFixture.Layout.WeftWorktreePath(slug)} {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Errorf("fabric: %s still exists after Remove; want removed", target)
		}
	}
}

// TestCheckout_DifferentialEquivalence covers checkout onto an existing pair
// branch and checkout onto a host-only branch that forces a weft fork, asserting
// equivalent end state and equivalent fork-point isolation.
func TestCheckout_DifferentialEquivalence(t *testing.T) {
	t.Run("ExistingPairBranch", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		const targetBranch = "diff-checkout-target"

		warpSlug := filepath.Base(dp.WarpFixture.Hub)
		fabricSlug := filepath.Base(dp.FabricFixture.Hub)
		if err := warpengine.WireJunctions(dp.WarpFixture.Layout, warpSlug); err != nil {
			t.Fatalf("warpengine.WireJunctions: %v", err)
		}
		if err := fabricengine.WireJunctions(dp.FabricFixture.Layout, fabricSlug); err != nil {
			t.Fatalf("fabricengine.WireJunctions: %v", err)
		}

		lyxtest.MustRun(t, dp.WarpFixture.Hub, "git", "branch", targetBranch)
		lyxtest.MustRun(t, dp.WarpFixture.Layout.WeftRepoRoot(), "git", "branch", targetBranch)

		lyxtest.MustRun(t, dp.FabricFixture.Hub, "git", "branch", targetBranch)
		lyxtest.MustRun(t, dp.FabricFixture.Layout.WeftRepoRoot(), "git", "branch", fabricengine.WeftBranchName(targetBranch))

		warpRes, err := dp.Warp.Checkout(dp.WarpFixture.Layout, targetBranch)
		if err != nil {
			t.Fatalf("warpengine Checkout: %v", err)
		}
		fabricRes, err := dp.Fabric.Checkout(dp.FabricFixture.Layout, targetBranch)
		if err != nil {
			t.Fatalf("fabricengine Checkout: %v", err)
		}

		if warpRes.Branch != fabricRes.Branch {
			t.Errorf("Branch: warp=%q fabric=%q; want equal", warpRes.Branch, fabricRes.Branch)
		}

		if got := currentBranchOf(t, dp.WarpFixture.Hub); got != targetBranch {
			t.Errorf("warp host branch = %q; want %q", got, targetBranch)
		}
		if got := currentBranchOf(t, dp.FabricFixture.Hub); got != targetBranch {
			t.Errorf("fabric host branch = %q; want %q", got, targetBranch)
		}

		if got := currentBranchOf(t, dp.WarpFixture.Layout.WeftWorktree()); got != targetBranch {
			t.Errorf("warp weft branch = %q; want %q", got, targetBranch)
		}
		if got, want := currentBranchOf(t, dp.FabricFixture.Layout.WeftWorktree()), fabricengine.WeftBranchName(targetBranch); got != want {
			t.Errorf("fabric weft branch = %q; want %q", got, want)
		}
	})

	t.Run("ForkMissingWeftBranch", func(t *testing.T) {
		t.Parallel()

		dp := buildDiffPair(t, "")
		const targetBranch = "diff-checkout-fork"

		warpSlug := filepath.Base(dp.WarpFixture.Hub)
		fabricSlug := filepath.Base(dp.FabricFixture.Hub)
		if err := warpengine.WireJunctions(dp.WarpFixture.Layout, warpSlug); err != nil {
			t.Fatalf("warpengine.WireJunctions: %v", err)
		}
		if err := fabricengine.WireJunctions(dp.FabricFixture.Layout, fabricSlug); err != nil {
			t.Fatalf("fabricengine.WireJunctions: %v", err)
		}

		// Create the target branch on the host only — no corresponding weft branch on
		// either side, forcing switchOrForkWeft's fork path.
		lyxtest.MustRun(t, dp.WarpFixture.Hub, "git", "branch", targetBranch)
		lyxtest.MustRun(t, dp.FabricFixture.Hub, "git", "branch", targetBranch)

		warpRes, err := dp.Warp.Checkout(dp.WarpFixture.Layout, targetBranch)
		if err != nil {
			t.Fatalf("warpengine Checkout (fork): %v", err)
		}
		fabricRes, err := dp.Fabric.Checkout(dp.FabricFixture.Layout, targetBranch)
		if err != nil {
			t.Fatalf("fabricengine Checkout (fork): %v", err)
		}

		if warpRes.Branch != fabricRes.Branch {
			t.Errorf("Branch: warp=%q fabric=%q; want equal", warpRes.Branch, fabricRes.Branch)
		}

		if got := currentBranchOf(t, dp.WarpFixture.Layout.WeftWorktree()); got != targetBranch {
			t.Errorf("warp weft branch = %q; want %q (forked)", got, targetBranch)
		}
		if got, want := currentBranchOf(t, dp.FabricFixture.Layout.WeftWorktree()), fabricengine.WeftBranchName(targetBranch); got != want {
			t.Errorf("fabric weft branch = %q; want %q (forked, suffixed)", got, want)
		}

		// Fork-point isolation: both sides fork from their own pre-switch weft HEAD
		// (main / main-weft respectively), not from an unrelated ancestor — mirrored
		// from weftwiring_test.go's TestWeftForkPointSubtaskIsolation case.
		warpWeftRoot := dp.WarpFixture.Layout.WeftRepoRoot()
		warpMergeBase, _, exitCode, _ := gitexec.RunGit([]string{"merge-base", targetBranch, "main"}, warpWeftRoot)
		if exitCode != 0 {
			t.Fatalf("warp merge-base %s main failed", targetBranch)
		}
		warpMainTip, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "refs/heads/main"}, warpWeftRoot)
		if exitCode != 0 {
			t.Fatalf("warp rev-parse refs/heads/main failed")
		}
		if strings.TrimSpace(warpMergeBase) != strings.TrimSpace(warpMainTip) {
			t.Errorf("warp fork point: merge-base(%s, main) = %s; want %s (main's tip)",
				targetBranch, strings.TrimSpace(warpMergeBase), strings.TrimSpace(warpMainTip))
		}

		fabricWeftRoot := dp.FabricFixture.Layout.WeftRepoRoot()
		fabricWeftMain := fabricengine.WeftBranchName("main")
		fabricMergeBase, _, exitCode, _ := gitexec.RunGit(
			[]string{"merge-base", fabricengine.WeftBranchName(targetBranch), fabricWeftMain}, fabricWeftRoot,
		)
		if exitCode != 0 {
			t.Fatalf("fabric merge-base %s %s failed", fabricengine.WeftBranchName(targetBranch), fabricWeftMain)
		}
		fabricMainTip, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "refs/heads/" + fabricWeftMain}, fabricWeftRoot)
		if exitCode != 0 {
			t.Fatalf("fabric rev-parse refs/heads/%s failed", fabricWeftMain)
		}
		if strings.TrimSpace(fabricMergeBase) != strings.TrimSpace(fabricMainTip) {
			t.Errorf("fabric fork point: merge-base(%s, %s) = %s; want %s (%s's tip)",
				fabricengine.WeftBranchName(targetBranch), fabricWeftMain,
				strings.TrimSpace(fabricMergeBase), strings.TrimSpace(fabricMainTip), fabricWeftMain)
		}
	})
}

// TestList_DifferentialEquivalence adds the same slug on both sides and asserts
// List returns structurally equivalent entries (basename, branch, Main flag) in
// the same order.
func TestList_DifferentialEquivalence(t *testing.T) {
	t.Parallel()

	const slug = "diff-list"
	dp := buildDiffPair(t, "")

	if _, err := dp.Warp.Add(dp.WarpFixture.Layout, slug, warpengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup warpengine Add: %v", err)
	}
	if _, err := dp.Fabric.Add(dp.FabricFixture.Layout, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup fabricengine Add: %v", err)
	}

	warpEntries, err := dp.Warp.List(dp.WarpFixture.Hub)
	if err != nil {
		t.Fatalf("warpengine List: %v", err)
	}
	fabricEntries, err := dp.Fabric.List(dp.FabricFixture.Hub)
	if err != nil {
		t.Fatalf("fabricengine List: %v", err)
	}

	if len(warpEntries) != len(fabricEntries) {
		t.Fatalf("List() len: warp=%d fabric=%d; want equal", len(warpEntries), len(fabricEntries))
	}

	for i := range warpEntries {
		warpBase := filepath.Base(filepath.FromSlash(warpEntries[i].Path))
		fabricBase := filepath.Base(filepath.FromSlash(fabricEntries[i].Path))
		if warpBase != fabricBase {
			t.Errorf("entries[%d] basename: warp=%q fabric=%q; want equal", i, warpBase, fabricBase)
		}
		if warpEntries[i].Branch != fabricEntries[i].Branch {
			t.Errorf("entries[%d].Branch: warp=%q fabric=%q; want equal", i, warpEntries[i].Branch, fabricEntries[i].Branch)
		}
		if warpEntries[i].Main != fabricEntries[i].Main {
			t.Errorf("entries[%d].Main: warp=%v fabric=%v; want equal", i, warpEntries[i].Main, fabricEntries[i].Main)
		}
	}
}
