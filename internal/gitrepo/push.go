// push.go implements the push surface: Push (a single synchronous push with rebase-retry resilience), PushCoalesced (a single-pusher lock plus one guarded push, coalescing across processes via the lock queue rather than an internal retry loop), PushRebaseFree (a single plain push that never rebases, for callers that supply their own serialization), PushBranch (the same plain push of a named branch to a named remote, checked out or not), DropRemoteTracking (the removal of a branch's remote-tracking ref and upstream setting), and DeleteRemoteBranch (a single remote branch deletion, idempotent when the ref is already absent), DeleteRemoteBranchLeased (the same deletion, only while the remote branch sits at an expected SHA), and UpdateRemoteBranchLeased (a forced move of a remote branch, only while it sits at an expected SHA).
// All of them are push-shaped remote calls, the tracking-ref drop included, each git call run under the repository's remote-refs lock; committing is always the caller's separate StageAndCommit or StageAllAndCommit call.

package gitrepo

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
)

// ErrPushRejected is matched with errors.Is against the error PushRebaseFree returns when the remote rejected the push, as not a fast-forward or for a remote reason such as a declining hook.
// A rejection alone does not show that origin holds commits the branch lacks; a caller that must know fetches and compares.
var ErrPushRejected = errors.New("gitrepo: push rejected")

// ErrLeaseRejected is matched with errors.Is against the error UpdateRemoteBranchLeased and DeleteRemoteBranchLeased return when the remote branch no longer sits at the expected SHA.
// It is a distinguishable error, not a failure of the push itself.
var ErrLeaseRejected = errors.New("gitrepo: lease rejected (remote branch moved)")

// leaseRejectedTrigger is the git-push stderr substring meaning a --force-with-lease expectation no longer held; git words it `! [rejected] <ref> (stale info)`.
// The package doc's locale caveat applies.
const leaseRejectedTrigger = "stale info"

// PushLockFileName is the name of the single-pusher lock file PushCoalesced acquires in the repo's
// worktree root.
const PushLockFileName = ".gitrepo-push.lock"

// rebaseRetryTriggers are the git-push stderr substrings indicating the
// remote has commits this checkout lacks.
var rebaseRetryTriggers = []string{"non-fast-forward", "rejected", "fetch first"}

// ErrPushAuthRefused is matched with errors.Is against the error PushRebaseFree returns when the remote refused the push for want of authentication or permission.
// Retrying cannot clear it, and it is neither a rejection nor a transport failure.
// The error also carries git's refusal line.
var ErrPushAuthRefused = errors.New("gitrepo: push refused for authentication or permission")

// authRefusalTriggers are the git-push stderr substrings meaning the remote refused the push for want of authentication or permission, in GitHub's wording.
// The package doc's locale caveat applies.
var authRefusalTriggers = []string{"Permission to", "denied to", "returned error: 403", "Authentication failed", "could not read Username"}

// remoteRefAbsentTrigger is the git-push-delete stderr substring meaning the
// remote ref was already gone before the delete ran. Git's fuller wording is
// `error: unable to delete '<branch>': remote ref does not exist`, which
// contains this substring verbatim, so matching on it alone needs no list. A
// future git rewording that drops or changes this substring must fail the
// test that pins it, rather than silently reclassifying the common case as
// an error.
const remoteRefAbsentTrigger = "remote ref does not exist"

// Push runs git push, recovering from one non-fast-forward rejection via pull --rebase before
// retrying.
// The worktree must be clean.
// Callers must re-read CurrentSHA after Push if SHAs were captured beforehand.
// A branch carrying the local-only mark is refused with an error satisfying errors.Is(err, ErrLocalBranch).
func (r *Repo) Push() error {
	return r.pushWithRebaseRetry()
}

// ErrPullRebaseFailed is matched with errors.Is against the error a push returns after its recovery rebase failed and was aborted, telling a replayable conflict from every other push failure.
var ErrPullRebaseFailed = errors.New("gitrepo: pull --rebase failed")

