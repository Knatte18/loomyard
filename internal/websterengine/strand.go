// strand.go implements webster's own strand/spawn seam helpers: the Starter seam, the OrchestratorStarter/OrchestratorHandle spawn seam, and the StrandStopper seam, inlining direct shuttleengine calls.
// Webster's waits read a strand's liveness and turn end through gatefixer's StrandLive and TurnEndedAfter.
// These are deliberately module-local rather than shared, since the spawn seam is part of
// webster's own contract shape.
// The spawn-seam interfaces are exported so webstercli can assign a real *shuttleengine.Runner into
// them (Go's structural typing means *shuttleengine.Runner satisfies Starter and StrandStopper with no adapter glue).

package websterengine

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Starter is the seam a batch's implementer or recovery strand spawns through.
// Start blocks until the spawned provider is past its startup gates (shuttle's guarantee) and
// returns an error when it never became ready; it never waits for the run to finish.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// OrchestratorHandle is the started-but-not-yet-finished orchestrator-role spawn a caller blocks
// on.
// StrandGUID identifies the reed strand, Wait blocks until terminal shuttle outcome.
type OrchestratorHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// OrchestratorStarter is the seam a caller spawns an orchestrator-role strand through, deliberately
// two-phase (start, then wait) so strand identity is learned and persisted before blocking.
type OrchestratorStarter interface {
	StartOrchestrator(shuttleengine.Spec) (OrchestratorHandle, error)
}

// RecoveryStrandRemoveError is RemoveRecoveryStrands's failure: the strand it could not remove and why.
type RecoveryStrandRemoveError struct {
	GUID string
	Err  error
}

func (e *RecoveryStrandRemoveError) Error() string {
	return fmt.Sprintf("could not remove recovery strand %s: %v", e.GUID, e.Err)
}

func (e *RecoveryStrandRemoveError) Unwrap() error { return e.Err }

// StrandStopper is the seam a leftover strand is stopped through by guid.
// StopStrand records the stop on the strand's run, then ends the strand when it is live, logging that kill at Warn, and is a no-op for a strand that is not.
type StrandStopper interface {
	StopStrand(guid string) error
}

// RemoveRecoveryStrands stops every live recovery strand the state records, in batch-number order, then every cold verify-gate fixer strand and every plan fixer strand, each in spawn order.
// Only a batch of Kind "recovery" with a StrandGUID names a batch's one;
// implementer forks and the Master have no recorded strand.
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
