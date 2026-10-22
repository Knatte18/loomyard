// skip.go decides which pairs skip the quiet wait and waits for such a pair's run lock before its owned processes are ended.
// A holder a pause or a network-gate wait leaves behind passes the wait, since the owned-process end and the loop end reach it.
// It also holds EndBeforeSessionDown, the local-state reading and owned-process end `lyx reed down` runs before it kills a pair's session.

package pairteardown

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// driverChecksSkipped reports whether the pair has nothing left to do, so the quiet wait is not run.
// It is read only while the task worktree is present.
// It reads true, stopping at the first reading that holds, for an ended or owned_elsewhere verdict, a live pause entry covering the slug, a board entry that reads done,
// and a Finalize Done in the task worktree's status history with no live registered batten, since batten still runs Land and deploy after Finalize Done.
// A read error is returned.
func (t *Teardown) driverChecksSkipped(req Request) (bool, error) {
	if req.Verdict == fabricengine.EntryEnded || req.Verdict == fabricengine.EntryOwnedElsewhere {
		return true, nil
	}
	covered, _, err := t.paused(req.Slug)
	if err != nil {
		return false, err
	}
	if covered {
		return true, nil
	}
	status, found, err := t.readClaim(req.Slug)
	if err != nil {
		return false, err
	}
	if found && status != nil && *status == "done" {
		return true, nil
	}
	landed, err := t.landed(req.Slug)
	if err != nil || !landed {
		return false, err
	}
	alive, err := t.battenAlive(req.Slug)
	return !alive, err
}

// ErrRunRunning is the sentinel EndBeforeSessionDown's refusal wraps when the pair's run is running and no pause covers it.
var ErrRunRunning = errors.New("the run is running and unpaused")

// EndBeforeSessionDown ends the pair's registered processes ahead of a session end that is not part of a teardown, and returns the processes it ended.
// It reads only local state, in order, and stops at the first reading that holds:
// a live pause entry covers the slug, the slug's board entry reads done, the status history holds a Finalize Done with no registered batten alive,
// or the run is not running, which is no Go-driver lock held, the loop lock free, no live driver strand and no registered batten alive.
// It then waits for the run lock, which a holder a pause or a network-gate wait leaves passes, and ends the registered processes.
// A run that is running and unpaused is refused with an error wrapping ErrRunRunning, nothing ended.
// A refused run-lock wait is returned as it is, and processes that survive the end are returned wrapped with each survivor's pid.
func (t *Teardown) EndBeforeSessionDown(ctx context.Context, slug string) ([]runproc.Ended, error) {
	covered, hubEntry, err := t.paused(slug)
	if err != nil {
		return nil, err
	}
	settled := covered
	if !settled {
		settled, err = t.settledForSessionDown(slug)
		if err != nil {
			return nil, err
		}
	}
	if !settled {
		return nil, fmt.Errorf("%w: pair %q; pause it with `lyx tender pause %s`, then run `lyx reed down` again", ErrRunRunning, slug, slug)
	}

	if err := t.awaitOrPass(ctx, slug, covered, hubEntry, "`lyx reed down`"); err != nil {
		return nil, err
	}
	ended, err := t.endOwned(slug)
	var survivors *runproc.SurvivorsError
	if errors.As(err, &survivors) {
		return ended, fmt.Errorf("processes of pair %q survived the end: %s; run `lyx reed down` again once they are gone: %w", slug, survivorNames(survivors.Survivors), err)
	}
	if err != nil {
		return ended, fmt.Errorf("end the pair's registered processes: %w", err)
	}
	return ended, nil
}

// settledForSessionDown reports whether the run has nothing left to do without a pause: its board entry reads done, it finished a Finalize and no registered batten is alive, or it is not running.
func (t *Teardown) settledForSessionDown(slug string) (bool, error) {
	status, found, err := t.readClaim(slug)
	if err != nil {
		return false, err
	}
	if found && status != nil && *status == "done" {
		return true, nil
	}
	landed, err := t.landed(slug)
	if err != nil {
		return false, err
	}
	alive, err := t.battenAlive(slug)
	if err != nil {
		return false, err
	}
	if alive {
		return false, nil
	}
	if landed {
		return true, nil
	}
	state, err := t.quiet(slug)
	if err != nil {
		return false, err
	}
	return !state.goDriver && !state.loop && state.driver == "", nil
}

