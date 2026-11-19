// recordsduringmerge_integration_test.go covers the records commit of a one-repository hub while a merge is in progress.
// Each row drives a real hubforge pair into a merge state and commits records around it.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordsMergeTrailers is the trailer block the transition rows commit with.
var recordsMergeTrailers = []fabricengine.Trailer{{Key: "Run-Transition", Value: "x"}}

// recordsMergePair is a task pair of a one-repository hub whose code file conflicts with the parent's.
type recordsMergePair struct {
	hub    *hubforge.Hub
	task   fabricengine.AddResult
	loc    *lymcwd.Location
	parent string
}

// newRecordsMergePair returns a pair whose task and parent each edited code.txt after the fork.
// A merge of the parent into the task conflicts on code.txt.
func newRecordsMergePair(t *testing.T) recordsMergePair {
	t.Helper()

	h := newOneRepositoryMergeHub(t)
	prime := h.PrimeWorktree()
	task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
	gitkit.CommitFile(t, task.Path, "code.txt", "task\n", "task edit of the code file")
	gitkit.CommitFile(t, prime, "code.txt", "parent\n", "parent edit of the code file")
	loc, err := lymcwd.ResolveWorktree(task.Path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", task.Path, err)
	}
	return recordsMergePair{hub: h, task: task, loc: loc, parent: gitkit.CurrentBranch(t, prime)}
}

// runGitAllowingFailure runs git in dir and ignores its exit status, for a command that stops on a conflict.
func runGitAllowingFailure(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	_, _ = cmd.CombinedOutput()
}

// mergeHeadPresent reports whether the worktree at dir has a merge in progress.
func mergeHeadPresent(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "-q", "--verify", "MERGE_HEAD")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// startPlainMerge leaves the pair's task worktree mid-way through a plain git merge of the parent.
// The merge is conflicted on code.txt.
func (p recordsMergePair) startPlainMerge(t *testing.T) {
	t.Helper()

	runGitAllowingFailure(t, p.task.Path, "merge", "--no-commit", p.parent)
	if !mergeHeadPresent(p.task.Path) {
		t.Fatalf("no merge in progress in %s after the plain merge", p.task.Path)
	}
}

// commitStatus writes the records file _lym/status.txt in the task worktree and commits it as a transition does.
func (p recordsMergePair) commitStatus(t *testing.T) (sha string, committed bool, err error) {
	t.Helper()

	writeOneRepositoryFile(t, p.task.Path, "_lym/status.txt", "status\n")
	return fabricengine.CommitAnchoredPathsWithTrailers(fabricengine.NewMutations(p.loc.HubPath), p.loc, []string{"_lym/status.txt"},
		"transition", recordsMergeTrailers, fabricengine.SyncOptions{})
}

// requireStatusCommitted fails the test unless the status commit sha is HEAD of the task worktree.
// The commit must also hold _lym/status.txt.
func (p recordsMergePair) requireStatusCommitted(t *testing.T, sha string) {
	t.Helper()

	if head := gitkit.RevParse(t, p.task.Path, "HEAD"); head != sha {
		t.Errorf("HEAD = %s; want the records commit %s", head, sha)
	}
	if got := gitkit.Git(t, p.task.Path, "show", "HEAD:_lym/status.txt"); got != "status" {
		t.Errorf("_lym/status.txt at HEAD = %q; want status", got)
	}
}

// stopMergeBeforeRecordsStep leaves the task worktree with a fabric merge-in of the parent.
// The merge is stopped before its records step.
// The parent holds a records file the task lacks, and the stopped merge has staged it.
func (p recordsMergePair) stopMergeBeforeRecordsStep(t *testing.T) *fabricengine.Fabric {
	t.Helper()

	gitkit.CommitFile(t, p.hub.PrimeWorktree(), "_lym/parent-only.txt", "parent\n", "parent-only records")
	f := openOneRepositoryFabric(t, p.task.Path)
	crash := errors.New("crash before the records step")
	f.SetAfterMergeStartForTest(func() error { return crash })
	if _, err := f.MergeIn(p.parent); !errors.Is(err, crash) {
		t.Fatalf("MergeIn() error = %v; want the injected crash", err)
	}
	f.SetAfterMergeStartForTest(nil)
	return f
}

