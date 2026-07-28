// push.go implements the push surface: Push (a single synchronous push with
// rebase-retry resilience) and PushCoalesced (a single-pusher lock plus one
// guarded push, coalescing across processes via the lock queue rather than
// an internal retry loop). Both are push-only; committing is always the
// caller's separate StageAndCommit or StageAllAndCommit call.

package gitrepo

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"

	"github.com/Knatte18/loomyard/internal/lock"
)

// PushLockFileName is the pinned, repo-agnostic name of the single-pusher lock
// file PushCoalesced acquires in the repo's worktree root (discussion.md's
// "Lock ownership" decision). gitrepo manages no .gitignore entry for it:
// StageAndCommit only ever stages an explicit file list, so this lock file
// is never staged or committed regardless of whether a caller ignores it. It
// is exported so a consumer whose own verbs gate on worktree cleanliness
// (fabric's remove dirty gate) can git-exclude the artifact instead of
// hardcoding the literal.
const PushLockFileName = ".gitrepo-push.lock"

// rebaseRetryTriggers are the git-push stderr substrings that mean the
// remote has commits this checkout lacks — a recoverable rejection, not a
// genuine failure — so pushWithRebaseRetry attempts one pull --rebase before
// retrying.
var rebaseRetryTriggers = []string{"non-fast-forward", "rejected", "fetch first"}

// Push runs a single git push, transparently recovering from exactly one
// non-fast-forward-style rejection via pull --rebase before retrying.
//
// Rebase-retry precondition: git pull --rebase aborts if the worktree has
// dirty tracked files ("cannot pull with rebase: unstaged changes"). Push
// never stages or stashes on the caller's behalf — StageAndCommit is always
// the caller's separate, prior step — so a clean tree with respect to
// tracked files is the caller's responsibility for the rebase-retry path to
// recover; gitrepo does not auto-stash.
//
// SHA invalidation: when the retry path fires, the rebase rewrites the local
// commits it replays, so any SHA captured before Push — StageAndCommit's
// return value in particular — may no longer name a commit on the branch
// after a SUCCESSFUL push. SHAExists does not catch this (the pre-rebase
// object survives locally via the reflog), so a stale SHA recorded via
// SetSnapshotSHA would push an off-history snapshot. Callers must re-read
// CurrentSHA after a successful Push before recording any SHA.
func (r *Repo) Push() error {
	return r.pushWithRebaseRetry()
}

// pushWithRebaseRetry runs git push and, on a rejection matching
// rebaseRetryTriggers, runs git pull --rebase once and retries the push —
// aborting the rebase and returning an error if the rebase itself fails. Any
// other push failure returns an error including git's stderr. This is the
// shared retry core reused by both Push and PushCoalesced's guarded push.
//
// Every push attempt passes -c push.autoSetupRemote=true so a checkout with
// no upstream configured yet (the very first push of a branch) still
// succeeds and establishes the tracking branch, matching the no-upstream
// treated-as-unpushed contract documented on hasUnpushed — without
// gitrepo needing to know the branch or remote name to set it explicitly.
func (r *Repo) pushWithRebaseRetry() error {
	_, stderr, code, err := r.run("-c", "push.autoSetupRemote=true", "push")
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}

	if !containsAny(stderr, rebaseRetryTriggers) {
		return fmt.Errorf("gitrepo: git push: %s", stderr)
	}

	// The remote has commits we don't; rebase our commits on top and retry
	// the push once. A rebase failure (e.g. the dirty-tracked-file
	// precondition above is violated) must not leave a rebase in progress.
	_, rebaseStderr, rebaseCode, err := r.run("pull", "--rebase")
	if err != nil {
		return err
	}
	if rebaseCode != 0 {
		// When pull failed before a rebase ever started (dirty tree, no
		// tracking information), the abort itself fails with "no rebase in
		// progress" — expected and ignorable. Any other abort failure means
		// the worktree may genuinely be left mid-rebase, which the returned
		// error must say instead of implying a clean state.
		_, abortStderr, abortCode, abortErr := r.run("rebase", "--abort")
		if abortErr != nil {
			return fmt.Errorf("gitrepo: git pull --rebase: %s (and rebase --abort could not run, repository may be left mid-rebase: %v)", rebaseStderr, abortErr)
		}
		if abortCode != 0 && !strings.Contains(strings.ToLower(abortStderr), "no rebase in progress") {
			return fmt.Errorf("gitrepo: git pull --rebase: %s (and rebase --abort failed, repository may be left mid-rebase: %s)", rebaseStderr, abortStderr)
		}
		return fmt.Errorf("gitrepo: git pull --rebase: %s", rebaseStderr)
	}

	_, stderr, code, err = r.run("-c", "push.autoSetupRemote=true", "push")
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("gitrepo: git push (retry after rebase): %s", stderr)
	}
	return nil
}

// containsAny reports whether s contains any of substrs.
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// PushCoalesced pushes whatever is currently unpushed under a single-pusher
// lock, giving cross-process coalescing: a burst of concurrent callers
// serializes on the lock, and each one that finds nothing unpushed once it
// acquires the lock returns immediately instead of pushing again. The
// coalescing is a single guarded push per lock acquisition, not an internal
// retry loop; git push itself sends every commit ahead of upstream
// atomically, and the lock queue is what turns a burst of writers into as
// few pushes as possible. An unbounded loop on hasUnpushed would spin
// forever if a push ever succeeded without configuring an upstream, since
// hasUnpushed would keep reporting true. The guarded push shares Push's
// rebase-retry, including its SHA
// invalidation caveat: re-read CurrentSHA after a successful call before
// recording any pre-call SHA (see Push).
func (r *Repo) PushCoalesced() error {
	l, err := lock.AcquireWriteLock(filepath.Join(r.path, PushLockFileName))
	if err != nil {
		return fmt.Errorf("gitrepo: acquire push lock: %w", err)
	}
	defer l.Release()

	unpushed, err := r.hasUnpushed()
	if err != nil {
		return err
	}
	if !unpushed {
		// Another pusher already pushed everything while we waited on the lock.
		return nil
	}
	return r.pushWithRebaseRetry()
}

