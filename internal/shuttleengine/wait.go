// wait.go implements Run.Wait: the poll loop that reads a run's events.jsonl, classifies its
// terminal outcome (done/died/timeout), probes the startup window for a trust-dialog
// dismissal or a fast-failing dead pane, and runs the done-outcome cleanup (strand removal + run
// dir deletion).
// It also hosts awaitStartup, the startup probe run.go's own start method calls before issuing a run
// handle at all, and abandonStartup, start's teardown for a provider that never became ready or never took its first input.
// Wait and awaitStartup are the only two places in the run loop that sleep — both through the clock
// seam defined here, which lets tests replay a whole poll sequence instantly.
// A pane that goes not-live (crashed, killed, or exited) is classified done rather than died when
// every output file already exists — the file contract can be satisfied an instant before the
// process disappears, racing ahead of its own Stop hook.
// died is reserved for a strand reed STILL TRACKS AND STILL HOLDS A PANE FOR, whose pane is not
// alive; the two ways reed's own bookkeeping can go away instead — a strand it no longer tracks
// (errStrandNotTracked) and a strand whose pane binding it cleared (errStrandPaneBindingCleared) —
// are mechanism failures, not classifications.
// A turn end without every output file never ends the run: Wait logs the hold and keeps polling the same agent,
// so every other exit (OutcomeDone, OutcomeDied, a liveness mechanism failure, OutcomeTimeout) is the only way out.
// That observation is logged at Info, so it reaches internal/logger's DURABLE trace sink
// (.lyx/logs/trace-<stamp>-<trace>-<pid>.log, unconditionally enabled at Info) and not the detached
// driver's own log, which captures stderr and therefore only Warn and above. An operator asking "is
// this interview waiting for me, or is it wedged?" reads the trace sink.
// An autonomous run's pane that shows a permission or input dialog past a bound is also reported to the parent, once per dialog, through the same notifier; see dialog.go.
// A hold never extends run.deadline: a held run is bounded by its caller's own Spec.Timeout (run_timeout_min only where that is zero),
// and by the liveness check, which still classifies a dead pane.
// A turn end that leaves background work outstanding while an output file is missing is a wait, never a held stop: a shell of either signal and a fork are live work, so none expires into a hold or a notice.
// It is bounded only by the run's own deadline and the liveness check, so a shell that never ends ends the run OutcomeTimeout, and lyx reaps no shell.
// A gated run's waiting turn end is never an arrival: it waits until the task's completion starts the next turn, bounded by the run deadline and the liveness check.
// At the deadline classifyDeadlineExpiry ends the run OutcomeDone when the output files exist, and finalize evaluates the gate once more over the tree as it stands.
// Every shell outstanding when the run ends, by whatever exit, is recorded in Result.EndedShells.
//
// The events-tick Done branch splits in two, on whether run.gate is empty:
// an ungated run's Done finalizes exactly as before the gate existed, unaware the gate exists at all.
// A gated Done instead evaluates the gate's entries through run.evaluateGate() and, when an entry failed with its own budget remaining, sends a one-line re-prompt naming the findings file and keeps polling rather than finalizing -- the bounded re-prompt loop the "one GateSpec at every hop" and "attempts counts re-prompts actually sent" plan decisions describe.
// A gated Done is also the writer's turn boundary, and a PassOnCap entry may answer pending there:
// the loop sends the entry's carried text (only at a boundary), keeps polling, and re-evaluates with no new arrival while the writer is idle at most once per gateRecheckEvery, so a verdict recorded meanwhile is read without a new arrival.
// Both paths run through handleGatedBoundary, and the run deadline and liveness checks keep running throughout, never extended by the wait.
// For an engine with the idle reading and session signals, a boundary is confirmed idle in fact before the gate sends:
// a turn start left unmatched by a later turn end clears the boundary, the loop keeps polling and reads the pane every tick,
// and the boundary is restored when the engine reports that turn interrupted or the pane has read idle for turnStartIdleOverride.
// Every gate send goes through sendWithin, so its idle wait and submit window end by the run deadline.
// A re-prompt that fails busy or unlanded spends no attempt and is re-sent on a later tick, bounded by the run deadline.
// A gated run off the boundary, mid-turn or attached with no boundary seen, also asks each entry's read-only Expired probe once per gateRecheckEvery;
// the first entry whose wait bound has run out finalizes the run done, so the bound ends the wait even when the writer never ends its turn.
// That finalize call is a positive outcome, so it needs no allOutputFilesExist check.
// The other finalize call sites in this file (the events-unreadable/status-retry mechanism-failure exits via finishedDespiteMechanismFailure, the liveness-tick exit, and the deadline exit) are left untouched:
// each is reached precisely because the session is gone, the clock ran out, or the bookkeeping broke, so no Send is attempted there and finalize's own evaluation reports Attempts as it stands.
//
// # Completion Signal Invariant
//
// Any code path in this package that finalizes a NEGATIVE answer to "did this run finish" -- an
// OutcomeDied, an OutcomeTimeout, a runOutcomeStopped record, a mechanism-failure error, or a verdictRespawnEligible -- must
// first consult allOutputFilesExist over the run's OutputFiles, directly or through one of the three
// helpers that own the check: classifyDeadlineExpiry, finishedDespiteMechanismFailure (both here),
// or soleFinishedCandidate (attach.go). The startup step (awaitStartup, abandonStartup) honours the
// same rule through the same primitives: its retry-cap and tick-cap exits call allOutputFilesExist
// directly, its not-ready answer comes through checkLivenessTick/classifyStartupWindow (which already
// consult it), and its run-deadline exit through classifyDeadlineExpiry.
//
// The rule exists because those are two different questions and only one of them is the one being
// asked. A clock running out, reed no longer tracking a strand, reed's pane binding going stale,
// reed.Status erroring for maxStatusRetries consecutive ticks, the events file staying unreadable
// for maxEventsReadRetries, reed.json being absent or undecodable at attach time -- every one of
// those answers "has something gone wrong", and none of them answers "did this run finish". The
// agent's output files ARE its return value, so a run that wrote all of them finished, whatever went
// wrong afterwards. Publishing the first answer as though it were the second records a completed,
// expensive LLM step as a failure, after which the next resume archives the finished files and
// re-runs it -- precisely the rework the Completion Signal Invariant above exists to
// prevent.
//
// It is named here, rather than left implied by the individual helpers, because it was omitted SIX
// times by six different edits before anyone noticed it was one rule: crucible rounds opus5-high-r4
// (both deadlines), fable5-high-r5 (both retry-exhausted caps), fable5-xhigh-r6 (Attach's
// dispositionCandidate) and opus5-high-r7 (Attach's three reed-state gates) each found the next
// instance by asking "is there another exit shaped like the ones already fixed". Every one was an
// omission to CALL an already-correct, already-shared primitive -- never a divergent
// reimplementation of it -- which is why this is a stated invariant plus a tripwire test
// (completionsignal_enforcement_test.go) rather than a registry in the shape of
// internal/planparser/shape.go's ref-shape ledger. That mechanism needs a closed value enum to build
// a flat map over, and the negative outcomes here live in three different types across two files.
// See PATTERN-completion-signal for the cross-reference a reader who
// checks the PATTERN entries first will find.
//
// The one structural precondition the rule rests on: allOutputFilesExist is vacuously true for an
// empty list, and no *Run with an empty OutputFiles ever exists. Spec.validate refuses one on the
// Start path, and collectAttachCandidates set-matches against a persisted RunState written by a
// validated Start, so the Attach path cannot produce one either.

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