// abortedRebaseError carries the failed rebase's message unchanged and unwraps to ErrPullRebaseFailed.
type abortedRebaseError struct {
	message string
}

// Error returns the failed rebase's message.
func (e abortedRebaseError) Error() string { return e.message }

// Unwrap returns ErrPullRebaseFailed.
func (e abortedRebaseError) Unwrap() error { return ErrPullRebaseFailed }

// pushWithRebaseRetry runs git push and on rebaseRetryTrigger matches, runs
// git pull --rebase once and retries, aborting if rebase fails. Sets
// push.autoSetupRemote=true so first push establishes tracking.
// A rebase that failed and was aborted returns an error satisfying errors.Is(err, ErrPullRebaseFailed).
func (r *Repo) pushWithRebaseRetry() error {
	err := r.pushPlainWithRetry("", "")
	if err == nil {
		return nil
	}
	// An exhausted retry is a plain push failure: its "remote rejected" lines contain a rebaseRetryTriggers substring, and a retryable refusal must never trigger the rebase.
	if errors.Is(err, ErrPushRetriesExhausted) {
		return fmt.Errorf("gitrepo: git push: %w", err)
	}

	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) || !containsAny(gitErr.Stderr, rebaseRetryTriggers) {
		return fmt.Errorf("gitrepo: git push: %w", err)
	}

	rebaseErr := r.withRemoteRefs(func() error {
		_, err := r.runChecked("pull", "--rebase")
		return err
	})
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
		return abortedRebaseError{message: fmt.Sprintf("gitrepo: git pull --rebase: %s", rebaseGitErr.Stderr)}
	}

	err = r.pushPlainWithRetry("", "")
	if err != nil {
		return fmt.Errorf("gitrepo: git push (retry after rebase): %w", err)
	}
	return nil
}

// pushPlainWithRetry runs the plain push through PushWithRetry, each attempt under the remote-refs lock and every backoff outside it.
// With remote and branch both empty it pushes the checked-out branch to its configured destination; a branch that cannot be read leaves the ref empty, so nothing is retried.
// With a named branch it pushes refs/heads/<branch> to the same ref on remote, and the branch need not be checked out.
// A branch carrying the local-only mark is refused with ErrLocalBranch before git runs, and a mark that cannot be read is returned as the error.
func (r *Repo) pushPlainWithRetry(remote, branch string) error {
	pushArgs := []string{"-c", "push.autoSetupRemote=true", "push"}
	if branch == "" {
		if err := r.refuseMarkedCurrentBranch(); err != nil {
			return err
		}
		branch, _ = r.CurrentBranch()
	} else {
		marked, err := r.BranchMarkedLocal(branch)
		if err != nil {
			return err
		}
		if marked {
			return ErrLocalBranch
		}
		pushArgs = append(pushArgs, remote, "refs/heads/"+branch+":refs/heads/"+branch)
	}
	return PushWithRetry(func() error {
		return r.withRemoteRefs(func() error {
			_, err := r.runChecked(pushArgs...)
			return err
		})
	}, branch, PushBackoffDelays(), r.pushSleep)
}

// PushBranch pushes the local branch to the same-named branch on the named remote, without rebasing or forcing; the branch need not be checked out.
// A rejection returns an error wrapping ErrPushRejected with git's rejection line, a refusal for want of authentication or permission wraps ErrPushAuthRefused,
// and a retryable remote refusal that outlasts the bounded retry wraps ErrPushRetriesExhausted.
// A branch carrying the local-only mark is refused with ErrLocalBranch before git runs.
// The push writes the branch's remote-tracking ref, each attempt under the remote-refs lock.
func (r *Repo) PushBranch(remote, branch string) error {
	return mapPlainPushError(r.pushPlainWithRetry(remote, branch))
}

