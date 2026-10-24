// arbiterstop.go decides which stops the arbiter may settle: one pure function over what Go reads off the status file and the stop envelope, and the reads that feed it.

package arbiterstop

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// HandBackWayForward is the way-forward clause every arbiter refusal ends with: today's hand-back to the run's parent.
const HandBackWayForward = "way forward: report the stop to the run's parent in the stop report, and `lyx loom status <run-id>` shows the stop"

// Stop is what Go reads off the status file, the stop envelope and the review run directory to decide whether the arbiter may settle a stop.
type Stop struct {
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
	// EscalationCause is the pending escalation's cause; empty when none is pending.
	EscalationCause shedadapters.EscalationCause
	// EscalationErr is the error reading the escalation record returned.
	EscalationErr error
	// LandingHalt reports whether the stop is a blocked landing halt that only the run's parent lifts.
	LandingHalt bool
}

// Eligibility returns nil when the arbiter may settle stop and a refusal otherwise.
// Eligible are an error stop of kind producer or bootstrap, of kind interrupted under the reinvoke policy or with a non-empty transient class.
// Eligible too is a halted blocked stop, bar a landing halt.
// Eligible too is a halted awaiting stop with a pending escalation of cause `hard-cap` or `circling`.
// Every other stop is refused, a landing halt included, since only the run's parent lifts it, and the refusal ends with HandBackWayForward.
func Eligibility(stop Stop) error {
	refuse := func(reason string) error {
		return fmt.Errorf("arbiter: stop not eligible: %s; %s", reason, HandBackWayForward)
	}
	if stop.Kind != "" {
		switch {
		case stop.Kind == shedverbs.KindProducer, stop.Kind == shedverbs.KindBootstrap, stop.Transient != "":
			return nil
		case stop.Kind == shedverbs.KindInterrupted && stop.InterruptPolicy == shedverbs.InterruptPolicyReinvoke:
			return nil
		case stop.Kind == shedverbs.KindInterrupted:
			return refuse(fmt.Sprintf("the run was interrupted at %q, whose policy hands back", stop.Producer))
		default:
			return refuse(fmt.Sprintf("an error of kind %q is not one the arbiter settles", stop.Kind))
		}
	}
	switch stop.State {
	case shedengine.StateBlocked:
		if stop.LandingHalt {
			return refuse(fmt.Sprintf("the halt at %q is a landing halt, which only the run's parent lifts", stop.Producer))
		}
		return nil
	case shedengine.StateAwaiting:
		switch {
		case stop.EscalationErr != nil:
			return refuse(fmt.Sprintf("the review escalation at %q is unreadable: %v", stop.Producer, stop.EscalationErr))
		case !stop.EscalationPending:
			return refuse(fmt.Sprintf("the run awaits at %q with no pending review escalation", stop.Producer))
		case stop.EscalationCause == shedadapters.EscalationHardCap, stop.EscalationCause == shedadapters.EscalationCircling:
			return nil
		default:
			return refuse(fmt.Sprintf("the review escalation at %q has cause %q, which goes to the run's parent", stop.Producer, stop.EscalationCause))
		}
	case shedengine.StateFailed:
		return refuse("the error envelope carries no kind")
	default:
		return refuse(fmt.Sprintf("a %s run is not a halted stop", stop.State))
	}
}

// Readers holds the disk reads Check makes.
type Readers struct {
	// PendingEscalation reports, at a Bouncer row, whether the latest round awaits a decision on its escalation, with the cause and the path of its brief; not pending elsewhere.
	PendingEscalation func(producer string) (cause shedadapters.EscalationCause, briefPath string, pending bool, err error)
	// InterruptPolicy returns the interrupt policy of a row.
	InterruptPolicy func(producer string) string
}

// Check builds the Stop for status, the envelope's kind and transient class from readers, and returns Eligibility's answer.
// The brief path is the review escalation's, and only for an admitted awaiting stop.
// An unreadable escalation record is a not-eligible refusal, never a separate error.
func Check(status shedengine.Status, kind, transient string, readers Readers) (briefPath string, err error) {
	stop := Stop{
		State:           status.State,
		Producer:        status.CurrentProducer,
		Kind:            kind,
		Transient:       transient,
		InterruptPolicy: readers.InterruptPolicy(status.CurrentProducer),
		LandingHalt:     status.State == shedengine.StateBlocked && landingshed.HandsBack(status.CurrentProducer, status.Error),
	}
	var escalationBrief string
	if status.State == shedengine.StateAwaiting {
		stop.EscalationCause, escalationBrief, stop.EscalationPending, stop.EscalationErr = readers.PendingEscalation(status.CurrentProducer)
	}
	if err := Eligibility(stop); err != nil {
		return "", err
	}
	if stop.EscalationPending {
		briefPath = escalationBrief
	}
	return briefPath, nil
}

// EscalationEligible reports whether Check admits an awaiting stop at producer.
func EscalationEligible(producer string, readers Readers) bool {
	status := shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: producer}
	_, err := Check(status, "", "", readers)
	return err == nil
}

// NewReaders builds the real readers of the task worktree at location.
func NewReaders(location *lyxcwd.Location) Readers {
	reviewsDir := loomengine.LoomReviewsDir(location)
	return Readers{
		PendingEscalation: func(producer string) (shedadapters.EscalationCause, string, bool, error) {
			subdir, isBouncer, err := loomrecipe.BouncerRunSubdir(producer)
			if err != nil || !isBouncer {
				return "", "", false, err
			}
			return shedadapters.PendingEscalationCause(filepath.Join(reviewsDir, subdir))
		},
		InterruptPolicy: loomshed.InterruptPolicyFor,
	}
}
