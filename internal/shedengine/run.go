// run.go implements (*Shed).Run and (*Shed).Step, which both drive one shared loop body.
// stepLocked is that body.
// It reads the status file and looks up the current producer.
// It checks pause and cancellation, calls the producer, appends and persists the outcome, and routes on it.
// Run loops stepLocked to a terminal state, and Step runs it once and returns one StepResult.
// Everything a producer does past its own Call return value is invisible to this loop.

package shedengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// ReasonNoOnStuckTarget is the error a blocked halt persists when a Stuck row has no OnStuck
// target and its producer supplied no OutputPointer.Reason.
const ReasonNoOnStuckTarget = "stuck with no OnStuck target"

// ReasonBounceBudgetExhausted is the exact prefix of the error the budget-exhausted blocked arm persists, regardless
// of any OutputPointer.Reason the producer supplied.
// The persisted error continues with the exhausted row and the told resume clause as its way forward,
// so a reader matches it with strings.HasPrefix, never equality.
const ReasonBounceBudgetExhausted = "bounce budget exhausted"

// ReasonPreconditionUnmet is the exact prefix of a blocked halt's reason for an unmet precondition.
// A producer sets it when a precondition the run cannot settle on its own is unmet.
// The reason continues with what is unmet and its way forward as a trailing clause,
// so a reader matches it with strings.HasPrefix, never equality.
// Shed never writes it itself.
const ReasonPreconditionUnmet = "precondition unmet"

// ReasonRerouteCapReached is the exact prefix of the reason the reroute-cap blocked halt persists.
// The reason continues with the capped row, its reroute target, the producer's own reason and the way forward,
// so a reader matches it with strings.HasPrefix, never equality.
const ReasonRerouteCapReached = "reroute cap reached"

// stuckReason normalizes a producer's Reason to one line.
// It trims the text and collapses every run of line-break characters to a single space.
// An empty result falls back to ReasonNoOnStuckTarget.
func stuckReason(reason string) string {
	reason = oneLine(reason)
	if reason == "" {
		return ReasonNoOnStuckTarget
	}
	return reason
}

// oneLine trims text and collapses every run of line-break characters to a single space.
// An empty result stays empty.
func oneLine(text string) string {
	text = strings.TrimSpace(text)
	var b strings.Builder
	inBreak := false
	for _, r := range text {
		if r == '\r' || r == '\n' {
			if !inBreak {
				b.WriteByte(' ')
			}
			inBreak = true
			continue
		}
		inBreak = false
		b.WriteRune(r)
	}
	return b.String()
}

// findProducer looks up name in producers, returning the matching definition and whether it was found.
// It never guesses.
// A caller that gets found == false must hard-error rather than fabricate a fallback.
func findProducer(producers []ProducerDef, name string) (ProducerDef, bool) {
	for _, p := range producers {
		if p.Name == name {
			return p, true
		}
	}
	return ProducerDef{}, false
}

// RowAt returns the row a run whose status names current is at:
// current itself, or the first producer's name when current is empty, the start a fresh seed records.
// It returns current unchanged when producers is empty.
func RowAt(producers []ProducerDef, current string) string {
	if current == "" && len(producers) > 0 {
		return producers[0].Name
	}
	return current
}

// StepResult reports one iteration of the run loop.
// It lets a caller observe and control one iteration from outside.
// Doing so gives up none of the loop's crash-safety or routing guarantees.
// Each field holds the value the loop carries into its next iteration or returns to its caller.
type StepResult struct {
	// Producer is the name of the producer this step called.
	// It is empty when no producer was called this step.
	// That covers the already-done short-circuit and the pause or cancel exit.
	Producer string
	// Outcome is the verdict the called producer returned.
	// It is empty when no producer returned a verdict this step:
	// when Producer is empty, or when a called producer failed under a cancelled context.
	Outcome Outcome
	// Output is the called producer's OutputPointer.Path.
	// It is empty whenever Producer is empty, and for a gate or terminal row that names no artifact.
	Output string
	// Next is current_producer as this step persisted it.
	// It names the producer the following step, or a resumed run, will call.
	Next string
	// State is the State this step persisted alongside Next.
	State State
	// Reason is populated alongside StateBlocked, StateAwaiting and every StatePaused.
	// It is a fired pause condition's reason, "pause requested" for a pause, or "cancelled" for a cancelled context.
	// A pause request or condition that carries a caller label has " by <label>" appended.
	// On a paused step it is the same text the status file records as pause_reason.
	Reason string
	// ParentNotice is the producer's one-line notice for the run's parent, populated only alongside StateAwaiting.
	ParentNotice string
	// History is the full persisted history as it stands when this step returns, not only the
	// entry (if any) this step appended.
	History []HistoryEntry
}

// preflight validates the Shed and creates both lock paths' parent directories before the run lock is acquired.
//
// internal/lock opens a lock file with O_CREATE but never creates its parent directory.
// The paths are still told; preflight only ensures they are usable.
// Callers cannot be assumed to have made the directories, so preflight runs on every call, not only the first.
func (s *Shed) preflight() error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.LockPath), 0o755); err != nil {
		return fmt.Errorf("shedengine: create run lock parent dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.StatusLockPath), 0o755); err != nil {
		return fmt.Errorf("shedengine: create status lock parent dir: %w", err)
	}
	return nil
}

// busyWayForward returns the trailing clause of an ErrShedBusy refusal.
// It addresses the run by runID when one is told and names pauseCommand as the command that halts the live driver.
func busyWayForward(runID, pauseCommand string) string {
	return fmt.Sprintf("way forward: %q halts the live driver at its next producer boundary; check the holder with %s, then retry", pauseCommand, runVerb("status", runID))
}

