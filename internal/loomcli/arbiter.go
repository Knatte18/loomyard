// arbiter.go implements the body of `lyx loom arbiter`.
// At a stop the run's driver cannot settle, it spawns a cold arbiter agent through shuttle, waits for its verdict file and prints the validated verdict.
// The body works over injected seams, so every branch is testable with a fake shuttle and clock; the wiring that adapts the real ones sits beside it.

package loomcli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The causes an envelope carries when the verb, not the agent, made the verdict hand-on.
const (
	arbiterCauseTimeout      = "timeout"
	arbiterCauseDied         = "died"
	arbiterCauseMissing      = "missing"
	arbiterCauseUnparseable  = "unparseable"
	arbiterCausePrecondition = "precondition"
)

// arbiterRun is the handle of a started arbiter.
type arbiterRun interface {
	// Wait blocks until the arbiter ends and classifies how.
	Wait() (shuttleengine.Result, error)
	// Stop ends the arbiter's strand and records the stop.
	Stop() error
	// StrandGUID names the arbiter's strand.
	StrandGUID() string
}

// arbiterShuttle is the part of shuttle the verb drives.
type arbiterShuttle interface {
	// Start launches spec and returns its handle.
	Start(spec shuttleengine.Spec) (arbiterRun, error)
	// AttachIfLive waits on a live run matching spec's output files, and reports false when none is live.
	AttachIfLive(spec shuttleengine.Spec) (shuttleengine.Result, bool, error)
	// StopStrand ends the strand guid names and records the stop.
	StopStrand(guid string) error
}

// arbiterInput is what the verb's command line carries.
type arbiterInput struct {
	// RunID is the run whose stop is judged.
	RunID string
	// EnvelopePath is the stop envelope the driver saved.
	EnvelopePath string
	// ReportPath is the driver's stop or fork report; empty when there is none.
	ReportPath string
	// CaseBriefPath is the tackler's case brief; empty when the stop reached the arbiter without one.
	CaseBriefPath string
}

// arbiterDeps is every seam and fixed value the verb reads.
type arbiterDeps struct {
	// ReadStatus reads the run's status; found is false when it has no status file.
	ReadStatus func() (status shedengine.Status, found bool, err error)
	// Settings is the resolved arbiter role.
	Settings loomengine.ArbiterSettings
	// ArbiterDir is the durable directory holding every attempt of the run.
	ArbiterDir string
	// AttemptDir names the directory of attempt number.
	AttemptDir func(number int) string
	// MarkerPath is the ephemeral in-flight marker's path.
	MarkerPath string
	// Producers is the run's producer list, for goto checks.
	Producers []shedengine.ProducerDef
	// Stop holds the reads that decide whether the stop is eligible.
	Stop arbiterstop.Readers
	// PendingCirclingDecision reports, at a Bouncer row, whether a circling decision is recorded and unsettled; false elsewhere.
	PendingCirclingDecision func(producer string) (bool, error)
	// Prompt holds the prompt inputs that do not vary per attempt; StencilsDir is where the arbiter stencil is read from.
	Prompt      arbiterPromptInput
	StencilsDir string
	// Shuttle starts, attaches to and stops the arbiter.
	Shuttle arbiterShuttle
	// Now is the clock.
	Now func() time.Time
	// RelaunchRunID is the run id the re-launch line names.
	RelaunchRunID string
}

// arbiterStopEnvelope is the part of the driver's stop envelope the verb reads.
type arbiterStopEnvelope struct {
	Kind      string `json:"kind"`
	Transient string `json:"transient"`
}

// arbiterVerb runs `lyx loom arbiter` over deps and prints its envelope to out, returning the exit code.
// It spawns at most Settings.MaxSpawns agents per run, counted from the attempt records on disk so no restart resets the count, and moves no run itself.
// A re-run for the stop an attempt already arbitrated spawns nothing: it returns that attempt's verdict, or waits on its live arbiter.
func arbiterVerb(out io.Writer, deps arbiterDeps, in arbiterInput) int {
	call, err := newArbiterCall(deps, in)
	if err != nil {
		return output.Err(out, err.Error())
	}
	envelope, err := call.settle()
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, arbiterEnvelopeFields(envelope))
}

