// wait.go implements Run.Wait: the poll loop that reads a run's events.jsonl, classifies its
// terminal outcome (done/asking/died/timeout), probes the startup window for a trust-dialog
// dismissal or a fast-failing dead pane, and runs the done-outcome cleanup (strand removal + run
// dir deletion).
// Wait is the only place in the run loop that sleeps — the clock seam defined here lets tests
// replay a whole poll sequence instantly.
// A pane that goes not-live (crashed, killed, or exited) is classified done rather than died when
// every output file already exists — the file contract can be satisfied an instant before the
// process disappears, racing ahead of its own Stop hook.
// died is reserved for a strand reed STILL TRACKS AND STILL HOLDS A PANE FOR, whose pane is not
// alive; the two ways reed's own bookkeeping can go away instead — a strand it no longer tracks
// (errStrandNotTracked) and a strand whose pane binding it cleared (errStrandPaneBindingCleared) —
// are mechanism failures, not classifications.
// When the run's Spec.AwaitOperator is true, an OutcomeAsking classification is non-terminal: Wait
// logs the observation and keeps polling instead of returning, so an interactive interview survives
// its first question batch. Every other exit (OutcomeDone, OutcomeDied, a liveness mechanism
// failure, OutcomeTimeout) is unaffected.

package shuttleengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// clock abstracts time for tests.
type clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// realClock is the production clock: real wall-clock time, real sleeping.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// defaultPollIntervalMS is the template.yaml default poll interval.
const defaultPollIntervalMS = 500

// pollInterval returns Wait's tick interval, flooring non-positive values
// to the template default to prevent busy-spinning.
func pollInterval(cfg Config) time.Duration {
	if cfg.PollIntervalMS <= 0 {
		return defaultPollIntervalMS * time.Millisecond
	}
	return time.Duration(cfg.PollIntervalMS) * time.Millisecond
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

// Wait blocks until run reaches a terminal outcome.
// Error is reserved for mechanism failures that leave no classifiable outcome.
// When run.spec.AwaitOperator is true, an OutcomeAsking classification does not count as terminal —
// Wait logs the ask and keeps polling, so it terminates only on OutcomeDone, OutcomeDied, a liveness
// mechanism failure, or OutcomeTimeout.
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

	started := false
	eventsFailures := 0
	statusFailures := 0

	for tick := 1; ; tick++ {
		outcome, message, err := run.pollEventsTick()
		if err != nil {
			eventsFailures++
			if eventsFailures >= maxEventsReadRetries {
				return run.identity(), fmt.Errorf("shuttle: events file unreadable after %d attempts: %w", maxEventsReadRetries, err)
			}
		} else {
			eventsFailures = 0
			if outcome == OutcomeAsking && run.spec.AwaitOperator {
				// AwaitOperator makes an ask non-terminal: log the observation so the driver log
				// records each one, and keep polling instead of finalizing here. OutcomeDone still
				// falls through to finalize below, unaffected by this branch.
				logger.Info("shuttle: awaiting operator, ask observed", "strandGUID", run.state.StrandGUID, "lastAssistantMessage", message)
			} else if outcome != "" {
				return run.finalize(outcome, message)
			}
		}

		if tick%livenessEvery == 0 {
			livenessOutcome, err := run.checkLivenessTick(&started, startupDeadline)
			if err != nil {
				statusFailures++
				if statusFailures >= maxStatusRetries {
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
					return run.finalize(livenessOutcome, "")
				}
			}
		}

		if run.clock.Now().After(run.deadline) {
			return run.finalize(OutcomeTimeout, "")
		}

		run.clock.Sleep(interval)
	}
}

