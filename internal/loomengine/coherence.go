// coherence.go implements the pure, in-memory validator that checks a decoded shedengine.Status
// shell plus its unmarshaled loom Status product against the fresh-start invariants CheckSeed
// enforces before a task is fit to run.
// It performs no I/O and spawns nothing, so it is exhaustively table-tested in Tier 1
// (coherence_test.go).

package loomengine

import (
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// checkCoherence validates a decoded shedengine.Status shell alongside its unmarshaled loom
// product against the fresh-start invariants check 4 enforces.
// expectedProducer is the current_producer value a coherent seed must carry, and
// toleratedProducers is the set of producer names a history entry may name without tripping the
// fresh-start rule -- both told by the caller, since they are the caller's own durable row
// identities, never derived here.
// It collects all violated rules into the returned slice.
func checkCoherence(shed shedengine.Status, product Status, expectedProducer string, toleratedProducers []string) []Failure {
	var failures []Failure

	mandatory := []struct {
		name  string
		value string
	}{
		{"slug", product.Slug},
		{"parent", product.Parent},
	}
	for _, m := range mandatory {
		if m.value == "" {
			failures = append(failures, Failure{
				Check:  CheckSeedIncoherent,
				Reason: fmt.Sprintf("mandatory field %q is empty or absent", m.name),
			})
		}
	}

	// shedengine.Run persists the *next* row's name into current_producer and appends the
	// finished row's history entry before calling that next row, so at the instant this row's
	// own seed check runs, the file already names this row. The expected name is therefore told
	// by the caller, not derived here.
	if shed.CurrentProducer != expectedProducer {
		failures = append(failures, Failure{
			Check:  CheckSeedIncoherent,
			Reason: fmt.Sprintf("current_producer %q is not %q", shed.CurrentProducer, expectedProducer),
		})
	}

	switch shed.State {
	case shedengine.StateRunning, shedengine.StatePaused, shedengine.StateBlocked, shedengine.StateFailed:
		// Every non-terminal state is tolerated: a blocked or failed run at Preflight is exactly
		// the resumable half-finished shape the fresh-start check below narrows onto.
	case shedengine.StateDone:
		failures = append(failures, Failure{
			Check:  CheckSeedIncoherent,
			Reason: fmt.Sprintf("state %q is a finished run", shedengine.StateDone),
		})
	default:
		failures = append(failures, Failure{
			Check:  CheckSeedIncoherent,
			Reason: fmt.Sprintf("state %q is not a valid shedengine.State", shed.State),
		})
	}

	// shed.Error is never validated: it is tolerated at any value, including non-empty, because
	// it is the previous halt's reason a human resumes after reading.
	// shed.Activity is never validated either: Shed recomposes it mechanically on every persist,
	// so validating it here would assert Shed's own arithmetic against itself.

	for i, h := range shed.History {
		if h.Outcome != shedengine.Done && h.Outcome != shedengine.Stuck {
			failures = append(failures, Failure{
				Check:  CheckSeedIncoherent,
				Reason: fmt.Sprintf("history[%d].outcome %q is not a valid outcome", i, h.Outcome),
			})
		}
		if !isRFC3339UTC(h.At) {
			failures = append(failures, Failure{
				Check:  CheckSeedIncoherent,
				Reason: fmt.Sprintf("history[%d].at %q is not RFC3339 UTC", i, h.At),
			})
		}
	}

	// The fresh-start check is narrower than "history is empty": shedengine.Run appends a history
	// entry before persisting StateBlocked, including on the OnStuck: "" escalation path, so a
	// Stuck outcome at either the generic row or loom's own row leaves that row's own entry
	// behind -- which is why both names are in the tolerated set. A history entry naming any
	// producer outside that set is the real half-finished signal, so a blocked run at a tolerated
	// row stays resumable rather than failing CheckHalfFinished on every subsequent resume
	// attempt, forever.
	for _, h := range shed.History {
		if !slices.Contains(toleratedProducers, h.Producer) {
			failures = append(failures, Failure{
				Check:  CheckHalfFinished,
				Reason: fmt.Sprintf("history names producer %q, not one of the tolerated set %v: status.json is not a fresh seed", h.Producer, toleratedProducers),
			})
			break
		}
	}
	if product.StartSha != nil || shed.PauseRequested {
		failures = append(failures, Failure{
			Check:  CheckHalfFinished,
			Reason: "status.json is not a fresh seed: start_sha or pause_requested is already set",
		})
	}

	return failures
}

// isRFC3339UTC reports whether ts parses as RFC3339 with UTC offset.
func isRFC3339UTC(ts string) bool {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	_, offset := t.Zone()
	return offset == 0
}