// arbiterEnvelopeFields renders envelope as the verb's output keys; relaunch appears for a resume only.
func arbiterEnvelopeFields(e loomengine.ArbiterEnvelope) map[string]any {
	verbs := e.Verbs
	if verbs == nil {
		verbs = []string{}
	}
	fields := map[string]any{
		"verdict":      e.Verdict,
		"reason":       e.Reason,
		"question":     e.Question,
		"goto":         e.Goto,
		"verbs":        verbs,
		"cause":        e.Cause,
		"attempt":      e.Attempt,
		"attempt_dir":  e.AttemptDir,
		"verdict_path": e.VerdictPath,
	}
	if e.Verdict == loomengine.ArbiterVerdictResume {
		fields["relaunch"] = e.Relaunch
	}
	return fields
}

// arbiterRefusal builds an error whose way forward is the hand-back to the run's parent.
func arbiterRefusal(format string, args ...any) error {
	return fmt.Errorf("arbiter: "+format+"; %s", append(args, arbiterHandBackWayForward)...)
}

// arbiterCall is one run of the verb over a status read at its start.
type arbiterCall struct {
	deps   arbiterDeps
	in     arbiterInput
	status shedengine.Status
	stop   arbiterStopEnvelope
	key    sheddriver.StopKey
}

// newArbiterCall reads the status and the stop envelope, refusing when either is missing or unreadable.
func newArbiterCall(deps arbiterDeps, in arbiterInput) (*arbiterCall, error) {
	status, found, err := deps.ReadStatus()
	if err != nil {
		return nil, arbiterRefusal("status or envelope unreadable: the status file: %v", err)
	}
	if !found {
		return nil, arbiterRefusal("status or envelope unreadable: the run has no status file")
	}
	data, err := os.ReadFile(in.EnvelopePath)
	if err != nil {
		return nil, arbiterRefusal("status or envelope unreadable: the stop envelope: %v", err)
	}
	var stop arbiterStopEnvelope
	if err := json.Unmarshal(data, &stop); err != nil {
		return nil, arbiterRefusal("status or envelope unreadable: the stop envelope %s: %v", in.EnvelopePath, err)
	}
	return &arbiterCall{deps: deps, in: in, status: status, stop: stop, key: sheddriver.StopKeyOf(status)}, nil
}

// settle returns the validated verdict for the call's stop: an earlier attempt's when one arbitrated this stop, else a fresh attempt's.
func (c *arbiterCall) settle() (loomengine.ArbiterEnvelope, error) {
	attempts, err := loomengine.ListArbiterAttempts(c.deps.ArbiterDir)
	if err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].StopKey == c.key {
			return c.settleExisting(attempts[i])
		}
	}
	return c.settleFresh(attempts)
}

// settleExisting settles the stop from attempt, the newest one for it, spawning nothing.
func (c *arbiterCall) settleExisting(attempt loomengine.ArbiterAttempt) (loomengine.ArbiterEnvelope, error) {
	applied, err := loomengine.ArbiterApplied(attempt.Dir)
	if err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	if applied {
		return loomengine.ArbiterEnvelope{}, arbiterRefusal("an applied resume left the stop key unchanged: attempt %d was applied and the run stopped at the same place", attempt.Number)
	}
	verdictPath := loomengine.ArbiterVerdictPath(attempt.Dir)
	if _, found, verdictErr := loomengine.ReadArbiterVerdict(verdictPath); found || verdictErr != nil {
		return c.conclude(attempt)
	}
	marker, markerFound, err := loomengine.ReadArbiterMarker(c.deps.MarkerPath)
	if err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	now := c.deps.Now()
	switch {
	case markerFound && marker.Attempt == attempt.Number && !now.Before(marker.Deadline):
		return c.stopAndHandOn(attempt, marker.StrandGUID, arbiterCauseTimeout, "the arbiter ran past its deadline")
	case markerFound && marker.Attempt == attempt.Number:
		return c.attachAndConclude(attempt, marker.StrandGUID, marker.Deadline.Sub(now), arbiterCauseDied)
	case attempt.Deadline.After(now):
		return c.attachAndConclude(attempt, attempt.StrandGUID, attempt.Deadline.Sub(now), arbiterCauseMissing)
	}
	return c.handOn(attempt, arbiterCauseMissing, "the arbiter left no verdict")
}

