//go:build integration

// add_remoteleftover_integration_test.go covers Add's pre-flight probes of both origins: a live pair's branches on origin are adopted, and a leftover remote branch from a removed pair is refused with an *ErrRemoteLeftover before Add's first mutation, or, when provably replaceable, does not block the add.
//
// Every hub here is built through hubforge.NewHub with an empty branch_prefix, so a slug's warp branch is the bare slug and its weft branch is <slug>-weft.
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
	"github.com/Knatte18/loomyard/internal/hubforge"
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
func requireNothingCreated(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	l := h.Location
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
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
		t.Errorf("local warp branch %q exists after a refused Add", slug)
	}
	if gitkit.BranchExists(t, h.PrimeWeft(), fabricengine.WeftBranchName(slug)) {
		t.Errorf("local weft branch %q exists after a refused Add", fabricengine.WeftBranchName(slug))
	}
}

// removedPair adds slug, plain-removes it, and returns the hub.
func removedPair(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove(%q): %v", slug, err)
	}
	return h
}

// TestAdd_OriginOnlyWeftAdopted covers a weft branch on origin that has moved past its archive tag, with no local weft branch: the pair is live, so Add adopts it.
func TestAdd_OriginOnlyWeftAdopted(t *testing.T) {
	t.Parallel()

	const slug = "leftover-unarchived"
	weftBranch := fabricengine.WeftBranchName(slug)
	h := removedPair(t, slug)
	pushCommitToOrigin(t, h.WeftBare, weftBranch)
	pushedTip := gitkit.RevParse(t, h.WeftBare, weftBranch)

	if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	weft := fabricengine.WeftWorktreePath(h.Location, slug)
	if got := gitkit.Git(t, weft, "rev-parse", "--abbrev-ref", weftBranch+"@{upstream}"); got != "origin/"+weftBranch {
		t.Errorf("upstream of %s = %q; want origin/%s", weftBranch, got, weftBranch)
	}
	if !strings.Contains(strings.Join(gitkit.LsFiles(t, weft), "\n"), "leftover.txt") {
		t.Errorf("adopted weft worktree does not carry the pushed leftover.txt")
	}
	if _, err := gitexec.Run([]string{"merge-base", "--is-ancestor", pushedTip, "refs/heads/" + weftBranch}, h.WeftBare); err != nil {
		t.Errorf("origin weft branch no longer descends from the pushed tip %s: %v", pushedTip, err)
	}
	warpTip := gitkit.RevParse(t, fabricengine.WorktreePath(h.Location, slug), "HEAD")
	if want := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD"); warpTip != want {
		t.Errorf("warp branch = %s; want a fork of the prime's HEAD %s", warpTip, want)
	}
}

// TestAdd_LiveLocalWeftAgainstOrigin covers a local weft branch Add would adopt whose origin copy has moved on.
func TestAdd_LiveLocalWeftAgainstOrigin(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		localCommit  bool
		wantRefusal  bool
		wantAdvanced bool
	}{
		{name: "local behind origin fast-forwards", wantAdvanced: true},
		{name: "local diverged from origin is refused", localCommit: true, wantRefusal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const slug = "leftover-adopt"
			weftBranch := fabricengine.WeftBranchName(slug)
			h := hubforge.NewHub(t, ".")
			topology := h.Topology
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
			oldTip := gitkit.RevParse(t, h.WeftBare, weftBranch)
			if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
				t.Fatalf("setup Remove: %v", err)
			}
			mustGit(h.PrimeWeft(), "branch", weftBranch, oldTip)
			if tc.localCommit {
				seed := t.TempDir() + "/seed"
				mustGit(h.PrimeWeft(), "worktree", "add", seed, weftBranch)
				gitkit.CommitFile(t, seed, "local.txt", "local\n", "local weft work")
				mustGit(h.PrimeWeft(), "worktree", "remove", seed)
			}
			localTip := gitkit.RevParse(t, h.PrimeWeft(), weftBranch)
			pushCommitToOrigin(t, h.WeftBare, weftBranch)
			originTip := gitkit.RevParse(t, h.WeftBare, weftBranch)

			res, err := topology.Add(h.Location, slug, fabricengine.AddOptions{})
			if tc.wantRefusal {
				if err == nil {
					t.Fatalf("Add succeeded; want a leftover refusal")
				}
				requireRemoteLeftover(t, err, weftBranch)
				if n := res.Mutations.Len(); n != 0 {
					t.Errorf("record has %d entries; want empty", n)
				}
				if got := gitkit.RevParse(t, h.PrimeWeft(), weftBranch); got != localTip {
					t.Errorf("local weft branch moved %s -> %s", localTip, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Add: %v", err)
			}

			weft := fabricengine.WeftWorktreePath(h.Location, slug)
			if !strings.Contains(strings.Join(gitkit.LsFiles(t, weft), "\n"), "leftover.txt") {
				t.Errorf("adopted weft worktree does not carry origin's leftover.txt")
			}
			advanced := 0
			for _, m := range res.Mutations.Entries() {
				if m.Kind == fabricengine.KindRepoAdvanced && strings.Contains(m.Detail, originTip) {
					advanced++
				}
			}
			if advanced != 1 {
				t.Errorf("repo_advanced entries naming origin's tip = %d; want 1", advanced)
			}
		})
	}
}

