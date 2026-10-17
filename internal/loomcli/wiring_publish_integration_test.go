//go:build integration

// wiring_publish_integration_test.go drives Merge-In and then Publish's rejected-push half through wire()'s real seams and the production landingDeps closures over a real fabric pair,
// so the stuck reason carries what the production RemoteOnlyCommits closure reads from the pair's origin.

package loomcli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
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
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "publishrejected"
	hubforge.AddPair(t, hub, slug)
	worktree := hub.PairCodeWorktree(slug)
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
		shedrun.RecipeLoom,
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
		nil,
		nil,
		nil,
		planVerifySource(location),
		parentLockDeps(location, shedrun.SelfRunID, "", ""),
		time.Time{},
	)
	// The pair has no run status file to commit and no plan to read a verify command from.
	deps.CommitStatus = nil
	deps.VerifyCommand = nil
	c.env.Landing = deps

	// The parent moved locally and was never pushed, so Merge-In runs with the push told skipped and reads the local tip alone.
	mergeInDeps := c.env.Landing
	mergeInDeps.PushSkipped = true
	mergeIn, err := landingshed.NewMergeIn(mergeInDeps)
	if err != nil {
		t.Fatalf("NewMergeIn error = %v; want nil", err)
	}
	if outcome, ptr, err := mergeIn.Call(context.Background()); err != nil || outcome != shedengine.Done {
		t.Fatalf("Merge-In Call = %q, %q, %v; want Done", outcome, ptr.Reason, err)
	}

	// Publish tests the parent against both its local and its upstream tip, so the parent's progress is pushed before it runs.
	gitkit.Git(t, hub.PrimeWorktree(), "push", "origin", parentBranch)

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

// TestWireLanding_TellsLanguageSkills asserts wireLanding hands the hub's language skills, as wire loaded them, to the landing deps over a real hub and pair.
func TestWireLanding_TellsLanguageSkills(t *testing.T) {
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "languagelanding"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	boardDir := fabricengine.BoardDir(location.HubPath)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v; want nil", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(boardDir, "language"), []byte("skills:\n  - scribe:golang-testing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(language.yaml) error = %v; want nil", err)
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	if err := c.wireLanding(); err != nil {
		t.Fatalf("wireLanding() = %v; want nil", err)
	}

	if want := []string{"scribe:golang-testing"}; !slices.Equal(c.env.Landing.LanguageSkills, want) {
		t.Errorf("env.Landing.LanguageSkills = %v; want %v", c.env.Landing.LanguageSkills, want)
	}

	seedFile := shedrun.SeedFile(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(seedFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(run directory) error = %v; want nil", err)
	}
	if err := os.WriteFile(seedFile, []byte(`{"recipe":"loom","driver":"go","started_at":"2026-10-01T08:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(seed) error = %v; want nil", err)
	}
	if err := c.wireLanding(); err != nil {
		t.Fatalf("wireLanding() with a seeded start = %v; want nil", err)
	}
	if want := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC); !c.env.Landing.RunStart.Equal(want) {
		t.Errorf("env.Landing.RunStart = %v; want %v", c.env.Landing.RunStart, want)
	}

	if err := os.WriteFile(seedFile, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile(undecodable seed) error = %v; want nil", err)
	}
	err = c.wireLanding()
	if err == nil || !strings.Contains(err.Error(), seedFile) || !strings.Contains(err.Error(), "then lyx shed seed self --recipe loom --driver go --param parent=") || !strings.Contains(err.Error(), "restore the original started_at in the seed") {
		t.Errorf("wireLanding() with an undecodable seed = %v; want an error naming %s, the re-seed way forward and the started_at remedy", err, seedFile)
	}
}
