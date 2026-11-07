// status.go declares the persisted status-file shape and its JSON tags.
// The State type here is not internal/state, the persistence package.
// Keep the two apart when reading identifiers here and in their callers.

package shedengine

import "encoding/json"

// State is the status file's lifecycle field.
// It is a superset of RunOutcome, adding running and failed, which Run never returns.
type State string

// The six legal State values.
// A persisted value outside this set, including the empty string, is a hard error at the read gate.
// StateAwaiting is the planned hand-off halt.
// It behaves like blocked wherever a halt is observed, but it is no failure and spends no bounce budget.
const (
	StateRunning  State = "running"
	StatePaused   State = "paused"
	StateDone     State = "done"
	StateBlocked  State = "blocked"
	StateFailed   State = "failed"
	StateAwaiting State = "awaiting"
)

// valid reports whether s is one of the six legal State constants.
// The empty string is rejected rather than tolerated:
// State is a mandatory enum string read from a file an external actor seeds,
// so a typo or a partial seed would otherwise be silently treated as running, or as done.
// PauseRequested (bool) and History (slice) tolerate their zero value, since neither is a mandatory enum string.
func (s State) valid() bool {
	switch s {
	case StateRunning, StatePaused, StateDone, StateBlocked, StateFailed, StateAwaiting:
		return true
	default:
		return false
	}
}

// Activity is the human-facing summary Shed fills mechanically on every persist.
type Activity struct {
	Now  string `json:"now"`
	Last string `json:"last"`
	Wait string `json:"wait"`
}

// The kinds of GateSpan.Kind.
const (
	// GateSpanKindGate is one run of a gate entry's closure.
	GateSpanKindGate = "gate"
	// GateSpanKindHeld is the time a gate entry was pending and held the run.
	GateSpanKindHeld = "held"
	// GateSpanKindRework is the time from a delivered re-prompt to the writer's next arrival or the run's end.
	GateSpanKindRework = "rework"
)

// GateSpan is one span of a gate entry, as a history entry stores it.
// It is a plain struct of strings so shedengine imports nothing for it.
type GateSpan struct {
	// Gate is the gate entry's name as recorded.
	Gate string `json:"gate"`
	// Kind is one of the GateSpanKind constants.
	Kind string `json:"kind"`
	// Start and End are RFC 3339 UTC times with fractional seconds, so a sub-second span keeps its length.
	Start string `json:"start"`
	End   string `json:"end"`
}

// CallerShed is the caller label of the automatic steps that lym itself takes.
const CallerShed = "shed"

// HistoryEntry is one producer call's durable record, and the element type of Result.History.
// A budget-exempt Stuck folds into an immediately preceding identical exempt Stuck rather than appending,
// so one entry can stand for a run of consecutive calls.
type HistoryEntry struct {
	Producer string  `json:"producer"`
	Outcome  Outcome `json:"outcome"`
	Output   string  `json:"output"`
	At       string  `json:"at"`
	// GateAttempts is the gate's own re-prompt count (shuttleengine.GateOutcome.Attempts).
	// It is carried through so the status file shows it, as the producer-gate contract requires:
	// an explicit attempt counter bounds the loop.
	// Nil means this call's producer applied no gate.
	// A non-nil 0 is a gate that passed on its first try, a verdict distinct from ungated.
	// Collapsing the two into nil would silently lose it.
	GateAttempts *int `json:"gate_attempts,omitempty"`
	// GateSpans are the time spans this call's gate entries took, in the order they started.
	// A status file written before the field existed decodes as nil, and a nil list is not written.
	GateSpans []GateSpan `json:"gate_spans,omitempty"`
	// BudgetExempt is true on a Stuck entry that is not counted against the bounce budget.
	// Such an entry is one the producer marked, a Halt, or one at a row with no OnStuck; absent, it decodes as false.
	BudgetExempt bool `json:"budget_exempt,omitempty"`
	// Repeats is the number of further identical calls folded into this entry, zero when none.
	Repeats int `json:"repeats,omitempty"`
	// LastAt is the RFC 3339 UTC time of the most recent folded call, empty when none;
	// At keeps the first call's time.
	LastAt string `json:"last_at,omitempty"`
	// RoutedTo names the row a reroute entry sent the run to, or the row a Stuck entry's OnStuck edge bounced it to.
	// It is empty on every other entry, and on a Stuck entry written by an older binary.
	// A status file written before the field existed decodes as empty.
	RoutedTo string `json:"routed_to,omitempty"`
	// RerouteCapped is true on the Stuck entry of a reroute-cap halt, which resets the row's reroute count.
	// A status file written before the field existed decodes as false.
	RerouteCapped bool `json:"reroute_capped,omitempty"`
	// BudgetCapped is true on the Stuck entry of a bounce-budget-exhausted halt.
	// A resume of that halt grants one more budget, and a goto onto it grants none.
	// A status file written before the field existed decodes as false.
	BudgetCapped bool `json:"budget_capped,omitempty"`
	// GenerationStart is true on a Stuck entry that started a new review generation, which restarts the row's bounce count at that entry.
	// A status file written before the field existed decodes as false.
	GenerationStart bool `json:"generation_start,omitempty"`
	// By is the caller label of the goto or resume that wrote this entry, empty on every other entry.
	// A status file written before the field existed decodes as empty.
	By string `json:"by,omitempty"`
	// Reason is the entry gate's text on a skipped entry, empty on every other entry.
	// A status file written before the field existed decodes as empty.
	Reason string `json:"reason,omitempty"`
	// Machine is the machine whose shed appended this entry; empty when none was given, and on goto or resume entries.
	// A status file written before the field existed decodes as empty.
	Machine string `json:"machine,omitempty"`
	// Block is the block path of the row that wrote this entry.
	// It is the enclosing block rows' names, outermost first, joined by "/", and it is empty outside every block.
	// A status file written before the field existed decodes as empty.
	Block string `json:"block,omitempty"`
}

