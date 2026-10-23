// cancel.go holds the cancel phase of the teardown: it ends a pair's owned processes and strands first and waits for the run lock after, for a caller that ends the run instead of waiting for it.

package pairteardown

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// CancelLockWait is the bound on every wait of the cancel phase, and the one a caller's own lock and marker waits share with it.
const CancelLockWait = runLockWait

// CancelRequest names the pair a cancel ends and the caller's part of the phase.
type CancelRequest struct {
	// Slug names the pair.
	Slug string
	// Keep says the pair stays, so the loop lock is released with the teardown's mark withdrawn.
	Keep bool
	// AfterEnd runs once the pair's owned processes are ended, and takes the caller's own locks.
	// A non-empty busy names a lock path still held; a hook that reports busy holds nothing, having released whatever it took.
	// The caller releases whatever the hook took on every return of the phase, a refusal included.
	// A nil hook takes nothing.
	AfterEnd func() (busy string, err error)
}

// CancelResult reports what the cancel phase ended.
type CancelResult struct {
	// Ended lists the registered processes of the pair, and the processes below them, that the owner-record end signalled and found gone.
	Ended []runproc.Ended
	// StrandsStopped names the live strands of the task worktree that were stopped.
	StrandsStopped []string
	// SessionEnded is true when the session end ran.
	SessionEnded bool
	// TaskWorktreeGone is true when the task worktree was already gone, so no strand was stopped through shuttle.
	TaskWorktreeGone bool
}

// EndForCancel ends the pair's registered processes and strands, then its loop and session, for a caller that ends the run for good.
// It skips the quiet wait, the removal refusal probe and the pre-end run-lock wait, and waits for the run lock after the processes are ended.
// With the task worktree present the order is: the owned-process end under the prime's and the pair's records write locks, the driver-lock probe, req.AfterEnd, the strand stop, the loop end, the run-lock wait and the session end;
// with it gone, only the owned-process end, req.AfterEnd and the session end by name run.
// A hook that reports busy gets the owned-process end and the probe once more, then the hook again; a lock still busy then is refused.
// Every refusal wraps ErrDriverBusy where a busy holder caused it and names `lyx batten cancel <slug>` as the way to run the phase again.
func (t *Teardown) EndForCancel(ctx context.Context, req CancelRequest) (CancelResult, error) {
	var res CancelResult
	gone, err := t.gone(req.Slug)
	if err != nil {
		return res, err
	}
	res.TaskWorktreeGone = gone
	retry := cancelRetry(req.Slug)

	endOwnedAndProbe := func() error {
		if err := t.endOwnedUnderWriteLocks(req.Slug, &res); err != nil {
			return err
		}
		if gone {
			return nil
		}
		held, err := t.driverLockHeld(req.Slug)
		if err != nil {
			return err
		}
		if held {
			return t.driverLockBusyError(req.Slug, retry)
		}
		return nil
	}

	if err := endOwnedAndProbe(); err != nil {
		return res, err
	}
	if req.AfterEnd != nil {
		busy, err := req.AfterEnd()
		if err != nil {
			return res, err
		}
		if busy != "" {
			if err := endOwnedAndProbe(); err != nil {
				return res, err
			}
			busy, err = req.AfterEnd()
			if err != nil {
				return res, err
			}
			if busy != "" {
				return res, fmt.Errorf("%w: %s is still held after the run's processes were ended: a batten of %s or \"lyx batten cleanup\" may hold it; see the holder with \"lyx tender status\" or \"ps\", or in Task Manager on Windows, and run \"lyx batten cancel %s\" again once it is free",
					ErrDriverBusy, busy, req.Slug, req.Slug)
			}
		}
	}

	var releaseLoopLock func(sessionEnded bool) error
	if !gone {
		res.StrandsStopped, err = t.stopStrands(req.Slug)
		if err != nil {
			return res, fmt.Errorf("stop the pair's strands: %w; for this cancel, run %s again", err, retry)
		}
		releaseLoopLock, err = t.seizeLoop(ctx, req.Slug, func() (bool, error) { return t.killLoop(req.Slug) })
		if err != nil {
			return res, cancelAgain(err, req.Slug)
		}
		if err := t.awaitRunLock(ctx, req.Slug); err != nil {
			return res, errors.Join(cancelAgain(err, req.Slug), releaseLoopLock(false))
		}
	}

	ended, abandoned, err := t.endSession(req.Slug, gone)
	res.SessionEnded = ended
	if abandoned != "" {
		logger.Warn("pairteardown: a foreign session was left alone by the cancel's session end", "slug", req.Slug, "session", abandoned)
	}
	if releaseLoopLock != nil {
		err = errors.Join(err, releaseLoopLock(err == nil && !req.Keep))
	}
	if err != nil {
		return res, err
	}
	logger.Info("pairteardown: cancel ended the pair", "slug", req.Slug, "ended_processes", len(res.Ended), "strands_stopped", len(res.StrandsStopped), "task_worktree_gone", gone)
	return res, nil
}

// cancelRetry names the command that runs the cancel phase again, the way forward of every refusal the phase raises.
func cancelRetry(slug string) string {
	return fmt.Sprintf("`lyx batten cancel %s`", slug)
}