// pausedIn reads the hub's pause record and reports whether a live entry covers the slug, and whether a hub entry is among the live entries.
func (t *Teardown) pausedIn(slug string) (covered, hubEntry bool, err error) {
	geometry, err := hubgeom.PauseGeometry(t.prime)
	if err != nil {
		return false, false, err
	}
	record, err := pauserecord.Read(geometry.RecordPath)
	if err != nil {
		return false, false, err
	}
	_, covered = record.Covering(slug)
	for _, entry := range record.Live {
		if entry.Scope == pauserecord.ScopeHub {
			hubEntry = true
		}
	}
	return covered, hubEntry, nil
}

// awaitRunLockOrPass waits, bounded by runLockWait, for the present task worktree's run lock to be released.
// At the bound it reads the holder, the newest unfinished step's pid or else the slug's registered driver, and returns nil when runLockHolderPasses lets it pass;
// any other holder is refused with runLockRefusal's error, which names retry as the step to run again.
func (t *Teardown) awaitRunLockOrPass(ctx context.Context, slug string, paused, hubEntry bool, retry string) error {
	task, err := t.taskLocation(slug)
	if err != nil {
		return err
	}
	lockPath := shedrun.RunLock(task, shedrun.SelfRunID)
	err = awaitRunLockFree(ctx, lockPath, int(runLockWait/quietPollInterval), t.sleep)
	if !errors.Is(err, errRunLockHeld) {
		return err
	}

	holder, holderKnown := shedverbs.RunningStepPID(shedrun.StepsDir(task, shedrun.SelfRunID))
	if !holderKnown {
		holder, holderKnown = t.driverPID(slug)
	}
	geometry, err := hubgeom.PauseGeometry(t.prime)
	if err != nil {
		return err
	}
	waiting, err := pauserecord.ListWaiting(geometry.RecordPath, parentlock.ProcessAlive)
	if err != nil {
		return err
	}
	store := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(t.prime.HubPath))}
	parentLocked, err := store.HeldBy(slug)
	if err != nil {
		return err
	}
	if runLockHolderPasses(holder, holderKnown, waiting, slug, paused, parentLocked) {
		return nil
	}
	return runLockRefusal(holder, holderKnown, slug, lockPath, t.goos, retry, paused, hubEntry, parentLocked)
}

// runLockHolderPasses reports whether the holder of the run lock is left for the owned-process end and the loop end to reach.
// It passes when a waiting marker of the slug names the holder's own pid, which is a holder waiting at the network gate.
// It also passes when a pause covers the slug with no parent lock record naming it, which is a step waiting on its interrupted agent.
// The paused pass applies to a known and an unknown holder alike.
func runLockHolderPasses(holder int, holderKnown bool, waiting []pauserecord.WaitingMarker, slug string, paused, parentLocked bool) bool {
	if holderKnown {
		for _, marker := range waiting {
			if marker.Slug == slug && marker.PID == holder {
				return true
			}
		}
	}
	return paused && !parentLocked
}

// runLockRefusal builds the ErrDriverBusy refusal for a run lock holder that does not pass, naming the holder and the way forward.
// A paused holder under a parent lock record is told to resume the run so its step finishes its merge, with the resume form hubEntry picks.
// An unpaused holder is told to pause the run first, unless a parent lock record names the slug, and an unknown one also gets the lock's path and goos's way of finding its holder.
// Every way forward ends with running retry again.
func runLockRefusal(holder int, holderKnown bool, slug, lockPath, goos, retry string, paused, hubEntry, parentLocked bool) error {
	holderText := "an unknown process"
	if holderKnown {
		holderText = fmt.Sprintf("pid %d", holder)
	}
	base := fmt.Errorf("%w: the run lock %s of pair %q is held by %s", ErrDriverBusy, lockPath, slug, holderText)

	if paused {
		resume := fmt.Sprintf("`lyx tender resume %s`", slug)
		if hubEntry {
			resume = "`lyx tender resume` with no slug, which lifts every live entry of the hub, other pairs' pauses and a pause held for another machine included,"
		}
		return fmt.Errorf("%w, and a parent lock record names the pair; resume the run with %s so the step finishes its merge, then run %s again;"+
			" under a pause held for another machine the run is not resumed: run %s again once the holder ends, and never lift that pause",
			base, resume, retry, retry)
	}

	pauseStep := ""
	if !parentLocked {
		pauseStep = fmt.Sprintf("pause the run with `lyx tender pause %s`, then run %s again; ", slug, retry)
	}
	if holderKnown {
		return fmt.Errorf("%w; %srun %s again once that pid ends", base, pauseStep, retry)
	}
	find := "find its holder in the replaced-binary part of `lyx tender status` or with `ps` and end it"
	if goos == "windows" {
		find = "end the pair's `lyx loom drive` process in Task Manager"
	}
	return fmt.Errorf("%w; %s%s, then run %s again", base, pauseStep, find, retry)
}
