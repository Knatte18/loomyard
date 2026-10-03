//go:build integration

// add_remoteleftover_integration_test.go covers Add's pre-flight probes of both origins: a leftover remote branch from a removed pair is refused with an *ErrRemoteLeftover before Add's first mutation, or, when provably replaceable, does not block the add.
//
// Every hub here is built through newFabricFixture with an empty branch_prefix, so a slug's warp branch is the bare slug and its weft branch is <slug>-weft.
// pushCommitToOrigin plants the leftover's divergence from a throwaway clone of the fixture's bare.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// pushCommitToOrigin clones bare, adds one commit to branch, and pushes it back, so origin's branch diverges from any local copy.
func pushCommitToOrigin(t *testing.T, bare, branch string) {
	t.Helper()

	clone := t.TempDir()
	mustGit(clone, "clone", "--quiet", bare, ".")
	mustGit(clone, "checkout", "--quiet", branch)
	gitkit.CommitFile(t, clone, "leftover.txt", "leftover\n", "leftover work")
	mustGit(clone, "push", "--quiet", "origin", branch)
}

// requireRemoteLeftover asserts err is an *ErrRemoteLeftover for branch and returns it.
func requireRemoteLeftover(t *testing.T, err error, branch string) *fabricengine.ErrRemoteLeftover {
	t.Helper()

	var leftover *fabricengine.ErrRemoteLeftover
	if !errors.As(err, &leftover) {
		t.Fatalf("Add error = %v; want *ErrRemoteLeftover", err)
	}
	if leftover.Branch != branch {
		t.Errorf("ErrRemoteLeftover.Branch = %q; want %q", leftover.Branch, branch)
	}
	if !strings.Contains(err.Error(), branch) {
		t.Errorf("message %q does not name branch %q", err.Error(), branch)
	}
	return leftover
}

// requireNothingCreated asserts a refused Add left no worktree, portal or launcher and no local branch.
func requireNothingCreated(t *testing.T, f fabricFixture, slug string) {
	t.Helper()

	l := f.Layout
	for _, p := range []string{
		fabricengine.WorktreePath(l, slug),
		fabricengine.WeftWorktreePath(l, slug),
		fabricengine.PortalLink(l, slug),
		fabricengine.LauncherDir(l, slug),
	} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after a refused Add (stat err = %v)", p, err)
		}
	}
	if gitkit.BranchExists(t, f.Hub, slug) {
		t.Errorf("local warp branch %q exists after a refused Add", slug)
	}
	if gitkit.BranchExists(t, f.WeftPrime, fabricengine.WeftBranchName(slug)) {
		t.Errorf("local weft branch %q exists after a refused Add", fabricengine.WeftBranchName(slug))
	}
}

// removedPair adds slug, plain-removes it, and returns the fixture.
func removedPair(t *testing.T, slug string) fabricFixture {
	t.Helper()

	f := newFabricFixture(t)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}
	if _, err := topology.Remove(f.Layout, slug, false, false); err != nil {
		t.Fatalf("setup Remove(%q): %v", slug, err)
	}
	return f
}

// TestAdd_UnarchivedWeftLeftoverRefusedAtPreflight covers a weft branch on origin that has moved past its archive tag.
func TestAdd_UnarchivedWeftLeftoverRefusedAtPreflight(t *testing.T) {
	t.Parallel()

	const slug = "leftover-unarchived"
	weftBranch := fabricengine.WeftBranchName(slug)
	f := removedPair(t, slug)
	pushCommitToOrigin(t, f.WeftBare, weftBranch)
	before := gitkit.RevParse(t, f.WeftBare, weftBranch)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	res, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add succeeded; want a leftover refusal")
	}
	requireRemoteLeftover(t, err, weftBranch)
	for _, want := range []string{"lyx fabric remove " + slug, "git push origin --delete " + weftBranch} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks remedy %q", err.Error(), want)
		}
	}
	if n := res.Mutations.Len(); n != 0 {
		t.Errorf("record has %d entries; want empty", n)
	}
	requireNothingCreated(t, f, slug)
	if after := gitkit.RevParse(t, f.WeftBare, weftBranch); after != before {
		t.Errorf("origin weft branch moved %s -> %s", before, after)
	}
}

