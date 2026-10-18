// coalesce.go holds the generic loop-until-clean coalescing primitive coalescePush and fabric's own
// two-sided rebase-free push entry CoalescePushBothAt built on top of it.
// The generic primitive stays caller-agnostic — it owns only the absorbing lock and the loop, never
// any commit/stage/push policy — while fabric's push step (and the small helpers it needs) sit
// alongside it in this same file, per the coalescing-loop-in-fabricengine-via-closures Shared
// Decision.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// coalescePush drives a caller-supplied step to completion under one absorbing lock,
// looping while progressed=true, exiting and releasing the lock on false or error.
// The step function supplies all commit/stage/push policy.
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

// headOrEmpty returns the current HEAD SHA, mapping unborn HEAD (no commits) to ("", nil).
// Empty path is a true no-op, never resolving "" to the process's cwd, preserving
// the empty-warp no-op contract for weft-only sync paths.
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

// pushRebaseFreeLogged runs a rebase-free push, mapping rejection or an exhausted retry to a warning
// and nil return (commits left unpushed per fabric's contract); other errors propagate.
func pushRebaseFreeLogged(path string) error {
	return unpushedOutcome(path, gitrepo.New(path).PushRebaseFree())
}

// unpushedOutcome maps the result of a rebase-free push of path to fabric's contract.
// A diverged remote and a remote that kept refusing the push both log a Warn and return nil, leaving the commits unpushed for a later push;
// a branch carrying the local-only mark logs an Info and returns nil, since it is never pushed; any other error is returned.
func unpushedOutcome(path string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gitrepo.ErrLocalBranch):
		logger.Info("fabricengine: local-only branch, not pushed", "path", path)
		return nil
	case errors.Is(err, gitrepo.ErrPushRejected):
		logger.Warn("fabricengine: push rejected, remote diverged; commits left unpushed", "path", path)
		return nil
	case errors.Is(err, gitrepo.ErrPushRetriesExhausted):
		logger.Warn("fabricengine: remote kept refusing the push; commits left unpushed for a later push", "path", path, "error", err)
		return nil
	default:
		return err
	}
}

// CoalescePushBothAt pushes both warp and weft under fabric's absorbing push lock, looping until
// neither side advances — a rebase-free entry point honoring SkipGit/SkipPush.
// weftPath must be non-empty: the absorbing push lock's only sanctioned home.
// is under weftPath's .weft/ (a warp-root lock is forbidden by the lock-artifact-under-weft /
// no-warp-root-gitrepo-push-lock Shared Decisions), so an empty weftPath returns an error rather
// than falling back to warpPath (which would put a lock at the pristine warp root) or defaulting to
// the process cwd (which ensureWeftLockDirAt("") would do — mkdir .weft and git rev-parse relative
// to cwd).
// It blocks until the lock is free and marks the sides it pushes pending, so it also respawns a side another pusher left flagged.
// This is a latent edge only: the detached push child always supplies the weft path, so production never hits this guard.
// It supplies the warp path too, except in the prime (see SpawnDetachedPush and Fabric.Commit's spawnDetachedPushFn call).
// warpPath may still be empty when weftPath is present — that pushes only the weft side;
// a warp-only push (warpPath set, weftPath empty) is not a supported coalescing entry and is
// rejected by the same guard.
func CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) (res PushResult, err error) {
	return coalescePushSides(PushSpawn{WarpPath: warpPath, WeftPath: weftPath}, opts, "CoalescePushBothAt", func(lockPath string) (*lock.FileLock, bool, error) {
		held, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			return nil, false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
		}
		return held, true, nil
	})
}

// CoalescePushRequest is the detached push child's push: it pushes the sides req carries, warp as the code side and weft as the records side, under fabric.push.lock.
// A child that finds the lock held returns nil at once with nothing pushed, leaving its commits to the holder through the pending flag it marked first;
// the holder pushes them once it releases, and respawns a side it does not push itself with the flag's slugs.
// req.WeftPath must be non-empty, since the lock lives under the records worktree; it honors SkipGit and SkipPush.
func CoalescePushRequest(req PushSpawn, opts SyncOptions) (res PushResult, err error) {
	return coalescePushSides(req, opts, "CoalescePushRequest", lock.TryAcquireWriteLock)
}

// coalescePushSides runs the two-sided rebase-free push of req under fabric.push.lock, acquired through acquire and flagged by the sides req carries.
// caller names the public entry for the error of a missing weft path.
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

	owed := map[string]PushSpawn{pushSideRecords: {WeftPath: weftPath, PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug}}
	if warpPath != "" {
		owed[pushSideCode] = PushSpawn{WarpPath: warpPath, PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug}
	}

	step := func() (progressed bool, err error) {
		beforeWarp, err := headOrEmpty(warpPath)
		if err != nil {
			return false, err
		}
		beforeWeft, err := headOrEmpty(weftPath)
		if err != nil {
			return false, err
		}

		// A side with an empty path or an unborn HEAD has nothing to push;
		// skip it, but it still participates in the before/after HEAD
		// comparison below (its empty-string HEAD is trivially stable).
		// warpRepo/weftRepo and their HasUnpushed samples are only taken for a side that is actually
		// attempted this iteration, beside the beforeWarp/beforeWeft HEAD sampling already here — the
		// same before/after shape card 19 uses for PushWeft and PushWarpAt, applied per side.
		var warpRepo, weftRepo *gitrepo.Repo
		var warpHadUnpushed, weftHadUnpushed bool
		var warpUnpushedErr, weftUnpushedErr error
		if warpPath != "" && beforeWarp != "" {
			warpRepo = gitrepo.New(warpPath)
			warpHadUnpushed, warpUnpushedErr = warpRepo.HasUnpushed()
			if err := pushRebaseFreeLogged(warpPath); err != nil {
				return false, err
			}
		}
		if weftPath != "" && beforeWeft != "" {
			weftRepo = gitrepo.New(weftPath)
			weftHadUnpushed, weftUnpushedErr = weftRepo.HasUnpushed()
			if err := pushRebaseFreeLogged(weftPath); err != nil {
				return false, err
			}
		}

		afterWarp, err := headOrEmpty(warpPath)
		if err != nil {
			return false, err
		}
		afterWeft, err := headOrEmpty(weftPath)
		if err != nil {
			return false, err
		}

		if warpRepo != nil {
			recordPushIfAdvanced(rec, warpRepo, "code", warpPath, warpHadUnpushed, warpUnpushedErr)
		}
		if weftRepo != nil {
			recordPushIfAdvanced(rec, weftRepo, "records", weftPath, weftHadUnpushed, weftUnpushedErr)
		}

		return afterWarp != beforeWarp || afterWeft != beforeWeft, nil
	}

	_, err = coalesceFlagged(lockDir, owed, acquire, step)
	return PushResult{}, err
}