// cancelAgain wraps err, which names retrying the removal, with the cancel's own re-entry.
func cancelAgain(err error, slug string) error {
	return fmt.Errorf("%w; for this cancel, run \"lyx batten cancel %s\" again once the holder ends", err, slug)
}

// endOwnedUnderWriteLocks ends the pair's registered processes under the records write locks and releases the locks as soon as the end returns.
// The ended processes are added to res.
func (t *Teardown) endOwnedUnderWriteLocks(slug string, res *CancelResult) error {
	release, err := t.holdWriteLocks(slug)
	if err != nil {
		return err
	}
	ended, endErr := t.endOwned(slug)
	releaseErr := release()
	res.Ended = append(res.Ended, ended...)

	var survivors *runproc.SurvivorsError
	switch {
	case errors.As(endErr, &survivors):
		return errors.Join(cancelSurvivorsError(slug, survivors.Survivors, t.goos), releaseErr)
	case endErr != nil:
		return errors.Join(fmt.Errorf("end the pair's registered processes: %w", endErr), releaseErr)
	}
	return releaseErr
}

// cancelSurvivorsError builds the ErrDriverBusy refusal for registered processes that survived the cancel's end.
// It names each survivor and the way to find and end it on goos, then `lyx batten cancel <slug>` again.
func cancelSurvivorsError(slug string, survivors []runproc.Ended, goos string) error {
	find := "find and end them with `lyx tender status` and `ps`"
	if goos == "windows" {
		find = "find and end them in Task Manager"
	}
	return fmt.Errorf("%w: processes of pair %q survived the end: %s; %s, then run %s again", ErrDriverBusy, slug, survivorNames(survivors), find, cancelRetry(slug))
}

// holdRecordsWriteLocks takes the prime's records write lock and, while the pair's records worktree stands, the pair's, each bounded by t.writeLockWait.
// The returned release drops what was taken.
// A lock still held at the bound releases what was taken and refuses with an ErrDriverBusy that names the lock's path.
func (t *Teardown) holdRecordsWriteLocks(slug string) (release func() error, err error) {
	worktrees := []string{fabricengine.RecordsWorktree(t.prime)}
	if pair := fabricengine.RecordsWorktreePath(t.prime, slug); fileExists(pair) {
		worktrees = append(worktrees, pair)
	}

	var releases []func() error
	releaseAll := func() error {
		var errs []error
		for i := len(releases) - 1; i >= 0; i-- {
			errs = append(errs, releases[i]())
		}
		return errors.Join(errs...)
	}
	for _, worktree := range worktrees {
		releaseOne, held, err := fabricengine.HoldRecordsWriteLock(worktree, t.writeLockWait)
		if err != nil {
			return nil, errors.Join(err, releaseAll())
		}
		if !held {
			return nil, errors.Join(writeLockBusyError(slug, fabricengine.RecordsLockDirPath(worktree)), releaseAll())
		}
		releases = append(releases, releaseOne)
	}
	return releaseAll, nil
}

// writeLockBusyError builds the ErrDriverBusy refusal for a records write lock still held at the bound, naming its directory and the way forward.
// The prime's lock is shared by the hub, so its holder is often another run's or the orch's healthy commit; ending it is the last resort.
func writeLockBusyError(slug, lockDir string) error {
	return fmt.Errorf("%w: the records write lock in %s is held; see the holder with `lyx tender status` or `ps`, or in Task Manager on Windows, and run %s again once it lets go, ending the holder only when the same process keeps the lock across re-runs",
		ErrDriverBusy, lockDir, cancelRetry(slug))
}

// stopPairStrands stops every live strand of the present task worktree through shuttle's stop verb and returns their names.
// A strand stopped before a failure stays in the returned names.
func (t *Teardown) stopPairStrands(slug string) ([]string, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return nil, err
	}
	reed, err := t.reedEngine(task)
	if err != nil {
		return nil, err
	}
	rows, err := reed.Directory()
	if err != nil {
		return nil, err
	}
	guids, names := liveStrandGUIDs(rows)
	if len(guids) == 0 {
		return nil, nil
	}

	configDirs := fabricengine.ConfigDirs(task)
	shuttleCfg, err := shuttleengine.LoadConfig(configDirs, "shuttle")
	if err != nil {
		return nil, err
	}
	engines, err := shuttleengines.New(shuttleCfg, configDirs)
	if err != nil {
		return nil, err
	}
	geom, err := hubgeom.ReedGeometry(task)
	if err != nil {
		return nil, err
	}
	pauseGeom, err := hubgeom.PauseGeometry(task)
	if err != nil {
		return nil, err
	}
	runner := shuttleengine.NewRunner(reed, engines, geom.AnchorPath, geom.WorktreeRoot, shuttleCfg)
	runner.SetPause(pauseGeom)

	var stopped []string
	for i, guid := range guids {
		if err := runner.StopStrand(guid); err != nil {
			return stopped, fmt.Errorf("stop strand %s: %w", names[i], err)
		}
		stopped = append(stopped, names[i])
	}
	return stopped, nil
}

// liveStrandGUIDs returns the guid and the name of every live row, in row order, whatever the row's name.
func liveStrandGUIDs(rows []reedengine.DirectoryRow) (guids, names []string) {
	for _, row := range rows {
		if row.Live {
			guids = append(guids, row.GUID)
			names = append(names, row.Name)
		}
	}
	return guids, names
}
