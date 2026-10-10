// stop.go is shuttle's stop verb: it ends a run's strand after recording the stop in run.json, in both the handle form (Run.Stop) and the guid form (Runner.StopStrand).

package shuttleengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// runOutcomeStopped is the RunState.Outcome of a run lyx stopped before it finished.
// It is a record-only terminal value, never a Result.Outcome.
const runOutcomeStopped = "stopped"

// stopOutcome returns the outcome a stop leaves in a run's record: outcome itself when it is done or every file in outputFiles exists, runOutcomeStopped otherwise.
// A finished run's files outrank the stop, so a stop never turns finished work into a stopped record.
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

// Stop ends the run's strand and records the stop.
// A record already terminal keeps its outcome, and a record still reading running whose output files all exist keeps running; any other record reads stopped.
// The record is written before the strand is removed, so a failed write returns its error with the strand untouched, and a failed removal leaves the terminal record over a live strand.
// A Wait on this handle that sees the strand go finalizes with the stop mark, so it records stopped unless the run's files show it done.
func (run *Run) Stop() error {
	run.recordMu.Lock()
	if !isTerminalOutcome(run.state.Outcome) {
		previousOutcome, previousMark := run.state.Outcome, run.stopMarked
		run.stopMarked = true
		run.state.Outcome = stopOutcome(runOutcomeRunning, run.spec.OutputFiles)
		if err := run.saveState(); err != nil {
			run.state.Outcome, run.stopMarked = previousOutcome, previousMark
			run.recordMu.Unlock()
			return fmt.Errorf("shuttle: stop strand %s: record the stop in %s: %w", run.state.StrandGUID, run.runDir, err)
		}
	}
	run.recordMu.Unlock()
	return removeStrandIfLive(run.runner.reed, run.state.StrandGUID)
}

// StopStrand ends the strand guid names and records the stop, without needing an in-process Run handle.
// A guid with no run record is stopped by removing its strand alone.
// A Wait in another process may overwrite the stopped record with its own terminal outcome, and either answer is terminal.
func (r *Runner) StopStrand(guid string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	_, runDir, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return removeStrandIfLive(r.reed, guid)
	}
	return r.stopRecorded(runDir, guid)
}

// stopRecorded writes the stop into the record at runDir, then removes the strand guid names.
// A record already terminal keeps its outcome, and one still reading running whose output files all exist keeps running.
// A failed read or write returns its error with the strand untouched.
func (r *Runner) stopRecorded(runDir, guid string) error {
	rs, found, err := loadRunState(runDir)
	if err != nil {
		return fmt.Errorf("shuttle: stop strand %s: read the run record in %s: %w", guid, runDir, err)
	}
	if found && !isTerminalOutcome(rs.Outcome) {
		if stopOutcome(runOutcomeRunning, rs.OutputFiles) == runOutcomeStopped {
			rs.Outcome = runOutcomeStopped
			if err := saveRunState(runDir, rs); err != nil {
				return fmt.Errorf("shuttle: stop strand %s: record the stop in %s: %w", guid, runDir, err)
			}
		}
	}
	return removeStrandIfLive(r.reed, guid)
}

// removeStrandIfLive removes guid's strand when reed reports it live, logging the removal because it kills a real agent process.
// A probe that could not answer has not shown the strand dead, so it returns an error rather than guessing.
func removeStrandIfLive(reed ReedOps, guid string) error {
	status, err := reed.Status()
	if err != nil {
		return fmt.Errorf("shuttle: stop strand %s: probe reed before removing it: %w", guid, err)
	}
	strand, tracked := strandStatusByGUID(status.Strands, guid)
	if !tracked || !strand.Live {
		return nil
	}
	logger.Warn("shuttle: stopping a live strand", "strandGUID", guid)
	if _, err := reed.RemoveStrand(guid, false); err != nil {
		return fmt.Errorf("shuttle: stop strand %s: remove it: %w", guid, err)
	}
	return nil
}
