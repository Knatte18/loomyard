// pushanchored.go holds PushAnchored, the fabric-vocabulary-neutral synchronous push beside
// CommitAnchoredPaths' commit — the two entry points that let a caller outside the Fabric
// Vocabulary Invariant's owner set commit and push into the weft sibling without ever naming a
// weft path.

package fabricengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// PushAnchored pushes unpushed commits in l's weft sibling worktree directly via
// gitrepo.PushRebaseFree, taking no lock and resolving its target the same way
// CommitAnchoredPaths does — from l alone, via WeftWorktree(l) — so a caller outside the Fabric
// Vocabulary Invariant's owner set never learns the weft exists.
//
// Its underlying primitive is gitrepo.PushRebaseFree, never gitrepo.PushCoalesced, for the same two
// reasons PushWarpRebaseFreeAt already chose it over PushWarpAt: PushCoalesced's rejected-push
// retry path runs `git pull --rebase`, rewriting this side's SHAs and invalidating the
// correspondence index out from under a running weft, and it takes a repo-root push-lock file that
// would contend with SpawnDetachedPush children and landing-time pushes on every transition. Taking
// no lock here is deliberate, not an oversight: PushRebaseFree is lock-free by construction, which
// is half of why it was chosen, matching PushWarpAt/PushWarpRebaseFreeAt, neither of which takes a
// recorder parameter either — PushResult already embeds MutationRecord, so the Mutation Record
// Invariant is satisfied without one.
//
// A rejected push surfaces gitrepo.ErrPushRejected UNWRAPPED — this is load-bearing, not
// incidental. gitrepo.PushRebaseFree returns that sentinel bare rather than wrapped with %w, and the
// loom-side per-transition closure this function was added for matches exactly that sentinel with
// errors.Is to warn and continue on a routine rejection while treating every other push error as
// fatal; wrapping it here would silently turn that routine rejection into a run-halting error.
//
// Returns (PushResult{}, nil) immediately, with no lock taken and nothing recorded, when
// opts.SkipGit or opts.SkipPush is true.
func PushAnchored(l *lyxcwd.Location, opts SyncOptions) (res PushResult, err error) {
	target := WeftWorktree(l)
	rec := NewMutations(filepath.Dir(target))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	repo := gitrepo.New(target)
	hadUnpushed, hadUnpushedErr := repo.HasUnpushed()
	if err := repo.PushRebaseFree(); err != nil {
		return PushResult{}, err
	}
	recordPushIfAdvanced(rec, repo, hadUnpushed, hadUnpushedErr)

	return PushResult{}, nil
}
