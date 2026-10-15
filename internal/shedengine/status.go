// status.go declares the persisted status-file shape and its JSON tags.
// internal/state (the persistence package) and this file's own State type are two different
// things -- keep the two apart when reading identifiers in this file and its callers.

package shedengine

import "encoding/json"

// State is the status file's own lifecycle field: a superset of RunOutcome, adding the two
// values Run can never return (running, failed).
type State string

// The six legal State values. A persisted value outside this set -- including the empty
// string -- is a hard error at the read gate.
// StateAwaiting is the planned hand-off halt: it behaves like blocked wherever a halt is
// observed, but is no failure and spends no bounce budget.
const (
	StateRunning  State = "running"
	StatePaused   State = "paused"
	StateDone     State = "done"
	StateBlocked  State = "blocked"
	StateFailed   State = "failed"
	StateAwaiting State = "awaiting"
)

// valid reports whether s is one of the six legal State constants.
// The empty string is rejected rather than tolerated: State is a mandatory enum string read from
// a file an external actor seeds, so a typo or a partial seed would otherwise fall through to
// undefined behaviour -- silently treated as running, or as done.
// The in-repo precedent for this split is checkCoherence in internal/loomengine/coherence.go,
// which treats an empty mandatory enum string as absent and therefore a violation, while
// reserving zero-value tolerance for the nullable, bool, and slice fields. PauseRequested (bool)
// and History (slice) below keep that zero-value tolerance here for the same reason.
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

// CallerShed is the caller label of lyx's own automatic steps: the Go driver and batten.
const CallerShed = "shed"

// HistoryEntry is one producer call's durable record, and the element type of Result.History.
// A budget-exempt Stuck folds into an immediately preceding identical exempt Stuck rather than appending,
// so one entry can stand for a run of consecutive calls.
type HistoryEntry struct {
	Producer string  `json:"producer"`
	Outcome  Outcome `json:"outcome"`
	Output   string  `json:"output"`
	At       string  `json:"at"`
	// GateAttempts is the gate's own re-prompt count (shuttleengine.GateOutcome.Attempts),
	// carried through so the status file shows it -- the same idiom as max_bounces, per
	// the producer-gate contract's "an explicit attempt counter bounds the loop"
	// requirement. Nil means this call's producer applied no gate; non-nil is shown even when
	// its value is 0 (a gate that passed on the first try), since 0 is a meaningful, distinct
	// verdict from "ungated" and collapsing the two would silently lose it.
	GateAttempts *int `json:"gate_attempts,omitempty"`
	// GateSpans are the time spans this call's gate entries took, in the order they started.
	// A status file written before the field existed decodes as nil, and a nil list is not written.
	GateSpans []GateSpan `json:"gate_spans,omitempty"`
	// BudgetExempt is true on a Stuck entry the producer marked as not counted against the bounce budget.
	// A status file written before the field existed decodes as false and counts as before.
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
	// By is the caller label of the goto or resume that wrote this entry, empty on every other entry.
	// A status file written before the field existed decodes as empty.
	By string `json:"by,omitempty"`
}

// Status is the whole status file.
//
// Field ownership is split four ways.
// CurrentProducer, State, Error, Activity, and History are Shed-owned and rewritten on every persist.
// PauseRequested is shared write-to-clear, set true only by an outside actor and written false only by Shed, exactly once, in the persist that records StatePaused.
// PauseBefore and PauseAfter are outside-set and cleared by Shed together, only in the persist that records StatePaused for a fired condition and in Goto's persist.
// Product is external-writer-owned and only ever carried through.
type Status struct {
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
	PendingBy      string `json:"pending_by,omitempty"`
	PauseRequested bool   `json:"pause_requested"`
	// PauseBefore names the row the run pauses before running, empty when none is recorded.
	PauseBefore string `json:"pause_before,omitempty"`
	// PauseAfter names the row the run pauses after it returns a routed running outcome, empty when none is recorded.
	PauseAfter string         `json:"pause_after,omitempty"`
	Activity   Activity       `json:"activity"`
	History    []HistoryEntry `json:"history"`
	// Product is an opaque product-owned payload Shed round-trips verbatim and never inspects,
	// validates, or interprets. It carries no compatibility claim for loom's own schema.
	Product json.RawMessage `json:"product,omitempty"`
}

// ParseStatusTolerant decodes data, the bytes of a status.json a caller read elsewhere, such as from a git tree.
// An unknown key reads, and no State value is checked, so a finished or older record the live read gate refuses still decodes.
func ParseStatusTolerant(data []byte) (Status, error) {
	var st Status
	if err := json.Unmarshal(data, &st); err != nil {
		return Status{}, err
	}
	return st, nil
}
