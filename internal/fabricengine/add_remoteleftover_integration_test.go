// add_remoteleftover_integration_test.go covers Add's pre-flight probes of both origins:
// a live pair's branches on origin are adopted,
// and a leftover remote branch from a removed pair is refused with an *ErrRemoteLeftover before Add's first mutation.
// A provably replaceable leftover does not block the add.
//
// Every hub here is built through hubforge.CopyHub with an empty branch_prefix,
// so a slug's warp branch is the bare slug and its weft branch is <slug>-weft.
// pushCommitToOrigin plants the leftover's divergence from a throwaway clone of the fixture's bare.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pushCommitToOrigin clones bare, adds one commit to branch, and pushes it back;
// so origin's branch diverges from any local copy.
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

// requireNothingCreated asserts a refused Add left no worktree or launcher and no local branch.
func requireNothingCreated(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	l := h.Location
	for _, p := range []string{
		fabricengine.WorktreePath(l, slug),
		fabricengine.RecordsWorktreePath(l, slug),
		fabricengine.LauncherDir(l, slug),
	} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after a refused Add (stat err = %v)", p, err)
		}
	}
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
		t.Errorf("local warp branch %q exists after a refused Add", slug)
	}
	if gitkit.BranchExists(t, h.PrimeRecords(), fabricengine.RecordsBranchName(slug)) {
		t.Errorf("local weft branch %q exists after a refused Add", fabricengine.RecordsBranchName(slug))
	}
}

// removedPair adds slug, plain-removes it, and returns the hub.
func removedPair(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove(%q): %v", slug, err)
	}
	return h
}

// TestAdd_LiveLocalWeftAgainstOrigin covers a local weft branch Add would adopt whose origin copy has moved on.
//
//testtiming:keep the repo_advanced mutation entry naming origin's tip on a fast-forward, and an empty record with the local weft branch unmoved on a diverged refusal; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_LiveLocalWeftAgainstOrigin(t *testing.T) {
	tiergate.Require(t, 1)
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
			weftBranch := fabricengine.RecordsBranchName(slug)
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			topology := h.Topology
			hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
			oldTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)
			if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
				t.Fatalf("setup Remove: %v", err)
			}
			mustGit(h.PrimeRecords(), "branch", weftBranch, oldTip)
			if tc.localCommit {
				seed := t.TempDir() + "/seed"
				mustGit(h.PrimeRecords(), "worktree", "add", seed, weftBranch)
				gitkit.CommitFile(t, seed, "local.txt", "local\n", "local weft work")
				mustGit(h.PrimeRecords(), "worktree", "remove", seed)
			}
			localTip := gitkit.RevParse(t, h.PrimeRecords(), weftBranch)
			pushCommitToOrigin(t, h.RecordsBare, weftBranch)
			originTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)

			res, err := topology.Add(h.Location, slug, fabricengine.AddOptions{})
			if tc.wantRefusal {
				if err == nil {
					t.Fatalf("Add succeeded; want a leftover refusal")
				}
				requireRemoteLeftover(t, err, weftBranch)
				if n := res.Mutations.Len(); n != 0 {
					t.Errorf("record has %d entries; want empty", n)
				}
				if got := gitkit.RevParse(t, h.PrimeRecords(), weftBranch); got != localTip {
					t.Errorf("local weft branch moved %s -> %s", localTip, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Add: %v", err)
			}

			weft := fabricengine.RecordsWorktreePath(h.Location, slug)
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

// TestAdd_LiveLocalWeftAdoptsDivergedWarpFromOrigin covers a live pair held by a surviving local weft branch;
// its origin warp branch holds work the prime's HEAD lacks, while an archive tag also covers origin's weft tip.
//
//testtiming:keep origin's diverged warp branch left untouched at its tip while the live pair adopts it; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_LiveLocalWeftAdoptsDivergedWarpFromOrigin(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "leftover-live-warp"
	weftBranch := fabricengine.RecordsBranchName(slug)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	wt := fabricengine.WorktreePath(h.Location, slug)
	warpTip := gitkit.CommitFile(t, wt, "work.txt", "work\n", "warp work")
	mustGit(wt, "push", "--quiet", "origin", slug)
	weftTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}
	requireArchiveCovers(t, h.RecordsBare, slug, weftTip)
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
		mustGit(h.PrimeWorktree(), "branch", "-D", slug)
	}
	mustGit(h.PrimeRecords(), "branch", weftBranch, weftTip)

	if _, err := topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if got := gitkit.RevParse(t, wt, "HEAD"); got != warpTip {
		t.Errorf("warp branch = %s; want origin's tip %s", got, warpTip)
	}
	if got := gitkit.Git(t, wt, "rev-parse", "--abbrev-ref", slug+"@{upstream}"); got != "origin/"+slug {
		t.Errorf("upstream of %s = %q; want origin/%s", slug, got, slug)
	}
	if got := gitkit.RevParse(t, h.CodeBare, slug); got != warpTip {
		t.Errorf("origin warp branch moved to %s; want unchanged %s", got, warpTip)
	}
}

