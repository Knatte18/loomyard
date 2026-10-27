// producer.go defines the ShedProducer seam Shed drives once per iteration -- the minimal contract
// a producer must satisfy so Shed can call, resume, and route it uniformly regardless of what the
// producer does internally.

package shedengine

import "context"

// Outcome is one producer call's verdict.
type Outcome string

// The three legal Outcome values. A ShedProducer must return exactly one of these;
// any other value is an engine-level failure, not a fourth verdict.
// Awaiting is the planned human hand-off: the run halts in state awaiting, with no routing, no
// bounce budget spent, and the producer re-called on resume.
const (
	Done     Outcome = "done"
	Stuck    Outcome = "stuck"
	Awaiting Outcome = "awaiting"
)

// Yield is the verdict of a producer at a wait its own next call re-checks.
// It persists nothing: no history entry, no status write and no activity, so the run stays at the same row and running.
// Only such a producer returns it, and batten's InnerRun is the only one.
// It has no State twin; the run it ends reports RunYielded.
const Yield Outcome = "yield"

// OutcomeGoto is a history-only Outcome, written by `lym shed goto` and never returned by a producer.
// A producer returning it is unrecognised, as any value outside the three verdicts is.
// A goto entry leaves every row's bounce-budget count as it was.
// It is not named Goto because the Goto function owns that name.
const OutcomeGoto Outcome = "goto"

// OutcomeReroute is a history-only Outcome, written by the reroute arm of a Stuck verdict whose OutputPointer.Reroute is set and never returned by a producer.
// A reroute entry ends the bounce-budget episode of every row sharing its routed target's Segment.
const OutcomeReroute Outcome = "reroute"

// OutcomeSkipped is a history-only Outcome, written when a row's EntryGate does not hold and never returned by a producer.
// A skipped entry carries the gate's reason, routes by the row's OnDone as a Done does, spends no bounce budget and ends the row's bounce-budget episode as a Done does.
const OutcomeSkipped Outcome = "skipped"

// EntryGate decides whether a row runs when the run enters it.
// It reports holds true to run the row; otherwise the row is skipped and reason, which names the check with the values it read, is recorded on the skipped history entry.
// A non-nil err means the gate could not decide: the run is written failed and the row is not entered.
type EntryGate func() (holds bool, reason string, err error)

// OutputPointer names a producer's artifact for a human to read.
// Shed never introspects Path's contents, never validates it, and never stats it to make a
// control-flow decision -- step 4 is an unconditional re-call.
type OutputPointer struct {
	Path string // "" = no artifact (gate or terminal producer)
	// GateAttempts is this call's gate re-prompt count, carried straight through to the
	// persisted HistoryEntry.GateAttempts (see that field's own doc comment). Nil means this
	// producer applied no gate to this call; a gated producer sets it regardless of Path,
	// including on the gate-failed exit whose Path is deliberately empty.
	GateAttempts *int
	// GateSpans are this call's gate spans, carried straight through to the persisted HistoryEntry.GateSpans.
	// Nil means the call recorded none.
	GateSpans []GateSpan
	// Reason is one human-readable line naming why the call is stuck or awaiting. Shed reads it
	// only on a Stuck verdict whose row has no OnStuck target, persisting it as the blocked halt's
	// error, and on an Awaiting verdict, persisting it as the awaiting halt's error; every other
	// arm ignores it. The empty value means "none supplied".
	Reason string
	// ParentNotice is advisory text for the run's parent.
	// Shed reads it only on an Awaiting verdict, persisting it on one line on the status file and returning it on the step result, and never routes on it.
	ParentNotice string
	// BudgetExempt marks a Stuck verdict as not counted against its row's bounce budget, for a producer whose Stuck is a wait with its own bound.
	// Shed reads it only on a Stuck verdict and ignores it on every other outcome.
	BudgetExempt bool
	// Reroute marks a Stuck verdict as taking the row's OnReroute edge instead of OnStuck.
	// Shed reads it only on a Stuck verdict; a row with no OnReroute that returns it is a wiring fault.
	Reroute bool
	// Halt marks a Stuck verdict as blocking the run at its own row with Reason, whatever the row's OnStuck, and as not counted against the bounce budget.
	// Shed reads it only on a Stuck verdict and it can only stop a run, never advance or route it.
	Halt bool
	// GenerationStart marks a Stuck verdict as the one that starts a new review generation.
	// Shed reads it only on a Stuck verdict: the verdict is recorded as counted but never halts on the budget, and the episode count restarts at it.
	// Only a producer that starts a generation sets it, once per generation.
	GenerationStart bool
}

// ShedProducer is the seam Shed drives once per iteration: Call runs one producer to a verdict
// and reports it, an optional output pointer, and a hard error.
// Two obligations Shed cannot enforce mechanically bind every implementation: return exactly
// one of the three legal outcomes (Done, Stuck or Awaiting), and surface context cancellation as a non-nil error, never as Stuck.
type ShedProducer interface {
	Call(ctx context.Context) (Outcome, OutputPointer, error)
}

// ProducerDef is one entry in Shed's flat, ordered producer list: the seam plus the routing and
// budget config that governs it, entirely independent of the entry's position in the list.
type ProducerDef struct {
	// Name is the identity current_producer names on disk; it must be non-empty.
	Name string
	// Producer is the seam this entry drives; it must be non-nil.
	Producer ShedProducer
	// OnStuck is what makes a bounce-back a per-producer config value rather than a hardcoded
	// branch in the loop: "" escalates to a human, else Stuck bounces back to the Name it names.
	OnStuck string
	// OnReroute names the one row a Stuck verdict carrying OutputPointer.Reroute goes to, forward or backward and across segments; "" means the row cannot reroute.
	OnReroute string
	// OnDone is the sole router for a Done verdict, forward or backward through the list. The
	// empty value is load-bearing and silent, exactly as OnStuck: "" already is: an omitted
	// OnDone ends the whole Shed run quietly rather than failing loud, while a non-empty OnDone
	// jumps to the Name it names regardless of this entry's list position.
	OnDone string
	// Segment is a grouping label; "" means standalone. It has two mechanical effects:
	// validate()'s rule that a non-empty OnStuck must name a target sharing this producer's
	// Segment, and the bounce budget's episode, which a Done by any row of the same Segment ends.
	Segment string
	// MaxBounces is this producer's own episode Stuck budget. 0 means "inherit Shed.MaxBounces",
	// never "no bounces allowed".
	MaxBounces int
	// EntryGate is evaluated when the run enters the row; nil is an ungated row that always runs.
	// A gate that does not hold skips the row through OnDone.
	EntryGate EntryGate
}
