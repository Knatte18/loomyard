// state.go declares the run states, their precedence order, and the derivation of a run's state from its tracked records.

package runfsm

import "github.com/Knatte18/loomyard/internal/shedengine"

// State is one state of a run, derived from its tracked records.
type State string

// The run states.
const (
	// Landed is a run whose work reached its parent branch.
	Landed State = "landed"
	// Cancelled is a run whose cancel finished.
	Cancelled State = "cancelled"
	// Cancelling is a run whose cancel started and has not finished.
	Cancelling State = "cancelling"
	// Paused is a run held by a pause intent or a shed pause.
	Paused State = "paused"
	// Halted is a run whose inner run is blocked, failed or awaiting its parent.
	Halted State = "halted"
	// Landing is a run at or past its landing row.
	Landing State = "landing"
	// Running is a run whose batten has started driving its task.
	Running State = "running"
	// Starting is a run with a batten record that has not started driving.
	Starting State = "starting"
	// Queued is a released task with no batten record.
	Queued State = "queued"
	// NoRun is a slug that no run holds, such as a draft.
	NoRun State = "none"
)

// Records holds the tracked records a run's state derives from, and nothing machine-local.
// The caller fills the row facts from the recipes, so this package names no row.
type Records struct {
	// Released is true for a task entry whose board status is empty.
	Released bool
	// BoardDone is true for an entry whose board status is done.
	BoardDone bool
	// Batten is the batten run record, read only when BattenFound.
	Batten shedengine.Status
	// BattenFound is true when the batten record exists.
	BattenFound bool
	// BattenStarted is true when batten's row is Run-Shed or later.
	BattenStarted bool
	// TaskBranchOnOrigin is true when the task branch is on origin.
	TaskBranchOnOrigin bool
	// Child is the child run record, read only when ChildFound.
	Child shedengine.Status
	// ChildFound is true when the child record exists.
	ChildFound bool
	// ChildLanding is true when the child is at Finalize or past it, or batten is at Land or later.
	ChildLanding bool
}

// Reading is the result of deriving a run's state.
type Reading struct {
	// State is the derived state.
	State State
	// PairKept is true for a Cancelled run whose finish kept the pair.
	PairKept bool
}

// order lists the states highest precedence first.
// Each entry holds the condition under which Derive takes the state, over the records after the crash-leftover rule.
var order = []struct {
	state State
	holds func(r Records) bool
}{
	{Landed, func(r Records) bool {
		if r.BattenFound {
			return r.Batten.State == shedengine.StateDone
		}
		return r.BoardDone
	}},
	{Cancelled, func(r Records) bool {
		return r.BattenFound && r.Batten.Cancel != nil && r.Batten.Cancel.Finish != nil
	}},
	{Cancelling, func(r Records) bool {
		return r.BattenFound && r.Batten.Cancel != nil && r.Batten.Cancel.Finish == nil
	}},
	{Paused, func(r Records) bool {
		batten := r.BattenFound && (r.Batten.PauseIntent != nil || r.Batten.State == shedengine.StatePaused)
		return batten || (r.ChildFound && r.Child.State == shedengine.StatePaused)
	}},
	{Halted, func(r Records) bool {
		if !r.ChildFound {
			return false
		}
		switch r.Child.State {
		case shedengine.StateBlocked, shedengine.StateFailed, shedengine.StateAwaiting:
			return true
		default:
			return false
		}
	}},
	{Landing, func(r Records) bool { return r.ChildLanding }},
	{Running, func(r Records) bool { return r.BattenStarted }},
	{Starting, func(r Records) bool { return r.BattenFound }},
	{Queued, func(r Records) bool { return r.Released && !r.BattenFound }},
	{NoRun, func(Records) bool { return true }},
}

// Order returns the run states, highest precedence first.
func Order() []State {
	states := make([]State, len(order))
	for i, entry := range order {
		states[i] = entry.state
	}
	return states
}

// Derive returns the state of the run the records describe: the first state of Order whose condition holds.
// A batten record whose shed state is cancelled while its task branch is gone from origin counts as no record.
// It is the leftover of a cancel that crashed after removing the pair.
func Derive(r Records) Reading {
	if r.BattenFound && r.Batten.State == shedengine.StateCancelled && !r.TaskBranchOnOrigin {
		r.BattenFound = false
		r.Batten = shedengine.Status{}
	}
	for _, entry := range order {
		if !entry.holds(r) {
			continue
		}
		reading := Reading{State: entry.state}
		if entry.state == Cancelled {
			reading.PairKept = !r.Batten.Cancel.Finish.Remove
		}
		return reading
	}
	return Reading{State: NoRun}
}

// clearShedPause reads a shed state of paused as running, and leaves every other state as it is.
func clearShedPause(state shedengine.State) shedengine.State {
	if state == shedengine.StatePaused {
		return shedengine.StateRunning
	}
	return state
}

// TransitionState returns the board display state of one status transition.
// A shed state of paused reads Paused, and blocked, failed and awaiting read Halted.
// Otherwise landing reads Landing, started reads Running and neither reads Starting.
func TransitionState(state shedengine.State, started, landing bool) State {
	switch state {
	case shedengine.StatePaused:
		return Paused
	case shedengine.StateBlocked, shedengine.StateFailed, shedengine.StateAwaiting:
		return Halted
	}
	switch {
	case landing:
		return Landing
	case started:
		return Running
	default:
		return Starting
	}
}
