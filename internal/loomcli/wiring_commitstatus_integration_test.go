// wiring_commitstatus_integration_test.go drives the per-transition status seam and commitRecordsVerb.
// Both run against a real fabric pair, with real MergeStateActive, CommitAnchoredPaths, PushPairAnchored, and git.
// Stub-based tests pass over a seam that stages the status file into a foreign merge's index before killing the run.
// A stub Commit has no index to stage into and no git to refuse it.
// Only the composed shape shows what the three dispositions do to a repository.

package loomcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// realSeamFixture builds a hub, adds one pair, and seeds a status file at loom's own status path.
// It returns the seam wired to that pair through loomCommitStatusDeps, the same call wire() makes.
// It also returns the pair's records sibling path, for direct git inspection.
func realSeamFixture(t *testing.T) (seam func(producer, state string) error, location *lymcwd.Location, recordsSibling string) {
	t.Helper()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "commitstatus"
	hubforge.AddPair(t, hub, slug)
	codeWorktree := hub.PairCodeWorktree(slug)

	location, err := lymcwd.ResolveWorktree(codeWorktree)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s) error = %v; want nil", codeWorktree, err)
	}
	writeStatusFile(t, location, `{"current_producer":"seed","state":"running"}`)

	return newCommitStatusSeam(loomCommitStatusDeps(location, shedrun.SelfRunID)), location, hub.PairRecordsSibling(slug)
}

// realSeamStatusRel is the status file's relative path for the self run.
func realSeamStatusRel(location *lymcwd.Location) string {
	return shedrun.StatusRel(location, shedrun.SelfRunID)
}

// writeStatusFile writes content at loom's own status path for location, creating the directory the first call needs.
// It goes through shedrun's own accessor rather than a hand-built join,
// so the test commits exactly the path the seam's own pathspec names.
func writeStatusFile(t *testing.T, location *lymcwd.Location, content string) {
	t.Helper()
	path := shedrun.StatusFile(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v; want nil", path, err)
	}
}

// startForeignMergeInRecords leaves a live MERGE_HEAD in the records sibling by running plain git there.
// This is the operator behaviour the Fabric Git Invariant's carve-out permits.
// No fabric verb puts merge state in the records, so this is the only way it gets there.
func startForeignMergeInRecords(t *testing.T, recordsSibling string) {
	t.Helper()
	current := gitkit.CurrentBranch(t, recordsSibling)
	gitkit.Git(t, recordsSibling, "checkout", "-q", "-b", "foreign-side")
	if err := os.WriteFile(filepath.Join(recordsSibling, "foreign.txt"), []byte("foreign\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(foreign.txt) error = %v; want nil", err)
	}
	gitkit.Git(t, recordsSibling, "add", "foreign.txt")
	gitkit.Git(t, recordsSibling, "commit", "-q", "-m", "foreign side commit")
	gitkit.Git(t, recordsSibling, "checkout", "-q", current)
	gitkit.Git(t, recordsSibling, "merge", "--no-commit", "--no-ff", "foreign-side")

	if !mergeHeadPresent(t, recordsSibling) {
		t.Fatal("no MERGE_HEAD in the records sibling after a plain-git merge --no-commit; the fixture proves nothing without one")
	}
}

