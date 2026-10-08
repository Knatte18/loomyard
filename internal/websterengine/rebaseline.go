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

// followUpCardLanding is the landing for a decision that reaches a card whose batch is done, or an overview outside its Card Index: a follow-up card after the last begun batch.
// It carries no "way forward: " prefix.
const followUpCardLanding = "add a follow-up card after the last begun batch that carries the decision, with its Card Index line in " + planOverviewFile + ", then run `lyx webster rebaseline --card NN` naming it"

// overviewIndexOnly reports whether 00-overview.md differs from the run's record only inside its Card Index: the state records an overview frame hash and the file's frame hash still equals it.
// A state without the hash reports false.
func overviewIndexOnly(st *State, planDir string) (bool, error) {
	if st.PlanOverviewFrameHash == "" {
		return false, nil
	}
	now, err := overviewFrameHash(planDir)
	if err != nil {
		return false, err
	}
	return now == st.PlanOverviewFrameHash, nil
}

// overviewRefusal is the refusal for a 00-overview.md change Rebaseline cannot accept: one outside the Card Index, or any change in a state that recorded no overview frame.
func overviewRefusal(st *State) error {
	if st.PlanOverviewFrameHash == "" {
		return fmt.Errorf("%w: %s changed and this run recorded no overview frame, so the change cannot be confined to its Card Index; it carries the plan's integration verify and is never rebaselined; way forward: restore %s, or %s", ErrRebaselineCardSetChanged, planOverviewFile, planOverviewFile, freshRestartSteps(stepRun))
	}
	return fmt.Errorf("%w: %s changed outside its Card Index; it carries the plan's integration verify and only its Card Index is rebaselined; way forward: restore %s, or %s", ErrRebaselineCardSetChanged, planOverviewFile, planOverviewFile, followUpCardLanding)
}

// editableTerminalStatus reports whether a batch with this terminal status accepts a named card edit.
func editableTerminalStatus(status string) bool {
	return status == DigestStatusFailed || status == DigestStatusDead || status == DigestStatusStuck
}

// cardNumberInt returns the number of a NN-<slug> card id, or 0 when the id has none.
func cardNumberInt(id string) int {
	n, _ := strconv.Atoi(cardNumberOf(id))
	return n
}

// RebaselineDeps is what Rebaseline reads: the edited on-disk plan, the batchifier and size source that group its cards, and the run state whose fingerprint it restamps.
type RebaselineDeps struct {
	Plan   *planparser.Plan
	Active batcher.Batcher
	Sizes  batcher.SizeSource
	// Base is Merriam's start base the tail's batches are priced from.
	Base  batcher.StartBase
	State *State
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
	// CardsAmended are the NN-<slug> ids of the accepted cards that belong to an in-flight batch, which a recovery re-runs on the edited text.
	CardsAmended []string
}

// rebaselineBatches returns the batches the edited plan runs as, and whether they replace the recorded partition.
// With a recorded partition, the batches up to and including the last begun one are kept as recorded, with their profile and estimate, and every plan card after them is batched by deps.Active;
// a plan whose first cards are not exactly the kept batches' recorded cards wraps ErrRebaselineCardSetChanged.
// A state without a partition is grouped by the identity batchifier and records none.
// Either way the result is asserted with CheckBatchOrder.
// The tail is batched told how many batches precede it, the number kept, so its first batch is priced at the position after them.
// That count is derived in Go from the recorded partition, never typed by an agent, and is told as if Merriam were never resumed, so after a resume the estimate is conservative;
// work Merriam does between batches that the model does not know makes it too low, and nothing re-checks it at run time.
func rebaselineBatches(deps RebaselineDeps) ([]batcher.Batch, bool, error) {
	st, plan := deps.State, deps.Plan
	if len(st.Partition) == 0 {
		batches, err := formBatches(plan, batcher.Identity(), deps.Sizes, 0, deps.Base)
		return batches, false, err
	}

	keptCount := 0
	for i, pb := range st.Partition {
		if st.Batches[cardNumberInt(pb.Cards[0])] != nil {
			keptCount = i + 1
		}
	}

	var changed []string
	var batches []batcher.Batch
	offset := 0
	for _, pb := range st.Partition[:keptCount] {
		end := min(offset+len(pb.Cards), len(plan.Cards))
		batch := batcher.Batch{Cards: plan.Cards[offset:end], Profile: pb.Profile, Estimate: pb.Estimate, Breakdown: pb.Breakdown}
		if now := batchCardIDs(batch); !slices.Equal(now, pb.Cards) {
			nowText := "no longer in the plan"
			if len(now) > 0 {
				nowText = "[" + strings.Join(now, ", ") + "]"
			}
			changed = append(changed, fmt.Sprintf("batch %d recorded [%s], plan now %s", cardNumberInt(pb.Cards[0]), strings.Join(pb.Cards, ", "), nowText))
		}
		batches = append(batches, batch)
		offset = end
	}
	if len(changed) > 0 {
		return nil, false, fmt.Errorf("%w: %s; way forward: restore those cards in the plan, or %s", ErrRebaselineCardSetChanged, strings.Join(changed, "; "), freshRestartSteps(stepRun))
	}

	if tail := plan.Cards[offset:]; len(tail) > 0 {
		tailBatches, err := batchCards(plan, tail, deps.Active, deps.Sizes, len(batches), deps.Base)
		if err != nil {
			return nil, false, err
		}
		batches = append(batches, tailBatches...)
	}
	if err := CheckBatchOrder(batches); err != nil {
		return nil, false, err
	}
	return batches, true, nil
}

