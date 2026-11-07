// strand.go implements webster's strand spawn seam, which calls shuttleengine directly.
// The seam interfaces are exported so webstercli can pass a real *shuttleengine.Runner into them.
// It satisfies Starter and StrandStopper structurally, so no adapter is needed.

package websterengine

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Starter is the seam a batch's implementer or recovery strand spawns through.
// Start blocks until the spawned provider is past its startup gates, which shuttle guarantees.
// It returns an error when the provider never becomes ready, and it never waits for the run to finish.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// OrchestratorHandle is the started-but-not-yet-finished orchestrator-role spawn a caller blocks on.
// StrandGUID identifies the reed strand.
// Wait blocks until the shuttle run reaches a terminal outcome.
type OrchestratorHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// OrchestratorStarter is the seam a caller spawns an orchestrator-role strand through.
// It is two-phase, start then wait, so the strand identity is learned and persisted before the caller blocks.
type OrchestratorStarter interface {
	StartOrchestrator(shuttleengine.Spec) (OrchestratorHandle, error)
}

// RecoveryStrandRemoveError is a recovery strand that could not be removed, with its guid and the cause.
type RecoveryStrandRemoveError struct {
	GUID string
	Err  error
}

func (e *RecoveryStrandRemoveError) Error() string {
	return fmt.Sprintf("could not remove recovery strand %s: %v", e.GUID, e.Err)
}

func (e *RecoveryStrandRemoveError) Unwrap() error { return e.Err }

// StrandStopper is the seam a leftover strand is stopped through, by guid.
// StopStrand records the stop on the strand's run.
// It ends a live strand, logging that kill at Warn, and does nothing for a strand that is not live.
type StrandStopper interface {
	StopStrand(guid string) error
}

// RemoveRecoveryStrands stops every recovery strand the state records.
// It stops the recovery strands in batch-number order.
// Then it stops the verify-gate fixer strands and the plan fixer strands, each in spawn order.
// Only a recovery batch with a StrandGUID has one; implementer forks and the Master record no strand.
// The first failure returns a *RecoveryStrandRemoveError naming the strand guid.
// A nil state stops nothing.
func RemoveRecoveryStrands(stopper StrandStopper, st *State) error {
	if st == nil {
		return nil
	}
	numbers := make([]int, 0, len(st.Batches))
	for number := range st.Batches {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		bs := st.Batches[number]
		if bs == nil || bs.Kind != "recovery" || bs.StrandGUID == "" {
			continue
		}
		if err := stopper.StopStrand(bs.StrandGUID); err != nil {
			return &RecoveryStrandRemoveError{GUID: bs.StrandGUID, Err: err}
		}
	}
	for _, rec := range st.VerifyFixers {
		if err := stopper.StopStrand(rec.StrandGUID); err != nil {
			return &RecoveryStrandRemoveError{GUID: rec.StrandGUID, Err: err}
		}
	}
	for _, rec := range st.PlanFixers {
		if err := stopper.StopStrand(rec.StrandGUID); err != nil {
			return &RecoveryStrandRemoveError{GUID: rec.StrandGUID, Err: err}
		}
	}
	return nil
}
