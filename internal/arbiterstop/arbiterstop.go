// arbiterstop.go decides which stops the arbiter may settle, and builds the readers that feed that decision.

package arbiterstop

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// HandBackWayForward is the way-forward clause that ends a refusal of a loom run's stop.
// It sends the stop to the run's parent, and `lym loom status <run-id>` shows the stop.
const HandBackWayForward = "way forward: report the stop to the run's parent in the stop report, and `lym loom status <run-id>` shows the stop"

// BattenHandBackWayForward is the way-forward clause that ends a batten refusal.
// It hands the stop back to the run's parent, which finds it through `lym batten status <slug>`.
const BattenHandBackWayForward = "way forward: report the stop to the run's parent in the stop report, and `lym batten status <slug>` shows the stop"

// Stop holds the facts read off the status file, the stop envelope and the review run directory.
// The arbiter decides from them whether it may settle the stop.
type Stop struct {
	// State is the state the status file records, which decides how the stop is judged.
	State shedengine.State
	// Producer is the row the status file names as current, where the stop sits.
	Producer string
	// Kind is the stop envelope's refusal kind; empty for a halt that is not an error.
	Kind string
	// Transient is the stop envelope's transient class; empty when the failure is not transient.
	Transient string
	// InterruptPolicy is the interrupt policy of the current row, which decides whether an interrupted stop is reinvoked or handed back.
	InterruptPolicy string
	// EscalationPending reports whether Producer is a Bouncer row whose latest round awaits a decision on its escalation.
	EscalationPending bool
	// EscalationCause is the pending escalation's cause; empty when none is pending.
	EscalationCause shedadapters.EscalationCause
	// EscalationErr is the error the escalation read returned; a non-nil value makes the stop not eligible.
	EscalationErr error
	// LandingHalt reports whether the stop is a blocked landing halt that only the run's parent lifts.
	LandingHalt bool
	// DecisionHalt reports whether the stop is a blocked decision halt that only the run's parent lifts.
	DecisionHalt bool
	// Batten reports whether the stop belongs to a batten run, whose stops are judged by the batten rule.
	Batten bool
	// BudgetExhausted reports whether the stop is a blocked stop whose error says the bounce budget is spent.
	BudgetExhausted bool
	// PreconditionUnmet reports whether the stop is a blocked stop whose error says a precondition is unmet.
	PreconditionUnmet bool
	// PauseHonoured reports whether the stop is a blocked Land stop that honours a pause, which the arbiter never relaunches over.
	PauseHonoured bool
}

// HandsBack reports whether a blocked stop at producer goes to the run's parent, not to a recipe row or the arbiter.
// It is true for a landing halt, and for a reason that starts with `shedadapters.DecisionHaltPrefix`.
func HandsBack(producer, reason string) bool {
	return landingshed.HandsBack(producer, reason) || strings.HasPrefix(reason, shedadapters.DecisionHaltPrefix)
}

// Eligibility returns nil when the arbiter may settle stop, and a refusal otherwise.
// A refusal ends with a way-forward clause that sends the stop to the run's parent.
// A loom stop is settled when it is an error stop of kind producer or bootstrap, or it has a transient class.
// It is also settled when it is an interrupted stop under the reinvoke policy.
// It is also settled when it is a blocked halt that is neither a landing halt nor a decision halt.
// It is also settled when it awaits with a pending escalation of cause `hard-cap`, `circling` or `decision`.
// Every other loom stop is refused, including a landing halt, a decision halt and a `budget` escalation.
// Only the run's parent lifts those.
// A batten stop is settled unless its bounce budget is spent or a precondition is unmet.
// A Land stop that honours a pause is refused too.
func Eligibility(stop Stop) error {
	if stop.Batten {
		return battenEligibility(stop)
	}
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
		if stop.DecisionHalt {
			return refuse(fmt.Sprintf("the halt at %q is a decision halt only the run's parent lifts", stop.Producer))
		}
		return nil
	case shedengine.StateAwaiting:
		switch {
		case stop.EscalationErr != nil:
			return refuse(fmt.Sprintf("the review escalation at %q is unreadable: %v", stop.Producer, stop.EscalationErr))
		case !stop.EscalationPending:
			return refuse(fmt.Sprintf("the run awaits at %q with no pending review escalation", stop.Producer))
		case stop.EscalationCause == shedadapters.EscalationHardCap, stop.EscalationCause == shedadapters.EscalationCircling, stop.EscalationCause == shedadapters.EscalationDecision:
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

// battenEligibility judges a batten stop: the tackler escalated it, so the stop is eligible unless it is a hand-back to the run's parent.
func battenEligibility(stop Stop) error {
	refuse := func(reason string) error {
		return fmt.Errorf("arbiter: stop not eligible: %s; %s", reason, BattenHandBackWayForward)
	}
	switch {
	case stop.BudgetExhausted:
		return refuse(fmt.Sprintf("the bounce budget at %q is spent, which goes to the run's parent", stop.Producer))
	case stop.PreconditionUnmet:
		return refuse(fmt.Sprintf("a precondition at %q is unmet, which goes to the run's parent", stop.Producer))
	case stop.PauseHonoured:
		return refuse(fmt.Sprintf("the halt at %q honours a pause, which the arbiter never relaunches over", stop.Producer))
	}
	return nil
}

// Readers holds the disk reads that feed the eligibility decision.
type Readers struct {
	// PendingEscalation reports, at a Bouncer row, whether the latest round awaits a decision on its escalation.
	// It gives the cause and brief path; other rows report nothing pending, and it may be nil for a batten run.
	PendingEscalation func(producer string) (cause shedadapters.EscalationCause, briefPath string, pending bool, err error)
	// InterruptPolicy returns the interrupt policy of a row.
	InterruptPolicy func(producer string) string
	// Batten selects the batten eligibility rule for every stop judged.
	Batten bool
}

// NewBattenReaders builds the readers of a batten run, where every stop follows the batten rule.
// No `PendingEscalation` read is made, and every row has the hand-back interrupt policy.
func NewBattenReaders() Readers {
	return Readers{
		Batten:          true,
		InterruptPolicy: func(string) string { return shedverbs.InterruptPolicyHandback },
	}
}

// Check judges whether the arbiter may settle the stop that status, kind and transient class describe.
// It judges on the facts the readers supply, and returns an error explaining a refusal.
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
		DecisionHalt:    status.State == shedengine.StateBlocked && strings.HasPrefix(status.Error, shedadapters.DecisionHaltPrefix),
		Batten:          readers.Batten,
	}
	if status.State == shedengine.StateBlocked {
		stop.BudgetExhausted = strings.HasPrefix(status.Error, shedengine.ReasonBounceBudgetExhausted)
		stop.PreconditionUnmet = strings.HasPrefix(status.Error, shedengine.ReasonPreconditionUnmet)
		stop.PauseHonoured = status.CurrentProducer == battenrecipe.NameLand && strings.HasPrefix(status.Error, battenshed.PauseHonouredPrefix)
	}
	var escalationBrief string
	if status.State == shedengine.StateAwaiting && readers.PendingEscalation != nil {
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

// EscalationEligible reports whether the arbiter may settle an awaiting stop at producer.
// The stop carries no envelope kind or transient class.
func EscalationEligible(producer string, readers Readers) bool {
	status := shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: producer}
	_, err := Check(status, "", "", readers)
	return err == nil
}

// NewReaders builds the real readers of the task worktree at location.
func NewReaders(location *lymcwd.Location) Readers {
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
