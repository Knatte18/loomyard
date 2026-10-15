// gate.go declares the gate contract: the exported types a caller uses to attach mechanical validators to a gated run and read its report (Gate, GateEntry, GateSpec, GateResult, GateOutcome, GateEntryOutcome, GateEntryState, GateSpan, GateSpanKind), plus the package-private helpers the run loop (run.go, wait.go) spends them through.
// It inherits the Shuttle Provider-Seam Invariant (doc.go): the gate loop asks "is a new event in"
// only through the existing Engine.ParseEvents seam, and knows nothing about any provider's own hook
// payloads.

package shuttleengine

import (
	"fmt"
	"time"
)

// gateFindingsFileName is the file name a failed gate's findings are written to inside the run
// directory, overwritten by every subsequent failed attempt.
const gateFindingsFileName = "gate-findings.md"

// GateResult is one gate closure invocation's verdict.
type GateResult struct {
	// Passed reports whether the gate found no defect.
	Passed bool
	// Findings is the detailed what-is-wrong text, empty when Passed. It is written to a per-run
	// findings file and never sent inline — see the "findings always ride a file" decision.
	Findings string
	// Pending reports that the gate has no verdict yet and holds the run: the wait loop keeps polling, re-evaluating at turn boundaries, without re-prompting or counting a failure.
	// Only a PassOnCap or MayHold entry may return it, and a Pending result is never Passed.
	// The run deadline and liveness checks keep running while an entry is pending, and a deadline or liveness finalize evaluates the entry's Final closure instead.
	Pending bool
	// Send is one line the wait loop sends the writer when the result is Pending, empty for none.
	// The closure never sends; the loop does, only at a turn boundary.
	Send string
	// SendFailedWayForward is the clause the loop's Warn quotes when sending Send fails.
	SendFailedWayForward string
	// Terminal marks a failing result the run finalizes at once, with no re-prompt, whatever the entry's in-memory failure count; the count is not incremented for it.
	// Its Findings text rides GateOutcome.Reason to the producer, because finalize deletes the findings file with the run directory.
	// Only a must-pass entry may return it: Terminal on a passed or pending result, or from a PassOnCap entry, is a returned gate error.
	Terminal bool
}

// Gate is a mechanical validator a gated run consults at each arrival and at its single verdict site (finalize):
// evaluate the run's declared output artifacts and report GateResult.
// It takes no argument, because the two production validators (planparser.ParsePlan,
// discussionparser's own validator) take materially different path shapes, and a closure captures
// its own told paths rather than this seam threading them through.
type Gate func() (GateResult, error)

// GateEntry is one gate in a GateSpec: a validator closure with its own re-prompt budget.
// A closure answers passed, failed or pending; a pending answer holds the run at a turn boundary (see GateResult.Pending).
type GateEntry struct {
	// Name labels the entry in GateOutcome.Entries and in the re-prompt log line.
	Name string
	// Gate is the validator this entry consults.
	Gate Gate
	// Forked, when non-nil, is the closure evaluated in place of Gate for a run whose record carries fork authorization.
	// Final keeps precedence over it at a final evaluation.
	Forked Gate
	// Final, when non-nil, is the closure finalize evaluates in place of Gate, so a pending-capable entry is read at the final verdict without its opening and consuming transitions.
	// Only a PassOnCap or MayHold entry may set it: a non-nil Final on any other entry is a returned gate error at evaluation, so Final can never let a must-pass entry through.
	Final Gate
	// Expired, when non-nil, is a read-only probe reporting whether the entry's wait has run out; nil means none.
	// It sends, counts and writes nothing, and only an entry whose Attempts is above zero is probed.
	// The loop calls it only while the writer is not at an idle boundary, where the entry's own closure handles its expiry,
	// so a wait bound ends the run even when the writer never ends its turn.
	Expired func() (bool, error)
	// Attempts is the entry's re-prompt budget: how many consecutive failures re-prompt the agent before the entry gives up.
	// 0 means the entry is off: it is skipped at every arrival, whatever its PassOnCap.
	Attempts int
	// PassOnCap lets the run through, rather than failing it, once the entry has failed Attempts consecutive times:
	// from then on the entry's closure is not run again and the entry is reported let through.
	// The count lives in memory for one shuttle run, so after an attach a capped entry starts from zero and can fire again,
	// and a pass before the cap resets the count, so the entry can fire again later;
	// a request that must not repeat makes its own closure idempotent.
	// A PassOnCap entry may also pend and set Final; every other entry needs MayHold for that.
	PassOnCap bool
	// MayHold lets a must-pass entry return Pending and set Final, which evaluateGate otherwise refuses on an entry that is not PassOnCap.
	// It lifts those two guards and nothing else: the entry stays must-pass, a failure sets GateOutcome.Passed false, and a Pending result never counts as passed, so finalize never reads a held entry as a pass.
	// A Pending result at a turn boundary holds the run exactly as a PassOnCap entry's does.
	// Only the parent-review wiring sets it.
	MayHold bool
	// Reprompt, when non-nil, renders the re-prompt line the wait loop sends after this entry fails with budget remaining, in place of the default line; nil keeps the default.
	// It receives the findings file's path.
	// The line is validated where every send is: a result of several lines, or an empty or whitespace-only line, fails the send and ends the loop with the attempts spent so far.
	Reprompt func(findingsPath string) string
}

