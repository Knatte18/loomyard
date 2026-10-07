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

// split is the best split found of a prefix of the cards into a fixed number of batches:
// its largest batch's PeakContext, and the first card and PeakContext of its last batch.
type split struct {
	found   bool
	largest float64
	start   int
	peak    float64
}

// Batch splits cards into the fewest contiguous batches, in card order.
// The k-th batch of the result is the fork at position k, whose start context grows with k, so a segment's peak depends on the batch it forms.
// A one-card segment is always feasible;
// a longer segment needs at most MaxCards cards and a PeakContext at its own position within Budget.
// Budget limits only segments of two or more cards, so a card whose own peak at its position exceeds it still runs alone, and nothing halts or warns;
// the recorded Estimate and Position show it.
// Among splits with the fewest batches it takes the one whose largest batch has the smallest PeakContext, so batches come out balanced;
// a remaining tie goes to the split whose last batch is shortest.
// Each returned Batch carries the profile name, its PeakContext at its own position as Estimate and the components behind it as Breakdown.
// The search is a dynamic program over (batches so far, last card placed), O(cards² × MaxCards) in segment evaluations.
func (b costBatcher) Batch(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource) ([]Batch, error) {
	w := b.params.Weights
	loads := make([]cardLoad, len(cards))
	for i, card := range cards {
		load, err := loadCard(plan, card, sizes, w)
		if err != nil {
			return nil, fmt.Errorf("cost batcher %q: estimate card %d: %w", b.name, card.Number, err)
		}
		loads[i] = load
	}

	// table[k][end] is the best split of cards[:end] into exactly k batches, the last of them at position k.
	table := make([][]split, len(cards)+1)
	for k := range table {
		table[k] = make([]split, len(cards)+1)
	}
	table[0][0] = split{found: true}
	for end := 1; end <= len(cards); end++ {
		var peak peakAccumulator
		peak.start(w, 1)
		for begin := end - 1; begin >= 0; begin-- {
			peak.add(loads[begin], w)
			if end-begin > 1 && (end-begin > b.params.MaxCards || peak.value > b.params.Budget) {
				// The peak at position 1 only grows as the segment reaches back, and later positions only add to it, so no longer segment ending here fits either.
				break
			}
			// peak.value is the segment's peak at position 1; each later position adds its growth.
			for k := 1; k <= begin+1; k++ {
				before := table[k-1][begin]
				if !before.found {
					continue
				}
				segmentPeak := peak.value + float64(k-1)*w.BatchGrowth
				if end-begin > 1 && segmentPeak > b.params.Budget {
					continue
				}
				largest := max(before.largest, segmentPeak)
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
		breakdown := breakdownOf(cards[begin:end], loads[begin:end], w, k)
		batches[k-1] = Batch{Cards: cards[begin:end], Profile: b.name, Estimate: table[k][end].peak, Breakdown: &breakdown}
		end = begin
	}
	return batches, nil
}
