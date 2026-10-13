// tackler.go implements the Go driver's call to a cold tackler agent.
// At a stop no rule settles, it spawns the tackler through shuttle, waits for its verdict file and returns the validated envelope.
// The call works over injected seams, so every branch is testable with a fake shuttle and clock; it moves no run itself.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The causes beyond the arbiter's shared ones that a tackler envelope carries when the call, not the agent, made the hand-back.
const (
	tacklerCauseQuestion  = "question"
	tacklerCauseCaseBrief = "case-brief"
)

// errTacklerUnavailable is returned when the tackler is switched off or its spawn budget is spent, so the driver hands back without a spawn.
var errTacklerUnavailable = errors.New("tackler unavailable")

// errTacklerResumeReturned is returned when an attempt's resume was applied and the run stopped at the same stop key again.
var errTacklerResumeReturned = errors.New("tackler resume returned")

// tacklerDeps is every seam and fixed value the call reads.
type tacklerDeps struct {
	// Closed reports whether the stop key's attempt spawned at spawned was already handed to the parent: a hand-back settle record for the key later than spawned.
	Closed func(key loomengine.ArbiterStopKey, spawned time.Time) (bool, error)
	// ReadStatus reads the run's status; found is false when it has no status file.
	ReadStatus func() (status shedengine.Status, found bool, err error)
	// Settings is the resolved tackler role.
	Settings loomengine.TacklerSettings
	// TacklerDir is the durable directory holding every attempt of the run.
	TacklerDir string
	// AttemptDir names the directory of attempt number.
	AttemptDir func(number int) string
	// MarkerPath is the ephemeral in-flight marker's path.
	MarkerPath string
	// Producers is the run's producer list, for goto checks.
	Producers []shedengine.ProducerDef
	// Snapshot reads the worktree state a tackler must leave as it found it.
	Snapshot func() (loomengine.WorktreeSnapshot, error)
	// PausedWithin returns the time the hub's pause record covered the run's slug inside the window from to.
	PausedWithin func(from, to time.Time) (time.Duration, error)
	// Prompt holds the prompt inputs that do not vary per attempt; StencilsDir is where the tackler stencil is read from.
	Prompt      tacklerPromptInput
	StencilsDir string
	// Shuttle starts, attaches to and stops the tackler.
	Shuttle arbiterShuttle
	// Now is the clock.
	Now func() time.Time
}

// tacklerInput is what one call carries about the stop it judges.
type tacklerInput struct {
	// RunID is the run whose stop is judged.
	RunID string
	// EnvelopePath is the stop envelope the driver saved.
	EnvelopePath string
	// ReportPath is the driver's stop report; empty when there is none.
	ReportPath string
	// TraceCopy is the copy of the stopped step's trace.
	TraceCopy string
	// StderrPath is the stopped step's stderr capture.
	StderrPath string
}

// tacklerCall is one call over a status read at its start.
type tacklerCall struct {
	deps   tacklerDeps
	in     tacklerInput
	status shedengine.Status
	key    loomengine.ArbiterStopKey
}

// tacklerRefusal builds an error whose way forward is the hand-back to the run's parent.
func tacklerRefusal(format string, args ...any) error {
	return fmt.Errorf("tackler: "+format+"; %s", append(args, arbiterHandBackWayForward)...)
}

// newTacklerCall reads the status, refusing when it is missing or unreadable.
func newTacklerCall(deps tacklerDeps, in tacklerInput) (*tacklerCall, error) {
	status, found, err := deps.ReadStatus()
	if err != nil {
		return nil, tacklerRefusal("status unreadable: the status file: %v", err)
	}
	if !found {
		return nil, tacklerRefusal("status unreadable: the run has no status file")
	}
	return &tacklerCall{deps: deps, in: in, status: status, key: loomengine.StopKeyOf(status)}, nil
}

