// pushanchored.go holds PushPairAnchored, the synchronous push that takes the weft-side push lock.
// It lets a caller outside the Fabric Vocabulary Invariant's owner set push without naming a weft path.
// In the hub's prime it pushes only the records side and reports the code branch as PushResult.CodePushSkipped.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
)

// StatusPushLockWait is the bounded push-lock wait that `commit-records` passes.
// It is also the default for a pair's teardown lock.
// It is long enough to ride out a detached push child, and short enough that a stuck holder never stalls a run.
const StatusPushLockWait = 30 * time.Second

// LockWaitUnbounded is the lockWait value that blocks until the push lock is free.
const LockWaitUnbounded time.Duration = 0

// ErrPushLockBusy is wrapped by the error a bounded push returns when the push lock is still held once its wait ran out.
// Nothing was pushed.
// The error names the lock's holder, its pid and how long it has held the lock, when the holder recorded them.
var ErrPushLockBusy = errors.New("fabricengine: push lock busy")

// IsPushRejected reports whether err carries a remote's non-fast-forward rejection of a push.
func IsPushRejected(err error) bool {
	return errors.Is(err, gitrepo.ErrPushRejected)
}

// acquirePushLock returns a function that takes the push lock at the path it is given.
// LockWaitUnbounded blocks until the lock is free.
// A positive lockWait gives up once it expires, reporting the lock as not acquired.
func acquirePushLock(lockWait time.Duration) func(lockPath string) (*lock.FileLock, bool, error) {
	return func(lockPath string) (*lock.FileLock, bool, error) {
		if lockWait == LockWaitUnbounded {
			held, err := lock.AcquireWriteLock(lockPath)
			if err != nil {
				return nil, false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
			}
			return held, true, nil
		}

		held, acquired, err := lock.AcquireWriteLockWithin(lockPath, lockWait)
		if err != nil {
			return nil, false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
		}
		return held, acquired, nil
	}
}

// pushLockBusyError returns the error for a synchronous push whose wait for the push lock under lockDir ran out.
// It names the lock's holder as its holder record gives it.
func pushLockBusyError(lockDir string, lockWait time.Duration) error {
	holder := describePushLockHolder(lockDir, time.Now(), proc.IsAlive)
	return fmt.Errorf("%w: %s still held after %s, %s; neither side was pushed", ErrPushLockBusy, filepath.Join(lockDir, weftPushLockFile), lockWait, holder)
}

// PushPairAnchored pushes the unpushed commits of l's code worktree and its paired records sibling.
// It pushes rebase-free, under the weft-side absorbing push lock.
// When l is the hub's prime it pushes the records side only, since its code branch is the operator's parent branch.
// A code branch with unpushed commits is reported in PushResult.CodePushSkipped:
// the note names the branch and says it was left for the operator to push.
// Otherwise the code side is pushed first.
// The records side is attempted even when the code side failed.
// A side whose HEAD is unborn is skipped.
// Each side retries a rejection over the push backoff, after a fetch, while the remote tip is contained in local HEAD.
// A failing side's error names the side, code or records, in `fabricengine: push <side> side at <path>: %w`.
// errors.Is(err, gitrepo.ErrPushRejected) holds for a rejected side;
// when both sides fail, the error is the errors.Join of the two.
// lockWait bounds only the wait for the lock:
// a positive value gives up with an error wrapping ErrPushLockBusy and neither side pushed;
// LockWaitUnbounded blocks.
// Each side it pushes is marked pending before the lock is taken and re-read after it is released,
// so a code commit made while another holder pushes is pushed by whichever holder releases last.
// For a pair, not the prime, it first takes the pair's teardown lock shared and holds it through the push.
// While any holder has that lock exclusive, it pushes nothing.
// It returns an empty PushResult and an error wrapping ErrPairPushRunning that names the pair;
// the commits stay local for the pair's next push.
// The prime's records push takes no teardown lock.
//
// Every side that observably advanced is recorded in the returned PushResult.
// Returns (PushResult{}, nil) immediately, with no lock taken, when opts.SkipGit or opts.SkipPush is true.
// A failure to resolve whether l is the prime returns that error before the lock is taken and nothing is pushed.
func PushPairAnchored(l *lymcwd.Location, opts SyncOptions, lockWait time.Duration) (res PushResult, err error) {
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

	request, err := statusPushRequest(l, "")
	if err != nil {
		return PushResult{}, err
	}
	if request.TeardownSlug != "" {
		if err := ensureTeardownLockDir(l.HubPath, request.TeardownSlug); err != nil {
			return PushResult{}, err
		}
		heldTeardown, acquired, err := lock.TryAcquireReadLock(TeardownLockPath(l.HubPath, request.TeardownSlug))
		if err != nil {
			return PushResult{}, fmt.Errorf("fabricengine: take the teardown lock: %w", err)
		}
		if !acquired {
			return PushResult{}, fmt.Errorf("the pair %q is being made, discarded or removed, so nothing was pushed and its commits stay local for the pair's next push: %w", request.TeardownSlug, ErrPairPushRunning)
		}
		defer func() { _ = heldTeardown.Release() }()
	}
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return PushResult{}, err
	}
	owed := map[string]PushSpawn{pushSideRecords: {WeftPath: weftPath, PauseSlug: request.PauseSlug, TeardownSlug: request.TeardownSlug}}
	if !isPrime {
		owed[pushSideCode] = PushSpawn{WarpPath: warpPath, PauseSlug: request.PauseSlug, TeardownSlug: request.TeardownSlug}
	}

	var codePushSkipped string
	pushSide := func(side, path string) error {
		head, err := headOrEmpty(path)
		if err != nil {
			return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
		}
		if head == "" {
			return nil
		}
		var retryRecords func() (bool, error)
		if side == pushSideRecords {
			retryRecords = func() (bool, error) { return retryWeftPush(rec, l.HubPath, path) }
		}
		if err := pushClassified(rec, l.HubPath, side, path, opts, retryRecords); err != nil {
			return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
		}
		return nil
	}

	step := func() (progressed bool, err error) {
		codePushSkipped = ""
		var warpErr error
		if isPrime {
			codePushSkipped = unpushedCodeBranchNote(warpPath)
		} else {
			warpErr = pushSide(pushSideCode, warpPath)
		}
		weftErr := pushSide(pushSideRecords, weftPath)
		return false, errors.Join(warpErr, weftErr)
	}

	acquired, err := coalesceFlagged(lockDir, owed, acquirePushLock(lockWait), step)
	if err == nil && !acquired {
		return PushResult{}, pushLockBusyError(lockDir, lockWait)
	}
	return PushResult{CodePushSkipped: codePushSkipped}, err
}

// unpushedCodeBranchNote returns the CodePushSkipped text for the code worktree at path.
// It returns "" when the branch has nothing unpushed or its state cannot be read.
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
