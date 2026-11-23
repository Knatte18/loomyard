// replyhold.go holds a run's finishing turn end while its agent awaits the reply to a question it asked.
// The runner's reply probe answers whether the run's strand awaits one.
// The reply starts the agent's next turn, and that turn's end decides afresh.

package shuttleengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// replyQuietProbes is how many probes in a row, gateRecheckEvery apart, must find no reply awaited before a hold may end.
// A reply read between two probes starts the agent's turn, which ends the hold first when it writes a turn start.
// A turn the reply wakes may write none, so the release also needs the agent's pane idle.
const replyQuietProbes = 2

// replyHold is one turn end held while the run's strand awaits a reply.
type replyHold struct {
	// probedAt is the run clock's time of the latest probe.
	probedAt time.Time
	// quietProbes counts the latest probes in a row that found no reply awaited.
	quietProbes int
}

// awaitsReply asks the runner's reply probe whether the run's strand awaits a reply.
// A runner told no probe and a run whose record names no strand answer false.
// A probe error answers false too, so the run ends as it would with no probe; it warns once per episode.
func (run *Run) awaitsReply() bool {
	probe, strand := run.runner.replyProbe, run.state.StrandName
	if probe == nil || strand == "" {
		return false
	}
	awaits, err := probe(strand)
	if err != nil {
		run.failures.Fail("reply-probe", "shuttle: reply probe failed, reading the strand as awaiting no reply", "strandGUID", run.state.StrandGUID, "strand", strand, "error", err)
		return false
	}
	run.failures.Recover("reply-probe", "shuttle: reply probe answers again", "strandGUID", run.state.StrandGUID, "strand", strand)
	return awaits
}

// holdForReply reports whether a turn end that would end the run, or start its gate's evaluation, is held for a reply.
// It starts the hold when the run's strand awaits one.
// The hold sends no notice, since the parent already holds the question, and it never extends the run deadline.
func (run *Run) holdForReply() bool {
	if !run.awaitsReply() {
		return false
	}
	run.replyHold = &replyHold{probedAt: run.clock.Now()}
	logger.Info("shuttle: turn end held, the agent awaits a reply", "strandGUID", run.state.StrandGUID, "strand", run.state.StrandName)
	return true
}

// agentBusy reports whether the run's pane shows the agent mid-turn.
// An engine that cannot read idleness and a pane that cannot be captured read as not busy,
// so the hold never depends on a reading it cannot take.
func (run *Run) agentBusy() bool {
	cycler, ok := run.engine.(SessionCycler)
	if !ok {
		return false
	}
	capture, err := run.runner.reed.CapturePane(run.state.StrandGUID)
	if err != nil {
		return false
	}
	return run.engine.Startup(capture) != StartupReady || !cycler.IdleSession(capture)
}

// releaseReplyHold ends the held turn end once replyQuietProbes probes in a row find no reply awaited.
// The pane must also read idle.
// A busy pane keeps the hold, and the next probe interval reads it again.
// It reports whether Wait finishes with the returned Result and error.
// An ungated run then finalizes done.
// A gated one evaluates its gate at the boundary, as at the turn end it held;
// under a pause the boundary waits for the first uncovered tick.
func (run *Run) releaseReplyHold() (Result, bool, error) {
	hold := run.replyHold
	now := run.clock.Now()
	if now.Sub(hold.probedAt) < gateRecheckEvery {
		return Result{}, false, nil
	}
	hold.probedAt = now
	if run.awaitsReply() {
		hold.quietProbes = 0
		return Result{}, false, nil
	}
	hold.quietProbes++
	if hold.quietProbes < replyQuietProbes || run.agentBusy() {
		return Result{}, false, nil
	}
	run.replyHold = nil
	logger.Info("shuttle: turn end released, no reply awaited", "strandGUID", run.state.StrandGUID, "strand", run.state.StrandName)
	if len(run.gate) == 0 {
		result, err := run.finalizeAndRemoveDiedWindow(OutcomeDone)
		return result, true, err
	}
	if run.covered {
		run.boundaryHeld = true
		return Result{}, false, nil
	}
	if run.gateAtBoundary && run.boundaryIdle() {
		return run.handleGatedBoundary()
	}
	return Result{}, false, nil
}
