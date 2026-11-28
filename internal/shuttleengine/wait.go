// wait.go implements Run.Wait, the poll loop that reads a run's events.jsonl and classifies its terminal outcome:
// done, died, timeout or api-error.
// It probes the startup window for a trust-dialog dismissal or a fast-failing dead pane,
// and it runs the done-outcome cleanup: strand removal and run directory deletion.
// It also hosts awaitStartup, the startup probe that the start method in run.go calls before a run handle exists.
// It also hosts abandonStartup, the teardown for a provider that never became ready or never took its first input.
// Wait and awaitStartup are the only two places in the run loop that sleep.
// Both sleep through the clock seam defined here, which lets tests replay a whole poll sequence instantly.
// A pane that goes not-live is classified done, not died, when every output file already exists:
// a crash, a kill or an exit counts, since the file contract can be satisfied an instant before the process disappears.
// That can race ahead of the run's own Stop hook.
// died is reserved for a strand that reed still tracks and still holds a pane for, whose pane is not alive;
// the two ways reed's bookkeeping can go away instead are mechanism failures, not classifications.
// errStrandNotTracked covers a strand reed does not track.
// errStrandPaneBindingCleared covers a strand whose pane binding reed cleared.
// The exception is a handle whose own Stop removed the strand:
// its Wait ends OutcomeDied, which finalize records as stopped unless the files show the run done.
// A turn end without every output file never ends the run: Wait logs the hold and keeps polling the same agent.
// The run ends only through the other exits: OutcomeDone, OutcomeDied, a liveness mechanism failure or OutcomeTimeout.
// OutcomeAPIError ends it only for a spec with APIErrorRetries, or for a turn end the engine marks ModelRejected.
// The hold is logged at Info, so it reaches the durable trace sink in internal/logger.
// It does not reach the detached driver's own log, which captures stderr and so keeps only Warn and above.
// The trace sink is .lym/logs/trace-<stamp>-<trace>-<pid>.log, unconditionally enabled at Info.
// An operator who asks whether a run is waiting for input or is wedged reads the trace sink.
// A permission or input dialog that an autonomous run's pane shows past a bound is reported to the parent:
// once per dialog, through the same notifier. The rules live in dialog.go.
// A hold never extends run.deadline.
// A held run is bounded by its caller's own Spec.Timeout (run_timeout_min only where that is zero).
// The liveness check still classifies a dead pane while the run is held.
// While an output file is missing, a turn end that leaves a fork or an awaited shell outstanding is a wait.
// It is never a held stop, and it expires into no hold or notice.
// The wait is bounded only by the run deadline and the liveness check:
// a shell that never ends ends the run OutcomeTimeout, and lym reaps no shell.
// A turn end whose only outstanding work is shells the session does not await is no work product's wait.
// A gated run with an output file missing holds it at once.
// A gated run with every output file present does not count the run Done while such a shell runs.
// The shell may hold a commit or a gate run whose result the session never read.
// The gate tells the agent, once per shell, to wait for the shell and read its result.
// Once every such shell has been outstanding for the shell wait bound, the turn end counts Done through the gate.
// An ungated run with an output file missing holds once every such shell has been outstanding for the shell wait bound.
// The hold notifies the parent once.
// A gated run's waiting turn end on a fork or an awaited shell is never an arrival:
// it waits until the task's completion starts the next turn, bounded by the run deadline and the liveness check.
// At the deadline, classifyDeadlineExpiry ends the run OutcomeDone when the output files exist.
// finalize then evaluates the gate once more over the tree as it stands.
// Every shell outstanding when the run ends, by whatever exit, is recorded in Result.EndedShells.
//
// The events-tick Done branch splits in two, on whether run.gate is empty:
// An ungated run's Done finalizes without consulting the gate.
// A gated Done instead evaluates the gate's entries through run.evaluateGate().
// When an entry failed with its own budget remaining, it sends a one-line re-prompt naming the findings file.
// The run keeps polling rather than finalizing. This is the bounded re-prompt loop.
// Each re-prompt actually sent counts as an attempt.
// A gated Done is also the writer's turn boundary, and a PassOnCap entry may answer pending there:
// the loop sends the entry's carried text at a boundary only, and keeps polling.
// While the writer is idle with no new arrival, the loop re-evaluates at most once per gateRecheckEvery.
// A verdict recorded meanwhile is therefore read without a new arrival.
// Both paths run through handleGatedBoundary.
// The run deadline and liveness checks keep running throughout, and the wait never extends them.
// For an engine with the idle reading and session signals, a boundary is confirmed idle before the gate sends:
// a turn start left unmatched by a later turn end clears the boundary.
// The loop then keeps polling and reads the pane every tick.
// The boundary is restored when the engine reports the turn interrupted, or when a session start follows it;
// the pane reading idle for turnStartIdleOverride restores it too, unless the engine reads that turn in flight.
// Every gate send goes through sendWithin, so its idle wait and submit window end by the run deadline.
// A re-prompt that fails busy or unlanded spends no attempt and is re-sent on a later tick, bounded by the run deadline.
// Held maxNoInputBoxReprompts times in a row, a pane with no input box ends the loop with the attempts spent so far.
// The loop thus ends before a picker that shuttle cannot dismiss holds the run until its deadline.
// A gated run off the boundary also asks each entry's read-only Expired probe once per gateRecheckEvery.
// That covers a run mid-turn, or attached with no boundary seen.
// The first entry whose wait bound has run out finalizes the run done.
// The bound ends the wait even when the writer never ends its turn.
// That finalize call is a positive outcome, so it needs no allOutputFilesExist check.
// The other finalize call sites in this file attempt no Send:
// the mechanism-failure exits through finishedDespiteMechanismFailure, the liveness-tick exit and the deadline exit.
// Each is reached because the session is gone, the clock ran out, or the bookkeeping broke.
// The finalize evaluation reports Attempts as it stands.
//
// # Completion Signal Invariant
//
// Any code path in this package that finalizes a negative answer to "did this run finish":
// it must first consult allOutputFilesExist over the run's OutputFiles.
// These negative answers include OutcomeDied, OutcomeTimeout, OutcomeAPIError and runOutcomeStopped records.
// So do a mechanism-failure error and a verdictRespawnEligible.
// The check runs directly or through one of the helpers that own it:
// classifyDeadlineExpiry, finishedDespiteMechanismFailure (both here), or soleFinishedCandidate in attach.go.
// The startup step, awaitStartup and abandonStartup, follows the same rule through the same primitives:
// its retry-cap and tick-cap exits call allOutputFilesExist directly;
// its not-ready answer comes through checkLivenessTick and classifyStartupWindow, which already consult it;
// its run-deadline exit goes through classifyDeadlineExpiry.
//
// The rule rests on two different questions, and only one of them is the one asked.
// A deadline running out, reed not tracking a strand, or a stale pane binding answers whether something went wrong.
// So do maxStatusRetries consecutive reed.Status errors and maxEventsReadRetries unreadable reads of the events file.
// So does a reed.json absent or undecodable at attach time.
// None of them answers whether the run finished.
// The agent's output files are its return value,
// so a run that wrote all of them finished, whatever went wrong afterwards.
// Publishing the first answer as though it were the second records a completed, expensive LLM step as a failure.
// The next resume then archives the finished files and re-runs the step, which is the rework this invariant prevents.
//
// Each such path calls the shared primitive; it does not reimplement it.
// The tripwire test completionsignal_enforcement_test.go guards the rule.
//
// The rule rests on one structural precondition:
// a *Run with an empty OutputFiles exists only for an unawaited session.
// Spec.validate refuses an empty list for every other spec.
// An unawaited run never reaches Wait or Attach, since Runner.StartSession never calls Wait.
// Attach set-matches a persisted record's output files against the caller's spec.
// Every caller passes an awaited spec that names its output files, so an unawaited record's empty list never matches.
// allOutputFilesExist answers false for an empty list, so no negative answer of an unawaited run reads as finished.

package shuttleengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// Clock is a run's time source: Now reads the time deadlines are measured against and Sleep waits between polls.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// realClock is the production clock: real wall-clock time, real sleeping.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// AwakeClock is a Clock that also reads a monotonic awake time, one that stops while the machine sleeps.
// A run whose clock lacks it detects no sleep.
type AwakeClock interface {
	Clock
	// Awake returns the time elapsed on a clock that does not run during a machine sleep.
	Awake() time.Duration
}

// awakeReading returns the awake reading of realClock, whose origin is taken at first use.
var awakeReading = sync.OnceValue(pauserecord.MonotonicAwake)

// Awake returns the time since the process first read it on the monotonic clock.
// Where the platform stops that clock during suspend, the reading excludes time spent asleep.
func (realClock) Awake() time.Duration { return awakeReading()() }

// pollFloor is the shortest poll interval; a configured value below it is floored to it.
const pollFloor = time.Second

// gateRecheckEvery is the minimum spacing between gate evaluations that the wait loop starts with no new arrival.
// Each evaluation re-reads the entries' files; this spacing keeps those reads rare,
// and a verdict read this much later costs nothing.
const gateRecheckEvery = 5 * time.Second

// maxNoInputBoxReprompts bounds the consecutive re-prompt sends a pane showing no input box may hold.
// The gated loop then ends with the attempts spent so far.
// Each held send has already waited out its idle window,
// so the bound ends a wait on a picker shuttle cannot dismiss within a few idle windows, not at the run deadline.
const maxNoInputBoxReprompts = 5

// pollInterval returns the wait loop's tick interval.
// It floors a value below pollFloor, non-positive values included, to pollFloor so the loop cannot busy-spin.
// It floors silently; the config load logs the one warning.
func pollInterval(cfg Config) time.Duration {
	interval := time.Duration(cfg.PollIntervalMS) * time.Millisecond
	if interval < pollFloor {
		return pollFloor
	}
	return interval
}

// dismissalGap returns the spacing of the startup probe's backoff ceiling and of startup-gate dismissals:
// pollInterval(cfg) times LivenessEveryNPolls, with a non-positive multiple counted as 1.
func dismissalGap(cfg Config) time.Duration {
	livenessEvery := cfg.LivenessEveryNPolls
	if livenessEvery <= 0 {
		livenessEvery = 1
	}
	return pollInterval(cfg) * time.Duration(livenessEvery)
}

// maxEventsReadRetries bounds consecutive event-read failures before reporting a mechanism failure.
const maxEventsReadRetries = 3

// maxStatusRetries bounds consecutive liveness-check failures:
// a reed.Status that could not be run,
// a Status that ran but no longer lists this run's strand,
// and a Status that lists it with no pane bound.
const maxStatusRetries = 2

// errStrandNotTracked reports that reed answered the liveness check successfully,
// but its strand table no longer holds this run's guid.
//
// That answer says nothing about the agent.
// reed's bookkeeping is reset under runs whose agents keep working:
// a lym reed down or remove, a lost or rebuilt reed.json, a git clean of .lym,
// or a worktree renamed while the run was in flight.
// Classifying the run as dead would make an unattended caller respawn it.
// That leaves two agents on one worktree, the first unreachable.
var errStrandNotTracked = errors.New(
	"reed's strand table no longer holds this run's strand — reed's bookkeeping was reset under the run " +
		"(a reed remove/down, a lost or rebuilt reed.json, or a worktree renamed while the run was in flight), " +
		`which says nothing about the agent: its process may still be working in its pane. Check "lym reed status"`)

// errStrandPaneBindingCleared reports that reed still lists this run's strand but holds no pane id for it.
//
// The strand is not dead.
// reed clears its pane bindings when the persisted pane generation is not the incarnation now running:
// a restored backup, a copied .lym, or a reed.json older than the session.
// It then reports the strand with an empty pane id, which the liveness lookup answers as not live.
// The agent may still work in a pane reed can no longer address.
// Classifying that as dead would make an unattended caller start a second agent on the worktree.
var errStrandPaneBindingCleared = errors.New(
	"reed still tracks this run's strand but holds no pane id for it — its pane binding was cleared " +
		"as stale (reed does that when the persisted pane generation is not the session incarnation now " +
		"running: a restored backup, a copied .lym, or a reed.json older than the session), " +
		`which says nothing about the agent: its process may still be working in a pane reed can no longer address. Check "lym reed status"`)

