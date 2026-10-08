// warpforward.go collects the warp-only forwarding methods on *Fabric: thin, one-line delegations
// to the paired gitrepo.Repo verb on f.code, added so an out-of-package caller can invoke a
// warp-mutating git verb through Fabric's public API — preserving the one-repo illusion — without
// ever touching f.code directly.
// Kept in its own file rather than folded into fabric.go to keep the delegation cluster isolated
// from Fabric's construction and cross-repo plumbing.

package fabricengine

import (
	"sync"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// CurrentBranch returns the short name of the branch the warp checkout's HEAD currently points at.
// It is a thin delegation to gitrepo.Repo.CurrentBranch on f.code,
// and inherits that method's rejection of detached HEAD (returns wrapped error).
func (f *Fabric) CurrentBranch() (string, error) {
	return f.code.CurrentBranch()
}

// HeadSHA returns the full commit SHA the warp checkout's HEAD currently points at.
// It is a thin, read-only delegation to gitrepo.Repo.CurrentSHA on f.code,
// so a caller reads a commit without naming a fabric side.
func (f *Fabric) HeadSHA() (string, error) {
	return f.code.CurrentSHA()
}

// IsAncestor reports whether sha is an ancestor of ref in the warp checkout.
// It is a thin, read-only delegation to gitrepo.Repo.IsAncestor on f.code.
func (f *Fabric) IsAncestor(sha, ref string) (bool, error) {
	return f.code.IsAncestor(sha, ref)
}

// HeadContains reports whether sha is the commit at HEAD of the worktree at worktreePath, or one of its ancestors.
// It reads in-process through gitrepo.Repo.HeadContains and takes a path rather than a *Fabric, so a caller running from any worktree of a hub, paired or not, can ask.
func HeadContains(worktreePath, sha string) (bool, error) {
	return gitrepo.New(worktreePath).HeadContains(sha)
}

// StencilSource builds the stencil seed pass's Source for the worktree at worktreePath.
// An empty sourceDir returns the zero Source, and an empty revision (an unstamped binary) returns a Source with no Build.
// Otherwise Build asks HeadContains for revision at most once per Source, so a pass with several drifted stencils reads ancestry once and a pass with none runs no git.
// A failed read is logged and reported as unknown ancestry.
func StencilSource(worktreePath, sourceDir, revision string) stencilstore.Source {
	if sourceDir == "" {
		return stencilstore.Source{}
	}
	if revision == "" {
		return stencilstore.Source{Dir: sourceDir}
	}

	var once sync.Once
	ancestry := stencilstore.BuildAncestryUnknown
	return stencilstore.Source{
		Dir: sourceDir,
		Build: func() stencilstore.BuildAncestry {
			once.Do(func() {
				held, err := HeadContains(worktreePath, revision)
				switch {
				case err != nil:
					logger.Warn("fabricengine: reading whether the worktree holds the build commit failed", "worktree", worktreePath, "revision", revision, "error", err)
				case held:
					ancestry = stencilstore.BuildInHead
				default:
					ancestry = stencilstore.BuildNotInHead
				}
			})
			return ancestry
		},
	}
}

// ResetHard has moved to destroy.go, where it becomes the gated executor for the ResetHard
// primitive — see that file's own doc comment. It is not a thin delegation like its neighbours
// above: there is exactly one correct ownership/dirtiness declaration for "reset this Fabric's warp
// checkout", so the gated wrapper hardcodes it rather than exposing it, and it lives beside the rest
// of the destructive surface instead of here.
