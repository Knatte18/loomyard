// stop.go is shuttle's stop verb: it ends a run's strand after recording the stop in run.json.
// It has two forms, the handle form (Run.Stop) and the guid form (Runner.StopStrand).

package shuttleengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// runOutcomeStopped is the RunState.Outcome of a run lym stopped before it finished.
// It is a terminal value that exists only in the run record, not in a reported result.
const runOutcomeStopped = "stopped"

// stopOutcome returns the outcome a stop leaves in a run's record:
// outcome itself when it is done or every file in a non-empty outputFiles exists, runOutcomeStopped otherwise.
// A finished run's files outrank the stop, so a stop never turns finished work into a stopped record.
// An empty outputFiles is an unawaited run's, which has no files to finish, so only its outcome can keep it.
func stopOutcome(outcome string, outputFiles []string) string {
	if outcome == string(OutcomeDone) || allOutputFilesExist(outputFiles) {
		return outcome
	}
	return runOutcomeStopped
}

// saveState persists run.state to run.json.
// The caller holds run.recordMu, which guards the stop mark, run.state.Outcome and every save of run.state.
func (run *Run) saveState() error {
	return saveRunState(run.runDir, run.state)
}

// Stop removes the run's strand, when it is removable, and records the stop.
// A record already terminal keeps its outcome.
// A record still reading running keeps running when all its output files exist.
// Any other record, an unawaited run's included, reads stopped.
// The record is read, changed and written in one hold of its lock,
// so a stop racing another writer of the record loses neither change.
// The handle's copy of the record follows what the hold read and wrote.
// The record is written before the strand is removed, so a failed write returns its error with the strand untouched,
// and a failed removal leaves the terminal record over a live strand.
// A Wait on this handle that sees the strand go finalizes with the stop mark,
// so it records stopped unless the run's files show it done.
func (run *Run) Stop() error {
	run.recordMu.Lock()
	previousState, previousMark := run.state, run.stopMarked
	err := updateRunState(run.runDir, func(rs RunState, found bool) (RunState, error) {
		if !found {
			rs = run.state
		}
		run.state = rs
		if isTerminalOutcome(rs.Outcome) {
			return rs, errLeaveRunRecord
		}
		run.stopMarked = true
		rs.Outcome = stopOutcome(runOutcomeRunning, run.spec.OutputFiles)
		run.state = rs
		return rs, nil
	})
	if err != nil {
		run.state, run.stopMarked = previousState, previousMark
		run.recordMu.Unlock()
		return fmt.Errorf("shuttle: stop strand %s: record the stop in %s: %w", run.state.StrandGUID, run.runDir, err)
	}
	run.recordMu.Unlock()
	return removeStrandIfPresent(run.runner.reed, run.state.StrandGUID)
}

// updateRelaunchRecord changes the run's record through change, in one hold of its lock.
// The handle's copy of the record follows what the hold read and wrote.
// A record a stop made terminal is left as it is, and stopped reports it.
// A failed read or write returns its error with the record and the handle's copy as they were.
func (run *Run) updateRelaunchRecord(change func(rs *RunState)) (stopped bool, err error) {
	run.recordMu.Lock()
	defer run.recordMu.Unlock()
	previous := run.state
	err = updateRunState(run.runDir, func(rs RunState, found bool) (RunState, error) {
		if !found {
			rs = run.state
		}
		run.state = rs
		if isTerminalOutcome(rs.Outcome) {
			stopped = true
			return rs, errLeaveRunRecord
		}
		change(&rs)
		run.state = rs
		return rs, nil
	})
	if err != nil {
		run.state = previous
	}
	return stopped, err
}

// stopForRelaunch is the stop verb's relaunch mode: it ends the run's strand process and keeps the strand and its GUID.
// In one hold of the record it first marks the record relaunching, so every reader of a running run reads it as live.
// It records no outcome, so a Wait on the handle keeps its run.
// It reports true, having ended nothing, when a stop already made the record terminal.
// When ending the process fails while the strand still reads live, it ends the strand's hold and clears the mark.
// It returns the failure, joined with the hold's own failure when ending the hold failed too.
// The caller then leaves the run as it was.
// When ending it fails over a strand no longer live, the process is gone and the relaunch may go on.
func (run *Run) stopForRelaunch() (bool, error) {
	markedAt := run.clock.Now().UTC().Format(time.RFC3339)
	guid := run.state.StrandGUID
	stopped, err := run.updateRelaunchRecord(func(rs *RunState) { rs.RelaunchingAt = markedAt })
	if err != nil {
		return false, fmt.Errorf("shuttle: relaunch strand %s: mark the record relaunching in %s: %w", guid, run.runDir, err)
	}
	if stopped {
		return true, nil
	}
	reed := run.runner.reed
	endErr := reed.EndStrandProcess(guid)
	if endErr == nil {
		return false, nil
	}
	if status, statusErr := reed.Status(); statusErr == nil {
		if strand, tracked := strandStatusByGUID(status.Strands, guid); !tracked || !strand.Live {
			logger.Warn("shuttle: ending the strand's process failed over a strand that is no longer live; relaunching it", "strandGUID", guid, "error", endErr)
			return false, nil
		}
	}
	cause := endErr
	if releaseErr := reed.ReleaseStrandHold(guid); releaseErr != nil {
		cause = fmt.Errorf("%w; end its hold: %w", endErr, releaseErr)
	}
	if _, clearErr := run.updateRelaunchRecord(func(rs *RunState) { rs.RelaunchingAt = "" }); clearErr != nil {
		logger.Warn("shuttle: clear the relaunching mark after a failed relaunch stop failed", "strandGUID", guid, "error", clearErr)
	}
	return false, fmt.Errorf("shuttle: relaunch strand %s: end its process: %w", guid, cause)
}

