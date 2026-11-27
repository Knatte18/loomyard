// push.go implements the push surface: pushes, remote-branch moves and deletions, and tracking-ref removal.
// Each push sends to the remote's push URL outside the remote-refs lock.
// Git writes no remote-tracking ref while the transfer runs.
// Once the transfer succeeded, the push records the result in the tracking ref under the lock.
// The tracking-ref drop runs wholly under the lock.
// Committing is always the caller's separate StageAndCommit or StageAllAndCommit call.

package gitrepo

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// ErrPushRejected is matched with errors.Is against the error PushRebaseFree returns when the remote rejects the push.
// The rejection is a non-fast-forward push or a remote reason such as a declining hook.
// A rejection alone does not show that origin holds commits the branch lacks; a caller that must know fetches and compares.
var ErrPushRejected = errors.New("gitrepo: push rejected")

// ErrLeaseRejected is matched with errors.Is against the error that a leased remote-branch update or delete returns.
// It is reported when the remote branch no longer sits at the expected SHA, not as a failure of the push itself.
var ErrLeaseRejected = errors.New("gitrepo: lease rejected (remote branch moved)")

// leaseRejectedTrigger is the git-push stderr substring meaning a --force-with-lease expectation no longer held.
// Git words it `! [rejected] <ref> (stale info)`, untranslated, so a translated git locale defeats the match.
const leaseRejectedTrigger = "stale info"

// PushLockFileName is the name of the single-pusher lock file that a coalesced push takes in the checkout's git dir.
const PushLockFileName = "lym-push.lock"

// PushLockPath returns the path of the single-pusher lock file, which lives in the checkout's own git dir.
// No commit can stage a file there.
// A caller that must keep a coalesced push off the checkout while it works takes the same file.
func (r *Repo) PushLockPath() (string, error) {
	gitDir, err := r.GitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, PushLockFileName), nil
}

// rebaseRetryTriggers are the git-push stderr substrings that mean the remote has commits this checkout lacks.
var rebaseRetryTriggers = []string{"non-fast-forward", "rejected", "fetch first"}

// ErrPushAuthRefused is matched with errors.Is against the error PushRebaseFree returns when the remote refuses it.
// Retrying cannot clear the refusal, which is neither a rejection nor a transport failure.
// The error also carries git's refusal line.
var ErrPushAuthRefused = errors.New("gitrepo: push refused for authentication or permission")

// authRefusalTriggers match git-push stderr that says the remote refused for want of authentication or permission.
// They use GitHub's wording, untranslated, so a translated git locale defeats the match.
var authRefusalTriggers = []string{"Permission to", "denied to", "returned error: 403", "Authentication failed", "could not read Username"}

// remoteRefAbsentTrigger is the git-push-delete stderr substring for a remote ref already gone before the delete ran.
// Git's fuller wording is `error: unable to delete '<branch>': remote ref does not exist`.
// That line contains this substring verbatim, so matching on it alone needs no list.
// A future git rewording that drops or changes this substring must fail the test that pins it.
// The common case must not silently turn into an error.
const remoteRefAbsentTrigger = "remote ref does not exist"

// Push runs git push, and after one non-fast-forward rejection runs pull --rebase and retries.
// The worktree must be clean.
// Callers must re-read CurrentSHA after Push if SHAs were captured beforehand.
func (r *Repo) Push() error {
	return r.pushWithRebaseRetry()
}

// ErrPullRebaseFailed is matched with errors.Is against the error a push returns when its recovery rebase was aborted.
// It tells a replayable conflict from every other push failure.
var ErrPullRebaseFailed = errors.New("gitrepo: pull --rebase failed")

// abortedRebaseError carries the failed rebase's message unchanged and unwraps to ErrPullRebaseFailed.
type abortedRebaseError struct {
	message string
}

// Error returns the failed rebase's message.
func (e abortedRebaseError) Error() string { return e.message }

// Unwrap returns ErrPullRebaseFailed.
func (e abortedRebaseError) Unwrap() error { return ErrPullRebaseFailed }

// pushWithRebaseRetry pushes the checked-out branch.
// On a rebaseRetryTriggers match it runs git pull --rebase once, retries, and aborts the rebase if it fails.
// A first push establishes tracking.
// A rebase that failed and was aborted returns an error satisfying errors.Is(err, ErrPullRebaseFailed).
func (r *Repo) pushWithRebaseRetry() error {
	err := r.pushPlainWithRetry("", "", "")
	if err == nil {
		return nil
	}
	// An exhausted retry is a plain push failure.
	// Its remote-rejected lines match rebaseRetryTriggers, so the rebase must not run for it.
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
		// Branch on the error's type, not its message: only a *GitError means git ran and rejected the rebase.
		// That justifies the rebase --abort below.
		// Any other error means git never ran, so an abort would be wrong, and this returns at once.
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

	err = r.pushPlainWithRetry("", "", "")
	if err != nil {
		return fmt.Errorf("gitrepo: git push (retry after rebase): %w", err)
	}
	return nil
}