// Awake returns the time since the process first read it on the monotonic clock, which stops during suspend where the platform stops it.
func (realClock) Awake() time.Duration { return awakeReading()() }

// pollFloor is the shortest poll interval; a configured value below it is floored to it.
const pollFloor = time.Second

// gateRecheckEvery is the shortest spacing between two gate evaluations the wait loop starts with no new arrival.
// A verdict read this much later costs nothing, and each evaluation re-reads the entries' files.
const gateRecheckEvery = 5 * time.Second

// pollInterval returns Wait's tick interval, flooring a value below pollFloor (non-positive included) to pollFloor to prevent busy-spinning.
// It floors silently; LoadConfig logs the one warning.
func pollInterval(cfg Config) time.Duration {
	interval := time.Duration(cfg.PollIntervalMS) * time.Millisecond
	if interval < pollFloor {
		return pollFloor
	}
	return interval
}

// dismissalGap returns the spacing of the startup probe's backoff ceiling and of startup-gate dismissals:
// pollInterval(cfg) times LivenessEveryNPolls, the multiple floored to 1 exactly as Run.Wait floors it.
func dismissalGap(cfg Config) time.Duration {
	livenessEvery := cfg.LivenessEveryNPolls
	if livenessEvery <= 0 {
		livenessEvery = 1
	}
	return pollInterval(cfg) * time.Duration(livenessEvery)
}

// maxEventsReadRetries bounds consecutive event-read failures before reporting a mechanism failure.
const maxEventsReadRetries = 3

// maxStatusRetries bounds consecutive liveness-check failures — a reed.Status that could not be run,
// a Status that ran but no longer lists this run's strand, and a Status that lists it with no pane
// bound.
const maxStatusRetries = 2

// errStrandNotTracked reports that reed answered the liveness check successfully but its strand table
// no longer holds this run's guid.
//
// It exists because that answer says nothing about the agent. A strand reed still tracks whose pane is
// not alive IS a dead run; a strand reed does not track at all is reed's own bookkeeping going away
// under a run whose agent very often keeps working — reed's strand table is reset by an ordinary
// `lyx reed down`/`remove`, by a `git clean -xdf` of .lyx (a sanctioned operator action under the
// Durable-vs-Ephemeral State Invariant), by reed's own documented remedy for a corrupt reed.json
// ("delete it by hand to keep the session"), and by a worktree renamed while a run is in flight, which
// leaves this process's told anchor path pointing at a directory reed then re-creates empty.
// Classifying any of those OutcomeDied reports a live agent as gone, and an unattended caller answers
// "died" by respawning — leaving two agents on one worktree, the first unreachable. That is the
// duplicate-agent hazard reed's own foreign-session refusal exists to prevent, one layer up.
// Interrupt/Send already draw exactly this distinction (see requireLiveStrand); this is Wait's half.
var errStrandNotTracked = errors.New(
	"reed's strand table no longer holds this run's strand — reed's bookkeeping was reset under the run " +
		"(a reed remove/down, a lost or rebuilt reed.json, or a worktree renamed while the run was in flight), " +
		`which says nothing about the agent: its process may still be working in its pane. Check "lyx reed status"`)

// errStrandPaneBindingCleared reports that reed answered the liveness check successfully and still
// lists this run's strand, but holds NO pane id for it.
//
// It exists because that is a third answer, and the one the not-live branch below would otherwise
// swallow: a strand reed tracks with no pane bound is not a strand whose pane died, it is a strand
// reed can no longer address at all. Reed clears every pane binding in a state file whose recorded
// pane generation is not the incarnation now running (adoptPaneGenerationLocked — a restored backup,
// a hand-copied .lyx, or simply a reed.json older than the session), and Status then reports the
// strand with an empty PaneID, which its liveness lookup answers false for. Reproduced live in round
// 4 against a real agent: reed logged the clear, shuttle answered ok:true outcome:"died" 4 seconds
// later, and tmux still reported the pane alive with the agent working inside it — after which
// restoring the stamp made the same strand report live:true again on the same pane.
// Classifying that OutcomeDied is the same duplicate-agent hazard errStrandNotTracked exists to
// avoid: an unattended caller reads "died" as "gone, retry" and puts a second agent on a worktree
// whose first one is still working, unreachably.
var errStrandPaneBindingCleared = errors.New(
	"reed still tracks this run's strand but holds no pane id for it — its pane binding was cleared " +
		"as stale (reed does that when the persisted pane generation is not the session incarnation now " +
		"running: a restored backup, a copied .lyx, or a reed.json older than the session), " +
		`which says nothing about the agent: its process may still be working in a pane reed can no longer address. Check "lyx reed status"`)

// ErrNotStarted reports that a run's provider never became ready inside its startup window or never took its first input.
// StartGated wraps it, after tearing the strand down, when awaitStartup's loop resolves not-ready or a start-time send fails —
// see abandonStartup for the teardown and the full error text a caller actually sees.
var ErrNotStarted = errors.New("shuttle: the provider never became ready")

