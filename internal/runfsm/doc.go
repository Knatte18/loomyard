// Package runfsm derives a run's state from tracked records alone and decides whether a verb may act on the run.
// It imports only the standard library and internal/shedengine, for the record type, and does no I/O:
// callers read the tracked records and the machine-local facts and hand them in.
//
// Derive reads the records and returns the run's state, the highest-precedence state whose condition holds.
// Act answers from the state and the machine-local inputs whether a verb acts, waits or hands the run over.
// Verdict.Refusal words the wait or hand-over with its way forward.
// Judge runs both over one set of inputs, with the states a verb sets aside cleared first.
//
// The board's "<state> · <row>" text is a display of the state: TransitionState maps one status transition to it.
package runfsm
