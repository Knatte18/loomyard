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

// pushRebaseFreeLogged runs a rebase-free push, mapping rejection to a warning
// and nil return (commits left unpushed per fabric's contract); other errors propagate.
func pushRebaseFreeLogged(path string) error {
	err := gitrepo.New(path).PushRebaseFree()
	if err == nil {
		return nil
	}
	if errors.Is(err, gitrepo.ErrPushRejected) {
		logger.Warn("fabricengine: push rejected, remote diverged; commits left unpushed", "path", path)
		return nil
	}
	return err
}

// CoalescePushBothAt pushes both warp and weft under fabric's absorbing push lock, looping until
// neither side advances — a rebase-free entry point honoring SkipGit/SkipPush.
// weftPath must be non-empty: the absorbing push lock's only sanctioned home.
// is under weftPath's .weft/ (a warp-root lock is forbidden by the lock-artifact-under-weft /
// no-warp-root-gitrepo-push-lock Shared Decisions), so an empty weftPath returns an error rather
// than falling back to warpPath (which would put a lock at the pristine warp root) or defaulting to
// the process cwd (which ensureWeftLockDirAt("") would do — mkdir .weft and git rev-parse relative
// to cwd).
// This is a latent edge only: the detached push child always supplies both paths (see
// SpawnDetachedPush and Fabric.Commit's spawnDetachedPushFn(f.warpPath, f.weftPath) call), so
// production never hits this guard.
// warpPath may still be empty when weftPath is present — that pushes only the weft side;
// a warp-only push (warpPath set, weftPath empty) is not a supported coalescing entry and is
// rejected by the same guard.
func CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) (res PushResult, err error) {
	rec := NewMutations(filepath.Dir(warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}
	if weftPath == "" {
		return PushResult{}, fmt.Errorf("fabricengine: CoalescePushBothAt requires a weft path for the absorbing push lock")
	}

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return PushResult{}, err
	}
	lockPath := filepath.Join(lockDir, weftPushLockFile)

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
		if warpPath != "" && beforeWarp != "" {
			if err := pushRebaseFreeLogged(warpPath); err != nil {
				return false, err
			}
		}
		if weftPath != "" && beforeWeft != "" {
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

		return afterWarp != beforeWarp || afterWeft != beforeWeft, nil
	}

	return PushResult{}, coalescePush(lockPath, step)
}