// TestAdd_WarpLeftoverRefusedAtPreflight covers a landed warp branch still on origin after its local copy is gone.
func TestAdd_WarpLeftoverRefusedAtPreflight(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "leftover-warp"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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

// TestAdd_SkipPushSkipsLeftoverProbes covers the origin-only-weft setup re-added under SkipPush:
// origin is not consulted, so both branches fork.
// Origin's warp branch stays unchanged, since Add pushes nothing and AddPairWith skips its push under SkipPush.
//
//testtiming:keep both origin branches left unmoved and the leftover weft not deleted under SkipPush; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_SkipPushSkipsLeftoverProbes(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "leftover-skippush"
	weftBranch := fabricengine.RecordsBranchName(slug)
	h := removedPair(t, slug)
	pushCommitToOrigin(t, h.RecordsBare, weftBranch)
	weftBefore := gitkit.RevParse(t, h.RecordsBare, weftBranch)
	warpBefore := gitkit.RevParse(t, h.CodeBare, slug)

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != weftBefore {
		t.Errorf("origin weft branch moved %s -> %s", weftBefore, got)
	}
	if got := gitkit.RevParse(t, h.CodeBare, slug); got != warpBefore {
		t.Errorf("origin warp branch moved %s -> %s", warpBefore, got)
	}
	weft := fabricengine.RecordsWorktreePath(h.Location, slug)
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

// TestAdd_DivergedArchivedWeftLeftoverReplaced covers a pushed weft commit:
// the re-created weft branch does not descend from it.
//
//testtiming:keep the archive tag covering the old pushed weft tip, and the remote_branch_deleted mutation entry for the replaced origin weft branch; coverage of its blocks by other tests does not show an assertion of this
func TestAdd_DivergedArchivedWeftLeftoverReplaced(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "leftover-diverged"
	weftBranch := fabricengine.RecordsBranchName(slug)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	topology := h.Topology
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	gitkit.CommitFile(t, fabricengine.RecordsWorktreePath(h.Location, slug), "extra.txt", "extra.txt\n", "extra weft work")
	mustGit(fabricengine.RecordsWorktreePath(h.Location, slug), "push", "--quiet", "origin", weftBranch)
	oldTip := gitkit.RevParse(t, h.RecordsBare, weftBranch)
	if _, err := topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("setup Remove: %v", err)
	}

	res, err := topology.Add(h.Location, slug, fabricengine.AddOptions{})
	if err != nil {
		t.Fatalf("re-Add: %v", err)
	}
	// Add replaced the leftover itself; the caller's later push lands the new weft branch where it was.
	if gitkit.BranchExists(t, h.RecordsBare, weftBranch) {
		t.Errorf("origin weft branch %q still present after Add; want the archived leftover deleted and nothing pushed", weftBranch)
	}
	var deleted bool
	for _, m := range res.Mutations.Entries() {
		deleted = deleted || (m.Kind == fabricengine.KindRemoteBranchDeleted && m.Target == weftBranch)
	}
	if !deleted {
		t.Errorf("record %+v carries no remote_branch_deleted entry for %q", res.Mutations.Entries(), weftBranch)
	}
	pushNewPair(t, h, slug)
	newTip := gitkit.RevParse(t, fabricengine.RecordsWorktreePath(h.Location, slug), "HEAD")
	if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != newTip {
		t.Errorf("origin weft branch = %s; want new local weft tip %s", got, newTip)
	}
	requireArchiveCovers(t, h.RecordsBare, slug, oldTip)
}

// pushNewPair pushes the pair Add just created for slug in the foreground;
// it stands in for the detached push a production caller spawns.
func pushNewPair(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	if _, err := fabricengine.CoalescePushBothAt(fabricengine.WorktreePath(h.Location, slug), fabricengine.RecordsWorktreePath(h.Location, slug), fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("push the new pair %q: %v", slug, err)
	}
}

// TestAdd_WeftReplaceLeaseRaceRefused covers origin's weft branch moving between the pre-flight and the replacement.
// It sets a package hook, so it must not run in parallel.
func TestAdd_WeftReplaceLeaseRaceRefused(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SetAddBeforeWeftReplaceHookForTest sets the package-level addBeforeWeftReplaceHook.
	const slug = "leftover-race"
	weftBranch := fabricengine.RecordsBranchName(slug)
	h := removedPair(t, slug)

	var moved string
	fabricengine.SetAddBeforeWeftReplaceHookForTest(t, func() {
		pushCommitToOrigin(t, h.RecordsBare, weftBranch)
		moved = gitkit.RevParse(t, h.RecordsBare, weftBranch)
	})

	topology := h.Topology
	_, err := topology.Add(h.Location, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add succeeded; want a lease failure")
	}
	if !strings.Contains(err.Error(), weftBranch) {
		t.Errorf("message %q does not name branch %q", err.Error(), weftBranch)
	}
	if _, statErr := os.Lstat(fabricengine.RecordsWorktreePath(h.Location, slug)); !os.IsNotExist(statErr) {
		t.Errorf("weft worktree remains after the refused Add (stat err = %v)", statErr)
	}
	if moved == "" {
		t.Fatal("the replace hook never ran; want Add to reach the step-12 replacement")
	}
	if got := gitkit.RevParse(t, h.RecordsBare, weftBranch); got != moved {
		t.Errorf("origin weft branch %q = %q; want the moved tip %q", weftBranch, got, moved)
	}
}

