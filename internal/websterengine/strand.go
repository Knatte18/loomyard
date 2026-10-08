// strand.go implements webster's own strand/spawn seam helpers: StrandLive and TurnEnded, the Starter seam, the OrchestratorStarter/OrchestratorHandle spawn seam, and the StrandStopper seam, inlining direct shuttleengine calls.
// These are deliberately module-local rather than shared, since the spawn seam is part of
// webster's own contract shape.
// StrandLive and TurnEnded are EXPORTED because internal/webstercli calls them directly;
// the spawn-seam interfaces are exported so webstercli can assign a real *shuttleengine.Runner into
// them (Go's structural typing means *shuttleengine.Runner satisfies Starter and StrandStopper with no adapter glue).

package websterengine

import (
	"fmt"
	"os"
	"sort"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// StrandLive reports whether guid names a strand reed currently tracks as live.
// guid absent from the result reports (false, nil).
// Liveness is NEVER read from persisted reed state;
// only a live Status() query can answer this.
func StrandLive(reed shuttleengine.ReedOps, guid string) (bool, error) {
	status, err := reed.Status()
	if err != nil {
		return false, fmt.Errorf("websterengine: reed status: %w", err)
	}
	for _, s := range status.Strands {
		if s.GUID == guid {
			return s.Live, nil
		}
	}
	return false, nil
}

// TurnEnded reports whether an implementer's turn has ended without satisfying the file contract.
// It delegates event-grammar parsing to engine.ParseEvents, reporting true only when at least one
// Event carries Kind == shuttleengine.EventStop.
// Missing events file reports (false, nil).
// ParseEvents errors propagate.
func TurnEnded(eventsPath string, engine shuttleengine.Engine) (bool, error) {
	return TurnEndedAfter(eventsPath, 0, engine)
}

// TurnEndedAfter is TurnEnded over the events from byte offset on, so the skill-load turns shuttle ran before the prompt never count.
// An offset past the file's end reads no events.
func TurnEndedAfter(eventsPath string, offset int64, engine shuttleengine.Engine) (bool, error) {
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("websterengine: read events file %s: %w", eventsPath, err)
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	data = data[offset:]
	if len(data) == 0 {
		return false, nil
	}

	events, err := engine.ParseEvents(data)
	if err != nil {
		return false, fmt.Errorf("websterengine: parse events %s: %w", eventsPath, err)
	}

	for _, e := range events {
		if e.Kind == shuttleengine.EventStop {
			return true, nil
		}
	}
	return false, nil
}

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
// StopStrand records the stop on the strand's run, then ends the strand when it is live, and is a no-op for a strand that is not.
type StrandStopper interface {
	StopStrand(guid string) error
}

// RemoveRecoveryStrands stops every live recovery strand the state records, in batch-number order.
// Only a batch of Kind "recovery" with a StrandGUID names one;
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
	return nil
}
