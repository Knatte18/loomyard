// arbitereligibility.go decides which stops the arbiter may settle: one pure function over what Go reads off the status file and the stop envelope.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// arbiterHandBackWayForward is the way-forward clause every arbiter refusal ends with: today's hand-back to the run's parent.
const arbiterHandBackWayForward = "way forward: report the stop to the run's parent in the stop report, and `lyx loom status <run-id>` shows the stop"

// arbiterStop is what Go reads off the status file and the stop envelope to decide whether the arbiter may settle a stop.
type arbiterStop struct {
	// State is the status file's state.
	State shedengine.State
	// Producer is the status file's current producer.
	Producer string
	// Kind is the stop envelope's refusal kind; empty for a halt that is not an error.
	Kind string
	// Transient is the stop envelope's transient class; empty when the failure is not transient.
	Transient string
	// InterruptPolicy is the interrupt policy of Producer's row.
	InterruptPolicy string
	// EscalationPending reports whether Producer is a Bouncer row whose latest round awaits a decision on its escalation.
	EscalationPending bool
	// PublishHaltNamesTarget reports whether a blocked stop at the Publish row carries a verify-failure record that names a target.
	PublishHaltNamesTarget bool
}

// arbiterEligibility returns nil when the arbiter may settle stop and a refusal otherwise.
// Eligible are an error stop of kind producer or bootstrap, of kind interrupted under the reinvoke policy or with a non-empty transient class.
// Eligible too are a halted blocked stop unless it is the Publish halt that names a target, and a halted awaiting stop with a pending escalation.
// Every other stop is refused, and the refusal ends with the hand-back way forward.
func arbiterEligibility(stop arbiterStop) error {
	refuse := func(reason string) error {
		return fmt.Errorf("arbiter: stop not eligible: %s; %s", reason, arbiterHandBackWayForward)
	}
	if stop.Kind != "" {
		switch {
		case stop.Kind == shedverbs.KindProducer, stop.Kind == shedverbs.KindBootstrap, stop.Transient != "":
			return nil
		case stop.Kind == shedverbs.KindInterrupted && stop.InterruptPolicy == loomshed.InterruptPolicyReinvoke:
			return nil
		case stop.Kind == shedverbs.KindInterrupted:
			return refuse(fmt.Sprintf("the run was interrupted at %q, whose policy hands back", stop.Producer))
		default:
			return refuse(fmt.Sprintf("an error of kind %q is not one the arbiter settles", stop.Kind))
		}
	}
	switch stop.State {
	case shedengine.StateBlocked:
		if stop.PublishHaltNamesTarget {
			return refuse("a Publish halt that names a target belongs to batten's retry and then to the parent")
		}
		return nil
	case shedengine.StateAwaiting:
		if stop.EscalationPending {
			return nil
		}
		return refuse(fmt.Sprintf("the run awaits at %q with no pending review escalation", stop.Producer))
	case shedengine.StateFailed:
		return refuse("the error envelope carries no kind")
	default:
		return refuse(fmt.Sprintf("a %s run is not a halted stop", stop.State))
	}
}