// ErrNotStarted reports that a run's provider never became ready inside its startup window.
// It also reports a provider that never took its first input.
// A gated start wraps it after tearing the strand down, when startup ends not ready or a start-time send fails.
var ErrNotStarted = errors.New("shuttle: the provider never became ready")

// ErrAgentDied reports that a run's provider started and then died before it wrote its outputs.
// shuttle never returns it.
// A caller that turns an OutcomeDied result into an error wraps it,
// so a classifier can tell the death from other failures.
var ErrAgentDied = errors.New("shuttle: the agent died before it wrote its outputs")

// Wait blocks until run reaches a terminal outcome.
// It returns an error only for a mechanism failure that leaves no classifiable outcome.
// A turn end without every output file is held, never terminal: Wait logs the hold and keeps polling.
// It terminates on OutcomeDone, OutcomeDied, a liveness mechanism failure or OutcomeTimeout.
// It also terminates on OutcomeAPIError once a spec's APIErrorRetries re-sends of API-error turn ends are spent.
// It terminates at once on a turn end the engine marks ModelRejected.
//
// A result with an error still carries the run's identity: SessionID, StrandGUID and RunDir.
// Its Outcome is empty.
// The caller needs them most then: no cleanup ran,
// so the run directory is still on disk and the strand may still be registered.
// Without them the caller cannot diagnose, resume or tear down the run.
//
// A mechanism failure is never reported as OutcomeDied:
// reed returns the same untyped error for a foreign-session refusal.
// The run may still be alive under another session, and no sentinel tells the two apart.
// Reporting it as OutcomeDied would report a live agent as dead.
// A status without this run's strand, or one that holds it with no pane bound, exits here too.
//
// The exception is a handle Stop marked: its own Stop removed the strand,
// so a status without it ends the run as OutcomeDied.
// finalize records that as stopped unless the files show the run done.
func (run *Run) Wait() (Result, error) {
	cfg := run.runner.cfg
	interval := pollInterval(cfg)
	livenessEvery := cfg.LivenessEveryNPolls
	if livenessEvery <= 0 {
		livenessEvery = 1
	}
	startupTimeout := time.Duration(cfg.StartupTimeoutS) * time.Second
	startupDeadline := run.clock.Now().Add(startupTimeout)

	// started seeds from run.state.Started alone, for a started run and an attached one alike.
	// A handle from Start has Started true, except on the satisfied-file-contract branch, where re-probing is harmless.
	// Liveness reads cannot tell a booted provider from a failed launch or a killed driver.
	// Only a persisted Started fact skips the probe.
	// A probe against a mid-turn pane would misclassify a live agent as OutcomeDied.
	// It could also play the trust-dismiss keys into a live agent's pane when its capture trips a dialog needle.
	// So the startup probe in checkLivenessTick and classifyStartupWindow runs only for runs never marked Started.
	// For an attached run of that kind, the startup window counts from this Wait call's entry.
	// Liveness checks run for every run regardless of Started; only the startup probe depends on it.
	started := run.state.Started
	run.sessionLiveness = sessionLivenessState{}
	eventsFailures := 0
	statusFailures := 0

	// A mark left by a crashed step is gone at the next touch,
	// and every return leaves none behind.
	run.clearWait()
	defer run.endWait()
	run.armHoldGuard()

	// The signal cursor starts at the prompt offset on every path, never at run.offset, which an attach may start later.
	run.shadow = sessionShadow{active: true, cursor: run.state.PromptOffset}
	defer func() { run.shadow.active = false }()

	// The dialog latch reads asks from the larger of the prompt offset and the last dialog-notified offset,
	// so a reattach never re-notifies a dialog it already did.
	run.dialog = dialogLatch{cursor: max(run.state.PromptOffset, run.state.DialogNotifiedOffset)}
	run.apiErrors = apiErrorWatch{cursor: run.state.PromptOffset}

	run.turnSince = run.clock.Now()
	for tick := 1; ; tick++ {
		run.observePause()
		run.deliverPromptOnAttach()
		outcome, held, err := run.pollEventsTick()
		if err == nil && run.turnStartRead {
			run.turnSince = run.clock.Now()
		}
		run.syncShellWait()
		if err == nil && (run.eventsRead || run.turnStartRead) {
			run.endHeldWait()
			run.pendingHold = nil
		}
		if err != nil {
			eventsFailures++
			if eventsFailures >= maxEventsReadRetries {
				// A satisfied file contract outranks this mechanism failure;
				// it outranks every other negative answer in the loop.
				if result, ferr, ok := run.finishedDespiteMechanismFailure(); ok {
					return result, ferr
				}
				return run.identity(), fmt.Errorf("shuttle: events file unreadable after %d attempts: %w", maxEventsReadRetries, err)
			}
		} else {
			eventsFailures = 0
			if held != nil {
				// A turn end without every output file never ends the run.
				// Log it so the durable trace records each one, and keep polling the same agent.
				logger.Info("shuttle: turn end held, output files missing", "strandGUID", run.state.StrandGUID, "offset", held.offset, "lastAssistantMessage", held.message)
				continued := run.continueAPIErrorStall(held)
				// A turn start read past this turn end in the same batch means the next turn is already under way, so no hold shows.
				if !run.turnStartRead {
					run.beginHeldWait(held.offset)
				}
				switch {
				case continued:
				case run.deliverPromptAtHold(held):
					// The agent never got its task; the turn its prompt now starts is the work, so the parent has nothing to answer.
				case run.holdNoticePending() && run.usageLimited(held):
					// The parent cannot act while the account is limited,
					// and a notice sent now lands in its context as stale clutter after the reset.
					logger.Info("shuttle: held turn end stopped on the usage limit; no hold notice", "strandGUID", run.state.StrandGUID, "offset", held.offset)
				case run.holdNoticePending() && run.operatorStartedTurn(held):
					// The operator is talking to the agent in its pane, so the turn end is no stall for the parent.
					logger.Info("shuttle: held turn end of a turn the operator started; no hold notice", "strandGUID", run.state.StrandGUID, "offset", held.offset)
				case run.holdNoticePending() && run.wakesOnBackground(held):
					// The session wakes when its own background work completes, so the parent has nothing to answer.
					logger.Info("shuttle: held turn end left background work running; no hold notice", "strandGUID", run.state.StrandGUID, "offset", held.offset)
				case run.spec.HoldDeferral == nil:
					run.notifyHeld(held)
				case !run.turnStartRead:
					run.pendingHold, run.pendingHoldAt = held, run.clock.Now()
				}
			} else if outcome != "" && (outcome != OutcomeDone || len(run.gate) == 0) {
				// Any other outcome, or a Done with no gate, finalizes at once.
				return run.finalizeAndRemoveDiedWindow(outcome)
			} else if outcome == OutcomeDone {
				// A gated Done marks the writer's turn boundary: per-turn gate state resets here.
				run.endReworkSpans(run.clock.Now())
				run.gateAtBoundary = true
				run.startCleared = false
				run.unsentReprompt = false
				run.noBoxReprompts = 0
				run.boundaryHeld = false
				if run.covered {
					// A pause covers the session: nothing is evaluated or sent until the first uncovered tick.
					run.boundaryHeld = true
				} else if run.boundaryIdle() {
					if result, finished, ferr := run.handleGatedBoundary(); finished {
						return result, ferr
					}
				}
			} else if run.boundaryHeld && !run.covered && run.gateAtBoundary {
				// The pause lifted: evaluate the held arrival as if it had come now.
				run.boundaryHeld = false
				if run.boundaryIdle() {
					if result, finished, ferr := run.handleGatedBoundary(); finished {
						return result, ferr
					}
				}
			} else if !run.covered && (run.gatePending || run.unsentReprompt) && run.gateAtBoundary {
				// No new arrival, an entry is pending or a re-prompt is unsent, and the writer is idle:
				// re-evaluate, so a verdict recorded since is read without waiting for the writer to speak.
				// A pending entry is re-read at most once per gateRecheckEvery;
				// an unsent re-prompt is retried every tick, since that sends a re-prompt and reads no verdict.
				if (run.unsentReprompt || run.gateRecheckDue()) && run.boundaryIdle() {
					if result, finished, ferr := run.handleGatedBoundary(); finished {
						return result, ferr
					}
				}
			} else if !run.covered && run.startCleared && run.startHoldReleased() {
				// The boundary a turn start cleared is restored: evaluate it as on a new turn end, pending or not.
				if result, finished, ferr := run.handleGatedBoundary(); finished {
					return result, ferr
				}
			} else if len(run.gate) != 0 && !run.gateAtBoundary && run.gateRecheckDue() {
				// Nothing arrived and the writer is off the boundary, mid-turn or with no boundary seen since an attach:
				// ask the entries' read-only probes whether a wait bound has run out.
				// The writer's own turn end would otherwise never show that.
				run.lastGateEvalAt = run.clock.Now()
				if name, expired := run.probeExpiry(); expired {
					logger.Info("shuttle: gate: entry wait expired off the boundary, finalizing", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "gate", name)
					run.gateEndCause = fmt.Sprintf("the %s entry's wait ran out", name)
					run.gateVerdict = nil
					return run.finalize(OutcomeDone)
				}
			}
		}

		if result, finished, ferr := run.retryAPIError(); finished {
			return result, ferr
		}
		run.sendShellNudge()
		run.settleDeferredHold()
		run.watchDialog()
		run.logSessionState("", tick%livenessEvery == 0)

		if tick%livenessEvery == 0 {
			livenessOutcome, err := run.checkLivenessTick(&started, startupDeadline, false)
			if err != nil {
				statusFailures++
				if statusFailures >= maxStatusRetries {
					// A satisfied file contract outranks this mechanism failure;
					// it outranks every other negative answer in the loop.
					if result, ferr, ok := run.finishedDespiteMechanismFailure(); ok {
						return result, ferr
					}
					switch {
					case errors.Is(err, errStrandNotTracked):
						return run.identity(), fmt.Errorf("shuttle: reed did not track strand %q on %d consecutive liveness checks: %w", run.state.StrandGUID, maxStatusRetries, err)
					case errors.Is(err, errStrandPaneBindingCleared):
						return run.identity(), fmt.Errorf("shuttle: reed held no pane binding for strand %q on %d consecutive liveness checks: %w", run.state.StrandGUID, maxStatusRetries, err)
					default:
						return run.identity(), fmt.Errorf("shuttle: reed status failed %d times consecutively: %w", maxStatusRetries, err)
					}
				}
			} else {
				statusFailures = 0
				if livenessOutcome != "" {
					return run.finalizeAndRemoveDiedWindow(livenessOutcome)
				}
			}
		}

		if !run.covered && run.clock.Now().After(run.deadline) {
			// The run deadline answers whether the clock ran out, not whether the run finished.
			// A run whose every output file is on disk finished, whatever the clock says,
			// so the expiry is classified rather than taken as OutcomeTimeout.
			return run.finalize(run.classifyDeadlineExpiry(OutcomeTimeout))
		}

		run.clock.Sleep(interval)
	}
}

// observePause reads the pause and machine sleep on each Wait tick, then moves run.deadline by the newly excluded time.
// It feeds the sleep watch when the clock is an AwakeClock,
// and it records the runner's coverage of the run's own session in run.covered.
// The session is judged by the slug of its strand's name, the same slug a send into it is judged by.
// The excluded time is the union of the detected sleeps and the coverage's paused intervals,
// so the deadline counts only awake, unpaused time.
// A runner with no pause geometry never covers the run.
// An unreadable record reads as uncovered, and it logs one Warn per change of error.
func (run *Run) observePause() {
	now := run.clock.Now()
	if awake, ok := run.clock.(AwakeClock); ok {
		run.sleeps.Observe(now, awake.Awake())
	}
	var coverage pauserecord.Coverage
	if geometry := run.runner.pause; geometry.RecordPath != "" {
		var err error
		coverage, err = pauserecord.ReadCoverage(geometry.RecordPath, run.coverageSlug(), now)
		switch {
		case err == nil:
			run.pauseReadError = ""
		case err.Error() != run.pauseReadError:
			run.pauseReadError = err.Error()
			logger.Warn("shuttle: pause record unreadable, reading the run as uncovered", "strandGUID", run.state.StrandGUID, "error", err)
		}
	}
	run.covered = coverage.Covered
	excluded := pauserecord.Excluded(append(run.sleeps.Sleeps(), coverage.Intervals...), run.createdAt, now)
	run.deadline = run.deadline.Add(excluded - run.deadlineExcluded)
	run.deadlineExcluded = excluded
}