// startStoppedCherryPickInRecords leaves a cherry-pick in progress in the records sibling, with a clean index.
// It picks an empty commit, which git stops on with CHERRY_PICK_HEAD set, as a fabric replay mid-pick leaves it.
func startStoppedCherryPickInRecords(t *testing.T, recordsSibling string) {
	t.Helper()
	current := gitkit.CurrentBranch(t, recordsSibling)
	gitkit.Git(t, recordsSibling, "checkout", "-q", "-b", "replayed-side")
	gitkit.Git(t, recordsSibling, "commit", "-q", "--allow-empty", "-m", "replayed side commit")
	gitkit.Git(t, recordsSibling, "checkout", "-q", current)
	runGitAllowingFailure(t, recordsSibling, "cherry-pick", "replayed-side")

	path := gitkit.Git(t, recordsSibling, "rev-parse", "--git-path", "CHERRY_PICK_HEAD")
	if !filepath.IsAbs(path) {
		path = filepath.Join(recordsSibling, path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no CHERRY_PICK_HEAD in the records sibling after picking an empty commit: %v", err)
	}
}

// runGitAllowingFailure runs git in dir and ignores its exit status, for a command that stops on a conflict.
func runGitAllowingFailure(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	_, _ = cmd.CombinedOutput()
}

// mergeHeadPresent reports whether the repo at dir currently has a live MERGE_HEAD.
func mergeHeadPresent(t *testing.T, dir string) bool {
	t.Helper()
	path := gitkit.Git(t, dir, "rev-parse", "--git-path", "MERGE_HEAD")
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	_, err := os.Stat(path)
	return err == nil
}

// TestCommitStatusSeam_Real drives the status seam and the commit-records verb through every disposition.
// It runs them over one real pair.
// The steps share state, so a step that relies on an earlier one says so.
// The ordinary dispositions run first while the remote is healthy;
// the mid-merge skips share one live foreign merge;
// the remote-failure steps run last because they leave the branch diverged.
// The seam pushes its transition to the remote before it returns, and so does commit-records.
// Each step changes the status file first, so every seam call has something of its own to commit.
func TestCommitStatusSeam_Real(t *testing.T) {
	tiergate.Require(t, 1)
	seam, location, recordsSibling := realSeamFixture(t)

	statusRel := realSeamStatusRel(location)
	statusChanges := 0
	changeStatus := func() {
		t.Helper()
		statusChanges++
		writeStatusFile(t, location, fmt.Sprintf(`{"current_producer":"seed","state":"running","change":%d}`, statusChanges))
	}
	headChangedFiles := func() string {
		t.Helper()
		return gitkit.Git(t, recordsSibling, "show", "--name-only", "--format=", "HEAD")
	}
	statusOnly := func(t *testing.T) {
		t.Helper()
		if got := headChangedFiles(); got != filepath.ToSlash(statusRel) {
			t.Errorf("records HEAD touched %q; want only the status file", got)
		}
	}

	// The seed written by realSeamFixture is this step's own change.
	t.Run("ordinary path commits and pushes the transition", func(t *testing.T) {
		before := gitkit.RevParse(t, recordsSibling, "HEAD")

		if err := seam("Discussion-Write", "running"); err != nil {
			t.Fatalf("seam(Discussion-Write, running) error = %v; want nil", err)
		}

		after := gitkit.RevParse(t, recordsSibling, "HEAD")
		if after == before {
			t.Fatalf("records HEAD = %q; want it moved off %q — the seam must land a real commit", after, before)
		}
		if got := gitkit.Git(t, recordsSibling, "log", "-1", "--format=%s"); got != "loom: Discussion-Write -> running" {
			t.Errorf("records HEAD subject = %q; want %q", got, "loom: Discussion-Write -> running")
		}
		if got := headChangedFiles(); !strings.Contains(got, statusRel) {
			t.Errorf("records HEAD touched %q; want it to include %q", got, statusRel)
		}
		if got := gitkit.Git(t, recordsSibling, "log", "--oneline", "@{u}..HEAD"); got != "" {
			t.Errorf("unpushed records commits after the seam = %q; want none — the seam pushes before it returns", got)
		}
		if got := remoteBranchTip(t, recordsSibling); got != after {
			t.Errorf("remote records branch = %q; want it at the records HEAD %q", got, after)
		}
	})

	t.Run("no reviews directory touches only the status", func(t *testing.T) {
		changeStatus()
		if err := seam("Discussion-Write", "running"); err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}
		statusOnly(t)
	})

	// The recipe-build state: the reviews root holds only an empty segment directory.
	t.Run("empty reviews segment touches only the status", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(loomengine.LoomReviewsDir(location), "plan"), 0o755); err != nil {
			t.Fatal(err)
		}
		changeStatus()
		if err := seam("Discussion-Write", "running"); err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}
		statusOnly(t)
	})

	t.Run("round record is committed", func(t *testing.T) {
		files := []string{"plan/round-1-review.md", "plan/round-1-fixer-report.md", "plan/round-1-focus.md"}
		for _, f := range files {
			writeReviewsDirFile(t, location, f, "record\n")
		}
		changeStatus()

		if err := seam("Plan-Review", "running"); err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}

		got := headChangedFiles()
		for _, f := range files {
			want := filepath.ToSlash(filepath.Join(loomengine.LoomReviewsDirRel(), f))
			if !strings.Contains(got, want) {
				t.Errorf("records HEAD touched %q; want it to include %q", got, want)
			}
		}
		if st := gitkit.Git(t, recordsSibling, "status", "--porcelain", "--", filepath.ToSlash(loomengine.LoomReviewsDirRel())); st != "" {
			t.Errorf("records status under the reviews directory = %q; want clean", st)
		}
	})

	// Relies on "round record is committed": round-1-review.md is already in the records HEAD.
	t.Run("archive rename commits addition and deletion", func(t *testing.T) {
		dir := filepath.Join(loomengine.LoomReviewsDir(location), "plan")
		if err := os.Rename(filepath.Join(dir, "round-1-review.md"), filepath.Join(dir, "round-1-review-20260101T000000.md")); err != nil {
			t.Fatal(err)
		}
		changeStatus()
		if err := seam("Plan-Review", "done"); err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}

		got := gitkit.Git(t, recordsSibling, "show", "--no-renames", "--name-status", "--format=", "HEAD")
		prefix := filepath.ToSlash(filepath.Join(loomengine.LoomReviewsDirRel(), "plan")) + "/"
		if !strings.Contains(got, "A\t"+prefix+"round-1-review-20260101T000000.md") {
			t.Errorf("records HEAD = %q; want the timestamped sibling added", got)
		}
		if !strings.Contains(got, "D\t"+prefix+"round-1-review.md") {
			t.Errorf("records HEAD = %q; want the original deleted", got)
		}
	})

	t.Run("friction note and drive report are committed", func(t *testing.T) {
		writeRecordFile(t, filepath.Join(loomengine.LoomFrictionDir(location), "note.md"), "friction\n")
		writeRecordFile(t, filepath.Join(shedrun.DriveReportsDir(location, shedrun.SelfRunID), "report.md"), "report\n")
		changeStatus()

		if err := seam("Plan-Write", "running"); err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}

		got := headChangedFiles()
		for _, want := range []string{
			filepath.ToSlash(filepath.Join(loomengine.LoomDurableDirRel(), "friction", "note.md")),
			filepath.ToSlash(filepath.Join(shedrun.DriveReportsRel(location, shedrun.SelfRunID), "report.md")),
		} {
			if !strings.Contains(got, want) {
				t.Errorf("records HEAD touched %q; want it to include %q", got, want)
			}
		}
		for _, dir := range []string{loomengine.LoomDurableDirRel(), shedrun.DriveReportsRel(location, shedrun.SelfRunID)} {
			if st := gitkit.Git(t, recordsSibling, "status", "--porcelain", "--", filepath.ToSlash(dir)); st != "" {
				t.Errorf("records status under %s = %q; want clean", dir, st)
			}
		}
	})

	// Relies on "friction note and drive report are committed": the friction note is already in the records HEAD.
	t.Run("friction archive rename commits addition and deletion", func(t *testing.T) {
		archive := filepath.Join(loomengine.LoomDurableDir(location), "friction-20260101T000000")
		if err := os.Rename(loomengine.LoomFrictionDir(location), archive); err != nil {
			t.Fatal(err)
		}
		changeStatus()
		if err := seam("Plan-Write", "done"); err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}

		got := gitkit.Git(t, recordsSibling, "show", "--no-renames", "--name-status", "--format=", "HEAD")
		prefix := filepath.ToSlash(loomengine.LoomDurableDirRel()) + "/"
		if !strings.Contains(got, "A\t"+prefix+"friction-20260101T000000/note.md") {
			t.Errorf("records HEAD = %q; want the archive sibling added", got)
		}
		if !strings.Contains(got, "D\t"+prefix+"friction/note.md") {
			t.Errorf("records HEAD = %q; want the old friction path deleted", got)
		}
		if st := gitkit.Git(t, recordsSibling, "status", "--porcelain", "--", filepath.ToSlash(loomengine.LoomDurableDirRel())); st != "" {
			t.Errorf("records status under the loom durable directory = %q; want clean", st)
		}
	})

	// A drive report written after the last transition lands on the records tip, and the records origin's branch carries it.
	t.Run("commit-records commits and pushes a late drive report", func(t *testing.T) {
		deps := loomCommitStatusDeps(location, shedrun.SelfRunID)
		writeRecordFile(t, filepath.Join(shedrun.DriveReportsDir(location, shedrun.SelfRunID), "late-report.md"), "stop report\n")

		var out bytes.Buffer
		if code := commitRecordsVerb(&out, deps); code != 0 {
			t.Fatalf("exit = %d; want 0; output %s", code, out.String())
		}

		want := filepath.ToSlash(filepath.Join(shedrun.DriveReportsRel(location, shedrun.SelfRunID), "late-report.md"))
		if got := headChangedFiles(); !strings.Contains(got, want) {
			t.Errorf("records HEAD touched %q; want it to include %q", got, want)
		}
		if got := gitkit.Git(t, recordsSibling, "log", "--oneline", "@{u}..HEAD"); got != "" {
			t.Errorf("unpushed records commits = %q; want none", got)
		}
		branch := gitkit.CurrentBranch(t, recordsSibling)
		remote := gitkit.Git(t, recordsSibling, "ls-remote", "origin", "refs/heads/"+branch)
		head := gitkit.RevParse(t, recordsSibling, "HEAD")
		if !strings.HasPrefix(remote, head) {
			t.Errorf("origin %s = %q; want it at the records HEAD %s", branch, remote, head)
		}
	})

	// Relies on the step above: its commit left the tree clean.
	t.Run("commit-records on a clean tree is a no-op success", func(t *testing.T) {
		before := gitkit.RevParse(t, recordsSibling, "HEAD")

		var out bytes.Buffer
		if code := commitRecordsVerb(&out, loomCommitStatusDeps(location, shedrun.SelfRunID)); code != 0 {
			t.Fatalf("exit = %d; want 0; output %s", code, out.String())
		}
		if got := gitkit.RevParse(t, recordsSibling, "HEAD"); got != before {
			t.Errorf("records HEAD = %q; want unchanged %q — a clean tree adds no commit", got, before)
		}
	})

	// The skip disposition against a real foreign merge: nothing is committed or staged into the operator's index,
	// and their MERGE_HEAD survives untouched.
	// The merge stays live for the next step.
	t.Run("mid-merge skips without touching the merge", func(t *testing.T) {
		changeStatus()
		startForeignMergeInRecords(t, recordsSibling)
		before := gitkit.RevParse(t, recordsSibling, "HEAD")
		stagedBefore := gitkit.Git(t, recordsSibling, "diff", "--cached", "--name-only")

		if err := seam("Discussion-Write", "running"); err != nil {
			t.Fatalf("seam(...) error = %v; want nil — a mid-merge records side skips, it does not halt the run", err)
		}

		if got := gitkit.RevParse(t, recordsSibling, "HEAD"); got != before {
			t.Errorf("records HEAD = %q; want unchanged %q — the skip must commit nothing", got, before)
		}
		if !mergeHeadPresent(t, recordsSibling) {
			t.Error("MERGE_HEAD is gone after the seam ran; want the operator's merge left exactly as it was")
		}
		if got := gitkit.Git(t, recordsSibling, "diff", "--cached", "--name-only"); got != stagedBefore {
			t.Errorf("staged paths = %q; want unchanged %q — the skip must not stage the status file into the operator's merge index", got, stagedBefore)
		}
	})

	// The lost race the unlocked probe leaves open, deterministically:
	// MergeActive answers false once, as it would if the operator started their merge a millisecond later.
	// Every other dep, including the re-probe, is the real one.
	// Without the re-probe, git's "cannot do a partial commit during a merge" halts the whole run;
	// the step must resolve as the skip, with the operator's merge intact.
	// Relies on the foreign merge the step above left live, and ends it.
	t.Run("merge going live after the probe skips instead of halting", func(t *testing.T) {
		before := gitkit.RevParse(t, recordsSibling, "HEAD")
		changeStatus()

		deps := loomCommitStatusDeps(location, shedrun.SelfRunID)
		realMergeActive := deps.MergeActive
		probes := 0
		deps.MergeActive = func() (bool, error) {
			probes++
			if probes == 1 {
				return false, nil
			}
			return realMergeActive()
		}

		if err := newCommitStatusSeam(deps)("Discussion-Write", "running"); err != nil {
			t.Fatalf("seam(...) error = %v; want nil — a commit failure the re-probe explains as a live merge takes the skip", err)
		}
		if probes != 2 {
			t.Errorf("MergeActive called %d time(s); want exactly 2 — the pre-commit probe and the re-probe that explains its failure", probes)
		}
		if got := gitkit.RevParse(t, recordsSibling, "HEAD"); got != before {
			t.Errorf("records HEAD = %q; want unchanged %q", got, before)
		}
		if !mergeHeadPresent(t, recordsSibling) {
			t.Error("MERGE_HEAD is gone after the lost race; want the operator's merge left exactly as it was")
		}

		gitkit.Git(t, recordsSibling, "merge", "--abort")
		if mergeHeadPresent(t, recordsSibling) {
			t.Fatal("MERGE_HEAD survives the abort; the steps after this one need a records side with no merge in progress")
		}
	})

	// A records cherry-pick in a two-repository hub does not count.
	// Fabric's own replay cherry-picks under the write lock the commit waits on.
	// The real probe runs while the pick is in progress; the pick then ends, as the replay does, before the commit.
	// Relies on the step above leaving no merge in progress.
	t.Run("records cherry-pick in progress commits the transition", func(t *testing.T) {
		changeStatus()
		startStoppedCherryPickInRecords(t, recordsSibling)
		before := gitkit.RevParse(t, recordsSibling, "HEAD")

		deps := loomCommitStatusDeps(location, shedrun.SelfRunID)
		realMergeActive := deps.MergeActive
		probes := 0
		deps.MergeActive = func() (bool, error) {
			probes++
			active, err := realMergeActive()
			if probes == 1 {
				gitkit.Git(t, recordsSibling, "cherry-pick", "--abort")
			}
			return active, err
		}

		if err := newCommitStatusSeam(deps)("Discussion-Write", "running"); err != nil {
			t.Fatalf("seam(...) error = %v; want nil", err)
		}
		if got := gitkit.RevParse(t, recordsSibling, "HEAD"); got == before {
			t.Errorf("records HEAD = %q; want a transition commit — a records cherry-pick must not skip it", got)
		}
		statusOnly(t)
	})

	// The task branch advanced on its remote by a second clone, past every tip the run's transitions name:
	// The commit-records commit is itself a transition, so the owner's code push resets origin's branch to its tip.
	// The records reach their remote, and the local task branch is left as it was.
	t.Run("task branch advanced on origin is overridden by the owner's push", func(t *testing.T) {
		taskWorktree := location.WorktreePath()
		branch := gitkit.CurrentBranch(t, taskWorktree)
		clone := t.TempDir()
		gitkit.Git(t, clone, "clone", "-q", "-b", branch, gitkit.Git(t, taskWorktree, "remote", "get-url", "origin"), ".")
		gitkit.Git(t, clone, "config", "user.email", "rogue@example.test")
		gitkit.Git(t, clone, "config", "user.name", "rogue")
		writeRecordFile(t, filepath.Join(clone, "rogue.txt"), "rogue\n")
		gitkit.Git(t, clone, "add", "rogue.txt")
		gitkit.Git(t, clone, "commit", "-q", "-m", "rogue advance")
		gitkit.Git(t, clone, "push", "-q", "origin", branch)

		commitInTaskWorktree(t, location, "diverging.txt")
		taskHead := gitkit.RevParse(t, taskWorktree, "HEAD")
		writeRecordFile(t, filepath.Join(shedrun.DriveReportsDir(location, shedrun.SelfRunID), "diverged-report.md"), "stop report\n")

		var out bytes.Buffer
		if code := commitRecordsVerb(&out, loomCommitStatusDeps(location, shedrun.SelfRunID)); code != 0 {
			t.Fatalf("exit = %d; want 0; output %s", code, out.String())
		}
		if got := gitkit.RevParse(t, taskWorktree, "HEAD"); got != taskHead {
			t.Errorf("task branch HEAD = %q; want unchanged %q", got, taskHead)
		}
		if got := remoteBranchTip(t, taskWorktree); got != taskHead {
			t.Errorf("remote task branch = %q; want it reset to the local tip %q", got, taskHead)
		}
		if got, want := remoteBranchTip(t, recordsSibling), gitkit.RevParse(t, recordsSibling, "HEAD"); got != want {
			t.Errorf("remote records branch = %q; want it at the records HEAD %q", got, want)
		}
	})

	// The transition's tracked writes and its code commit are on origin before the seam returns:
	// the records branch holds the transition and a fixer attempts file written under the durable directory,
	// and the code branch holds the code commit.
	// It runs after the diverged step.
	// A transition naming a pushed code tip makes the owner's push override another clone's advance of the code branch.
	t.Run("transition pushes the records and the code side", func(t *testing.T) {
		attemptsRel := filepath.ToSlash(filepath.Join(loomengine.LoomDurableDirRel(), "fixer-attempts.json"))
		writeRecordFile(t, filepath.Join(loomengine.LoomDurableDir(location), "fixer-attempts.json"), "{\"attempts\":1}\n")
		commitInTaskWorktree(t, location, "code-change.txt")
		changeStatus()

		if err := seam("Code-Review", "running"); err != nil {
			t.Fatalf("seam(Code-Review, running) error = %v; want nil", err)
		}

		if got := headChangedFiles(); !strings.Contains(got, attemptsRel) {
			t.Errorf("records HEAD touched %q; want it to include %q", got, attemptsRel)
		}
		if got, want := remoteBranchTip(t, recordsSibling), gitkit.RevParse(t, recordsSibling, "HEAD"); got != want {
			t.Errorf("remote records branch = %q; want it at the records HEAD %q", got, want)
		}
		taskWorktree := location.WorktreePath()
		if got, want := remoteBranchTip(t, taskWorktree), gitkit.RevParse(t, taskWorktree, "HEAD"); got != want {
			t.Errorf("remote code branch = %q; want it at the code HEAD %q", got, want)
		}
	})

	// Relies on the step above: its records HEAD was made after the code commit.
	t.Run("transition commit names its code tip and its run", func(t *testing.T) {
		message := gitkit.Git(t, recordsSibling, "log", "-1", "--format=%B")
		codeHead := gitkit.RevParse(t, location.WorktreePath(), "HEAD")

		trailers := gitkit.Git(t, recordsSibling, "log", "-1", "--format=%(trailers:only,valueonly,separator=%x2C)")
		if !strings.Contains(message, codeHead) || !strings.Contains(trailers, codeHead) {
			t.Errorf("records HEAD message = %q; want a trailer naming the code HEAD %q", message, codeHead)
		}
		if want := fabricengine.TransitionTrailerKey + ": " + shedrun.ResolveRunID(location, shedrun.SelfRunID); !strings.Contains(message, want) {
			t.Errorf("records HEAD message = %q; want it to contain the trailer %q", message, want)
		}
	})
}

