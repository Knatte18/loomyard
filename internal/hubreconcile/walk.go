// walk.go walks a hub's worktrees under the hub lock: reconcile each one's config, commit what was written, and stamp the build.

package hubreconcile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// walkHooks are test seams run at fixed points of the walk; a nil hook is skipped.
type walkHooks struct {
	// afterEnumerate runs after each listing of the hub's code worktrees.
	afterEnumerate func()
	// beforeCommit runs between a worktree's config write and its commit, with the worktree root.
	beforeCommit func(worktreePath string)
}

// ensure is Ensure with test hooks.
func ensure(geom Geometry, opts Options, hooks walkHooks) error {
	running := runningStamp()
	pairCall := opts.Pair != ""

	if !pairCall {
		if _, current := readCurrentStamp(geom.StampPath(), running.BuildKey); current {
			return nil
		}
	}

	wait := opts.LockWait
	if wait == 0 {
		wait = DefaultLockWait
	}
	if err := os.MkdirAll(filepath.Dir(geom.LockPath()), 0o755); err != nil {
		return fmt.Errorf("hubreconcile: create lock dir: %w", err)
	}
	hubLock, acquired, err := lock.AcquireWriteLockWithin(geom.LockPath(), wait)
	if err != nil {
		return fmt.Errorf("hubreconcile: %w", err)
	}
	if !acquired {
		return &LockTimeoutError{Path: geom.LockPath(), Wait: wait}
	}
	defer func() { _ = hubLock.Release() }()

	revision := revisionLabel(running.Identity)

	if pairCall {
		return reconcilePair(geom, filepath.Clean(opts.Pair), revision, hooks)
	}

	previous, current := readCurrentStamp(geom.StampPath(), running.BuildKey)
	if current {
		return nil
	}
	logger.Info("hubreconcile: reconciling hub config for a new build", "previous_revision", previous.Revision, "running_revision", running.Revision)

	skipped, err := walkHub(geom, revision, hooks)
	if err != nil {
		return err
	}
	if skipped {
		logger.Info("hubreconcile: a worktree is mid-merge, so the build stamp stays stale and the next start verb retries")
		return nil
	}
	return writeStamp(geom.StampPath(), running)
}

// readCurrentStamp returns the stamp at path and whether it names key.
// An unreadable or unparseable stamp is logged and counts as stale.
func readCurrentStamp(path, key string) (s stamp, current bool) {
	s, found, err := readStamp(path)
	if err != nil {
		logger.Warn("hubreconcile: ignoring unreadable build stamp", "path", path, "error", err)
		return stamp{}, false
	}
	return s, found && s.BuildKey == key
}

// walkHub reconciles the hub-wide config, the prime and then every pair, re-listing until no unwalked pair remains.
// It reports whether any worktree was skipped as mid-merge.
func walkHub(geom Geometry, revision string, hooks walkHooks) (skipped bool, err error) {
	worktrees, err := fabricengine.CodeWorktrees(geom.WorktreePath)
	if err != nil {
		return false, fmt.Errorf("hubreconcile: list code worktrees: %w", err)
	}
	if hooks.afterEnumerate != nil {
		hooks.afterEnumerate()
	}
	prime, found := findPrime(worktrees)
	if !found {
		return false, fmt.Errorf("hubreconcile: no main code worktree among the hub's worktrees listed from %s", geom.WorktreePath)
	}

	if err := reconcileHubWide(geom.BoardDir, prime.Anchor, revision); err != nil {
		return false, err
	}
	walked := map[string]bool{prime.Path: true}
	primeSkipped, err := reconcileWorktree(geom.BoardDir, prime, revision, hooks)
	if err != nil {
		return false, err
	}
	skipped = primeSkipped

	for {
		worktrees, err := fabricengine.CodeWorktrees(geom.WorktreePath)
		if err != nil {
			return false, fmt.Errorf("hubreconcile: list code worktrees: %w", err)
		}
		if hooks.afterEnumerate != nil {
			hooks.afterEnumerate()
		}
		foundNew := false
		for _, w := range worktrees {
			if walked[w.Path] {
				continue
			}
			foundNew = true
			walked[w.Path] = true
			pairSkipped, err := reconcileWorktree(geom.BoardDir, w, revision, hooks)
			if err != nil {
				return false, err
			}
			skipped = skipped || pairSkipped
		}
		if !foundNew {
			return skipped, nil
		}
	}
}