// Wait blocks until run reaches a terminal outcome.
// Error is reserved for mechanism failures that leave no classifiable outcome.
// A turn end without every output file is held, never terminal: Wait logs the hold and keeps polling,
// so it terminates only on OutcomeDone, OutcomeDied, a liveness mechanism failure, or OutcomeTimeout.
//
// An error result still carries the run's IDENTITY — SessionID, StrandGUID, and RunDir — with an
// empty Outcome, because a mechanism failure is precisely when a caller needs them: no cleanup ran,
// so the run directory is still on disk and the strand may still be registered, and without those
// three the caller cannot diagnose, resume, or tear down what it started.
// Reproduced live: tearing the reed session down under an in-flight run returned reed's
// no-session error and, before this, a wholly zero Result.
//
// It is deliberately NOT reclassified as OutcomeDied. reed returns the same untyped error for its
// foreign-session refusal — a renamed worktree or a copied .lyx, where the run may well be alive
// under another session — and reed exposes no sentinel that tells the two apart, so guessing "died"
// would report a live agent as dead.
// The same reasoning binds the two cases where reed does NOT fail: a Status that succeeds without
// this run's strand in it, and one that returns the strand with no pane bound, both exit here rather
// than as OutcomeDied — see errStrandNotTracked and errStrandPaneBindingCleared.
func (run *Run) Wait() (Result, error) {
	cfg := run.runner.cfg
	interval := pollInterval(cfg)
	livenessEvery := cfg.LivenessEveryNPolls
	if livenessEvery <= 0 {
		livenessEvery = 1
	}
	startupTimeout := time.Duration(cfg.StartupTimeoutS) * time.Second
	startupDeadline := run.clock.Now().Add(startupTimeout)

	// started seeds from run.state.Started alone — for a started run and an attached one alike. A
	// handle from Start carries Started: true unless it was issued on the satisfied-file-contract
	// branch, where re-probing is harmless and still classifies OutcomeDone (on an events-tick Done,
	// or at the latest at Wait's own startup-window expiry through classifyDeadlineExpiry). Reed's own
	// liveness reads, which an attached run's reconstruction consults before ever reaching Wait, are
	// strictly stronger evidence than the capture heuristic below for telling a pane APART FROM
	// NOTHING, but they cannot tell a booted, mid-turn provider apart from a pane whose launch command
	// already failed (a live shell sitting at its own prompt after a bad binary path) or whose driver
	// was killed before its own first liveness tick ever ran: reed reports both "live", and the run's
	// own run.json still carries the runOutcomeRunning sentinel in every one of those cases, because
	// nothing ever wrote a terminal Outcome to it. That is why liveness alone is never enough on its
	// own to skip the probe, whether reported by Attach's own reads or by this loop's own
	// checkLivenessTick: only a persisted Started fact does. Started is false for every run.json a
	// pre-this-change binary wrote too, which is the same safe direction as RunState.Outcome's own
	// compat rule: the probe runs one extra time rather than being skipped when it should not have
	// been.
	// The probe in checkLivenessTick/classifyStartupWindow is therefore reached from this seed only
	// for an attached run whose run.json was never marked Started — a killed driver, or a launch whose
	// binary never existed — and Wait still computes its own startupDeadline from its own entry time
	// for that case, since the startup window an attach's own awaitStartup never ran belongs to this
	// Wait call, not to some earlier one. Once Started is true, re-running the probe against a pane
	// that is mid-turn would misclassify a live interview as OutcomeDied one startup_timeout_s into
	// this Wait, or worse, play the trust-dismiss key sequence into a live agent's pane if its capture
	// happens to trip a trust-dialog needle — so a confirmed-ready run, started or attached, skips it.
	// The not-tracked and not-live branches of checkLivenessTick sit above this short-circuit, so every
	// run keeps full liveness coverage regardless of Started — only the startup probe itself is
	// conditional on it.
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

	// The dialog latch reads asks from the larger of the prompt offset and the last dialog-notified offset, so a reattach never re-notifies a dialog it already did.
	run.dialog = dialogLatch{cursor: max(run.state.PromptOffset, run.state.DialogNotifiedOffset)}

	run.turnSince = run.clock.Now()
	for tick := 1; ; tick++ {
		run.observePause()
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
				// A satisfied file contract outranks this mechanism failure exactly as it outranks
				// every other negative answer in this loop -- see finishedDespiteMechanismFailure.
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
				case run.spec.HoldDeferral == nil:
					run.notifyHeld(held)
				case !run.turnStartRead:
					run.pendingHold, run.pendingHoldAt = held, run.clock.Now()
				}
			} else if outcome != "" && (outcome != OutcomeDone || len(run.gate) == 0) {
				// Not a gated Done: finalize exactly as this branch always has.
				return run.finalizeAndRemoveDiedWindow(outcome)
			} else if outcome == OutcomeDone {
				// A gated Done is the writer's turn boundary: let the shared helper judge it.
				run.gateAtBoundary = true
				run.startCleared = false
				run.unsentReprompt = false
				if run.boundaryIdle() {
					if result, finished, ferr := run.handleGatedBoundary(); finished {
						return result, ferr
					}
				}
			} else if (run.gatePending || run.unsentReprompt) && run.gateAtBoundary {
				// No new arrival, an entry is pending or a re-prompt is unsent, and the writer is idle: re-evaluate, so a verdict recorded since is read without waiting for the writer to speak.
				// A pending entry is re-read at most once per gateRecheckEvery; an unsent re-prompt is retried every tick, since that re-sends a re-prompt and reads no verdict.
				if (run.unsentReprompt || run.gateRecheckDue()) && run.boundaryIdle() {
					if result, finished, ferr := run.handleGatedBoundary(); finished {
						return result, ferr
					}
				}
			} else if run.startCleared && run.startHoldReleased() {
				// The boundary a turn start cleared is restored: evaluate it as on a new turn end, pending or not.
				if result, finished, ferr := run.handleGatedBoundary(); finished {
					return result, ferr
				}
			} else if len(run.gate) != 0 && !run.gateAtBoundary && run.gateRecheckDue() {
				// Nothing arrived and the writer is off the boundary, mid-turn or with no boundary seen since an attach:
				// ask the entries' read-only probes whether a wait bound has run out, which the writer's own turn end would otherwise never show.
				run.lastGateEvalAt = run.clock.Now()
				if name, expired := run.probeExpiry(); expired {
					logger.Info("shuttle: gate: entry wait expired off the boundary, finalizing", "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "gate", name)
					run.gateVerdict = nil
					return run.finalize(OutcomeDone)
				}
			}
		}

		run.settleDeferredHold()
		run.watchDialog()
		run.logSessionState("", tick%livenessEvery == 0)

		if tick%livenessEvery == 0 {
			livenessOutcome, err := run.checkLivenessTick(&started, startupDeadline, false)
			if err != nil {
				statusFailures++
				if statusFailures >= maxStatusRetries {
					// A satisfied file contract outranks this mechanism failure exactly as it
					// outranks every other negative answer in this loop -- see
					// finishedDespiteMechanismFailure.
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
			// classifyDeadlineExpiry, not a bare OutcomeTimeout: the run deadline answers "has the
			// clock run out", never "did this run finish", and a run whose every output file is on
			// disk finished whatever the clock says — see that function.
			return run.finalize(run.classifyDeadlineExpiry(OutcomeTimeout))
		}

		run.clock.Sleep(interval)
	}
}

