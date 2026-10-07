// pushanchored.go holds PushAnchored and PushPairAnchored, the fabric-vocabulary-neutral synchronous pushes beside CommitAnchoredPaths' commit.
// They are the entry points that let a caller outside the Fabric Vocabulary Invariant's owner set commit and push into the weft sibling, and push the code worktree with it, without ever naming a weft path.
// In the hub's prime PushPairAnchored pushes the records side only and reports a code branch it left unpushed as PushResult.CodePushSkipped.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// StatusPushLockWait is the bounded push-lock wait the per-transition status pushes and `commit-records` pass.
// It is long enough to ride out a detached push child, and short enough that a stuck holder never stalls a run.
const StatusPushLockWait = 30 * time.Second

// LockWaitUnbounded is the lockWait value that blocks until the push lock is free, as the detached push child does.
const LockWaitUnbounded time.Duration = 0

// ErrPushLockBusy is wrapped by the error a bounded push returns when the push lock is still held once its wait ran out.
// Nothing was pushed.
var ErrPushLockBusy = errors.New("fabricengine: push lock busy")

// IsPushRejected reports whether err carries a remote's non-fast-forward rejection of a push.
// It is the side-neutral form of errors.Is(err, gitrepo.ErrPushRejected).
func IsPushRejected(err error) bool {
	return errors.Is(err, gitrepo.ErrPushRejected)
}

// acquirePushLock takes the absorbing push lock under weftPath's lock directory, the same file CoalescePushBothAt holds.
// LockWaitUnbounded blocks until the lock is free.
// A positive lockWait gives up once it expires, returning an error wrapping ErrPushLockBusy.
func acquirePushLock(weftPath string, lockWait time.Duration) (*lock.FileLock, error) {
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(lockDir, weftPushLockFile)

	if lockWait == LockWaitUnbounded {
		held, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			return nil, fmt.Errorf("fabricengine: acquire push lock: %w", err)
		}
		return held, nil
	}

	held, acquired, err := lock.AcquireWriteLockWithin(lockPath, lockWait)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: acquire push lock: %w", err)
	}
	if !acquired {
		return nil, fmt.Errorf("%w: %s still held after %s; neither side was pushed", ErrPushLockBusy, lockPath, lockWait)
	}
	return held, nil
}

// pushRebaseFreeRetryingOnce pushes repo rebase-free.
// On a rejection it fetches, and pushes once more only when the fetched remote tip is already contained in local HEAD, which makes the rejection a stale remote-tracking ref rather than a divergence.
// Any other outcome returns the push's own error, a rejection as the bare gitrepo.ErrPushRejected.
// It never rebases, merges or forces.
// A remote holding commits the local branch lacks is left alone.
func pushRebaseFreeRetryingOnce(repo *gitrepo.Repo) error {
	err := repo.PushRebaseFree()
	if !errors.Is(err, gitrepo.ErrPushRejected) {
		return err
	}
	if fetchErr := repo.Fetch(); fetchErr != nil {
		return err
	}
	if hasUnpulled, unpulledErr := repo.HasUnpulled(); unpulledErr != nil || hasUnpulled {
		return err
	}
	return repo.PushRebaseFree()
}

// PushAnchored pushes unpushed commits in l's weft sibling worktree rebase-free.
// It resolves its target the same way CommitAnchoredPaths does, from l alone via RecordsWorktree(l),
// so a caller outside the Fabric Vocabulary Invariant's owner set never learns the weft exists.
// It pushes the records side only.
//
// The push runs under the weft-side absorbing push lock,
// so it never races a detached push child or another pushing verb.
// Contending with that child is the point, since a lock-free status push racing it can leave a stale remote-tracking ref behind and be rejected without any real divergence.
// lockWait bounds only the wait for the lock:
// a positive value gives up with an error wrapping ErrPushLockBusy and pushes nothing;
// LockWaitUnbounded blocks;
// and a holder lasts as long as git does.
// A rejection is retried once after a fetch when the remote tip is already contained in local HEAD;
// a rejection that stays still surfaces gitrepo.ErrPushRejected UNWRAPPED,
// so a caller can discriminate it from every other push failure with errors.Is.
// The underlying primitive is gitrepo.PushRebaseFree, never gitrepo.PushCoalesced, whose rejected-push path runs `git pull --rebase` and would rewrite this side's SHAs under a running weft.
//
// Returns (PushResult{}, nil) immediately, with no lock taken and nothing recorded, when opts.SkipGit or opts.SkipPush is true.
func PushAnchored(l *lyxcwd.Location, opts SyncOptions, lockWait time.Duration) (res PushResult, err error) {
	target := RecordsWorktree(l)
	rec := NewMutations(filepath.Dir(target))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	held, err := acquirePushLock(target, lockWait)
	if err != nil {
		return PushResult{}, err
	}
	defer func() { _ = held.Release() }()

	repo := gitrepo.New(target)
	hadUnpushed, hadUnpushedErr := repo.HasUnpushed()
	if err := pushRebaseFreeRetryingOnce(repo); err != nil {
		return PushResult{}, err
	}
	recordPushIfAdvanced(rec, repo, "weft", target, hadUnpushed, hadUnpushedErr)

	return PushResult{}, nil
}

