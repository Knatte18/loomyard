// accountrelaunch.go relaunches a run's session under another account after a usage limit, inside the run.
// The run keeps its run directory, its strand and its GUID; only the provider process behind the strand changes.

package shuttleengine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// accountSwitchContinueLine is the first turn of a session relaunched under another account after a usage limit.
const accountSwitchContinueLine = "[lym] Your Claude account was switched after a usage limit. Carry on with your task where you stopped."

// relaunchLaunch selects the launch a relaunch gives the session.
type relaunchLaunch int

const (
	// relaunchContinue resumes the run's session, with the continue line as its first turn.
	relaunchContinue relaunchLaunch = iota
	// relaunchFresh starts a new session with the run's own skills and prompt.
	relaunchFresh
	// relaunchRepeat launches the run's own spec again unchanged.
	relaunchRepeat
)

// errRelaunchDeclined marks a relaunch that never began, so the caller handles the usage limit as it would without one.
var errRelaunchDeclined = errors.New("shuttle: the relaunch did not begin")

// switchAtLoadLimit gives a usage limit that ended a load turn to the engine's account switch.
// raw is the turn end's events-file line.
// limited reports that the engine reads the turn end as a usage limit and so asked the switch.
// switched reports that the switch moved the worktree to another account.
// An engine without the switch or the activity reading leaves the turn end to the load hold.
// So does a turn end that is no usage limit.
// So does a failed switch, which is logged at Warn.
func (run *Run) switchAtLoadLimit(raw []byte) (limited, switched bool) {
	switcher, canSwitch := run.engine.(AccountSwitcher)
	reader, canRead := run.engine.(ActivityReader)
	if !canSwitch || !canRead {
		return false, false
	}
	turnEnd := Event{Raw: raw}
	if !reader.TurnEndActivity(turnEnd).UsageLimit.Limited {
		return false, false
	}
	relaunch, err := switcher.SwitchAccount(run.runDir, turnEnd)
	if err != nil {
		logger.Warn("shuttle: the account switch at a usage limit failed; the load hold counts the turn end", "strandGUID", run.state.StrandGUID, "error", err)
		return true, false
	}
	return true, relaunch
}

// settleStaleRelaunch settles a relaunch whose waiter died, once its relaunching mark is past its bound.
// It re-reads the mark from the record first, so a relaunch another process finished or cleared is seen.
// It does nothing for a record with no mark.
// It does nothing for a mark within its bound, which a live relaunch or a Wait beside it keeps.
// Past the bound, over a strand that is not live, held or not, it ends the run as a strand that died.
// A run whose output files all exist ends done, through classifyDeadlineExpiry.
// It removes the strand in both cases.
// Over a live strand it clears the mark and takes the record as it stands, which already names the new launch.
// Wait then goes on as an attach, from the record's prompt offset, and types a prompt line the dead waiter never sent.
// A failed read of reed's status or of the record leaves the mark for a later tick.
// It returns the Result and true when it ended the run.
func (run *Run) settleStaleRelaunch() (Result, bool, error) {
	run.recordMu.Lock()
	marked := run.state.RelaunchingAt != ""
	run.recordMu.Unlock()
	if !marked {
		return Result{}, false, nil
	}
	record, found, err := loadRunState(run.runDir)
	if err != nil || !found {
		return Result{}, false, nil
	}
	run.recordMu.Lock()
	run.state.RelaunchingAt = record.RelaunchingAt
	_, past := run.state.relaunching(run.clock.Now(), relaunchBound(run.runner.cfg))
	run.recordMu.Unlock()
	if !past {
		return Result{}, false, nil
	}

	status, err := run.runner.reed.Status()
	if err != nil {
		return Result{}, false, nil
	}
	strand, tracked := strandStatusByGUID(status.Strands, run.state.StrandGUID)
	if !tracked || !strand.Live {
		logger.Warn("shuttle: a relaunch's waiter died and its bound passed over a strand that is not live; the run ends as a strand that died", "runDir", run.runDir, "strandGUID", run.state.StrandGUID)
		result, ferr := run.finalize(run.classifyDeadlineExpiry(OutcomeDied))
		if removeErr := removeStrandIfPresent(run.runner.reed, run.state.StrandGUID); removeErr != nil {
			logger.Warn("shuttle: remove the strand of a stale relaunch failed", "strandGUID", run.state.StrandGUID, "error", removeErr)
		}
		return result, true, ferr
	}

	previousOffset := run.state.PromptOffset
	stopped, err := run.updateRelaunchRecord(func(rs *RunState) { rs.RelaunchingAt = "" })
	if err != nil {
		logger.Warn("shuttle: clear a stale relaunching mark failed; it is read again at the next tick", "runDir", run.runDir, "error", err)
		return Result{}, false, nil
	}
	if stopped {
		return Result{}, false, nil
	}
	logger.Warn("shuttle: a relaunch's waiter died and its bound passed over a live strand; waiting on it as an attach", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID)
	if run.state.PromptOffset != previousOffset {
		run.reseedAfterRelaunch()
	}
	return Result{}, false, nil
}