// observePause is Wait's per-tick reading of the pause and of machine sleep.
// It feeds the run's sleep watch when the clock is an AwakeClock, reads the runner's coverage of its pair into run.covered, and moves run.deadline by the change in the time excluded since the run was created.
// The excluded time is the union of the detected sleeps and the coverage's paused intervals, so the deadline counts awake, unpaused time only.
// A runner told no pause geometry never covers the run.
// An unreadable record reads uncovered, with one Warn per change of error.
func (run *Run) observePause() {
	now := run.clock.Now()
	if awake, ok := run.clock.(AwakeClock); ok {
		run.sleeps.Observe(now, awake.Awake())
	}
	var coverage pauserecord.Coverage
	if geometry := run.runner.pause; geometry.RecordPath != "" {
		var err error
		coverage, err = pauserecord.ReadCoverage(geometry.RecordPath, geometry.Slug, now)
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

// gateRecheckDue reports whether gateRecheckEvery has passed since the last gate evaluation, which is always so before the first.
func (run *Run) gateRecheckDue() bool {
	return run.lastGateEvalAt.IsZero() || run.clock.Now().Sub(run.lastGateEvalAt) >= gateRecheckEvery
}

// probeExpiry calls the Expired probe of every entry that has one and is not off, in list order, and returns the name of the first entry whose probe reports expired.
// A probe error counts as not expired, so the loop keeps polling under its deadline and liveness checks;
// it warns once per entry and episode, and an answer after it logs one Info.
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

// gateEntryError builds a gate-contract violation error for the named entry.
// It is not an Errorf call on purpose: evaluateGate's returns are infrastructure faults wrapped by their callers with "shuttle: gate: %w", not negative verdicts on whether the run finished.
func gateEntryError(name, problem string) error {
	return errors.New("entry " + strconv.Quote(name) + " " + problem)
}

// handleGatedBoundary evaluates the gate list from its first entry at a turn boundary and reports whether Wait finishes with the returned Result and error.
// It is reached from a gated Done arrival and from a poll tick with no new arrival while an entry is pending, the writer is idle and gateRecheckEvery has passed since the last evaluation.
// Run.Interrupt is never used here: the gate fires at a turn boundary, when there is no in-progress turn to interrupt.
// No new deadline is introduced and run.deadline is never extended: the loop runs under the deadline Start already set from spec.Timeout,
// so a timeout mid-wait still reaches classifyDeadlineExpiry, which classifies OutcomeDone when the files are present and therefore still runs the gate one final time through finalize.
//
// A pass, a terminal failure (GateResult.Terminal, whatever the entry's failure count, with no re-prompt and no count incremented), or a failure whose budget is spent, finalizes done.
// A failure with budget remaining re-prompts and keeps polling.
// A re-prompt send that fails with ErrSessionBusy or ErrSubmissionNotLanded leaves the entry's failure and sent counts unchanged, clears the memo and keeps the writer at the boundary with the re-prompt marked unsent, so a later tick re-sends it until the run deadline;
// any other re-prompt send failure ends the loop as it always has.
// A pending result sends its Send text when non-empty and keeps polling;
// a failed pending send warns once per entry and episode, naming the entry, the error and the closure's way-forward, logs Debug while it keeps failing and Info when it is delivered, leaves the entry pending and the writer at the boundary, and never ends the loop.
// The memo is cleared after a pending result, so the next evaluation, and a finalize after it, read the closures afresh.
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
		return Result{}, false, nil
	}
	failed := run.gateFailedAt
	if failed < 0 || run.gateTerminal || run.gateFails[failed] >= run.gate[failed].Attempts {
		// No failing entry, a terminal failure, or a spent budget: the verdict is the memo evaluateGate just stored, which finalize reads rather than re-validating.
		result, ferr := run.finalize(OutcomeDone)
		return result, true, ferr
	}
	// An entry failed with budget remaining: re-prompt the agent and keep polling.
	reprompt := gateRepromptText(run.gateFindingsPath)
	if render := run.gate[failed].Reprompt; render != nil {
		reprompt = render(run.gateFindingsPath)
	}
	if serr := run.sendWithin(reprompt); serr != nil {
		if errors.Is(serr, ErrSessionBusy) || errors.Is(serr, ErrSubmissionNotLanded) {
			// No attempt was spent: keep the writer at the boundary and re-send on a later tick.
			// The send cleared its own text where it could, but for an engine with the idle reading a box it left occupied fails that re-send busy, until the run deadline ends the loop.
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
		result, ferr := run.finalize(OutcomeDone)
		return result, true, ferr
	}
	run.unsentReprompt = false
	run.gateAtBoundary = false
	run.gateFails[failed]++
	run.gateSent[failed]++
	for i, entry := range verdict.Entries {
		if entry.State == GateEntryPassed {
			run.gateFails[i] = 0
		}
	}
	// The run directory is deleted on the Done cleanup every exhausted gate takes, so the findings TEXT (never just its path) is folded into this Warn line — the durable record of why the gate failed, same as the deleted producers' own warn lines carried.
	// The file was just written by evaluateGate, so a read failure here is unexpected but non-fatal to the loop.
	findingsText, rerr := os.ReadFile(run.gateFindingsPath)
	if rerr != nil {
		findingsText = []byte(fmt.Sprintf("<unreadable: %v>", rerr))
	}
	logger.Warn("shuttle: gate: re-prompting after a failed attempt", "strandGUID", run.state.StrandGUID, "gate", run.gate[failed].Name, "attempt", run.gateFails[failed], "budget", run.gate[failed].Attempts, "findings", string(findingsText))
	// Clear the memo so the next arrival re-validates rather than reading this arrival's stale verdict.
	run.gateVerdict = nil
	return Result{}, false, nil
}

// startupTickCap returns the maximum number of checkLivenessTick calls awaitStartup's loop performs.
// Its interval is the startup probe's base interval, the poll interval and the shortest sleep the probe takes.
// The cap is the ceiling of startupTimeout/interval, plus 1, plus maxStatusRetries.
//
// It is the Live-Substrate Spawn Observability retry clause's attempt-COUNT bound: every tick's
// reed.Status and pane capture each run a real tmux process through reed, so the count cap is the
// clause's required bound — and it also terminates the loop under a clock that never advances. The
// maxStatusRetries slack keeps a run of tolerated status errors from eating the ticks the window
// itself needs.
//
// A negative startupTimeout is treated as 0 before the division, so the cap is never below
// 1 + maxStatusRetries.
func startupTickCap(startupTimeout, interval time.Duration) int {
	if startupTimeout < 0 {
		startupTimeout = 0
	}
	ticks := int(math.Ceil(float64(startupTimeout) / float64(interval)))
	return ticks + 1 + maxStatusRetries
}

// awaitStartup runs the same startup probe Wait runs — checkLivenessTick's liveness check, pane
// capture, the engine's startup classification, and the engine's trust-dismiss sequence for a
// recognized one-time gate — on its own, until the provider reaches StartupReady or the window (or
// the run's own deadline, whichever is shorter) closes. run.go's start calls it once, before ever
// returning a handle: this is what makes a *Run issued by Start/StartGated already past its
// provider's startup gates.
//
// The answer it reports is "the provider's input TUI is on screen, with any recognized one-time gate
// dismissed", never "the provider read its prompt": a ready-then-idle session still reads as ready,
// and that residual degrades to the caller's own outer watch budget.
//
// Return mapping: (Result{}, nil) when the handle may be issued — StartupReady reached (with Started
// persisted to run.json by checkLivenessTick, so a later Attach skips the startup probe), or the run's
// file contract already satisfied; (result, err) with errors.Is(err, ErrNotStarted) and a died/timeout
// Result.Outcome when the pane died, the startup window closed, or the run's own deadline arrived
// first, all with the teardown abandonStartup performs already done.
// The same teardown and a died Result follow when checkLivenessTick failed maxStatusRetries consecutive times with the file contract unsatisfied.
// The error then wraps ErrNotStarted beside the mechanism failure's own sentinel, worded in the same family Wait uses.
//
// The window is the shorter of startup_timeout_s and the time left until run.deadline (floored at
// 0): a run.Timeout shorter than startup_timeout_s must still expire on schedule, classified
// OutcomeTimeout rather than OutcomeDied, so RunGated's caller sees "the agent was working" rather
// than "it never started" when its own configured deadline, not the startup window, is what ran out.
// startup_timeout_s 0 is included, with deliberately no floor: it makes the probe answer on its first
// tick unless the TUI is already ready, the same fast-fail every producer run's Wait applies under
// that config.
//
// Every negative answer honors the Completion Signal Invariant: the not-ready answer is reached only
// through checkLivenessTick/classifyStartupWindow, which already consult allOutputFilesExist, the
// run-deadline exit goes through classifyDeadlineExpiry, and the retry-cap and tick-cap exits below
// consult allOutputFilesExist directly.
//
// The tick cap (startupTickCap) bounds the loop by count as well as by the deadline.
//
// Probe cadence: awaitStartup calls checkLivenessTick once per sleep, and the sleep backs off while the pane does not change.
// The base is pollInterval(cfg) and the ceiling is dismissalGap(cfg).
// The first sleep is the base.
// After each later probe the sleep returns to the base when the pane's capture differs from the one the previous probe left, and otherwise doubles up to the ceiling.
// A probe whose capture failed leaves the capture unchanged, so it backs off.
// A ready provider shows within the first one or two probes, because Claude's boot redraws the pane and resets the backoff.
// Gate keys are spaced by checkLivenessTick's dismissal gap rather than by the probe cadence,
// so a capture taken before the provider redraws after a dismissal never drives a second key into the next gate.
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

// abandonStartup is start's teardown for a provider that never became ready or never took its first input:
// the strand is torn down and the run's own Outcome is finalized while the run
// directory and its last pane capture are kept for diagnosis.
// cause is nil when the provider never reached StartupReady inside its window,
// and otherwise the error that ended the start, which the returned error wraps beside ErrNotStarted.
//
// In order: any capture checkLivenessTick recorded is saved to startupCaptureFileName (a write
// failure is a Warn, and the returned error then says no capture was saved); run.finalize(outcome)
// persists RunState.Outcome and logs "run finished", skipping the gate and cleanup since outcome is
// never OutcomeDone here; the strand is removed whatever Spec.KeepPane says — KeepPane governs a
// COMPLETED run's pane retention, not a startup failure's, and a provider that never came up leaves
// nothing worth keeping the pane around for; the run directory is left in place either way, so the
// capture file (and prompt.md/settings.json/events.jsonl) are findable; and a Warn names the run dir,
// strand guid and outcome, per the Live-Substrate Spawn Observability invariant.
//
// The returned error wraps ErrNotStarted and names the run dir, the strand guid, the outcome, whether
// the capture was saved, and whether the strand removal itself succeeded — carrying reed's own error
// text and an operator remedy when it did not, since a strand abandonStartup could not remove is the
// one residual an operator must clear by hand.
//
// When cause is nil and the run never started, the error also carries the reason the last startup probe held a ready pane back for want of a session-start line, if it recorded one.
// The error ends with ". The pane's last lines:" and the last startup capture's tail when all three hold:
// cause is nil, the provider never reached StartupReady (run.state.Started is false), and a startup capture was recorded.
// Every other teardown reports a cause that already carries its own screen or a mechanism failure with none,
// or a pane that died after it was ready, whose last capture is the ready screen from before the death.
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
	if _, rerr := run.runner.reed.RemoveStrand(run.state.StrandGUID, false); rerr != nil {
		logger.Warn("shuttle: startup: remove strand after not-ready teardown failed", "strandGUID", run.state.StrandGUID, "error", rerr)
		removeNote = fmt.Sprintf("the strand could NOT be removed (%v); remove it by hand (\"lyx reed status\" / \"lyx reed remove\")", rerr)
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
		format += ". The pane's last lines:\n%s"
		args = append(args, captureTail(run.lastStartupCapture))
	}

	return result, fmt.Errorf(format, args...)
}

