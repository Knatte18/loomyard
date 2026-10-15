// tackler.go implements the shed driver's call to a cold tackler agent.
// At a stop no rule settles, it spawns the tackler through shuttle, waits for its verdict file and returns the validated envelope.
// The call works over injected seams, so every branch is testable with a fake shuttle and clock; it moves no run itself.

package sheddriver

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The causes beyond the shared ones that a tackler envelope carries when the call, not the agent, made the hand-back.
const (
	tacklerCauseQuestion  = "question"
	tacklerCauseCaseBrief = "case-brief"
)

// TacklerSettings is the tackler role's resolved model choice, timeout and spawn budget, threaded onto the tackler session's launch spec.
type TacklerSettings struct {
	shuttleengine.ModelChoice
	// Timeout is how long one tackler spawn may run.
	Timeout time.Duration
	// MaxSpawns is how many tackler spawns a run may use; zero turns the tackler off.
	MaxSpawns int
}

// ErrTacklerUnavailable is returned when the tackler is switched off or its spawn budget is spent, so the driver hands back without a spawn.
var ErrTacklerUnavailable = errors.New("tackler unavailable")

// ErrTacklerResumeReturned is returned when an attempt's resume was applied and the run stopped at the same stop key again.
var ErrTacklerResumeReturned = errors.New("tackler resume returned")

// TacklerDeps is every seam and fixed value the call reads.
type TacklerDeps struct {
	// Closed reports whether the stop key's attempt spawned at spawned was already handed to the parent: a hand-back settle record for the key later than spawned.
	Closed func(key StopKey, spawned time.Time) (bool, error)
	// ReadStatus reads the run's status; found is false when it has no status file.
	ReadStatus func() (status shedengine.Status, found bool, err error)
	// Settings is the resolved tackler role.
	Settings TacklerSettings
	// TacklerDir is the durable directory holding every attempt of the run.
	TacklerDir string
	// AttemptDir names the directory of attempt number.
	AttemptDir func(number int) string
	// MarkerPath is the ephemeral in-flight marker's path.
	MarkerPath string
	// Producers is the run's producer list, for goto checks.
	Producers []shedengine.ProducerDef
	// Snapshot reads the worktree state a tackler must leave as it found it; nil for a recipe that keeps no tamper comparison, which records an empty snapshot.
	Snapshot func() (WorktreeSnapshot, error)
	// PausedWithin returns the time the hub's pause record covered the run's slug inside the window from to.
	PausedWithin func(from, to time.Time) (time.Duration, error)
	// Recipe is the one-line description of the recipe the run belongs to, filled into the tackler's prompt.
	Recipe string
	// GotoRoutedFrom is whether a resume may name the row the halting history entry was routed from as well as the current row.
	GotoRoutedFrom bool
	// Prompt holds the prompt inputs that do not vary per attempt; StencilsDir is where the tackler stencil is read from.
	Prompt      TacklerPromptInput
	StencilsDir string
	// Shuttle starts, attaches to and stops the tackler.
	Shuttle AgentShuttle
	// Now is the clock.
	Now func() time.Time
}

// TacklerInput is what one call carries about the stop it judges.
type TacklerInput struct {
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

// TacklerCall is one call over a status read at its start.
type TacklerCall struct {
	deps   TacklerDeps
	in     TacklerInput
	status shedengine.Status
	key    StopKey
}

// tacklerRefusal builds an error whose way forward is the hand-back to the run's parent.
func tacklerRefusal(format string, args ...any) error {
	return fmt.Errorf("tackler: "+format+"; %s", append(args, HandBackWayForward)...)
}

// NewTacklerCall reads the status, refusing when it is missing or unreadable.
func NewTacklerCall(deps TacklerDeps, in TacklerInput) (*TacklerCall, error) {
	status, found, err := deps.ReadStatus()
	if err != nil {
		return nil, tacklerRefusal("status unreadable: the status file: %v", err)
	}
	if !found {
		return nil, tacklerRefusal("status unreadable: the run has no status file")
	}
	return &TacklerCall{deps: deps, in: in, status: status, key: StopKeyOf(status)}, nil
}

// Settle returns the validated envelope for the call's stop: an earlier attempt's when one judged this stop and was not yet handed to the parent, else a fresh attempt's.
// It returns ErrTacklerUnavailable when no attempt exists and none may be spawned, and ErrTacklerResumeReturned when the stop's attempt resumed the run and the run stopped at the same place.
func (c *TacklerCall) Settle() (TacklerEnvelope, error) {
	attempts, err := ListTacklerAttempts(c.deps.TacklerDir)
	if err != nil {
		return TacklerEnvelope{}, err
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].StopKey != c.key {
			continue
		}
		closed, err := c.deps.Closed(c.key, attempts[i].SpawnedAt)
		if err != nil {
			return TacklerEnvelope{}, err
		}
		if closed {
			continue
		}
		return c.settleExisting(attempts[i])
	}
	return c.settleFresh(attempts)
}