// coverageSlug returns the slug the run's own session is judged by.
// It uses the slug of the strand name in the run's record.
// Failing that, it uses the slug of the name reed reports for the strand, and then the runner's geometry slug.
// A lookup that resolves a name is kept, so reed is asked at most until the first success.
func (run *Run) coverageSlug() string {
	if run.sessionSlugKnown {
		return run.sessionSlug
	}
	slug, resolved := run.runner.resolveTargetSlug(run.state.StrandGUID, run.state.StrandName)
	if resolved && run.state.StrandName == "" {
		run.sessionSlug, run.sessionSlugKnown = slug, true
	}
	return slug
}

// gateRecheckDue reports whether gateRecheckEvery has passed since the last gate evaluation.
// It is always true before the first evaluation.
func (run *Run) gateRecheckDue() bool {
	return run.lastGateEvalAt.IsZero() || run.clock.Now().Sub(run.lastGateEvalAt) >= gateRecheckEvery
}

// probeExpiry calls the Expired probe of every entry that has one and is not off, in list order.
// It returns the name of the first entry whose probe reports expired.
// A probe error counts as not expired, so the loop keeps polling under its deadline and liveness checks;
// it warns once per entry and episode, and an answer after an error logs one Info.
func (run *Run) probeExpiry() (string, bool) {
	for _, entry := range run.gate {
		if entry.Attempts <= 0 || entry.Expired == nil {
			continue
		}
		expired, err := entry.Expired()
		if err != nil {
			run.failures.Fail("expiry:"+entry.Name, "shuttle: gate: expiry probe failed, treating the entry as not expired", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "gate", entry.Name, "error", err)
			continue
		}
		run.failures.Recover("expiry:"+entry.Name, "shuttle: gate: expiry probe answers again", "strandGUID", run.state.StrandGUID, "gate", entry.Name)
		if expired {
			return entry.Name, true
		}
	}
	return "", false
}

// gateEntryError builds the error for a gate entry that breaks the gate contract.
// It uses errors.New rather than fmt.Errorf.
// evaluateGate's returns are infrastructure faults that callers wrap with "shuttle: gate: %w".
// They are not negative verdicts on whether the run finished.
func gateEntryError(name, problem string) error {
	return errors.New("entry " + strconv.Quote(name) + " " + problem)
}

// handleGatedBoundary evaluates the gate list from its first entry at a turn boundary,
// and it reports whether Wait finishes with the returned Result and error.
// It runs on a gated Done arrival, and on a poll tick that brings no new arrival.
// That poll tick runs only while an entry is pending and the writer is idle.
// It also needs gateRecheckEvery to have passed since the last evaluation.
// Run.Interrupt is not used here: the gate fires at a turn boundary, when no turn is in progress to interrupt.
// It sets no new deadline and never extends run.deadline: the loop runs under the deadline Start set.
// A timeout mid-wait still reaches classifyDeadlineExpiry, which classifies OutcomeDone when the files are present,
// and the gate then runs one final time through finalize.
//
// A pass finalizes done, and so does a failure whose budget is spent.
// A terminal failure (GateResult.Terminal) also finalizes done, whatever the entry's failure count.
// It sends no re-prompt.
// A failure with budget remaining re-prompts and keeps polling.
// A re-prompt send that fails with ErrSessionBusy, ErrSubmissionNotLanded or ErrPaused changes neither count.
// It clears the memo and keeps the writer at the boundary with the re-prompt marked unsent,
// so a later tick re-sends it until the run deadline.
// When maxNoInputBoxReprompts such failures follow each other from a pane with no input box, the loop finalizes done.
// The attempts spent so far are kept.
// Any other re-prompt send failure ends the loop.
// A pending result sends its Send text when non-empty and keeps polling.
// A failed pending send warns once per entry and episode, naming the entry, the error and the closure's way-forward.
// It logs Debug while it keeps failing and Info when it is delivered.
// It leaves the entry pending and the writer at the boundary, and never ends the loop.
// The memo is cleared after a pending result, so the next evaluation and any later finalize read the closures afresh.
func (run *Run) handleGatedBoundary() (Result, bool, error) {
	run.lastGateEvalAt = run.clock.Now()
	verdict, gerr := run.evaluateGate(false)
	if gerr != nil {
		return run.identity(), true, fmt.Errorf("shuttle: gate: %w", gerr)
	}
	if run.gatePending {
		run.gateVerdict = nil
		if run.gatePendingSend == "" {
			return Result{}, false, nil
		}
		pendingKey := "pending:" + run.gate[run.gatePendingAt].Name
		if serr := run.sendWithin(run.gatePendingSend); serr != nil {
			run.failures.Fail(pendingKey, "shuttle: gate: pending send failed, entry stays pending", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "gate", run.gate[run.gatePendingAt].Name, "error", serr, "wayForward", run.gatePendingWayForward)
			return Result{}, false, nil
		}
		run.failures.Recover(pendingKey, "shuttle: gate: pending send delivered again", "strandGUID", run.state.StrandGUID, "gate", run.gate[run.gatePendingAt].Name)
		run.gateAtBoundary = false
		run.boundaryHeld = false
		return Result{}, false, nil
	}
	failed := run.gateFailedAt
	if failed < 0 || run.gateTerminal || run.gateFails[failed] >= run.gate[failed].Attempts {
		// No failing entry, a terminal failure, or a spent budget: finalize reads the verdict evaluateGate just stored.
		// It does not re-validate the verdict.
		result, ferr := run.finalize(OutcomeDone)
		return result, true, ferr
	}
	// An entry failed with budget remaining: re-prompt the agent and keep polling.
	reprompt := gateRepromptText(run.gateFindingsPath)
	if render := run.gate[failed].Reprompt; render != nil {
		reprompt = render(run.gateFindingsPath)
	}
	if serr := run.sendWithin(reprompt); serr != nil {
		if errors.Is(serr, ErrSessionBusy) || errors.Is(serr, ErrSubmissionNotLanded) || errors.Is(serr, ErrPaused) {
			var busy *SessionBusyError
			if errors.As(serr, &busy) && busy.NoInputBox {
				run.noBoxReprompts++
			} else {
				run.noBoxReprompts = 0
			}
			if run.noBoxReprompts >= maxNoInputBoxReprompts {
				logger.Warn("shuttle: gate: re-prompt held by a pane with no input box past the bound, ending the loop with the attempts spent so far", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "gate", run.gate[failed].Name, "holds", run.noBoxReprompts, "error", serr)
				run.gateEndCause = fmt.Sprintf("its pane showed no input box through %d re-prompt sends", run.noBoxReprompts)
				result, ferr := run.finalize(OutcomeDone)
				return result, true, ferr
			}
			// No attempt was spent: keep the writer at the boundary and re-send on a later tick.
			// The send clears its own text where it can.
			// An engine that judges idle by its input box fails each re-send busy while the send leaves that box occupied.
			// The loop then ends at the run deadline.
			// A held send is an ordinary retry iteration: Info when it first holds, Debug while it stays held.
			logHeld := logger.Debug
			if !run.unsentReprompt {
				logHeld = logger.Info
			}
			run.unsentReprompt = true
			logHeld("shuttle: gate: re-prompt not delivered, retrying on a later tick without spending an attempt", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "gate", run.gate[failed].Name, "error", serr)
			run.gateVerdict = nil
			return Result{}, false, nil
		}
		logger.Warn("shuttle: gate: re-prompt send failed, ending the loop with the attempts spent so far", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "error", serr)
		run.gateEndCause = fmt.Sprintf("its re-prompt could not be sent: %v", serr)
		result, ferr := run.finalize(OutcomeDone)
		return result, true, ferr
	}
	run.unsentReprompt = false
	run.noBoxReprompts = 0
	run.gateAtBoundary = false
	run.boundaryHeld = false
	run.gateFails[failed]++
	run.gateSent[failed]++
	run.gateReworkSince[failed] = run.clock.Now()
	for i, entry := range verdict.Entries {
		if entry.State == GateEntryPassed {
			run.gateFails[i] = 0
		}
	}
	// The run directory is deleted on the Done cleanup every exhausted gate takes.
	// So the findings text, not just its path, goes into this Warn line.
	// The Warn is the durable record of why the gate failed.
	// The file was written by evaluateGate, so a read failure here is unexpected, and the loop goes on.
	findingsText, rerr := os.ReadFile(run.gateFindingsPath)
	if rerr != nil {
		findingsText = []byte(fmt.Sprintf("<unreadable: %v>", rerr))
	}
	logger.Warn("shuttle: gate: re-prompting after a failed attempt", "strandGUID", run.state.StrandGUID, "gate", run.gate[failed].Name, "attempt", run.gateFails[failed], "budget", run.gate[failed].Attempts, "findings", string(findingsText))
	// Clear the memo so the next arrival re-validates rather than reading this arrival's stale verdict.
	run.gateVerdict = nil
	return Result{}, false, nil
}

// startupTickCap returns the maximum number of checkLivenessTick calls that awaitStartup's loop makes.
// The interval is the startup probe's base interval, which is also its shortest sleep.
// The cap is the ceiling of startupTimeout/interval, plus 1, plus maxStatusRetries.
//
// Each tick runs a real tmux process through reed, for its status and its pane capture.
// The count is the attempt bound that the Live-Substrate Spawn Observability retry clause requires.
// It also ends the loop when the clock never advances.
// The maxStatusRetries slack keeps tolerated status errors from using up the ticks the window needs.
//
// A negative startupTimeout counts as 0 before the division, so the cap is never below 1 + maxStatusRetries.
func startupTickCap(startupTimeout, interval time.Duration) int {
	if startupTimeout < 0 {
		startupTimeout = 0
	}
	ticks := int(math.Ceil(float64(startupTimeout) / float64(interval)))
	return ticks + 1 + maxStatusRetries
}