// pushPlainWithRetry runs the plain push through PushWithRetry.
// Every attempt and backoff runs outside the remote-refs lock.
// Once an attempt succeeds, the pushed commit is recorded in the branch's tracking ref under the lock.
// With both remote and branch empty, it pushes the checked-out branch to the default push remote.
// It pushes to the branch of the same name there, and a branch with no upstream takes that branch as its upstream.
// With a named branch it pushes source to refs/heads/<branch> on remote.
// An empty source means refs/heads/<branch>, and the branch need not be checked out.
// The commit recorded is the one source names when the push starts.
func (r *Repo) pushPlainWithRetry(remote, source, branch string) error {
	if branch == "" {
		current, err := r.CurrentBranch()
		if err != nil {
			return err
		}
		branch = current
	}
	if source == "" {
		source = "refs/heads/" + branch
	}
	dest, err := r.resolvePushDestination(remote, branch)
	if err != nil {
		return err
	}
	sha, err := r.ResolveSHA(source)
	if err != nil {
		return err
	}
	err = PushWithRetry(func() error {
		_, err := r.runChecked("push", dest.url, source+":refs/heads/"+branch)
		return err
	}, branch, PushBackoffDelays(), r.pushSleep)
	if err != nil {
		return err
	}
	r.recordLandedPush(dest, sha, true)
	return nil
}

// recordLandedPush records a push whose transfer already landed.
// It logs a failure of the record instead of returning it.
// The remote has changed, so reporting the push as failed would be wrong.
// A fetch corrects a tracking ref that the push moved.
// A ref that a delete left behind stays until something drops it, so the warning names that ref.
func (r *Repo) recordLandedPush(dest pushDestination, sha string, keepNewer bool) {
	if err := r.recordPush(dest, sha, keepNewer); err != nil {
		logger.Warn("gitrepo: push landed but its tracking ref was not recorded", "branch", dest.branch, "trackingRef", dest.trackingRef, "error", err)
	}
}

// PushBranch pushes the local branch to the same-named branch on the named remote, without rebasing or forcing;
// the branch need not be checked out.
// A rejection returns an error wrapping ErrPushRejected with git's rejection line.
// A refusal for want of authentication or permission wraps ErrPushAuthRefused.
// A retryable remote refusal that outlasts the bounded retry wraps ErrPushRetriesExhausted.
// The transfer runs outside the remote-refs lock, and the branch's remote-tracking ref is written under it afterwards.
func (r *Repo) PushBranch(remote, branch string) error {
	return mapPlainPushError(r.pushPlainWithRetry(remote, "", branch))
}

// PushCommit pushes the commit sha to the branch on the named remote, without rebasing or forcing.
// It needs no local branch.
// It records the push in the branch's remote-tracking ref and returns the classified push errors.
func (r *Repo) PushCommit(remote, sha, branch string) error {
	return mapPlainPushError(r.pushPlainWithRetry(remote, sha, branch))
}

// configKeyNotSetExit is the exit code `git config --unset` answers when the key is absent.
const configKeyNotSetExit = 5

// DropRemoteTracking deletes the remote-tracking ref refs/remotes/<remote>/<branch>.
// It unsets branch.<branch>.remote and branch.<branch>.merge, all in one hold of the remote-refs lock.
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

// PushRebaseFree pushes the checked-out branch without rebasing.
// A branch with no upstream gets its remote branch as upstream.
// A rejection returns an error wrapping ErrPushRejected with git's rejection line.
// It covers a non-fast-forward and a remote reason such as a declining hook.
// A remote "(failed)" or "cannot lock ref" refusal is retried a bounded number of times,
// and an exhausted retry wraps ErrPushRetriesExhausted rather than mapping to ErrPushRejected.
// A refusal for want of authentication or permission wraps ErrPushAuthRefused with git's refusal line.
// It is never retried, and it never maps to ErrPushRejected.
// It takes no push-serialising lock of its own;
// the transfer runs outside the repository's remote-refs lock,
// and the tracking ref and upstream are written under it afterwards.
func (r *Repo) PushRebaseFree() error {
	return mapPlainPushError(r.pushPlainWithRetry("", "", ""))
}

