// arbiterstop.go decides which stops the arbiter may settle: one pure function over what Go reads off the status file and the stop envelope, and the reads that feed it.

package arbiterstop

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/verifytree"
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
	// VerifyHaltNamesTarget reports whether a blocked stop at Merge-In or Publish carries a verify-failure record that names a target and belongs to the halt.
	VerifyHaltNamesTarget bool
}

// Eligibility returns nil when the arbiter may settle stop and a refusal otherwise.
// Eligible are an error stop of kind producer or bootstrap, of kind interrupted under the reinvoke policy or with a non-empty transient class.
// Eligible too are a halted blocked stop unless it is a Merge-In or Publish verify halt whose own failure record names a target.
// Eligible too is a halted awaiting stop with a pending escalation of cause `hard-cap` or `circling`.
// Every other stop is refused, and the refusal ends with HandBackWayForward.
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
		if stop.VerifyHaltNamesTarget {
			return refuse("a Merge-In or Publish verify halt whose failure record names a target belongs to batten's retry and then to the parent")
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

// Readers holds the four disk reads Check makes.
type Readers struct {
	// PendingEscalation reports, at a Bouncer row, whether the latest round awaits a decision on its escalation, with the cause and the path of its brief; not pending elsewhere.
	PendingEscalation func(producer string) (cause shedadapters.EscalationCause, briefPath string, pending bool, err error)
	// VerifyRecord returns the verify-failure record and whether one exists.
	VerifyRecord func() (record battenshed.ChildPublishFailure, found bool, err error)
	// Head returns the pair's HEAD commit.
	Head func() (string, error)
	// InterruptPolicy returns the interrupt policy of a row.
	InterruptPolicy func(producer string) string
}

// Check builds the Stop for status, the envelope's kind and transient class from readers, and returns Eligibility's answer.
// The brief path is the review escalation's, and only for an admitted awaiting stop.
// An unreadable escalation record is a not-eligible refusal, never a separate error;
// a failure reading the verify record or HEAD at a blocked Merge-In or Publish stop is a refusal that ends with HandBackWayForward.
func Check(status shedengine.Status, kind, transient string, readers Readers) (briefPath string, err error) {
	stop := Stop{
		State:           status.State,
		Producer:        status.CurrentProducer,
		Kind:            kind,
		Transient:       transient,
		InterruptPolicy: readers.InterruptPolicy(status.CurrentProducer),
	}
	var escalationBrief string
	if status.State == shedengine.StateAwaiting {
		stop.EscalationCause, escalationBrief, stop.EscalationPending, stop.EscalationErr = readers.PendingEscalation(status.CurrentProducer)
	}
	if status.State == shedengine.StateBlocked && (status.CurrentProducer == loomshed.NameMergeIn || status.CurrentProducer == loomshed.NamePublish) {
		record, found, err := readers.VerifyRecord()
		if err != nil {
			return "", fmt.Errorf("arbiter: the verify failure record: %v; %s", err, HandBackWayForward)
		}
		if found && record.Target != "" {
			head, err := readers.Head()
			if err != nil {
				return "", fmt.Errorf("arbiter: the pair's HEAD: %v; %s", err, HandBackWayForward)
			}
			stop.VerifyHaltNamesTarget = battenshed.RecordMatchesHalt(record, head, status, []string{loomshed.NameMergeIn, loomshed.NamePublish})
		}
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
	verifyPaths := verifytree.NewPaths(location.WorktreePath(), verifytree.Dir(location.AnchorPath()))
	return Readers{
		PendingEscalation: func(producer string) (shedadapters.EscalationCause, string, bool, error) {
			subdir, isBouncer, err := loomrecipe.BouncerRunSubdir(producer)
			if err != nil || !isBouncer {
				return "", "", false, err
			}
			return shedadapters.PendingEscalationCause(filepath.Join(reviewsDir, subdir))
		},
		VerifyRecord: func() (battenshed.ChildPublishFailure, bool, error) {
			record, found, err := verifytree.ReadPublishFailure(verifyPaths)
			if err != nil || !found {
				return battenshed.ChildPublishFailure{}, false, err
			}
			return battenshed.ChildPublishFailure{Row: record.Row, Head: record.Head, Target: record.Target, WrittenAt: record.WrittenAt}, true, nil
		},
		Head: func() (string, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return "", err
			}
			return fab.HeadSHA()
		},
		InterruptPolicy: loomshed.InterruptPolicyFor,
	}
}
