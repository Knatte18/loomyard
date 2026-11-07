// cost.go implements costBatcher, the cost-model batcher.
// It splits the card sequence into the fewest contiguous batches whose estimated peak context fits the budget,
// and no batch holds a card that renames or deletes an earlier card's target.

package batcher

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// CostParams are the hard limits and weights of a cost-model batchifier.
type CostParams struct {
	// Budget is the estimated peak context above which a segment of two or more cards is not formed.
	Budget float64

	// MaxCards is the most cards one batch holds.
	MaxCards int

	// Weights are the estimator's coefficients.
	Weights Weights
}

// costBatcher batches cards by minimising the number of contiguous segments that fit the budget.
type costBatcher struct {
	name   string
	params CostParams
}

// NewCost returns a cost-model batcher that reports name as its profile name.
// A card whose own estimated peak context exceeds the budget runs alone, so no input leaves the split infeasible.
func NewCost(name string, p CostParams) Batcher {
	return costBatcher{name: name, params: p}
}

// Name reports the profile name this batchifier was built under.
func (b costBatcher) Name() string {
	return b.name
}

// split is the best split found of a prefix of the cards into a fixed number of batches:
// its largest batch's peak context, and the first card and peak context of its last batch.
type split struct {
	found   bool
	largest float64
	start   int
	peak    float64
}

// Batch splits cards into the fewest contiguous batches, in card order.
// The k-th batch of the result is the fork at position before + k.
// before is the number of batches the run executes ahead of cards[0].
// A batch's start context grows with its position, so a segment's peak depends on the batch it forms.
// base is part of every segment's start context.
// A one-card segment is always feasible;
// a longer segment holds at most MaxCards cards and keeps its peak within the budget at its own position.
// It also contains no card that renames or deletes an earlier card's target in the segment.
// The budget limits only segments of two or more cards,
// so a card whose own peak at its position exceeds it still runs alone, and nothing halts or warns;
// the batch's estimate and the position it was priced at show it.
// Among splits with the fewest batches, it takes the one whose largest batch has the smallest estimated peak context.
// That keeps batches balanced; a remaining tie goes to the split whose last batch is shortest.
// Each returned Batch carries the profile name and its estimated peak context at its own position as Estimate.
// Its Breakdown holds the components behind that estimate.
// The search is a dynamic program over (batches so far, last card placed), O(cards² × MaxCards) in segment evaluations.
func (b costBatcher) Batch(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource, before int, base StartBase) ([]Batch, error) {
	w := b.params.Weights
	loads := make([]cardLoad, len(cards))
	for i, card := range cards {
		load, err := loadCard(plan, card, sizes, w)
		if err != nil {
			return nil, fmt.Errorf("cost batcher %q: estimate card %d: %w", b.name, card.Number, err)
		}
		loads[i] = load
	}

	boundaries := renameBoundaries(plan, cards)

	// table[k][end] is the best split of cards[:end] into exactly k batches, the last of them at position before + k.
	table := make([][]split, len(cards)+1)
	for k := range table {
		table[k] = make([]split, len(cards)+1)
	}
	table[0][0] = split{found: true}
	for end := 1; end <= len(cards); end++ {
		var peak peakAccumulator
		peak.start(w, base, before+1)
		for begin := end - 1; begin >= 0; begin-- {
			peak.add(loads[begin], w)
			if end-begin > 1 && (end-begin > b.params.MaxCards || peak.value > b.params.Budget || boundaries[begin] < end) {
				// The peak at the first position only grows as the segment reaches back.
				// Later positions only add to it, so no longer segment ending here fits either.
				// The inner loop only lowers begin,
				// so a longer segment ending at end still holds the covering pair and is infeasible too.
				break
			}
			// peak.value is the segment's peak at position before + 1; each later position adds its growth.
			for k := 1; k <= begin+1; k++ {
				prefix := table[k-1][begin]
				if !prefix.found {
					continue
				}
				segmentPeak := peak.value + float64(k-1)*w.BatchGrowth
				if end-begin > 1 && segmentPeak > b.params.Budget {
					continue
				}
				largest := max(prefix.largest, segmentPeak)
				if current := table[k][end]; !current.found || largest < current.largest {
					table[k][end] = split{found: true, largest: largest, start: begin, peak: segmentPeak}
				}
			}
		}
	}

	count := 0
	for k := 1; k <= len(cards) && count == 0; k++ {
		if table[k][len(cards)].found {
			count = k
		}
	}
	batches := make([]Batch, count)
	for end, k := len(cards), count; k > 0; k-- {
		begin := table[k][end].start
		breakdown := breakdownOf(cards[begin:end], loads[begin:end], w, base, before+k)
		batches[k-1] = Batch{Cards: cards[begin:end], Profile: b.name, Estimate: table[k][end].peak, Breakdown: &breakdown}
		end = begin
	}
	return batches, nil
}
