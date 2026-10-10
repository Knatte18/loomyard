// loop.go ends a pair's detached step loop ahead of the session end: it kills the loop and its in-flight step tree, holds the loop lock through the session end and waits for the run lock.

package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// runLockWait bounds the wait for the loop lock to be taken and for the run lock to be released.
const runLockWait = 2 * time.Minute

// errRunLockHeld is the sentinel awaitRunLockFree returns once its bound is spent with the run lock still held.
var errRunLockHeld = errors.New("the run lock is still held")

// killLoop kills the loop the pid file at pidPath records and the step tree it recorded, and reports whether it killed anything.
// An absent pid file, and the teardown's own mark, kill nothing.
// On Linux the loop is killed by pid only while the loop lock at lockPath is held and the pid still has the recorded start time;
// on Windows the loop's named job jobName is terminated, which ends the loop, its child and their descendants at once.
// The pid file is left in place.
func killLoop(pidPath, lockPath, jobName string) (bool, error) {
	rec, found, err := shedverbs.ReadLoopPIDRecord(pidPath)
	if err != nil || !found || rec.LoopID == shedverbs.TeardownLoopID {
		return false, err
	}
	loopKilled, loopErr := killLoopProcessPlatform(rec, lockPath, jobName)
	logger.Info("pairteardown: killed the loop", "loop_id", rec.LoopID, "pid", rec.Loop.PID, "job", jobName, "killed", loopKilled, "error", errorText(loopErr))
	treeKilled, treeErr := proc.KillTree(rec.Child)
	logger.Info("pairteardown: killed the loop's step tree", "loop_id", rec.LoopID, "group", rec.Child.PGID, "pid", rec.Child.PID, "killed", treeKilled, "error", errorText(treeErr))
	return loopKilled || treeKilled, errors.Join(loopErr, treeErr)
}

// seizeLoopLock takes the loop lock at lockPath, killing the loop through kill before each attempt, and returns the function that releases it.
// A loop spawned between a kill and the take holds the lock, so the kill runs again and the take is retried through sleep, at quietPollInterval, for at most attempts retries.
func seizeLoopLock(ctx context.Context, kill func() (bool, error), lockPath string, attempts int, sleep func(context.Context, time.Duration) error) (func() error, error) {
	for i := 0; ; i++ {
		if _, err := kill(); err != nil {
			return nil, err
		}
		held, ok, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			return nil, err
		}
		if ok {
			return held.Release, nil
		}
		if i >= attempts {
			return nil, fmt.Errorf("the loop lock %s is still held after the loop was killed; retry the removal once its holder ends", lockPath)
		}
		if err := sleep(ctx, quietPollInterval); err != nil {
			return nil, err
		}
	}
}

// holdLoopLock kills the loop through kill, takes the loop lock at lockPath and replaces the pid file at pidPath with the teardown's mark, a record naming no loop process and no child.
// The returned release function drops the lock.
// With sessionEnded true it leaves the mark, which bars every later loop up to the removal;
// with false it first puts back the pid file it replaced, or removes the mark when there was none.
func holdLoopLock(ctx context.Context, kill func() (bool, error), pidPath, lockPath string, attempts int, sleep func(context.Context, time.Duration) error) (func(sessionEnded bool) error, error) {
	releaseLock, err := seizeLoopLock(ctx, kill, lockPath, attempts, sleep)
	if err != nil {
		return nil, err
	}
	prior, hadPrior, err := shedverbs.ReadLoopPIDRecord(pidPath)
	if err == nil {
		err = shedverbs.WriteLoopPIDRecord(pidPath, shedverbs.LoopPIDRecord{LoopID: shedverbs.TeardownLoopID})
	}
	if err != nil {
		return nil, errors.Join(err, releaseLock())
	}
	return func(sessionEnded bool) error {
		var restoreErr error
		switch {
		case sessionEnded:
		case hadPrior:
			restoreErr = shedverbs.WriteLoopPIDRecord(pidPath, prior)
		default:
			if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				restoreErr = err
			}
		}
		return errors.Join(restoreErr, releaseLock())
	}, nil
}

// awaitRunLockFree waits until the run lock at lockPath is released, retrying through sleep, at quietPollInterval, for at most attempts retries.
// It returns errRunLockHeld once the bound is spent.
func awaitRunLockFree(ctx context.Context, lockPath string, attempts int, sleep func(context.Context, time.Duration) error) error {
	for i := 0; ; i++ {
		free, err := runLockFree(lockPath)
		if err != nil {
			return err
		}
		if free {
			return nil
		}
		if i >= attempts {
			return errRunLockHeld
		}
		if err := sleep(ctx, quietPollInterval); err != nil {
			return err
		}
	}
}

// awaitRunLockNamingHolder is awaitRunLockFree whose spent bound names the holder: the pid of the newest unfinished step record in stepsDir, or an unknown process when none is unfinished, as for a Go driver, which writes none.
// The loop's pid file never names the holder, since the loop and its child are dead by then.
// The error ends with the way forward of retrying the removal once the holder ends.
func awaitRunLockNamingHolder(ctx context.Context, lockPath, stepsDir, slug string, attempts int, sleep func(context.Context, time.Duration) error) error {
	err := awaitRunLockFree(ctx, lockPath, attempts, sleep)
	if !errors.Is(err, errRunLockHeld) {
		return err
	}
	holder := "an unknown process"
	if pid, ok := shedverbs.RunningStepPID(stepsDir); ok {
		holder = fmt.Sprintf("pid %d", pid)
	}
	return fmt.Errorf("%w by %s in pair %q; retry the removal once the holder ends", err, holder, slug)
}

// errorText returns err's text, or the empty string for nil.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
