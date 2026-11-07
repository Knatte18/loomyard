// producer.go defines the ShedProducer seam Shed drives once per iteration: the minimal contract
// a producer satisfies so Shed can call, resume and route it uniformly, whatever it does internally.

package shedengine

import "context"

// Outcome is one producer call's verdict.
type Outcome string

// The three legal Outcome values. A ShedProducer must return exactly one of these;
// any other value is an engine-level failure, not a fourth verdict.
// Awaiting is the planned human hand-off: the run halts in state awaiting with no routing and no bounce budget spent,
// and the producer is re-called on resume.
const (
	Done     Outcome = "done"
	Stuck    Outcome = "stuck"
	Awaiting Outcome = "awaiting"
)

// Yield is the verdict of a producer at a wait its own next call re-checks.
// It persists nothing: no history entry, no status write and no activity, so the run stays at the same row and running.
// It has no State twin; the run it ends reports RunYielded.
const Yield Outcome = "yield"

// OutcomeGoto is a history-only Outcome, written by `lym shed goto` and never returned by a producer.
// A producer returning it is unrecognised, as any value outside the three verdicts is.
// A goto entry leaves every row's bounce-budget count as it was.
const OutcomeGoto Outcome = "goto"

// OutcomeReroute is a history-only Outcome written when a Stuck verdict asks for a reroute.
// It is never returned by a producer.
// A reroute entry ends the bounce-budget episode of every row sharing its routed target's Segment.
const OutcomeReroute Outcome = "reroute"

// OutcomeSkipped is a history-only Outcome written when a row's EntryGate does not hold.
// It is never returned by a producer.
// A skipped entry carries the gate's reason and routes by the row's SkipTo when set and by its OnDone otherwise.
// It spends no bounce budget and ends the row's bounce-budget episode, as a Done does.
const OutcomeSkipped Outcome = "skipped"

// EntryGate decides whether a row runs when the run enters it.
// It reports holds true to run the row.
// Otherwise the row is skipped and reason is recorded on its skipped history entry.
// The reason names the check with the values it read.
// A non-nil err means the gate could not decide: the run is written failed and the row is not entered.
type EntryGate func() (holds bool, reason string, err error)

// OutputPointer names a producer's artifact for a human to read.
// The engine never introspects Path's contents, never validates it, and never stats it to make a control-flow decision.
type OutputPointer struct {
	Path string // "" = no artifact (gate or terminal producer)
	// GateAttempts is this call's gate re-prompt count, carried through to the persisted HistoryEntry.GateAttempts.
	// Nil means the producer applied no gate to this call.
	// A gated producer sets it regardless of Path.
	// That covers the gate-failed exit, whose Path is empty.
	GateAttempts *int
	// GateSpans are this call's gate spans, carried straight through to the persisted HistoryEntry.GateSpans.
	// Nil means the call recorded none.
	GateSpans []GateSpan
	// Reason is one human-readable line naming why the call is stuck or awaiting.
	// On a Stuck verdict whose row has no OnStuck target, the engine persists it as the blocked halt's error.
	// On an Awaiting verdict the engine persists it as the awaiting halt's error.
	// Every other arm ignores it, and an empty value means none was supplied.
	Reason string
	// ParentNotice is advisory text for the run's parent.
	// On an Awaiting verdict the engine persists it on one status-file line and returns it; it never routes on it.
	ParentNotice string
	// BudgetExempt marks a Stuck verdict as not counted against its row's bounce budget.
	// Set it on a Stuck that is a wait with its own bound; the engine ignores it on every other outcome.
	BudgetExempt bool
	// Reroute marks a Stuck verdict as taking the row's OnReroute edge instead of OnStuck.
	// The engine reads it only on a Stuck verdict; a row with no OnReroute that returns it is a wiring fault.
	Reroute bool
	// Halt marks a Stuck verdict as blocking the run at its own row with Reason, whatever the row's OnStuck.
	// It is not counted against the bounce budget and can only stop a run, never advance or route it.
	Halt bool
	// GenerationStart marks a Stuck verdict as the one that starts a new review generation.
	// That verdict is recorded as counted, never halts on the budget, and restarts the episode count.
	// Only a producer that starts a generation sets it, once per generation.
	GenerationStart bool
}

// ShedProducer is the seam Shed drives once per iteration.
// Call runs one producer to a verdict and reports it, an optional output pointer, and a hard error.
// Two obligations bind every implementation, and Shed cannot enforce either mechanically:
// a producer returns exactly one of the three legal outcomes, Done, Stuck or Awaiting;
// it surfaces context cancellation as a non-nil error, never as Stuck.
type ShedProducer interface {
	Call(ctx context.Context) (Outcome, OutputPointer, error)
}

// ProducerDef is one entry in the engine's flat, ordered producer list:
// the seam plus the routing and budget config that governs it, independent of the entry's position in the list.
type ProducerDef struct {
	// Name is the identity current_producer names on disk; it must be non-empty.
	Name string
	// Producer is the seam this entry drives; it must be non-nil.
	Producer ShedProducer
	// OnStuck makes a bounce-back a per-producer config value rather than a hardcoded branch in the loop.
	// An empty value escalates to a human; otherwise Stuck bounces back to the Name it names.
	OnStuck string
	// OnReroute is the row a Stuck verdict with Reroute set goes to, across segments; "" means the row cannot reroute.
	OnReroute string
	// OnDone is the sole router for a Done verdict, forward or backward through the list.
	// An empty OnDone ends the whole run quietly rather than failing loud.
	// A non-empty OnDone jumps to the Name it names, whatever this entry's list position.
	// The empty value is load-bearing and silent, as an empty OnStuck is.
	OnDone string
	// Segment is a grouping label; "" means standalone. It has two mechanical effects:
	// one is the rule that a non-empty OnStuck names a row sharing this Segment;
	// the other is the bounce budget's episode, which a Done by any row of the same Segment ends.
	Segment string
	// MaxBounces is this entry's own episode Stuck budget.
	// Zero inherits the engine's budget; it never means no bounces are allowed.
	MaxBounces int
	// EntryGate is evaluated when the run enters the row; nil is an ungated row that always runs.
	// A gate that does not hold skips the row through SkipTo when set and through OnDone otherwise.
	EntryGate EntryGate
	// SkipTo is the row a skipped entry of this row routes to; "" means the row skips by OnDone.
	// A row whose skip must jump past the rows its Done enters, such as the row that opens a block, sets it.
	// It must name a producer in the list.
	SkipTo string
	// Block is the row's block path: the names of the block rows enclosing it, outermost first, joined by "/".
	// "" is a row outside every block.
	// The engine only copies it onto the history entries the row writes.
	Block string
}

// skipTarget returns the row a skip of d routes to: SkipTo when set, OnDone otherwise.
// It is the one place a skip's route is read.
func (d ProducerDef) skipTarget() string {
	if d.SkipTo != "" {
		return d.SkipTo
	}
	return d.OnDone
}
