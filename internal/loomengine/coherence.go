// coherence.go implements the pure, in-memory validator that checks a decoded shedengine.Status
// shell plus its unmarshaled loom Status product against the fresh-start invariants Preflight
// enforces before a task is fit to run.
// It performs no I/O and spawns nothing, so it is exhaustively table-tested in Tier 1
// (coherence_test.go).

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// checkCoherence validates a decoded shedengine.Status shell alongside its unmarshaled loom
// product against the fresh-start invariants check 4 enforces.
// It collects all violated rules into the returned slice.
func checkCoherence(shed shedengine.Status, product Status) []Failure {
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

	// check 4 is only ever reached while Preflight's own gate holds, and that gate is only
	// satisfied by the very first producer in loom's list -- so a coherent seed's
	// current_producer must always name it.
	if shed.CurrentProducer != "Preflight" {
		failures = append(failures, Failure{
			Check:  CheckSeedIncoherent,
			Reason: fmt.Sprintf("current_producer %q is not %q", shed.CurrentProducer, "Preflight"),
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
	// Stuck outcome at row 1 (Preflight itself) leaves one Preflight entry behind. A history
	// entry naming any producer other than "Preflight" is the real half-finished signal -- an
	// entry naming "Preflight" itself is tolerated, so a blocked row-1 run stays resumable rather
	// than failing CheckHalfFinished on every subsequent resume attempt, forever.
	for _, h := range shed.History {
		if h.Producer != "Preflight" {
			failures = append(failures, Failure{
				Check:  CheckHalfFinished,
				Reason: fmt.Sprintf("history names producer %q, not just %q: status.json is not a fresh seed", h.Producer, "Preflight"),
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