// attachAndConclude waits on the live arbiter of attempt for at most remaining and concludes the attempt.
// guid is the strand a failed wait stops; when it is empty the attached run's own strand is stopped.
// absentCause is the cause of the hand-on when no arbiter is live and none left a verdict.
func (c *arbiterCall) attachAndConclude(attempt loomengine.ArbiterAttempt, guid string, remaining time.Duration, absentCause string) (loomengine.ArbiterEnvelope, error) {
	spec := arbiterSpec("", loomengine.ArbiterVerdictPath(attempt.Dir), c.deps.Settings)
	spec.Timeout = remaining
	result, found, err := c.deps.Shuttle.AttachIfLive(spec)
	if err == nil && !found {
		return c.handOn(attempt, absentCause, "no arbiter is live for the attempt and it left no verdict")
	}
	if guid == "" {
		guid = result.StrandGUID
	}
	return c.concludeWait(attempt, result, err, func() error { return c.deps.Shuttle.StopStrand(guid) }, guid)
}

// settleFresh spawns a new arbiter for the stop, after the eligibility and budget checks.
func (c *arbiterCall) settleFresh(attempts []loomengine.ArbiterAttempt) (loomengine.ArbiterEnvelope, error) {
	briefPath, err := c.checkEligible()
	if err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	settings := c.deps.Settings
	if settings.MaxSpawns == 0 {
		return loomengine.ArbiterEnvelope{}, arbiterRefusal("off: arbiter_max_spawns is 0")
	}
	if len(attempts) >= settings.MaxSpawns {
		return loomengine.ArbiterEnvelope{}, arbiterRefusal("spawn budget spent: the run has used %d of its %d arbiter spawns", len(attempts), settings.MaxSpawns)
	}

	number := 1
	if len(attempts) > 0 {
		number = attempts[len(attempts)-1].Number + 1
	}
	dir := c.deps.AttemptDir(number)
	now := c.deps.Now()
	attempt := loomengine.ArbiterAttempt{
		Number:        number,
		StopKey:       c.key,
		PauseBefore:   c.status.PauseBefore,
		PauseAfter:    c.status.PauseAfter,
		SpawnedAt:     now,
		Deadline:      now.Add(settings.Timeout),
		EnvelopePath:  c.in.EnvelopePath,
		ReportPath:    c.in.ReportPath,
		CaseBriefPath: c.in.CaseBriefPath,
		Dir:           dir,
	}
	if err := loomengine.WriteArbiterAttempt(dir, attempt); err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}

	promptInput := c.deps.Prompt
	promptInput.RunID = c.in.RunID
	promptInput.EnvelopePath = c.in.EnvelopePath
	promptInput.ReportPath = c.in.ReportPath
	promptInput.BriefPath = briefPath
	promptInput.CaseBriefPath = c.in.CaseBriefPath
	promptInput.VerdictPath = loomengine.ArbiterVerdictPath(dir)
	prompt, err := arbiterPrompt(c.deps.StencilsDir, promptInput)
	if err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	if err := os.WriteFile(loomengine.ArbiterPromptPath(dir), []byte(prompt), 0o644); err != nil {
		return loomengine.ArbiterEnvelope{}, fmt.Errorf("arbiter: write the attempt's prompt: %w", err)
	}

	logger.Info("loom: arbiter spawn", "run", c.in.RunID, "attempt", number, "producer", c.status.CurrentProducer, "state", string(c.status.State), "deadline", attempt.Deadline)
	run, err := c.deps.Shuttle.Start(arbiterSpec(prompt, promptInput.VerdictPath, settings))
	if err != nil {
		return loomengine.ArbiterEnvelope{}, fmt.Errorf("arbiter: start the arbiter agent: %w; %s", err, arbiterHandBackWayForward)
	}
	attempt.StrandGUID = run.StrandGUID()
	if err := c.recordStartedArbiter(dir, attempt); err != nil {
		// The run must not proceed beside a live arbiter nothing tracks.
		if stopErr := run.Stop(); stopErr != nil {
			return loomengine.ArbiterEnvelope{}, fmt.Errorf("%w; stopping the arbiter strand %s also failed: %v", err, attempt.StrandGUID, stopErr)
		}
		return loomengine.ArbiterEnvelope{}, err
	}

	result, waitErr := run.Wait()
	return c.concludeWait(attempt, result, waitErr, run.Stop, attempt.StrandGUID)
}