// TestAdd_LiveLocalWeftAdoptsDivergedWarpFromOrigin covers a live pair, by a surviving local weft branch, whose origin warp branch holds work the prime's HEAD lacks, while an archive tag also covers origin's weft tip.
func TestAdd_LiveLocalWeftAdoptsDivergedWarpFromOrigin(t *testing.T) {
	t.Parallel()

	const slug = "leftover-live-warp"
	weftBranch := fabricengine.WeftBranchName(slug)
	h := hubforge.NewHub(t, ".")
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	wt := fabricengine.WorktreePath(h.Location, slug)
	warpTip := gitkit.CommitFile(t, wt, "work.txt", "work\n", "warp work")
	mustGit(wt, "push", "--quiet", "origin", slug)
	weftTip := gitkit.RevParse(t, h.WeftBare, weftBranch)
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}
	requireArchiveCovers(t, h.WeftBare, slug, weftTip)
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
		mustGit(h.PrimeWorktree(), "branch", "-D", slug)
	}
	mustGit(h.PrimeWeft(), "branch", weftBranch, weftTip)

	if _, err := topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if got := gitkit.RevParse(t, wt, "HEAD"); got != warpTip {
		t.Errorf("warp branch = %s; want origin's tip %s", got, warpTip)
	}
	if got := gitkit.Git(t, wt, "rev-parse", "--abbrev-ref", slug+"@{upstream}"); got != "origin/"+slug {
		t.Errorf("upstream of %s = %q; want origin/%s", slug, got, slug)
	}
	if got := gitkit.RevParse(t, h.WarpBare, slug); got != warpTip {
		t.Errorf("origin warp branch moved to %s; want unchanged %s", got, warpTip)
	}
}

// TestAdd_WarpLeftoverRefusedAtPreflight covers a landed warp branch still on origin after its local copy is gone.
func TestAdd_WarpLeftoverRefusedAtPreflight(t *testing.T) {
	t.Parallel()

	const slug = "leftover-warp"
	h := hubforge.NewHub(t, ".")
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	wt := fabricengine.WorktreePath(h.Location, slug)
	gitkit.CommitFile(t, wt, "work.txt", "work\n", "warp work")
	mustGit(wt, "push", "--quiet", "origin", slug)
	mustGit(h.PrimeWorktree(), "merge", "--squash", slug)
	gitkit.Git(t, h.PrimeWorktree(), "commit", "-m", "land "+slug)
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}

	_, err := topology.Add(h.Location, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add succeeded; want a leftover refusal")
	}
	if leftover := requireRemoteLeftover(t, err, slug); leftover.Slug != slug {
		t.Errorf("ErrRemoteLeftover.Slug = %q; want %q", leftover.Slug, slug)
	}
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
		t.Errorf("local warp branch %q exists after a refused Add", slug)
	}
}

// TestAdd_WarpFastForwardableLeftoverProceeds covers an origin warp branch that is an ancestor of the prime's HEAD.
func TestAdd_WarpFastForwardableLeftoverProceeds(t *testing.T) {
	t.Parallel()

	const slug = "leftover-warp-ff"
	h := hubforge.NewHub(t, ".")
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	if _, err := topology.Remove(h.Location, slug, false, true); err != nil {
		t.Fatalf("setup Remove(remote): %v", err)
	}
	gitkit.Git(t, h.PrimeWorktree(), "commit", "--allow-empty", "-m", "advance prime")

	if _, err := topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	newTip := gitkit.RevParse(t, fabricengine.WorktreePath(h.Location, slug), "HEAD")
	out, err := gitexec.Run([]string{"rev-parse", "refs/heads/" + slug}, h.WarpBare)
	if err != nil {
		t.Fatalf("rev-parse origin warp branch: %v", err)
	}
	if got := strings.TrimSpace(out); got != newTip {
		t.Errorf("origin warp branch = %s; want new warp tip %s", got, newTip)
	}
}

