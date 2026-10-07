// rebaseline.go implements Rebaseline, the engine half of `lyx webster rebaseline`:
// it accepts a mid-run plan edit as the run's plan while every begun batch keeps its recorded card set.

package websterengine

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrRebaselineCardSetChanged is the sentinel Rebaseline returns when the edited plan no longer holds a begun batch's recorded card set, which no restamp can make safe.
var ErrRebaselineCardSetChanged = errors.New("webster: the edited plan changes the cards of a batch this run already begun")

// editableTerminalStatus reports whether a batch with this terminal status accepts a named card edit.
func editableTerminalStatus(status string) bool {
	return status == DigestStatusFailed || status == DigestStatusDead || status == DigestStatusStuck
}

// cardNumberInt returns the number of a NN-<slug> card id, or 0 when the id has none.
func cardNumberInt(id string) int {
	n, _ := strconv.Atoi(cardNumberOf(id))
	return n
}

// RebaselineDeps is what Rebaseline reads: the edited on-disk plan, the batches sequenced from it,
// and the run state whose fingerprint it restamps.
type RebaselineDeps struct {
	Plan    *planparser.Plan
	Batches []batcher.Batch
	State   *State
	// Cards are the card numbers the operator names as edited; a changed card file whose number is absent is refused.
	Cards []int
	// Geom locates the worktree whose history names the run's start commit.
	Geom Geometry
}

// RebaselineResult reports what one successful Rebaseline changed.
type RebaselineResult struct {
	// PreviousFingerprint is State.PlanFingerprint before the restamp.
	PreviousFingerprint string
	// Fingerprint is the on-disk plan's fingerprint now recorded in State.
	Fingerprint string
	// BatchesKept is the number of begun batch records the edited plan still holds unchanged.
	BatchesKept int
	// CardsAccepted are the changed card file names this call accepted.
	CardsAccepted []string
}

