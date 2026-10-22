// pendingflags.go holds the pending-flag protocol of fabric's records push lock, fabric.push.lock.
// A pusher marks the sides it owes before it tries the lock, the holder pushes under the lock and re-reads the flags after releasing,
// so a pusher that finds the lock held may leave its commits to the holder and exit.

package fabricengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/weftname"
)

const (
	// pushSideCode and pushSideRecords name the two sides a push request can owe, the keys of coalesceFlagged's owed map.
	pushSideCode    = "code"
	pushSideRecords = "records"
)

// errPendingFlagCorrupt is wrapped by the error takePending returns for a flag whose content does not decode.
var errPendingFlagCorrupt = errors.New("fabricengine: pending flag does not decode")

// takenFlagCounter makes each takePending's private file name unique within the process.
var takenFlagCounter atomic.Uint64

// pendingFlagPath returns the path of side's pending flag in lockDir, the lock directory that holds fabric.push.lock.
func pendingFlagPath(lockDir, side string) string {
	return filepath.Join(lockDir, "pending."+side)
}

// exitDeferred ends a detached push child that a pause covers; a test swaps it for a recorder.
var exitDeferred = func() { os.Exit(0) }

// pushDeferral returns the function a detached push child's gate calls when a pause covers it:
// it marks pending again every owed side not in pushed, logs the sides left local, and calls exit.
// The commits stay local and the flags stay on disk for the next push, so the push is deferred and not failed.
func pushDeferral(lockDir string, owed map[string]PushSpawn, pushed map[string]bool, exit func()) func() {
	return func() {
		left := map[string]PushSpawn{}
		var sides []string
		for side, req := range owed {
			if !pushed[side] {
				left[side] = req
				sides = append(sides, side)
			}
		}
		sort.Strings(sides)
		if err := markPending(lockDir, left); err != nil {
			logger.Warn("fabricengine: pending flags of a deferred push not written; the commits stay local for the next push", "lock_dir", lockDir, "error", err)
		}
		logger.Info("fabricengine: a pause covers the push; exiting with the commits left local", "sides", sides)
		exit()
	}
}

// markPending writes one flag per owed side into lockDir, each holding that side's path, the slugs of the push it serves and its parent-locked mark, and nothing else.
// A flag is written atomically, so a reader sees no flag or a whole one.
func markPending(lockDir string, owed map[string]PushSpawn) error {
	for side, req := range owed {
		narrowed := PushSpawn{PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug, ParentLocked: req.ParentLocked}
		switch side {
		case pushSideCode:
			narrowed.WarpPath = req.WarpPath
		case pushSideRecords:
			narrowed.WeftPath = req.WeftPath
		default:
			return fmt.Errorf("fabricengine: unknown push side %q", side)
		}
		content, err := json.Marshal(narrowed)
		if err != nil {
			return fmt.Errorf("fabricengine: encode pending flag %s: %w", pendingFlagPath(lockDir, side), err)
		}
		if err := fsx.AtomicWriteBytes(pendingFlagPath(lockDir, side), content); err != nil {
			return fmt.Errorf("fabricengine: write pending flag: %w", err)
		}
	}
	return nil
}

// takePending takes side's pending flag out of lockDir and reports whether one was there.
// It renames the flag to a name private to this call before reading and removing it,
// so no process holds a file at a flag path open and a writer's rename onto that path never meets a sharing violation.
// A flag whose content does not decode is still removed and reported found, with an error wrapping errPendingFlagCorrupt and the JSON error.
func takePending(lockDir, side string) (req PushSpawn, found bool, err error) {
	flag := pendingFlagPath(lockDir, side)
	taken := fmt.Sprintf("%s.taken.%d.%d", flag, os.Getpid(), takenFlagCounter.Add(1))
	if err := os.Rename(flag, taken); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PushSpawn{}, false, nil
		}
		return PushSpawn{}, false, fmt.Errorf("fabricengine: take pending flag %s: %w", flag, err)
	}
	content, readErr := os.ReadFile(taken)
	removeErr := os.Remove(taken)
	if readErr != nil {
		return PushSpawn{}, true, fmt.Errorf("fabricengine: read pending flag %s: %w", flag, readErr)
	}
	if removeErr != nil {
		return PushSpawn{}, true, fmt.Errorf("fabricengine: remove pending flag %s: %w", flag, removeErr)
	}
	if err := json.Unmarshal(content, &req); err != nil {
		return PushSpawn{}, true, fmt.Errorf("%w: %s: %w", errPendingFlagCorrupt, flag, err)
	}
	return req, true, nil
}

