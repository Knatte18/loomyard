// relaunch.go resumes a gated run's session in a fresh strand.
// It applies when the strand a re-prompt was meant for can take no keys.

package shuttleengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// relaunchForReprompt resumes the run's session in a fresh strand, with text as the resumed session's first turn.
// It exists for a gated run whose writer must be re-prompted while its strand can take no keys:
// reed lost the strand, holds no pane for it, or its pane died.
// The session is resumed only when the engine can resume a session and name the process holding one,
// and no live process holds the session, so a writer still working in a pane reed lost is never run twice.
// A resume check that cannot rule out a live holder refuses the resume.
// The run keeps its run directory, its events file and its gate state.
// The engine's Prepare replaces the run directory's prompt.md with text.
// Only the strand changes.
// The record names the new strand before the old strand is removed and before the new provider's startup is awaited.
// That startup reads the events file only past its size at the resume.
// So the first session's lines never mark it started.
// It returns a non-nil err, with no strand added, when the session cannot be resumed safely.
// It does so too when the launch fails before the strand registers.
// When the new provider never becomes ready, the startup teardown finalizes the run:
// finished is then true, with the teardown's result and error.
func (run *Run) relaunchForReprompt(text string) (result Result, finished bool, err error) {
	if run.state.SessionID == "" {
		return Result{}, false, errors.New("the run records no session to resume")
	}
	finder, ok := run.engine.(SessionHolderFinder)
	if !ok {
		return Result{}, false, fmt.Errorf("engine %q cannot name the process holding a session, so a resume could run the session twice", run.engineName)
	}
	holder, held, err := finder.SessionHolder(run.state.SessionID)
	if err != nil {
		return Result{}, false, fmt.Errorf("look for a process holding session %q: %w", run.state.SessionID, err)
	}
	if held {
		return Result{}, false, fmt.Errorf("process %d still holds session %q in a pane reed cannot address", holder, run.state.SessionID)
	}

	runner := run.runner
	spec := run.spec
	spec.ResumeSessionID = run.state.SessionID
	spec.Prompt = text
	// The resumed session keeps the skills it loaded, so its first turn is the re-prompt itself.
	// The skills still shape the settings and passages the resumed session runs under.
	spec.SkipSkillLoadTurn = true
	warning, err := runner.checkResume(spec, run.engine)
	if err != nil {
		return Result{}, false, err
	}
	if warning != "" {
		return Result{}, false, fmt.Errorf("a live process holding session %q could not be ruled out: %s", run.state.SessionID, warning)
	}
	startupEventsOffset := eventsSize(run.state.EventsPath)
	launch, err := run.engine.Prepare(run.runDir, spec, runner.cfg)
	if err != nil {
		return Result{}, false, fmt.Errorf("prepare the resumed session: %w", err)
	}
	// A gated run's outputs existing does not mean the gate passed, so the strand carries no done-when list.
	strand, err := runner.reed.AddStrand(strandAddSpec(spec, run.engineName, launch, nil))
	if err != nil {
		return Result{}, false, fmt.Errorf("add the resumed session's strand: %w", err)
	}
	goneStrandGUID := run.state.StrandGUID
	logger.Warn("shuttle: gate: the re-prompt's strand was gone; resumed the session in a fresh strand", "runDir", run.runDir, "oldStrandGUID", goneStrandGUID, "strandGUID", strand.GUID, "sessionID", launch.SessionID)

	run.recordMu.Lock()
	previous := run.state
	run.state.StrandGUID, run.state.StrandName = strand.GUID, strand.Name
	run.state.SessionID = launch.SessionID
	run.state.Started = false
	run.state.PromptLine = ""
	run.state.RecordsSessionStarts = launch.RecordsSessionStarts
	saveErr := run.saveState()
	if saveErr != nil {
		run.state = previous
	}
	run.recordMu.Unlock()
	if saveErr != nil {
		// No record names the new strand, so nothing could find or stop it later.
		if _, rerr := runner.reed.RemoveStrand(strand.GUID, false); rerr != nil {
			logger.Warn("shuttle: gate: remove the resumed session's strand after a failed record save (non-fatal)", "strandGUID", strand.GUID, "error", rerr)
		}
		return Result{}, false, fmt.Errorf("record the resumed session's strand in %s: %w", run.runDir, saveErr)
	}

	// The run directory's record stands when the copy cannot be kept, so a failure here never fails the resume.
	if err := copyManifest(runDirRoot(runner.cfg, runner.anchorPath), run.state.RunID, run.runDir); err != nil {
		logger.Warn("shuttle: gate: keep the resumed launch's manifest copy failed (non-fatal)", "runDir", run.runDir, "error", err)
	}
	// The record names the new strand now, so removing the old one leaves no running record naming a removed strand.
	// A failed removal leaves a strand reed reconciles later, and never fails the resume.
	if err := removeStrandIfPresent(runner.reed, goneStrandGUID); err != nil {
		logger.Warn("shuttle: gate: remove the replaced strand after the resume (non-fatal)", "strandGUID", goneStrandGUID, "error", err)
	}

	run.startupEventsOffset = startupEventsOffset
	if result, err := run.awaitStartup(); err != nil {
		return result, true, err
	}
	return Result{}, false, nil
}