// pauseCommandFor returns the command a way forward names to stop the run at a producer boundary.
// It is the told command when one is set, and otherwise the default `lym shed pause <run-id> --after <producer>`.
// The run-id is omitted when none is told, and an empty producer reads `<current_producer>`.
func pauseCommandFor(told, runID, producer string) string {
	if told != "" {
		return told
	}
	if producer == "" {
		producer = "<current_producer>"
	}
	if runID == "" {
		return "lym shed pause --after " + producer
	}
	return "lym shed pause " + runID + " --after " + producer
}

// missingStatusWayForward is the trailing clause of a refusal for a missing status file.
const missingStatusWayForward = "way forward: seed the run through its recipe's bootstrap verb, or \"lym shed seed\" for a recipe without one"

// MissingStatusClause returns the told missing-status clause, or the generic one when none is told.
func MissingStatusClause(told string) string {
	if told != "" {
		return told
	}
	return missingStatusWayForward
}

// runVerb renders a quoted `lym shed <verb> [<run-id>]` command for a way-forward text.
func runVerb(verb, runID string) string {
	if runID == "" {
		return fmt.Sprintf("\"lym shed %s\"", verb)
	}
	return fmt.Sprintf("\"lym shed %s %s\"", verb, runID)
}

// gotoCommand renders the quoted goto command a way-forward text names, addressing the run by run-id when one is told.
func gotoCommand(runID, target string) string {
	if runID == "" {
		return fmt.Sprintf("\"lym shed goto --to %s\"", target)
	}
	return fmt.Sprintf("\"lym shed goto %s --to %s\"", runID, target)
}

// budgetWayForward returns the trailing clause of a bounce-budget-exhausted halt, falling back to the run verb that resumes the row.
func (s *Shed) budgetWayForward() string {
	if s.BudgetWayForward != "" {
		return s.BudgetWayForward
	}
	return fmt.Sprintf("%s resumes the row and grants it one more budget", runVerb("run", s.RunID))
}