// recordStartedArbiter saves the started arbiter's strand in its attempt record and writes the in-flight marker.
func (c *arbiterCall) recordStartedArbiter(dir string, attempt loomengine.ArbiterAttempt) error {
	if err := loomengine.WriteArbiterAttempt(dir, attempt); err != nil {
		return err
	}
	marker := loomengine.ArbiterMarker{Attempt: attempt.Number, StrandGUID: attempt.StrandGUID, Deadline: attempt.Deadline}
	return loomengine.WriteArbiterMarker(c.deps.MarkerPath, marker)
}

// checkEligible refuses a stop the arbiter may not settle.
// It returns the path of the review escalation brief at a Bouncer stop, and empty elsewhere.
func (c *arbiterCall) checkEligible() (briefPath string, err error) {
	return arbiterstop.Check(c.status, c.stop.Kind, c.stop.Transient, c.deps.Stop)
}

// concludeWait concludes attempt after a wait ended with result and waitErr.
// A verdict on disk wins whatever the wait reported; otherwise a run that did not finish is stopped through stop and reads as a hand-on.
func (c *arbiterCall) concludeWait(attempt loomengine.ArbiterAttempt, result shuttleengine.Result, waitErr error, stop func() error, guid string) (loomengine.ArbiterEnvelope, error) {
	if _, found, verdictErr := loomengine.ReadArbiterVerdict(loomengine.ArbiterVerdictPath(attempt.Dir)); found || verdictErr != nil {
		return c.conclude(attempt)
	}
	switch {
	case waitErr != nil:
		return c.stopRunAndHandOn(attempt, stop, guid, arbiterCauseDied, fmt.Sprintf("waiting on the arbiter failed: %v", waitErr))
	case result.Outcome == shuttleengine.OutcomeTimeout:
		return c.stopRunAndHandOn(attempt, stop, guid, arbiterCauseTimeout, "the arbiter ran past its deadline")
	case result.Outcome != shuttleengine.OutcomeDone:
		return c.stopRunAndHandOn(attempt, stop, guid, arbiterCauseDied, fmt.Sprintf("the arbiter ended %s without a verdict", result.Outcome))
	}
	return c.handOn(attempt, arbiterCauseMissing, "the arbiter finished without writing a verdict")
}

// stopAndHandOn stops the strand guid names through shuttle's stop verb, logs it and reads the attempt as a hand-on with cause.
func (c *arbiterCall) stopAndHandOn(attempt loomengine.ArbiterAttempt, guid, cause, detail string) (loomengine.ArbiterEnvelope, error) {
	return c.stopRunAndHandOn(attempt, func() error { return c.deps.Shuttle.StopStrand(guid) }, guid, cause, detail)
}

// stopRunAndHandOn runs stop, logs it and reads the attempt as a hand-on with cause.
// A stop that fails is returned as an error, since the arbiter may still be live and the run must not proceed beside it.
func (c *arbiterCall) stopRunAndHandOn(attempt loomengine.ArbiterAttempt, stop func() error, guid, cause, detail string) (loomengine.ArbiterEnvelope, error) {
	logger.Warn("loom: arbiter stopped", "run", c.in.RunID, "attempt", attempt.Number, "strand", guid, "cause", cause)
	if err := stop(); err != nil {
		return loomengine.ArbiterEnvelope{}, fmt.Errorf("arbiter: stop the arbiter strand %s: %w; %s", guid, err, arbiterHandBackWayForward)
	}
	return c.handOn(attempt, cause, detail)
}