// awaitStartup runs the startup probe that Wait runs, on its own, before a *Run handle is issued.
// The probe runs checkLivenessTick's liveness check and a pane capture.
// It then applies the engine's startup classification and its trust-dismiss sequence for a recognized one-time gate.
// The loop runs until the provider reaches StartupReady or the window closes.
//
// Its answer is that the provider's input TUI is on screen, with any recognized one-time gate dismissed.
// It never says that the provider read its prompt.
// A ready-then-idle session still reads as ready, and that residual is left to the caller's outer watch budget.
//
// It returns (Result{}, nil) when the handle may be issued.
// That holds when StartupReady was reached, or when the run's file contract is already satisfied.
// On StartupReady, checkLivenessTick persists Started to run.json, so a later Attach skips the startup probe.
// It returns (result, err) when the pane died, the startup window closed, or the run's own deadline arrived first.
// The error wraps ErrNotStarted, and the Outcome is died or timeout.
// Each of these cases tears down through abandonStartup first.
// The same teardown and a died Result follow when checkLivenessTick fails maxStatusRetries times in a row.
// That holds only while the file contract is unsatisfied,
// and the error then wraps ErrNotStarted beside the mechanism failure's own sentinel.
//
// The window is the shorter of startup_timeout_s and the time left until run.deadline, floored at 0.
// A run.Timeout shorter than startup_timeout_s still expires on schedule, as OutcomeTimeout rather than OutcomeDied.
// A caller can then tell a working run from one that never started.
// That holds when the run's own deadline, not the startup window, is what ran out.
// A startup_timeout_s of 0 has no floor: the probe answers on its first tick unless the TUI is already ready.
//
// Every negative answer honors the Completion Signal Invariant.
// The not-ready answer comes only from checkLivenessTick and classifyStartupWindow.
// Both already consult allOutputFilesExist.
// The run-deadline exit goes through classifyDeadlineExpiry.
// The retry-cap and tick-cap exits consult allOutputFilesExist directly.
//
// The tick cap (startupTickCap) bounds the loop by count as well as by the deadline.
//
// Probe cadence: awaitStartup calls checkLivenessTick once per sleep.
// The base is pollInterval(cfg) and the ceiling is dismissalGap(cfg).
// The first sleep is the base.
// After each later probe, the sleep returns to the base if the pane's capture differs from the previous probe's.
// Otherwise it doubles, up to the ceiling.
// A probe whose capture failed leaves the capture unchanged, so it backs off.
// A ready provider shows within the first one or two probes.
// This is because Claude's boot redraws the pane and resets the backoff.
// Gate keys are spaced by the dismissal gap, not by the probe cadence,
// so a capture taken before the provider redraws after a dismissal never sends a second key into the next gate.
//
// Manual live-substrate verification recipe: docs/reference/claude-trust-dialog-repro.md.
func (run *Run) awaitStartup() (Result, error) {
	cfg := run.runner.cfg
	base := pollInterval(cfg)
	ceiling := dismissalGap(cfg)
	startupTimeout := time.Duration(cfg.StartupTimeoutS) * time.Second
	startupDeadline := run.clock.Now().Add(startupTimeout)

	window := startupTimeout
	if untilDeadline := run.deadline.Sub(run.clock.Now()); untilDeadline < window {
		window = untilDeadline
	}
	if window < 0 {
		window = 0
	}
	maxTicks := startupTickCap(window, base)

	sleep := base
	previousCapture := ""
	started := false
	run.sessionLiveness = sessionLivenessState{}
	statusFailures := 0
	for tick := 0; tick < maxTicks; tick++ {
		outcome, err := run.checkLivenessTick(&started, startupDeadline, true)
		if err != nil {
			statusFailures++
			if statusFailures >= maxStatusRetries {
				if allOutputFilesExist(run.spec.OutputFiles) {
					return Result{}, nil
				}
				switch {
				case errors.Is(err, errStrandNotTracked):
					return run.abandonStartup(OutcomeDied, fmt.Errorf("shuttle: startup: reed did not track strand %q on %d consecutive liveness checks (run dir %s): %w", run.state.StrandGUID, maxStatusRetries, run.runDir, err))
				case errors.Is(err, errStrandPaneBindingCleared):
					return run.abandonStartup(OutcomeDied, fmt.Errorf("shuttle: startup: reed held no pane binding for strand %q on %d consecutive liveness checks (run dir %s): %w", run.state.StrandGUID, maxStatusRetries, run.runDir, err))
				default:
					return run.abandonStartup(OutcomeDied, fmt.Errorf("shuttle: startup: reed status failed %d times consecutively for strand %q (run dir %s): %w", maxStatusRetries, run.state.StrandGUID, run.runDir, err))
				}
			}
		} else {
			statusFailures = 0
			if started || outcome == OutcomeDone {
				return Result{}, nil
			}
			if outcome == OutcomeDied {
				return run.abandonStartup(OutcomeDied, nil)
			}
		}

		if run.clock.Now().After(run.deadline) {
			if run.classifyDeadlineExpiry(OutcomeTimeout) == OutcomeDone {
				return Result{}, nil
			}
			return run.abandonStartup(OutcomeTimeout, nil)
		}

		if tick > 0 {
			if run.lastStartupCapture != previousCapture {
				sleep = base
			} else {
				sleep = min(2*sleep, ceiling)
			}
		}
		previousCapture = run.lastStartupCapture
		run.clock.Sleep(sleep)
	}

	if allOutputFilesExist(run.spec.OutputFiles) {
		return Result{}, nil
	}
	return run.abandonStartup(OutcomeDied, nil)
}

// abandonStartup tears down a start whose provider never became ready or never took its first input.
// It finalizes the run's Outcome and removes the strand, keeping the run directory and last pane capture.
// cause is nil when the provider never reached StartupReady inside its window.
// Otherwise cause is the error that ended the start, and the returned error wraps it beside ErrNotStarted.
//
// In order, it saves any capture that checkLivenessTick recorded to startupCaptureFileName.
// A write failure is a Warn, and the returned error then says no capture was saved.
// It calls run.finalize(outcome), which persists RunState.Outcome and logs "run finished".
// It skips the gate and cleanup, since outcome is never OutcomeDone here.
// It removes the strand, trying up to startupRemoveAttempts times with a doubling run-clock wait.
// Spec.KeepPane does not apply here.
// It governs retention of a completed run's pane, and a provider that never came up leaves no pane worth keeping.
// It leaves the run directory in place, so the capture file, prompt.md, settings.json and events.jsonl stay findable.
// The Warn names the run dir, strand guid and outcome, as the Live-Substrate Spawn Observability invariant requires.
//
// The returned error wraps ErrNotStarted.
// It names the run dir, the strand guid, the outcome, whether the capture was saved, and whether the strand was removed.
// When removal failed, the error carries reed's own error text and an operator remedy:
// a strand that abandonStartup could not remove is the one residual an operator must clear by hand.
//
// When cause is nil and the run never started, the error also carries a reason.
// The reason is a missing session-start line, when the last startup probe recorded one for holding back a ready pane.
// The error ends with the last startup capture's excerpt under three conditions.
// The cause is nil.
// The provider never reached StartupReady (run.state.Started is false).
// A startup capture was recorded.
// The excerpt is the capture's last lines and, for a capture longer than one tail, its first lines before them.
// Other teardowns leave the excerpt out.
// Their cause already carries a screen or a mechanism failure.
// A pane that died after it was ready has the ready screen as its last capture.
func (run *Run) abandonStartup(outcome Outcome, cause error) (Result, error) {
	captureNote := "no pane capture was saved"
	if run.lastStartupCapture != "" {
		capturePath := filepath.Join(run.runDir, startupCaptureFileName)
		if err := os.WriteFile(capturePath, []byte(run.lastStartupCapture), 0o644); err != nil {
			logger.Warn("shuttle: startup: save last pane capture failed (non-fatal)", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "error", err)
		} else {
			captureNote = fmt.Sprintf("the last pane capture was saved to %s", startupCaptureFileName)
		}
	}

	result, _ := run.finalize(outcome)

	removeNote := "the strand was removed"
	if rerr := run.removeStartupStrand(); rerr != nil {
		logger.Warn("shuttle: startup: remove strand after not-ready teardown failed", "strandGUID", run.state.StrandGUID, "attempts", startupRemoveAttempts, "error", rerr)
		removeNote = fmt.Sprintf("the strand could NOT be removed after %d attempts (%v); remove it by hand (\"lym reed status\" / \"lym reed remove\")", startupRemoveAttempts, rerr)
	} else {
		run.startupStrandRemoved = true
	}

	format := "shuttle: start: %w — run dir %s, strand %q, outcome %q; %s; %s"
	args := []any{ErrNotStarted, run.runDir, run.state.StrandGUID, string(outcome), captureNote, removeNote}
	if cause == nil {
		logger.Warn("shuttle: provider never became ready; strand torn down", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "outcome", string(outcome))
	} else {
		logger.Warn("shuttle: start failed before the provider took its first input; strand torn down", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "outcome", string(outcome), "cause", cause)
		format += ": %w"
		args = append(args, cause)
	}
	if cause == nil && !run.state.Started && run.startupHookReason != "" {
		format += "; %s"
		args = append(args, run.startupHookReason)
	}
	if cause == nil && !run.state.Started && run.lastStartupCapture != "" {
		format += ". %s"
		args = append(args, startupCaptureExcerpt(run.lastStartupCapture))
	}

	return result, fmt.Errorf(format, args...)
}

const (
	// startupRemoveAttempts is how many times the startup teardown tries to remove the run's strand in all.
	startupRemoveAttempts = 3
	// startupRemoveFirstDelay is the run-clock wait before the teardown's first removal retry; each later wait doubles it.
	startupRemoveFirstDelay = time.Second
)

// removeStartupStrand removes the run's strand.
// While the removal fails it tries again after a doubling wait on the run clock, up to startupRemoveAttempts tries.
// It returns nil once a removal succeeds, and the last removal's error otherwise.
func (run *Run) removeStartupStrand() error {
	delay := startupRemoveFirstDelay
	for attempt := 1; ; attempt++ {
		_, err := run.runner.reed.RemoveStrand(run.state.StrandGUID, false)
		if err == nil || attempt == startupRemoveAttempts {
			return err
		}
		logger.Warn("shuttle: startup: remove strand failed, retrying", "strandGUID", run.state.StrandGUID, "attempt", attempt, "delay", delay, "error", err)
		run.clock.Sleep(delay)
		delay *= 2
	}
}

// startupCaptureExcerpt returns the pane lines the not-ready error ends with, each block under its own heading.
// A capture whose non-blank lines fit in paneTailLines is shown whole as the last lines.
// A longer one shows its first lines, up to paneTailLines of them and none the tail repeats.
// It then shows its last paneTailLines lines.
// The first lines keep the first error line of a launch that scrolled away.
func startupCaptureExcerpt(capture string) string {
	lines := nonBlankLines(capture)
	if len(lines) <= paneTailLines {
		return "The pane's last lines:\n" + strings.Join(lines, "\n")
	}
	head := lines[:min(paneTailLines, len(lines)-paneTailLines)]
	return "The pane's first lines:\n" + strings.Join(head, "\n") + "\nThe pane's last lines:\n" + strings.Join(lines[len(lines)-paneTailLines:], "\n")
}