// stepLocked runs one iteration of the loop body and reports it as a StepResult.
// The caller must already hold the run lock, and stepLocked never acquires or releases it.
func (s *Shed) stepLocked(ctx context.Context) (StepResult, error) {
	// The read gate: a missing status file is a hard error, since Shed never seeds one.
	st, found, err := state.ReadJSON[Status](s.StatusPath, s.StatusLockPath)
	if err != nil {
		return StepResult{}, fmt.Errorf("shedengine: read status file %q: %w", s.StatusPath, err)
	}
	if !found {
		return StepResult{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one; %s", s.StatusPath, MissingStatusClause(s.MissingStatusWayForward))
	}
	if !st.State.valid() {
		return StepResult{}, fmt.Errorf("shedengine: status file %q carries an invalid state %q", s.StatusPath, st.State)
	}

	// The already-done short-circuit sits after step 1's read and before step 2's lookup.
	// A done file whose current_producer is no longer in the list returns cleanly and does not hard-error.
	// A finished task must not become un-queryable because someone later edited the producer list.
	// Next and History come from the file, so a re-step's StepResult matches the original completing step's.
	if st.State == StateDone {
		return StepResult{Next: st.CurrentProducer, State: StateDone, History: st.History}, nil
	}
	// StateBlocked, StateFailed and StateAwaiting do not short-circuit.
	// The loop proceeds and re-calls current_producer.
	// That is how a human resumes, after fixing the cause of the halt or finishing the hand-off.

	// Step 2, the lookup.
	// An empty current_producer is the run's start, the list's first row, so a seed never names a row the recipe lacks.
	// Every write this step makes then names the row itself.
	// A named row that is not found is a hard error that changes nothing on disk.
	// The exception is a row the recipe's Retired map names:
	// the run moves once onto the row the map names and pauses, and the driver hands the pause back.
	// Shed never guesses: it neither restarts from the first producer nor advances to the nearest match.
	// Both fabricate a status nobody confirmed.
	// started records whether the status on disk names a row yet, which step 3b reads.
	started := st.CurrentProducer != ""
	st.CurrentProducer = RowAt(s.Producers, st.CurrentProducer)
	def, ok := findProducer(s.Producers, st.CurrentProducer)
	if !ok {
		if target, retired := s.Retired[st.CurrentProducer]; retired {
			from := st.CurrentProducer
			wayForward := s.RetiredWayForward
			if wayForward == "" {
				wayForward = runVerb("run", s.RunID)
			}
			if s.RetiredMove != nil {
				if err := s.RetiredMove(from, target, st.State != StateRunning); err != nil {
					return StepResult{}, fmt.Errorf("shedengine: move the run off the retired row %s: %w; way forward: fix the cause it names, then %s", from, err, wayForward)
				}
			}
			reason := fmt.Sprintf("the row %s is retired from the recipe, so the run moved to %s and paused; way forward: %s", from, target, wayForward)
			if err := s.persistTransient(target, StatePaused, "", st.History, false, false, "", "", "", reason); err != nil {
				return StepResult{}, err
			}
			return StepResult{Next: target, State: StatePaused, Reason: reason, History: st.History}, nil
		}
		names := make([]string, len(s.Producers))
		for i, p := range s.Producers {
			names[i] = p.Name
		}
		return StepResult{}, fmt.Errorf("shedengine: current_producer %q in %q names no producer in the list; the producer list has changed since the file was last written; way forward: %s moves the run onto a row that exists; valid producers: %s", st.CurrentProducer, s.StatusPath, gotoCommand(s.RunID, "<producer>"), strings.Join(names, ", "))
	}

	// Step 3, the pause and cancellation check.
	// An operator's Ctrl-C and a parent deadline are operational stops, not failures.
	// Both resume exactly like an explicit pause request.
	//
	// A pause_before condition naming the row about to run fires first.
	// It pauses here with its own reason and clears both conditions.
	// It consumes pause_requested only when that flag was set.
	if st.PauseBefore != "" && st.PauseBefore == st.CurrentProducer && ctx.Err() == nil {
		reason := labelled(conditionReason("before", st.PauseBefore), st.PauseBeforeBy)
		if pauseErr := s.persistTransient(st.CurrentProducer, StatePaused, "", st.History, st.PauseRequested, true, "", "", "", reason); pauseErr != nil {
			return StepResult{}, pauseErr
		}
		return StepResult{Next: st.CurrentProducer, State: StatePaused, Reason: reason, History: st.History}, nil
	}
	if st.PauseRequested || ctx.Err() != nil {
		// Clearing the flag in the same persist stops the next step re-pausing forever on the flag it resumes from.
		// The durable record that the run is paused is the state, not the flag.
		// A recorded condition is carried through: a bare pause never drops one.
		reason := labelled(reasonPauseRequested, st.PauseRequestedBy)
		if ctx.Err() != nil {
			reason = reasonCancelled
		}
		if pauseErr := s.persistTransient(st.CurrentProducer, StatePaused, "", st.History, true, false, "", "", "", reason); pauseErr != nil {
			return StepResult{}, pauseErr
		}
		return StepResult{Next: st.CurrentProducer, State: StatePaused, Reason: reason, History: st.History}, nil
	}

	// Step 3a, the entry evaluation.
	// A gated row the run has not entered runs its gate here.
	// A row whose gate does not hold is skipped, possibly several in a chain.
	// After a skip the status holds the row to call, so it is read again.
	gated := def.EntryGate != nil && st.EnteredRow != def.Name
	def, ended, err := s.enter(st, def)
	if err != nil {
		return StepResult{}, err
	}
	if ended != nil {
		return *ended, nil
	}
	if gated {
		st, found, err = state.ReadJSON[Status](s.StatusPath, s.StatusLockPath)
		if err != nil {
			return StepResult{}, fmt.Errorf("shedengine: read status file %q: %w", s.StatusPath, err)
		}
		if !found {
			return StepResult{}, fmt.Errorf("shedengine: status file %q vanished mid-run; Shed refuses to create one", s.StatusPath)
		}
		// The status on disk still records the run's start as empty, so the resolved row is applied again.
		started = st.CurrentProducer != ""
		st.CurrentProducer = RowAt(s.Producers, st.CurrentProducer)
	}

	// Step 3b, the resume write.
	// A run resumed from paused, blocked, failed, or awaiting reaches this point about to call a producer.
	// The status file still says that halt state.
	// For an LLM row the call takes minutes.
	// Meanwhile the file, the status strand, and "lym loom status" all describe a run that is already spawning.
	// The stale error text is cleared with it.
	// Activity.Wait is composed from that text, so the pane would otherwise assert a failure the loop is retrying past.
	//
	// A run at its start, whose status still names no row, takes the same write: the start write.
	// Without it the status names no row until the first row's verdict.
	// A first row that runs for minutes, such as a writer, would show as no row to CommitStatus's readers.
	//
	// This is the only write in the loop that does not record a producer's verdict.
	// It is conditional: on the ordinary running-to-running path it never fires,
	// so the loop's one-persist-per-iteration shape holds for every step after the first.
	if st.State != StateRunning || !started {
		if err := s.persistMarked(true, st.CurrentProducer, StateRunning, "", st.History, false, false, "", "", "", "", false); err != nil {
			return StepResult{}, err
		}
	}

	// Step 4: call the looked-up definition's producer.
	outcome, output, callErr := def.Producer.Call(ctx)

	// Steps 5 and 6 are computed in memory, then committed with one persist call for this iteration.
	// Written as two writes, a crash between them leaves current_producer naming the producer that just finished.
	// The next step would re-call it and append a duplicate history entry.
	// That defeats the crash-safety that step 5 provides.
	//
	// The history entry is appended or folded (see appendOrFold) onto a copy of the history read at step 1.
	// The read slice is never mutated in place.
	// An outcome the producer never reached records nothing at all; see the skip below.
	// routedTo is the row a Stuck verdict's OnStuck edge bounced the run to.
	// It is empty on every entry that routed nowhere.
	appendHistory := func(routedTo string) []HistoryEntry {
		if outcome == "" {
			// A producer that returned an error and no outcome reached no verdict, so there is nothing to record.
			// The entry is skipped rather than given a placeholder value.
			// history[].outcome is a persisted enum: done, stuck and awaiting, plus the history-only goto that no producer returns.
			// No other spelling means "the call did not get that far".
			//
			// Written there, the empty string is out of vocabulary on disk.
			// The seed-coherence check in internal/loomengine rejects it.
			// The status file would then refuse every later resume.
			// That turns one recoverable producer error into a permanently unresumable run.
			// It would also compose into activity.last as a dangling "Plan-Write →" with nothing after the arrow.
			// Dropping it loses nothing: the failure's own text is written to error,
			// and current_producer still names the producer that failed.
			//
			// A non-empty outcome outside the vocabulary is still recorded verbatim.
			// There the value is the diagnosis: it is what the broken adapter actually returned.
			// Copied onto a non-nil slice so an empty history stays [] on disk rather than null.
			return append(make([]HistoryEntry, 0, len(st.History)), st.History...)
		}
		return appendOrFold(st.History, HistoryEntry{
			Producer:     def.Name,
			Outcome:      outcome,
			Output:       output.Path,
			At:           nowRFC3339(),
			GateAttempts: output.GateAttempts,
			GateSpans:    output.GateSpans,
			// A Stuck that blocks at a row with no OnStuck never reaches a budget, so it is exempt too.
			BudgetExempt:    outcome == Stuck && (output.BudgetExempt || output.Halt || def.OnStuck == ""),
			RoutedTo:        routedTo,
			GenerationStart: outcome == Stuck && output.GenerationStart,
			Machine:         s.Machine,
			Block:           def.Block,
		})
	}

	switch {
	case callErr != nil && ctx.Err() != nil:
		// Non-nil error with a cancelled context: the pause exit, exactly as step 3 takes it.
		// The predicate is ctx.Err() != nil, not an errors.Is check against a cancellation sentinel.
		// The context's own state is ground truth about whether an operator stopped the run.
		// It stays correct even if a producer wraps or discards the sentinel.
		// A producer whose own derived context times out returns a deadline error while the parent is healthy:
		// that is a genuine producer failure, not an operator stop.
		//
		// No history entry is appended, because the producer never reached a verdict.
		// current_producer stays put, so the next step re-calls the producer, as it does after a crash before the persist.
		// The accepted trade is that a genuine, unrelated producer error, arriving as an operator cancels, reads as a pause.
		// That is harmless, because the producer is re-called on resume and the real error surfaces then.
		if err := s.persistTransient(st.CurrentProducer, StatePaused, "", st.History, true, false, "", "", "", reasonCancelled); err != nil {
			return StepResult{}, err
		}
		return StepResult{Producer: def.Name, Next: st.CurrentProducer, State: StatePaused, Reason: reasonCancelled, History: st.History}, nil

	case callErr != nil:
		// Non-nil error with a healthy context: an engine-level failure, never a producer verdict.
		// It is never routed anywhere, and a human resolves it.
		// No further producer is called.
		// A persist failure is joined onto the producer failure, not returned in its place.
		// The producer error is why the step halted, and the persist error is a second, independent fault.
		// Replacing one with the other would hide the failure that stopped the step, the only one an operator can act on.
		// A classified failure is wrapped with the transient mark, persisted with its class, and returned marked.
		if s.Transient != nil {
			callErr = MarkTransient(s.Transient(callErr), callErr)
		}
		nextHistory := appendHistory("")
		if persistErr := s.persistTransient(st.CurrentProducer, StateFailed, callErr.Error(), nextHistory, false, false, "", TransientOf(callErr), "", ""); persistErr != nil {
			return StepResult{}, errors.Join(callErr, persistErr)
		}
		return StepResult{}, callErr

	case outcome == Stuck && output.Reroute && !output.Halt:
		return s.rerouteStuck(st, def, output)

	case outcome == Stuck:
		switch {
		// A Halt Stuck blocks at its own row whatever the row's OnStuck, and is budget-exempt through appendHistory.
		case output.Halt || def.OnStuck == "":
			nextHistory := appendHistory("")
			reason := stuckReason(output.Reason)
			if err := s.persist(st.CurrentProducer, StateBlocked, reason, nextHistory, false, ""); err != nil {
				return StepResult{}, err
			}
			return StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: st.CurrentProducer, State: StateBlocked, Reason: reason, History: nextHistory}, nil
		// An exempt Stuck skips the budget comparison below.
		// Comparing would block the first exempt one, since the count already reads N after N counted Stucks.
		// The count argument is st.History, the slice read at step 1, never nextHistory.
		// A post-append read would shift the boundary by one.
		// A generation-start Stuck is counted but never halts: it opens a new episode whose count reads 1.
		case !output.BudgetExempt && !output.GenerationStart && episodeStuckCount(st.History, def, s.Producers) >= effectiveMaxBounces(def, s.MaxBounces):
			// The boundary is exact: a budget of three performs three bounce-backs and blocks on the fourth Stuck.
			// The halting entry is marked budget_capped, so a resume grants one more budget and a goto onto it grants none.
			nextHistory := appendHistory("")
			nextHistory[len(nextHistory)-1].BudgetCapped = true
			reason := fmt.Sprintf("%s for %s; way forward: %s", ReasonBounceBudgetExhausted, def.Name, s.budgetWayForward())
			if err := s.persist(st.CurrentProducer, StateBlocked, reason, nextHistory, false, ""); err != nil {
				return StepResult{}, err
			}
			return StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: st.CurrentProducer, State: StateBlocked, Reason: reason, History: nextHistory}, nil
		default:
			nextHistory := appendHistory(def.OnStuck)
			next, reason, err := s.routedState(def)
			if err != nil {
				return StepResult{}, err
			}
			if err := s.persistRouted(def.OnStuck, next, nextHistory, next == StatePaused, def.OnStuck, reason); err != nil {
				return StepResult{}, err
			}
			return StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: def.OnStuck, State: next, Reason: reason, History: nextHistory}, nil
		}

	case outcome == Awaiting:
		// The planned hand-off halt is never routed: neither OnStuck nor OnDone is read.
		// It is never counted against the bounce budget.
		// current_producer stays put, so the next step re-calls the same producer through step 3b's resume write.
		nextHistory := appendHistory("")
		reason := stuckReason(output.Reason)
		notice := oneLine(output.ParentNotice)
		if err := s.persistTransient(st.CurrentProducer, StateAwaiting, reason, nextHistory, false, false, "", "", notice, ""); err != nil {
			return StepResult{}, err
		}
		return StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: st.CurrentProducer, State: StateAwaiting, Reason: reason, ParentNotice: notice, History: nextHistory}, nil

	case outcome == Done:
		nextHistory := appendHistory("")
		if def.OnDone == "" {
			// current_producer keeps this producer's own name, never the empty string.
			// activity.now is current_producer's name, and Next is the name current_producer held when this step returned.
			// An empty value would leave both meaningless at the terminal.
			// A reader of a finished status file most needs them there.
			// The terminal is chosen by an empty OnDone, not by list position.
			if err := s.persist(def.Name, StateDone, "", nextHistory, false, ""); err != nil {
				return StepResult{}, err
			}
			return StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: def.Name, State: StateDone, History: nextHistory}, nil
		}
		// A non-empty OnDone needs no lookup here.
		// validate has already rejected an OnDone that names no producer in the list,
		// so the name is persisted as-is and step 2's lookup resolves it on the next iteration.
		next, reason, err := s.routedState(def)
		if err != nil {
			return StepResult{}, err
		}
		if err := s.persistRouted(def.OnDone, next, nextHistory, next == StatePaused, "", reason); err != nil {
			return StepResult{}, err
		}
		return StepResult{Producer: def.Name, Outcome: outcome, Output: output.Path, Next: def.OnDone, State: next, Reason: reason, History: nextHistory}, nil

	case outcome == Yield:
		// A yield is a wait the producer's own next call re-checks.
		// Nothing is written, so the run stays at this row, running, exactly as it was.
		// No history entry, no activity, no commit-status call, and no bounce budget, reroute or route is read.
		return StepResult{Producer: def.Name, Outcome: outcome, Next: st.CurrentProducer, State: StateRunning, History: st.History}, nil

	default:
		// An Outcome that is none of Done, Stuck, Awaiting or Yield, returned with a nil error, is an engine-level failure.
		// Outcome is an open string type, so routing would otherwise have an undefined fourth case.
		// Coercing an unknown value to Stuck would consume bounce budget for a broken adapter.
		// Coercing it to Done would advance past a producer that may not have done its work.
		nextHistory := appendHistory("")
		failErr := fmt.Errorf("shedengine: producer %q returned an unrecognised outcome %q", def.Name, outcome)
		// The persist error is joined onto the failure, not returned in its place.
		if persistErr := s.persist(st.CurrentProducer, StateFailed, failErr.Error(), nextHistory, false, ""); persistErr != nil {
			return StepResult{}, errors.Join(failErr, persistErr)
		}
		return StepResult{}, failErr
	}
}

