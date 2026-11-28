// coalesce.go holds coalescePush, the loop that runs a step under one absorbing lock until it stops progressing.
// It also holds the two-sided rebase-free push built on that loop.
// Its entries are CoalescePushBothAt and CoalescePushRequest.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// pushLockGateOp names the pause-gate operation a process marks while it wants or holds a push lock.
// A pusher passes the gate under this name before it takes its first lock and stays admitted until it releases the last,
// so a pause holds a pusher only while it holds no lock that other pushers wait on.
const pushLockGateOp = "push-lock"

// coalescePush runs step under one absorbing lock, looping while step reports progressed=true.
// It returns on the first false or error from step and releases the lock.
// It passes the process's pause gate before it takes the lock and stays admitted until the release.
// step supplies all commit, stage and push policy.
func coalescePush(lockPath string, step func() (progressed bool, err error)) error {
	leaveGate := pauserecord.Admit(pushLockGateOp)
	defer leaveGate()
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

// pushSideDetached pushes one side of a detached push and reports whether the side landed.
// A side landed when its push returned nil and its worktree then holds nothing unpushed.
// A side with a ceiling in opts landed once origin holds the ceiling, and is not pushed when origin already does.
// A push failure it records, a deferral and an ended branch leave the commits unpushed for a later push:
// each returns landed false and a nil error.
// Any other error is returned.
func pushSideDetached(rec *Mutations, hubPath, side, path string, opts SyncOptions, retryRecords func() (bool, error)) (landed bool, err error) {
	if opts.ceiling != "" {
		unpushed, err := unpushedUnder(path, opts.ceiling)
		if err != nil || !unpushed {
			return err == nil, err
		}
	}
	err = pushClassified(rec, hubPath, side, path, opts, retryRecords)
	var failed *PushFailedError
	switch {
	case err == nil:
	case errors.As(err, &failed):
		logger.Warn("fabricengine: push failed and was recorded; commits left unpushed", "path", path, "class", string(failed.Record.Class), "error", err)
		return false, nil
	default:
		return false, err
	}
	unpushed, err := unpushedUnder(path, opts.ceiling)
	if err != nil {
		return false, fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
	}
	return !unpushed, nil
}

// CoalescePushBothAt pushes warp and weft under fabric's absorbing push lock.
// It loops until neither side's HEAD advances, and it honors SkipGit and SkipPush.
// It is a foreground push, for callers that supply both paths and wait for the result.
// weftPath must be non-empty, since the lock lives under weftPath's .weft/ directory.
// An empty weftPath returns an error rather than placing the lock at the warp root or in the working directory.
// warpPath may be empty, in which case only the weft side is pushed.
// A call with warpPath set and weftPath empty is rejected by the same guard.
// It blocks until the lock is free,
// and it marks the sides it pushes pending.
func CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) (res fabriclayout.PushResult, err error) {
	return coalescePushSides(fabriclayout.PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, opts, fabriclayout.MachineNames{}, "CoalescePushBothAt", func(lockPath string) (*lock.FileLock, bool, error) {
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
// The holder pushes the sides it owes, and a flag of a side it does not owe stays on disk for that worktree's next push.
// req.WeftPath must be non-empty, since the lock lives under the records worktree.
// It honors SkipGit and SkipPush.
// A rejected records push reads the run's record on origin first, for the run req names, through here.
// A takeover entry of a machine here does not hold returns a *TakeoverError, with no side pushed after the rejection.
func CoalescePushRequest(req fabriclayout.PushSpawn, opts SyncOptions, here fabriclayout.MachineNames) (res fabriclayout.PushResult, err error) {
	return coalescePushSides(req, opts, here, "CoalescePushRequest", lock.TryAcquireWriteLock)
}

// coalescePushSides runs the two-sided rebase-free push of req under fabric.push.lock.
// It takes the lock through acquire and flags the sides req carries as owed.
// The run a rejected records push reads on origin is req.TeardownSlug, the pair's slug.
// The prime has no such slug, so it reads req.PauseSlug.
// caller names the public entry point in the error for a missing weft path.
// The code side goes out with SyncOptions.AfterRecords set, after the records side landed.
// A records commit held back for the pair's run caps the records side at that commit's first parent.
// A process that supplied no run-records paths is refused before the lock, with nothing recorded.
// The refusal is there because a rejected records push reads a run's record.
// For the duration of the call it installs a gate deferral.
// A pause triggers it at the gate before an acquire of the lock:
// the flags of the owed sides go back, and the process exits.
func coalescePushSides(req fabriclayout.PushSpawn, opts SyncOptions, here fabriclayout.MachineNames, caller string, acquire func(lockPath string) (*lock.FileLock, bool, error)) (res fabriclayout.PushResult, err error) {
	warpPath, weftPath := req.WarpPath, req.WeftPath
	hubRoot := ""
	if warpPath != "" {
		hubRoot = filepath.Dir(warpPath)
	}
	rec := NewMutations(hubRoot)
	defer func() { res.Mutations = rec.Snapshot() }()

	if err := refuseUnsuppliedSeams(fabriccore.SeamRunRecordPaths); err != nil {
		return fabriclayout.PushResult{}, err
	}
	if opts.SkipGit || opts.SkipPush {
		return fabriclayout.PushResult{}, nil
	}
	if weftPath == "" {
		return fabriclayout.PushResult{}, fmt.Errorf("fabricengine: %s requires a weft path for the absorbing push lock", caller)
	}

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return fabriclayout.PushResult{}, err
	}
	hubPath := filepath.Dir(weftPath)
	runSlug := req.TeardownSlug
	if runSlug == "" {
		runSlug = req.PauseSlug
	}

	owed := map[string]fabriclayout.PushSpawn{pushSideRecords: {WeftPath: weftPath, PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug, ParentLocked: req.ParentLocked}}
	if warpPath != "" {
		owed[pushSideCode] = fabriclayout.PushSpawn{WarpPath: warpPath, PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug, ParentLocked: req.ParentLocked}
	}

	codeOpts := opts
	codeOpts.AfterRecords = true
	defer pauserecord.SetGateDeferral(pushDeferral(lockDir, owed, exitDeferred))()

	readHeads := func() (map[string]string, error) {
		warp, err := headOrEmpty(warpPath)
		if err != nil {
			return nil, err
		}
		weft, err := headOrEmpty(weftPath)
		if err != nil {
			return nil, err
		}
		return map[string]string{pushSideCode: warp, pushSideRecords: weft}, nil
	}

	step := func() (progressed bool, err error) {
		before, err := readHeads()
		if err != nil {
			return false, err
		}

		// A side with an empty path or an unborn HEAD has nothing to push, so it is skipped;
		// it still takes part in the before/after HEAD comparison below, where its empty HEAD is trivially stable.
		// The records side goes first, and the code side only once the records side landed.
		recordsLanded := true
		if before[pushSideRecords] != "" {
			recordsOpts := opts
			// The records worktree's name carries the slug of the pair whose run may hold a commit.
			heldSlug, _ := fabriclayout.WeftWarpSlug(filepath.Base(weftPath))
			if recordsOpts.ceiling, err = recordsCeilingAt(hubPath, weftPath, heldSlug); err != nil {
				return false, err
			}
			retryWeft := func() (bool, error) {
				return retryWeftPush(rec, hubPath, weftPath, runSlug, here, recordsOpts.ceiling)
			}
			recordsLanded, err = pushSideDetached(rec, hubPath, fabriclayout.RepoRecords, weftPath, recordsOpts, retryWeft)
			if err != nil {
				return false, err
			}
		}
		if recordsLanded && warpPath != "" && before[pushSideCode] != "" {
			if _, err := pushSideDetached(rec, hubPath, fabriclayout.RepoCode, warpPath, codeOpts, nil); err != nil {
				return false, err
			}
		}

		after, err := readHeads()
		if err != nil {
			return false, err
		}
		return fabriccore.SidesProgressed(before, after), nil
	}

	_, err = coalesceFlagged(lockDir, owed, acquire, step)
	return fabriclayout.PushResult{}, err
}
