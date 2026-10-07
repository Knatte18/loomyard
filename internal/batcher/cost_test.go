// cost_test.go verifies the cost-model batchifier's hard limits, tie rule and optimality against a brute-force search over every contiguous split.
// Tier-1 (pure logic, no git, no spawn).

package batcher_test

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// costWeights make a one-card edit of a 20-line file peak at 10 startup + 1 fork message + 1 card message + 20 lines = 32.
var costWeights = batcher.Weights{StartupContext: 10, ForkMessages: 1, MessageContext: 1, TargetMessages: 1, ContextPerLine: 1}

var costSizes = fakeSizes{lines: map[string]int{
	"internal/a/a.go":   20,
	"internal/b/b.go":   20,
	"internal/c/c.go":   40,
	"internal/d/d.go":   10,
	"internal/big/b.go": 100,
}}

// batchSizes returns the card count of each batch, in order.
func batchSizes(batches []batcher.Batch) []int {
	sizes := make([]int, len(batches))
	for i, batch := range batches {
		sizes[i] = len(batch.Cards)
	}
	return sizes
}

// TestCostBatchifier_Limits asserts the grouping and per-batch peak the cost batchifier returns for each hard limit and the tie rule.
func TestCostBatchifier_Limits(t *testing.T) {
	t.Parallel()

	const unlimited = 1e9
	plan := &planparser.Plan{Language: "go"}
	sameFile := func(n int) []planparser.Card {
		cards := make([]planparser.Card, n)
		for i := range cards {
			cards[i] = editCard(i+1, []string{"internal/a/a.go"})
		}
		return cards
	}
	tests := []struct {
		name         string
		params       batcher.CostParams
		cards        []planparser.Card
		wantSizes    []int
		wantEstimate []float64
	}{
		{
			name:         "cards within the budget share one fork, and its peak counts a shared file once",
			params:       batcher.CostParams{Budget: unlimited, MaxCards: 5, Weights: costWeights},
			cards:        sameFile(3),
			wantSizes:    []int{3},
			wantEstimate: []float64{10 + 1 + 3 + 20},
		},
		{
			name:   "cards in unrelated packages share one fork when it fits",
			params: batcher.CostParams{Budget: unlimited, MaxCards: 5, Weights: costWeights},
			cards: []planparser.Card{
				editCard(1, []string{"internal/a/a.go"}),
				editCard(2, []string{"internal/b/b.go"}),
			},
			wantSizes:    []int{2},
			wantEstimate: []float64{10 + 1 + 2 + 20 + 20},
		},
		{
			name:   "a segment whose peak exceeds the budget is split",
			params: batcher.CostParams{Budget: 60, MaxCards: 5, Weights: costWeights},
			cards: []planparser.Card{
				editCard(1, []string{"internal/a/a.go"}),
				editCard(2, []string{"internal/b/b.go"}),
				editCard(3, []string{"internal/c/c.go"}),
			},
			wantSizes:    []int{2, 1},
			wantEstimate: []float64{53, 52},
		},
		{
			name:         "a card over the budget runs alone and the split is still returned",
			params:       batcher.CostParams{Budget: 30, MaxCards: 5, Weights: costWeights},
			cards:        sameFile(2),
			wantSizes:    []int{1, 1},
			wantEstimate: []float64{32, 32},
		},
		{
			name:      "no batch holds more than MaxCards, and a full tie leaves the last batch shortest",
			params:    batcher.CostParams{Budget: unlimited, MaxCards: 2, Weights: costWeights},
			cards:     sameFile(5),
			wantSizes: []int{2, 2, 1},
		},
		{
			// big alone peaks at 112 and big with a at 133, while a with b peaks at 53 and all three at 154.
			name:   "among the fewest batches the split with the smaller largest peak wins",
			params: batcher.CostParams{Budget: 140, MaxCards: 5, Weights: costWeights},
			cards: []planparser.Card{
				editCard(1, []string{"internal/big/b.go"}),
				editCard(2, []string{"internal/a/a.go"}),
				editCard(3, []string{"internal/b/b.go"}),
			},
			wantSizes: []int{1, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			batcherUnderTest := batcher.NewCost("cautious", tt.params)
			if batcherUnderTest.Name() != "cautious" {
				t.Errorf("Name = %q; want %q", batcherUnderTest.Name(), "cautious")
			}
			batches, err := batcherUnderTest.Batch(plan, tt.cards, costSizes)
			if err != nil {
				t.Fatalf("Batch: %v", err)
			}
			if got := batchSizes(batches); !reflect.DeepEqual(got, tt.wantSizes) {
				t.Errorf("batch sizes = %v; want %v", got, tt.wantSizes)
			}
			for i, want := range tt.wantEstimate {
				if batches[i].Estimate != want {
					t.Errorf("batch %d Estimate = %v; want %v", i, batches[i].Estimate, want)
				}
			}
		})
	}
}