// mapPlainPushError maps the error of a plain push to the package's classified push errors.
// Nil stays nil.
// An exhausted retry wraps ErrPushRetriesExhausted, and an authentication refusal wraps ErrPushAuthRefused.
// A rejection wraps ErrPushRejected, and every other failure is wrapped as a git push failure.
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
// An absent remote ref returns (false, nil), an idempotent success.
// The executors in fabric's destruction gate are idempotent for an already-absent target,
// and the common case is a branch that was never pushed.
// deleted is returned separately from err so the caller's mutation record can tell a removal from an absent ref.
// The deletion runs outside the remote-refs lock,
// and the branch's remote-tracking ref is deleted under it afterwards.
func (r *Repo) DeleteRemoteBranch(remote, branch string) (deleted bool, err error) {
	dest, err := r.resolvePushDestination(remote, branch)
	if err != nil {
		return false, err
	}
	// The short branch name makes git report an absent remote ref; a full refs/heads/ name deletes a missing ref silently.
	_, err = r.runChecked("push", dest.url, "--delete", branch)
	if err == nil {
		r.recordLandedPush(dest, "", false)
		return true, nil
	}

	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, remoteRefAbsentTrigger) {
		return false, nil
	}
	return false, fmt.Errorf("gitrepo: git push --delete: %w", err)
}

// DeleteRemoteBranchLeased deletes branch on the named remote via `git push --delete` under a force-with-lease.
// The lease succeeds only while the remote branch still sits at expectSHA.
// expectSHA must be a valid hex object name, or ErrInvalidSHA is returned before any git spawn.
// An already-absent remote ref is a failed lease, not an idempotent success:
// the lease names the SHA the caller saw, so a branch that moved or vanished since then fails it.
// A failed lease returns an error matching ErrLeaseRejected under errors.Is and naming expectSHA;
// every other failure returns the wrapped git error and never matches it.
// The deletion runs outside the remote-refs lock,
// and the branch's remote-tracking ref is deleted under it afterwards.
func (r *Repo) DeleteRemoteBranchLeased(remote, branch, expectSHA string) error {
	if !validSHA(expectSHA) {
		return ErrInvalidSHA
	}

	dest, err := r.resolvePushDestination(remote, branch)
	if err != nil {
		return err
	}
	lease := "--force-with-lease=refs/heads/" + branch + ":" + expectSHA
	_, err = r.runChecked("push", lease, dest.url, "--delete", branch)
	if err == nil {
		r.recordLandedPush(dest, "", false)
		return nil
	}
	var gitErr *gitexec.GitError
	if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, leaseRejectedTrigger) {
		return fmt.Errorf("gitrepo: git push --delete (leased at %s): %w", expectSHA, ErrLeaseRejected)
	}
	return fmt.Errorf("gitrepo: git push --delete (leased at %s): %w", expectSHA, err)
}

// UpdateRemoteBranchLeased moves branch on the named remote to sha under a force-with-lease on expectSHA.
// sha may be an ancestor of expectSHA, so the branch can move backwards.
// sha and expectSHA must be valid hex object names, or ErrInvalidSHA is returned before any git spawn.
// A branch that moved or vanished since expectSHA was read is a failed lease, returned bare as ErrLeaseRejected;
// every other failure (network, authentication, a hook rejection) returns the wrapped git error and never the sentinel.
// It is the package's only force-shaped push, and it never takes a lease it did not read itself.
// The transfer runs outside the remote-refs lock,
// and the branch's remote-tracking ref moves to sha under it afterwards.
func (r *Repo) UpdateRemoteBranchLeased(remote, branch, sha, expectSHA string) error {
	if !validSHA(sha) || !validSHA(expectSHA) {
		return ErrInvalidSHA
	}

	dest, err := r.resolvePushDestination(remote, branch)
	if err != nil {
		return err
	}
	lease := "--force-with-lease=refs/heads/" + branch + ":" + expectSHA
	_, err = r.runChecked("push", lease, dest.url, sha+":refs/heads/"+branch)
	if err == nil {
		r.recordLandedPush(dest, sha, false)
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
// The push may run `git pull --rebase` and retry,
// so a SHA the caller read before the call can be stale after it.
func (r *Repo) PushCoalesced() error {
	lockPath, err := r.PushLockPath()
	if err != nil {
		return err
	}
	l, err := lock.AcquireWriteLock(lockPath)
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
// a non-zero git exit folds into (true, nil).
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
// No upstream configured is treated as unpulled (true).
// That is the conservative answer for a caller deciding whether a rejection is safe to retry.
// A spawn failure returns (false, err);
// a non-zero git exit folds into (true, nil).
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