// reconcilePair reconciles the one pair whose root is pairPath, whatever the stamp says.
func reconcilePair(geom Geometry, pairPath, revision string, hooks walkHooks) error {
	worktrees, err := fabricengine.CodeWorktrees(geom.WorktreePath)
	if err != nil {
		return fmt.Errorf("hubreconcile: list code worktrees: %w", err)
	}
	for _, w := range worktrees {
		if w.Path == pairPath {
			_, err := reconcileWorktree(geom.BoardDir, w, revision, hooks)
			return err
		}
	}
	return fmt.Errorf("hubreconcile: %s is not one of the hub's code worktrees", pairPath)
}

// findPrime returns the main code worktree of a listing.
func findPrime(worktrees []fabricengine.CodeWorktree) (fabricengine.CodeWorktree, bool) {
	for _, w := range worktrees {
		if w.Main {
			return w, true
		}
	}
	return fabricengine.CodeWorktree{}, false
}

// reconcileHubWide reconciles the hub-wide config at boardDir, seeded from the prime's anchor, and commits what it wrote on the board.
// A push failure is logged and never fatal.
func reconcileHubWide(boardDir, primeAnchor, revision string) error {
	bolt := fabricengine.NewBolt(boardDir)
	_, committed, err := bolt.CommitWritten("lyx: reconcile hub-wide config for build "+revision, func() ([]string, error) {
		results, err := configsync.ReconcileHubWideAt(boardDir, primeAnchor, true)
		if err != nil {
			return nil, err
		}
		var written []string
		for _, result := range results {
			if !result.Applied {
				continue
			}
			logResult(boardDir, result)
			written = append(written, configengine.ConfigFileRel(result.Module))
			for _, legacy := range result.MigratedFrom {
				written = append(written, configengine.ConfigFileRel(legacy))
			}
		}
		return written, nil
	}, fabricengine.SyncOptions{})
	if err != nil {
		return worktreeErrorFor(boardDir, err)
	}
	if !committed {
		return nil
	}
	if pushErr := bolt.Push(fabricengine.SyncOptions{}); pushErr != nil {
		logger.Warn("hubreconcile: hub-wide config committed but push failed", "board", boardDir, "error", pushErr)
	}
	return nil
}