// pollEventsTick reads the events.jsonl bytes appended since run.offset and classifies them by the batch's last event.
// run.offset advances past the bytes only after ParseEvents succeeds.
// A parse failure leaves them unconsumed.
// The next tick then re-reads and re-classifies them.
// Discarding the batch could lose the run's only qualifying event.
// A batch that holds more than one event is classified by its most recent one.
// Every consumed byte still counts once parsing succeeds, so no earlier event of the batch is reprocessed.
// When every output file exists the run is done, whatever the last event's Kind is.
// Otherwise the turn end is held, whatever its Kind:
// an EventStop or an EventAsk with output files missing returns a held turn end.
// It carries the event's message and the offset just past its line, and the caller keeps polling the same agent.
// The Kind only selects the source of Message inside ParseEvents; it does not change this branch.
// An EventWaiting is the one Kind that is not held at once: the session waits on its own background work.
// The tick then returns the waiting turn end's expiry answer.
// That answer is nothing while the work is outstanding and the files are missing.
// An ungated run with an empty outstanding list and every output file present is done at once.
// When an output file is missing, a gated run whose outstanding list holds only non-awaited shells is held at once.
// With every output file present, it keeps waiting when a turn start past the waiting turn end is already read.
// That turn start means the next turn is under way.
// Otherwise it returns the expiry's answer, and queues the shell nudge only while that answer is nothing.
// While a shell is inside the bound, the expiry answers nothing.
// Once every shell has been outstanding for the bound, it answers done, so the gate runs as at any turn end.
// A turn start read past a recorded waiting turn end clears it.
// So no expiry counts the run done in a turn already under way.
// A fork or an awaited shell keeps the turn waiting however long it runs.
// Only the run's own deadline and the liveness check bound that wait, and lym reaps no shell.
// A hold never extends run.deadline, so a held run is bounded by its caller's own deadline and by the liveness check.
// Returns outcome == "" and a nil held turn end when there is nothing new to classify yet.
func (run *Run) pollEventsTick() (Outcome, *heldTurnEnd, error) {
	run.eventsRead = false
	run.turnStartRead = false
	startOffset := run.offset
	data, newOffset, err := readEventsFrom(run.state.EventsPath, startOffset)
	if err != nil {
		return "", nil, err
	}
	if len(data) == 0 {
		return run.expiredTurnEnd()
	}

	events, err := run.engine.ParseEvents(data)
	if err != nil {
		return "", nil, err
	}
	run.offset = newOffset
	guardArmed := run.holdGuard
	turnStarts := turnStartOffsets(run.engine, data, startOffset)
	if len(turnStarts) > 0 {
		run.holdGuard = false
		run.promptTurnStarted = true
	}
	// The reference is the held turn end on show; a held turn end this tick returns replaces it below.
	run.turnStartRead = run.wait.heldOffset > 0 && turnStartAfter(turnStarts, run.wait.heldOffset)
	if run.waitingTurnEnd != nil && turnStartAfter(turnStarts, run.waitingTurnEnd.offset) {
		// The next turn is under way, so the recorded waiting turn end no longer expires.
		// Its tasks stay in countedTasks for the run's record.
		if len(run.waitingTasks) > 0 {
			run.countedTasks = run.waitingTasks
		}
		run.waitingTasks, run.waitingTurnEnd = nil, nil
	}
	if len(events) == 0 {
		return run.expiredTurnEnd()
	}

	run.eventsRead = true
	run.countedTasks = nil
	last := events[len(events)-1]
	if last.Kind == EventWaiting {
		run.recordWaiting(last)
		run.waitingTurnEnd = &heldTurnEnd{message: last.Message, offset: offsetPastEvent(data, startOffset, newOffset, last), raw: last.Raw, background: len(last.Outstanding) > 0}
		if len(run.waitingTasks) == 0 && len(run.gate) == 0 && allOutputFilesExist(run.spec.OutputFiles) {
			return OutcomeDone, nil, nil
		}
		if len(run.gate) != 0 && run.unawaitedShellsOnly(run.waitingTasks) {
			held := run.waitingTurnEnd
			if allOutputFilesExist(run.spec.OutputFiles) {
				if turnStartAfter(turnStarts, held.offset) {
					// The next turn is already under way, so the shell is neither named nor waited on.
					run.countedTasks, run.waitingTasks, run.waitingTurnEnd = run.waitingTasks, nil, nil
					return "", nil, nil
				}
				// The shell may hold a commit or a gate run whose result the session never read, so Done waits on it up to the bound.
				// A turn end that counts Done here is not nudged: the line would claim the run is not done.
				outcome, _, err := run.expiredTurnEnd()
				if outcome == "" {
					run.queueShellNudge()
				}
				return outcome, nil, err
			}
			// A shell the session does not await is no work product, so the turn end is held as a plain one.
			run.countedTasks, run.waitingTasks = run.waitingTasks, nil
			run.turnStartRead = turnStartAfter(turnStarts, held.offset)
			return "", held, nil
		}
		return run.expiredTurnEnd()
	}
	run.waitingTasks = nil
	run.waitingTurnEnd = nil
	if allOutputFilesExist(run.spec.OutputFiles) {
		return OutcomeDone, nil, nil
	}
	held := &heldTurnEnd{message: last.Message, offset: offsetPastEvent(data, startOffset, newOffset, last), raw: last.Raw}
	if guardArmed && !slices.ContainsFunc(turnStarts, func(start int64) bool { return start < held.offset }) {
		logger.Warn("shuttle: turn end before the prompt's turn start; not held", "strandGUID", run.state.StrandGUID, "offset", held.offset, "lastAssistantMessage", held.message)
		return "", nil, nil
	}
	run.turnStartRead = turnStartAfter(turnStarts, held.offset)
	return "", held, nil
}

// turnStartAfter reports whether any of the turn-start offsets lies past offset.
func turnStartAfter(turnStarts []int64, offset int64) bool {
	return slices.ContainsFunc(turnStarts, func(start int64) bool { return start > offset })
}

// armHoldGuard sets the hold guard for a wait from the events file as it stands.
// A turn start before the prompt offset proves the provider writes turn starts for typed turns, so the guard arms;
// a turn start from the prompt offset up to the run's read offset lifts it at once;
// a wait attached past the prompt's turn start sees that turn start.
// An engine that does not parse session signals and a zero prompt offset leave the guard off.
// So do a file with no turn start before the prompt offset and an unreadable file.
func (run *Run) armHoldGuard() {
	run.holdGuard, run.promptTurnStarted = false, false
	promptOffset := run.state.PromptOffset
	if promptOffset <= 0 {
		return
	}
	data, _, err := readEventsFrom(run.state.EventsPath, 0)
	if err != nil {
		return
	}
	promptOffset = min(promptOffset, int64(len(data)))
	if len(turnStartOffsets(run.engine, data[:promptOffset], 0)) == 0 {
		return
	}
	run.holdGuard = true
	if end := min(max(run.offset, promptOffset), int64(len(data))); len(turnStartOffsets(run.engine, data[promptOffset:end], promptOffset)) > 0 {
		run.holdGuard, run.promptTurnStarted = false, true
	}
}

// turnStartOffsets returns the events-file offset just past the line of each turn start the engine reads from data.
// The offsets are in file order.
// data is the slice of the events file that begins at startOffset.
// An engine that does not parse session signals reads none.
func turnStartOffsets(engine Engine, data []byte, startOffset int64) []int64 {
	starts := positionedTurnStarts(engine, data, startOffset)
	offsets := make([]int64, len(starts))
	for i, start := range starts {
		offsets[i] = start.end
	}
	return offsets
}

// positionedTurnStart is one turn start read from an events file, with the offset just past its line.
type positionedTurnStart struct {
	signal SessionSignal
	end    int64
}

// positionedTurnStarts returns each turn start the engine reads from data, in file order.
// Each carries the events-file offset just past its line.
// data is the slice of the events file that begins at startOffset.
// An engine that does not parse session signals reads none.
func positionedTurnStarts(engine Engine, data []byte, startOffset int64) []positionedTurnStart {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return nil
	}
	signals, _ := parser.ParseSessionSignals(data)
	var starts []positionedTurnStart
	searchFrom := 0
	for _, signal := range signals {
		at := bytes.Index(data[searchFrom:], signal.Raw)
		if at < 0 || len(signal.Raw) == 0 {
			continue
		}
		end := searchFrom + at + len(signal.Raw)
		if newline := bytes.IndexByte(data[end:], '\n'); newline >= 0 {
			end += newline + 1
		}
		searchFrom = end
		if signal.Kind == SessionSignalTurnStart {
			starts = append(starts, positionedTurnStart{signal: signal, end: startOffset + int64(end)})
		}
	}
	return starts
}

// heldTurnEnd is a plain Stop or a live ask that left the run's output files missing.
// The run is held rather than ended.
type heldTurnEnd struct {
	// message is the agent's last message at the turn end.
	message string
	// offset is the events-file byte offset just past the turn end's line.
	offset int64
	// raw is the turn end's events-file line.
	raw []byte
	// background is true when the turn end left background work of the session that is still running.
	background bool
}

// offsetPastEvent returns the events-file offset just past the line ev was parsed from.
// It takes the batch's data, its start offset and the offset past the whole batch.
// An event whose Raw line is not found in the batch resolves to the batch's end.
func offsetPastEvent(data []byte, startOffset, batchEnd int64, ev Event) int64 {
	at := bytes.LastIndex(data, ev.Raw)
	if at < 0 || len(ev.Raw) == 0 {
		return batchEnd
	}
	end := at + len(ev.Raw)
	if newline := bytes.IndexByte(data[end:], '\n'); newline >= 0 {
		end += newline + 1
	}
	return startOffset + int64(end)
}

// recordWaiting keeps a waiting turn end's outstanding list and logs what it waits on.
// It records the current time as first sight of each shell id not seen before.
func (run *Run) recordWaiting(ev Event) {
	run.waitingTasks = ev.Outstanding
	tasks := make([]string, 0, len(ev.Outstanding))
	for _, task := range ev.Outstanding {
		tasks = append(tasks, fmt.Sprintf("kind=%s id=%s label=%q signal=%s", task.Kind, task.ID, task.Label, task.Signal))
	}
	logger.Info("shuttle: turn end waiting on background work", "strandGUID", run.state.StrandGUID, "outstanding", strings.Join(tasks, "; "))
	now := run.clock.Now()
	for _, task := range ev.Outstanding {
		if task.Kind != BackgroundShell {
			continue
		}
		if _, seen := run.shellFirstSeen[task.ID]; !seen {
			if run.shellFirstSeen == nil {
				run.shellFirstSeen = map[string]time.Time{}
			}
			run.shellFirstSeen[task.ID] = now
		}
	}
}

// shellWaitBound returns how long a shell may stay outstanding before it is logged and shown in the wait marker.
func (run *Run) shellWaitBound() time.Duration {
	return ShellWaitBound(run.runner.cfg)
}

// ShellWaitBound returns how long a background shell of either signal may stay outstanding before the wait logs it.
// The wait shows the shell in the wait marker after the same bound.
// The bound is cfg.BackgroundShellWaitMin minutes, or the template default when that value is not positive.
// It is also the bound after which a waiting turn end whose every outstanding task is a non-awaited shell expires:
// The expiry turns an ungated turn end with an output file missing into a held turn end.
// It turns a gated one with every output file present into a Done.
func ShellWaitBound(cfg Config) time.Duration {
	minutes := cfg.BackgroundShellWaitMin
	if minutes <= 0 {
		minutes = defaultBackgroundShellWaitMin
	}
	return time.Duration(minutes) * time.Minute
}

// defaultBackgroundShellWaitMin is the template.yaml default shell wait bound in minutes.
const defaultBackgroundShellWaitMin = 10

// awaitedShell reports whether the shell's label starts with one of the spec's awaited prefixes.
func (run *Run) awaitedShell(task BackgroundTask) bool {
	return awaitedLabel(task.Label, run.spec.AwaitedShellPrefixes)
}

// awaitedLabel reports whether label starts with one of prefixes.
func awaitedLabel(label string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(label, prefix) {
			return true
		}
	}
	return false
}

// expiredTurnEnd decides the recorded waiting turn end:
// it counts it as a Done at once, expires it into a held turn end or a Done, or keeps it waiting.
// An ungated run counts it at once whenever every output file exists, whatever is outstanding.
// Expiry needs every outstanding task to be a non-awaited shell, each outstanding for the bound:
// an ungated run with an output file missing turns it into a held turn end,
// and a gated run with every output file present turns it into a Done.
// A fork or an awaited shell outstanding keeps it waiting however long it runs.
// Any other gated turn end is never an arrival:
// output files may be left over from an earlier arrival while the session works on in the background.
// It waits until the task's completion starts the next turn, the run's deadline or the liveness check.
// A shell outstanding past the bound is logged once per shell.
// Counting or expiring a turn end clears the waiting list and keeps its tasks in countedTasks for the run's record;
// a turn start read past the recorded turn end clears it the same way.
// Returns outcome == "" and a nil held turn end while the turn keeps waiting.
func (run *Run) expiredTurnEnd() (Outcome, *heldTurnEnd, error) {
	if len(run.waitingTasks) == 0 {
		return "", nil, nil
	}
	now := run.clock.Now()
	bound := run.shellWaitBound()
	for _, task := range run.waitingTasks {
		if task.Kind == BackgroundShell {
			run.logPayloadShellPastBound(task, now.Sub(run.shellFirstSeen[task.ID]), bound)
		}
	}
	filesExist := allOutputFilesExist(run.spec.OutputFiles)
	if len(run.gate) != 0 {
		if !filesExist || !run.unawaitedShellsPastBound(now) {
			return "", nil, nil
		}
		run.countedTasks, run.waitingTasks = run.waitingTasks, nil
		return OutcomeDone, nil, nil
	}
	if !filesExist {
		if run.waitingTurnEnd == nil || !run.unawaitedShellsPastBound(now) {
			return "", nil, nil
		}
		held := run.waitingTurnEnd
		run.countedTasks, run.waitingTasks = run.waitingTasks, nil
		return "", held, nil
	}
	run.countedTasks = run.waitingTasks
	run.waitingTasks = nil
	return OutcomeDone, nil, nil
}