// TestAdd_AdoptedWeftDivergedRefusedAtPreflight covers a local weft branch Add would adopt whose origin copy has moved on.
func TestAdd_AdoptedWeftDivergedRefusedAtPreflight(t *testing.T) {
	t.Parallel()

	const slug = "leftover-adopt"
	weftBranch := fabricengine.WeftBranchName(slug)
	f := newFabricFixture(t)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	oldTip := gitkit.RevParse(t, f.WeftBare, weftBranch)
	if _, err := topology.Remove(f.Layout, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}
	mustGit(f.WeftPrime, "branch", weftBranch, oldTip)
	pushCommitToOrigin(t, f.WeftBare, weftBranch)

	res, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add succeeded; want a leftover refusal")
	}
	requireRemoteLeftover(t, err, weftBranch)
	if n := res.Mutations.Len(); n != 0 {
		t.Errorf("record has %d entries; want empty", n)
	}
}

// TestAdd_WarpLeftoverRefusedAtPreflight covers a landed warp branch still on origin after its local copy is gone.
func TestAdd_WarpLeftoverRefusedAtPreflight(t *testing.T) {
	t.Parallel()

	const slug = "leftover-warp"
	f := newFabricFixture(t)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	wt := fabricengine.WorktreePath(f.Layout, slug)
	gitkit.CommitFile(t, wt, "work.txt", "work\n", "warp work")
	mustGit(wt, "push", "--quiet", "origin", slug)
	mustGit(f.Hub, "merge", "--squash", slug)
	gitkit.Git(t, f.Hub, "commit", "-m", "land "+slug)
	if _, err := topology.Remove(f.Layout, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}

	_, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add succeeded; want a leftover refusal")
	}
	if leftover := requireRemoteLeftover(t, err, slug); leftover.Slug != slug {
		t.Errorf("ErrRemoteLeftover.Slug = %q; want %q", leftover.Slug, slug)
	}
	if gitkit.BranchExists(t, f.Hub, slug) {
		t.Errorf("local warp branch %q exists after a refused Add", slug)
	}
}

// TestAdd_WarpFastForwardableLeftoverProceeds covers an origin warp branch that is an ancestor of the prime's HEAD.
func TestAdd_WarpFastForwardableLeftoverProceeds(t *testing.T) {
	t.Parallel()

	const slug = "leftover-warp-ff"
	f := newFabricFixture(t)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	if _, err := topology.Remove(f.Layout, slug, false, true); err != nil {
		t.Fatalf("setup Remove(remote): %v", err)
	}
	gitkit.Git(t, f.Hub, "commit", "--allow-empty", "-m", "advance prime")

	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	newTip := gitkit.RevParse(t, fabricengine.WorktreePath(f.Layout, slug), "HEAD")
	out, err := gitexec.Run([]string{"rev-parse", "refs/heads/" + slug}, f.Bare)
	if err != nil {
		t.Fatalf("rev-parse origin warp branch: %v", err)
	}
	if got := strings.TrimSpace(out); got != newTip {
		t.Errorf("origin warp branch = %s; want new warp tip %s", got, newTip)
	}
}

// TestAdd_SkipPushSkipsLeftoverProbes covers the unarchived-leftover setup re-added under SkipPush.
// Origin's warp branch stays unchanged only because step 11's ungated push re-pushes the same prime HEAD the first Add pushed.
func TestAdd_SkipPushSkipsLeftoverProbes(t *testing.T) {
	t.Parallel()

	const slug = "leftover-skippush"
	weftBranch := fabricengine.WeftBranchName(slug)
	f := removedPair(t, slug)
	pushCommitToOrigin(t, f.WeftBare, weftBranch)
	weftBefore := gitkit.RevParse(t, f.WeftBare, weftBranch)
	warpBefore := gitkit.RevParse(t, f.Bare, slug)

	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(SkipPush): %v", err)
	}
	if got := gitkit.RevParse(t, f.WeftBare, weftBranch); got != weftBefore {
		t.Errorf("origin weft branch moved %s -> %s", weftBefore, got)
	}
	if got := gitkit.RevParse(t, f.Bare, slug); got != warpBefore {
		t.Errorf("origin warp branch moved %s -> %s", warpBefore, got)
	}
}

// requireArchiveCovers asserts an archive/<slug>/* tag on the weft bare targets oldTip or a descendant of it.
func requireArchiveCovers(t *testing.T, weftBare, slug, oldTip string) {
	t.Helper()

	out, err := gitexec.Run([]string{"for-each-ref", "--format=%(refname)", "refs/tags/archive/" + slug + "/"}, weftBare)
	if err != nil {
		t.Fatalf("list archive tags: %v", err)
	}
	for _, ref := range strings.Fields(out) {
		if _, err := gitexec.Run([]string{"merge-base", "--is-ancestor", oldTip, ref}, weftBare); err == nil {
			return
		}
	}
	t.Errorf("no archive/%s/* tag covers old tip %s (tags: %q)", slug, oldTip, out)
}