// requireMergeGoneWithoutSourceRecords fails the test unless the stopped merge is gone and no source records remain.
func (p recordsMergePair) requireMergeGoneWithoutSourceRecords(t *testing.T, f *fabricengine.Fabric) {
	t.Helper()

	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Errorf("merge record exists = (%v, %v); want none", exists, err)
	}
	if mergeHeadPresent(p.task.Path) {
		t.Error("MERGE_HEAD is present; want the merge gone")
	}
	if tracked := gitkit.Git(t, p.task.Path, "ls-tree", "-r", "--name-only", "HEAD", "--", "_lym/parent-only.txt"); tracked != "" {
		t.Errorf("HEAD holds %q; want the source's records absent", tracked)
	}
	if indexed := gitkit.Git(t, p.task.Path, "ls-files", "--", "_lym/parent-only.txt"); indexed != "" {
		t.Errorf("the index holds %q; want the source's records absent", indexed)
	}
	if _, err := os.Stat(filepath.Join(p.task.Path, "_lym", "parent-only.txt")); !os.IsNotExist(err) {
		t.Errorf("the source's records file is on disk (stat err %v); want it gone", err)
	}
}

func TestRecordsCommit_DuringMerge(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("TransitionDuringACodeMergeCommitsAndPushesAndConcludeKeepsIt", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		p.startPlainMerge(t)

		sha, committed, err := p.commitStatus(t)
		if err != nil || !committed {
			t.Fatalf("CommitAnchoredPathsWithTrailers() = (%q, %v, %v); want a landed commit", sha, committed, err)
		}
		p.requireStatusCommitted(t, sha)
		if message := gitkit.Git(t, p.task.Path, "log", "-1", "--format=%B"); !strings.Contains(message, "Run-Transition: x") {
			t.Errorf("commit message = %q; want the trailer block", message)
		}
		if !mergeHeadPresent(p.task.Path) {
			t.Error("MERGE_HEAD is gone after the records commit; want the merge still in progress")
		}

		if _, err := fabricengine.PushPairAnchored(p.loc, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
			t.Fatalf("PushPairAnchored() error = %v; want the records commit pushed", err)
		}
		if got := gitkit.RevParse(t, p.hub.CodeBare, "refs/heads/"+p.task.Branch); got != sha {
			t.Errorf("origin branch tip = %s; want the records commit %s", got, sha)
		}

		writeOneRepositoryFile(t, p.task.Path, "code.txt", "resolved\n")
		gitkit.MustRun(t, p.task.Path, "git", "add", "--", "code.txt")
		gitkit.MustRun(t, p.task.Path, "git", "commit", "--no-edit")
		if !gitkit.IsAncestor(t, p.task.Path, sha, "HEAD") {
			t.Errorf("the records commit %s is no ancestor of the concluded merge", sha)
		}
		if got := gitkit.Git(t, p.task.Path, "show", "HEAD:_lym/status.txt"); got != "status" {
			t.Errorf("_lym/status.txt after the conclude = %q; want status", got)
		}
	})

	t.Run("GitMergeAbortKeepsTheRecordsCommit", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		p.startPlainMerge(t)
		sha, _, err := p.commitStatus(t)
		if err != nil {
			t.Fatalf("CommitAnchoredPathsWithTrailers() error = %v", err)
		}

		gitkit.MustRun(t, p.task.Path, "git", "merge", "--abort")

		p.requireStatusCommitted(t, sha)
		if mergeHeadPresent(p.task.Path) {
			t.Error("MERGE_HEAD is present after the abort")
		}
	})

	t.Run("FabricMergeAbortKeepsTheRecordsCommit", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		f := openOneRepositoryFabric(t, p.task.Path)
		if res, err := f.MergeIn(p.parent); err != nil || len(res.Conflicts) == 0 {
			t.Fatalf("MergeIn() = (conflicts %v, %v); want the merge parked on a conflict", res.Conflicts, err)
		}
		sha, _, err := p.commitStatus(t)
		if err != nil {
			t.Fatalf("CommitAnchoredPathsWithTrailers() error = %v", err)
		}

		if _, err := f.MergeAbort(); err != nil {
			t.Fatalf("MergeAbort() error = %v; want the abort to pass over the records commit", err)
		}

		p.requireStatusCommitted(t, sha)
		if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
			t.Errorf("merge record exists = (%v, %v); want none", exists, err)
		}
	})

	t.Run("SyncDuringAMergeCommitsRecordsOnly", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		p.startPlainMerge(t)
		writeOneRepositoryFile(t, p.task.Path, "_lym/status.txt", "status\n")

		res, err := openOneRepositoryFabric(t, p.task.Path).Commit([]string{"_lym/status.txt"}, "sync", nil, fabricengine.SyncOptions{})

		if err != nil || !res.WeftCommitted {
			t.Fatalf("Commit() = (records committed %v, %v); want a records commit around the merge", res.WeftCommitted, err)
		}
		p.requireStatusCommitted(t, res.WeftSHA)
		if !mergeHeadPresent(p.task.Path) {
			t.Error("MERGE_HEAD is gone after the records commit; want the merge still in progress")
		}
	})

	t.Run("CodePathListDuringAMergeRefuses", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		p.startPlainMerge(t)
		before := gitkit.RevParse(t, p.task.Path, "HEAD")

		_, err := openOneRepositoryFabric(t, p.task.Path).Commit([]string{"code.txt"}, "code", nil, fabricengine.SyncOptions{})

		var refusal *fabricengine.ErrForeignMergeState
		if !errors.As(err, &refusal) {
			t.Fatalf("Commit() error = %v; want an *ErrForeignMergeState", err)
		}
		if got := gitkit.RevParse(t, p.task.Path, "HEAD"); got != before {
			t.Errorf("HEAD = %s; want %s unchanged by the refusal", got, before)
		}
	})

	t.Run("CherryPickRefusesNamingTheOperation", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		gitkit.MustRun(t, p.task.Path, "git", "checkout", "-q", "-b", "other", p.task.Branch+"~1")
		gitkit.CommitFile(t, p.task.Path, "code.txt", "other\n", "other edit of the code file")
		gitkit.MustRun(t, p.task.Path, "git", "checkout", "-q", p.task.Branch)
		runGitAllowingFailure(t, p.task.Path, "cherry-pick", "other")

		_, _, err := p.commitStatus(t)

		if err == nil || !strings.Contains(err.Error(), "cherry-pick") || !strings.Contains(err.Error(), "plain git") {
			t.Fatalf("CommitAnchoredPathsWithTrailers() error = %v; want a refusal naming cherry-pick and plain git", err)
		}
		if operation, opErr := fabricengine.MergeStateOperation(p.loc); opErr != nil || operation != "cherry-pick" {
			t.Errorf("MergeStateOperation() = (%q, %v); want cherry-pick", operation, opErr)
		}
	})

	t.Run("CrashBeforeStepThreeCommitsAbortsAndPushes", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		f := p.stopMergeBeforeRecordsStep(t)

		sha, committed, err := p.commitStatus(t)

		if err != nil || !committed {
			t.Fatalf("CommitAnchoredPathsWithTrailers() = (%q, %v, %v); want a landed commit", sha, committed, err)
		}
		p.requireStatusCommitted(t, sha)
		p.requireMergeGoneWithoutSourceRecords(t, f)
		if got, err := os.ReadFile(filepath.Join(p.task.Path, "_lym", "status.txt")); err != nil || string(got) != "status\n" {
			t.Errorf("_lym/status.txt on disk = (%q, %v); want status", got, err)
		}

		if _, err := fabricengine.PushPairAnchored(p.loc, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
			t.Fatalf("PushPairAnchored() error = %v", err)
		}
		if got := gitkit.Git(t, p.hub.CodeBare, "show", p.task.Branch+":_lym/status.txt"); got != "status" {
			t.Errorf("_lym/status.txt on origin = %q; want status", got)
		}
		if tracked := gitkit.Git(t, p.hub.CodeBare, "ls-tree", "-r", "--name-only", p.task.Branch, "--", "_lym/parent-only.txt"); tracked != "" {
			t.Errorf("origin holds %q; want the source's records absent", tracked)
		}
	})

	t.Run("UnsettledMergeReadsRecordsUnsettled", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		p.stopMergeBeforeRecordsStep(t)

		state, err := fabricengine.MidMerge(p.loc)

		if err != nil || state.Kind != fabricengine.MidMergeRecordsUnsettled || state.Verb != fabricengine.MergeVerbMergeIn || state.Source != p.parent {
			t.Errorf("MidMerge() = (%+v, %v); want a records-unsettled merge-in of %s", state, err, p.parent)
		}
	})

	t.Run("FabricMergeAbortOfAnUnsettledMergeKeepsAReceivingEdit", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		f := p.stopMergeBeforeRecordsStep(t)
		writeOneRepositoryFile(t, p.task.Path, "_lym/receiving.txt", "receiving\n")

		if _, err := f.MergeAbort(); err != nil {
			t.Fatalf("MergeAbort() error = %v; want the abort to pass", err)
		}

		if got := gitkit.Git(t, p.task.Path, "show", "HEAD:_lym/receiving.txt"); got != "receiving" {
			t.Errorf("_lym/receiving.txt at HEAD = %q; want the receiving edit kept", got)
		}
		p.requireMergeGoneWithoutSourceRecords(t, f)
	})

	t.Run("ContinueOfAnUnsettledMergeRefuses", func(t *testing.T) {
		t.Parallel()
		p := newRecordsMergePair(t)
		f := p.stopMergeBeforeRecordsStep(t)

		_, err := f.MergeContinue("")

		var refusal *fabricengine.MergeGuardError
		if !errors.As(err, &refusal) || !strings.Contains(err.Error(), "merge records step unfinished") {
			t.Fatalf("MergeContinue() error = %v; want a guard refusal for the unfinished records step", err)
		}
		if !strings.Contains(err.Error(), `lym fabric merge --abort`) {
			t.Errorf("refusal = %q; want it to name `lym fabric merge --abort`", err)
		}
	})

	t.Run("TwoRepositoryParkedMergeContinues", func(t *testing.T) {
		t.Parallel()
		h := hubforge.NewHub(t, ".")
		prime := h.PrimeWorktree()
		gitkit.CommitFile(t, prime, "code.txt", "base\n", "track the code file")
		task := hubforge.AddPairWith(t, h, "task", fabricengine.AddOptions{SkipPush: true})
		gitkit.CommitFile(t, task.Path, "code.txt", "task\n", "task edit of the code file")
		gitkit.CommitFile(t, prime, "code.txt", "parent\n", "parent edit of the code file")
		f := openOneRepositoryFabric(t, task.Path)
		loc, err := lymcwd.ResolveWorktree(task.Path)
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", task.Path, err)
		}

		if res, err := f.MergeIn(gitkit.CurrentBranch(t, prime)); err != nil || len(res.Conflicts) == 0 {
			t.Fatalf("MergeIn() = (conflicts %v, %v); want the merge parked on a conflict", res.Conflicts, err)
		}
		if state, err := fabricengine.MidMerge(loc); err != nil || state.Kind != fabricengine.MidMergeParked {
			t.Fatalf("MidMerge() = (%+v, %v); want a parked merge", state, err)
		}
		writeOneRepositoryFile(t, task.Path, "code.txt", "resolved\n")
		gitkit.MustRun(t, task.Path, "git", "add", "--", "code.txt")

		res, err := f.MergeContinue("")

		if err != nil || !res.Committed {
			t.Errorf("MergeContinue() = (committed %v, %v); want the parked merge concluded", res.Committed, err)
		}
	})
}