// unawaitedShellsPastBound reports whether every outstanding task of the waiting turn end is a non-awaited shell.
// Each such shell must have been outstanding for at least the shell wait bound at now.
func (run *Run) unawaitedShellsPastBound(now time.Time) bool {
	if !run.unawaitedShellsOnly(run.waitingTasks) {
		return false
	}
	bound := run.shellWaitBound()
	for _, task := range run.waitingTasks {
		if now.Sub(run.shellFirstSeen[task.ID]) < bound {
			return false
		}
	}
	return true
}

// queueShellNudge queues the line naming each shell of the waiting turn end that no sent nudge has named.
// It queues nothing when every shell was named before,
// so an agent that ends its turn again over the same shell is not told twice.
// A shell counts as named once its nudge line has been sent.
func (run *Run) queueShellNudge() {
	var ids, labels []string
	for _, task := range run.waitingTasks {
		if run.nudgedShells[task.ID] {
			continue
		}
		ids = append(ids, task.ID)
		labels = append(labels, delimitedAgentText(task.Label))
	}
	if len(labels) > 0 {
		run.shellNudge, run.shellNudgeIDs = shellNudgeText(labels), ids
	}
}

// shellNudgeText returns the one line that tells an agent its turn ended while background shells still run.
// The agent never read those shells' results,
// and the line names each shell by its delimited label.
func shellNudgeText(labels []string) string {
	return "[lym] Your turn ended while these background shells still run: " + strings.Join(labels, ", ") +
		". The run does not count as done while one runs: wait for each to finish, read its result and act on it before you end your turn."
}

// sendShellNudge types the queued shell nudge into the run's pane through the gated send, then clears it.
// It marks the shells the nudge named once the send succeeds.
// While a pause covers the run, it drops the line unsent.
// A failed send is logged at Info and not retried in place, since the shell's completion still starts the session's next turn.
// A dropped or failed line leaves its shells unnamed, so the next waiting turn end over them queues it again.
func (run *Run) sendShellNudge() {
	text, ids := run.shellNudge, run.shellNudgeIDs
	if text == "" {
		return
	}
	run.shellNudge, run.shellNudgeIDs = "", nil
	if run.covered {
		logger.Info("shuttle: shell nudge dropped under a pause", "strandGUID", run.state.StrandGUID)
		return
	}
	if err := run.sendWithin(text); err != nil {
		logger.Info("shuttle: shell nudge not sent", "strandGUID", run.state.StrandGUID, "error", err)
		return
	}
	if run.nudgedShells == nil {
		run.nudgedShells = map[string]bool{}
	}
	for _, id := range ids {
		run.nudgedShells[id] = true
	}
	logger.Info("shuttle: told the agent to read the background shells its turn end left running", "strandGUID", run.state.StrandGUID)
}

// unawaitedShellsOnly reports whether tasks is non-empty and holds only background shells that are not awaited.
func (run *Run) unawaitedShellsOnly(tasks []BackgroundTask) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if task.Kind != BackgroundShell || run.awaitedShell(task) {
			return false
		}
	}
	return true
}

// logPayloadShellPastBound logs at Warn, once per shell id, that a background shell has been outstanding for at least bound.
// It names the run and the shell's label and changes no decision.
func (run *Run) logPayloadShellPastBound(task BackgroundTask, outstanding, bound time.Duration) {
	if outstanding < bound || run.payloadShellLogged[task.ID] {
		return
	}
	if run.payloadShellLogged == nil {
		run.payloadShellLogged = map[string]bool{}
	}
	run.payloadShellLogged[task.ID] = true
	logger.Warn("shuttle: background shell past wait min", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "signal", task.Signal, "shell", task.Label)
}

// logEndedShells logs at Info the shells outstanding at the run's end.
// With the strand removed, it says the removal ended them.
// On every other end, it says the run ended with them outstanding and the strand is left to its caller.
// lym kills no shell itself, so a shell detached from the pane's process tree survives the removal.
func (run *Run) logEndedShells(ended []EndedShell, strandRemoved bool) {
	if len(ended) == 0 {
		return
	}
	labels := make([]string, 0, len(ended))
	for _, shell := range ended {
		labels = append(labels, fmt.Sprintf("%s (%s, outstanding %s)", shell.Label, shell.Signal, shell.Outstanding.Round(time.Second)))
	}
	message := "shuttle: run ended with background shells outstanding; the strand is left to its caller"
	if strandRemoved {
		message = "shuttle: strand removal ended the background shells it could"
	}
	logger.Info(message, "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "shells", strings.Join(labels, "; "))
}

// endedShells lists the shells outstanding when the run ends.
// It reads them from the recorded waiting turn end, or from the turn end counted at once when none is recorded.
// Each entry's duration runs from the shell's first sighting to now.
func (run *Run) endedShells(now time.Time) []EndedShell {
	tasks := run.waitingTasks
	if len(tasks) == 0 {
		tasks = run.countedTasks
	}
	var ended []EndedShell
	for _, task := range tasks {
		if task.Kind != BackgroundShell {
			continue
		}
		ended = append(ended, EndedShell{Label: task.Label, ID: task.ID, Signal: task.Signal, Outstanding: now.Sub(run.shellFirstSeen[task.ID])})
	}
	return ended
}

// readEventsFrom reads path from byte offset onward, returning the bytes up to the last complete line.
// A partial trailing line is left unconsumed for the next tick.
func readEventsFrom(path string, offset int64) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, offset, nil
		}
		return nil, offset, fmt.Errorf("open events file: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seek events file: %w", err)
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, offset, fmt.Errorf("read events file: %w", err)
	}
	if len(buf) == 0 {
		return nil, offset, nil
	}

	lastNL := bytes.LastIndexByte(buf, '\n')
	if lastNL == -1 {
		return nil, offset, nil
	}

	consumed := buf[:lastNL+1]
	return consumed, offset + int64(len(consumed)), nil
}

// strandStatusByGUID returns the strand reed reports for guid, and whether reed reported one at all.
// The second result separates a strand reed tracks with no live pane from one it does not track at all.
// A bare liveness flag cannot express that difference.
func strandStatusByGUID(strands []reedengine.StrandStatus, guid string) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if s.GUID == guid {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// allOutputFilesExist reports whether files is non-empty and every entry in it exists on disk.
// An empty list is an unawaited run's, which has no file contract to satisfy, so it never reads as finished.
func allOutputFilesExist(files []string) bool {
	if len(files) == 0 {
		return false
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return false
		}
	}
	return true
}

// checkLivenessTick checks strand liveness and probes the pane during startup.
// It returns a non-nil error when the status call cannot run.
// It also returns one for two status answers that describe reed's own bookkeeping rather than the agent's fate:
// a status that no longer lists this run's strand, and one that lists it with no pane bound.
// Both are failures of the mechanism, not classifications of the agent.
// A satisfied file contract wins over every negative answer.
// The agent's output files are its return value.
// Their existence classifies OutcomeDone whether the pane died or reed stopped tracking or addressing it.
// For a stopped handle, a status that no longer lists the strand answers OutcomeDied rather than an error.
// The stop removed the strand, so its going is the stop, not reed's bookkeeping.
// The run is recorded as stopped.
//
// A startup gate is dismissed at most once per dismissal gap on the run's clock.
// A probe inside the gap that still sees a gate plays nothing and does not send the dismissal keys.
// Any play that sent at least one input starts the gap, whether it failed partway or not.
// An empty sequence, or a first input that failed, does not start it.
// The throttle only removes key presses and never adds one.
// The startup deadline and the tick cap still end a gate that never clears.
//
// Once the strand reads live, every tick checks whether the session is gone.
// This check runs before the started short-circuit, so startup is covered too.
// A gone session returns OutcomeDone when every output file exists and OutcomeDied otherwise, as a dead pane does.
// The wait's own startup probe already runs at least one dismissal gap apart, so the throttle never holds a dismissal there.
//
// For a record with `RecordsSessionStarts`, a ready pane marks the run started only once the events file also holds a session-start line.
// Until then the tick stays not started, records the reason on the run for the teardown, and answers as a still-booting pane does.
// Gate detection and dismissal run as for any run.
// A session-start line an earlier launch of the same run left satisfies the line half; the pane half still gates it.
//
// With eventsProof set, a record with `RecordsSessionStarts` also starts once the events file holds a session start followed by a turn start.
// That check runs before the pane capture; a false answer falls through to the capture path, so a recognized gate is still dismissed.
// Only the launch's own startup wait sets eventsProof.
// Its events file is created fresh for the launch, so only this launch's hooks satisfy it.
// The wait's probe of a not-yet-started run may read an earlier incarnation's lines, so it leaves eventsProof unset.
func (run *Run) checkLivenessTick(started *bool, startupDeadline time.Time, eventsProof bool) (Outcome, error) {
	status, err := run.runner.reed.Status()
	if err != nil {
		return "", fmt.Errorf("reed status: %w", err)
	}

	strand, tracked := strandStatusByGUID(status.Strands, run.state.StrandGUID)
	if !tracked {
		if allOutputFilesExist(run.spec.OutputFiles) {
			return OutcomeDone, nil
		}
		if run.markedStopped() {
			return OutcomeDied, nil
		}
		return "", errStrandNotTracked
	}
	if !strand.Live {
		if allOutputFilesExist(run.spec.OutputFiles) {
			return OutcomeDone, nil
		}
		// Reed reports a strand it holds no pane id for as not live, as it does for a strand whose pane died.
		// Those are different facts, and only the second is a dead run.
		// An anchor:hidden strand is the one case where an empty pane id is normal.
		// It keeps the not-live classification.
		// Reed realizes a pane for every other anchor, so an empty pane id there is a cleared binding.
		if strand.PaneID == "" && run.spec.Display.Anchor != render.AnchorHidden {
			return "", errStrandPaneBindingCleared
		}
		return OutcomeDied, nil
	}

	if run.sessionGone() {
		if allOutputFilesExist(run.spec.OutputFiles) {
			return OutcomeDone, nil
		}
		return OutcomeDied, nil
	}

	if *started {
		return "", nil
	}

	if eventsProof && run.state.RecordsSessionStarts && turnStartedAfterSessionStart(run.engine, run.state.EventsPath) {
		*started = true
		run.markStarted()
		return "", nil
	}

	capture, err := run.runner.reed.CapturePane(run.state.StrandGUID)
	if err != nil {
		run.failures.Fail("startup capture", "shuttle: capture pane during startup probe (non-fatal, retrying)", "strandGUID", run.state.StrandGUID, "error", err)
		// A pane that cannot be captured tells us nothing about whether the provider came up,
		// so it stays inside the startup window and expires with it.
		return run.classifyStartupWindow(startupDeadline), nil
	}
	run.failures.Recover("startup capture", "shuttle: startup probe capture works again", "strandGUID", run.state.StrandGUID)
	// Recorded on every successful capture, not just a ready one: abandonStartup's not-ready teardown
	// saves whatever the last capture showed as a diagnosis artifact, so a still-pending or
	// still-gated pane's last screen is on disk exactly as a ready one's would be.
	run.lastStartupCapture = capture

	switch run.engine.Startup(capture) {
	case StartupReady:
		if run.state.RecordsSessionStarts {
			recorded, readErr := sessionStartRecorded(run.engine, run.state.EventsPath)
			switch {
			case readErr != nil:
				run.startupHookReason = fmt.Sprintf("the events file %s could not be read for a session-start line: %v", run.state.EventsPath, readErr)
			case !recorded:
				run.startupHookReason = fmt.Sprintf("no session-start line in the events file %s", run.state.EventsPath)
			default:
				run.startupHookReason = ""
			}
			if run.startupHookReason != "" {
				break
			}
		}
		*started = true
		run.markStarted()
		return "", nil
	case StartupTrustPrompt:
		now := run.clock.Now()
		if run.dismissed && now.Sub(run.dismissedAt) < dismissalGap(run.runner.cfg) {
			logger.Debug("shuttle: startup gate dismissal held for the dismissal gap", "strandGUID", run.state.StrandGUID)
			break
		}
		inputs := run.engine.TrustDismissSequence(capture)
		sent, err := playInputsSent(run.runner.reed, run.state.StrandGUID, inputs)
		if sent > 0 {
			run.dismissed, run.dismissedAt = true, now
		}
		if err != nil {
			run.failures.Fail("startup dismiss", "shuttle: dismiss trust prompt (non-fatal)", "strandGUID", run.state.StrandGUID, "error", err)
		} else if len(inputs) > 0 {
			run.failures.Recover("startup dismiss", "shuttle: trust prompt dismissal sends again", "strandGUID", run.state.StrandGUID)
			// Logged at Info, not Debug: a dismissal is a lifecycle event, not a routine polling probe.
			logger.Info("shuttle: dismissed startup gate", "strandGUID", run.state.StrandGUID, "inputs", len(inputs))
		}
	}
	// StartupPending and an uncleared trust prompt both mean still not ready.
	// Both share the one deadline that governs the whole startup window.
	return run.classifyStartupWindow(startupDeadline), nil
}