// relaunchWithinBound reports whether the run's record carries a relaunching mark younger than its bound.
func (run *Run) relaunchWithinBound() bool {
	run.recordMu.Lock()
	defer run.recordMu.Unlock()
	within, _ := run.state.relaunching(run.clock.Now(), relaunchBound(run.runner.cfg))
	return within
}

// switchAccountAtLimit gives the usage limit that ended a held turn to the engine's account switch.
// handled is true when the switch moved the worktree to another account and the session was relaunched under it.
// A turn end handled so is neither counted nor re-sent as an API error.
// finished is true when the relaunch ended the run, and the Result and error are Wait's own.
// An engine without the switch, a turn end that is no usage limit and a run a pause covers all report neither.
// So do a failed switch and a relaunch that never began, and the turn end is handled as it is without a switch.
func (run *Run) switchAccountAtLimit(held *heldTurnEnd) (result Result, finished, handled bool, err error) {
	switcher, ok := run.engine.(AccountSwitcher)
	if !ok || run.covered || !run.usageLimited(held) {
		return Result{}, false, false, nil
	}
	relaunch, switchErr := switcher.SwitchAccount(run.runDir, Event{Raw: held.raw})
	if switchErr != nil {
		logger.Warn("shuttle: the account switch at a usage limit failed; the limit is handled as it is without a switch", "strandGUID", run.state.StrandGUID, "error", switchErr)
		return Result{}, false, false, nil
	}
	if !relaunch {
		return Result{}, false, false, nil
	}
	previous := run.apiErrors.handledOffset
	run.apiErrors.handledOffset = max(previous, held.offset)
	result, finished, err = run.relaunchUnderAccount(relaunchContinue, false)
	if errors.Is(err, errRelaunchDeclined) {
		logger.Warn("shuttle: the relaunch under the switched account did not begin; the limit is handled as it is without a switch", "strandGUID", run.state.StrandGUID, "error", err)
		run.apiErrors.handledOffset = previous
		return Result{}, false, false, nil
	}
	return result, finished, true, err
}

// reseedAfterRelaunch points Wait's readers at the relaunched session.
// Each starts at the record's prompt offset, which is the events file's size at the relaunch.
// So nothing of the limited session is read again.
// The pending hold, the held-wait mark and the API-error count of the limited session are dropped.
func (run *Run) reseedAfterRelaunch() {
	run.recordMu.Lock()
	offset, dialogNotified := run.state.PromptOffset, run.state.DialogNotifiedOffset
	run.recordMu.Unlock()
	run.offset = offset
	run.shadow.cursor = offset
	run.dialog = dialogLatch{cursor: max(offset, dialogNotified)}
	run.apiErrors = apiErrorWatch{cursor: offset, handledOffset: run.apiErrors.handledOffset}
	run.waitingTasks, run.waitingTurnEnd = nil, nil
	run.pendingHold = nil
	run.endHeldWait()
	run.armHoldGuard()
	run.turnSince = run.clock.Now()
}

// startFreshAfterRefusedResume starts the session of a refused resume fresh, in the same run, on the Wait path.
// handled is true when it relaunched or ended the run; the turn end is then neither counted nor re-sent.
// finished is true when the relaunch ended the run, and the Result and error are Wait's own.
// After a relaunch whose pane starts, Wait's readers are pointed at the new session.
// A relaunch that never began reports it unhandled, so the turn end is counted as any other API error.
func (run *Run) startFreshAfterRefusedResume() (result Result, finished, handled bool, err error) {
	result, finished, err = run.relaunchUnderAccount(relaunchFresh, false)
	if errors.Is(err, errRelaunchDeclined) {
		logger.Warn("shuttle: the fresh start of a refused resume did not begin; the API error is handled as any other", "strandGUID", run.state.StrandGUID, "error", err)
		return Result{}, false, false, nil
	}
	if err == nil && !finished {
		run.reseedAfterRelaunch()
	}
	return result, finished, true, err
}

// relaunchAfterLoadHold relaunches the launch of a start whose skill-load hold ended uncounted.
// launch is relaunchRepeat for a usage limit whose account was switched, and relaunchFresh for a refused resume.
// It returns as loadSkillsThenPrompt does:
// a relaunch whose pane starts, or whose output files all exist, returns Result{} and nil,
// and a relaunch that ends the run returns the Result beside an error wrapping ErrNotStarted.
// A relaunch that never began ends the start as a provider that died would, so the start's own retry judges it.
func (run *Run) relaunchAfterLoadHold(launch relaunchLaunch) (Result, error) {
	result, _, err := run.relaunchUnderAccount(launch, true)
	if errors.Is(err, errRelaunchDeclined) {
		result, _, err = run.endFailedRelaunch(err, true)
	}
	return result, err
}