// pollEventsTick reads any events.jsonl bytes appended since run.offset and
// parses them via the engine. run.offset only advances past what it read
// AFTER ParseEvents succeeds: if ParseEvents errors, the bytes stay
// unconsumed so Wait's events-failure retry actually re-reads and
// re-classifies them on the next tick, rather than silently discarding a
// batch that may contain the run's only qualifying event (the Engine seam
// permits an erroring parser; the retry counter Wait maintains implies this
// re-read guarantee). It classifies OutcomeDone/OutcomeAsking from the LAST
// Event among the newly parsed ones (a batch containing more than one
// event — e.g. an interrupted turn immediately followed by a resumed one —
// is classified by its most recent one, and every consumed byte still
// counts once parsing succeeds, so none of the earlier events in the same
// batch is ever reprocessed). The done/asking branch below is the SAME
// two-way check regardless of the last event's Kind: an EventStop with no
// output files and an EventAsk with no output files both classify
// OutcomeAsking identically — Kind only selects Message's source, inside
// ParseEvents, not this branch (a Kind switch here would be dead code,
// since both non-done kinds behave the same way). This is what makes a live,
// in-progress tool-call signal the engine surfaces (see claudeengine's
// ParseEvents for the concrete provider mapping) classify as a real-time
// asking the instant the tool call opens, exactly like today's turn-end
// asking case. Returns
// outcome == "" when there is nothing new to classify yet.
func (run *Run) pollEventsTick() (Outcome, string, error) {
	data, newOffset, err := readEventsFrom(run.state.EventsPath, run.offset)
	if err != nil {
		return "", "", err
	}
	if len(data) == 0 {
		return "", "", nil
	}

	events, err := run.runner.engine.ParseEvents(data)
	if err != nil {
		return "", "", err
	}
	// Only now, with parsing proven successful, is it safe to advance past
	// these bytes — a parse failure must leave them for the next tick to
	// retry rather than discarding them unread.
	run.offset = newOffset
	if len(events) == 0 {
		return "", "", nil
	}

	last := events[len(events)-1]
	if allOutputFilesExist(run.spec.OutputFiles) {
		return OutcomeDone, "", nil
	}
	return OutcomeAsking, last.Message, nil
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
func (run *Run) checkLivenessTick(started *bool, startupDeadline time.Time) (Outcome, error) {
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

	if *started {
		return "", nil
	}

	capture, err := run.runner.reed.CapturePane(run.state.StrandGUID)
	if err != nil {
		logger.Warn("shuttle: capture pane during startup probe (non-fatal, retrying)", "strandGUID", run.state.StrandGUID, "error", err)
		// A pane that cannot be captured has told us nothing about whether the provider came up,
		// so it stays inside the startup window and expires with it — see classifyStartupWindow.
		return run.classifyStartupWindow(startupDeadline), nil
	}

	switch run.runner.engine.Startup(capture) {
	case StartupReady:
		*started = true
		return "", nil
	case StartupTrustPrompt:
		if err := playInputs(run.runner.reed, run.state.StrandGUID, run.runner.engine.TrustDismissSequence()); err != nil {
			logger.Warn("shuttle: dismiss trust prompt (non-fatal)", "strandGUID", run.state.StrandGUID, "error", err)
		}
	}
	// StartupPending and a trust prompt that has not yet cleared are both "still not ready", and
	// share the one deadline that governs the whole startup window.
	return run.classifyStartupWindow(startupDeadline), nil
}

// classifyStartupWindow reports OutcomeDied once startupDeadline has passed on a run whose provider
// has still not reached StartupReady, and "" while that window is still open.
//
// It is called from every not-yet-started exit of checkLivenessTick, not just the still-booting one,
// because the startup window belongs to the WINDOW rather than to one classification within it.
// Enforcing it only on StartupPending let the two silent paths — a trust prompt whose dismissal
// never takes, and a pane that fails every capture — skip the deadline entirely and burn the full
// run_timeout_min (30 minutes by default) instead of startup_timeout_s (90), and be reported as
// OutcomeTimeout ("the agent was working") rather than OutcomeDied ("it never started").
func (run *Run) classifyStartupWindow(startupDeadline time.Time) Outcome {
	if run.clock.Now().After(startupDeadline) {
		return OutcomeDied
	}
	return ""
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

// finalize builds run's terminal Result and performs cleanup for OutcomeDone.
// For fork mode, audits fork subagents and attaches the result.
//
// It is also the run's teardown observability point, which the Live-Substrate Spawn Observability
// invariant requires to be as instrumented as the spawn is: Start logs "run started" through
// internal/logger, so without the logger.Info below the durable Info+ trace file would show every
// shuttle run beginning and none of them ending. The two cleanup failures go to logger.Warn for the
// same reason — a teardown that did not confirm clean is exactly what that level is for, and the
// bare log package they used before never reaches the trace sink at all.
func (run *Run) finalize(outcome Outcome, message string) (Result, error) {
	result := Result{
		Outcome:              outcome,
		SessionID:            run.state.SessionID,
		StrandGUID:           run.state.StrandGUID,
		LastAssistantMessage: message,
		RunDir:               run.runDir,
	}

	if outcome == OutcomeDone && run.spec.ForkSubagents {
		audit, err := run.runner.engine.AuditForks(run.state.SessionID, run.runner.anchorPath)
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
	if cleaned {
		if _, err := run.runner.reed.RemoveStrand(run.state.StrandGUID, false); err != nil {
			logger.Warn("shuttle: cleanup: remove strand failed (non-fatal)", "strandGUID", run.state.StrandGUID, "error", err)
		}
		if err := os.RemoveAll(run.runDir); err != nil {
			logger.Warn("shuttle: cleanup: remove run dir failed (non-fatal)", "runDir", run.runDir, "error", err)
		}
	}

	logger.Info("shuttle: run finished", "runDir", run.runDir, "strandGUID", run.state.StrandGUID, "sessionID", run.state.SessionID, "outcome", string(outcome), "cleanedUp", cleaned)
	return result, nil
}