// plantStaleTracking plants what a removed pair's push leaves behind:
// branch's remote-tracking ref at tip and its upstream setting in the repository at dir.
func plantStaleTracking(dir, branch, tip string) {
	mustGit(dir, "update-ref", "refs/remotes/origin/"+branch, tip)
	mustGit(dir, "config", "branch."+branch+".remote", "origin")
	mustGit(dir, "config", "branch."+branch+".merge", "refs/heads/"+branch)
}

// requireTrackingStanding asserts that dir's repository holds both or neither of these for branch:
// its remote-tracking ref and its upstream merge setting.
func requireTrackingStanding(t *testing.T, dir, branch string, want bool) {
	t.Helper()

	_, _, refCode, err := gitexec.RunGit([]string{"show-ref", "--verify", "--quiet", "refs/remotes/origin/" + branch}, dir)
	if err != nil {
		t.Fatalf("show-ref in %s: %v", dir, err)
	}
	_, _, configCode, err := gitexec.RunGit([]string{"config", "--get", "branch." + branch + ".merge"}, dir)
	if err != nil {
		t.Fatalf("config --get in %s: %v", dir, err)
	}
	if got := refCode == 0; got != want {
		t.Errorf("remote-tracking ref of %q standing in %s = %v; want %v", branch, dir, got, want)
	}
	if got := configCode == 0; got != want {
		t.Errorf("upstream merge setting of %q standing in %s = %v; want %v", branch, dir, got, want)
	}
}

// staleTrackingHub returns the hub of a removed pair for slug, where origin's branch is deleted on one side;
// that side's repository keeps the tracking ref and upstream setting.
// The local warp branch a removal may leave is deleted, so the next Add forks.
func staleTrackingHub(t *testing.T, slug string, records bool) *hubforge.Hub {
	t.Helper()

	h := removedPair(t, slug)
	if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
		mustGit(h.PrimeWorktree(), "branch", "-D", slug)
	}
	if records {
		weftBranch := fabricengine.RecordsBranchName(slug)
		tip := gitkit.RevParse(t, h.RecordsBare, weftBranch)
		mustGit(h.RecordsBare, "branch", "-D", weftBranch)
		plantStaleTracking(h.PrimeRecords(), weftBranch, tip)
		return h
	}
	tip := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")
	mustGit(h.CodeBare, "branch", "-D", slug)
	plantStaleTracking(h.PrimeWorktree(), slug, tip)
	return h
}

// TestAdd_DropsStaleTrackingRef covers a removed pair's stale remote-tracking ref and upstream setting:
// Add drops them on the side whose origin branch is gone, before it makes anything.
func TestAdd_DropsStaleTrackingRef(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("a weft branch deleted on origin", func(t *testing.T) {
		t.Parallel()

		const slug = "stale-records"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := staleTrackingHub(t, slug, true)
		requireTrackingStanding(t, h.PrimeRecords(), weftBranch, true)

		if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
			t.Fatalf("Add: %v", err)
		}

		requireTrackingStanding(t, h.PrimeRecords(), weftBranch, false)
		report, err := fabricengine.ReconcileOrigin(h.Location, fabricengine.ReconcileScope{Run: slug, JudgeOnly: true}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})
		if err != nil {
			t.Fatalf("ReconcileOrigin: %v", err)
		}
		for _, outcome := range report.Branches {
			if outcome.State == fabricengine.BranchGone && (outcome.Branch == slug || outcome.Branch == weftBranch) {
				t.Errorf("reconcile read the new pair's branch %q as gone from origin", outcome.Branch)
			}
		}
	})

	t.Run("a warp branch deleted on origin", func(t *testing.T) {
		t.Parallel()

		const slug = "stale-warp"
		h := staleTrackingHub(t, slug, false)
		requireTrackingStanding(t, h.PrimeWorktree(), slug, true)

		if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{}); err != nil {
			t.Fatalf("Add: %v", err)
		}

		requireTrackingStanding(t, h.PrimeWorktree(), slug, false)
	})

	t.Run("a drop that fails refuses the Add before anything is made", func(t *testing.T) {
		t.Parallel()

		const slug = "stale-locked"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := staleTrackingHub(t, slug, true)
		commonDir := gitkit.Git(t, h.PrimeRecords(), "rev-parse", "--git-common-dir")
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(h.PrimeRecords(), commonDir)
		}
		if err := os.WriteFile(filepath.Join(commonDir, "config.lock"), nil, 0o644); err != nil {
			t.Fatalf("plant config.lock: %v", err)
		}

		_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{})
		if err == nil {
			t.Fatal("Add succeeded; want the failed drop to refuse it")
		}
		if !strings.Contains(err.Error(), weftBranch) {
			t.Errorf("message %q does not name branch %q", err.Error(), weftBranch)
		}
		requireNothingCreated(t, h, slug)
	})
}
