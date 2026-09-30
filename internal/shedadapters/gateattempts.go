// gateattempts.go implements the one helper both gated producers (SingleLLMProducer, BurlerProducer)
// share to carry a gate's re-prompt count onto shedengine.OutputPointer.GateAttempts.

package shedadapters

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// gateFailedReason is the one-line Stuck reason for a gate that did not pass, shared by both gated
// producers so the wording never diverges.
func gateFailedReason(gate *shuttleengine.GateOutcome) string {
	return fmt.Sprintf("gate did not pass after %d attempts; findings: %s", gate.Attempts, gate.FindingsPath)
}

// gateAttemptsPointer returns a pointer to gate.Attempts, or nil when gate itself is nil (the
// producer applied no gate to this call). Read shedengine.OutputPointer.GateAttempts's own doc
// comment for why a nil/non-nil distinction is kept rather than collapsing an ungated call and a
// gate that passed on its first attempt onto the same zero value.
func gateAttemptsPointer(gate *shuttleengine.GateOutcome) *int {
	if gate == nil {
		return nil
	}
	attempts := gate.Attempts
	return &attempts
}