// Run walks the loop in one call, from wherever the status file's current_producer sits.
// It stops on pause or cancellation, blocked, awaiting, done, a yield, or an error.
// A yield ends the loop with RunYielded, and the run is left exactly as it was.
// Result is valid only when the returned error is nil.
// Every hard-error path returns an unpopulated Result alongside its error, so check the error before reading Outcome.
// The producer list carries no routing meaning once Done routes by OnDone.
// It is storage, the order validate iterates in, and cosmetic display order.
func (s *Shed) Run(ctx context.Context) (Result, error) {
	if err := s.preflight(); err != nil {
		return Result{}, err
	}

	// An OS advisory lock is reclaimed on process death, so a killed run never bricks a later resume.
	// internal/state's per-write lock does not substitute for it.
	// That lock is held only for one write, never across a whole Call.
	// Two concurrent runs could therefore both read the same current_producer and both spawn it.
	runLock, locked, err := lock.TryAcquireWriteLock(s.LockPath)
	if err != nil {
		return Result{}, fmt.Errorf("shedengine: acquire run lock %q: %w", s.LockPath, err)
	}
	if !locked {
		return Result{}, fmt.Errorf("%w: %q; %s", ErrShedBusy, s.LockPath, busyWayForward(s.RunID, pauseCommandFor(s.PauseWayForward, s.RunID, "")))
	}
	defer runLock.Release()

	if err := s.refuseUnderRunLock(); err != nil {
		return Result{}, err
	}

	for {
		res, err := s.stepLocked(ctx)
		if err != nil {
			return Result{}, err
		}
		if res.Outcome == Yield {
			return Result{Outcome: RunYielded, HaltedProducer: res.Next, History: res.History}, nil
		}
		if res.State == StateRunning {
			continue
		}
		// RunOutcome(res.State) is a conversion, not a lookup table.
		// shed.go pins RunOutcome's four string values to State's four clean-exit values.
		// StateRunning never reaches this line, since the loop continues above it.
		// StateFailed arrives only alongside a non-nil error, already returned above.
		//
		// HaltedProducer equals res.Next in every arm of stepLocked.
		// There Next is the value persist wrote as current_producer.
		return Result{
			Outcome:        RunOutcome(res.State),
			HaltedProducer: res.Next,
			Reason:         res.Reason,
			History:        res.History,
		}, nil
	}
}