// pollEventsTick reads any events.jsonl bytes appended since run.offset and
// parses them via the engine. run.offset only advances past what it read
// AFTER ParseEvents succeeds: if ParseEvents errors, the bytes stay
// unconsumed so Wait's events-failure retry actually re-reads and
// re-classifies them on the next tick, rather than silently discarding a
// batch that may contain the run's only qualifying event (the Engine seam
// permits an erroring parser; the retry counter Wait maintains implies this
// re-read guarantee).
// It classifies the batch by its LAST event:
// a batch containing more than one event, e.g. an interrupted turn immediately followed by a resumed one, is classified by its most recent one,
// and every consumed byte still counts once parsing succeeds, so none of the earlier events in the same batch is ever reprocessed.
// When every output file exists the run is done whatever the last event's Kind was.
// Otherwise the turn end is held, whatever its Kind:
// an EventStop and an EventAsk with output files missing return a held turn end carrying the event's message and the offset just past its line, and Wait keeps polling the same agent.
// Kind only selects Message's source, inside ParseEvents, not this branch.
// An EventWaiting is the one Kind that is not held at once: the session is waiting on its own background work,
// so the tick returns what expiredTurnEnd answers, which is nothing while the work is outstanding and the files are missing;
// an ungated one with an empty outstanding list and every output file present is done at once.
// A hold never extends run.deadline, so a held run is bounded by its caller's own deadline and by the liveness check.
// A waiting turn end never expires: a shell of either signal and a fork keep the turn waiting however long they run.
// It is bounded only by the run's own deadline and the liveness check, so a shell that never ends ends the run OutcomeTimeout, and lyx reaps no shell.
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

	events, err := run.runner.engine.ParseEvents(data)
	if err != nil {
		return "", nil, err
	}
	// Only now, with parsing proven successful, is it safe to advance past
	// these bytes — a parse failure must leave them for the next tick to
	// retry rather than discarding them unread.
	run.offset = newOffset
	guardArmed := run.holdGuard
	turnStarts := turnStartOffsets(run.runner.engine, data, startOffset)
	if len(turnStarts) > 0 {
		run.holdGuard = false
		run.promptTurnStarted = true
	}
	// The reference is the held turn end on show; a held turn end this tick returns replaces it below.
	run.turnStartRead = run.wait.heldOffset > 0 && turnStartAfter(turnStarts, run.wait.heldOffset)
	if len(events) == 0 {
		return run.expiredTurnEnd()
	}

	run.eventsRead = true
	run.countedTasks = nil
	last := events[len(events)-1]
	if last.Kind == EventWaiting {
		run.recordWaiting(last)
		if len(run.waitingTasks) == 0 && len(run.gate) == 0 && allOutputFilesExist(run.spec.OutputFiles) {
			return OutcomeDone, nil, nil
		}
		return run.expiredTurnEnd()
	}
	run.waitingTasks = nil
	if allOutputFilesExist(run.spec.OutputFiles) {
		return OutcomeDone, nil, nil
	}
	held := &heldTurnEnd{message: last.Message, offset: offsetPastEvent(data, startOffset, newOffset, last)}
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

// armHoldGuard sets the hold guard for a Wait from the events file as it stands.
// A turn start before the prompt offset proves the provider writes turn starts for typed turns, so the guard arms;
// a turn start from the prompt offset up to the run's read offset, which a Wait attached past the prompt's turn start sees, lifts it at once.
// An engine without SessionSignalParser, a zero prompt offset, a file with no turn start before the prompt offset and an unreadable file leave the guard off.
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
	if len(turnStartOffsets(run.runner.engine, data[:promptOffset], 0)) == 0 {
		return
	}
	run.holdGuard = true
	if end := min(max(run.offset, promptOffset), int64(len(data))); len(turnStartOffsets(run.runner.engine, data[promptOffset:end], promptOffset)) > 0 {
		run.holdGuard, run.promptTurnStarted = false, true
	}
}

// turnStartOffsets returns the events-file offset just past the line of each turn start the engine reads from data, in file order.
// data is the slice of the events file that begins at startOffset.
// An engine without SessionSignalParser reads none.
func turnStartOffsets(engine Engine, data []byte, startOffset int64) []int64 {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return nil
	}
	signals, _ := parser.ParseSessionSignals(data)
	var offsets []int64
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
			offsets = append(offsets, startOffset+int64(end))
		}
	}
	return offsets
}

// heldTurnEnd is a plain Stop or a live ask that left the run's output files missing, so the run is held rather than ended.
type heldTurnEnd struct {
	// message is the agent's last message at the turn end.
	message string
	// offset is the events-file byte offset just past the turn end's line.
	offset int64
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

// recordWaiting keeps a waiting turn end's outstanding list, logs what it waits on once, and stamps each shell id not seen before with now.
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

// ShellWaitBound returns how long a background shell of either signal stays outstanding before the wait logs it and shows it in the wait marker, cfg.BackgroundShellWaitMin, flooring a non-positive hand-built value to the template default as pollInterval does.
// The bound expires nothing: a turn end waiting on a shell keeps waiting until its output files exist, the run's deadline or the liveness check.
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

// expiredTurnEnd decides the recorded waiting turn end: it either counts it as a Done at once or keeps it waiting, and never holds it.
// An ungated run counts it at once whenever every output file exists, whatever is outstanding.
// A gated run's waiting turn end is never an arrival, because the output files may be left over from an earlier arrival while the session works on in the background;
// it waits on every outstanding task, a shell or a fork, until the task's completion starts the next turn, the run's deadline or the liveness check.
// A turn end with an output file missing waits the same way.
// A shell outstanding past the bound is logged once per shell, and that changes no decision.
// Counting a turn end at once clears the waiting list and keeps its tasks in countedTasks for the run's record.
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
	if len(run.gate) != 0 || !allOutputFilesExist(run.spec.OutputFiles) {
		return "", nil, nil
	}
	run.countedTasks = run.waitingTasks
	run.waitingTasks = nil
	return OutcomeDone, nil, nil
}

