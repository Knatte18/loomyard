// mergestate.go judges whether a pull request can merge into its base now, and names what stops it when it cannot.

package pullrequest

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// MergeRetryWindow is how long a merge is retried.
// A read of a pull request whose mergeability GitHub has not computed is retried for the same length of time.
const MergeRetryWindow = time.Minute

// GitHub's mergeable_state values, the strings the merge judgment compares against.
const (
	mergeableClean    = "clean"
	mergeableHasHooks = "has_hooks"
	mergeableUnstable = "unstable"
	mergeableDirty    = "dirty"
	mergeableBlocked  = "blocked"
	mergeableBehind   = "behind"
	mergeableUnknown  = "unknown"
	mergeableDraft    = "draft"
)

// satisfiedConclusions are the conclusions GitHub counts as satisfying a required check.
var satisfiedConclusions = []string{"success", "neutral", "skipped"}

// MergeState is a pull request's state as read from GitHub, with the judgment of whether it can merge now.
type MergeState struct {
	// State is where the pull request stands.
	State State
	// Draft reports whether the pull request is a draft.
	Draft bool
	// Head is the pull request's current head commit.
	Head string
	// MergeableState is GitHub's mergeable_state of the pull request.
	MergeableState string
	// Approved reports whether a standing review approves Head.
	Approved bool
	// Checks are the required checks of the base branch, each with its conclusion at Head.
	Checks []Check
	// MergeableNow reports whether GitHub would merge the pull request now.
	// It must be open, not a draft, at the beam's head, and in a mergeable state of clean, has_hooks or unstable.
	MergeableNow bool
	// Causes name what stops the merge, one per cause; empty exactly when MergeableNow.
	Causes []string
}

// ReadMergeState reads pull request number and the required checks of base at its head.
// It judges whether the pull request can merge now, with beamHead as the head it must be at.
// A mergeable_state that GitHub has not yet computed is read again after sleep, doubling from one second.
// Reads stop once MergeRetryWindow of the now clock has passed since the first read,
// and the unknown state is then reported as a cause.
// A failed GitHub call, and a failed sleep, are returned.
func ReadMergeState(ctx context.Context, api API, number int, beamHead, base string, now func() time.Time, sleep func(ctx context.Context, d time.Duration) error) (MergeState, error) {
	started := now()
	status, err := api.Status(ctx, number)
	if err != nil {
		return MergeState{}, fmt.Errorf("pullrequest: read pull request #%d: %w", number, err)
	}
	delay := time.Second
	for status.State == StateOpen && status.MergeableState == mergeableUnknown {
		elapsed := now().Sub(started)
		if elapsed >= MergeRetryWindow {
			break
		}
		if err := sleep(ctx, min(delay, MergeRetryWindow-elapsed)); err != nil {
			return MergeState{}, err
		}
		delay *= 2
		if status, err = api.Status(ctx, number); err != nil {
			return MergeState{}, fmt.Errorf("pullrequest: read pull request #%d: %w", number, err)
		}
	}
	checks, err := api.RequiredChecks(ctx, base, status.HeadSHA)
	if err != nil {
		return MergeState{}, fmt.Errorf("pullrequest: read the required checks of %s at %s: %w", base, status.HeadSHA, err)
	}

	causes := mergeCauses(status, beamHead, checks)
	return MergeState{
		State:          status.State,
		Draft:          status.Draft,
		Head:           status.HeadSHA,
		MergeableState: status.MergeableState,
		Approved:       status.Approved,
		Checks:         checks,
		MergeableNow:   len(causes) == 0,
		Causes:         causes,
	}, nil
}

// mergeCauses names every reason a pull request in status cannot merge with beamHead as its head.
// The reasons come in the order a reader fixes them.
// A pull request not open gets no mergeable_state cause, since GitHub's mergeable_state says nothing of one.
func mergeCauses(status Status, beamHead string, checks []Check) []string {
	var causes []string
	switch status.State {
	case StateClosed:
		causes = append(causes, "closed")
	case StateMerged:
		causes = append(causes, "merged")
	}
	if status.Draft {
		causes = append(causes, "draft")
	}
	if status.HeadSHA != beamHead {
		causes = append(causes, fmt.Sprintf("head %s is not the beam's head %s", status.HeadSHA, beamHead))
	}
	if status.State != StateOpen {
		return causes
	}

	switch status.MergeableState {
	case mergeableClean, mergeableHasHooks, mergeableUnstable, mergeableDraft:
	case mergeableDirty:
		causes = append(causes, "dirty: the pull request conflicts with its base branch")
	case mergeableBlocked:
		causes = append(causes, "blocked: a review or a required check is still owed")
		for _, check := range checks {
			if slices.Contains(satisfiedConclusions, check.Conclusion) {
				continue
			}
			conclusion := check.Conclusion
			if conclusion == "" {
				conclusion = "not finished"
			}
			causes = append(causes, fmt.Sprintf("required check %s is %s", check.Name, conclusion))
		}
	case mergeableBehind:
		causes = append(causes, "behind: the base branch is ahead of the pull request's head")
	case mergeableUnknown:
		causes = append(causes, "GitHub has not yet computed whether it can merge")
	default:
		causes = append(causes, fmt.Sprintf("the mergeable state %q is not one that merges", status.MergeableState))
	}
	return causes
}