// Status is the whole status file.
//
// Field ownership follows who writes each field.
// CurrentProducer, State, PauseReason, Error, Activity, and History are Shed-owned and rewritten on every persist.
// PauseRequested is shared write-to-clear.
// An outside actor sets it true; Shed writes it false, exactly once, in the persist that records StatePaused.
// PauseBefore and PauseAfter are set from outside.
// Shed clears them together in the persist that records StatePaused for a fired condition, and in Goto's persist.
// PauseRequestedBy, PauseBeforeBy and PauseAfterBy are set from outside beside the request they accompany.
// They are cleared in the same write as the request.
// EnteredRow is Shed-owned.
// Its writers are the entry evaluation, the routed writes, Goto, Relaunch and RenameProducers.
// Product is external-writer-owned and only ever carried through.
type Status struct {
	// CurrentProducer names the row the run is at;
	// empty means the run is at its start, the first row of its producer list.
	CurrentProducer string `json:"current_producer"`
	State           State  `json:"state"`
	Error           string `json:"error"`
	// Transient is the TransientClass of the failure that put the run in StateFailed, empty on every other write.
	// A status file written before the field existed decodes as empty.
	Transient string `json:"transient,omitempty"`
	// ParentNotice is the one-line notice an awaiting producer handed for the run's parent, empty on every other write.
	// A status file written before the field existed decodes as empty.
	ParentNotice string `json:"parent_notice,omitempty"`
	// PendingBy is the caller label of a step that resumed a halted run, waiting for the next appended history entry to carry it.
	// A status file written before the field existed decodes as empty.
	PendingBy string `json:"pending_by,omitempty"`
	// EnteredRow names the row the run entered, so the row is called again without evaluating its gate.
	// Its entry gate held or a goto bypassed it. A status without the field, or naming another row, reads as not entered.
	EnteredRow     string `json:"entered_row,omitempty"`
	PauseRequested bool   `json:"pause_requested"`
	// PauseRequestedBy is the outside actor's caller label for PauseRequested, cleared when the request is consumed.
	// A status file written before the field existed decodes as empty.
	PauseRequestedBy string `json:"pause_requested_by,omitempty"`
	// PauseBefore names the row the run pauses before running, empty when none is recorded.
	PauseBefore string `json:"pause_before,omitempty"`
	// PauseBeforeBy is the caller label of the outside actor that recorded PauseBefore, cleared with the conditions.
	// A status file written before the field existed decodes as empty.
	PauseBeforeBy string `json:"pause_before_by,omitempty"`
	// PauseAfter names the row the run pauses after it returns a routed running outcome, empty when none is recorded.
	PauseAfter string `json:"pause_after,omitempty"`
	// PauseAfterBy is the caller label of the outside actor that recorded PauseAfter, cleared with the conditions.
	// A status file written before the field existed decodes as empty.
	PauseAfterBy string `json:"pause_after_by,omitempty"`
	// PauseReason is why the run is paused, set by every write that records StatePaused.
	// It is empty on every other write. A status file written before the field existed decodes as empty.
	PauseReason string         `json:"pause_reason,omitempty"`
	Activity    Activity       `json:"activity"`
	History     []HistoryEntry `json:"history"`
	// Product is an opaque, product-owned payload Shed round-trips verbatim and never inspects, validates, or interprets.
	// It carries no compatibility claim for loom's own schema.
	Product json.RawMessage `json:"product,omitempty"`
}

// ParseStatusTolerant decodes data, the bytes of a status.json a caller read elsewhere, such as from a git tree.
// Unknown keys are ignored and no State value is checked,
// so a finished or older record that a strict read would refuse still decodes.
func ParseStatusTolerant(data []byte) (Status, error) {
	var st Status
	if err := json.Unmarshal(data, &st); err != nil {
		return Status{}, err
	}
	return st, nil
}