// commitInTaskWorktree commits a new file named name in the task worktree with plain git, as an agent's raw commit would.
func commitInTaskWorktree(t *testing.T, location *lymcwd.Location, name string) {
	t.Helper()
	taskWorktree := location.WorktreePath()
	writeRecordFile(t, filepath.Join(taskWorktree, name), name+"\n")
	gitkit.Git(t, taskWorktree, "add", name)
	gitkit.Git(t, taskWorktree, "-c", "user.email=agent@example.test", "-c", "user.name=agent", "commit", "-q", "-m", "raw commit "+name)
}

// remoteBranchTip returns the SHA origin holds for the branch checked out in repo.
func remoteBranchTip(t *testing.T, repo string) string {
	t.Helper()
	fields := strings.Fields(gitkit.Git(t, repo, "ls-remote", "origin", "refs/heads/"+gitkit.CurrentBranch(t, repo)))
	if len(fields) == 0 {
		t.Fatalf("origin of %s holds no branch %s", repo, gitkit.CurrentBranch(t, repo))
	}
	return fields[0]
}

// writeReviewsDirFile writes content at rel under the reviews directory, creating its directories.
func writeReviewsDirFile(t *testing.T, location *lymcwd.Location, rel, content string) {
	t.Helper()
	path := filepath.Join(loomengine.LoomReviewsDir(location), rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v; want nil", path, err)
	}
}

// writeRecordFile writes content at path, creating its directory.
func writeRecordFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
