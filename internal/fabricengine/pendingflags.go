// pendingflags.go holds the pending-flag protocol of fabric's records push lock, fabric.push.lock.
// A pusher marks the sides it owes before it tries the lock.
// The holder pushes under the lock and re-reads the flags after releasing,
// so a pusher that finds the lock held may leave its commits to the holder and exit.

package fabricengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

const (
	// pushSideCode and pushSideRecords name the two sides a push request can owe; they are the keys of the owed map.
	pushSideCode    = "code"
	pushSideRecords = "records"
)

// errPendingFlagCorrupt marks a pending flag whose content does not decode.
// Errors that report such a flag wrap it.
var errPendingFlagCorrupt = errors.New("fabricengine: pending flag does not decode")

// takenFlagCounter numbers the private names a taken pending flag is renamed to.
// Each name is unique within the process.
var takenFlagCounter atomic.Uint64

// pendingFlagPath returns the path of side's pending flag in lockDir, the lock directory that holds fabric.push.lock.
func pendingFlagPath(lockDir, side string) string {
	return filepath.Join(lockDir, "pending."+side)
}

// PairPushDeferred reports whether a detached push of the pair at l left a side's pending flag on disk.
// A push that a pause deferred leaves one.
// The flag stays until the pair's next push carries that side, so a lift reads from it whether a push is owed.
func PairPushDeferred(l *lymcwd.Location) bool {
	lockDir := fabriclayout.RecordsLockDirPath(RecordsWorktree(l))
	for _, side := range []string{pushSideCode, pushSideRecords} {
		if pathPresent(pendingFlagPath(lockDir, side)) {
			return true
		}
	}
	return false
}

// exitDeferred ends a detached push child that a pause covers; a test swaps it for a recorder.
var exitDeferred = func() { os.Exit(0) }

// pushDeferral returns the function a detached push child's gate calls when a pause covers it:
// it marks every owed side pending again, logs the sides, and calls exit.
// The gate is passed before each acquire of the push lock and stays passed while the lock is held.
// On the first acquire nothing has been pushed yet.
// On a later acquire, after earlier steps pushed, the flags it puts back may name sides with nothing left to push.
// The commits not yet pushed stay local and the flags stay on disk for the next push,
// so the push is deferred and not failed.
func pushDeferral(lockDir string, owed map[string]fabriclayout.PushSpawn, exit func()) func() {
	return func() {
		var sides []string
		for side := range owed {
			sides = append(sides, side)
		}
		sort.Strings(sides)
		if err := markPending(lockDir, owed); err != nil {
			logger.Warn("fabricengine: pending flags of a deferred push not written; the commits stay local for the next push", "lock_dir", lockDir, "error", err)
		}
		logger.Info("fabricengine: a pause covers the push; exiting with its pending flags put back", "sides", sides)
		exit()
	}
}

// markPending writes one flag per owed side into lockDir.
// Each flag holds that side's path, the slugs of the push it serves, and its parent-locked mark.
// A flag is written atomically, so a reader sees no flag or a whole one.
func markPending(lockDir string, owed map[string]fabriclayout.PushSpawn) error {
	for side, req := range owed {
		narrowed := fabriclayout.PushSpawn{PauseSlug: req.PauseSlug, TeardownSlug: req.TeardownSlug, ParentLocked: req.ParentLocked}
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
// A flag whose content does not decode is still removed and reported found.
// The error wraps errPendingFlagCorrupt and the JSON error.
func takePending(lockDir, side string) (req fabriclayout.PushSpawn, found bool, err error) {
	flag := pendingFlagPath(lockDir, side)
	taken := fmt.Sprintf("%s.taken.%d.%d", flag, os.Getpid(), takenFlagCounter.Add(1))
	if err := os.Rename(flag, taken); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fabriclayout.PushSpawn{}, false, nil
		}
		return fabriclayout.PushSpawn{}, false, fmt.Errorf("fabricengine: take pending flag %s: %w", flag, err)
	}
	content, readErr := os.ReadFile(taken)
	removeErr := os.Remove(taken)
	if readErr != nil {
		return fabriclayout.PushSpawn{}, true, fmt.Errorf("fabricengine: read pending flag %s: %w", flag, readErr)
	}
	if removeErr != nil {
		return fabriclayout.PushSpawn{}, true, fmt.Errorf("fabricengine: remove pending flag %s: %w", flag, removeErr)
	}
	if err := json.Unmarshal(content, &req); err != nil {
		return fabriclayout.PushSpawn{}, true, fmt.Errorf("%w: %s: %w", errPendingFlagCorrupt, flag, err)
	}
	return req, true, nil
}

// coalesceFlagged is the holder loop of fabric.push.lock under lockDir.
// It marks the owed sides pending and acquires the lock.
// Before each acquire it passes the process's pause gate, and it stays admitted until it releases the lock,
// so a pause holds it only while it holds no lock and never while other pushers wait on it.
// It writes its holder record.
// It runs step with the owed flags taken before each run, looping while step reports progress.
// After the last run it releases the lock and re-reads the flags.
// A flag of an owed side set again sends the loop back to acquiring,
// and a flag of a side it does not owe stays on disk for its worktree's next push.
// acquired is false only when the first acquire missed, which leaves the commits and the flags to the current holder;
// a missed re-acquire after a step ran reports true, since the caller's own push landed.
// A step error releases the lock and is returned.
func coalesceFlagged(lockDir string, owed map[string]fabriclayout.PushSpawn, acquire func(lockPath string) (*lock.FileLock, bool, error), step func() (progressed bool, err error)) (acquired bool, err error) {
	if err := markPending(lockDir, owed); err != nil {
		return false, err
	}
	lockPath := filepath.Join(lockDir, weftPushLockFile)

	for {
		leaveGate := pauserecord.Admit(pushLockGateOp)
		held, ok, err := acquire(lockPath)
		if err != nil || !ok {
			leaveGate()
			return acquired, err
		}
		acquired = true
		err = runHeld(holdPushLock(held, lockDir, time.Now()), lockDir, owed, step)
		leaveGate()
		if err != nil {
			return true, err
		}

		if ownedFlagSet(lockDir, owed) {
			continue
		}
		return true, nil
	}
}

// runHeld runs step under the held push lock, taking the owed flags before each run.
// It calls release, the lock's release, before returning.
func runHeld(release func() error, lockDir string, owed map[string]fabriclayout.PushSpawn, step func() (bool, error)) (err error) {
	defer func() {
		if releaseErr := release(); err == nil {
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
func ownedFlagSet(lockDir string, owed map[string]fabriclayout.PushSpawn) bool {
	for side := range owed {
		if _, err := os.Stat(pendingFlagPath(lockDir, side)); err == nil {
			return true
		}
	}
	return false
}