// conclude validates the attempt's verdict file and finishes the call with it.
func (c *arbiterCall) conclude(attempt loomengine.ArbiterAttempt) (loomengine.ArbiterEnvelope, error) {
	verdict, found, err := loomengine.ReadArbiterVerdict(loomengine.ArbiterVerdictPath(attempt.Dir))
	if err != nil {
		return c.handOn(attempt, arbiterCauseUnparseable, err.Error())
	}
	if !found {
		return c.handOn(attempt, arbiterCauseMissing, "the arbiter left no verdict")
	}
	if verdict.Verdict == loomengine.ArbiterVerdictResume {
		if problem := c.resumePrecondition(verdict); problem != "" {
			return c.handOn(attempt, arbiterCausePrecondition, problem)
		}
	}
	envelope := loomengine.ArbiterEnvelope{
		Verdict:     verdict.Verdict,
		Reason:      verdict.Reason,
		Question:    verdict.Question,
		Goto:        verdict.Goto,
		Verbs:       verdict.Verbs,
		Attempt:     attempt.Number,
		AttemptDir:  attempt.Dir,
		VerdictPath: loomengine.ArbiterVerdictPath(attempt.Dir),
	}
	if verdict.Verdict == loomengine.ArbiterVerdictResume {
		envelope.Relaunch = fmt.Sprintf("lyx shed step %s --until-stop --arbiter-attempt %d", c.deps.RelaunchRunID, attempt.Number)
	}
	return c.finish(envelope)
}

// resumePrecondition returns what stops a resume from being sound at the call's stop, or empty when nothing does.
func (c *arbiterCall) resumePrecondition(verdict loomengine.ArbiterVerdict) string {
	switch c.status.State {
	case shedengine.StateAwaiting:
		pending, err := c.deps.PendingCirclingDecision(c.status.CurrentProducer)
		if err != nil {
			return fmt.Sprintf("the circling decision could not be read: %v", err)
		}
		if !pending {
			return "a resume at a review escalation needs its circling decision recorded first, and none is pending"
		}
	case shedengine.StateBlocked:
		if verdict.Goto == "" {
			return "a resume at a blocked stop needs a goto target, and the verdict names none"
		}
	}
	if verdict.Goto != "" {
		if err := shedengine.CheckGotoTarget(c.deps.Producers, c.status, verdict.Goto, c.in.RunID); err != nil {
			return err.Error()
		}
	}
	return ""
}

// handOn finishes the call with a hand-on the verb made itself, naming cause.
func (c *arbiterCall) handOn(attempt loomengine.ArbiterAttempt, cause, detail string) (loomengine.ArbiterEnvelope, error) {
	return c.finish(loomengine.ArbiterEnvelope{
		Verdict:     loomengine.ArbiterVerdictHandOn,
		Reason:      detail,
		Question:    fmt.Sprintf("The arbiter could not settle this stop (%s); decide it yourself and report the choice in the stop report.", cause),
		Cause:       cause,
		Attempt:     attempt.Number,
		AttemptDir:  attempt.Dir,
		VerdictPath: loomengine.ArbiterVerdictPath(attempt.Dir),
	})
}

// finish removes the in-flight marker and saves envelope in its attempt directory.
func (c *arbiterCall) finish(envelope loomengine.ArbiterEnvelope) (loomengine.ArbiterEnvelope, error) {
	if err := loomengine.RemoveArbiterMarker(c.deps.MarkerPath); err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	if err := loomengine.WriteArbiterEnvelope(loomengine.ArbiterEnvelopePath(envelope.AttemptDir), envelope); err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	return envelope, nil
}