// GateSpec is the ordered list of gate entries a gated run consults at each arrival — the one value every downstream seam (RunGated, AttachGated, a producer's RunOpts.Gate) carries from the point it is known through to the run loop, per the "one GateSpec at every hop" decision.
// An empty or nil list means "ungated", which is what every row but the gated ones supplies.
type GateSpec []GateEntry

// GateEntryState is one entry's state at the final arrival a GateOutcome reports.
type GateEntryState string

const (
	// GateEntryPassed marks an entry whose closure ran and passed.
	GateEntryPassed GateEntryState = "passed"
	// GateEntryFailed marks the entry whose closure ran and failed, stopping the evaluation.
	GateEntryFailed GateEntryState = "failed"
	// GateEntryLetThrough marks a PassOnCap entry past its cap, whose closure was not run.
	GateEntryLetThrough GateEntryState = "let_through"
	// GateEntryOff marks an entry with Attempts 0, wherever it sits in the list.
	GateEntryOff GateEntryState = "off"
	// GateEntryWaiting marks the entry whose closure returned pending, stopping the evaluation; a PassOnCap entry counts toward Passed like a let-through entry, a MayHold must-pass entry does not.
	GateEntryWaiting GateEntryState = "waiting"
	// GateEntryNotReached marks a non-off entry after the entry that stopped the evaluation.
	GateEntryNotReached GateEntryState = "not_reached"
)

// GateEntryOutcome is one entry's line in GateOutcome.Entries.
type GateEntryOutcome struct {
	// Name is the entry's GateEntry.Name.
	Name string
	// Attempts counts the re-prompts sent for this entry.
	Attempts int
	// State is the entry's state at the final arrival.
	State GateEntryState
	// Spans are the time spans this entry took on the run clock, in the order they started; nil when none was recorded.
	Spans []GateSpan
}

// GateSpanKind names what a GateSpan measured.
type GateSpanKind string

const (
	// GateSpanGate is one run of the entry's closure.
	GateSpanGate GateSpanKind = "gate"
	// GateSpanHeld is the time the entry was pending and held the run.
	GateSpanHeld GateSpanKind = "held"
	// GateSpanRework is the time from a delivered re-prompt for the entry to the writer's next arrival or the run's end.
	GateSpanRework GateSpanKind = "rework"
)

// GateSpan is one span of a gate entry, measured on the run clock.
// A span whose End reads before its Start is kept as recorded.
type GateSpan struct {
	Kind  GateSpanKind
	Start time.Time
	End   time.Time
}

// GateOutcome is a gated run's 1:1 gate report, carried on Result.Gate.
type GateOutcome struct {
	// Passed reports whether every entry that is neither off nor PassOnCap passed at the run's final arrival.
	Passed bool
	// Entries reports each entry in list order.
	Entries []GateEntryOutcome
	// Attempts counts re-prompts actually SENT on this run over all entries, and nothing else:
	// a gate that passed first try reports 0;
	// a Done reached with no live session reports however many re-prompts had already been sent before the session was lost (0 when it was lost before the first one);
	// a deadline that expires after N sends reports N.
	// See the "attempts counts re-prompts actually sent" decision.
	Attempts int
	// FindingsPath is the absolute path of the findings file the failing entry wrote, or empty when Passed.
	// It is diagnostic text ONLY, never a path to dereference after Wait returns: the
	// findings file lives in the run directory that finalize deletes on the Done cleanup every
	// exhausted gate takes, so a producer that opens it is a defect.
	FindingsPath string
	// Reason is the Findings text of the terminal failing result, empty otherwise.
	// It rides the outcome because finalize deletes the run directory and the findings file with it, so a terminal entry's explanation reaches the producer only here.
	Reason string
}

// endReworkSpans ends every open rework span at at.
func (run *Run) endReworkSpans(at time.Time) {
	for i, since := range run.gateReworkSince {
		if since.IsZero() {
			continue
		}
		run.gateSpans[i] = append(run.gateSpans[i], GateSpan{Kind: GateSpanRework, Start: since, End: at})
		run.gateReworkSince[i] = time.Time{}
	}
}

// closeGateSpans ends the open held span, if any, at at.
func (run *Run) closeGateSpans(at time.Time) {
	if run.gateHeldAt < 0 || run.gateHeldAt >= len(run.gateSpans) {
		return
	}
	run.gateSpans[run.gateHeldAt] = append(run.gateSpans[run.gateHeldAt], GateSpan{Kind: GateSpanHeld, Start: run.gateHeldSince, End: at})
	run.gateHeldAt = -1
}

// withGateSpans returns a copy of outcome whose entries carry copies of the run's spans, so the memoised verdict is never written through.
func (run *Run) withGateSpans(outcome *GateOutcome) *GateOutcome {
	stamped := *outcome
	stamped.Entries = make([]GateEntryOutcome, len(outcome.Entries))
	copy(stamped.Entries, outcome.Entries)
	for i := range stamped.Entries {
		if i < len(run.gateSpans) && len(run.gateSpans[i]) != 0 {
			stamped.Entries[i].Spans = append([]GateSpan(nil), run.gateSpans[i]...)
		}
	}
	return &stamped
}

// gateRepromptText returns the default single-line re-prompt text Wait sends the agent after a failed gate
// attempt, naming findingsPath and instructing the agent to read it, fix every finding, and end its
// turn. The returned text carries no newline, the one-line shape validateSendText (run.go) requires.
// An entry whose Reprompt is set supplies its own line instead.
func gateRepromptText(findingsPath string) string {
	return fmt.Sprintf("Gate findings recorded at %s — read it, fix every finding, and end your turn.", findingsPath)
}