// logPayloadShellPastBound logs at Warn, once per shell id, that a background shell of either signal has been outstanding for at least the bound.
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
// With the strand removed it says the removal ended them; on every other end it says the run ended with them outstanding and the strand is left to its caller.
// lyx kills no shell itself, so a shell detached from the pane's process tree survives the removal.
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

// endedShells lists each shell outstanding when the run ends, from the recorded waiting turn end, or from the turn end counted at once when none is recorded.
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

// readEventsFrom reads path from byte offset onward, returning bytes up to
// the last complete line. Partial lines are left unconsumed for the next tick.
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
// The second return value is what separates "reed tracks this strand and its pane is not alive" from
// "reed does not track this strand", which a bare liveness bool cannot express — see errStrandNotTracked.
func strandStatusByGUID(strands []reedengine.StrandStatus, guid string) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if s.GUID == guid {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// allOutputFilesExist reports whether every entry in files exists on disk.
func allOutputFilesExist(files []string) bool {
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return false
		}
	}
	return true
}

// checkLivenessTick checks strand liveness and probes the pane during
// startup. Returns a non-nil error for a reed.Status that could not be run, and for the two Status
// answers that report reed's own bookkeeping rather than the agent's fate: a Status that no longer
// lists this run's strand (errStrandNotTracked), and one that lists it with no pane bound
// (errStrandPaneBindingCleared) — see each for why it is a mechanism failure rather than a
// classification.
// A satisfied file contract wins over every negative answer: the agent's output files ARE its return
// value, so their existence classifies OutcomeDone whether the pane died or reed simply stopped
// tracking or addressing it.
//
// A startup gate is dismissed at most once per dismissalGap on the run's clock: a probe inside the gap that still sees a gate plays nothing and does not ask for the dismissal keys.
// Any play that sent at least one input starts the gap, failed partway or not; an empty sequence or a first input that failed does not.
// Bound: the throttle only removes key presses and never adds one, and the startup deadline and the tick cap still end a gate that never clears.
//
// Once the strand reads live, every tick asks sessionGone, before the started short-circuit so startup is covered too;
// a gone session returns OutcomeDone when every output file exists and OutcomeDied otherwise, the way a dead pane does.
// Run.Wait's own startup probe already runs at least one gap apart, so the throttle never holds a dismissal there.
//
// For a record with RecordsSessionStarts, a ready pane marks the run started only once the events file also holds a session-start line.
// Until then the tick stays not started, records the reason on the run for abandonStartup and answers as a still-booting pane does; gate detection and dismissal run as for any run.
// Bound: a session-start line an earlier launch of the same run left satisfies the line half, and the pane half still gates it.
//
// With eventsProof set, a record with RecordsSessionStarts is also started once the events file holds a session start followed by a turn start, checked before the pane capture;
// a false answer falls through to the capture path, so a recognized gate is still dismissed.
// Only start's probe sets it: its events file is created fresh for the launch, so only this launch's hooks satisfy it, while Wait's probe of a not-yet-started run may read an earlier incarnation's lines.
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
		return "", errStrandNotTracked
	}
	if !strand.Live {
		if allOutputFilesExist(run.spec.OutputFiles) {
			return OutcomeDone, nil
		}
		// Reed reports not-live for a strand it holds no pane id for, exactly as it does for one whose
		// pane died — but those are different facts, and only the second is a dead run. An
		// anchor:hidden strand is the one case where an empty pane id is normal rather than a cleared
		// binding (reed realizes a pane for every other anchor), so it keeps today's classification;
		// every other run's strand carries a pane id from the moment AddStrand persists it.
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

	if eventsProof && run.state.RecordsSessionStarts && turnStartedAfterSessionStart(run.runner.engine, run.state.EventsPath) {
		*started = true
		run.markStarted()
		return "", nil
	}

	capture, err := run.runner.reed.CapturePane(run.state.StrandGUID)
	if err != nil {
		run.failures.Fail("startup capture", "shuttle: capture pane during startup probe (non-fatal, retrying)", "strandGUID", run.state.StrandGUID, "error", err)
		// A pane that cannot be captured has told us nothing about whether the provider came up,
		// so it stays inside the startup window and expires with it — see classifyStartupWindow.
		return run.classifyStartupWindow(startupDeadline), nil
	}
	run.failures.Recover("startup capture", "shuttle: startup probe capture works again", "strandGUID", run.state.StrandGUID)
	// Recorded on every successful capture, not just a ready one: abandonStartup's not-ready teardown
	// saves whatever the last capture showed as a diagnosis artifact, so a still-pending or
	// still-gated pane's last screen is on disk exactly as a ready one's would be.
	run.lastStartupCapture = capture

	switch run.runner.engine.Startup(capture) {
	case StartupReady:
		if run.state.RecordsSessionStarts {
			recorded, readErr := sessionStartRecorded(run.runner.engine, run.state.EventsPath)
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
		inputs := run.runner.engine.TrustDismissSequence(capture)
		sent, err := playInputsSent(run.runner.reed, run.state.StrandGUID, inputs)
		if sent > 0 {
			run.dismissed, run.dismissedAt = true, now
		}
		if err != nil {
			run.failures.Fail("startup dismiss", "shuttle: dismiss trust prompt (non-fatal)", "strandGUID", run.state.StrandGUID, "error", err)
		} else if len(inputs) > 0 {
			run.failures.Recover("startup dismiss", "shuttle: trust prompt dismissal sends again", "strandGUID", run.state.StrandGUID)
			// Info, not Debug: a dismissal is a lifecycle event that happens at most once or twice
			// per run, not a routine polling probe — and Wait's producers gain this line too.
			logger.Info("shuttle: dismissed startup gate", "strandGUID", run.state.StrandGUID, "inputs", len(inputs))
		}
	}
	// StartupPending and a trust prompt that has not yet cleared are both "still not ready", and
	// share the one deadline that governs the whole startup window.
	return run.classifyStartupWindow(startupDeadline), nil
}

// markStarted persists that the provider is past its startup gates.
// It is persisted immediately, not left for finalize: this is the one fact a killed driver must
// not lose, since it is what tells a LATER Attach apart a genuinely booted, mid-turn pane
// from one whose launch already failed (see Wait's own doc comment on started).
// Best-effort, like every other mid-run persistence in this package (finalize's Outcome write):
// a save failure costs a future re-attach one extra startup probe, never this run's own correctness.
func (run *Run) markStarted() {
	run.recordMu.Lock()
	run.state.Started = true
	err := run.saveState()
	run.recordMu.Unlock()
	if err != nil {
		logger.Warn("shuttle: persist run started failed (non-fatal)", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "error", err)
	}
}

// sessionLivenessState is what sessionGone keeps across the liveness ticks of one wait.
type sessionLivenessState struct {
	// sizeTaken reports that firstTickSize holds the events file's size at the wait's first liveness tick.
	sizeTaken     bool
	firstTickSize int64
	// readAlive holds the session ids a liveness tick of this wait read alive.
	readAlive map[string]bool
}

// sessionGone reports whether this tick proves the provider's process gone behind a live pane.
// It answers false for an engine without both SessionSignalParser and SessionProber.
// Otherwise it reads the registry once for the newest session id the events file names, the record's SessionID when none, and answers true when either holds:
// the signals past the events-file size taken at the wait's first call end in a process-ending session end and the reading is not alive,
// or the reading is dead for an id this wait read alive at an earlier tick.
// An alive reading records the id and answers false, as does an unproven reading, a dead one for an id this wait never read alive and a session end at or before the first-call size.
// An unreadable events file counts no session end.
func (run *Run) sessionGone() bool {
	parser, parses := run.runner.engine.(SessionSignalParser)
	prober, probes := run.runner.engine.(SessionProber)
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

// sessionStartRecorded reports whether the events file holds a session-start signal of any source, read from the file's start.
// An absent file reads false with no error and an unreadable one returns the read error; an engine without SessionSignalParser reads false.
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

// turnStartedAfterSessionStart reports whether the events file holds a turn-start signal after a session-start signal, in file order, read from the file's start.
// An absent or unreadable file and an engine without SessionSignalParser read false.
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

// classifyStartupWindow reports the run's outcome once startupDeadline has passed on a run whose
// provider has still not reached StartupReady, and "" while that window is still open.
//
// It is called from every not-yet-started exit of checkLivenessTick, not just the still-booting one,
// because the startup window belongs to the WINDOW rather than to one classification within it.
// Enforcing it only on StartupPending let the two silent paths — a trust prompt whose dismissal
// never takes, and a pane that fails every capture — skip the deadline entirely and burn the full
// run_timeout_min (30 minutes by default) instead of startup_timeout_s (90), and be reported as
// OutcomeTimeout ("the agent was working") rather than OutcomeDied ("it never started").
//
// The expired answer comes from classifyDeadlineExpiry rather than being a bare OutcomeDied,
// because an expired startup window is a NEGATIVE answer like any other and the file contract
// outranks all of them — see that function for the whole argument.
func (run *Run) classifyStartupWindow(startupDeadline time.Time) Outcome {
	if run.clock.Now().After(startupDeadline) {
		return run.classifyDeadlineExpiry(OutcomeDied)
	}
	return ""
}

// classifyDeadlineExpiry reports the outcome for a deadline that has just expired: OutcomeDone when
// the run's file contract is already satisfied, and expired otherwise.
//
// It exists because a deadline answers "has the clock run out", never "did this run finish", and
// the two deadlines in this file — the startup window and the run deadline — both used to publish
// the first as though it were the second. checkLivenessTick's own doc comment already states the
// governing rule ("a satisfied file contract wins over every negative answer"), and its
// not-tracked and not-live branches both honour it; the two deadline paths did not, so a run whose
// agent had written every file in OutputFiles but left no parseable terminal event behind was
// classified OutcomeDied at startup_timeout_s or OutcomeTimeout at the run deadline. Reproduced
// live in crucible round opus5-high-r4 against the real built binary, once per deadline: both of
// Discussion-Write's declared output files present on disk, events.jsonl never written, and the
// step recorded as a shed failure — after which the next resume archived the finished files and
// re-ran the step, which is precisely the rework the Completion Signal Invariant
// ("inside an attached or started run's own wait loop … the step finished; read it and advance")
// exists to prevent.
//
// allOutputFilesExist is vacuously true for an empty list, which would make this wrong for a run
// with no declared outputs — there is none: Spec.validate refuses an empty OutputFiles on the Start
// path, and on the Attach path collectAttachCandidates set-matches against a persisted RunState
// written by a validated Start, so an empty spec matches no candidate and no *Run is ever built for
// it. Spec.validate states the same rule from the other side when it refuses a spec whose output
// file ALREADY exists, "a pre-existing file would satisfy the file contract immediately".
func (run *Run) classifyDeadlineExpiry(expired Outcome) Outcome {
	if allOutputFilesExist(run.spec.OutputFiles) {
		return OutcomeDone
	}
	return expired
}

// finishedDespiteMechanismFailure finalizes OutcomeDone when the run's file contract is already
// satisfied at a point Wait would otherwise abandon the run with a mechanism-failure error, and
// reports whether it did (the bool is false, with a zero Result and nil error, when the contract is
// not satisfied and the caller must surface its mechanism error unchanged).
//
// It exists for the same reason classifyDeadlineExpiry does, extended to Wait's two retry-exhausted
// exits: the events file staying unreadable for maxEventsReadRetries consecutive ticks, and
// reed.Status erroring for maxStatusRetries consecutive ticks, both answer "has reed's own
// bookkeeping or the events file gone wrong", never "did this run finish". checkLivenessTick's own
// doc comment already governs both -- "a satisfied file contract wins over every negative answer" --
// and its not-tracked and not-live branches beside these exits both honour it; the two
// retry-exhausted caps did not, so a run whose agent had written every declared output file but left
// reed.json corrupt (a crash, a full disk, a kill -9 during a reed write) or events.jsonl
// unparseable was recorded as a mechanism failure, after which the next resume archived the finished
// files and re-ran the step -- precisely the rework the Completion Signal Invariant
// exists to prevent. Reproduced live in crucible round fable5-high-r5 against the real built binary:
// a shuttle run whose declared output file was on disk, reed.json truncated mid-run, returned the
// consecutive-status-failure mechanism error rather than OutcomeDone.
//
// The empty-OutputFiles vacuous-truth concern classifyDeadlineExpiry documents does not arise here
// either, and for the same reason: Spec.validate refuses an empty OutputFiles on the Start path and
// collectAttachCandidates set-matches against a validated Start's persisted record on the Attach
// path, so no *Run with an empty OutputFiles ever reaches this loop.
func (run *Run) finishedDespiteMechanismFailure() (Result, error, bool) {
	if allOutputFilesExist(run.spec.OutputFiles) {
		result, err := run.finalize(OutcomeDone)
		return result, err, true
	}
	return Result{}, nil, false
}

// identity returns the run's identifying fields with an empty Outcome: what Wait hands back
// alongside a mechanism-failure error, so the caller keeps the handles it needs to diagnose,
// resume, or tear down a run that reached no classification.
func (run *Run) identity() Result {
	return Result{
		SessionID:  run.state.SessionID,
		StrandGUID: run.state.StrandGUID,
		RunDir:     run.runDir,
	}
}

// evaluateGate runs run's gate entries once per arrival, memoising the verdict on run.gateVerdict — which is what makes "the gate runs exactly once per settling" true:
// this method returns the stored memo unchanged when it is already non-nil, and Wait clears the memo to nil before each re-prompt so the next arrival re-validates rather than reading the first arrival's stale verdict.
//
// Returns (nil, nil) when run.gate is empty (an ungated run) — the method's own way of saying "there is nothing to evaluate", distinct from Result.Gate's own nil-means-no-gate contract.
//
// The entries run in list order and stop at the first failure.
// An off entry (Attempts 0) is reported off wherever it sits;
// a PassOnCap entry whose failure count has reached its Attempts is not run and is reported let through;
// every other entry runs its closure and is reported passed, failed or, for a PassOnCap or MayHold entry, waiting;
// every non-off entry after the stopping entry is reported not reached.
// Passed is true only when every entry that is neither off nor PassOnCap passed at this arrival, so a pending MayHold entry is never a pass.
// A Terminal flag on a passed or pending result or from a PassOnCap entry, or Pending or a non-nil Final on an entry that is neither PassOnCap nor MayHold, is a returned gate error.
// A Terminal failing result is recorded on run.gateTerminal, and its Findings text is carried on GateOutcome.Reason.
// The method never changes a count — only Wait's re-prompt branch does — so a finalize evaluating afresh after a lost session or a deadline cannot change one.
//
// A non-nil error from a closure is returned verbatim, stores no memo and changes no count, per the "a gate error is never not passed" decision:
// a gate that could not run has found no defect, it is an infrastructure fault, not a GateResult{Passed: false}.
//
// On a failing entry the method writes GateResult.Findings to <run.runDir>/gateFindingsFileName, overwriting any previous arrival's file so the agent always reads the current complaint (the "findings always ride a file" decision), and sets GateOutcome.FindingsPath to that path when the outcome did not pass.
// A write failure is a returned error, not a failed gate — findings the agent can never read cannot fix anything.
// Both of this method's error returns come back unwrapped, on purpose:
// this is a helper the two finalize/Wait call sites already wrap with their own "shuttle: gate: %w" context, so wrapping here too would double it, and it keeps this method's own negative-verdict return sites free of the completionsignal_enforcement_test.go tripwire's Errorf marker, which is reserved for the two call sites that actually finalize a run.
//
// GateOutcome.Attempts is the sum of the entries' re-prompts sent, per the "attempts counts re-prompts actually sent" decision.
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
	}
	for _, entry := range run.gate {
		if entry.Final != nil && !entry.PassOnCap && !entry.MayHold {
			return nil, gateEntryError(entry.Name, "sets Final but is not pass_on_cap")
		}
	}

	outcome := &GateOutcome{Passed: true, Entries: make([]GateEntryOutcome, len(run.gate))}
	failedAt := -1
	pendingAt := -1
	pendingSend, pendingWayForward := "", ""
	findingsPath := ""
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
			if final && entry.Final != nil {
				closure = entry.Final
			}
			run.beginGateWait(entry.Name)
			result, err := closure()
			run.endGateWait()
			if err != nil {
				return nil, err
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
		outcome.Reason = reason
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
// For fork mode, audits fork subagents and attaches the result.
//
// finalize is the gate's single verdict site: immediately on entry, and only when outcome ==
// OutcomeDone, it calls run.evaluateGate() and stamps the returned pointer onto result.Gate — before
// the RunState.Outcome write, the fork-audit block, and cleanup, so no Done escapes ungated and an
// exhausted gate still takes the ordinary Done-path cleanup. One verdict site reached by all four of
// Wait's finalize calls is what makes "no Done escapes ungated" true BY CONSTRUCTION rather than by
// four correct edits — classifyStartupWindow and classifyDeadlineExpiry cannot host the gate
// themselves, because they return an Outcome rather than a Result.
//
// The Outcome write happens for EVERY terminal outcome, before the fork-audit block: finalize returns
// early with (result, err) when AuditForks fails, so a write placed after the audit would leave a run
// that genuinely classified OutcomeDone persisted at "running" — the exact live-but-idle state the
// sentinel exists to prevent, if its pane is still alive on a later resume. The write is best-effort:
// a saveRunState failure here is a logger.Warn, never an error, because the run genuinely reached its
// outcome and turning a successful run into a failure over a bookkeeping write would invert the cost.
// The accepted residual is that such a record keeps "running", so if its pane is also still live and
// idle it stays attachable for that one run.
//
// It is also the run's teardown observability point, which the Live-Substrate Spawn Observability
// invariant requires to be as instrumented as the spawn is: Start logs "run started" through
// internal/logger, so without the logger.Info below the durable Info+ trace file would show every
// shuttle run beginning and none of them ending. The two cleanup failures go to logger.Warn for the
// same reason — a teardown that did not confirm clean is exactly what that level is for, and the
// bare log package they used before never reaches the trace sink at all.
func (run *Run) finalize(outcome Outcome) (Result, error) {
	run.logSessionState(outcome, true)
	result := Result{
		Outcome:     outcome,
		SessionID:   run.state.SessionID,
		StrandGUID:  run.state.StrandGUID,
		RunDir:      run.runDir,
		EndedShells: run.endedShells(run.clock.Now()),
		EndedAt:     run.clock.Now(),
	}
	if createdAt, err := time.Parse(time.RFC3339, run.state.CreatedAt); err == nil {
		result.StartedAt = createdAt
	}

	if outcome == OutcomeDone {
		gateOutcome, err := run.evaluateGate(true)
		if err != nil {
			return result, fmt.Errorf("shuttle: gate: %w", err)
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

	if reader, ok := run.runner.engine.(UsageReader); ok && outcome == OutcomeDone {
		result.Usage = reader.SessionUsage(run.state.SessionID, run.runner.paneCwd)
	}

	if outcome == OutcomeDone && run.spec.ForkSubagents {
		audit, err := run.runner.engine.AuditForks(run.state.SessionID, run.runner.paneCwd)
		if err != nil {
			// The run itself SUCCEEDED and nothing has been cleaned up yet, so the caller gets the
			// whole classified Result back — identity AND Outcome — not the bare identity().
			// identity() is for Wait's mechanism-failure exits, where no outcome was ever reached;
			// here one was, and blanking it left a caller unable to tell a run that finished and
			// merely failed its audit from a run that never classified at all. Since this branch
			// skips cleanup either way, the identity fields alone do not separate them.
			// ForkAudit stays nil, which already means "not audited".
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

// diedCaptureFileName is where removeDiedOwnWindowStrand saves a died run's last pane capture in the run directory.
const diedCaptureFileName = "died-capture.txt"

// finalizeAndRemoveDiedWindow finalizes the run with outcome, then ends the window of a died run's own-window strand.
func (run *Run) finalizeAndRemoveDiedWindow(outcome Outcome) (Result, error) {
	result, err := run.finalize(outcome)
	if result.Outcome == OutcomeDied {
		run.removeDiedOwnWindowStrand()
	}
	return result, err
}

// removeDiedOwnWindowStrand saves a died run's last pane capture to died-capture.txt in the run directory and removes the run's strand, when the strand is tracked, own-window and present.
// A window 0 strand is left to reed's reconcile.
// A failed capture or write is a Warn and the removal still runs; a failed removal is a Warn naming `lyx reed remove` as the way forward.
func (run *Run) removeDiedOwnWindowStrand() {
	guid := run.state.StrandGUID
	reed := run.runner.reed
	status, err := reed.Status()
	if err != nil {
		logger.Warn("shuttle: died run: probe reed before removing its own-window strand failed (non-fatal)", "strandGUID", guid, "error", err)
		return
	}
	strand, tracked := strandStatusByGUID(status.Strands, guid)
	if !tracked || !strand.OwnWindow || !strand.Present {
		return
	}
	if capture, err := reed.CapturePane(guid); err != nil {
		logger.Warn("shuttle: died run: capture the own-window pane failed (non-fatal)", "strandGUID", guid, "error", err)
	} else if err := os.WriteFile(filepath.Join(run.runDir, diedCaptureFileName), []byte(capture), 0o644); err != nil {
		logger.Warn("shuttle: died run: save the last pane capture failed (non-fatal)", "runDir", run.runDir, "strandGUID", guid, "error", err)
	}
	if _, err := reed.RemoveStrand(guid, false); err != nil {
		logger.Warn("shuttle: died run: remove the own-window strand failed; run \"lyx reed remove\" to clear it", "strandGUID", guid, "error", err)
		return
	}
	logger.Info("shuttle: removed a died run's own-window strand", "runDir", run.runDir, "strandGUID", guid)
}