// relaunchUnderAccount relaunches the run's session in its strand, under the account the worktree now resolves to.
// launch selects the launch the session gets, and startPath says the caller is a start that holds no handle yet.
//
// It marks the record relaunching and ends the strand's process, keeping the strand, through stopForRelaunch.
// It builds the spec for launch and, for a spec that resumes a session, carries and checks the transcript.
// A check refused with ErrNoTranscript starts the session fresh instead.
// Any other refusal and any failure up to the strand's startup ends the run as a strand that died.
// It then records the new session, its deferred prompt line and the cleared started flag.
// It records the events file's size as the prompt and launch offsets, and whether the transcript was copied.
// All of that happens before reed relaunches the strand.
// The startup wait decides on the pane capture alone, since the events file still holds the limited session's lines.
// Once the strand started it clears the mark and types a deferred prompt line the way a launch does.
//
// It returns (Result{}, false, nil) once the session runs, and also, on the start path, when every output file exists.
// It returns the Result and true when it ended the run.
// On the Wait path the error is nil; on the start path it wraps ErrNotStarted.
// It returns an error wrapping errRelaunchDeclined when nothing was relaunched.
// A relaunch a stop ended removes the strand and returns as endStoppedRelaunch states.
func (run *Run) relaunchUnderAccount(launch relaunchLaunch, startPath bool) (Result, bool, error) {
	stopped, err := run.stopForRelaunch()
	if err != nil {
		return Result{}, false, fmt.Errorf("%w: %w", errRelaunchDeclined, err)
	}
	if stopped {
		return run.endStoppedRelaunch(startPath)
	}

	spec, copied, err := run.relaunchSpec(launch)
	if errors.Is(err, ErrNoTranscript) && launch != relaunchFresh {
		logger.Warn("shuttle: the session's transcript is in no known directory; relaunching it fresh", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "error", err)
		spec, copied, err = run.relaunchSpec(relaunchFresh)
	}
	if err != nil {
		return run.endFailedRelaunch(err, startPath)
	}
	launched, err := run.engine.Prepare(run.runDir, spec, run.runner.cfg)
	if err != nil {
		return run.endFailedRelaunch(fmt.Errorf("prepare the relaunched session: %w", err), startPath)
	}

	eventsOffset := eventsSize(run.state.EventsPath)
	stopped, err = run.updateRelaunchRecord(func(rs *RunState) {
		rs.SessionID = launched.SessionID
		rs.PromptLine = launched.PromptLine
		rs.Started = false
		rs.RecordsSessionStarts = launched.RecordsSessionStarts
		rs.TranscriptCopied = copied
		rs.PromptOffset, rs.LaunchOffset = eventsOffset, eventsOffset
	})
	if err != nil {
		return run.endFailedRelaunch(fmt.Errorf("record the relaunched session in %s: %w", run.runDir, err), startPath)
	}
	if stopped {
		return run.endStoppedRelaunch(startPath)
	}
	// Nothing of the limited session is read again, by a Wait or by a load hold.
	run.offset = eventsOffset
	run.loadTurnEnds = 0
	if err := copyManifest(runDirRoot(run.runner.cfg, run.runner.anchorPath), run.state.RunID, run.runDir); err != nil {
		logger.Warn("shuttle: keep the relaunched launch's manifest copy failed (non-fatal)", "runDir", run.runDir, "error", err)
	}
	reedSpec := reedengine.RelaunchSpec{Cmd: launched.Cmd, ResumeCmd: launched.ResumeCmd, SessionID: launched.SessionID}
	if err := run.runner.reed.RelaunchStrand(run.state.StrandGUID, reedSpec); err != nil {
		return run.endFailedRelaunch(fmt.Errorf("relaunch the strand: %w", err), startPath)
	}
	logger.Info("shuttle: relaunched the session under the switched account", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "sessionID", launched.SessionID)

	run.startupEventsOffset = eventsOffset
	run.dismissed, run.dismissedAt = false, time.Time{}
	if result, err := run.awaitStartup(false); err != nil {
		if startPath {
			return result, true, err
		}
		logger.Warn("shuttle: the relaunched session never became ready; the run ends", "strandGUID", run.state.StrandGUID, "outcome", string(result.Outcome), "error", err)
		return result, true, nil
	}

	stopped, err = run.updateRelaunchRecord(func(rs *RunState) { rs.RelaunchingAt = "" })
	if err != nil {
		logger.Warn("shuttle: clear the relaunching mark failed; it ends at its bound", "strandGUID", run.state.StrandGUID, "error", err)
	}
	if stopped {
		return run.endStoppedRelaunch(startPath)
	}
	if launched.PromptLine == "" {
		return Result{}, false, nil
	}
	if result, err := run.loadSkillsThenPrompt(launched.PromptLine, ""); err != nil {
		if startPath {
			return result, true, err
		}
		logger.Warn("shuttle: the relaunched session's skills and prompt were not delivered; the run ends", "strandGUID", run.state.StrandGUID, "outcome", string(result.Outcome), "error", err)
		return result, true, nil
	}
	return Result{}, false, nil
}