// DropRemoteTracking deletes the remote-tracking ref refs/remotes/<remote>/<branch> and unsets branch.<branch>.remote and branch.<branch>.merge, all in one hold of the remote-refs lock.
// An absent tracking ref and an absent upstream setting are no error, so a repeated call succeeds; any other git failure is returned.
// It touches no remote and no local branch.
func (r *Repo) DropRemoteTracking(remote, branch string) error {
	return r.withRemoteRefs(func() error {
		if _, err := r.runChecked("update-ref", "-d", "refs/remotes/"+remote+"/"+branch); err != nil {
			return fmt.Errorf("gitrepo: drop remote-tracking ref %s/%s: %w", remote, branch, err)
		}
		for _, key := range []string{"branch." + branch + ".remote", "branch." + branch + ".merge"} {
			_, err := r.runChecked("config", "--unset", key)
			var gitErr *gitexec.GitError
			if err != nil && !(errors.As(err, &gitErr) && gitErr.ExitCode == configKeyNotSetExit) {
				return fmt.Errorf("gitrepo: unset %s: %w", key, err)
			}
		}
		return nil
	})
}

// PushRebaseFree runs git push without rebasing, establishing upstream via
// push.autoSetupRemote=true.
// A rejection, as not a fast-forward or for a remote reason such as a declining hook, returns an error wrapping ErrPushRejected with git's rejection line;
// it returns an error satisfying errors.Is(err, ErrLocalBranch) for a branch carrying the local-only mark.
// A remote "(failed)" or "cannot lock ref" refusal is retried a bounded number of times, and an exhausted retry wraps ErrPushRetriesExhausted rather than mapping to ErrPushRejected.
// A refusal for want of authentication or permission wraps ErrPushAuthRefused with git's refusal line, is never retried, and never maps to ErrPushRejected.
// It takes no push-serialising lock of its own; each attempt runs under the repository's remote-refs lock.
func (r *Repo) PushRebaseFree() error {
	return mapPlainPushError(r.pushPlainWithRetry("", ""))
}

// mapPlainPushError maps the error of a plain push to the package's classified push errors.
// Nil stays nil, an exhausted retry wraps ErrPushRetriesExhausted, an authentication refusal wraps ErrPushAuthRefused, a rejection wraps ErrPushRejected, and every other failure is wrapped as a git push failure.
func mapPlainPushError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrPushRetriesExhausted) {
		return fmt.Errorf("gitrepo: git push: %w", err)
	}
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) {
		// A declined push prints a "rejected" line too, so the auth class is checked before the rejection class.
		if line, found := lastLineHolding(gitErr.Stderr, authRefusalTriggers); found {
			return fmt.Errorf("%w: %s", ErrPushAuthRefused, line)
		}
		if line, found := lastLineHolding(gitErr.Stderr, rebaseRetryTriggers); found {
			return fmt.Errorf("%w: %s", ErrPushRejected, line)
		}
	}
	return fmt.Errorf("gitrepo: git push: %w", err)
}

// lastLineHolding returns the last line of stderr that holds one of triggers, trimmed.
func lastLineHolding(stderr string, triggers []string) (line string, found bool) {
	lines := strings.Split(stderr, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if candidate := strings.TrimSpace(lines[i]); containsAny(candidate, triggers) {
			return candidate, true
		}
	}
	return "", false
}

// DeleteRemoteBranch deletes branch on the named remote via `git push --delete`.
// A remote ref that does not exist is reported as (false, nil) — an idempotent success recording no
// mutation and surfacing no error — because every executor in fabric's destruction gate is
// idempotent for an already-absent target and the common case is a branch that was never pushed.
// deleted is returned separately from err precisely so the caller can tell "removed it" from "there
// was nothing there", which is what the caller's mutation record needs.
func (r *Repo) DeleteRemoteBranch(remote, branch string) (deleted bool, err error) {
	err = r.withRemoteRefs(func() error {
		_, err := r.runChecked("push", remote, "--delete", branch)
		return err
	})
	if err == nil {
		return true, nil
	}

	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, remoteRefAbsentTrigger) {
		return false, nil
	}
	return false, fmt.Errorf("gitrepo: git push --delete: %w", err)
}

