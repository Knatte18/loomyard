// partition.go maps a run's recorded partition onto the current plan's cards.

package runplan

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrPartitionMismatch is the sentinel error for a recorded partition that does not name the same cards as the plan.
var ErrPartitionMismatch = errors.New("runplan: recorded partition does not match the plan")

// MapPartition maps the recorded batches onto the plan's cards by id.
// Each batch keeps its recorded profile and estimate.
// It refuses with ErrPartitionMismatch when a recorded id is not a card of the plan.
// It refuses the same way when a plan card is in no recorded batch.
// The refusal ends in wayForward, the caller's way forward from a plan that no longer matches its partition.
func MapPartition(plan *planparser.Plan, partition []PartitionBatch, wayForward string) ([]batcher.Batch, error) {
	cardsByID := make(map[string]planparser.Card, len(plan.Cards))
	for _, c := range plan.Cards {
		cardsByID[CardID(c)] = c
	}

	recorded := make(map[string]bool, len(plan.Cards))
	var unknown []string
	batches := make([]batcher.Batch, 0, len(partition))
	for _, pb := range partition {
		batch := batcher.Batch{Profile: pb.Profile, Estimate: pb.Estimate, Breakdown: pb.Breakdown}
		for _, id := range pb.Cards {
			card, ok := cardsByID[id]
			if !ok {
				unknown = append(unknown, id)
				continue
			}
			recorded[id] = true
			batch.Cards = append(batch.Cards, card)
		}
		batches = append(batches, batch)
	}

	var unrecorded []string
	for _, c := range plan.Cards {
		if id := CardID(c); !recorded[id] {
			unrecorded = append(unrecorded, id)
		}
	}
	if len(unknown) == 0 && len(unrecorded) == 0 {
		return batches, nil
	}

	sort.Strings(unknown)
	var problems []string
	if len(unknown) > 0 {
		problems = append(problems, "recorded cards "+strings.Join(unknown, ", ")+" are not cards of the plan")
	}
	if len(unrecorded) > 0 {
		problems = append(problems, "plan cards "+strings.Join(unrecorded, ", ")+" are in no recorded batch")
	}
	return nil, fmt.Errorf("%w: %s; way forward: %s", ErrPartitionMismatch, strings.Join(problems, "; "), wayForward)
}

// FindBatch returns the batch in batches whose number matches number.
// When none matches, it returns an error ending in wayForward, the caller's way to list the run's batches.
func FindBatch(batches []batcher.Batch, number int, wayForward string) (batcher.Batch, error) {
	for _, b := range batches {
		if n, _ := BatchIdentity(b); n == number {
			return b, nil
		}
	}
	return batcher.Batch{}, fmt.Errorf("runplan: batch %d not found in the plan's execution batches; way forward: %s", number, wayForward)
}

// CompletedCards returns the cards of every batch that has reached a terminal classification, in batches' own order.
//
// It scopes the dispatch-boundary re-resolution and the post-batch drift detection to work that has not landed yet.
// A plan describes intended change, so a card already built contradicts the tree it is re-resolved against:
// its Create target exists, its Delete target is gone, and its Rename's old side no longer resolves.
// A non-zero exclude also counts that batch as completed:
// the post-batch checks run while the batch they record is still non-terminal, and that batch's work has just landed.
func CompletedCards(batches []batcher.Batch, st *State, exclude int) []planparser.Card {
	if st == nil {
		return nil
	}

	var done []planparser.Card
	for _, b := range batches {
		number, _ := BatchIdentity(b)
		if number != exclude {
			bs, ok := st.Batches[number]
			if !ok || bs == nil || !bs.Terminal {
				continue
			}
		}
		done = append(done, b.Cards...)
	}
	return done
}

// UnbegunCards returns the cards of every batch with no record in st, in batches' own order.
func UnbegunCards(batches []batcher.Batch, st *State) []planparser.Card {
	var cards []planparser.Card
	for _, b := range batches {
		number, _ := BatchIdentity(b)
		if bs, ok := st.Batches[number]; ok && bs != nil {
			continue
		}
		cards = append(cards, b.Cards...)
	}
	return cards
}

// DispatchScope computes the dispatch scoping an implementer's begin, run and validate paths share.
// begun is every card of a batch that has a record, terminal or not, in batches' own order.
// forthcoming is the cards of every begun batch whose record is not terminal.
// A terminal batch's cards are never forthcoming: its done-checks proved its targets present.
// A nil state returns two nil slices.
//
// A batch begun but never recorded may already have landed its work, so its Create targets can exist.
// Validating it as unstarted would refuse the very resume that records it.
// Its targets may equally not have landed yet, which is why forthcoming exists:
// the dispatch validation excludes a forthcoming card's Create and Rename New targets from its status check,
// so a later card that Uses them is not refused.
func DispatchScope(batches []batcher.Batch, st *State) (begun, forthcoming []planparser.Card) {
	if st == nil {
		return nil, nil
	}

	for _, b := range batches {
		number, _ := BatchIdentity(b)
		bs, ok := st.Batches[number]
		if !ok || bs == nil {
			continue
		}
		begun = append(begun, b.Cards...)
		if !bs.Terminal {
			forthcoming = append(forthcoming, b.Cards...)
		}
	}
	return begun, forthcoming
}