// markedStopped reports whether Stop recorded a stop on this handle.
func (run *Run) markedStopped() bool {
	run.recordMu.Lock()
	defer run.recordMu.Unlock()
	return run.stopMarked
}

// StopStrand records the stop and removes the strand guid names.
// It removes the strand only when it is removable: live, held for a relaunch, or an own-window strand with a dead pane.
// It needs only the guid, not a handle from the process that started the run.
// A guid with no run record is stopped by removing its strand alone.
// A Wait in another process may overwrite the stopped record with its own terminal outcome.
// Either answer is terminal.
func (r *Runner) StopStrand(guid string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	_, runDir, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return removeStrandIfPresent(r.reed, guid)
	}
	return r.stopRecorded(runDir, guid)
}

// stopRecorded writes the stop into the record at runDir, then removes the strand guid names.
// It removes the strand only when it is removable: live, held for a relaunch, or an own-window strand with a dead pane.
// The record is read and written in one hold of its lock,
// so a stop racing another writer of the record loses neither change.
// A record already terminal keeps its outcome, and one still reading running whose output files all exist keeps running.
// A failed read or write returns its error with the strand untouched.
func (r *Runner) stopRecorded(runDir, guid string) error {
	var stopped RunState
	recorded := false
	err := updateRunState(runDir, func(rs RunState, found bool) (RunState, error) {
		if !found || isTerminalOutcome(rs.Outcome) || stopOutcome(runOutcomeRunning, rs.OutputFiles) != runOutcomeStopped {
			return rs, errLeaveRunRecord
		}
		rs.Outcome = runOutcomeStopped
		stopped, recorded = rs, true
		return rs, nil
	})
	if err != nil {
		return fmt.Errorf("shuttle: stop strand %s: record the stop in %s: %w", guid, runDir, err)
	}
	if recorded {
		r.reportStoppedSession(stopped)
	}
	return removeStrandIfPresent(r.reed, guid)
}

// reportStoppedSession reports the session of rs, a record the stop verb moved to stopped, to the session-end hook.
// The usage is what the record's engine reads for the session.
// It is the unknown reading when the set no longer holds the engine, or the engine has no UsageReader.
func (r *Runner) reportStoppedSession(rs RunState) {
	engine, engineName, err := r.engineFor(rs)
	usage := SessionUsage{}
	if err != nil {
		engineName = rs.Engine
	} else if reader, ok := engine.(UsageReader); ok {
		usage = reader.SessionUsage(rs.SessionID, r.paneCwd)
	}
	var startedAt time.Time
	if createdAt, err := time.Parse(time.RFC3339, rs.CreatedAt); err == nil {
		startedAt = createdAt
	}
	r.reportSessionEnd(SessionEnd{
		SessionID: rs.SessionID,
		Engine:    engineName,
		Role:      rs.Role,
		Model:     rs.Model,
		Worktree:  r.paneCwd,
		StartedAt: startedAt,
		EndedAt:   r.clock.Now(),
		Outcome:   Outcome(runOutcomeStopped),
		Usage:     usage,
	})
}

// strandRemovable reports whether a stop removes strand:
// a live strand, a strand held for a relaunch whatever its pane state,
// or an own-window strand whose pane is present but not live, a dead pane or one showing only its shell.
// A window 0 strand with a dead pane is not removable, since reconcile kills its pane at the next reed op.
func strandRemovable(strand reedengine.StrandStatus) bool {
	return strand.Live || strand.Held || (strand.OwnWindow && strand.Present)
}

// removeStrandIfPresent removes guid's strand when reed tracks it and it is removable.
// It logs the removal, because a removal may kill a real agent process.
// A probe that could not answer has not shown the strand gone, so it returns an error rather than guessing.
func removeStrandIfPresent(reed ReedOps, guid string) error {
	status, err := reed.Status()
	if err != nil {
		return fmt.Errorf("shuttle: stop strand %s: probe reed before removing it: %w", guid, err)
	}
	strand, tracked := strandStatusByGUID(status.Strands, guid)
	if !tracked || !strandRemovable(strand) {
		return nil
	}
	logger.Warn("shuttle: stopping a strand", "strandGUID", guid, "live", strand.Live)
	if _, err := reed.RemoveStrand(guid, false); err != nil {
		return fmt.Errorf("shuttle: stop strand %s: remove it: %w", guid, err)
	}
	return nil
}