// Rebaseline accepts the on-disk plan as the run's plan without discarding any batch record.
// It refuses, wrapping ErrRebaselineCardSetChanged, when a begun batch's card set differs from the card set the edited plan's batch of that number now holds, or the plan no longer has that number,
// or when a begun card's file content differs from the hash recorded at begin (a record without hashes compares ids only),
// except that a card named in deps.Cards is accepted when its batch is terminal failed, dead or stuck.
// It also refuses when 00-overview.md changed, or a changed card file's number is not in deps.Cards, unless the state predates State.PlanFileHashes.
// The start commit a refusal names is picked by git ancestry.
// The bound on the accepted card edit: only cards named with --card, only in batches terminal failed, dead or stuck,
// never a failed batch whose record lists Uncheckable entries, never a change to a batch's card-ID set, never 00-overview.md.
// A dead batch's strand, kept alive when classified dead, may still work on the old card;
// the restamp stops no strand, and the next recover-batch stops it before it spawns and archives a late report from it.
// Otherwise it restamps State.PlanFingerprint, State.PlanFileHashes and the CardHashes entry of each accepted card, and leaves every other field untouched.
// A refusal leaves the state unchanged.
// It never saves;
// the caller holds the state-mutation lease and saves, as for the bracket verbs.
func Rebaseline(deps RebaselineDeps) (*RebaselineResult, error) {
	if deps.Plan == nil {
		return nil, fmt.Errorf("webster: rebaseline requires a parsed plan; RebaselineDeps.Plan is nil")
	}
	if deps.State == nil {
		return nil, fmt.Errorf("webster: rebaseline requires loaded run state; RebaselineDeps.State is nil")
	}

	var cardsAccepted []string
	if len(deps.State.PlanFileHashes) > 0 {
		changedFiles, err := changedPlanFiles(deps.State, deps.Plan.Dir)
		if err != nil {
			return nil, fmt.Errorf("%w; way forward: transient, re-run `lyx webster rebaseline`", err)
		}
		var unnamed []string
		for _, name := range changedFiles {
			if name == planOverviewFile {
				return nil, fmt.Errorf("%w: %s changed; it carries the plan's integration verify and is never rebaselined; way forward: restore %s, or %s", ErrRebaselineCardSetChanged, planOverviewFile, planOverviewFile, freshRestartSteps)
			}
			n, convErr := strconv.Atoi(cardNumberOf(name))
			if convErr != nil || !slices.Contains(deps.Cards, n) {
				unnamed = append(unnamed, name)
				continue
			}
			cardsAccepted = append(cardsAccepted, name)
		}
		if len(unnamed) > 0 {
			return nil, fmt.Errorf("%w: %s changed but not named; way forward: re-run \"lyx webster rebaseline\" naming every changed card with --card NN, or restore the unnamed cards", ErrRebaselineCardSetChanged, strings.Join(unnamed, ", "))
		}
	}

	current := make(map[int][]string, len(deps.Batches))
	currentCards := make(map[int][]planparser.Card, len(deps.Batches))
	for _, b := range deps.Batches {
		n, _ := batchIdentity(b)
		current[n] = batchCardIDs(b)
		currentCards[n] = b.Cards
	}

	numbers := make([]int, 0, len(deps.State.Batches))
	for n, bs := range deps.State.Batches {
		if bs == nil {
			continue
		}
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	var changed, unfinished []string
	restamps := make(map[int]map[string]string)
	for _, n := range numbers {
		bs := deps.State.Batches[n]
		recorded := bs.Cards
		if len(recorded) == 0 {
			recorded = []string{fmt.Sprintf("%02d-%s", n, bs.Slug)}
		}
		now, ok := current[n]
		if ok && slices.Equal(recorded, now) {
			if len(bs.CardHashes) == 0 {
				continue
			}
			got, err := batchCardHashes(batcher.Batch{Cards: currentCards[n]}, deps.Plan.Dir)
			if err != nil {
				return nil, fmt.Errorf("%w; way forward: transient, re-run `lyx webster rebaseline`", err)
			}
			for _, id := range recorded {
				want, hashed := bs.CardHashes[id]
				if !hashed || got[id] == want {
					continue
				}
				switch {
				case !bs.Terminal:
					unfinished = append(unfinished, fmt.Sprintf("batch %d card %s changed since it was begun, and the batch is unfinished so a fork may still be working on it; run `lyx webster record-batch %d` or `lyx webster recover-batch %d` first, then re-run the rebaseline", n, id, n, n))
				case !editableTerminalStatus(bs.Status):
					changed = append(changed, fmt.Sprintf("batch %d card %s changed since it was begun, and the batch is %s so its work has landed and --card cannot accept the edit", n, id, bs.Status))
				case len(bs.Uncheckable) > 0:
					changed = append(changed, fmt.Sprintf("batch %d card %s changed since it was begun, and the batch failed on findings recovery cannot check so --card cannot accept the edit", n, id))
				case !slices.Contains(deps.Cards, cardNumberInt(id)):
					changed = append(changed, fmt.Sprintf("batch %d card %s changed since it was begun and is not named with --card", n, id))
				default:
					if restamps[n] == nil {
						restamps[n] = make(map[string]string)
					}
					restamps[n][id] = got[id]
				}
			}
			continue
		}
		nowText := "no longer in the plan"
		if ok {
			nowText = "[" + strings.Join(now, ", ") + "]"
		}
		changed = append(changed, fmt.Sprintf("batch %d recorded [%s], plan now %s", n, strings.Join(recorded, ", "), nowText))
	}
	if len(changed) > 0 {
		reasons := append(changed, unfinished...)
		return nil, fmt.Errorf("%w: %s; way forward: restore those cards in the plan, or %s", ErrRebaselineCardSetChanged, strings.Join(reasons, "; "), freshRestartSteps)
	}
	if len(unfinished) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrRebaselineCardSetChanged, strings.Join(unfinished, "; "))
	}
	for n, hashes := range restamps {
		for id, hash := range hashes {
			deps.State.Batches[n].CardHashes[id] = hash
		}
	}

	previous := deps.State.PlanFingerprint
	if err := restampBaseline(deps.State, deps.Plan.Dir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}
	return &RebaselineResult{
		PreviousFingerprint: previous,
		Fingerprint:         deps.State.PlanFingerprint,
		BatchesKept:         len(numbers),
		CardsAccepted:       cardsAccepted,
	}, nil
}