// settle returns the validated envelope for the call's stop: an earlier attempt's when one judged this stop and was not yet handed to the parent, else a fresh attempt's.
// It returns errTacklerUnavailable when no attempt exists and none may be spawned, and errTacklerResumeReturned when the stop's attempt resumed the run and the run stopped at the same place.
func (c *tacklerCall) settle() (loomengine.TacklerEnvelope, error) {
	attempts, err := loomengine.ListTacklerAttempts(c.deps.TacklerDir)
	if err != nil {
		return loomengine.TacklerEnvelope{}, err
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].StopKey != c.key {
			continue
		}
		closed, err := c.deps.Closed(c.key, attempts[i].SpawnedAt)
		if err != nil {
			return loomengine.TacklerEnvelope{}, err
		}
		if closed {
			continue
		}
		return c.settleExisting(attempts[i])
	}
	return c.settleFresh(attempts)
}

// settleExisting settles the stop from attempt, the newest open one for it, spawning nothing.
func (c *tacklerCall) settleExisting(attempt loomengine.TacklerAttempt) (loomengine.TacklerEnvelope, error) {
	applied, err := loomengine.TacklerApplied(attempt.Dir)
	if err != nil {
		return loomengine.TacklerEnvelope{}, err
	}
	if applied {
		return loomengine.TacklerEnvelope{}, fmt.Errorf("%w: attempt %d was applied and the run stopped at the same place", errTacklerResumeReturned, attempt.Number)
	}
	if _, found, verdictErr := loomengine.ReadTacklerVerdict(loomengine.TacklerVerdictPath(attempt.Dir)); found || verdictErr != nil {
		return c.conclude(attempt)
	}
	marker, markerFound, err := loomengine.ReadTacklerMarker(c.deps.MarkerPath)
	if err != nil {
		return loomengine.TacklerEnvelope{}, err
	}
	now := c.deps.Now()
	switch {
	case markerFound && marker.Attempt == attempt.Number && !now.Before(marker.Deadline):
		return c.stopRunAndHandBack(attempt, func() error { return c.deps.Shuttle.StopStrand(marker.StrandGUID) }, marker.StrandGUID, arbiterCauseTimeout, "the tackler ran past its deadline")
	case markerFound && marker.Attempt == attempt.Number:
		return c.attachAndConclude(attempt, marker.StrandGUID, marker.Deadline.Sub(now))
	case attempt.Deadline.After(now):
		return c.attachAndConclude(attempt, attempt.StrandGUID, attempt.Deadline.Sub(now))
	}
	return c.handBack(attempt, arbiterCauseMissing, "the tackler left no verdict")
}

// attachAndConclude waits on the live tackler of attempt for at most remaining and concludes the attempt.
// guid is the strand a failed wait stops; when it is empty the attached run's own strand is stopped.
func (c *tacklerCall) attachAndConclude(attempt loomengine.TacklerAttempt, guid string, remaining time.Duration) (loomengine.TacklerEnvelope, error) {
	result, found, err := c.attach(attempt, remaining)
	if err == nil && !found {
		return c.conclude(attempt)
	}
	if guid == "" {
		guid = result.StrandGUID
	}
	return c.concludeWait(attempt, result, err, func() error { return c.deps.Shuttle.StopStrand(guid) }, guid)
}

// attach waits on a live tackler of attempt for at most remaining.
func (c *tacklerCall) attach(attempt loomengine.TacklerAttempt, remaining time.Duration) (shuttleengine.Result, bool, error) {
	spec := tacklerSpec("", loomengine.TacklerVerdictPath(attempt.Dir), loomengine.TacklerCaseBriefPath(attempt.Dir), c.deps.Settings)
	spec.Timeout = remaining
	return c.deps.Shuttle.AttachIfLive(spec)
}

