// push.go implements the push surface: Push (a single synchronous push with
// rebase-retry resilience) and PushCoalesced (a single-pusher lock plus one
// guarded push — the board sync.go push-loop replacement, coalescing across
// processes via the lock queue rather than an internal retry loop). Both are
// push-only; committing is always the caller's separate StageAndCommit.

package gitrepo

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lock"
)

// pushLockFile is the pinned, repo-agnostic name of the single-pusher lock
// file PushCoalesced acquires in the repo's worktree root (discussion.md's
// "Lock ownership" decision). gitrepo manages no .gitignore entry for it:
// StageAndCommit only ever stages an explicit file list, so this lock file
// is never staged or committed regardless of whether a caller ignores it.
const pushLockFile = ".gitrepo-push.lock"

// rebaseRetryTriggers are the git-push stderr substrings that mean the
// remote has commits this checkout lacks — a recoverable rejection, not a
// genuine failure — so pushWithRebaseRetry attempts one pull --rebase before
// retrying. This is the full trigger set board's sync.go:pushUnpushed
// matches (not just git.go's smaller pair), required so PushCoalesced can
// fully replace that loop.
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
		r.run("rebase", "--abort")
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
// acquires the lock returns immediately instead of pushing again. This is
// the board sync.go push-loop replacement — the coalescing is a single
// guarded push per lock acquisition, not an internal retry loop; git push
// itself sends every commit ahead of upstream atomically, and the lock
// queue is what turns a burst of writers into as few pushes as possible. An
// unbounded loop on hasUnpushed would spin forever if a push ever succeeded
// without configuring an upstream, since hasUnpushed would keep reporting
// true.
func (r *Repo) PushCoalesced() error {
	l, err := lock.AcquireWriteLock(filepath.Join(r.path, pushLockFile))
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
// being skipped as "nothing to do".
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