// TestAdd_DivergedArchivedWeftLeftoverReplaced covers a pushed weft commit that the re-created weft branch does not descend from.
func TestAdd_DivergedArchivedWeftLeftoverReplaced(t *testing.T) {
	t.Parallel()

	const slug = "leftover-diverged"
	weftBranch := fabricengine.WeftBranchName(slug)
	f := newFabricFixture(t)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	gitkit.CommitFile(t, fabricengine.WeftWorktreePath(f.Layout, slug), "extra.txt", "extra.txt\n", "extra weft work")
	mustGit(fabricengine.WeftWorktreePath(f.Layout, slug), "push", "--quiet", "origin", weftBranch)
	oldTip := gitkit.RevParse(t, f.WeftBare, weftBranch)
	if _, err := topology.Remove(f.Layout, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}

	res, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{})
	if err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	newTip := gitkit.RevParse(t, fabricengine.WeftWorktreePath(f.Layout, slug), "HEAD")
	if got := gitkit.RevParse(t, f.WeftBare, weftBranch); got != newTip {
		t.Errorf("origin weft branch = %s; want new local weft tip %s", got, newTip)
	}
	requireArchiveCovers(t, f.WeftBare, slug, oldTip)

	deleted, pushed := -1, -1
	for i, m := range res.Mutations.Entries() {
		switch {
		case m.Kind == fabricengine.KindRemoteBranchDeleted && m.Target == weftBranch:
			deleted = i
		case m.Kind == fabricengine.KindBranchPushed && m.Target == weftBranch:
			pushed = i
		}
	}
	if deleted < 0 || pushed < 0 || deleted > pushed {
		t.Errorf("record order: remote_branch_deleted at %d, weft branch_pushed at %d; want deleted before pushed", deleted, pushed)
	}
}

// TestAdd_ArchivedAncestorWeftLeftoverReplaced covers an archive tag on an unpushed commit that is a strict descendant of origin's weft tip.
func TestAdd_ArchivedAncestorWeftLeftoverReplaced(t *testing.T) {
	t.Parallel()

	const slug = "leftover-ancestor"
	weftBranch := fabricengine.WeftBranchName(slug)
	f := newFabricFixture(t)
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}
	oldTip := gitkit.RevParse(t, f.WeftBare, weftBranch)
	gitkit.CommitFile(t, fabricengine.WeftWorktreePath(f.Layout, slug), "unpushed.txt", "unpushed.txt\n", "extra weft work")
	if _, err := topology.Remove(f.Layout, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}
	requireArchiveCovers(t, f.WeftBare, slug, oldTip)

	if _, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	newTip := gitkit.RevParse(t, fabricengine.WeftWorktreePath(f.Layout, slug), "HEAD")
	if got := gitkit.RevParse(t, f.WeftBare, weftBranch); got != newTip {
		t.Errorf("origin weft branch = %s; want new local weft tip %s", got, newTip)
	}
}

// TestAdd_WeftReplaceLeaseRaceRefused covers origin's weft branch moving between the pre-flight and the replacement.
// It sets a package hook, so it must not run in parallel.
func TestAdd_WeftReplaceLeaseRaceRefused(t *testing.T) {
	const slug = "leftover-race"
	weftBranch := fabricengine.WeftBranchName(slug)
	f := removedPair(t, slug)

	var moved string
	fabricengine.SetAddBeforeWeftReplaceHookForTest(t, func() {
		pushCommitToOrigin(t, f.WeftBare, weftBranch)
		moved = gitkit.RevParse(t, f.WeftBare, weftBranch)
	})

	topology := fabricengine.NewTopology(fabricengine.Config{})
	_, err := topology.Add(f.Layout, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add succeeded; want a lease failure")
	}
	if !strings.Contains(err.Error(), weftBranch) {
		t.Errorf("message %q does not name branch %q", err.Error(), weftBranch)
	}
	if _, statErr := os.Lstat(fabricengine.WeftWorktreePath(f.Layout, slug)); !os.IsNotExist(statErr) {
		t.Errorf("weft worktree remains after the refused Add (stat err = %v)", statErr)
	}
	if moved == "" {
		t.Fatal("the replace hook never ran; want Add to reach the step-12 replacement")
	}
	if got := gitkit.RevParse(t, f.WeftBare, weftBranch); got != moved {
		t.Errorf("origin weft branch %q = %q; want the moved tip %q", weftBranch, got, moved)
	}
}