// coalesceFlagged is the holder loop of fabric.push.lock under lockDir.
// It marks the owed sides pending, acquires the lock and runs step with the owed flags taken before each run, looping while step reports progress.
// After the last run it releases the lock and re-reads the flags:
// a flag of an owed side set again sends it back to acquiring, and a flag of a side it does not push is respawned as a detached push of that side.
// acquired is false only when the first acquire missed, which leaves the commits and the flags to the current holder;
// a missed re-acquire after a step ran reports true, since the caller's own push landed.
// A step error releases the lock and is returned.
func coalesceFlagged(lockDir string, owed map[string]PushSpawn, acquire func(lockPath string) (*lock.FileLock, bool, error), step func() (progressed bool, err error)) (acquired bool, err error) {
	if err := markPending(lockDir, owed); err != nil {
		return false, err
	}
	lockPath := filepath.Join(lockDir, weftPushLockFile)

	for {
		held, ok, err := acquire(lockPath)
		if err != nil {
			return acquired, err
		}
		if !ok {
			return acquired, nil
		}
		acquired = true
		if err := runHeld(held, lockDir, owed, step); err != nil {
			return true, err
		}

		if ownedFlagSet(lockDir, owed) {
			continue
		}
		respawnForeignFlags(lockDir, owed)
		return true, nil
	}
}

// runHeld runs step under held, taking the owed flags before each run, and releases held before returning.
func runHeld(held *lock.FileLock, lockDir string, owed map[string]PushSpawn, step func() (bool, error)) (err error) {
	defer func() {
		if releaseErr := held.Release(); err == nil {
			err = releaseErr
		}
	}()
	for {
		for side := range owed {
			// A corrupt flag of an owed side is taken like a whole one, since the holder pushes that side from its own request.
			if _, _, err := takePending(lockDir, side); err != nil && !errors.Is(err, errPendingFlagCorrupt) {
				return err
			}
		}
		progressed, err := step()
		if err != nil {
			return err
		}
		if !progressed {
			return nil
		}
	}
}

// ownedFlagSet reports whether a flag of any owed side is on disk.
func ownedFlagSet(lockDir string, owed map[string]PushSpawn) bool {
	for side := range owed {
		if _, err := os.Stat(pendingFlagPath(lockDir, side)); err == nil {
			return true
		}
	}
	return false
}

// respawnForeignFlags takes the flag of each side owed is missing and spawns that side's push with the flag's validated side path and slugs.
// A flag that does not decode, names a path that is not this lock directory's own worktree or carries a slug that is not a plain name is dropped with a Warn.
// The spawn starts in the board directory, never in the holder's own directory, which may be a pair worktree.
func respawnForeignFlags(lockDir string, owed map[string]PushSpawn) {
	for _, side := range []string{pushSideCode, pushSideRecords} {
		if _, isOwed := owed[side]; isOwed {
			continue
		}
		flag, found, err := takePending(lockDir, side)
		if !found {
			if err != nil {
				logger.Warn("fabricengine: pending flag could not be taken; dropped", "side", side, "lock_dir", lockDir, "error", err)
			}
			continue
		}
		if err != nil {
			logger.Warn("fabricengine: pending flag unusable; dropped", "flag", pendingFlagPath(lockDir, side), "error", err)
			continue
		}
		req, reason := foreignFlagRequest(lockDir, side, flag)
		if reason != "" {
			logger.Warn("fabricengine: pending flag rejected; dropped", "flag", pendingFlagPath(lockDir, side), "reason", reason)
			continue
		}
		if err := spawnDetachedPushFn(req); err != nil {
			logger.Warn("fabricengine: respawn of a pending push failed; the commits stay local for the next push", "flag", pendingFlagPath(lockDir, side), "error", err)
		}
	}
}

// foreignFlagRequest builds the detached push request a flag of side asks for, or names why the flag cannot be trusted.
// Only side's own path is kept, and it must be the worktree lockDir belongs to (records) or its paired code worktree (code).
func foreignFlagRequest(lockDir, side string, flag PushSpawn) (req PushSpawn, reason string) {
	for name, slug := range map[string]string{"PauseSlug": flag.PauseSlug, "TeardownSlug": flag.TeardownSlug} {
		if slug == ".." || strings.ContainsAny(slug, `/\`) {
			return PushSpawn{}, fmt.Sprintf("%s %q is not a plain name", name, slug)
		}
	}

	recordsWorktree := filepath.Dir(lockDir)
	req = PushSpawn{
		PauseSlug:    flag.PauseSlug,
		TeardownSlug: flag.TeardownSlug,
		ParentLocked: flag.ParentLocked,
		WorkDir:      BoardDir(filepath.Dir(recordsWorktree)),
	}
	switch side {
	case pushSideRecords:
		if flag.WeftPath == "" || RecordsLockDirPath(flag.WeftPath) != lockDir {
			return PushSpawn{}, fmt.Sprintf("records path %q is not the worktree of lock directory %s", flag.WeftPath, lockDir)
		}
		req.WeftPath = flag.WeftPath
	case pushSideCode:
		if flag.WarpPath == "" || RecordsLockDirPath(weftname.SiblingPath(filepath.Dir(flag.WarpPath), filepath.Base(flag.WarpPath))) != lockDir {
			return PushSpawn{}, fmt.Sprintf("code path %q is not the paired worktree of lock directory %s", flag.WarpPath, lockDir)
		}
		req.WarpPath = flag.WarpPath
		// The child's coalescing push requires the records path even for a code-only flag.
		req.WeftPath = recordsWorktree
	}
	return req, ""
}
