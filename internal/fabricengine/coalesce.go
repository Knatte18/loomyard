// coalesce.go hosts the generic loop-until-clean coalescing primitive
// coalescePush and fabric's own two-sided rebase-free push entry
// CoalescePushBothAt built on top of it. The generic primitive stays
// caller-agnostic — it owns only the absorbing lock and the loop, never any
// commit/stage/push policy — while fabric's push step (and the small helpers
// it needs) sit alongside it in this same file, per the
// coalescing-loop-in-fabricengine-via-closures Shared Decision.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// coalescePush drives a caller-supplied step to completion under one held
// absorbing lock at lockPath: it acquires the lock once, then calls step
// repeatedly, looping again each time step reports progressed == true and
// exiting — releasing the lock — on the first progressed == false or on a
// non-nil error, which propagates to the caller. coalescePush contains no
// commit, stage, ensure-ignored, or push logic of its own; step supplies all
// of that. It is unexported: Bolt.Sync and this file's own
// CoalescePushBothAt are its only two callers, both in-package, now that
// boardengine composes its coalescing loop through Bolt rather than calling
// this primitive directly.
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

// headOrEmpty returns the checkout at path's current HEAD SHA, mapping an
// unborn HEAD (gitrepo.ErrNoCommits — a repo with no commits yet) to ("", nil)
// rather than propagating it as a failure: an unborn side is a legitimate
// "nothing to push (yet)" state for the fabric push step's before/after
// comparison, not an error. An empty path is also a true no-op — ("", nil)
// without ever opening a repo — rather than falling through to
// gitrepo.New("").CurrentSHA(), which would resolve "" to the process's
// inherited cwd (via filepath.Abs) and open whatever git checkout happens to
// live there. That matters because CoalescePushBothAt's detached child is
// spawned with no cmd.Dir override, so the weft-only "lyx fabric sync" path
// (warpPath == "") would otherwise silently open (and read HEAD from) an
// unrelated checkout instead of treating the absent warp side as stable.
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

// pushRebaseFreeLogged runs a single rebase-free push at path, mapping a
// non-fast-forward rejection (gitrepo.ErrPushRejected) to a warning log line
// plus a nil return — the fabric push step's contract is to leave a diverged
// side's commits unpushed rather than reconcile them (reconciliation is slice
// 6, out of scope here) — while propagating any other failure (network,
// auth) as a genuine error.
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

// CoalescePushBothAt pushes both the warp and weft sides of a fabric hub
// under fabric's own absorbing push lock, looping until a push iteration
// advances neither side's HEAD — the rebase-free, lock-free-per-side
// entry point batch 3's CLI bypass handler wires in and board (batch 4)
// reuses the generic coalescePush half of. It honors opts.SkipGit/SkipPush by
// returning nil immediately, matching pushWeftAt/PushWarpAt's gating.
//
// weftPath must be non-empty: the absorbing push lock's only sanctioned home
// is under weftPath's .weft/ (a host-root lock is forbidden by the
// lock-artifact-under-weft / no-host-root-gitrepo-push-lock Shared
// Decisions), so an empty weftPath returns an error rather than falling back
// to warpPath (which would put a lock at the pristine host root) or
// defaulting to the process cwd (which ensureWeftLockDirAt("") would do —
// mkdir .weft and git rev-parse relative to cwd). This is a latent edge only:
// the detached push child always supplies both paths (see SpawnDetachedPush
// and Fabric.Commit's spawnDetachedPushFn(f.warpPath, f.weftPath) call), so
// production never hits this guard. warpPath may still be empty when
// weftPath is present — that pushes only the weft side; a warp-only push
// (warpPath set, weftPath empty) is not a supported coalescing entry and is
// rejected by the same guard.
func CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	if weftPath == "" {
		return fmt.Errorf("fabricengine: CoalescePushBothAt requires a weft path for the absorbing push lock")
	}

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return err
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

	return coalescePush(lockPath, step)
}