// reconcileWorktree reconciles and commits one code worktree's config.
// It reports skipped when a mid-merge state keeps the worktree from being written or committed, which leaves the build stamp stale.
// A pair that is incomplete or has been removed is passed over without being reported skipped.
func reconcileWorktree(boardDir string, w fabricengine.CodeWorktree, revision string, hooks walkHooks) (skipped bool, err error) {
	if !w.Main {
		complete, reason, err := fabricengine.PairCompleteAt(w.Path)
		if err != nil {
			return passOverOrFail(w.Path, err)
		}
		if !complete {
			logger.Info("hubreconcile: skipping an incomplete pair", "worktree", w.Path, "reason", reason)
			return false, nil
		}
	}

	f, err := fabricengine.OpenCodeWorktree(w.Path)
	if err != nil {
		return passOverOrFail(w.Path, err)
	}
	blocked, err := f.MergeBlocked()
	if err != nil {
		return false, &WorktreeError{Worktree: w.Path, Err: err}
	}
	if blocked {
		logger.Info("hubreconcile: skipping a worktree that is mid-merge", "worktree", w.Path)
		return true, nil
	}

	prior := snapshotConfig(w.Anchor)
	results, err := configsync.ReconcileAll(w.Anchor, boardDir, true)
	if err != nil {
		if _, statErr := os.Stat(w.Path); errors.Is(statErr, fs.ErrNotExist) {
			logger.Info("hubreconcile: skipping a worktree removed during the walk", "worktree", w.Path)
			return false, nil
		}
		return false, worktreeErrorFor(w.Path, err)
	}

	anchorRel, err := filepath.Rel(w.Path, w.Anchor)
	if err != nil {
		return false, &WorktreeError{Worktree: w.Path, Err: err}
	}
	var files []string
	for _, result := range results {
		if !result.Applied {
			continue
		}
		logResult(w.Path, result)
		files = append(files, filepath.Join(anchorRel, configengine.ConfigFileRel(result.Module)))
		for _, legacy := range result.MigratedFrom {
			files = append(files, filepath.Join(anchorRel, configengine.ConfigFileRel(legacy)))
		}
	}

	if hooks.beforeCommit != nil {
		hooks.beforeCommit(w.Path)
	}
	if len(files) == 0 {
		return false, nil
	}
	if _, err := f.Commit(files, "lyx: reconcile config for build "+revision, nil, fabricengine.SyncOptions{}); err != nil {
		var mergeInProgress *fabricengine.ErrMergeInProgress
		var foreignMergeState *fabricengine.ErrForeignMergeState
		if errors.As(err, &mergeInProgress) || errors.As(err, &foreignMergeState) {
			if restoreErr := restoreConfig(prior); restoreErr != nil {
				return false, &WorktreeError{Worktree: w.Path, Err: restoreErr}
			}
			logger.Info("hubreconcile: a merge began before the commit, so the config was restored", "worktree", w.Path)
			return true, nil
		}
		return false, &WorktreeError{Worktree: w.Path, Err: err}
	}
	return false, nil
}

// passOverOrFail maps an error from opening a worktree: a path that no longer exists is passed over, anything else fails the walk.
func passOverOrFail(worktreePath string, err error) (skipped bool, failure error) {
	var missing *fabricengine.ErrMissingPath
	if errors.As(err, &missing) {
		logger.Info("hubreconcile: skipping a worktree that no longer exists", "worktree", worktreePath)
		return false, nil
	}
	return false, &WorktreeError{Worktree: worktreePath, Err: err}
}

// worktreeErrorFor wraps a reconcile failure, taking the file and module from a *configsync.FileError.
func worktreeErrorFor(worktree string, err error) *WorktreeError {
	var fileErr *configsync.FileError
	if errors.As(err, &fileErr) {
		return &WorktreeError{Worktree: worktree, File: fileErr.Path, Module: fileErr.Module, Err: fileErr.Err}
	}
	return &WorktreeError{Worktree: worktree, Err: err}
}

// logResult logs one changed module.
func logResult(worktree string, result configsync.Result) {
	logger.Info("hubreconcile: config changed", "worktree", worktree, "module", result.Module,
		"added", result.Added, "removed", result.Removed, "migrated", result.Migrated)
}

// snapshotConfig reads every registry module's config file under anchor; an absent file maps to nil.
func snapshotConfig(anchor string) map[string][]byte {
	prior := make(map[string][]byte)
	for _, module := range configreg.Modules() {
		path := configengine.ConfigFile(anchor, module.Name)
		data, err := os.ReadFile(path)
		switch {
		case err != nil:
			prior[path] = nil
		case data == nil:
			prior[path] = []byte{}
		default:
			prior[path] = data
		}
	}
	return prior
}

// restoreConfig puts every file of a snapshot back to its prior bytes, removing the files that were absent.
func restoreConfig(prior map[string][]byte) error {
	var errs []error
	for path, data := range prior {
		if data == nil {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		if err := fsx.AtomicWriteBytes(path, data); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
