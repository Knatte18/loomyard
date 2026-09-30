package websterengine

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrRebaselineCardSetChanged is the sentinel Rebaseline returns when the edited plan no longer holds
// a begun batch's recorded card set, which no restamp can make safe.
var ErrRebaselineCardSetChanged = errors.New("webster: the edited plan changes the cards of a batch this run already begun")

// RebaselineDeps is what Rebaseline reads: the edited on-disk plan, the batches sequenced from it,
// and the run state whose fingerprint it restamps.
type RebaselineDeps struct {
	Plan    *planparser.Plan
	Batches []batcher.Batch
	State   *State
}

// RebaselineResult reports what one successful Rebaseline changed.
type RebaselineResult struct {
	// PreviousFingerprint is State.PlanFingerprint before the restamp.
	PreviousFingerprint string
	// Fingerprint is the on-disk plan's fingerprint now recorded in State.
	Fingerprint string
	// BatchesKept is the number of begun batch records the edited plan still holds unchanged.
	BatchesKept int
}

// Rebaseline accepts the on-disk plan as the run's plan without discarding any batch record.
// It refuses, wrapping ErrRebaselineCardSetChanged, when a begun batch's card set differs from the
// card set the edited plan's batch of that number now holds, or the plan no longer has that number.
// Otherwise it restamps State.PlanFingerprint and leaves every other field untouched.
// It never saves; the caller holds the state-mutation lease and saves, as for the bracket verbs.
func Rebaseline(deps RebaselineDeps) (*RebaselineResult, error) {
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: rebaseline requires a parsed plan; RebaselineDeps.Plan is nil")
	}
	if deps.State == nil {
		return nil, fmt.Errorf("webster: rebaseline requires loaded run state; RebaselineDeps.State is nil")
	}

	current := make(map[int][]string, len(deps.Batches))
	for _, b := range deps.Batches {
		n, _ := batchIdentity(b)
		current[n] = batchCardIDs(b)
	}

	numbers := make([]int, 0, len(deps.State.Batches))
	for n, bs := range deps.State.Batches {
		if n == integrationBatchKey || bs == nil {
			continue
		}
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	var changed []string
	startSHA := ""
	for _, n := range numbers {
		bs := deps.State.Batches[n]
		recorded := bs.Cards
		if len(recorded) == 0 {
			recorded = []string{fmt.Sprintf("%02d-%s", n, bs.Slug)}
		}
		if bs.StartSHA != "" && startSHA == "" {
			startSHA = bs.StartSHA
		}
		now, ok := current[n]
		if ok && slices.Equal(recorded, now) {
			continue
		}
		nowText := "no longer in the plan"
		if ok {
			nowText = "[" + strings.Join(now, ", ") + "]"
		}
		changed = append(changed, fmt.Sprintf("batch %d recorded [%s], plan now %s", n, strings.Join(recorded, ", "), nowText))
	}
	if len(changed) > 0 {
		return nil, fmt.Errorf("%w: %s; way forward: restore those cards in the plan, or reset the branch to the run's start commit %s with git and run \"lyx webster run --fresh\"", ErrRebaselineCardSetChanged, strings.Join(changed, "; "), startSHA)
	}

	previous := deps.State.PlanFingerprint
	if err := restampFingerprint(deps.State, deps.Plan.Dir); err != nil {
		return nil, err
	}
	return &RebaselineResult{
		PreviousFingerprint: previous,
		Fingerprint:         deps.State.PlanFingerprint,
		BatchesKept:         len(numbers),
	}, nil
}