// settleExisting settles the stop from attempt, the newest open one for it, spawning nothing.
func (c *TacklerCall) settleExisting(attempt TacklerAttempt) (TacklerEnvelope, error) {
	applied, err := TacklerApplied(attempt.Dir)
	if err != nil {
		return TacklerEnvelope{}, err
	}
	if applied {
		return TacklerEnvelope{}, fmt.Errorf("%w: attempt %d was applied and the run stopped at the same place", ErrTacklerResumeReturned, attempt.Number)
	}
	if _, found, verdictErr := ReadTacklerVerdict(TacklerVerdictPath(attempt.Dir)); found || verdictErr != nil {
		return c.conclude(attempt)
	}
	marker, markerFound, err := ReadTacklerMarker(c.deps.MarkerPath)
	if err != nil {
		return TacklerEnvelope{}, err
	}
	now := c.deps.Now()
	switch {
	case markerFound && marker.Attempt == attempt.Number && !now.Before(marker.Deadline):
		return c.stopRunAndHandBack(attempt, func() error { return c.deps.Shuttle.StopStrand(marker.StrandGUID) }, marker.StrandGUID, CauseTimeout, "the tackler ran past its deadline")
	case markerFound && marker.Attempt == attempt.Number:
		return c.attachAndConclude(attempt, marker.StrandGUID, marker.Deadline.Sub(now))
	case attempt.Deadline.After(now):
		return c.attachAndConclude(attempt, attempt.StrandGUID, attempt.Deadline.Sub(now))
	}
	return c.handBack(attempt, CauseMissing, "the tackler left no verdict")
}

