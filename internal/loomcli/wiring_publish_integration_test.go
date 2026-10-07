//go:build integration

// wiring_publish_integration_test.go drives Publish's rejected-push half through wire()'s real seams and the production landingDeps closures over a real fabric pair,
// so the stuck reason carries what the production RemoteOnlyCommits closure reads from the pair's origin.

package loomcli

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestWire_Real_PublishRejectedPushNamesRemoteTip asserts a Publish built from the production landingDeps halts at the rejected push with the real remote tip and a count of 1 in its reason.
// The remote task branch holds a commit the local branch lacks, so the halt precedes any GitHub call.
func TestWire_Real_PublishRejectedPushNamesRemoteTip(t *testing.T) {
	hub := hubforge.NewHub(t, ".")
	const slug = "publishrejected"
	hubforge.AddPair(t, hub, slug)
	worktree := hub.PairWarpWorktree(slug)
	location, err := lyxcwd.ResolveWorktree(worktree)
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	handle, err := fabricengine.Open(location)
	if err != nil {
		t.Fatalf("fabricengine.Open error = %v; want nil", err)
	}
	taskBranch, parentBranch, err := landingBranches(handle, location)
	if err != nil {
		t.Fatalf("landingBranches error = %v; want nil", err)
	}

	// The remote task branch gets a commit the local one never sees: push it, then rewind the local branch.
	base := gitkit.RevParse(t, worktree, "HEAD")
	remoteOnly := gitkit.CommitFile(t, worktree, "remote-only.txt", "remote only\n", "task branch: pushed from elsewhere")
	gitkit.Git(t, worktree, "push", "origin", "HEAD:refs/heads/"+taskBranch)
	gitkit.Git(t, worktree, "reset", "--hard", base)
	// The parent advances too, so the merge-in lands a commit the remote task branch lacks and the push cannot fast-forward.
	gitkit.CommitFile(t, hub.PrimeWorktree(), "parent-progress.txt", "parent progress\n", "parent: progress")

	originURL, err := handle.OriginURL()
	if err != nil {
		t.Fatalf("OriginURL error = %v; want nil", err)
	}
	syncOpts := fabricengine.EnvSyncOptions()
	deps := landingDeps(
		location,
		c.runDeps.Geom,
		taskBranch,
		originURL,
		parentBranch,
		syncOpts.SkipPush,
		func() error {
			_, err := handle.PushBranch(syncOpts)
			return err
		},
		c.registry,
		&shuttleengine.Runner{},
		landingshed.Config{RequirePRToBase: []string{parentBranch}, Conflict: "claude:test-model", ConflictTimeoutMin: 1},
		"",
	)
	// The pair has no run status file to commit and no plan to read a verify command from.
	deps.CommitStatus = nil
	deps.VerifyCommand = nil
	c.env.Landing = deps

	publish, err := landingshed.NewPublish(c.env.Landing)
	if err != nil {
		t.Fatalf("NewPublish error = %v; want nil", err)
	}
	outcome, ptr, err := publish.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Publish Call = %q, %q, %v; want Stuck", outcome, ptr.Reason, err)
	}
	want := "push rejected by the remote after the merge-in against parent branch \"" + parentBranch + "\"; the remote task branch is at " + remoteOnly +
		" and holds 1 commit(s) the local branch lacks; way forward: run `git merge origin/" + taskBranch + "` in the task worktree, then resume the run with `lyx loom start`"
	if ptr.Reason != want {
		t.Errorf("stuck reason = %q; want %q", ptr.Reason, want)
	}
}
