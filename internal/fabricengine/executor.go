// executor.go holds the loop that drives one verb's rule and the one mapping from an error to its outcome class.

package fabricengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// snapshotRead reads the named fields of phase into snapshot.
// A read that fails for a field returns a *fieldReadError, or several of them joined.
type snapshotRead func(phase fabriccore.Phase, fields []fabriccore.Field, snapshot *fabriccore.Snapshot) error

// actionRunner runs one action of the kind it is registered for.
type actionRunner func(action fabriccore.Action) error

// fieldReadError is the failure of one field's read in one store.
type fieldReadError struct {
	Field fabriccore.Field
	Store fabriccore.Store
	Err   error
}

// Error names the field and the store and keeps the inner error's message.
func (e *fieldReadError) Error() string {
	return fmt.Sprintf("read %s of the %s store: %v", e.Field, e.Store, e.Err)
}

// Unwrap returns the inner error, so errors.Is sees what it wraps.
func (e *fieldReadError) Unwrap() error {
	return e.Err
}

// runRule drives rule from an empty snapshot, taking start as its first answer, until an answer holds nothing.
// A refusal is returned as the error; a phase request locks and reads that phase;
// an action list runs in order until its first action that is not done, and its reread fields are then read again.
// A read that failed only as transport failures is recorded in the snapshot for the rule to decide on,
// and any other read error is returned with the snapshot as it stands.
// Every lock taken is held until runRule returns and released in reverse order.
// A nil lock takes none, and a nil sleep sleeps for real.
func runRule(
	rule fabriccore.Rule,
	start fabriccore.Answer,
	read snapshotRead,
	lock func(phase fabriccore.Phase) (release func(), err error),
	runners map[fabriccore.ActionKind]actionRunner,
	sleep func(time.Duration),
) (fabriccore.Snapshot, error) {
	var snapshot fabriccore.Snapshot
	var releases []func()
	defer func() {
		for _, release := range slices.Backward(releases) {
			release()
		}
	}()
	if sleep == nil {
		sleep = time.Sleep
	}

	// recordTransportFailures records err as read failures of phase and reports whether it did:
	// it does so only when every error err holds is a field's transport failure.
	recordTransportFailures := func(phase fabriccore.Phase, err error) bool {
		parts := []error{err}
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			parts = joined.Unwrap()
		}
		var failures []fabriccore.ReadFailure
		for _, part := range parts {
			var fieldErr *fieldReadError
			if !errors.As(part, &fieldErr) || outcomeClassOf(fieldErr.Err) != fabriccore.OutcomeTransportFailure {
				return false
			}
			failures = append(failures, fabriccore.ReadFailure{
				Phase:  phase,
				Field:  fieldErr.Field,
				Store:  fieldErr.Store,
				Class:  fabriccore.OutcomeTransportFailure,
				Detail: fieldErr.Err.Error(),
			})
		}
		snapshot.ReadFailures = append(snapshot.ReadFailures, failures...)
		return true
	}

	// latest is the phase requested last, whether or not its read succeeded.
	var latest fabriccore.Phase
	locked := map[fabriccore.Phase]bool{}
	answer := start
	for {
		var parts []string
		if answer.Refusal != nil {
			parts = append(parts, "a refusal")
		}
		if answer.Phase != "" {
			parts = append(parts, "a phase request")
		}
		if len(answer.Actions) > 0 {
			parts = append(parts, "an action list")
		}
		if len(parts) > 1 {
			return snapshot, fmt.Errorf("fabricengine: a rule answered %s at once; an answer holds one of them", strings.Join(parts, " and "))
		}

		switch {
		case answer.Refusal != nil:
			return snapshot, answer.Refusal

		case answer.Phase != "":
			latest = answer.Phase
			if lock != nil && !locked[latest] {
				release, err := lock(latest)
				if err != nil {
					return snapshot, err
				}
				releases = append(releases, release)
				locked[latest] = true
			}
			if err := read(latest, answer.Read, &snapshot); err != nil {
				if !recordTransportFailures(latest, err) {
					return snapshot, err
				}
			} else if !slices.Contains(snapshot.Phases, latest) {
				snapshot.Phases = append(snapshot.Phases, latest)
			}

		case len(answer.Actions) > 0:
			// Checked for the whole list first, so a missing runner leaves no action of the list run.
			for _, action := range answer.Actions {
				if action.Kind != fabriccore.ActionWait && runners[action.Kind] == nil {
					return snapshot, fmt.Errorf("fabricengine: no runner is registered for the action kind %q", action.Kind)
				}
			}
			for _, action := range answer.Actions {
				var err error
				if action.Kind == fabriccore.ActionWait {
					sleep(action.Delay)
				} else {
					err = runners[action.Kind](action)
				}
				outcome := fabriccore.Outcome{Action: action, Class: outcomeClassOf(err)}
				if err != nil {
					outcome.Detail = err.Error()
				}
				snapshot.Outcomes = append(snapshot.Outcomes, outcome)
				if outcome.Class != fabriccore.OutcomeDone {
					break
				}
			}
			if err := read(latest, answer.Reread, &snapshot); err != nil && !recordTransportFailures(latest, err) {
				return snapshot, err
			}

		default:
			return snapshot, nil
		}
		answer = rule(snapshot)
	}
}

// outcomeClassOf returns the class of the error an action or a read ended in; nil is done.
func outcomeClassOf(err error) fabriccore.OutcomeClass {
	var anchorStop *AnchorStopError
	var destructive *destructiveRefusal
	switch {
	case err == nil:
		return fabriccore.OutcomeDone
	case errors.As(err, &anchorStop):
		return fabriccore.OutcomeAnchorStop
	case errors.Is(err, gitrepo.ErrPushAuthRefused):
		return fabriccore.OutcomeAuthRefused
	case errors.Is(err, gitrepo.ErrPushRejected):
		return fabriccore.OutcomeRejected
	case errors.As(err, &destructive):
		return fabriccore.OutcomeDestructiveRefusal
	case gitexec.IsTransportFailure(err):
		return fabriccore.OutcomeTransportFailure
	default:
		return fabriccore.OutcomeFailed
	}
}