// Step runs exactly one iteration of the loop.
// It holds the run lock for this one call only, and releases it before returning.
//
// Between steps no driver runs, so a caller can drive a shed one step at a time from outside.
// An external supervisor can pause between steps, and a CLI verb can run once per human action.
// Step refuses with ErrShedBusy while another driver holds the lock, rather than racing it.
func (s *Shed) Step(ctx context.Context) (StepResult, error) {
	if err := s.preflight(); err != nil {
		return StepResult{}, err
	}

	runLock, locked, err := lock.TryAcquireWriteLock(s.LockPath)
	if err != nil {
		return StepResult{}, fmt.Errorf("shedengine: acquire run lock %q: %w", s.LockPath, err)
	}
	if !locked {
		return StepResult{}, fmt.Errorf("%w: %q; %s", ErrShedBusy, s.LockPath, busyWayForward(s.RunID, pauseCommandFor(s.PauseWayForward, s.RunID, "")))
	}
	defer runLock.Release()

	if err := s.refuseUnderRunLock(); err != nil {
		return StepResult{}, err
	}

	return s.stepLocked(ctx)
}

// refuseUnderRunLock runs LockedCheck, which must be called with the run lock held.
// A refusal is wrapped in ErrLockedCheckRefused, whose text leads so that the check's own way forward stays trailing.
func (s *Shed) refuseUnderRunLock() error {
	if s.LockedCheck == nil {
		return nil
	}
	if err := s.LockedCheck(); err != nil {
		return fmt.Errorf("%w: %w", ErrLockedCheckRefused, err)
	}
	return nil
}

