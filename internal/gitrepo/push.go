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
// being skipped as "nothing to do". A spawn failure returns (false, err);
// any other non-zero rev-list exit (no upstream, an upstream configured but
// never fetched, or any other rev-list failure) folds into (true, nil), so
// the caller still attempts the push.
//
// # Stays CLI-bound — measured, not migrated (card 21's reversal criterion)
//
// This method was migrated to a go-git ancestry walk in batch 4 (equal-hash
// shortcut first, then a full walk of upstream's ancestor set seeded as
// NewCommitPreorderIter's seenExternal for the HEAD walk) and reverted here
// after measuring it against the CLI spawn it replaced, per the Shared
// Decision's reversal criterion: if go-git is slower than the spawn it
// replaces, hasUnpushed reverts to the CLI, because it is one cheap command
// on PushCoalesced's hottest path and no principle here is worth a
// regression there.
//
// Measured on a real local clone of this checkout's own history
// (268 commits reachable from HEAD, not a handful-of-commits fixture),
// 20 trials per case, each go-git trial against a freshly-opened handle so
// the handle cache never masks the cost:
//
//   - Equal-hash shortcut (HEAD already equals upstream, the common
//     nothing-to-push case): go-git avg 239µs (total 4.79ms/20) vs the
//     CLI's single `rev-list --count` spawn avg 1.42ms (total 28.4ms/20) —
//     go-git about 6x faster here, as expected with no process to spawn.
//   - Walking path (HEAD one commit ahead of upstream, forcing the full
//     268-commit ancestor walk with no early exit): go-git avg 20.4ms
//     (total 408ms/20) vs the CLI avg 2.16ms (total 43.2ms/20) — go-git
//     about 9.4x SLOWER here. This is the path PushCoalesced actually hits
//     on every round with real unpushed work, so it is the number the
//     reversal criterion is judged on, not the shortcut.
//
// The walking path measurement is what fires the reversal: `git rev-list
// --count` is a single optimized C-level walk, while go-git's
// NewCommitPreorderIter decodes and parses every commit object it visits in
// Go, with no early exit across the upstream side of the walk regardless of
// how quickly the HEAD side would otherwise terminate. hasUnpushed therefore
// stays on the CLI, rejoining the pinned r.run list the boundary guard
// asserts and the CLI-bound set CONSTRAINTS.md's gitrepo Client Boundary
// Invariant names.
func (r *Repo) hasUnpushed() (bool, error) {
	stdout, _, code, err := r.run("rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		return false, err
	}
	if code != 0 {
		// No upstream configured (or some other rev-list failure); either
		// way, treat it as unpushed so the caller still attempts the push.
		return true, nil
	}
	return strings.TrimSpace(stdout) != "0", nil
}
