// pushanchored.go holds PushAnchored and PushPairAnchored, the fabric-vocabulary-neutral synchronous
// pushes beside CommitAnchoredPaths' commit — the entry points that let a caller outside the Fabric
// Vocabulary Invariant's owner set commit and push into the weft sibling, and push the warp worktree
// with it, without ever naming a weft path.

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

// StatusPushLockWait is the bounded push-lock wait the per-transition status pushes and
// `commit-records` pass: long enough to ride out a detached push child, short enough that a stuck
// holder never stalls a run.
const StatusPushLockWait = 30 * time.Second

// LockWaitUnbounded is the lockWait value that blocks until the push lock is free, as the detached
// push child does.
const LockWaitUnbounded time.Duration = 0

// ErrPushLockBusy is wrapped by the error a bounded push returns when the push lock is still held
// once its wait ran out; nothing was pushed.
var ErrPushLockBusy = errors.New("fabricengine: push lock busy")

// IsPushRejected reports whether err carries a remote's non-fast-forward rejection of a push, the
// side-neutral form of errors.Is(err, gitrepo.ErrPushRejected).
func IsPushRejected(err error) bool {
	return errors.Is(err, gitrepo.ErrPushRejected)
}

// acquirePushLock takes the absorbing push lock under weftPath's lock directory, the same file
// CoalescePushBothAt holds.
// LockWaitUnbounded blocks until the lock is free; a positive lockWait gives up once it expires,
// returning an error wrapping ErrPushLockBusy.
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

// pushRebaseFreeRetryingOnce pushes repo rebase-free; on a rejection it fetches and pushes once more
// only when the fetched remote tip is already contained in local HEAD, which makes the rejection a
// stale remote-tracking ref rather than a divergence.
// Any other outcome returns the push's own error, a rejection as the bare gitrepo.ErrPushRejected.
// Never rebases, merges or forces; a remote holding commits the local branch lacks is left alone.
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

// PushAnchored pushes unpushed commits in l's weft sibling worktree rebase-free, resolving its
// target the same way CommitAnchoredPaths does — from l alone, via WeftWorktree(l) — so a caller
// outside the Fabric Vocabulary Invariant's owner set never learns the weft exists.
// It pushes the records side only.
//
// The push runs under the weft-side absorbing push lock, so it never races a detached push child or
// another pushing verb; contending with that child is the point, since a lock-free status push
// racing it can leave a stale remote-tracking ref behind and be rejected without any real
// divergence.
// lockWait bounds only the wait for the lock: a positive value gives up with an error wrapping
// ErrPushLockBusy and pushes nothing, LockWaitUnbounded blocks, and a holder lasts as long as git
// does.
// A rejection is retried once after a fetch when the remote tip is already contained in local HEAD;
// a rejection that stays still surfaces gitrepo.ErrPushRejected UNWRAPPED, so a caller can
// discriminate it from every other push failure with errors.Is.
// The underlying primitive is gitrepo.PushRebaseFree, never gitrepo.PushCoalesced, whose rejected-
// push path runs `git pull --rebase` and would rewrite this side's SHAs under a running weft.
//
// Returns (PushResult{}, nil) immediately, with no lock taken and nothing recorded, when
// opts.SkipGit or opts.SkipPush is true.
func PushAnchored(l *lyxcwd.Location, opts SyncOptions, lockWait time.Duration) (res PushResult, err error) {
	target := WeftWorktree(l)
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

// PushPairAnchored pushes the unpushed commits of l's code worktree and of its paired records
// sibling, rebase-free, under the same absorbing push lock PushAnchored takes.
// The code side is pushed first and the records side is attempted even when the code side failed; a
// side whose HEAD is unborn is skipped.
// Each side retries a rejection once, exactly as PushAnchored does.
// A failing side's error is wrapped as `fabricengine: push <side> side at <path>: %w` with the side
// named warp or weft, so errors.Is(err, gitrepo.ErrPushRejected) holds for a rejected side; both
// failing returns the errors.Join of the two.
// lockWait is PushAnchored's: an expiry returns an error wrapping ErrPushLockBusy with neither side
// pushed.
//
// Every side that observably advanced is recorded in the returned PushResult.
// Returns (PushResult{}, nil) immediately, with no lock taken, when opts.SkipGit or opts.SkipPush
// is true.
func PushPairAnchored(l *lyxcwd.Location, opts SyncOptions, lockWait time.Duration) (res PushResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	warpPath := l.WorktreePath()
	weftPath := WeftWorktree(l)

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

	warpErr := pushSide("warp", warpPath)
	weftErr := pushSide("weft", weftPath)
	return PushResult{}, errors.Join(warpErr, weftErr)
}