// nowRFC3339 returns the current UTC time formatted as RFC3339, the format of every history[].at value.
// It reads the clock directly.
// Shed has no injectable clock field, since adding one would widen the struct shape the design pins.
// Tests assert the value structurally rather than by literal.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// episodeStuckCount counts the Stuck entries authored by def within its current episode.
// The episode runs from the most recent Done by def or by a producer sharing def's non-empty Segment.
// The scan returns at the first such Done.
// Otherwise it counts the entries whose Producer is def's name and whose Outcome is Stuck.
// Entries authored by other producers are skipped and never end the scan.
// A Burler-round row returns only Stuck, so its episode ends when its segment's Bouncer passes.
// Otherwise every later rework round would add to one ever-growing count.
// A done entry written by the hard-failure arm also ends the scan.
// That is accepted rather than special-cased.
// State "failed" halts the run, so every later continuation is a fresh human-initiated act.
// A skipped entry ends the episode as a Done does.
// A Stuck entry whose BudgetExempt is true is skipped and never counted.
// An OutcomeGoto entry is skipped like any other non-Done entry, so a goto keeps the spent budget.
// An OutcomeReroute entry ends the episode when its RoutedTo target shares def.Segment.
// When def.Segment is empty, the target must be def itself.
// A target no longer in the list ends no episode.
// A BudgetCapped Stuck entry of def is the one a budget-exhausted halt wrote.
// It ends the scan uncounted, unless the entry right after it is an OutcomeGoto entry.
// In that case it is counted and the scan goes on.
// A resume writes no entry, so it grants one more budget; a goto onto the halted run grants none.
// A GenerationStart Stuck entry of def is counted and ends the scan.
// The count therefore reads 1 right after a generation starts.
func episodeStuckCount(history []HistoryEntry, def ProducerDef, producers []ProducerDef) int {
	name := def.Name
	enders := segmentEnders(producers, def)
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]
		if entry.Outcome == OutcomeGoto {
			continue
		}
		if entry.Outcome == OutcomeReroute {
			if gotoEndsEpisode(entry.RoutedTo, def, producers) {
				return count
			}
			continue
		}
		if (entry.Outcome == Done || entry.Outcome == OutcomeSkipped) && (entry.Producer == name || slices.Contains(enders, entry.Producer)) {
			return count
		}
		if entry.Producer != name {
			continue
		}
		if entry.Outcome != Stuck || entry.BudgetExempt {
			continue
		}
		if entry.BudgetCapped && !(i+1 < len(history) && history[i+1].Outcome == OutcomeGoto) {
			return count
		}
		count++
		if entry.GenerationStart {
			return count
		}
	}
	return count
}

// gotoEndsEpisode reports whether a reroute naming target ends def's bounce-budget episode.
func gotoEndsEpisode(target string, def ProducerDef, producers []ProducerDef) bool {
	if def.Segment == "" {
		return target == def.Name
	}
	for _, p := range producers {
		if p.Name == target {
			return p.Segment == def.Segment
		}
	}
	return false
}

// segmentEnders returns the names of every other producer sharing def's non-empty Segment.
// A Done by any of them ends def's episode.
// A standalone producer has none, so its episode ends only at its own Done.
func segmentEnders(producers []ProducerDef, def ProducerDef) []string {
	if def.Segment == "" {
		return nil
	}
	var names []string
	for _, p := range producers {
		if p.Segment == def.Segment && p.Name != def.Name {
			names = append(names, p.Name)
		}
	}
	return names
}

// appendOrFold returns a fresh copy of history with entry recorded.
// An entry folds into the last element when both are budget-exempt Stucks;
// they must match on Producer, Output, GateAttempts and RoutedTo.
// Otherwise entry is appended unchanged.
// A fold keeps the last element's At and increments its Repeats.
// It sets LastAt to entry.At and appends entry's GateSpans to the last element's own.
// The last element's Repeats, LastAt and spans never take part in the match.
// It never writes through history's backing array or its entries' span arrays.
func appendOrFold(history []HistoryEntry, entry HistoryEntry) []HistoryEntry {
	next := make([]HistoryEntry, len(history), len(history)+1)
	copy(next, history)
	if n := len(next); n > 0 && foldsInto(next[n-1], entry) {
		next[n-1].Repeats++
		next[n-1].LastAt = entry.At
		if len(entry.GateSpans) != 0 {
			spans := make([]GateSpan, 0, len(next[n-1].GateSpans)+len(entry.GateSpans))
			spans = append(spans, next[n-1].GateSpans...)
			next[n-1].GateSpans = append(spans, entry.GateSpans...)
		}
		return next
	}
	return append(next, entry)
}

