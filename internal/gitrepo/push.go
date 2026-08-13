// push.go implements the push surface: Push (a single synchronous push with rebase-retry
// resilience), PushCoalesced (a single-pusher lock plus one guarded push, coalescing across
// processes via the lock queue rather than an internal retry loop), and PushRebaseFree (a single
// plain push that never rebases, for callers that supply their own serialization).
// All three are push-only; committing is always the caller's separate StageAndCommit or
// StageAllAndCommit call.

package gitrepo

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
)

// ErrPushRejected is returned by PushRebaseFree when push is rejected due to remote divergence.
// It is a distinguishable error, not a failure.
var ErrPushRejected = errors.New("gitrepo: push rejected (remote diverged)")

// PushLockFileName is the name of the single-pusher lock file PushCoalesced acquires in the repo's
// worktree root.
const PushLockFileName = ".gitrepo-push.lock"

// rebaseRetryTriggers are the git-push stderr substrings indicating the
// remote has commits this checkout lacks.
var rebaseRetryTriggers = []string{"non-fast-forward", "rejected", "fetch first"}

// Push runs git push, recovering from one non-fast-forward rejection via pull --rebase before
// retrying.
// The worktree must be clean.
// Callers must re-read CurrentSHA after Push if SHAs were captured beforehand.
func (r *Repo) Push() error {
	return r.pushWithRebaseRetry()
}

// pushWithRebaseRetry runs git push and on rebaseRetryTrigger matches, runs
// git pull --rebase once and retries, aborting if rebase fails. Sets
// push.autoSetupRemote=true so first push establishes tracking.
func (r *Repo) pushWithRebaseRetry() error {
	_, err := r.runChecked("-c", "push.autoSetupRemote=true", "push")
	if err == nil {
		return nil
	}

	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) || !containsAny(gitErr.Stderr, rebaseRetryTriggers) {
		return fmt.Errorf("gitrepo: git push: %w", err)
	}

	_, rebaseErr := r.runChecked("pull", "--rebase")
	if rebaseErr != nil {
		// The pull --rebase exit branch is control flow, not a message: only a
		// *GitError means git ran and rejected the rebase, which is what
		// justifies attempting rebase --abort below. Anything else means git
		// never ran, so an abort would be wrong and this returns immediately.
		var rebaseGitErr *gitexec.GitError
		if !errors.As(rebaseErr, &rebaseGitErr) {
			return rebaseErr
		}

		_, abortErr := r.runChecked("rebase", "--abort")
		var abortGitErr *gitexec.GitError
		switch {
		case abortErr == nil:
		case errors.As(abortErr, &abortGitErr):
			if !strings.Contains(strings.ToLower(abortGitErr.Stderr), "no rebase in progress") {
				return fmt.Errorf("gitrepo: git pull --rebase: %s (and rebase --abort failed, repository may be left mid-rebase: %s)", rebaseGitErr.Stderr, abortGitErr.Stderr)
			}
		default:
			return fmt.Errorf("gitrepo: git pull --rebase: %s (and rebase --abort could not run, repository may be left mid-rebase: %v)", rebaseGitErr.Stderr, abortErr)
		}
		return fmt.Errorf("gitrepo: git pull --rebase: %s", rebaseGitErr.Stderr)
	}

	_, err = r.runChecked("-c", "push.autoSetupRemote=true", "push")
	if err != nil {
		return fmt.Errorf("gitrepo: git push (retry after rebase): %w", err)
	}
	return nil
}

// PushRebaseFree runs git push without rebasing, establishing upstream via
// push.autoSetupRemote=true.
// Returns ErrPushRejected on divergence.
// Lock-free.
func (r *Repo) PushRebaseFree() error {
	_, err := r.runChecked("-c", "push.autoSetupRemote=true", "push")
	if err == nil {
		return nil
	}
	// ErrPushRejected is a sentinel consumers match with errors.Is, so it is
	// returned bare rather than wrapped with %w — a wrapper here would break
	// internal/fabricengine/coalesce.go's identity check.
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && containsAny(gitErr.Stderr, rebaseRetryTriggers) {
		return ErrPushRejected
	}
	return fmt.Errorf("gitrepo: git push: %w", err)
}

// containsAny reports whether s contains any substring from substrs.
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// PushCoalesced pushes under a single-pusher lock, giving cross-process coalescing.
// Returns immediately if nothing is unpushed once the lock is acquired.
// Shares Push's rebase-retry and SHA invalidation caveat.
func (r *Repo) PushCoalesced() error {
	l, err := lock.AcquireWriteLock(filepath.Join(r.path, PushLockFileName))
	if err != nil {
		return fmt.Errorf("gitrepo: acquire push lock: %w", err)
	}
	defer l.Release()

	unpushed, err := r.HasUnpushed()
	if err != nil {
		return err
	}
	if !unpushed {
		return nil
	}
	return r.pushWithRebaseRetry()
}

// HasUnpushed reports whether HEAD is ahead of its upstream.
// No upstream configured is treated as unpushed (true), so the first push still happens.
// A spawn failure returns (false, err);
// a non-zero git exit, recovered via errors.As(err, &gitErr), folds into (true, nil).
func (r *Repo) HasUnpushed() (bool, error) {
	stdout, err := r.runChecked("rev-list", "--count", "@{u}..HEAD")
	if err == nil {
		return strings.TrimSpace(stdout) != "0", nil
	}
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) {
		return true, nil
	}
	return false, err
}
