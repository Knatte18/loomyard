// landingdeps_integration_test.go drives landingDeps' seams over a real hub.
// ConfigChanges reads a pair's committed config files through real branches.
// Notify posts creel mail to the prime's orch.
// The PullRequest seams read the run's beam.yaml pull_request through the per-task lookup and the task branch's commits.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestLandingDeps_ConfigNoticeSeamsOverRealHub asserts three seams over a real hub.
// ConfigChanges reports a per-worktree module's changed config file, never a hub-wide module's.
// Notify posts nothing without a recorded orch strand, and fails with the notice line once one is recorded.
// CommitParentRecords commits the parent's run records alone, and nothing while it holds none.
// The steps share one hub and one prime orch state,
// and the queueing step relies on the no-strand step having queued nothing,
// so none runs in parallel.
func TestLandingDeps_ConfigNoticeSeamsOverRealHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, pairHubShape)
	const slug = hubforge.TemplatePairSlug
	taskRecords := h.PairRecordsSibling(slug)
	gitkit.CommitFile(t, taskRecords, configengine.ConfigFileRel("loom"), "task_setting: changed\n", "task: change loom config")
	gitkit.CommitFile(t, taskRecords, configengine.ConfigFileRel("board"), "task_setting: changed\n", "task: change board config")

	taskLocation, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s) error = %v; want nil", slug, err)
	}
	deps := landingDeps(taskLocation, shedrun.RecipeTapestry, websterengine.Geometry{}, slug, "main",
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

	if !t.Run("Notify with no orch strand recorded returns nil", func(t *testing.T) {
		if err := deps.Notify("loom: a notice"); err != nil {
			t.Fatalf("Notify() error = %v; want nil", err)
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

// TestParentRecordsCommitter_CommitsTheRecordedParent checks the committer's parent-records commit.
// Member m is forked from beam b, whose orch is stopped.
// The committer commits a run-records file left in b's records and leaves the prime's records untouched.
func TestParentRecordsCommitter_CommitsTheRecordedParent(t *testing.T) {
	tiergate.Require(t, 1)
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

// TestLandingDeps_PullRequestModeReadsThePerTaskLookup pins the Mode seam's per-task lookup of pull_request.
// The run's own worktree's value applies first, else the nearest parent's up to the prime.
func TestLandingDeps_PullRequestModeReadsThePerTaskLookup(t *testing.T) {
	tiergate.Require(t, 1)
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
	hubforge.AddPairWith(t, h, "plainbeam", fabricengine.AddOptions{Beam: true})

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
		return landingDeps(location, shedrun.RecipeTapestry, websterengine.Geometry{}, location.WorktreeName, "main",
			true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(location), parentLockDeps(location, shedrun.SelfRunID, "", ""))
	}

	prime, task, beam, member, plainBeam := h.Location, locate(h.PairCodeWorktree("task")), locate(h.PairCodeWorktree("beam")), locate(h.PairCodeWorktree("member")), locate(h.PairCodeWorktree("plainbeam"))
	setMode(prime, "off")
	setMode(task, "trail")
	setMode(beam, "review")

	tests := []struct {
		name     string
		location *lymcwd.Location
		wantMode string
	}{
		{"TaskReadsItsOwnValueNotThePrimes", task, "trail"},
		{"MemberWithNoValueReadsItsParentBeams", member, "review"},
		{"BeamWithNoValueReadsThePrimes", plainBeam, "off"},
		{"BeamReadsItsOwnValue", beam, "review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := depsOf(tt.location).PullRequest.Mode()
			if err != nil || mode != tt.wantMode {
				t.Errorf("Mode() = %q, %v; want %q, nil", mode, err, tt.wantMode)
			}
		})
	}
}

// TestLandingDeps_TaskHasCommitsReadsTheTaskHeadAgainstTheParent asserts the TaskHasCommits seam.
// It reports false while the task branch is level with its parent.
// It reports true once the task branch holds a commit the parent lacks.
// It reports an error when the parent branch does not resolve.
func TestLandingDeps_TaskHasCommitsReadsTheTaskHeadAgainstTheParent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, pairHubShape)
	const slug = hubforge.TemplatePairSlug
	taskWorktree := h.PairCodeWorktree(slug)
	taskLocation, err := lymcwd.ResolveWorktree(taskWorktree)
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s) error = %v; want nil", slug, err)
	}
	taskHasCommits := func(parentBranch string) (bool, error) {
		deps := landingDeps(taskLocation, shedrun.RecipeTapestry, websterengine.Geometry{}, slug, parentBranch,
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

// TestLandingDeps_RecordLandingWritesWhereTheDobbyReads pins the RecordLanding seam's write.
// It writes the commit and its empty-diff flag into the prime's scratch of the run named for the pair.
// shedrun.ReadLandedCommit reads them there.
func TestLandingDeps_RecordLandingWritesWhereTheDobbyReads(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, pairHubShape)
	const slug = hubforge.TemplatePairSlug
	taskLocation, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s) error = %v; want nil", slug, err)
	}
	deps := landingDeps(taskLocation, shedrun.RecipeTapestry, websterengine.Geometry{}, slug, "main",
		true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(taskLocation), parentLockDeps(taskLocation, shedrun.SelfRunID, "", ""))

	if err := deps.RecordLanding("abc123", true); err != nil {
		t.Fatalf("RecordLanding() error = %v; want nil", err)
	}

	got, err := shedrun.ReadLandedCommit(h.Location, slug)
	if want := (shedrun.LandedCommit{Commit: "abc123", EmptyDiff: true}); err != nil || got != want {
		t.Errorf("ReadLandedCommit(prime, task) = %+v, %v; want %+v, nil", got, err, want)
	}
}

