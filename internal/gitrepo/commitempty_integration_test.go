//go:build integration

// commitempty_integration_test.go covers Repo.CommitEmpty against a real git
// repository, reusing gitrepo_test.go's fixture helpers (newRepo, headFilesOf,
// requireCurrentSHA, runGit) rather than inventing a new harness.

package gitrepo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
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

// TestCommitEmpty drives CommitEmpty through one repository from an unborn HEAD to a born one.
// The steps run serially in one order and share the repository's state: the root-commit step relies
// on the staged entry the refusal step removes from the index, and the born-HEAD steps rely on the
// root commit and on a.txt committed by the step that opens them.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestCommitEmpty(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)

	// The only exercise of the git ls-files --cached branch: on an unborn HEAD there is no HEAD
	// tree for diff --cached to compare against, so this is the case that proves the pre-check was
	// specified for both states, not only the born one.
	if !t.Run("unborn HEAD with a staged file returns ErrIndexNotEmpty", func(t *testing.T) {
		writeFile(t, dir, "wip.txt", "half-staged WIP")
		gitkit.MustRun(t, dir, "git", "add", "wip.txt")

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
	}) {
		return
	}

	// A specified contract, not incidental behaviour: fabricengine reaches this path whenever
	// fabric has no commits yet.
	// Relies on the previous step's staged wip.txt, which this step unstages to get a clean index.
	if !t.Run("unborn HEAD with a clean index creates an empty root commit", func(t *testing.T) {
		gitkit.MustRun(t, dir, "git", "rm", "-f", "--cached", "wip.txt")

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

		if shown := headFilesOf(t, dir); shown != "" {
			t.Errorf("git show --name-only %s = %q; want empty (no content)", sha, shown)
		}
	}) {
		return
	}

	// The case the empty-commits-take-over-the-correspondence-entry decision rests on: an empty
	// commit's tree must be byte-identical to its parent's, never merely similar, or resolving a
	// revert target to it would silently restore a different fabric tree.
	if !t.Run("born HEAD with a clean index matches the parent tree", func(t *testing.T) {
		gitkit.CommitFile(t, dir, "a.txt", "initial", "init")
		parent := requireCurrentSHA(t, repo)

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
	}) {
		return
	}

	// An empty commit is never deduplicated into a no-op: CommitEmpty always commits when it
	// commits at all, unlike StageAndCommit's no-op signal on unchanged content.
	if !t.Run("two successive calls produce distinct shas", func(t *testing.T) {
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
	}) {
		return
	}

	// The never-sweep intent holds on the ordinary (born-HEAD) path too, not only the unborn one.
	t.Run("born HEAD with a staged file returns ErrIndexNotEmpty", func(t *testing.T) {
		headBefore := requireCurrentSHA(t, repo)
		gitkit.MustRun(t, dir, "git", "add", "wip.txt")

		sha, err := repo.CommitEmpty("must not happen")
		if !errors.Is(err, gitrepo.ErrIndexNotEmpty) {
			t.Fatalf("CommitEmpty() error = %v; want errors.Is(err, ErrIndexNotEmpty)", err)
		}
		if sha != "" {
			t.Errorf("CommitEmpty() sha = %q; want \"\"", sha)
		}

		if headAfter := requireCurrentSHA(t, repo); headAfter != headBefore {
			t.Errorf("HEAD after refused CommitEmpty() = %q; want unchanged %q", headAfter, headBefore)
		}
	})
}
