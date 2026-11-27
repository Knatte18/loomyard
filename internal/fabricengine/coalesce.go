// coalesce.go holds coalescePush, the loop that runs a step under one absorbing lock until it stops progressing.
// It also holds the two-sided rebase-free push built on that loop.
// Its entries are CoalescePushBothAt and CoalescePushRequest.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// coalescePush runs step under one absorbing lock, looping while step reports progressed=true.
// It returns on the first false or error from step and releases the lock.
// step supplies all commit, stage and push policy.
func coalescePush(lockPath string, step func() (progressed bool, err error)) error {
	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("fabricengine: acquire push lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	for {
		progressed, err := step()
		if err != nil {
			return err
		}
		if !progressed {
			return nil
		}
	}
}

// headOrEmpty returns the HEAD SHA of the repository at path, or "" when HEAD is unborn.
// An empty path returns "" with no error and never resolves to the process's working directory.
func headOrEmpty(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	sha, err := gitrepo.New(path).CurrentSHA()
	if err == nil {
		return sha, nil
	}
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return "", nil
	}
	return "", err
}

// pushSideDetached pushes one side of a detached push.
// A push failure it records leaves the commits unpushed for a later push and returns nil.
// Any other error is returned.
func pushSideDetached(rec *Mutations, hubPath, side, path string, opts SyncOptions, retryRecords func() (bool, error)) error {
	err := pushClassified(rec, hubPath, side, path, opts, retryRecords)
	var failed *PushFailedError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &failed):
		logger.Warn("fabricengine: push failed and was recorded; commits left unpushed", "path", path, "class", string(failed.Record.Class), "error", err)
		return nil
	default:
		return err
	}
}

// CoalescePushBothAt pushes warp and weft under fabric's absorbing push lock.
// It loops until neither side's HEAD advances, and it honors SkipGit and SkipPush.
// It is a foreground push, for callers that supply both paths and wait for the result.
// weftPath must be non-empty, since the lock lives under weftPath's .weft/ directory.
// An empty weftPath returns an error rather than placing the lock at the warp root or in the working directory.
// warpPath may be empty, in which case only the weft side is pushed.
// A call with warpPath set and weftPath empty is rejected by the same guard.
// It blocks until the lock is free,
// and it marks the sides it pushes pending, so it also respawns a side another pusher left flagged.
func CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) (res PushResult, err error) {
	return coalescePushSides(PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, opts, "CoalescePushBothAt", func(lockPath string) (*lock.FileLock, bool, error) {
		held, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			return nil, false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
		}
		return held, true, nil
	})
}

// CoalescePushRequest pushes the sides req carries under fabric.push.lock.
// Warp is the code side and weft the records side.
// When the lock is held, it returns nil at once with nothing pushed.
// It leaves its commits to the lock holder through the pending flags it marked first.
// The holder pushes them once it releases the lock, and respawns a side it does not push itself.
// req.WeftPath must be non-empty, since the lock lives under the records worktree.
// It honors SkipGit and SkipPush.
func CoalescePushRequest(req PushSpawn, opts SyncOptions) (res PushResult, err error) {
	return coalescePushSides(req, opts, "CoalescePushRequest", lock.TryAcquireWriteLock)
}

// coalescePushSides runs the two-sided rebase-free push of req under fabric.push.lock.
// It takes the lock through acquire and flags the sides req carries as owed.
// caller names the public entry point in the error for a missing weft path.
// For the duration of the call it installs a gate deferral that a pause triggers:
// the flags of the owed sides not yet pushed go back, and the process exits.
func coalescePushSides(req PushSpawn, opts SyncOptions, caller string, acquire func(lockPath string) (*lock.FileLock, bool, error)) (res PushResult, err error) {
	warpPath, weftPath := req.WarpPath, req.WeftPath
	hubRoot := ""
	if warpPath != "" {
		hubRoot = filepath.Dir(warpPath)
	}
	rec := NewMutations(hubRoot)
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}
	if weftPath == "" {
		return PushResult{}, fmt.Errorf("fabricengine: %s requires a weft path for the absorbing push lock", caller)
	}

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return PushResult{}, err
	}
	hubPath := filepath.Dir(weftPath)

	owed := map[string]PushSpawn{pushSideRecords: {WeftPath: weftPath, PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug, ParentLocked: req.ParentLocked}}
	if warpPath != "" {
		owed[pushSideCode] = PushSpawn{WarpPath: warpPath, PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug, ParentLocked: req.ParentLocked}
	}

	// pushed records the sides the current step has pushed,
	// so a deferral restores the flag of every owed side the step has not reached.
	pushed := map[string]bool{}
	defer pauserecord.SetGateDeferral(pushDeferral(lockDir, owed, pushed, exitDeferred))()

	step := func() (progressed bool, err error) {
		clear(pushed)
		beforeWarp, err := headOrEmpty(warpPath)
		if err != nil {
			return false, err
		}
		beforeWeft, err := headOrEmpty(weftPath)
		if err != nil {
			return false, err
		}

		// A side with an empty path or an unborn HEAD has nothing to push, so it is skipped;
		// it still takes part in the before/after HEAD comparison below, where its empty HEAD is trivially stable.
		if warpPath != "" && beforeWarp != "" {
			if err := pushSideDetached(rec, hubPath, failedPushSideCode, warpPath, opts, nil); err != nil {
				return false, err
			}
		}
		pushed[pushSideCode] = true
		if beforeWeft != "" {
			retryWeft := func() (bool, error) { return retryWeftPush(rec, hubPath, weftPath) }
			if err := pushSideDetached(rec, hubPath, failedPushSideRecords, weftPath, opts, retryWeft); err != nil {
				return false, err
			}
		}
		pushed[pushSideRecords] = true

		afterWarp, err := headOrEmpty(warpPath)
		if err != nil {
			return false, err
		}
		afterWeft, err := headOrEmpty(weftPath)
		if err != nil {
			return false, err
		}

		return afterWarp != beforeWarp || afterWeft != beforeWeft, nil
	}

	_, err = coalesceFlagged(lockDir, owed, acquire, step)
	return PushResult{}, err
}
