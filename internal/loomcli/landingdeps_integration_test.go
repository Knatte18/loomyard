//go:build tier1

// landingdeps_integration_test.go drives landingDeps' config-change seams over a real hub: ConfigChanges reads a pair's committed config files through real branches,
// and Notify posts creel mail to the prime's orch.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestLandingDeps_ConfigNoticeSeamsOverRealHub asserts ConfigChanges reports a per-worktree module's changed config file and never a hub-wide module's, that Notify posts nothing without a recorded orch strand and fails with the notice line once one is recorded, and that CommitParentRecords commits the parent's run records alone, and nothing while it holds none.
// The steps share one hub and one prime orch state,
// and the queueing step relies on the no-strand step having queued nothing,
// so none runs in parallel.
func TestLandingDeps_ConfigNoticeSeamsOverRealHub(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task")
	taskRecords := h.PairRecordsSibling("task")
	gitkit.CommitFile(t, taskRecords, configengine.ConfigFileRel("loom"), "task_setting: changed\n", "task: change loom config")
	gitkit.CommitFile(t, taskRecords, configengine.ConfigFileRel("board"), "task_setting: changed\n", "task: change board config")

	taskLocation, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("task"))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(task) error = %v; want nil", err)
	}
	deps := landingDeps(taskLocation, shedrun.RecipeLoom, websterengine.Geometry{}, "task", "main",
		true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(taskLocation), parentLockDeps(taskLocation, shedrun.SelfRunID, "", ""))

	t.Run("ConfigChanges reports the per-worktree file and not the hub-wide one", func(t *testing.T) {
		changes, err := deps.ConfigChanges()
		if err != nil {
			t.Fatalf("ConfigChanges() error = %v; want nil", err)
		}
		want := filepath.ToSlash(configengine.ConfigFileRel("loom"))
		if len(changes.Files) != 1 || changes.Files[0] != want {
			t.Errorf("ConfigChanges().Files = %q; want exactly [%q]", changes.Files, want)
		}
	})

	paths := orchcli.PrimePaths(h.Location)
	noticeCount := func() int {
		entries, err := os.ReadDir(paths.NoticesDir)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("ReadDir(%q) error = %v; want nil", paths.NoticesDir, err)
		}
		return len(entries)
	}

	if !t.Run("Notify with no orch strand recorded returns nil and writes no notice file", func(t *testing.T) {
		if err := deps.Notify("loom: a notice"); err != nil {
			t.Fatalf("Notify() error = %v; want nil", err)
		}
		if got := noticeCount(); got != 0 {
			t.Errorf("notice files = %d; want 0 without an orch strand", got)
		}
	}) {
		return
	}

	t.Run("Notify with an orch strand recorded returns an error holding the notice line", func(t *testing.T) {
		if err := orchengine.SaveState(paths, orchengine.State{Strand: "orch-strand-guid"}); err != nil {
			t.Fatalf("SaveState() error = %v; want nil", err)
		}
		// No creel daemon answers under go test, so the post fails and the error carries the line.
		err := deps.Notify("loom: a notice")
		if err == nil || !strings.Contains(err.Error(), "loom: a notice") {
			t.Fatalf("Notify() error = %v; want an error holding the notice line", err)
		}
		if got := noticeCount(); got != 0 {
			t.Errorf("notice files = %d; want 0", got)
		}
	})

	primeRecords := h.PrimeRecords()
	headBefore := gitkit.Git(t, primeRecords, "rev-parse", "HEAD")
	if !t.Run("CommitParentRecords commits nothing while the parent holds no run record", func(t *testing.T) {
		if err := deps.CommitParentRecords(); err != nil {
			t.Fatalf("CommitParentRecords() error = %v; want nil", err)
		}
		if got := gitkit.Git(t, primeRecords, "rev-parse", "HEAD"); got != headBefore {
			t.Errorf("parent HEAD = %s, was %s; want no commit", got, headBefore)
		}
	}) {
		return
	}

	t.Run("CommitParentRecords commits the parent's run records and leaves a dirty path elsewhere alone", func(t *testing.T) {
		record := filepath.Join(primeRecords, shedrun.RunsRootRel(), "sibling", "status.json")
		if err := os.MkdirAll(filepath.Dir(record), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(record, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		elsewhere := filepath.Join(primeRecords, "elsewhere.txt")
		if err := os.WriteFile(elsewhere, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := deps.CommitParentRecords(); err != nil {
			t.Fatalf("CommitParentRecords() error = %v; want nil", err)
		}
		if got := gitkit.Git(t, primeRecords, "status", "--porcelain"); got != "?? elsewhere.txt" {
			t.Errorf("parent status = %q; want only the path outside the run records left dirty", got)
		}
		want := "loom: run records checkpoint for landing " + taskLocation.WorktreeName
		if got := gitkit.Git(t, primeRecords, "log", "-1", "--format=%s"); got != want {
			t.Errorf("parent commit subject = %q; want %q, naming the landing slug", got, want)
		}
	})
}

// TestParentRecordsCommitter_CommitsTheRecordedParent shows the notice walk does not reach Finalize's parent-records commit:
// with member m forked from beam b, whose orch is stopped, the committer commits a run-records file left in b's records and leaves the prime's records untouched.
func TestParentRecordsCommitter_CommitsTheRecordedParent(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "m", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	member, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("m"))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(m) error = %v; want nil", err)
	}
	beamRecords, primeRecords := h.PairRecordsSibling("b"), h.PrimeRecords()
	record := filepath.Join(beamRecords, shedrun.RunsRootRel(), "sibling", "status.json")
	if err := os.MkdirAll(filepath.Dir(record), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	primeHead := gitkit.Git(t, primeRecords, "rev-parse", "HEAD")

	if err := parentRecordsCommitter(member, "m")(); err != nil {
		t.Fatalf("parentRecordsCommitter(m)() error = %v; want nil", err)
	}

	if got := gitkit.Git(t, beamRecords, "status", "--porcelain"); got != "" {
		t.Errorf("beam records status = %q; want the run record committed", got)
	}
	want := "loom: run records checkpoint for landing m"
	if got := gitkit.Git(t, beamRecords, "log", "-1", "--format=%s"); got != want {
		t.Errorf("beam records commit subject = %q; want %q", got, want)
	}
	if got := gitkit.Git(t, primeRecords, "rev-parse", "HEAD"); got != primeHead {
		t.Errorf("prime records HEAD = %s, was %s; want no commit", got, primeHead)
	}
}

// TestLandingDeps_PullRequestModeReadsTheParent asserts the PR-Review row's ParentMode seam reads the receiving branch's beam.yaml and never the task worktree's own:
// a task under the prime reads the prime's, a member under a beam reads the beam's, and the beam reads the prime's.
// It also asserts EntryField reads the run's own board entry.
func TestLandingDeps_PullRequestModeReadsTheParent(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	boardLayer := configengine.ConfigFile(h.BoardDir(), "beam")
	if err := os.MkdirAll(filepath.Dir(boardLayer), 0o755); err != nil {
		t.Fatalf("create the board's beam config directory: %v", err)
	}
	if err := os.WriteFile(boardLayer, []byte(dobbyengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write the board's beam.yaml: %v", err)
	}
	hubforge.AddPair(t, h, "task")
	hubforge.AddPairWith(t, h, "beam", fabricengine.AddOptions{Beam: true})
	hubforge.AddPairWith(t, h, "member", fabricengine.AddOptions{Parent: "beam"})

	locate := func(codeWorktree string) *lymcwd.Location {
		t.Helper()
		location, err := lymcwd.ResolveWorktree(codeWorktree)
		if err != nil {
			t.Fatalf("lymcwd.ResolveWorktree(%s) error = %v; want nil", codeWorktree, err)
		}
		return location
	}
	setMode := func(location *lymcwd.Location, mode string) {
		t.Helper()
		layer := configengine.ConfigFile(location.AnchorPath(), "beam")
		if err := os.MkdirAll(filepath.Dir(layer), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(layer), err)
		}
		if err := os.WriteFile(layer, []byte("pull_request: \""+mode+"\"\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", layer, err)
		}
	}
	depsOf := func(location *lymcwd.Location) landingshed.Deps {
		return landingDeps(location, shedrun.RecipeLoom, websterengine.Geometry{}, location.WorktreeName, "main",
			true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(location), parentLockDeps(location, shedrun.SelfRunID, "", ""))
	}

	prime, task, beam, member := h.Location, locate(h.PairCodeWorktree("task")), locate(h.PairCodeWorktree("beam")), locate(h.PairCodeWorktree("member"))
	setMode(prime, "off")
	setMode(task, "required")
	setMode(beam, "required")
	board, err := boardengine.OpenHub(h.Location.HubPath)
	if err != nil {
		t.Fatalf("boardengine.OpenHub() error = %v; want nil", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": "beam", "title": "Beam", "kind": "project", "labels": []string{"bug"}, "pull_request": "on"}); err != nil {
		t.Fatalf("UpsertTask(beam) error = %v; want nil", err)
	}

	tests := []struct {
		name         string
		location     *lymcwd.Location
		wantMode     string
		wantWorktree string
	}{
		{"TaskUnderThePrimeReadsThePrimesNotItsOwn", task, "off", prime.WorktreeName},
		{"MemberUnderABeamReadsTheBeams", member, "required", "beam"},
		{"BeamReadsThePrimes", beam, "off", prime.WorktreeName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, worktree, err := depsOf(tt.location).PullRequest.ParentMode()
			if err != nil || mode != tt.wantMode || worktree != tt.wantWorktree {
				t.Errorf("ParentMode() = %q, %q, %v; want %q, %q, nil", mode, worktree, err, tt.wantMode, tt.wantWorktree)
			}
		})
	}

	t.Run("EntryFieldReadsTheRunsOwnEntry", func(t *testing.T) {
		field, err := depsOf(beam).PullRequest.EntryField()
		if err != nil || field != "on" {
			t.Errorf("beam EntryField() = %q, %v; want \"on\", nil", field, err)
		}
		field, err = depsOf(member).PullRequest.EntryField()
		if err != nil || field != "" {
			t.Errorf("member EntryField() = %q, %v; want an empty field for an absent entry, nil", field, err)
		}
	})
}

// TestLandingDeps_TaskHasCommitsReadsTheTaskHeadAgainstTheParent asserts the TaskHasCommits seam is false for a task branch level with its parent, true once the task branch holds a commit the parent lacks, and an error when the parent branch does not resolve.
func TestLandingDeps_TaskHasCommitsReadsTheTaskHeadAgainstTheParent(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task")
	taskWorktree := h.PairCodeWorktree("task")
	taskLocation, err := lymcwd.ResolveWorktree(taskWorktree)
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(task) error = %v; want nil", err)
	}
	taskHasCommits := func(parentBranch string) (bool, error) {
		deps := landingDeps(taskLocation, shedrun.RecipeLoom, websterengine.Geometry{}, "task", parentBranch,
			true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(taskLocation), parentLockDeps(taskLocation, shedrun.SelfRunID, "", ""))
		return deps.PullRequest.TaskHasCommits()
	}

	if !t.Run("LevelWithTheParentHasNoCommits", func(t *testing.T) {
		got, err := taskHasCommits("main")
		if err != nil || got {
			t.Errorf("TaskHasCommits() = %v, %v; want false, nil for a task branch level with its parent", got, err)
		}
	}) {
		return
	}

	t.Run("OneCommitBeyondTheParentHasCommits", func(t *testing.T) {
		gitkit.CommitFile(t, taskWorktree, "task.txt", "work\n", "task: add work")
		got, err := taskHasCommits("main")
		if err != nil || !got {
			t.Errorf("TaskHasCommits() = %v, %v; want true, nil after one commit on the task branch", got, err)
		}
	})

	t.Run("AParentBranchThatDoesNotResolveIsAnError", func(t *testing.T) {
		got, err := taskHasCommits("no-such-branch")
		if err == nil || !strings.Contains(err.Error(), "no-such-branch") || got {
			t.Errorf("TaskHasCommits() = %v, %v; want false and an error naming the parent branch", got, err)
		}
	})
}