// PushPairAnchored pushes the unpushed commits of l's code worktree and of its paired records sibling, rebase-free, under the weft-side absorbing push lock.
// When l is the hub's prime it pushes the records side only, since the prime's code branch is the operator's parent branch:
// a code branch with unpushed commits is reported in PushResult.CodePushSkipped, naming the branch and saying it was left for the operator to push.
// Otherwise the code side is pushed first.
// The records side is attempted even when the code side failed.
// A side whose HEAD is unborn is skipped.
// Each side retries a rejection once, after a fetch, when the remote tip is already contained in local HEAD.
// A failing side's error is wrapped as `fabricengine: push <side> side at <path>: %w` with the side named warp or weft,
// so errors.Is(err, gitrepo.ErrPushRejected) holds for a rejected side;
// both failing returns the errors.Join of the two.
// lockWait bounds only the wait for the lock:
// a positive value gives up with an error wrapping ErrPushLockBusy and neither side pushed;
// LockWaitUnbounded blocks.
//
// Every side that observably advanced is recorded in the returned PushResult.
// Returns (PushResult{}, nil) immediately, with no lock taken, when opts.SkipGit or opts.SkipPush is true.
// A failure to resolve whether l is the prime returns that error before the lock is taken and nothing is pushed.
func PushPairAnchored(l *lyxcwd.Location, opts SyncOptions, lockWait time.Duration) (res PushResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	isPrime, err := IsPrimeWorktree(l)
	if err != nil {
		return PushResult{}, err
	}
	warpPath := l.WorktreePath()
	weftPath := RecordsWorktree(l)

	held, err := acquirePushLock(weftPath, lockWait)
	if err != nil {
		return PushResult{}, err
	}
	defer func() { _ = held.Release() }()

	pushSide := func(side, path string) error {
		head, err := headOrEmpty(path)
		if err != nil {
			return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
		}
		if head == "" {
			return nil
		}
		repo := gitrepo.New(path)
		hadUnpushed, hadUnpushedErr := repo.HasUnpushed()
		if err := pushRebaseFreeRetryingOnce(repo); err != nil {
			return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
		}
		recordPushIfAdvanced(rec, repo, side, path, hadUnpushed, hadUnpushedErr)
		return nil
	}

	var warpErr error
	var codePushSkipped string
	if isPrime {
		codePushSkipped = unpushedCodeBranchNote(warpPath)
	} else {
		warpErr = pushSide("warp", warpPath)
	}
	weftErr := pushSide("weft", weftPath)
	return PushResult{CodePushSkipped: codePushSkipped}, errors.Join(warpErr, weftErr)
}

// unpushedCodeBranchNote returns the CodePushSkipped text for the code worktree at path, or "" when its branch has nothing unpushed or the state cannot be read.
func unpushedCodeBranchNote(path string) string {
	head, err := headOrEmpty(path)
	if err != nil || head == "" {
		return ""
	}
	repo := gitrepo.New(path)
	unpushed, err := repo.HasUnpushed()
	if err != nil || !unpushed {
		return ""
	}
	branch, err := repo.CurrentBranch()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("branch %s has unpushed commits; the prime's code side is left for the operator to push", branch)
}