// Rebaseline accepts the on-disk plan as the run's plan without discarding any batch record.
// With a recorded partition it keeps every batch up to the last begun one as recorded, re-batches the plan's cards after it with the active batchifier, and replaces State.Partition with the result once every check passes;
// cards added, removed or reordered after the last begun batch are accepted.
// It refuses, wrapping ErrRebaselineCardSetChanged, when a begun batch's card set differs from the card set the edited plan's batch of that number now holds, or the plan no longer has that number, or when a begun card's file content differs from the hash recorded at begin (a record without hashes compares ids only), except that a card named in deps.Cards is accepted when its batch is in flight or terminal failed, dead or stuck.
// It also refuses when a changed card file's number is not in deps.Cards, unless the state predates State.PlanFileHashes.
// A change to 00-overview.md is accepted only when it is confined to the Card Index: the state records State.PlanOverviewFrameHash and the file's frame hash still equals it.
// The index change is then held to the same card-set rule, so only cards after the last begun batch can be added, removed or reordered.
// A change outside the Card Index, and any overview change in a state without the frame hash, is refused.
// The start commit a refusal names is picked by git ancestry.
// The bound on the accepted card edit: only cards named with --card, only in batches in flight or terminal failed, dead or stuck, never a failed batch whose record lists Uncheckable entries, never a change to a batch's card-ID set, never 00-overview.md outside its Card Index.
// A begun card's reworded one-line intent in the Card Index passes, since the card file stays pinned by its recorded hash.
// A dead batch's strand, kept alive when classified dead, may still work on the old card, and so may the fork or recovery strand of an in-flight batch;
// the restamp stops no strand, and the next recover-batch stops it before it spawns and archives a late report from it.
// A card accepted in an in-flight batch is also recorded in the batch's AmendedCards with Rendered false, which resets an entry already there.
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

	batches, partition, err := rebaselineBatches(deps)
	if err != nil {
		return nil, err
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
				indexOnly, overviewErr := overviewIndexOnly(deps.State, deps.Plan.Dir)
				if overviewErr != nil {
					return nil, fmt.Errorf("%w; way forward: transient, re-run `lyx webster rebaseline`", overviewErr)
				}
				if !indexOnly {
					return nil, overviewRefusal(deps.State)
				}
				continue
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

	current := make(map[int][]string, len(batches))
	currentCards := make(map[int][]planparser.Card, len(batches))
	for _, b := range batches {
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

	var changed, doneChanged []string
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
				case bs.Terminal && !editableTerminalStatus(bs.Status):
					doneChanged = append(doneChanged, fmt.Sprintf("batch %d card %s changed since it was begun, and the batch is %s so its work has landed and --card cannot accept the edit", n, id, bs.Status))
				case bs.Terminal && len(bs.Uncheckable) > 0:
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
	if len(changed)+len(doneChanged) > 0 {
		wayForward := "restore those cards in the plan, or " + freshRestartSteps(stepRun)
		switch {
		case len(changed) == 0:
			wayForward = "restore those cards in the plan, or " + followUpCardLanding
		case len(doneChanged) > 0:
			wayForward = "restore those cards in the plan, or, for a done batch's card, " + followUpCardLanding + ", or " + freshRestartSteps(stepRun)
		}
		return nil, fmt.Errorf("%w: %s; way forward: %s", ErrRebaselineCardSetChanged, strings.Join(append(changed, doneChanged...), "; "), wayForward)
	}
	var cardsAmended []string
	for _, n := range numbers {
		bs := deps.State.Batches[n]
		ids := make([]string, 0, len(restamps[n]))
		for id := range restamps[n] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			bs.CardHashes[id] = restamps[n][id]
			if bs.Terminal {
				continue
			}
			cardsAmended = append(cardsAmended, id)
			if i := slices.IndexFunc(bs.AmendedCards, func(a AmendedCard) bool { return a.Card == id }); i >= 0 {
				bs.AmendedCards[i].Rendered = false
			} else {
				bs.AmendedCards = append(bs.AmendedCards, AmendedCard{Card: id})
			}
		}
	}
	sort.Strings(cardsAmended)

	previous := deps.State.PlanFingerprint
	if err := restampBaseline(deps.State, deps.Plan.Dir, deps.Geom.WebsterDir); err != nil {
		return nil, err
	}
	if partition {
		RecordPartition(deps.State, batches)
	}
	return &RebaselineResult{
		PreviousFingerprint: previous,
		Fingerprint:         deps.State.PlanFingerprint,
		BatchesKept:         len(numbers),
		CardsAccepted:       cardsAccepted,
		CardsAmended:        cardsAmended,
	}, nil
}