// DeleteRemoteBranchLeased deletes branch on the named remote via `git push --force-with-lease=refs/heads/<branch>:<expectSHA> --delete`,
// succeeding only while the remote branch still sits at expectSHA.
// expectSHA must be a valid hex object name, or ErrInvalidSHA is returned before any git spawn.
// Unlike DeleteRemoteBranch, an already-absent remote ref is not an idempotent success:
// the lease names a SHA the caller saw, so a branch that moved or vanished since is a failed lease.
// A failed lease returns an error matching ErrLeaseRejected under errors.Is and naming expectSHA;
// every other failure returns the wrapped git error and never matches it.
func (r *Repo) DeleteRemoteBranchLeased(remote, branch, expectSHA string) error {
	if !validSHA(expectSHA) {
		return ErrInvalidSHA
	}

	lease := "--force-with-lease=refs/heads/" + branch + ":" + expectSHA
	err := r.withRemoteRefs(func() error {
		_, err := r.runChecked("push", lease, remote, "--delete", branch)
		return err
	})
	if err == nil {
		return nil
	}
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, leaseRejectedTrigger) {
		return fmt.Errorf("gitrepo: git push --delete (leased at %s): %w", expectSHA, ErrLeaseRejected)
	}
	return fmt.Errorf("gitrepo: git push --delete (leased at %s): %w", expectSHA, err)
}

// UpdateRemoteBranchLeased moves branch on the named remote to sha via `git push --force-with-lease=refs/heads/<branch>:<expectSHA>`, succeeding only while the remote branch still sits at expectSHA; sha may be an ancestor of expectSHA, so the branch can move backwards.
// sha and expectSHA must be valid hex object names, or ErrInvalidSHA is returned before any git spawn.
// A branch that moved or vanished since expectSHA was read is a failed lease, returned bare as ErrLeaseRejected;
// every other failure (network, authentication, a hook rejection) returns the wrapped git error and never the sentinel.
// It is the package's only force-shaped push: fabric's destruction gate is its only caller, and it never takes a lease it did not read itself.
func (r *Repo) UpdateRemoteBranchLeased(remote, branch, sha, expectSHA string) error {
	if !validSHA(sha) || !validSHA(expectSHA) {
		return ErrInvalidSHA
	}

	lease := "--force-with-lease=refs/heads/" + branch + ":" + expectSHA
	err := r.withRemoteRefs(func() error {
		_, err := r.runChecked("push", lease, remote, sha+":refs/heads/"+branch)
		return err
	})
	if err == nil {
		return nil
	}
	// ErrLeaseRejected is a sentinel consumers match with errors.Is, so it is returned bare.
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, leaseRejectedTrigger) {
		return ErrLeaseRejected
	}
	return fmt.Errorf("gitrepo: git push --force-with-lease (leased at %s): %w", expectSHA, err)
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
	if err := r.refuseMarkedCurrentBranch(); err != nil {
		return err
	}
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

// HasUnpulled reports whether the upstream, as last fetched, holds commits HEAD lacks.
// It reads the remote-tracking ref without fetching,
// so a caller wanting the remote's current state fetches first.
// No upstream configured is treated as unpulled (true), the conservative answer for a caller deciding whether a rejection is safe to retry.
// A spawn failure returns (false, err);
// a non-zero git exit, recovered via errors.As(err, &gitErr), folds into (true, nil).
func (r *Repo) HasUnpulled() (bool, error) {
	stdout, err := r.runChecked("rev-list", "--count", "HEAD..@{u}")
	if err == nil {
		return strings.TrimSpace(stdout) != "0", nil
	}
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) {
		return true, nil
	}
	return false, err
}