// relaunchSpec builds the spec a relaunch of kind launch starts from, and runs its resume check.
// It returns the spec and whether the check copied the session's transcript.
// A spec with no session to resume runs no check.
// A check that cannot rule out a process holding the session refuses the relaunch.
func (run *Run) relaunchSpec(launch relaunchLaunch) (Spec, bool, error) {
	spec := run.spec
	switch launch {
	case relaunchContinue:
		if run.state.SessionID == "" {
			return Spec{}, false, errors.New("the run records no session to resume")
		}
		spec.ResumeSessionID = run.state.SessionID
		spec.Prompt = accountSwitchContinueLine
		spec.SkipSkillLoadTurn = true
		spec.TranscriptCopied = false
	case relaunchFresh:
		spec.ResumeSessionID = ""
		spec.SkipSkillLoadTurn = false
		spec.TranscriptCopied = false
	}
	warning, copied, err := run.runner.checkResume(spec, run.engine)
	if err != nil {
		return Spec{}, false, err
	}
	if warning != "" {
		return Spec{}, false, fmt.Errorf("a live process holding session %q could not be ruled out: %s", spec.ResumeSessionID, warning)
	}
	return spec, copied, nil
}

// endFailedRelaunch ends a relaunch whose steps failed with cause, as a strand that died.
// The outcome is classified through classifyDeadlineExpiry, so a run whose output files all exist ends done.
// On the Wait path it finalizes once, removes the strand, and returns the Result beside the finalize's own error.
// On the start path with every output file existing, it clears the mark and returns Result{} and nil.
// That leaves the strand and the finalize to Wait.
// Otherwise it ends the start through abandonStartup, which finalizes once and removes the strand.
func (run *Run) endFailedRelaunch(cause error, startPath bool) (Result, bool, error) {
	logger.Warn("shuttle: the relaunch under the switched account failed; the run ends as a strand that died", "strandGUID", run.state.StrandGUID, "error", cause)
	outcome := run.classifyDeadlineExpiry(OutcomeDied)
	if startPath {
		if outcome == OutcomeDone {
			if stopped, err := run.updateRelaunchRecord(func(rs *RunState) { rs.RelaunchingAt = "" }); err != nil {
				logger.Warn("shuttle: clear the relaunching mark failed; it ends at its bound", "strandGUID", run.state.StrandGUID, "error", err)
			} else if stopped {
				return run.endStoppedRelaunch(startPath)
			}
			return Result{}, false, nil
		}
		result, err := run.abandonStartup(outcome, cause)
		return result, true, err
	}
	result, err := run.finalize(outcome)
	if removeErr := removeStrandIfPresent(run.runner.reed, run.state.StrandGUID); removeErr != nil {
		logger.Warn("shuttle: remove the strand of a run that ended in a relaunch failed", "strandGUID", run.state.StrandGUID, "error", removeErr)
	}
	return result, true, err
}

// endStoppedRelaunch ends a relaunch whose record a stop made terminal.
// It removes the strand, also one the stop failed to remove, and writes nothing to the record.
// A stop the handle made through Run.Stop finalizes the run as a strand that died, once;
// finalize keeps the record stopped, stamps the run end and reports the session end.
// A stop the runner made through StopStrand wrote the record and reported the session end itself.
// So nothing is finalized, and the run's identity is returned with Outcome OutcomeDied.
// The Wait path returns either beside a nil error.
// The start path returns the second beside an error wrapping ErrNotStarted.
// It also leaves startupStrandRemoved false, so the start never launches a stopped run again.
func (run *Run) endStoppedRelaunch(startPath bool) (Result, bool, error) {
	if err := removeStrandIfPresent(run.runner.reed, run.state.StrandGUID); err != nil {
		logger.Warn("shuttle: remove the strand of a relaunch a stop ended failed", "strandGUID", run.state.StrandGUID, "error", err)
	}
	if run.markedStopped() {
		result, err := run.finalize(OutcomeDied)
		return result, true, err
	}
	result := run.identity()
	result.Outcome = OutcomeDied
	if startPath {
		return result, true, fmt.Errorf("shuttle: start: a stop ended the session while it was relaunched, strand %q: %w", run.state.StrandGUID, ErrNotStarted)
	}
	return result, true, nil
}
