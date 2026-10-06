// git.go declares Git, the git surface webster's verbs read a worktree through, and realGit, the implementation every production run uses.
// Geometry carries the Git a run is told.
// A nil one means realGit, so only a test that fakes git sets it.

package websterengine

import (
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/quarry/quarry"
)

// Git is every question webster's bracket verbs and run-level checks put to a worktree's git repository.
// Each method takes the worktree to ask about, so one Git serves a run and its linked worktrees alike.
type Git interface {
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
	// MergeRejection returns why the merge commit does not qualify as a clean merge of the run's parent branch, or "" when it does.
	// parents are the commit's own parents; a parentBranch that cannot be resolved is itself a rejection.
	MergeRejection(worktree, commit string, parents []string, parentBranch ParentBranchFunc) string
	// SHAExists reports whether sha names a commit in the worktree's repository.
	SHAExists(worktree, sha string) bool
	// IsAncestor reports whether sha is an ancestor of ref.
	IsAncestor(worktree, sha, ref string) (bool, error)
	// IgnoredPath reports whether git ignores path in the worktree.
	IgnoredPath(worktree, path string) (bool, error)
	// OtherWorktrees returns the canonical root of every worktree of the repository except the given one.
	OtherWorktrees(worktree string) ([]string, error)
	// PathDiffers reports whether path differs from base in the worktree: a changed tracked file or an untracked new one.
	PathDiffers(worktree, base, path string) (bool, error)
	// WorktreeBlob returns the blob id of path's content in the worktree, or "" when the file is absent.
	WorktreeBlob(worktree, path string) (string, error)
	// CommitBlob returns the blob id of path at commit, or "" when the commit does not hold the path.
	CommitBlob(worktree, commit, path string) (string, error)
	// TreePathsWithBlob returns, sorted, every path in commit's tree whose object id is blob.
	TreePathsWithBlob(worktree, commit, blob string) ([]string, error)
	// Delta returns what the commits after fromSHA up to toSHA created, modified, renamed and deleted.
	Delta(worktree, fromSHA, toSHA string) (quarry.GitDeltaAnswer, error)
}

// realGit is Git over the repository on disk, each method delegating to the helper in gitwrap.go that owns the git call.
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

func (realGit) IsAncestor(worktree, sha, ref string) (bool, error) {
	return isAncestor(worktree, sha, ref)
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

func (realGit) Delta(worktree, fromSHA, toSHA string) (quarry.GitDeltaAnswer, error) {
	return planglyph.Delta(worktree, fromSHA, toSHA)
}

// git returns the Git the geometry was told, or the real one when none was.
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