// settleFresh spawns a new tackler for the stop, after the budget checks.
func (c *tacklerCall) settleFresh(attempts []loomengine.TacklerAttempt) (loomengine.TacklerEnvelope, error) {
	settings := c.deps.Settings
	if settings.MaxSpawns == 0 {
		return loomengine.TacklerEnvelope{}, fmt.Errorf("%w: tackler_max_spawns: 0 turns the tackler off", errTacklerUnavailable)
	}
	if len(attempts) >= settings.MaxSpawns {
		return loomengine.TacklerEnvelope{}, fmt.Errorf("%w: spawn budget spent: the run has used %d of its %d tackler spawns", errTacklerUnavailable, len(attempts), settings.MaxSpawns)
	}

	number := 1
	if len(attempts) > 0 {
		number = attempts[len(attempts)-1].Number + 1
	}
	snapshot, err := c.deps.Snapshot()
	if err != nil {
		return loomengine.TacklerEnvelope{}, tacklerRefusal("the worktree snapshot: %v", err)
	}
	dir := c.deps.AttemptDir(number)
	now := c.deps.Now()
	attempt := loomengine.TacklerAttempt{
		Number:       number,
		StopKey:      c.key,
		PauseBefore:  c.status.PauseBefore,
		PauseAfter:   c.status.PauseAfter,
		Snapshot:     snapshot,
		SpawnedAt:    now,
		Deadline:     now.Add(settings.Timeout),
		EnvelopePath: c.in.EnvelopePath,
		ReportPath:   c.in.ReportPath,
		Dir:          dir,
	}
	if err := loomengine.WriteTacklerAttempt(dir, attempt); err != nil {
		return loomengine.TacklerEnvelope{}, err
	}

	promptInput := c.deps.Prompt
	promptInput.RunID = c.in.RunID
	promptInput.EnvelopePath = c.in.EnvelopePath
	promptInput.ReportPath = c.in.ReportPath
	promptInput.TraceCopy = c.in.TraceCopy
	promptInput.StderrPath = c.in.StderrPath
	promptInput.VerdictPath = loomengine.TacklerVerdictPath(dir)
	promptInput.CaseBriefPath = loomengine.TacklerCaseBriefPath(dir)
	prompt, err := tacklerPrompt(c.deps.StencilsDir, promptInput)
	if err != nil {
		return loomengine.TacklerEnvelope{}, err
	}
	if err := os.WriteFile(loomengine.TacklerPromptPath(dir), []byte(prompt), 0o644); err != nil {
		return loomengine.TacklerEnvelope{}, fmt.Errorf("tackler: write the attempt's prompt: %w", err)
	}

	logger.Info("loom: tackler spawn", "run", c.in.RunID, "attempt", number, "producer", c.status.CurrentProducer, "deadline", attempt.Deadline)
	run, err := c.deps.Shuttle.Start(tacklerSpec(prompt, promptInput.VerdictPath, promptInput.CaseBriefPath, settings))
	if err != nil {
		return c.handBack(attempt, arbiterCauseDied, fmt.Sprintf("starting the tackler failed: %v", err))
	}
	attempt.StrandGUID = run.StrandGUID()
	if err := c.recordStartedTackler(attempt); err != nil {
		// The run must not proceed beside a live tackler nothing tracks.
		if stopErr := run.Stop(); stopErr != nil {
			return loomengine.TacklerEnvelope{}, fmt.Errorf("%w; stopping the tackler strand %s also failed: %v", err, attempt.StrandGUID, stopErr)
		}
		return loomengine.TacklerEnvelope{}, err
	}

	result, waitErr := run.Wait()
	return c.concludeWait(attempt, result, waitErr, run.Stop, attempt.StrandGUID)
}

// recordStartedTackler saves the started tackler's strand in its attempt record and writes the in-flight marker.
func (c *tacklerCall) recordStartedTackler(attempt loomengine.TacklerAttempt) error {
	if err := loomengine.WriteTacklerAttempt(attempt.Dir, attempt); err != nil {
		return err
	}
	marker := loomengine.TacklerMarker{Attempt: attempt.Number, StrandGUID: attempt.StrandGUID, Deadline: attempt.Deadline}
	return loomengine.WriteTacklerMarker(c.deps.MarkerPath, marker)
}

