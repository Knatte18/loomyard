// cost_test.go verifies the cost-model batchifier's hard limits, tie rule and optimality against a
// brute-force search over every contiguous split.
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

// costWeights make a one-card edit of a 20-line file cost 40 and merging cards on that file cheaper.
var costWeights = batcher.Weights{StartupContext: 10, ForkMessages: 1, TargetMessages: 1, ContextPerLine: 1}

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

// TestCostBatchifier_Limits asserts the grouping and per-batch estimate the cost batchifier returns for each hard limit and the tie rule.
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
			name:      "two small cards editing one file are grouped",
			params:    batcher.CostParams{AloneAbove: unlimited, Budget: unlimited, MaxCards: 5, Weights: costWeights},
			cards:     sameFile(2),
			wantSizes: []int{2},
		},
		{
			name:   "two small cards in unrelated packages are not grouped",
			params: batcher.CostParams{AloneAbove: unlimited, Budget: unlimited, MaxCards: 5, Weights: costWeights},
			cards: []planparser.Card{
				editCard(1, []string{"internal/a/a.go"}),
				editCard(2, []string{"internal/b/b.go"}),
			},
			wantSizes: []int{1, 1},
		},
		{
			name:      "cards above AloneAbove run alone",
			params:    batcher.CostParams{AloneAbove: 35, Budget: unlimited, MaxCards: 5, Weights: costWeights},
			cards:     sameFile(3),
			wantSizes: []int{1, 1, 1},
		},
		{
			name:      "the same cards group when AloneAbove allows",
			params:    batcher.CostParams{AloneAbove: unlimited, Budget: unlimited, MaxCards: 5, Weights: costWeights},
			cards:     sameFile(3),
			wantSizes: []int{3},
		},
		{
			name:         "a card over Budget but under AloneAbove runs alone and the split is still returned",
			params:       batcher.CostParams{AloneAbove: unlimited, Budget: 30, MaxCards: 5, Weights: costWeights},
			cards:        sameFile(3),
			wantSizes:    []int{1, 1, 1},
			wantEstimate: []float64{40, 40, 40},
		},
		{
			name:      "no batch holds more than MaxCards",
			params:    batcher.CostParams{AloneAbove: unlimited, Budget: unlimited, MaxCards: 2, Weights: costWeights},
			cards:     sameFile(4),
			wantSizes: []int{2, 2},
		},
		{
			name:      "equal-cost splits go to the one with more batches",
			params:    batcher.CostParams{AloneAbove: unlimited, Budget: unlimited, MaxCards: 3, Weights: batcher.Weights{}},
			cards:     sameFile(3),
			wantSizes: []int{1, 1, 1},
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

// TestCostBatchifier_OptimalAndFeasible checks, over a seeded input set, that the split keeps card
// order, respects every hard limit, reports each batch's SegmentCost, and has the least total cost
// of any feasible contiguous split, ties going to more batches.
func TestCostBatchifier_OptimalAndFeasible(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "go"}
	files := []string{"internal/a/a.go", "internal/b/b.go", "internal/c/c.go", "internal/d/d.go", "internal/big/b.go"}
	rng := rand.New(rand.NewSource(1))
	aloneAbove := []float64{30, 60, 1e9}
	budgets := []float64{50, 100, 200, 1e9}
	maxCards := []int{1, 2, 3, 6}

	for iteration := 0; iteration < 200; iteration++ {
		weights := batcher.Weights{
			StartupContext: float64(1 + rng.Intn(20)),
			ForkMessages:   float64(rng.Intn(4)),
			TargetMessages: float64(1 + rng.Intn(3)),
			UsesMessages:   float64(rng.Intn(3)),
			ContextPerLine: float64(rng.Intn(3)),
			PackageContext: float64(rng.Intn(10)),
		}
		params := batcher.CostParams{
			AloneAbove: aloneAbove[rng.Intn(len(aloneAbove))],
			Budget:     budgets[rng.Intn(len(budgets))],
			MaxCards:   maxCards[rng.Intn(len(maxCards))],
			Weights:    weights,
		}
		cards := make([]planparser.Card, 1+rng.Intn(6))
		for i := range cards {
			uses := []string{}
			if rng.Intn(2) == 0 {
				uses = append(uses, files[rng.Intn(len(files))])
			}
			cards[i] = editCard(i+1, []string{files[rng.Intn(len(files))]}, uses...)
		}

		wantTotal, wantBatches := bruteForceBest(t, plan, cards, params)
		batches, err := batcher.NewCost("cautious", params).Batch(plan, cards, costSizes)
		if err != nil {
			t.Fatalf("iteration %d: Batch: %v", iteration, err)
		}

		var gotTotal float64
		next := 0
		for _, batch := range batches {
			for _, card := range batch.Cards {
				if card.Number != cards[next].Number {
					t.Fatalf("iteration %d: card order broken at %d: got card %d", iteration, next, card.Number)
				}
				next++
			}
			estimate, err := batcher.SegmentCost(plan, batch.Cards, costSizes, weights)
			if err != nil {
				t.Fatalf("iteration %d: SegmentCost: %v", iteration, err)
			}
			if batch.Estimate != estimate || batch.Profile != "cautious" {
				t.Errorf("iteration %d: batch = {Profile %q, Estimate %v}; want {cautious, %v}", iteration, batch.Profile, batch.Estimate, estimate)
			}
			if len(batch.Cards) > 1 && !feasibleSegment(t, plan, batch.Cards, params) {
				t.Errorf("iteration %d: infeasible batch of %d cards", iteration, len(batch.Cards))
			}
			gotTotal += estimate
		}
		if next != len(cards) {
			t.Fatalf("iteration %d: batches cover %d cards; want %d", iteration, next, len(cards))
		}
		if gotTotal != wantTotal || len(batches) != wantBatches {
			t.Errorf("iteration %d: total %v in %d batches; want %v in %d", iteration, gotTotal, len(batches), wantTotal, wantBatches)
		}
	}
}

// feasibleSegment applies the multi-card hard limits to cards.
func feasibleSegment(t *testing.T, plan *planparser.Plan, cards []planparser.Card, params batcher.CostParams) bool {
	t.Helper()
	if len(cards) > params.MaxCards {
		return false
	}
	for _, card := range cards {
		own, err := batcher.SegmentCost(plan, []planparser.Card{card}, costSizes, params.Weights)
		if err != nil {
			t.Fatalf("SegmentCost: %v", err)
		}
		if own > params.AloneAbove {
			return false
		}
	}
	cost, err := batcher.SegmentCost(plan, cards, costSizes, params.Weights)
	if err != nil {
		t.Fatalf("SegmentCost: %v", err)
	}
	return cost <= params.Budget
}

// bruteForceBest enumerates every contiguous split of cards and returns the least total cost over
// the feasible ones, with the most batches among the splits that reach it.
func bruteForceBest(t *testing.T, plan *planparser.Plan, cards []planparser.Card, params batcher.CostParams) (float64, int) {
	t.Helper()
	bestTotal, bestBatches := math.Inf(1), 0
	for mask := 0; mask < 1<<(len(cards)-1); mask++ {
		var total float64
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
			cost, err := batcher.SegmentCost(plan, segment, costSizes, params.Weights)
			if err != nil {
				t.Fatalf("SegmentCost: %v", err)
			}
			total += cost
			batches++
			begin = end
		}
		if !feasible {
			continue
		}
		if total < bestTotal || (total == bestTotal && batches > bestBatches) {
			bestTotal, bestBatches = total, batches
		}
	}
	return bestTotal, bestBatches
}