// foldsInto reports whether entry folds into last.
// Both must be budget-exempt Stucks with equal Producer, Output, RoutedTo and GateAttempts values.
// Neither may be a reroute-cap halt.
func foldsInto(last, entry HistoryEntry) bool {
	if entry.Outcome != Stuck || !entry.BudgetExempt || last.Outcome != Stuck || !last.BudgetExempt {
		return false
	}
	if entry.RerouteCapped || last.RerouteCapped {
		return false
	}
	if last.Producer != entry.Producer || last.Output != entry.Output || last.RoutedTo != entry.RoutedTo {
		return false
	}
	if (last.GateAttempts == nil) != (entry.GateAttempts == nil) {
		return false
	}
	return last.GateAttempts == nil || *last.GateAttempts == *entry.GateAttempts
}

// effectiveMaxBounces resolves def's bounce budget, inheriting at two levels.
// It takes def.MaxBounces when that is greater than zero.
// Otherwise it takes shedMax when that is greater than zero, else defaultMaxBounces.
// A zero value never means "no bounces allowed" at either level.
func effectiveMaxBounces(def ProducerDef, shedMax int) int {
	if def.MaxBounces > 0 {
		return def.MaxBounces
	}
	if shedMax > 0 {
		return shedMax
	}
	return defaultMaxBounces
}

// persist is the single write path for the loop.
// It runs one state.UpdateJSON call whose mutate overwrites the Shed-owned fields:
// current_producer, state, error, history, and activity.
// composeActivity recomputes activity from the values being written.
// When consumePause is true, persist also writes pause_requested as false;
// otherwise pause_requested is left as re-read.
// persist never touches product.
//
// routedTo is the row a Stuck verdict was just routed to, and it is empty for every other write.
// It selects the "<producer> → bounced to <routedTo>" wording of activity.last.
// A persist that appends or folds a history entry recomposes activity.last.
// A persist that records no verdict carries the file's existing activity.last forward, read inside the mutate.
// Those are the pause writes and the step-3b resume write.
// So a pause right after a bounce keeps the bounce wording, and a resume after a halt keeps "<producer> → stuck".
//
// The merge exists rather than a whole-file rewrite from an in-memory copy because Shed is not the
// status file's only writer: a pause requested during a long producer call, and an external
// product update, must both survive.
// The merge is safe against a concurrent external writer that takes the same StatusLockPath.
// It is safe against no other writer, because internal/state's lock is advisory and keyed on the caller-supplied path.
//
// The found guard is not defensive noise.
// state.UpdateJSON treats a missing file as a non-error and writes whatever the mutate returns.
// Without the guard, a status file deleted mid-run would be silently re-created from a zero value.
// That would contradict the rule that Shed never seeds a status file.
//
// The persist keeps every key its type does not know, through internal/state.
// A key an external actor wrote, or a newer binary added, survives each write.
// Malformed JSON still fails loud on this path.
//
// persist calls s.CommitStatus only after the write, once UpdateJSON has returned and released StatusLockPath.
// It passes the transition's nextCurrentProducer and string(nextState).
// It never calls CommitStatus from inside the mutate callback.
// A synchronous network push made while StatusLockPath is held would block every ReadJSON reader on that path.
// Such a push would stall "lym loom status --watch" for its duration, which is the observability the seam serves.
// A nil CommitStatus is a no-op.
// A non-nil error the closure returns propagates out of persist and so halts Run,
// but the status-file write has already happened and is durable; that ordering is load-bearing.
// The accepted cost is a millisecond-scale window in which a reader sees the new state on disk
// before git carries it.
//
// The engine calls CommitStatus on every persist and never skips a call.
// It does not skip one because nextCurrentProducer equals the producer that just ran.
// State, history and error can all change while current_producer does not,
// and the pause and resume writes happen outside any producer call.
// Whether a closure skips some of its calls is the caller's decision, outside this package's guarantee.
// The batten seam (`internal/battencli`) skips its commit when the (producer, state) pair is unchanged,
// so it is blind to a call whose history or error changed while producer and state did not.
// That blindness is safe there because the one transition it skips is the still-running self-bounce.
// That is `battenrecipe.NameRunShed`, whose error is empty by construction:
// it is a Stuck verdict rather than a hard error.
// The history it leaves behind is committed by the next transition that changes producer or state.
func (s *Shed) persist(nextCurrentProducer string, nextState State, nextError string, nextHistory []HistoryEntry, consumePause bool, routedTo string) error {
	return s.persistTransient(nextCurrentProducer, nextState, nextError, nextHistory, consumePause, false, routedTo, "", "", "")
}

// The two reasons of a paused stop that no pause condition fired:
// an operator's pause request, and a cancelled context.
// They are never condition reasons, so a reader that classifies condition stops by prefix does not see them as one.
const (
	reasonPauseRequested = "pause requested"
	reasonCancelled      = "cancelled"
)

// conditionReason returns the reason of a fired pause condition: "paused <when> <target>, as requested".
func conditionReason(when, target string) string {
	return fmt.Sprintf("paused %s %s, as requested", when, target)
}

// labelled returns text followed by " by <by>" when by is set, and text alone otherwise.
func labelled(text, by string) string {
	if by == "" {
		return text
	}
	return text + " by " + by
}

