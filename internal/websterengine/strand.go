// strand.go implements webster's orchestrator spawn seam and its leftover-strand reclaim.
// The seam interfaces are exported so webstercli can pass a real *shuttleengine.Runner into them.

package websterengine

import (
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

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
	if err := runplan.StopRecoveryStrands(stopper, st.Batches); err != nil {
		return err
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