// concludeWait concludes attempt after a wait ended with result and waitErr.
// A verdict on disk wins whatever the wait reported.
// A timeout that ended while the hub's pause record covered part of the run moves the deadline out by the paused time and attaches again for the remainder.
// Any other run that did not finish is stopped through stop and reads as a hand-back.
func (c *tacklerCall) concludeWait(attempt loomengine.TacklerAttempt, result shuttleengine.Result, waitErr error, stop func() error, guid string) (loomengine.TacklerEnvelope, error) {
	for {
		if _, found, verdictErr := loomengine.ReadTacklerVerdict(loomengine.TacklerVerdictPath(attempt.Dir)); found || verdictErr != nil {
			return c.conclude(attempt)
		}
		switch {
		case waitErr != nil:
			return c.stopRunAndHandBack(attempt, stop, guid, arbiterCauseDied, fmt.Sprintf("waiting on the tackler failed: %v", waitErr))
		case result.Outcome == shuttleengine.OutcomeTimeout:
			extended, err := c.extendDeadlineByPausedTime(&attempt, guid)
			if err != nil {
				return loomengine.TacklerEnvelope{}, err
			}
			if !extended {
				return c.stopRunAndHandBack(attempt, stop, guid, arbiterCauseTimeout, "the tackler ran past its deadline")
			}
			var found bool
			result, found, waitErr = c.attach(attempt, attempt.Deadline.Sub(c.deps.Now()))
			if waitErr == nil && !found {
				return c.conclude(attempt)
			}
			if guid == "" {
				guid = result.StrandGUID
			}
			stop = func() error { return c.deps.Shuttle.StopStrand(guid) }
		case result.Outcome != shuttleengine.OutcomeDone:
			return c.stopRunAndHandBack(attempt, stop, guid, arbiterCauseDied, fmt.Sprintf("the tackler ended %s without a verdict", result.Outcome))
		default:
			return c.handBack(attempt, arbiterCauseMissing, "the tackler finished without writing a verdict")
		}
	}
}

// extendDeadlineByPausedTime moves attempt's deadline out so the tackler's awake time, not the wall time, bounds its run.
// It reports false, and changes nothing, when the pause record covered no new time inside the window, when it cannot be read (one Warn), or when the awake budget is spent.
// Otherwise it rewrites the attempt record and the in-flight marker with the new deadline.
func (c *tacklerCall) extendDeadlineByPausedTime(attempt *loomengine.TacklerAttempt, guid string) (bool, error) {
	now := c.deps.Now()
	paused, err := c.deps.PausedWithin(attempt.SpawnedAt, now)
	if err != nil {
		logger.Warn("loom: tackler paused time unreadable", "run", c.in.RunID, "attempt", attempt.Number, "error", err)
		return false, nil
	}
	deadline := attempt.SpawnedAt.Add(c.deps.Settings.Timeout + paused)
	if paused <= 0 || !deadline.After(attempt.Deadline) || !deadline.After(now) {
		return false, nil
	}
	attempt.Deadline = deadline
	if err := loomengine.WriteTacklerAttempt(attempt.Dir, *attempt); err != nil {
		return false, err
	}
	marker := loomengine.TacklerMarker{Attempt: attempt.Number, StrandGUID: guid, Deadline: deadline}
	if err := loomengine.WriteTacklerMarker(c.deps.MarkerPath, marker); err != nil {
		return false, err
	}
	return true, nil
}

// stopRunAndHandBack runs stop, logs it and reads the attempt as a hand-back with cause.
// A stop that fails is returned as an error, since the tackler may still be live and the run must not proceed beside it.
func (c *tacklerCall) stopRunAndHandBack(attempt loomengine.TacklerAttempt, stop func() error, guid, cause, detail string) (loomengine.TacklerEnvelope, error) {
	logger.Warn("loom: tackler stopped", "run", c.in.RunID, "attempt", attempt.Number, "strand", guid, "cause", cause)
	if err := stop(); err != nil {
		return loomengine.TacklerEnvelope{}, fmt.Errorf("tackler: stop the tackler strand %s: %w; %s", guid, err, arbiterHandBackWayForward)
	}
	return c.handBack(attempt, cause, detail)
}