// TestCostBatchifier_OptimalAndFeasible checks, over a seeded input set, that the split keeps card order, respects every hard limit, reports each batch's PeakContext with a breakdown whose components sum to it, and has the fewest batches of any feasible contiguous split, ties going to the smallest largest peak.
func TestCostBatchifier_OptimalAndFeasible(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	files := []string{"internal/a/a.go", "internal/b/b.go", "internal/c/c.go", "internal/d/d.go", "internal/big/b.go"}
	rng := rand.New(rand.NewSource(1))
	budgets := []float64{50, 100, 200, 1e9}
	maxCards := []int{1, 2, 3, 6}

	for iteration := 0; iteration < 200; iteration++ {
		weights := batcher.Weights{
			StartupContext: float64(1 + rng.Intn(20)),
			ForkMessages:   float64(rng.Intn(4)),
			MessageContext: float64(rng.Intn(5)),
			TargetMessages: float64(1 + rng.Intn(3)),
			UsesMessages:   float64(rng.Intn(3)),
			ContextPerLine: float64(rng.Intn(3)),
			PackageContext: float64(rng.Intn(10)),
		}
		params := batcher.CostParams{
			Budget:   budgets[rng.Intn(len(budgets))],
			MaxCards: maxCards[rng.Intn(len(maxCards))],
			Weights:  weights,
		}
		cards := make([]planparser.Card, 1+rng.Intn(6))
		for i := range cards {
			uses := []string{}
			if rng.Intn(2) == 0 {
				uses = append(uses, files[rng.Intn(len(files))])
			}
			cards[i] = editCard(i+1, []string{files[rng.Intn(len(files))]}, uses...)
		}

		wantBatches, wantLargest := bruteForceBest(t, plan, cards, params)
		batches, err := batcher.NewCost("cautious", params).Batch(plan, cards, costSizes)
		if err != nil {
			t.Fatalf("iteration %d: Batch: %v", iteration, err)
		}

		var gotLargest float64
		next := 0
		for _, batch := range batches {
			for _, card := range batch.Cards {
				if card.Number != cards[next].Number {
					t.Fatalf("iteration %d: card order broken at %d: got card %d", iteration, next, card.Number)
				}
				next++
			}
			peak := peakOf(t, plan, batch.Cards, weights)
			if batch.Estimate != peak || batch.Profile != "cautious" {
				t.Errorf("iteration %d: batch = {Profile %q, Estimate %v}; want {cautious, %v}", iteration, batch.Profile, batch.Estimate, peak)
			}
			if b := batch.Breakdown; b == nil || len(b.Cards) != len(batch.Cards) || b.Weights != weights {
				t.Errorf("iteration %d: breakdown %+v does not describe the batch's %d cards under its weights", iteration, b, len(batch.Cards))
			} else {
				sum := b.Startup + b.ReadUnion
				for _, cb := range b.Cards {
					sum += cb.Written
				}
				if sum != peak {
					t.Errorf("iteration %d: breakdown components sum to %v; want the peak %v", iteration, sum, peak)
				}
			}
			if len(batch.Cards) > 1 && !feasibleSegment(t, plan, batch.Cards, params) {
				t.Errorf("iteration %d: infeasible batch of %d cards", iteration, len(batch.Cards))
			}
			gotLargest = math.Max(gotLargest, peak)
		}
		if next != len(cards) {
			t.Fatalf("iteration %d: batches cover %d cards; want %d", iteration, next, len(cards))
		}
		if len(batches) != wantBatches || gotLargest != wantLargest {
			t.Errorf("iteration %d: %d batches, largest peak %v; want %d, %v", iteration, len(batches), gotLargest, wantBatches, wantLargest)
		}
	}
}

// peakOf returns the PeakContext of cards over costSizes.
func peakOf(t *testing.T, plan *planparser.Plan, cards []planparser.Card, w batcher.Weights) float64 {
	t.Helper()
	peak, err := batcher.PeakContext(plan, cards, costSizes, w)
	if err != nil {
		t.Fatalf("PeakContext: %v", err)
	}
	return peak
}

// feasibleSegment applies the multi-card hard limits to cards.
func feasibleSegment(t *testing.T, plan *planparser.Plan, cards []planparser.Card, params batcher.CostParams) bool {
	t.Helper()
	return len(cards) <= params.MaxCards && peakOf(t, plan, cards, params.Weights) <= params.Budget
}

// bruteForceBest enumerates every contiguous split of cards and returns the fewest batches over the feasible ones, with the smallest largest peak among the splits that reach it.
func bruteForceBest(t *testing.T, plan *planparser.Plan, cards []planparser.Card, params batcher.CostParams) (int, float64) {
	t.Helper()
	bestBatches, bestLargest := math.MaxInt, math.Inf(1)
	for mask := 0; mask < 1<<(len(cards)-1); mask++ {
		var largest float64
		batches, feasible := 0, true
		begin := 0
		for end := 1; end <= len(cards); end++ {
			if end < len(cards) && mask&(1<<(end-1)) == 0 {
				continue
			}
			segment := cards[begin:end]
			if len(segment) > 1 && !feasibleSegment(t, plan, segment, params) {
				feasible = false
				break
			}
			largest = math.Max(largest, peakOf(t, plan, segment, params.Weights))
			batches++
			begin = end
		}
		if !feasible {
			continue
		}
		if batches < bestBatches || (batches == bestBatches && largest < bestLargest) {
			bestBatches, bestLargest = batches, largest
		}
	}
	return bestBatches, bestLargest
}