// attachAndConclude waits on the live tackler of attempt for at most remaining and concludes the attempt.
// guid is the strand a failed wait stops; when it is empty the attached run's own strand is stopped.
func (c *TacklerCall) attachAndConclude(attempt TacklerAttempt, guid string, remaining time.Duration) (TacklerEnvelope, error) {
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
func (c *TacklerCall) attach(attempt TacklerAttempt, remaining time.Duration) (shuttleengine.Result, bool, error) {
	spec := tacklerSpec("", TacklerVerdictPath(attempt.Dir), TacklerCaseBriefPath(attempt.Dir), c.deps.Settings)
	spec.Timeout = remaining
	return c.deps.Shuttle.AttachIfLive(spec)
}

// settleFresh spawns a new tackler for the stop, after the budget checks.
func (c *TacklerCall) settleFresh(attempts []TacklerAttempt) (TacklerEnvelope, error) {
	settings := c.deps.Settings
	if settings.MaxSpawns == 0 {
		return TacklerEnvelope{}, fmt.Errorf("%w: tackler_max_spawns: 0 turns the tackler off", ErrTacklerUnavailable)
	}
	if len(attempts) >= settings.MaxSpawns {
		return TacklerEnvelope{}, fmt.Errorf("%w: spawn budget spent: the run has used %d of its %d tackler spawns", ErrTacklerUnavailable, len(attempts), settings.MaxSpawns)
	}

	number := 1
	if len(attempts) > 0 {
		number = attempts[len(attempts)-1].Number + 1
	}
	var snapshot WorktreeSnapshot
	if c.deps.Snapshot != nil {
		var err error
		snapshot, err = c.deps.Snapshot()
		if err != nil {
			return TacklerEnvelope{}, tacklerRefusal("the worktree snapshot: %v", err)
		}
	}
	dir := c.deps.AttemptDir(number)
	now := c.deps.Now()
	attempt := TacklerAttempt{
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
	if err := WriteTacklerAttempt(dir, attempt); err != nil {
		return TacklerEnvelope{}, err
	}

	promptInput := c.deps.Prompt
	promptInput.RunID = c.in.RunID
	promptInput.Recipe = c.deps.Recipe
	promptInput.GotoRows = formatGotoRows(c.gotoRows())
	promptInput.EnvelopePath = c.in.EnvelopePath
	promptInput.ReportPath = c.in.ReportPath
	promptInput.TraceCopy = c.in.TraceCopy
	promptInput.StderrPath = c.in.StderrPath
	promptInput.VerdictPath = TacklerVerdictPath(dir)
	promptInput.CaseBriefPath = TacklerCaseBriefPath(dir)
	prompt, err := tacklerPrompt(c.deps.StencilsDir, promptInput)
	if err != nil {
		return TacklerEnvelope{}, err
	}
	if err := os.WriteFile(TacklerPromptPath(dir), []byte(prompt), 0o644); err != nil {
		return TacklerEnvelope{}, fmt.Errorf("tackler: write the attempt's prompt: %w", err)
	}

	logger.Info("sheddriver: tackler spawn", "run", c.in.RunID, "attempt", number, "producer", c.status.CurrentProducer, "deadline", attempt.Deadline)
	run, err := c.deps.Shuttle.Start(tacklerSpec(prompt, promptInput.VerdictPath, promptInput.CaseBriefPath, settings))
	if err != nil {
		return c.handBack(attempt, CauseDied, fmt.Sprintf("starting the tackler failed: %v", err))
	}
	attempt.StrandGUID = run.StrandGUID()
	if err := c.recordStartedTackler(attempt); err != nil {
		// The run must not proceed beside a live tackler nothing tracks.
		if stopErr := run.Stop(); stopErr != nil {
			return TacklerEnvelope{}, fmt.Errorf("%w; stopping the tackler strand %s also failed: %v", err, attempt.StrandGUID, stopErr)
		}
		return TacklerEnvelope{}, err
	}

	result, waitErr := run.Wait()
	return c.concludeWait(attempt, result, waitErr, run.Stop, attempt.StrandGUID)
}

// recordStartedTackler saves the started tackler's strand in its attempt record and writes the in-flight marker.
func (c *TacklerCall) recordStartedTackler(attempt TacklerAttempt) error {
	if err := WriteTacklerAttempt(attempt.Dir, attempt); err != nil {
		return err
	}
	marker := TacklerMarker{Attempt: attempt.Number, StrandGUID: attempt.StrandGUID, Deadline: attempt.Deadline}
	return WriteTacklerMarker(c.deps.MarkerPath, marker)
}

// concludeWait concludes attempt after a wait ended with result and waitErr.
// A verdict on disk wins whatever the wait reported.
// A timeout that ended while the hub's pause record covered part of the run moves the deadline out by the paused time and attaches again for the remainder.
// Any other run that did not finish is stopped through stop and reads as a hand-back.
func (c *TacklerCall) concludeWait(attempt TacklerAttempt, result shuttleengine.Result, waitErr error, stop func() error, guid string) (TacklerEnvelope, error) {
	for {
		if _, found, verdictErr := ReadTacklerVerdict(TacklerVerdictPath(attempt.Dir)); found || verdictErr != nil {
			return c.conclude(attempt)
		}
		switch {
		case waitErr != nil:
			return c.stopRunAndHandBack(attempt, stop, guid, CauseDied, fmt.Sprintf("waiting on the tackler failed: %v", waitErr))
		case result.Outcome == shuttleengine.OutcomeTimeout:
			extended, err := c.extendDeadlineByPausedTime(&attempt, guid)
			if err != nil {
				return TacklerEnvelope{}, err
			}
			if !extended {
				return c.stopRunAndHandBack(attempt, stop, guid, CauseTimeout, "the tackler ran past its deadline")
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
			return c.stopRunAndHandBack(attempt, stop, guid, CauseDied, fmt.Sprintf("the tackler ended %s without a verdict", result.Outcome))
		default:
			return c.handBack(attempt, CauseMissing, "the tackler finished without writing a verdict")
		}
	}
}

// extendDeadlineByPausedTime moves attempt's deadline out so the tackler's awake time, not the wall time, bounds its run.
// It reports false, and changes nothing, when the pause record covered no new time inside the window, when it cannot be read (one Warn), or when the awake budget is spent.
// Otherwise it rewrites the attempt record and the in-flight marker with the new deadline.
func (c *TacklerCall) extendDeadlineByPausedTime(attempt *TacklerAttempt, guid string) (bool, error) {
	now := c.deps.Now()
	paused, err := c.deps.PausedWithin(attempt.SpawnedAt, now)
	if err != nil {
		logger.Warn("sheddriver: tackler paused time unreadable", "run", c.in.RunID, "attempt", attempt.Number, "error", err)
		return false, nil
	}
	deadline := attempt.SpawnedAt.Add(c.deps.Settings.Timeout + paused)
	if paused <= 0 || !deadline.After(attempt.Deadline) || !deadline.After(now) {
		return false, nil
	}
	attempt.Deadline = deadline
	if err := WriteTacklerAttempt(attempt.Dir, *attempt); err != nil {
		return false, err
	}
	marker := TacklerMarker{Attempt: attempt.Number, StrandGUID: guid, Deadline: deadline}
	if err := WriteTacklerMarker(c.deps.MarkerPath, marker); err != nil {
		return false, err
	}
	return true, nil
}

// stopRunAndHandBack runs stop, logs it and reads the attempt as a hand-back with cause.
// A stop that fails is returned as an error, since the tackler may still be live and the run must not proceed beside it.
func (c *TacklerCall) stopRunAndHandBack(attempt TacklerAttempt, stop func() error, guid, cause, detail string) (TacklerEnvelope, error) {
	logger.Warn("sheddriver: tackler stopped", "run", c.in.RunID, "attempt", attempt.Number, "strand", guid, "cause", cause)
	if err := stop(); err != nil {
		return TacklerEnvelope{}, fmt.Errorf("tackler: stop the tackler strand %s: %w; %s", guid, err, HandBackWayForward)
	}
	return c.handBack(attempt, cause, detail)
}

// conclude validates the attempt's verdict file and finishes the call with it.
func (c *TacklerCall) conclude(attempt TacklerAttempt) (TacklerEnvelope, error) {
	verdict, found, err := ReadTacklerVerdict(TacklerVerdictPath(attempt.Dir))
	if err != nil {
		return c.handBack(attempt, CauseUnparseable, err.Error())
	}
	if !found {
		return c.handBack(attempt, CauseMissing, "the tackler left no verdict")
	}
	envelope := TacklerEnvelope{
		Verdict:     verdict.Verdict,
		Reason:      verdict.Reason,
		Question:    verdict.Question,
		Goto:        verdict.Goto,
		Attempt:     attempt.Number,
		AttemptDir:  attempt.Dir,
		VerdictPath: TacklerVerdictPath(attempt.Dir),
	}
	switch verdict.Verdict {
	case TacklerVerdictHandBack:
		if verdict.Question == "" {
			return c.handBack(attempt, tacklerCauseQuestion, "a hand-back verdict names no question for the run's parent")
		}
	case TacklerVerdictEscalate:
		caseBrief := TacklerCaseBriefPath(attempt.Dir)
		if info, err := os.Stat(caseBrief); err != nil || info.Size() == 0 {
			return c.handBack(attempt, tacklerCauseCaseBrief, fmt.Sprintf("an escalate verdict needs a case brief at %s, and it is missing or empty", caseBrief))
		}
		envelope.CaseBriefPath = caseBrief
	case TacklerVerdictResume:
		if problem := c.resumePrecondition(verdict); problem != "" {
			return c.handBack(attempt, CausePrecondition, problem)
		}
	}
	return c.finish(envelope)
}

// gotoRows lists the rows a resume may name: the status's current row, and with GotoRoutedFrom the producer of the last history entry, the halting entry's routed-from row.
func (c *TacklerCall) gotoRows() []string {
	rows := []string{c.status.CurrentProducer}
	if !c.deps.GotoRoutedFrom {
		return rows
	}
	if history := c.status.History; len(history) > 0 {
		if routedFrom := history[len(history)-1].Producer; routedFrom != c.status.CurrentProducer {
			rows = append(rows, routedFrom)
		}
	}
	return rows
}

// resumePrecondition returns what stops a resume from being sound at the call's stop, or empty when nothing does.
// A goto may name only the rows gotoRows lists.
func (c *TacklerCall) resumePrecondition(verdict TacklerVerdict) string {
	if verdict.Goto == "" {
		if c.status.State == shedengine.StateBlocked {
			return "a resume at a blocked stop needs a goto target, and the verdict names none"
		}
		return ""
	}
	rows := c.gotoRows()
	named := false
	for _, row := range rows {
		named = named || verdict.Goto == row
	}
	if !named {
		return fmt.Sprintf("a tackler goto names only %s, and %q is none of them", formatGotoRows(rows), verdict.Goto)
	}
	if err := shedengine.CheckGotoTarget(c.deps.Producers, c.status, verdict.Goto, c.in.RunID); err != nil {
		return err.Error()
	}
	return ""
}

// formatGotoRows renders rows as backticked names joined by "or", the form the tackler prompt names them in.
func formatGotoRows(rows []string) string {
	quoted := make([]string, len(rows))
	for i, row := range rows {
		quoted[i] = "`" + row + "`"
	}
	return strings.Join(quoted, " or ")
}

// handBack finishes the call with a hand-back the call made itself, naming cause.
func (c *TacklerCall) handBack(attempt TacklerAttempt, cause, detail string) (TacklerEnvelope, error) {
	return c.finish(TacklerEnvelope{
		Verdict:     TacklerVerdictHandBack,
		Reason:      detail,
		Question:    fmt.Sprintf("The tackler could not settle this stop (%s); decide it yourself and report the choice in the stop report.", cause),
		Cause:       cause,
		Attempt:     attempt.Number,
		AttemptDir:  attempt.Dir,
		VerdictPath: TacklerVerdictPath(attempt.Dir),
	})
}

// finish removes the in-flight marker and saves envelope in its attempt directory.
func (c *TacklerCall) finish(envelope TacklerEnvelope) (TacklerEnvelope, error) {
	if err := RemoveTacklerMarker(c.deps.MarkerPath); err != nil {
		return TacklerEnvelope{}, err
	}
	if err := WriteTacklerEnvelope(TacklerEnvelopePath(envelope.AttemptDir), envelope); err != nil {
		return TacklerEnvelope{}, err
	}
	return envelope, nil
}