// TestAdd_SkipPushSkipsLeftoverProbes covers the origin-only-weft setup re-added under SkipPush: origin is not consulted, so both branches fork.
// Origin's warp branch stays unchanged only because step 11's ungated push re-pushes the same prime HEAD the first Add pushed.
func TestAdd_SkipPushSkipsLeftoverProbes(t *testing.T) {
	t.Parallel()

	const slug = "leftover-skippush"
	weftBranch := fabricengine.WeftBranchName(slug)
	h := removedPair(t, slug)
	pushCommitToOrigin(t, h.WeftBare, weftBranch)
	weftBefore := gitkit.RevParse(t, h.WeftBare, weftBranch)
	warpBefore := gitkit.RevParse(t, h.WarpBare, slug)

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if got := gitkit.RevParse(t, h.WeftBare, weftBranch); got != weftBefore {
		t.Errorf("origin weft branch moved %s -> %s", weftBefore, got)
	}
	if got := gitkit.RevParse(t, h.WarpBare, slug); got != warpBefore {
		t.Errorf("origin warp branch moved %s -> %s", warpBefore, got)
	}
	weft := fabricengine.WeftWorktreePath(h.Location, slug)
	if strings.Contains(strings.Join(gitkit.LsFiles(t, weft), "\n"), "leftover.txt") {
		t.Errorf("weft worktree carries origin's leftover.txt; want a fork that never consulted origin")
	}
	if gitkit.CurrentBranch(t, weft) != weftBranch {
		t.Errorf("weft worktree is not on %s", weftBranch)
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
	h := hubforge.NewHub(t, ".")
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	gitkit.CommitFile(t, fabricengine.WeftWorktreePath(h.Location, slug), "extra.txt", "extra.txt\n", "extra weft work")
	mustGit(fabricengine.WeftWorktreePath(h.Location, slug), "push", "--quiet", "origin", weftBranch)
	oldTip := gitkit.RevParse(t, h.WeftBare, weftBranch)
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}

	res, err := topology.Add(h.Location, slug, fabricengine.AddOptions{})
	if err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	newTip := gitkit.RevParse(t, fabricengine.WeftWorktreePath(h.Location, slug), "HEAD")
	if got := gitkit.RevParse(t, h.WeftBare, weftBranch); got != newTip {
		t.Errorf("origin weft branch = %s; want new local weft tip %s", got, newTip)
	}
	requireArchiveCovers(t, h.WeftBare, slug, oldTip)

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
	h := hubforge.NewHub(t, ".")
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	oldTip := gitkit.RevParse(t, h.WeftBare, weftBranch)
	gitkit.CommitFile(t, fabricengine.WeftWorktreePath(h.Location, slug), "unpushed.txt", "unpushed.txt\n", "extra weft work")
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}
	requireArchiveCovers(t, h.WeftBare, slug, oldTip)

	if _, err := topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	newTip := gitkit.RevParse(t, fabricengine.WeftWorktreePath(h.Location, slug), "HEAD")
	if got := gitkit.RevParse(t, h.WeftBare, weftBranch); got != newTip {
		t.Errorf("origin weft branch = %s; want new local weft tip %s", got, newTip)
	}
}

// TestAdd_WeftReplaceLeaseRaceRefused covers origin's weft branch moving between the pre-flight and the replacement.
// It sets a package hook, so it must not run in parallel.
func TestAdd_WeftReplaceLeaseRaceRefused(t *testing.T) {
	// Serial: SetAddBeforeWeftReplaceHookForTest sets the package-level addBeforeWeftReplaceHook.
	const slug = "leftover-race"
	weftBranch := fabricengine.WeftBranchName(slug)
	h := removedPair(t, slug)

	var moved string
	fabricengine.SetAddBeforeWeftReplaceHookForTest(t, func() {
		pushCommitToOrigin(t, h.WeftBare, weftBranch)
		moved = gitkit.RevParse(t, h.WeftBare, weftBranch)
	})

	topology := h.Topology
	_, err := topology.Add(h.Location, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add succeeded; want a lease failure")
	}
	if !strings.Contains(err.Error(), weftBranch) {
		t.Errorf("message %q does not name branch %q", err.Error(), weftBranch)
	}
	if _, statErr := os.Lstat(fabricengine.WeftWorktreePath(h.Location, slug)); !os.IsNotExist(statErr) {
		t.Errorf("weft worktree remains after the refused Add (stat err = %v)", statErr)
	}
	if moved == "" {
		t.Fatal("the replace hook never ran; want Add to reach the step-12 replacement")
	}
	if got := gitkit.RevParse(t, h.WeftBare, weftBranch); got != moved {
		t.Errorf("origin weft branch %q = %q; want the moved tip %q", weftBranch, got, moved)
	}
}