// markStarted persists that the provider is past its startup gates.
// It persists at once, since this is the one fact a killed driver must not lose.
// A later attach uses it to tell a booted, mid-turn pane from one whose launch already failed.
// The save is best-effort: a failure costs a later attach one extra startup probe, never this run's correctness.
func (run *Run) markStarted() {
	run.recordMu.Lock()
	run.state.Started = true
	err := run.saveState()
	run.recordMu.Unlock()
	if err != nil {
		logger.Warn("shuttle: persist run started failed (non-fatal)", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "error", err)
	}
}

// sessionLivenessState is the state a wait's liveness ticks carry from one tick to the next.
type sessionLivenessState struct {
	// sizeTaken reports that firstTickSize holds the events file's size at the wait's first liveness tick.
	sizeTaken     bool
	firstTickSize int64
	// readAlive holds the session ids a liveness tick of this wait read alive.
	readAlive map[string]bool
}

// sessionGone reports whether this tick proves the provider's process is gone behind a live pane.
// An engine that does not both parse session signals and probe session liveness always reports false.
// Otherwise it reads the registry once for the newest session id the events file names,
// or the record's session id when the file names none.
// It reports true when either of two proofs holds.
// The first is a process-ending session end in the events file past the size taken at the wait's first call;
// the process must also not be alive.
// The second is a dead reading for an id this wait read alive at an earlier tick.
// An alive reading records the id and reports false.
// So does an unproven reading: a dead one for an id this wait never read alive,
// or a session end at or before the first-call size.
// An unreadable events file counts no session end.
func (run *Run) sessionGone() bool {
	parser, parses := run.engine.(SessionSignalParser)
	prober, probes := run.engine.(SessionProber)
	if !parses || !probes {
		return false
	}
	state := &run.sessionLiveness
	if !state.sizeTaken {
		state.firstTickSize, state.sizeTaken = eventsSize(run.state.EventsPath), true
	}

	var pastFirstTick []SessionSignal
	sessionID := run.state.SessionID
	if data, err := os.ReadFile(run.state.EventsPath); err == nil {
		signals, _ := parser.ParseSessionSignals(data)
		for i := len(signals) - 1; i >= 0; i-- {
			if signals[i].SessionID != "" {
				sessionID = signals[i].SessionID
				break
			}
		}
		if state.firstTickSize <= int64(len(data)) {
			pastFirstTick, _ = parser.ParseSessionSignals(data[state.firstTickSize:])
		}
	}

	liveness := prober.ProcessLiveness(sessionID)
	if liveness == LivenessAlive {
		if state.readAlive == nil {
			state.readAlive = map[string]bool{}
		}
		state.readAlive[sessionID] = true
		return false
	}
	proof := ""
	switch {
	case len(pastFirstTick) > 0 && pastFirstTick[len(pastFirstTick)-1].Kind == SessionSignalSessionEnd && pastFirstTick[len(pastFirstTick)-1].EndsProcess:
		proof = "a process-ending session end past the wait's first tick, and the process not alive"
	case liveness == LivenessDead && state.readAlive[sessionID]:
		proof = "the process read alive earlier in this wait is now dead"
	default:
		return false
	}
	logger.Info("shuttle: the provider's process is gone behind a live pane", "strandGUID", run.state.StrandGUID, "sessionID", sessionID, "proof", proof)
	return true
}

// sessionStartRecorded reports whether the events file holds a session-start signal of any source.
// It reads the file from its start.
// An absent file reports false with no error, and an unreadable one returns the read error.
// An engine that does not parse session signals reports false.
func sessionStartRecorded(engine Engine, eventsPath string) (bool, error) {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return false, nil
	}
	data, err := os.ReadFile(eventsPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	signals, _ := parser.ParseSessionSignals(data)
	for _, signal := range signals {
		if signal.Kind == SessionSignalSessionStart {
			return true, nil
		}
	}
	return false, nil
}

// turnStartedAfterSessionStart reports whether the events file holds a turn-start signal after a session-start signal.
// Signals are taken in file order, from the file's start.
// An absent or unreadable file, and an engine that does not parse session signals, report false.
// It reads signals through the engine's parser, never by a provider's hook names.
func turnStartedAfterSessionStart(engine Engine, eventsPath string) bool {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return false
	}
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return false
	}
	signals, _ := parser.ParseSessionSignals(data)
	sessionStarted := false
	for _, signal := range signals {
		switch signal.Kind {
		case SessionSignalSessionStart:
			sessionStarted = true
		case SessionSignalTurnStart:
			if sessionStarted {
				return true
			}
		}
	}
	return false
}

// classifyStartupWindow reports the outcome once startupDeadline has passed.
// On a run whose provider has not reached StartupReady, it returns "" while the window is still open.
//
// The window covers every not-started exit of the liveness check, not only the still-booting one.
// Enforcing it on StartupPending alone let two silent paths skip the deadline:
// a trust prompt that never clears, and a pane that fails every capture.
// Such a run ran until the run deadline and was reported as timed out rather than died.
//
// Once the window has expired, the answer is OutcomeDone if the file contract is already satisfied.
// Otherwise it is OutcomeDied.
func (run *Run) classifyStartupWindow(startupDeadline time.Time) Outcome {
	if run.clock.Now().After(startupDeadline) {
		return run.classifyDeadlineExpiry(OutcomeDied)
	}
	return ""
}

// classifyDeadlineExpiry reports the outcome for a deadline that has just expired: OutcomeDone when
// the run's file contract is already satisfied, and expired otherwise.
// A deadline answers whether the clock has run out, not whether the run finished, so the file contract outranks it.
// Without that rule, a run whose agent wrote every output file but left no parseable terminal event was
// classified as died at the startup deadline or as timed out at the run deadline.
func (run *Run) classifyDeadlineExpiry(expired Outcome) Outcome {
	if allOutputFilesExist(run.spec.OutputFiles) {
		return OutcomeDone
	}
	return expired
}

// finishedDespiteMechanismFailure finalizes the run as OutcomeDone if its file contract is satisfied.
// It reports whether it did.
// It runs where a run would otherwise be abandoned with a mechanism-failure error.
// The two exits are: the events file unreadable for maxEventsReadRetries consecutive ticks;
// or reed.Status failing for maxStatusRetries consecutive ticks.
// They describe reed's bookkeeping, not whether the run finished; a satisfied file contract outranks them.
// Without this check, a run could be recorded as a mechanism failure after its agent wrote every output file;
// if reed.json was left corrupt, the next resume would archive the finished files and re-run the step.
// That is the rework the Completion Signal Invariant exists to prevent.
// When the contract is not satisfied, it returns a zero Result and a nil error, with false;
// the caller then surfaces its mechanism error unchanged.
//
// An empty OutputFiles exists only on an unawaited run, which never reaches this loop.
func (run *Run) finishedDespiteMechanismFailure() (Result, error, bool) {
	if allOutputFilesExist(run.spec.OutputFiles) {
		result, err := run.finalize(OutcomeDone)
		return result, err, true
	}
	return Result{}, nil, false
}

// identity returns the run's identifying fields with an empty Outcome.
// It is the Result for a run that reached no classification;
// the caller keeps the handles it needs to diagnose, resume, or tear down the run.
func (run *Run) identity() Result {
	return Result{
		SessionID:  run.state.SessionID,
		StrandGUID: run.state.StrandGUID,
		RunDir:     run.runDir,
		Engine:     run.engineName,
	}
}

// evaluateGate runs run's gate entries and returns the verdict, memoized on run.gateVerdict.
// A stored memo is returned unchanged, so the gate runs once per settling;
// Wait clears the memo before each re-prompt, so the next arrival re-validates.
//
// It returns (nil, nil) when run.gate is empty, an ungated run.
//
// The entries run in list order and stop at the first failure:
// an off entry (Attempts 0) is reported off wherever it sits;
// a PassOnCap entry whose failure count has reached its Attempts is not run and is reported let through;
// every other entry runs its closure and is reported passed, failed, or, for a PassOnCap or MayHold entry, waiting;
// every non-off entry after the stopping entry is reported not reached.
// Passed is true only when every entry that is neither off nor PassOnCap passed at this arrival;
// a pending MayHold entry is never a pass.
// A Terminal flag on a passed or pending result, or on a result from a PassOnCap entry, is a returned gate error;
// so is Pending, or a non-nil Final, on an entry that is neither PassOnCap nor MayHold.
// A Terminal failing result is recorded on run.gateTerminal, and its Findings text is carried on GateOutcome.Reason.
// The method never changes a count; only Wait's re-prompt branch does.
// So a finalize that evaluates afresh after a lost session or a deadline cannot change one.
//
// An entry's closure is Gate, replaced by Forked when the run's record carries fork authorization;
// at a final evaluation it is Final.
//
// A non-nil error from a closure is returned verbatim and stores no memo and changes no count.
// A gate that could not run has found no defect; it is an infrastructure fault, not a GateResult{Passed: false}.
//
// On a failing entry the method writes GateResult.Findings to <run.runDir>/gateFindingsFileName.
// It overwrites any previous arrival's file, so the agent always reads the current complaint.
// It sets GateOutcome.FindingsPath and GateOutcome.Findings when the outcome did not pass.
// A write failure is a returned error, not a failed gate, because findings the agent cannot read cannot fix anything.
// Both error returns are unwrapped, because the finalize and Wait call sites add their own "shuttle: gate: %w" context;
// wrapping here too would double it.
// Its negative-verdict returns use no Errorf:
// the completionsignal_enforcement_test.go tripwire reserves that marker for the call sites that finalize a run.
//
// GateOutcome.Attempts is the sum of the entries' re-prompts sent.
func (run *Run) evaluateGate(final bool) (*GateOutcome, error) {
	if run.gateVerdict != nil {
		return run.gateVerdict, nil
	}
	if len(run.gate) == 0 {
		return nil, nil
	}
	if len(run.gateFails) != len(run.gate) {
		run.gateFails = make([]int, len(run.gate))
		run.gateSent = make([]int, len(run.gate))
		run.gateSpans = make([][]GateSpan, len(run.gate))
		run.gateReworkSince = make([]time.Time, len(run.gate))
		run.gateHeldAt = -1
	}
	// While a held span is open, no entry of the gate records a gate span, so no two spans of one entry overlap.
	heldAtStart := run.gateHeldAt >= 0
	for _, entry := range run.gate {
		if entry.Final != nil && !entry.PassOnCap && !entry.MayHold {
			return nil, gateEntryError(entry.Name, "sets Final but is not pass_on_cap")
		}
	}

	outcome := &GateOutcome{Passed: true, Entries: make([]GateEntryOutcome, len(run.gate))}
	failedAt := -1
	pendingAt := -1
	pendingSend, pendingWayForward := "", ""
	findingsPath, findings := "", ""
	terminal, reason := false, ""
	for i, entry := range run.gate {
		line := GateEntryOutcome{Name: entry.Name, Attempts: run.gateSent[i]}
		outcome.Attempts += run.gateSent[i]
		switch {
		case entry.Attempts <= 0:
			line.State = GateEntryOff
		case failedAt >= 0 || pendingAt >= 0:
			line.State = GateEntryNotReached
			if !entry.PassOnCap {
				outcome.Passed = false
			}
		case entry.PassOnCap && run.gateFails[i] >= entry.Attempts:
			line.State = GateEntryLetThrough
		default:
			closure := entry.Gate
			if entry.Forked != nil && run.state.ForkSubagents {
				closure = entry.Forked
			}
			if final && entry.Final != nil {
				closure = entry.Final
			}
			run.beginGateWait(entry.Name)
			closureStart := run.clock.Now()
			result, err := closure()
			closureEnd := run.clock.Now()
			run.endGateWait()
			if err != nil {
				return nil, err
			}
			if !heldAtStart {
				run.gateSpans[i] = append(run.gateSpans[i], GateSpan{Kind: GateSpanGate, Start: closureStart, End: closureEnd})
			}
			if result.Pending {
				if !entry.PassOnCap && !entry.MayHold {
					return nil, gateEntryError(entry.Name, "returned pending but is not pass_on_cap")
				}
				if result.Terminal {
					return nil, gateEntryError(entry.Name, "returned a terminal pending result")
				}
				if !entry.PassOnCap {
					outcome.Passed = false
				}
				line.State = GateEntryWaiting
				pendingAt = i
				pendingSend = result.Send
				pendingWayForward = result.SendFailedWayForward
				break
			}
			if result.Passed {
				if result.Terminal {
					return nil, gateEntryError(entry.Name, "returned a terminal passed result")
				}
				line.State = GateEntryPassed
				break
			}
			if result.Terminal && entry.PassOnCap {
				return nil, gateEntryError(entry.Name, "returned a terminal result but is pass_on_cap")
			}
			findingsPath = filepath.Join(run.runDir, gateFindingsFileName)
			if writeErr := os.WriteFile(findingsPath, []byte(result.Findings), 0o644); writeErr != nil {
				return nil, writeErr
			}
			findings = result.Findings
			line.State = GateEntryFailed
			failedAt = i
			if !entry.PassOnCap {
				outcome.Passed = false
			}
			if result.Terminal {
				terminal = true
				reason = result.Findings
			}
		}
		outcome.Entries[i] = line
	}
	if !outcome.Passed {
		outcome.FindingsPath = findingsPath
		outcome.Findings = findings
		outcome.Reason = reason
	}

	evaluationEnd := run.clock.Now()
	if run.gateHeldAt >= 0 && run.gateHeldAt != pendingAt {
		run.closeGateSpans(evaluationEnd)
	}
	if pendingAt >= 0 && run.gateHeldAt < 0 {
		run.gateHeldAt = pendingAt
		run.gateHeldSince = evaluationEnd
	}

	run.gateVerdict = outcome
	run.gateFailedAt = failedAt
	run.gateTerminal = terminal
	run.gateFindingsPath = findingsPath
	run.gatePending = pendingAt >= 0
	run.gatePendingAt = pendingAt
	run.gatePendingSend = pendingSend
	run.gatePendingWayForward = pendingWayForward
	return outcome, nil
}

