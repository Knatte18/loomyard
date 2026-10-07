// cost.go implements costBatcher, the cost-model Batcher: it splits the card sequence into the fewest contiguous batches whose estimated peak context fits the budget, by an exact dynamic program.

package batcher

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// CostParams are the hard limits and weights of a cost-model batchifier.
type CostParams struct {
	// Budget is the PeakContext above which a segment of two or more cards is not formed.
	Budget float64

	// MaxCards is the most cards one batch holds.
	MaxCards int

	// Weights are the estimator's coefficients.
	Weights Weights
}

// costBatcher implements Batcher by minimising the number of contiguous segments that fit the budget.
type costBatcher struct {
	name   string
	params CostParams
}

// NewCost returns a cost-model Batcher whose Name is name.
// A card whose own PeakContext exceeds Budget runs alone, so no input leaves the split infeasible.
func NewCost(name string, p CostParams) Batcher {
	return costBatcher{name: name, params: p}
}

// Name reports the profile name this batchifier was built under.
func (b costBatcher) Name() string {
	return b.name
}

// split is the best split found of a prefix of the cards: its batch count and its largest batch's PeakContext.
type split struct {
	batches int
	largest float64
}

// better reports whether s beats o: fewer batches, then a smaller largest batch.
func (s split) better(o split) bool {
	return s.batches < o.batches || (s.batches == o.batches && s.largest < o.largest)
}

// Batch splits cards into the fewest contiguous batches, in card order.
// A one-card segment is always feasible;
// a longer segment needs at most MaxCards cards and a PeakContext within Budget.
// Among splits with the fewest batches it takes the one whose largest batch has the smallest PeakContext, so batches come out balanced;
// a remaining tie goes to the split whose last batch is shortest.
// Each returned Batch carries the profile name, its PeakContext as Estimate and the components behind it as Breakdown.
func (b costBatcher) Batch(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource) ([]Batch, error) {
	loads := make([]cardLoad, len(cards))
	for i, card := range cards {
		load, err := loadCard(plan, card, sizes, b.params.Weights)
		if err != nil {
			return nil, fmt.Errorf("cost batcher %q: estimate card %d: %w", b.name, card.Number, err)
		}
		loads[i] = load
	}

	// best[i] is the best split of cards[:i];
	// start[i] the first card of its last segment and peaks[i] that segment's PeakContext.
	best := make([]split, len(cards)+1)
	start := make([]int, len(cards)+1)
	peaks := make([]float64, len(cards)+1)
	for end := 1; end <= len(cards); end++ {
		found := false
		var peak peakAccumulator
		peak.start(b.params.Weights)
		for begin := end - 1; begin >= 0; begin-- {
			peak.add(loads[begin], b.params.Weights)
			if end-begin > 1 && (end-begin > b.params.MaxCards || peak.value > b.params.Budget) {
				// The peak only grows as the segment reaches back, so no longer segment ending here fits either.
				break
			}
			candidate := split{batches: best[begin].batches + 1, largest: max(best[begin].largest, peak.value)}
			if !found || candidate.better(best[end]) {
				best[end], start[end], peaks[end], found = candidate, begin, peak.value, true
			}
		}
	}

	batches := make([]Batch, best[len(cards)].batches)
	for end, i := len(cards), len(batches)-1; end > 0; i-- {
		breakdown := breakdownOf(cards[start[end]:end], loads[start[end]:end], b.params.Weights)
		batches[i] = Batch{Cards: cards[start[end]:end], Profile: b.name, Estimate: peaks[end], Breakdown: &breakdown}
		end = start[end]
	}
	return batches, nil
}