// routedState returns the state a routed running outcome of def persists.
// It is StatePaused when the status file's pause_after names def, and StateRunning otherwise.
// pause_after is read fresh, since an outside actor may have recorded it while def ran.
// The paused reason is the pause_after reason, followed by the condition's own label when one is set;
// the running reason is empty.
func (s *Shed) routedState(def ProducerDef) (State, string, error) {
	cur, found, err := state.ReadJSON[Status](s.StatusPath, s.StatusLockPath)
	if err != nil {
		return "", "", fmt.Errorf("shedengine: read status file %q: %w", s.StatusPath, err)
	}
	if found && cur.PauseAfter != "" && cur.PauseAfter == def.Name {
		return StatePaused, labelled(conditionReason("after", cur.PauseAfter), cur.PauseAfterBy), nil
	}
	return StateRunning, "", nil
}

// recordedVerdict reports whether writing next records a verdict against cur, the current history.
// A verdict is a longer history, or an equal-length one whose last entry's Repeats changed, which is a fold.
func recordedVerdict(cur, next []HistoryEntry) bool {
	if len(next) > len(cur) {
		return true
	}
	if len(next) == 0 || len(next) != len(cur) {
		return false
	}
	return next[len(next)-1].Repeats != cur[len(cur)-1].Repeats
}

// persistTransient writes the status file, recording the transient class and the parent notice.
// Every write sets Transient and ParentNotice, so a later step never carries a stale class or notice.
// clearConditions is true only for the write that records paused for a fired condition;
// every other write carries a recorded condition through unchanged.
// A write that appends a history entry hands the pending caller label to the appended entry and clears it;
// a write that appends none carries the label through.
// pauseReason is recorded as pause_reason when nextState is StatePaused.
// It keeps the entered row, so a resume or a halt re-calls the row without evaluating its entry gate.
func (s *Shed) persistTransient(nextCurrentProducer string, nextState State, nextError string, nextHistory []HistoryEntry, consumePause, clearConditions bool, routedTo string, transient TransientClass, parentNotice, pauseReason string) error {
	return s.persistMarked(false, nextCurrentProducer, nextState, nextError, nextHistory, consumePause, clearConditions, routedTo, transient, parentNotice, pauseReason, false)
}

// persistRouted writes a routed running outcome or a skip, with an empty error, transient class and parent notice.
// It clears the entered row, so a row is entered only between its entry evaluation and its verdict;
// a route back into it evaluates its gate again.
// It records ResumedBy as the pending caller label, since a skip can be the first write of a resumed run.
func (s *Shed) persistRouted(nextCurrentProducer string, nextState State, nextHistory []HistoryEntry, clearConditions bool, routedTo, pauseReason string) error {
	return s.persistMarked(true, nextCurrentProducer, nextState, "", nextHistory, false, clearConditions, routedTo, "", "", pauseReason, true)
}

// persistMarked writes the status file for one transition of the run.
// When markResume is true, it records ResumedBy as the pending caller label.
// It does so only over a blocked, failed or awaiting status with no label pending.
// The first label of a resumed run therefore stands, and a resume from paused records nothing.
// A write recording StatePaused sets pause_reason to pauseReason, and every other write clears it.
// Consuming pause_requested also clears pause_requested_by.
// Clearing the conditions clears both condition labels.
// A write that appends a skipped entry keeps a pending label for the next verdict's entry instead of the skipped one.
// clearEntered clears the entered row in the same write.
func (s *Shed) persistMarked(markResume bool, nextCurrentProducer string, nextState State, nextError string, nextHistory []HistoryEntry, consumePause, clearConditions bool, routedTo string, transient TransientClass, parentNotice, pauseReason string, clearEntered bool) error {
	err := state.UpdateJSON(s.StatusPath, s.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-run; Shed refuses to create one", s.StatusPath)
		}
		if len(nextHistory) > len(cur.History) && cur.PendingBy != "" && nextHistory[len(nextHistory)-1].Outcome != OutcomeSkipped {
			nextHistory = slices.Clone(nextHistory)
			nextHistory[len(nextHistory)-1].By = cur.PendingBy
			cur.PendingBy = ""
		}
		if markResume && s.ResumedBy != "" && cur.PendingBy == "" && resumableHalt(cur.State) {
			cur.PendingBy = s.ResumedBy
		}
		prevLast := cur.Activity.Last
		recorded := recordedVerdict(cur.History, nextHistory)
		cur.CurrentProducer = nextCurrentProducer
		cur.State = nextState
		cur.Error = nextError
		cur.Transient = string(transient)
		cur.ParentNotice = parentNotice
		cur.PauseReason = ""
		if nextState == StatePaused {
			cur.PauseReason = pauseReason
		}
		cur.History = nextHistory
		cur.Activity = composeActivity(nextCurrentProducer, nextHistory, nextState, nextError, routedTo)
		if !recorded {
			cur.Activity.Last = prevLast
		}
		if clearEntered {
			cur.EnteredRow = ""
		}
		if consumePause {
			cur.PauseRequested = false
			cur.PauseRequestedBy = ""
		}
		if clearConditions {
			cur.PauseBefore = ""
			cur.PauseAfter = ""
			cur.PauseBeforeBy = ""
			cur.PauseAfterBy = ""
		}
		return cur, nil
	})
	if err != nil {
		return err
	}
	if s.CommitStatus == nil {
		return nil
	}
	return s.CommitStatus(nextCurrentProducer, string(nextState))
}
