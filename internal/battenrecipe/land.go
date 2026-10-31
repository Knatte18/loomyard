// land.go is the one test of where a batten run stands against its Land row: past it or not, and when past it, whether it landed.

package battenrecipe

import (
	"slices"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// LandOutcome is how a batten run stands against its Land row.
type LandOutcome string

// The outcomes LandOutcomeOf reports.
const (
	// LandOutcomeAhead is a run not yet past its Land row.
	LandOutcomeAhead LandOutcome = ""
	// LandOutcomeLanded is a run past its Land row whose history holds a Land Done.
	LandOutcomeLanded LandOutcome = "landed"
	// LandOutcomeUnlanded is a run past its Land row whose history holds no Land Done, as a goto onto the teardown row leaves it.
	LandOutcomeUnlanded LandOutcome = "ended without landing"
)

// PastLand reports whether a batten run whose status reads state at producer is past its Land row: done, or at the one row after Land in any state.
func PastLand(state shedengine.State, producer string) bool {
	return state == shedengine.StateDone || producer == NameWorktreeTeardown
}

// LandOutcomeOf reports how the batten run whose status is status stands against its Land row.
func LandOutcomeOf(status shedengine.Status) LandOutcome {
	if !PastLand(status.State, status.CurrentProducer) {
		return LandOutcomeAhead
	}
	if slices.ContainsFunc(status.History, func(entry shedengine.HistoryEntry) bool {
		return entry.Producer == NameLand && entry.Outcome == shedengine.Done
	}) {
		return LandOutcomeLanded
	}
	return LandOutcomeUnlanded
}