// finalize builds run's terminal Result, persists the classification into the run's RunState.Outcome,
// and performs cleanup for OutcomeDone.
// For fork mode, it audits fork subagents and attaches the result.
//
// finalize is the single gate verdict site.
// On OutcomeDone it evaluates the gate before the Outcome write, the fork audit and cleanup;
// it stamps the verdict onto result.Gate, so no Done escapes ungated.
// An exhausted gate still takes the ordinary Done-path cleanup.
// Every finalize call in Wait reaches this one site; "no Done escapes ungated" then holds by construction.
// classifyStartupWindow and classifyDeadlineExpiry cannot host the gate themselves;
// they return an Outcome rather than a Result.
//
// The Outcome write happens for every terminal outcome, before the fork audit.
// finalize returns early with (result, err) when AuditForks fails.
// A write placed after the audit would leave a run that classified OutcomeDone persisted at "running";
// that is the live-but-idle state the sentinel exists to prevent, when its pane is still alive on a later resume.
// The write is best-effort: a saveRunState failure is logged with logger.Warn, never returned as an error;
// the run already reached its outcome, and failing a successful run over a bookkeeping write would invert the cost.
// The accepted residual is that such a record keeps "running";
// if its pane is also still live and idle, it stays attachable for that one run.
//
// finalize is also the run's teardown observability point.
// The Live-Substrate Spawn Observability invariant requires teardown to be instrumented as the spawn is:
// Start logs "run started" through internal/logger;
// without the logger.Info below, the durable Info+ trace file would show every run beginning and none ending.
// The cleanup failures go to logger.Warn for the same reason:
// a teardown that did not confirm clean is what that level is for.
func (run *Run) finalize(outcome Outcome) (Result, error) {
	run.logSessionState(outcome, true)
	result := Result{
		Outcome:     outcome,
		SessionID:   run.state.SessionID,
		StrandGUID:  run.state.StrandGUID,
		RunDir:      run.runDir,
		Engine:      run.engineName,
		EndedShells: run.endedShells(run.clock.Now()),
		EndedAt:     run.clock.Now(),

		SessionStates: slices.Clone(run.shadow.states),
	}
	if createdAt, err := time.Parse(time.RFC3339, run.state.CreatedAt); err == nil {
		result.StartedAt = createdAt
	}

	if outcome == OutcomeDone {
		// Rework spans still open end before the final evaluation, so its gate span never overlaps them.
		run.endReworkSpans(result.EndedAt)
		gateOutcome, err := run.evaluateGate(true)
		if err != nil {
			return result, fmt.Errorf("shuttle: gate: %w", err)
		}
		if gateOutcome != nil {
			run.closeGateSpans(result.EndedAt)
			gateOutcome = run.withGateSpans(gateOutcome)
			gateOutcome.EndedEarly = run.gateEndedEarly(gateOutcome)
		}
		result.Gate = gateOutcome
	}

	run.recordMu.Lock()
	recorded := string(outcome)
	if run.stopMarked {
		recorded = stopOutcome(recorded, run.spec.OutputFiles)
	}
	run.state.Outcome = recorded
	saveErr := run.saveState()
	run.recordMu.Unlock()
	if saveErr != nil {
		logger.Warn("shuttle: persist run outcome failed (non-fatal)", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "outcome", recorded, "error", saveErr)
	}
	// Stamped before the cleanup below removes the run directory;
	// a reader that finds no running run still sees this one has just ended.
	if err := stampRunEnd(filepath.Dir(run.runDir), run.clock.Now()); err != nil {
		logger.Warn("shuttle: stamp the run end failed (non-fatal)", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "error", err)
	}

	if reader, ok := run.engine.(UsageReader); ok {
		result.Usage = reader.SessionUsage(run.state.SessionID, run.runner.paneCwd)
	}
	// An attached run's spec is rebuilt without its role and model, which its record still holds.
	role, model := run.spec.Role, run.spec.Model
	if role == "" {
		role = run.state.Role
	}
	if model == "" {
		model = run.state.Model
	}
	run.runner.reportSessionEnd(SessionEnd{
		SessionID: result.SessionID,
		Engine:    result.Engine,
		Role:      role,
		Model:     model,
		Worktree:  run.runner.paneCwd,
		StartedAt: result.StartedAt,
		EndedAt:   result.EndedAt,
		Outcome:   outcome,
		Usage:     result.Usage,
	})

	if outcome == OutcomeDone && run.state.ForkSubagents {
		auditor, ok := run.engine.(ForkAuditor)
		if !ok {
			return result, fmt.Errorf("shuttle: run %s: engine %q cannot audit the forks its ForkSubagents record authorizes", run.runDir, run.engineName)
		}
		audit, err := auditor.AuditForks(run.state.SessionID, run.runner.paneCwd)
		if err != nil {
			// The run succeeded and nothing has been cleaned up yet, so the caller gets the whole classified Result.
			// identity() is for the mechanism-failure exits, where no outcome was reached.
			// Here one was, and the identity fields alone would make a run that finished look like one that never classified;
			// this branch skips cleanup either way, so they do not separate the two.
			// ForkAudit stays nil, which means "not audited".
			return result, fmt.Errorf("shuttle: audit forks for session %q: %w", run.state.SessionID, err)
		}
		result.ForkAudit = &audit
	}

	cleaned := outcome == OutcomeDone && !run.spec.KeepPane
	strandRemoved := false
	if cleaned {
		if _, err := run.runner.reed.RemoveStrand(run.state.StrandGUID, false); err != nil {
			logger.Warn("shuttle: cleanup: remove strand failed (non-fatal)", "strandGUID", run.state.StrandGUID, "error", err)
		} else {
			strandRemoved = true
		}
		if err := os.RemoveAll(run.runDir); err != nil {
			logger.Warn("shuttle: cleanup: remove run dir failed (non-fatal)", "runDir", run.runDir, "error", err)
		}
	}

	run.logEndedShells(result.EndedShells, strandRemoved)
	logger.Info("shuttle: run finished", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "outcome", string(outcome), "cleanedUp", cleaned)
	return result, nil
}

// diedCaptureFileName names the run-directory file that holds an ended run's last pane capture.
const diedCaptureFileName = "died-capture.txt"

// gateEndedEarly returns why the run ends while outcome's failing entry still has re-prompts left.
// It returns "" on a pass, a terminal failure and a failure whose entry spent its budget.
// A finalize that no boundary step gave a cause ends on the session's end or the run's deadline.
func (run *Run) gateEndedEarly(outcome *GateOutcome) string {
	failed := run.gateFailedAt
	if outcome.Passed || failed < 0 || run.gateTerminal || run.gateFails[failed] >= run.gate[failed].Attempts {
		return ""
	}
	if run.gateEndCause != "" {
		return run.gateEndCause
	}
	return "the session ended, or the run reached its deadline, before the writer was re-prompted"
}

// finalizeAndRemoveDiedWindow finalizes the run with outcome, then ends the window of a died run's own-window strand.
func (run *Run) finalizeAndRemoveDiedWindow(outcome Outcome) (Result, error) {
	result, err := run.finalize(outcome)
	if result.Outcome == OutcomeDied {
		run.removeEndedOwnWindowStrand()
	}
	return result, err
}

// removeEndedOwnWindowStrand saves a died or api-error run's last pane capture to died-capture.txt in the run directory;
// it then removes the run's strand when the strand is tracked, own-window and present.
// A window 0 strand is left to reed's reconcile.
// A failed capture or write is a Warn, and the removal still runs;
// a failed removal is a Warn naming `lym reed remove` as the way forward.
func (run *Run) removeEndedOwnWindowStrand() {
	guid := run.state.StrandGUID
	reed := run.runner.reed
	status, err := reed.Status()
	if err != nil {
		logger.Warn("shuttle: ended run: probe reed before removing its own-window strand failed (non-fatal)", "strandGUID", guid, "error", err)
		return
	}
	strand, tracked := strandStatusByGUID(status.Strands, guid)
	if !tracked || !strand.OwnWindow || !strand.Present {
		return
	}
	if capture, err := reed.CapturePane(guid); err != nil {
		logger.Warn("shuttle: ended run: capture the own-window pane failed (non-fatal)", "strandGUID", guid, "error", err)
	} else if err := os.WriteFile(filepath.Join(run.runDir, diedCaptureFileName), []byte(capture), 0o644); err != nil {
		logger.Warn("shuttle: ended run: save the last pane capture failed (non-fatal)", "runDir", run.runDir, "strandGUID", guid, "error", err)
	}
	if _, err := reed.RemoveStrand(guid, false); err != nil {
		logger.Warn("shuttle: ended run: remove the own-window strand failed; run \"lym reed remove\" to clear it", "strandGUID", guid, "error", err)
		return
	}
	logger.Info("shuttle: removed an ended run's own-window strand", "runDir", run.runDir, "strandGUID", guid)
}