// hasUnpushed reports whether HEAD is ahead of its upstream. When no
// upstream is configured yet it returns true, so the first push — which
// establishes the upstream tracking branch — still happens rather than
// being skipped as "nothing to do". Every failure — a failed handle open,
// and any go-git failure after a successful open — also returns (true, nil),
// matching the CLI's "any non-zero rev-list exit means true" posture,
// inverted relative to gitnativepoc's poc (which returned (false, err) on a
// walk failure): hasUnpushed can never itself surface an error, since
// PushCoalesced calls it immediately after a CLI push that may have just
// written new packs, and a (false, err) shape here would turn PushCoalesced
// from "attempt the push anyway" into a hard failure at
// boardengine/sync.go and fabricengine/weftgit.go. The CLI's own third
// outcome — a spawn failure, returned as (false, err) — has no go-git
// analogue: there is no process to fail to spawn, so that branch simply
// disappears rather than being ported.
//
// `@{u}` is unusable here: go-git's revision parser recognizes the syntax
// but ResolveRevision never implements the resulting AtUpstream case, so the
// upstream ref is resolved manually from branch config instead. Once
// resolved, HEAD's hash is compared against the upstream ref's hash directly
// first — the overwhelmingly common nothing-to-push case, a single
// comparison and no walk at all — before falling back to a full ancestry
// walk for the diverged/rebased case a plain hash comparison cannot
// classify. The upstream's full ancestor set (not merely its tip) is walked
// and passed as NewCommitPreorderIter's seenExternal map for the HEAD walk:
// seeding only the tip would wrongly report HEAD as ahead whenever HEAD is
// strictly behind upstream. Every CommitObject read behind both walks routes
// through the fingerprint-gated lookupObjectRetrying helper — not optional
// here, since this is one of the three methods the helper exists for: it
// swallows failure into true and so can never surface an "object not found"
// on its own, and PushCoalesced calls it immediately after CLI pushes that
// write packs.
func (r *Repo) hasUnpushed() (bool, error) {
	repo, err := r.goGit()
	if err != nil {
		return true, nil
	}

	r.goGitMu.RLock()
	symbolicHead, headErr := repo.Reference(plumbing.HEAD, false)
	r.goGitMu.RUnlock()
	if headErr != nil || symbolicHead.Type() != plumbing.SymbolicReference {
		// A detached HEAD or an unreadable ref has no branch to carry
		// tracking configuration, so there is nothing to compare against —
		// treat it the same as "no upstream configured".
		return true, nil
	}
	branch := symbolicHead.Target().Short()

	r.goGitMu.RLock()
	cfg, cfgErr := repo.Config()
	r.goGitMu.RUnlock()
	if cfgErr != nil {
		return true, nil
	}
	b, ok := cfg.Branches[branch]
	if !ok || b.Remote == "" || b.Merge == "" {
		return true, nil
	}

	r.goGitMu.RLock()
	upstreamRef, upstreamErr := repo.Reference(plumbing.NewRemoteReferenceName(b.Remote, b.Merge.Short()), true)
	r.goGitMu.RUnlock()
	if upstreamErr != nil {
		// Tracking is configured but the remote-tracking ref itself is
		// missing locally (never fetched, for instance) — nothing to
		// compare against yet, so fall back to the same "no upstream"
		// treatment gitrepo's `@{u}..HEAD` failure path uses.
		return true, nil
	}

	r.goGitMu.RLock()
	head, headHashErr := repo.Head()
	r.goGitMu.RUnlock()
	if headHashErr != nil {
		return true, nil
	}

	if head.Hash() == upstreamRef.Hash() {
		return false, nil
	}

	headCommit, err := lookupObjectRetrying(r, repo, func() (*object.Commit, error) {
		return repo.CommitObject(head.Hash())
	})
	if err != nil {
		return true, nil
	}
	upstreamCommit, err := lookupObjectRetrying(r, repo, func() (*object.Commit, error) {
		return repo.CommitObject(upstreamRef.Hash())
	})
	if err != nil {
		return true, nil
	}

	// Walk the full history reachable from upstream first, so the HEAD walk
	// below can exclude everything upstream can already reach — not just the
	// upstream tip itself — reproducing @{u}..HEAD's set-difference rather
	// than a single-hash exclusion.
	reachableFromUpstream := make(map[plumbing.Hash]bool)
	upstreamIter := object.NewCommitPreorderIter(upstreamCommit, nil, nil)
	if err := upstreamIter.ForEach(func(c *object.Commit) error {
		reachableFromUpstream[c.Hash] = true
		return nil
	}); err != nil {
		return true, nil
	}

	// storer.ErrStop terminates the walk without ForEach reporting it as a
	// failure (see plumbing/object/commit_walker.go) — the walk only ever
	// needs to know whether the ahead-of-upstream set is empty, never its
	// size, so it stops at the first commit found.
	ahead := false
	iter := object.NewCommitPreorderIter(headCommit, reachableFromUpstream, nil)
	if err := iter.ForEach(func(*object.Commit) error {
		ahead = true
		return storer.ErrStop
	}); err != nil {
		return true, nil
	}
	return ahead, nil
}
