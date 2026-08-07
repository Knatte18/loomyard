//go:build integration

// commitempty_integration_test.go covers Repo.CommitEmpty against real git
// repositories, reusing gitrepo_test.go's fixture helpers (newRepo, writeFile,
// commitAll, runGit) rather than inventing a new harness.

package gitrepo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// treeSHA resolves rev's tree object SHA in dir, used to assert an empty
// commit's tree is byte-identical to its parent's.
func treeSHA(t *testing.T, dir, rev string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, dir, "rev-parse", rev+"^{tree}")
	if err != nil {
		t.Fatalf("git rev-parse %s^{tree} error = %v", rev, err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse %s^{tree} exited %d: %s", rev, code, stderr)
	}
	return strings.TrimSpace(stdout)
}

// TestCommitEmpty_BornHEAD_CleanIndex_MatchesParentTree asserts the case batch 4's
// empty-commits-take-over-the-correspondence-entry decision rests on: an empty commit's tree must
// be byte-identical to its parent's, never merely similar, or resolving a revert target to it would
// silently restore a different fabric tree.
func TestCommitEmpty_BornHEAD_CleanIndex_MatchesParentTree(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	parent, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	sha, err := repo.CommitEmpty("empty snapshot commit")
	if err != nil {
		t.Fatalf("CommitEmpty() error = %v; want nil", err)
	}
	if sha == "" {
		t.Fatal("CommitEmpty() sha = \"\"; want non-empty")
	}
	if !repo.SHAExists(sha) {
		t.Errorf("SHAExists(%q) = false; want true for CommitEmpty's returned sha", sha)
	}

	if got, want := treeSHA(t, dir, sha), treeSHA(t, dir, parent); got != want {
		t.Errorf("CommitEmpty() tree = %q; want parent's tree %q (an empty commit must change no content)", got, want)
	}
}

// TestCommitEmpty_TwoSuccessiveCalls_ProduceDistinctSHAs pins that an empty commit is never
// deduplicated into a no-op: CommitEmpty always commits when it commits at all, unlike
// StageAndCommit's no-op signal on unchanged content.
func TestCommitEmpty_TwoSuccessiveCalls_ProduceDistinctSHAs(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	first, err := repo.CommitEmpty("first empty commit")
	if err != nil {
		t.Fatalf("CommitEmpty() error = %v; want nil", err)
	}

	second, err := repo.CommitEmpty("second empty commit")
	if err != nil {
		t.Fatalf("CommitEmpty() error = %v; want nil", err)
	}

	if first == second {
		t.Fatalf("CommitEmpty() called twice returned %q both times; want two distinct commits", first)
	}
}

// TestCommitEmpty_UnbornHEAD_CleanIndex_CreatesEmptyRootCommit asserts a specified contract, not
// incidental behaviour: fabricengine reaches this path whenever fabric has no commits yet.
func TestCommitEmpty_UnbornHEAD_CleanIndex_CreatesEmptyRootCommit(t *testing.T) {
	dir, repo := newRepo(t)

	if _, err := repo.CurrentSHA(); !errors.Is(err, gitrepo.ErrNoCommits) {
		t.Fatalf("CurrentSHA() before CommitEmpty() error = %v; want errors.Is(err, ErrNoCommits) (test needs an unborn HEAD)", err)
	}

	sha, err := repo.CommitEmpty("root empty commit")
	if err != nil {
		t.Fatalf("CommitEmpty() error = %v; want nil", err)
	}
	if sha == "" {
		t.Fatal("CommitEmpty() sha = \"\"; want non-empty")
	}

	// A root commit has no parent to resolve.
	if _, _, code, _ := runGit(t, dir, "rev-parse", "--verify", "-q", sha+"^"); code == 0 {
		t.Errorf("git rev-parse -q %s^ succeeded; want %s to be a root commit with no parent", sha, sha)
	}

	shown, stderr, code, err := runGit(t, dir, "show", "--name-only", "--format=", sha)
	if err != nil {
		t.Fatalf("git show error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git show exited %d: %s", code, stderr)
	}
	if strings.TrimSpace(shown) != "" {
		t.Errorf("git show --name-only %s = %q; want empty (no content)", sha, shown)
	}
}

// TestCommitEmpty_UnbornHEAD_StagedFile_ReturnsErrIndexNotEmpty is the only exercise of the git
// ls-files --cached branch: on an unborn HEAD there is no HEAD tree for diff --cached to compare
// against, so this is the case that proves the pre-check was specified for both states, not only
// the born one.
func TestCommitEmpty_UnbornHEAD_StagedFile_ReturnsErrIndexNotEmpty(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "wip.txt", "half-staged WIP")
	lyxtest.MustRun(t, dir, "git", "add", "wip.txt")

	sha, err := repo.CommitEmpty("must not happen")
	if !errors.Is(err, gitrepo.ErrIndexNotEmpty) {
		t.Fatalf("CommitEmpty() error = %v; want errors.Is(err, ErrIndexNotEmpty)", err)
	}
	if sha != "" {
		t.Errorf("CommitEmpty() sha = %q; want \"\"", sha)
	}

	if _, err := repo.CurrentSHA(); !errors.Is(err, gitrepo.ErrNoCommits) {
		t.Errorf("CurrentSHA() after refused CommitEmpty() error = %v; want errors.Is(err, ErrNoCommits) (HEAD must still be unborn)", err)
	}
}

// TestCommitEmpty_BornHEAD_StagedFile_ReturnsErrIndexNotEmpty asserts the never-sweep intent holds
// on the ordinary (born-HEAD) path too, not only the unborn one.
func TestCommitEmpty_BornHEAD_StagedFile_ReturnsErrIndexNotEmpty(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	headBefore, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	writeFile(t, dir, "wip.txt", "half-staged WIP")
	lyxtest.MustRun(t, dir, "git", "add", "wip.txt")

	sha, err := repo.CommitEmpty("must not happen")
	if !errors.Is(err, gitrepo.ErrIndexNotEmpty) {
		t.Fatalf("CommitEmpty() error = %v; want errors.Is(err, ErrIndexNotEmpty)", err)
	}
	if sha != "" {
		t.Errorf("CommitEmpty() sha = %q; want \"\"", sha)
	}

	headAfter, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("HEAD after refused CommitEmpty() = %q; want unchanged %q", headAfter, headBefore)
	}
}
