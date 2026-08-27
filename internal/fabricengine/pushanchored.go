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
// A rejected push surfaces gitrepo.ErrPushRejected UNWRAPPED: gitrepo.PushRebaseFree returns that
// sentinel bare rather than wrapped with %w, and this function passes it straight through so a
// caller CAN discriminate a routine rejection from every other push failure with errors.Is.
// Which callers actually do so is a separate question, and stating it precisely matters because an
// earlier version of this comment asserted a discrimination that no caller performed: the loom-side
// per-transition closure this function was added for (internal/loomcli's newCommitStatusSeam) warns
// and continues on EVERY push error, rejection or not, per the commit-hard-errors-push-warns
// decision — an offline laptop must not kill an autonomous run either. The unwrapped sentinel is
// therefore a property this package preserves and pins (pushanchored_integration_test.go), not one
// any current consumer's control flow depends on.
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
