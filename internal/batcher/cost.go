// cost.go implements costBatcher, the cost-model Batcher: it splits the card sequence into the contiguous batches with the lowest estimated cost under hard limits, by an exact dynamic program.

package batcher

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// CostParams are the hard limits and weights of a cost-model batchifier.
type CostParams struct {
	// AloneAbove is the one-card cost above which a card never shares a fork.
	AloneAbove float64

	// Budget is the cost above which a segment of two or more cards is not formed.
	Budget float64

	// MaxCards is the most cards one batch holds.
	MaxCards int

	// Weights are the estimator's coefficients.
	Weights Weights
}

// costBatcher implements Batcher by minimising the summed SegmentCost of contiguous segments.
type costBatcher struct {
	name   string
	params CostParams
}

// NewCost returns a cost-model Batcher whose Name is name.
// A card whose own cost exceeds AloneAbove or Budget runs alone, so no input leaves the split infeasible.
func NewCost(name string, p CostParams) Batcher {
	return costBatcher{name: name, params: p}
}

// Name reports the profile name this batchifier was built under.
func (b costBatcher) Name() string {
	return b.name
}

// Batch splits cards into contiguous batches of minimum total estimated cost, in card order.
// A one-card segment is always feasible;
// a longer segment needs every member's own cost at most AloneAbove, at most MaxCards cards and a SegmentCost within Budget.
// A tie between splits goes to the one with more batches.
// Each returned Batch carries the profile name and its SegmentCost as Estimate.
func (b costBatcher) Batch(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource) ([]Batch, error) {
	own := make([]float64, len(cards))
	for i := range cards {
		cost, err := SegmentCost(plan, cards[i:i+1], sizes, b.params.Weights)
		if err != nil {
			return nil, fmt.Errorf("cost batcher %q: %w", b.name, err)
		}
		own[i] = cost
	}

	// best[i] is the minimum total cost of splitting cards[:i];
	// count[i] the batches of that split;
	// start[i] the first card of its last segment.
	best := make([]float64, len(cards)+1)
	count := make([]int, len(cards)+1)
	start := make([]int, len(cards)+1)
	segmentCosts := make([]float64, len(cards)+1)
	for end := 1; end <= len(cards); end++ {
		for begin := end - 1; begin >= 0; begin-- {
			cost := own[end-1]
			if end-begin > 1 {
				// Each longer segment still holds the member or card count that rules this one out.
				if end-begin > b.params.MaxCards || own[begin] > b.params.AloneAbove || own[end-1] > b.params.AloneAbove {
					break
				}
				var err error
				cost, err = SegmentCost(plan, cards[begin:end], sizes, b.params.Weights)
				if err != nil {
					return nil, fmt.Errorf("cost batcher %q: %w", b.name, err)
				}
				if cost > b.params.Budget {
					continue
				}
			}
			total := best[begin] + cost
			batches := count[begin] + 1
			if count[end] == 0 || total < best[end] || (total == best[end] && batches > count[end]) {
				best[end], count[end], start[end], segmentCosts[end] = total, batches, begin, cost
			}
		}
	}

	batches := make([]Batch, count[len(cards)])
	for end, i := len(cards), len(batches)-1; end > 0; i-- {
		batches[i] = Batch{Cards: cards[start[end]:end], Profile: b.name, Estimate: segmentCosts[end]}
		end = start[end]
	}
	return batches, nil
}
