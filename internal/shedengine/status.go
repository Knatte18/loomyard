// status.go declares the persisted status-file shape and its JSON tags.
// internal/state (the persistence package) and this file's own State type are two different
// things -- keep the two apart when reading identifiers in this file and its callers.

package shedengine

import "encoding/json"

// State is the status file's own lifecycle field: a superset of RunOutcome, adding the two
// values Run can never return (running, failed).
type State string

// The five legal State values. A persisted value outside this set -- including the empty
// string -- is a hard error at the read gate.
const (
	StateRunning State = "running"
	StatePaused  State = "paused"
	StateDone    State = "done"
	StateBlocked State = "blocked"
	StateFailed  State = "failed"
)

// valid reports whether s is one of the five legal State constants.
// The empty string is rejected rather than tolerated: State is a mandatory enum string read from
// a file an external actor seeds, so a typo or a partial seed would otherwise fall through to
// undefined behaviour -- silently treated as running, or as done.
// The in-repo precedent for this split is checkCoherence in internal/loomengine/coherence.go,
// which treats an empty mandatory enum string as absent and therefore a violation, while
// reserving zero-value tolerance for the nullable, bool, and slice fields. PauseRequested (bool)
// and History (slice) below keep that zero-value tolerance here for the same reason.
func (s State) valid() bool {
	switch s {
	case StateRunning, StatePaused, StateDone, StateBlocked, StateFailed:
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

// HistoryEntry is one producer call's durable record, and the element type of Result.History.
type HistoryEntry struct {
	Producer string  `json:"producer"`
	Outcome  Outcome `json:"outcome"`
	Output   string  `json:"output"`
	At       string  `json:"at"`
}

// Status is the whole status file.
//
// Field ownership is split three ways: CurrentProducer, State, Error, Activity, and History are
// Shed-owned and rewritten on every persist; PauseRequested is shared write-to-clear, set true
// only by an outside actor and written false only by Shed, exactly once, in the persist that
// records StatePaused; Product is external-writer-owned and only ever carried through.
type Status struct {
	CurrentProducer string         `json:"current_producer"`
	State           State          `json:"state"`
	Error           string         `json:"error"`
	PauseRequested  bool           `json:"pause_requested"`
	Activity        Activity       `json:"activity"`
	History         []HistoryEntry `json:"history"`
	// Product is an opaque product-owned payload Shed round-trips verbatim and never inspects,
	// validates, or interprets. It carries no compatibility claim for loom's own schema.
	Product json.RawMessage `json:"product,omitempty"`
}