// TestPullRequestDeps_CommitsTheRecord asserts the CommitRecord seam commits the run's record on the records side.
// A record written through the deps' paths is committed, and so is its removal.
// A clean call commits nothing.
// Each commit is pushed synchronously, so origin's records branch holds it once the committer returns.
func TestPullRequestDeps_CommitsTheRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, pairHubShape)
	const slug = hubforge.TemplatePairSlug
	location, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(%s) error = %v; want nil", slug, err)
	}
	records := h.PairRecordsSibling(slug)

	recordsBranch := gitkit.Git(t, records, "rev-parse", "--abbrev-ref", "HEAD")
	requireOriginAtHead := func(when string) {
		t.Helper()
		origin, head := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+recordsBranch), gitkit.RevParse(t, records, "HEAD")
		if origin != head {
			t.Errorf("origin's records branch %s = %s; want the committed head %s", when, origin, head)
		}
	}

	deps := pullRequestDepsGuardedBy(prePushCheck{slug: slug, skip: func() (bool, error) { return true, nil }}, location, "main", true, &shuttleengine.Runner{})
	rel := filepath.ToSlash(shedrun.PullRequestRecordRel(location, shedrun.SelfRunID))
	headFiles := func(format string) string {
		t.Helper()
		return gitkit.Git(t, records, "show", format, "--format=", "HEAD")
	}

	if err := os.MkdirAll(filepath.Dir(deps.RecordPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pullrequest.WriteRecord(deps.RecordPath, deps.RecordLockPath, pullrequest.Record{Number: 7, URL: "https://example.test/pull/7"}); err != nil {
		t.Fatalf("WriteRecord() error = %v; want nil", err)
	}
	if err := deps.CommitRecord(); err != nil {
		t.Fatalf("CommitRecord() after a write error = %v; want nil", err)
	}
	if got := headFiles("--name-status"); got != "A\t"+rel {
		t.Errorf("records HEAD after a write = %q; want the record added", got)
	}
	requireOriginAtHead("after a write")

	if err := pullrequest.RemoveRecord(deps.RecordPath, deps.RecordLockPath); err != nil {
		t.Fatalf("RemoveRecord() error = %v; want nil", err)
	}
	if err := deps.CommitRecord(); err != nil {
		t.Fatalf("CommitRecord() after a removal error = %v; want nil", err)
	}
	if got := headFiles("--name-status"); got != "D\t"+rel {
		t.Errorf("records HEAD after a removal = %q; want the record deleted", got)
	}
	requireOriginAtHead("after a removal")

	headBefore := gitkit.RevParse(t, records, "HEAD")
	if err := deps.CommitRecord(); err != nil {
		t.Fatalf("CommitRecord() with nothing to commit error = %v; want nil", err)
	}
	if got := gitkit.RevParse(t, records, "HEAD"); got != headBefore {
		t.Errorf("records HEAD = %s with nothing to commit; want %s", got, headBefore)
	}
}

// TestLandingDeps_RemoveFollowUpIsEmptyWhenThePrimeIsUnreadable asserts the RemoveFollowUp seam returns no step.
// It does so when the hub holds no readable prime.
func TestLandingDeps_RemoveFollowUpIsEmptyWhenThePrimeIsUnreadable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	l := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "task", AnchorRel: "."}
	deps := landingDeps(l, shedrun.RecipeTapestry, websterengine.Geometry{}, "task", "main",
		true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(l), parentLockDeps(l, shedrun.SelfRunID, "", ""))

	if got := deps.RemoveFollowUp(); got != "" {
		t.Errorf("RemoveFollowUp() = %q over a hub with no readable prime; want empty", got)
	}
}

// TestLandingDeps_BeamCommitsMissingLeavesOutAMergeOfTheParent asserts the CommitsMissing seam of a pawl run.
// It lists the beam commits past a merged head and leaves out a merge that only brought the parent in.
// So Finalize reads a merged pull request as lacking what `lym beam ready` reads it as lacking.
func TestLandingDeps_BeamCommitsMissingLeavesOutAMergeOfTheParent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	worktree := h.PairCodeWorktree("b")
	beam, err := lymcwd.ResolveWorktree(worktree)
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(b) error = %v; want nil", err)
	}
	deps := landingDeps(beam, shedrun.RecipePawl, websterengine.Geometry{}, "b", "main",
		true, modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(beam), parentLockDeps(beam, shedrun.SelfRunID, "", ""))

	gitkit.CommitFile(t, worktree, "widgets.txt", "widgets\n", "Add widgets")
	mergedHead := gitkit.Git(t, worktree, "rev-parse", "HEAD")
	parentTip := gitkit.CommitFile(t, h.PrimeWorktree(), "parent.txt", "parent\n", "parent: progress")
	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "Merge the parent", parentTip)

	missing, err := deps.PullRequest.CommitsMissing(mergedHead, gitkit.Git(t, worktree, "rev-parse", "HEAD"))
	if err != nil || len(missing) != 0 {
		t.Errorf("CommitsMissing() = %q, %v for a beam past the merged head only by a merge of the parent; want none", missing, err)
	}

	gitkit.CommitFile(t, worktree, "gadgets.txt", "gadgets\n", "Add gadgets")
	missing, err = deps.PullRequest.CommitsMissing(mergedHead, gitkit.Git(t, worktree, "rev-parse", "HEAD"))
	if err != nil || len(missing) != 1 || !strings.HasSuffix(missing[0], " Add gadgets") {
		t.Errorf("CommitsMissing() = %q, %v after a beam commit; want the one commit named by its subject", missing, err)
	}
}
