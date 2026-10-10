// warpforward.go collects the warp-only forwarding methods on *Fabric: thin, one-line delegations
// to the paired gitrepo.Repo verb on f.code, added so an out-of-package caller can invoke a
// warp-mutating git verb through Fabric's public API — preserving the one-repo illusion — without
// ever touching f.code directly.
// Kept in its own file rather than folded into fabric.go to keep the delegation cluster isolated
// from Fabric's construction and cross-repo plumbing.

package fabricengine

import (
	"sync"

	"github.com/Knatte18/loomyard/internal/buildvcs"
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

// MergeBase returns the SHA of the best common ancestor of the warp checkout's HEAD and ref, where ref is a branch, a remote-tracking ref or a SHA.
// It is a read-only delegation to gitrepo.Repo.MergeBase on f.code, so a caller reads the task branch's fork point without naming a fabric side.
func (f *Fabric) MergeBase(ref string) (string, error) {
	head, err := f.code.CurrentSHA()
	if err != nil {
		return "", err
	}
	refSHA, err := f.code.ResolveSHA(ref)
	if err != nil {
		return "", err
	}
	return f.code.MergeBase(head, refSHA)
}

// HeadContains reports whether sha is the commit at HEAD of the worktree at worktreePath, or one of its ancestors.
// It reads in-process through gitrepo.Repo.HeadContains and takes a path rather than a *Fabric, so a caller running from any worktree of a hub, paired or not, can ask.
func HeadContains(worktreePath, sha string) (bool, error) {
	return gitrepo.New(worktreePath).HeadContains(sha)
}

// StencilSource builds the stencil seed pass's Source for the worktree at worktreePath, told the running binary's identity.
// A running identity with a revision sets Writer to its revision and commit time, and Older to the ordering of a recorded writer against it.
// Older orders by ancestry when the worktree holds both commits, answering RecordedOlder only for a strict ancestor.
// It orders by commit time when the worktree lacks either, answering RecordedOlder only when the recorded time is strictly before the running time and both are set.
// Otherwise, or on a failed read (logged once), it answers OrderingUnknown.
// An empty sourceDir returns a Source with no Dir and no Build, which keeps the drift warning silent, and an empty revision returns a Source with no Build.
// Otherwise Build asks HeadContains for the revision at most once per Source, so a pass with several drifted stencils reads ancestry once and a pass with none runs no git.
// A failed read is logged and reported as unknown ancestry.
func StencilSource(worktreePath, sourceDir string, running buildvcs.Identity) stencilstore.Source {
	source := stencilstore.Source{Dir: sourceDir}
	if running.Revision == "" {
		return source
	}

	source.Writer = stencilstore.Writer{Revision: running.Revision, Time: running.Time}

	var orderLogOnce sync.Once
	source.Older = func(recorded stencilstore.Writer) stencilstore.Ordering {
		repo := gitrepo.New(worktreePath)
		if repo.SHAExists(recorded.Revision) && repo.SHAExists(running.Revision) {
			ancestor, err := repo.IsAncestor(recorded.Revision, running.Revision)
			if err != nil {
				orderLogOnce.Do(func() {
					logger.Warn("fabricengine: reading whether the recorded build precedes the running build failed", "worktree", worktreePath, "recorded", recorded.Revision, "running", running.Revision, "error", err)
				})
				return stencilstore.OrderingUnknown
			}
			if ancestor && recorded.Revision != running.Revision {
				return stencilstore.RecordedOlder
			}
			return stencilstore.RecordedNotOlder
		}
		if recorded.Time.IsZero() || running.Time.IsZero() {
			return stencilstore.OrderingUnknown
		}
		if recorded.Time.Before(running.Time) {
			return stencilstore.RecordedOlder
		}
		return stencilstore.RecordedNotOlder
	}

	if sourceDir == "" {
		return source
	}

	var buildOnce sync.Once
	ancestry := stencilstore.BuildAncestryUnknown
	source.Build = func() stencilstore.BuildAncestry {
		buildOnce.Do(func() {
			held, err := HeadContains(worktreePath, running.Revision)
			switch {
			case err != nil:
				logger.Warn("fabricengine: reading whether the worktree holds the build commit failed", "worktree", worktreePath, "revision", running.Revision, "error", err)
			case held:
				ancestry = stencilstore.BuildInHead
			default:
				ancestry = stencilstore.BuildNotInHead
			}
		})
		return ancestry
	}
	return source
}

// ResetHard has moved to destroy.go, where it becomes the gated executor for the ResetHard
// primitive — see that file's own doc comment. It is not a thin delegation like its neighbours
// above: there is exactly one correct ownership/dirtiness declaration for "reset this Fabric's warp
// checkout", so the gated wrapper hardcodes it rather than exposing it, and it lives beside the rest
// of the destructive surface instead of here.
