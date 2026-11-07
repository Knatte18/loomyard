// warpforward.go holds the warp-only forwarding methods on *Fabric.
// Most of them delegate to gitrepo.Repo verbs on f.code,
// so an out-of-package caller can run warp git verbs through Fabric's public API.
// That keeps the one-repo illusion, since no caller touches f.code directly.
// The methods sit apart from fabric.go's construction and cross-repo plumbing.

package fabricengine

import (
	"sync"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// CurrentBranch returns the short name of the branch the warp checkout's HEAD currently points at.
// It returns an error when HEAD is detached.
func (f *Fabric) CurrentBranch() (string, error) {
	return f.code.CurrentBranch()
}

// HeadSHA returns the full commit SHA the warp checkout's HEAD currently points at.
// It is read-only.
func (f *Fabric) HeadSHA() (string, error) {
	return f.code.CurrentSHA()
}

// IsAncestor reports whether sha is an ancestor of ref in the warp checkout.
// It is read-only.
func (f *Fabric) IsAncestor(sha, ref string) (bool, error) {
	return f.code.IsAncestor(sha, ref)
}

// CommitExists reports whether the warp checkout holds the commit sha.
// It is read-only, and an invalid SHA or a read error reports false.
func (f *Fabric) CommitExists(sha string) bool {
	return f.code.SHAExists(sha)
}

// MergeBase returns the SHA of the best common ancestor of the warp checkout's HEAD and ref.
// ref is a branch, a remote-tracking ref or a SHA.
// It is read-only; a caller uses it to read a task branch's fork point.
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
// It takes a path rather than a *Fabric, so a caller in any worktree of a hub, paired or not, can ask.
func HeadContains(worktreePath, sha string) (bool, error) {
	return gitrepo.New(worktreePath).HeadContains(sha)
}

// StencilSource builds the stencil seed pass's Source for worktreePath, given the running binary's identity.
// A running identity with a revision sets Writer to its revision and commit time.
// Older orders by ancestry when the worktree holds both commits; it answers RecordedOlder only for a strict ancestor.
// It orders by commit time when the worktree lacks either commit.
// It answers RecordedOlder only when the recorded time is strictly before the running time and both are set.
// Otherwise, or on a failed read (logged once), it answers OrderingUnknown.
// An empty sourceDir returns a Source with no Dir and no Build, which keeps the drift warning silent,
// and an empty revision returns a Source with no Build.
// Build asks HeadContains for the revision at most once per Source,
// so a pass with several drifted stencils reads ancestry once and a pass with none runs no git.
// A failed Build read is logged and reported as unknown ancestry.
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

// ResetHard is not a thin delegation: its gated executor lives in destroy.go.
// It sits beside the rest of the destructive surface.
// Resetting this Fabric's warp checkout has exactly one correct ownership and dirtiness declaration,
// so the gated wrapper hardcodes it rather than exposing it.