// conclude validates the attempt's verdict file and finishes the call with it.
func (c *tacklerCall) conclude(attempt loomengine.TacklerAttempt) (loomengine.TacklerEnvelope, error) {
	verdict, found, err := loomengine.ReadTacklerVerdict(loomengine.TacklerVerdictPath(attempt.Dir))
	if err != nil {
		return c.handBack(attempt, arbiterCauseUnparseable, err.Error())
	}
	if !found {
		return c.handBack(attempt, arbiterCauseMissing, "the tackler left no verdict")
	}
	envelope := loomengine.TacklerEnvelope{
		Verdict:     verdict.Verdict,
		Reason:      verdict.Reason,
		Question:    verdict.Question,
		Goto:        verdict.Goto,
		Attempt:     attempt.Number,
		AttemptDir:  attempt.Dir,
		VerdictPath: loomengine.TacklerVerdictPath(attempt.Dir),
	}
	switch verdict.Verdict {
	case loomengine.TacklerVerdictHandBack:
		if verdict.Question == "" {
			return c.handBack(attempt, tacklerCauseQuestion, "a hand-back verdict names no question for the run's parent")
		}
	case loomengine.TacklerVerdictEscalate:
		caseBrief := loomengine.TacklerCaseBriefPath(attempt.Dir)
		if info, err := os.Stat(caseBrief); err != nil || info.Size() == 0 {
			return c.handBack(attempt, tacklerCauseCaseBrief, fmt.Sprintf("an escalate verdict needs a case brief at %s, and it is missing or empty", caseBrief))
		}
		envelope.CaseBriefPath = caseBrief
	case loomengine.TacklerVerdictResume:
		if problem := c.resumePrecondition(verdict); problem != "" {
			return c.handBack(attempt, arbiterCausePrecondition, problem)
		}
	}
	return c.finish(envelope)
}

// resumePrecondition returns what stops a resume from being sound at the call's stop, or empty when nothing does.
// A goto names the status's current row or the producer of the last history entry, the halting entry's routed-from row.
func (c *tacklerCall) resumePrecondition(verdict loomengine.TacklerVerdict) string {
	if verdict.Goto == "" {
		if c.status.State == shedengine.StateBlocked {
			return "a resume at a blocked stop needs a goto target, and the verdict names none"
		}
		return ""
	}
	lastProducer := ""
	if history := c.status.History; len(history) > 0 {
		lastProducer = history[len(history)-1].Producer
	}
	if verdict.Goto != c.status.CurrentProducer && verdict.Goto != lastProducer {
		return fmt.Sprintf("a tackler goto names the current row or the row the halting entry was routed from, and %q is neither", verdict.Goto)
	}
	if err := shedengine.CheckGotoTarget(c.deps.Producers, c.status, verdict.Goto, c.in.RunID); err != nil {
		return err.Error()
	}
	return ""
}

// handBack finishes the call with a hand-back the call made itself, naming cause.
func (c *tacklerCall) handBack(attempt loomengine.TacklerAttempt, cause, detail string) (loomengine.TacklerEnvelope, error) {
	return c.finish(loomengine.TacklerEnvelope{
		Verdict:     loomengine.TacklerVerdictHandBack,
		Reason:      detail,
		Question:    fmt.Sprintf("The tackler could not settle this stop (%s); decide it yourself and report the choice in the stop report.", cause),
		Cause:       cause,
		Attempt:     attempt.Number,
		AttemptDir:  attempt.Dir,
		VerdictPath: loomengine.TacklerVerdictPath(attempt.Dir),
	})
}

// finish removes the in-flight marker and saves envelope in its attempt directory.
func (c *tacklerCall) finish(envelope loomengine.TacklerEnvelope) (loomengine.TacklerEnvelope, error) {
	if err := loomengine.RemoveTacklerMarker(c.deps.MarkerPath); err != nil {
		return loomengine.TacklerEnvelope{}, err
	}
	if err := loomengine.WriteTacklerEnvelope(loomengine.TacklerEnvelopePath(envelope.AttemptDir), envelope); err != nil {
		return loomengine.TacklerEnvelope{}, err
	}
	return envelope, nil
}
