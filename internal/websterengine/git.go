// git.go declares Git, the questions webster's verbs ask about a worktree's git repository.
// realGit is the implementation every production run uses.
// A Geometry whose Git is nil uses realGit, so only a test that fakes git sets one.

package websterengine

import (
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// Git is every question webster's bracket verbs and run-level checks put to a worktree's git repository:
// the shared run-plan machinery's questions and webster's own.
// Each method takes the worktree to ask about, so one Git serves a run and its linked worktrees alike.
type Git interface {
	runplan.Git
	// HeadSHA returns the worktree's current HEAD commit SHA.
	HeadSHA(worktree string) (string, error)
	// Dirty reports whether the worktree has any uncommitted or untracked changes.
	Dirty(worktree string) (bool, error)
	// DirtyPaths returns the worktree-relative paths of every uncommitted or untracked change, empty for a clean tree.
	DirtyPaths(worktree string) ([]string, error)
	// MergeInProgress reports whether the worktree has a git merge in progress.
	MergeInProgress(worktree string) (bool, error)
	// CommitParents returns the parent SHAs of commit.
	CommitParents(worktree, commit string) ([]string, error)
	// MergeRejection returns why the merge commit is not a clean merge of the run's parent branch, or "" when it is.
	// parents lists the commit's parent SHAs; a parentBranch that cannot be resolved is itself a rejection.
	MergeRejection(worktree, commit string, parents []string, parentBranch ParentBranchFunc) string
	// CommitsNamedBy returns, sorted, the full SHA of every commit whose object name starts with prefix, empty when none does.
	CommitsNamedBy(worktree, prefix string) ([]string, error)
	// NonMergeCommitsBetween reports whether the first-parent range base..head holds a commit that is not a merge;
	// a base the repository does not hold is an error.
	NonMergeCommitsBetween(worktree, base, head string) (bool, error)
	// OtherWorktrees returns the canonical root of every worktree of the repository except the given one.
	OtherWorktrees(worktree string) ([]string, error)
}

// realGit implements Git over the repository on disk;
// each method delegates to the helper in gitwrap.go that owns the git call.
type realGit struct{}

var _ Git = realGit{}

func (realGit) HeadSHA(worktree string) (string, error) { return headSHA(worktree) }

func (realGit) Dirty(worktree string) (bool, error) { return dirty(worktree) }

func (realGit) DirtyPaths(worktree string) ([]string, error) { return verifytree.DirtyPaths(worktree) }

func (realGit) MergeInProgress(worktree string) (bool, error) { return mergeInProgress(worktree) }

func (realGit) CommitParents(worktree, commit string) ([]string, error) {
	return commitParents(worktree, commit)
}

func (realGit) MergeRejection(worktree, commit string, parents []string, parentBranch ParentBranchFunc) string {
	return mergeRejection(worktree, commit, parents, parentBranch)
}

func (realGit) SHAExists(worktree, sha string) bool { return shaExists(worktree, sha) }

func (realGit) CommitsNamedBy(worktree, prefix string) ([]string, error) {
	return commitsNamedBy(worktree, prefix)
}

func (realGit) IsAncestor(worktree, sha, ref string) (bool, error) {
	return isAncestor(worktree, sha, ref)
}

func (realGit) NonMergeCommitsBetween(worktree, base, head string) (bool, error) {
	return nonMergeCommitsBetween(worktree, base, head)
}

func (realGit) IgnoredPath(worktree, path string) (bool, error) { return ignoredPath(worktree, path) }

func (realGit) OtherWorktrees(worktree string) ([]string, error) { return otherWorktrees(worktree) }

func (realGit) PathDiffers(worktree, base, path string) (bool, error) {
	return worktreePathDiffers(worktree, base, path)
}

func (realGit) WorktreeBlob(worktree, path string) (string, error) {
	return worktreeBlob(worktree, path)
}

func (realGit) CommitBlob(worktree, commit, path string) (string, error) {
	return commitBlob(worktree, commit, path)
}

func (realGit) TreePathsWithBlob(worktree, commit, blob string) ([]string, error) {
	return treePathsWithBlob(worktree, commit, blob)
}

func (realGit) CommitLog(worktree, base, head string) ([]CommitLogEntry, error) {
	return commitLog(worktree, base, head)
}

// git returns the Git the geometry carries, or realGit when it carries none.
func (g Geometry) git() Git {
	return orReal(g.Git)
}

// orReal returns g, or the real Git when g is nil.
func orReal(g Git) Git {
	if g != nil {
		return g
	}
	return realGit{}
}