// runnerArbiterShuttle adapts the shuttle runner to arbiterShuttle.
type runnerArbiterShuttle struct {
	runner *shuttleengine.Runner
}

// Start launches spec through the runner and returns the started run.
func (s runnerArbiterShuttle) Start(spec shuttleengine.Spec) (arbiterRun, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// AttachIfLive waits on a live run matching spec's output files.
func (s runnerArbiterShuttle) AttachIfLive(spec shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	return s.runner.AttachIfLive(spec)
}

// StopStrand ends the strand guid names through shuttle's stop verb.
func (s runnerArbiterShuttle) StopStrand(guid string) error {
	return s.runner.StopStrand(guid)
}

// arbiterCmd builds the `arbiter` subcommand.
func (c *loomCLI) arbiterCmd() *cobra.Command {
	var envelopePath, reportPath string
	cmd := &cobra.Command{
		Use:         "arbiter <run-id>",
		Short:       "settle a stop the run's driver cannot, through a cold arbiter agent",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `arbiter is run only by the run's driver, at a stop it cannot settle. It spawns
a cold arbiter agent, waits for its verdict file and prints the validated verdict:
verdict (resume or hand-on), reason, question, goto, verbs, cause (empty for the
agent's own verdict), attempt, attempt_dir, verdict_path and, on a resume only,
the relaunch line the driver runs next.

A re-run for a stop an arbiter already judged spawns nothing: it returns that
verdict, or waits on the arbiter still running. Each refusal names its way
forward, the hand-back to the run's parent.

Example:
  lyx loom arbiter <run-id> --envelope <stop-envelope> --report <stop-report>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			deps, err := c.arbiterDepsFor()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), arbiterVerb(out, deps, arbiterInput{RunID: args[0], EnvelopePath: envelopePath, ReportPath: reportPath}))
			return nil
		},
	}
	cmd.Flags().StringVar(&envelopePath, "envelope", "", "the stop envelope the driver saved")
	cmd.Flags().StringVar(&reportPath, "report", "", "the driver's stop or fork report, when there is one")
	_ = cmd.MarkFlagRequired("envelope")
	return cmd
}

// arbiterDepsFor assembles the verb's seams for the run the pre-run armed.
// Every closure reads on demand and opens nothing at build time.
func (c *loomCLI) arbiterDepsFor() (arbiterDeps, error) {
	settings, err := loomengine.ResolveArbiter(c.cfg, c.registry)
	if err != nil {
		return arbiterDeps{}, err
	}
	location := c.location
	runID := shedrun.ResolveRunID(location, c.runID)
	reviewsDir := loomengine.LoomReviewsDir(location)
	bouncerRunDir := func(producer string) (string, bool, error) {
		subdir, isBouncer, err := c.bouncerSubdir(producer)
		if err != nil || !isBouncer {
			return "", false, err
		}
		return filepath.Join(reviewsDir, subdir), true, nil
	}
	return arbiterDeps{
		ReadStatus:  circlingStatusReader(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath),
		Settings:    settings,
		ArbiterDir:  shedrun.ArbiterDir(location, c.runID),
		AttemptDir:  func(number int) string { return shedrun.ArbiterAttemptDir(location, c.runID, number) },
		MarkerPath:  shedrun.ArbiterMarker(location, c.runID),
		Producers:   c.routing.Producers,
		StencilsDir: c.runDeps.Geom.StencilsDir,
		Stop:        arbiterstop.NewReaders(location),
		PendingCirclingDecision: func(producer string) (bool, error) {
			runDir, isBouncer, err := bouncerRunDir(producer)
			if err != nil || !isBouncer {
				return false, err
			}
			return shedadapters.PendingCirclingDecision(runDir)
		},
		Prompt: arbiterPromptInput{
			Slug:               seedSlug(location.WorktreeName),
			Worktree:           location.WorktreePath(),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
			SupportLogPath:     loomengine.DiscussionSupportLog(location),
		},
		Shuttle:       runnerArbiterShuttle{runner: c.runner},
		Now:           time.Now,
		RelaunchRunID: runID,
	}, nil
}
