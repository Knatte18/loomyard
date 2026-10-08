// partition_test.go covers ExecutionBatches and RecordPartition: which batches each state shape resolves to, that a recorded partition outlives a changed size source, and the refusals for a recorded partition that no longer matches the plan or runs backward.
// Tier 1: package websterengine_test, no git, no disk — a fake size source and a size-driven fake batchifier stand in for the real ones.

package websterengine_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// linesSizes is a SizeSource reporting the same line count for every file.
type linesSizes int

func (l linesSizes) Lines(string) (int, bool, error)  { return int(l), true, nil }
func (linesSizes) TestFiles(string) ([]string, error) { return nil, nil }

// sizeBatcher is a grouping batchifier whose grouping depends on the size source: one batch holding every card while a file is small, one batch per card once it grows.
type sizeBatcher struct{}

func (sizeBatcher) Name() string { return "size" }

func (sizeBatcher) Batch(_ *planparser.Plan, cards []planparser.Card, sizes batcher.SizeSource, _ int, _ batcher.StartBase) ([]batcher.Batch, error) {
	lines, _, err := sizes.Lines("any.go")
	if err != nil {
		return nil, err
	}
	if lines < 100 {
		return []batcher.Batch{{Cards: cards, Profile: "size", Estimate: 7}}, nil
	}
	batches := make([]batcher.Batch, len(cards))
	for i, card := range cards {
		batches[i] = batcher.Batch{Cards: []planparser.Card{card}, Profile: "size", Estimate: 3}
	}
	return batches, nil
}

// partitionPlan is a three-card plan whose cards have no targets, so any partition of it runs in order.
func partitionPlan() *planparser.Plan {
	return &planparser.Plan{Dir: "plan", Cards: []planparser.Card{
		{Number: 1, Slug: "a"},
		{Number: 2, Slug: "b"},
		{Number: 3, Slug: "c"},
	}}
}

// batchIDs renders each batch as its cards' NN-<slug> ids joined by "+", so a grouping compares as one slice.
func batchIDs(batches []batcher.Batch) []string {
	out := make([]string, len(batches))
	for i, b := range batches {
		ids := make([]string, len(b.Cards))
		for j, c := range b.Cards {
			ids[j] = fmt.Sprintf("%02d-%s", c.Number, c.Slug)
		}
		out[i] = strings.Join(ids, "+")
	}
	return out
}

// beforeBatcher is a batchifier whose single batch records the before it was handed as its estimate.
type beforeBatcher struct{}

func (beforeBatcher) Name() string { return "before" }

func (beforeBatcher) Batch(_ *planparser.Plan, cards []planparser.Card, _ batcher.SizeSource, before int, _ batcher.StartBase) ([]batcher.Batch, error) {
	return []batcher.Batch{{Cards: cards, Profile: "before", Estimate: float64(before)}}, nil
}

// TestExecutionBatches asserts the batches each state shape resolves to: the active batchifier's grouping with no state, the recorded partition (profile and estimate kept) when the state holds one even after the size source changed, and the identity batchifier over a state with no partition whatever batchifier is active.
func TestExecutionBatches(t *testing.T) {
	t.Parallel()

	plan := partitionPlan()
	recorded := &websterengine.State{}
	grouped, err := sizeBatcher{}.Batch(plan, plan.Cards, linesSizes(10), 0, batcher.StartBase{})
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	websterengine.RecordPartition(recorded, grouped)

	tests := []struct {
		name  string
		state *websterengine.State
		// active is the batchifier; nil means sizeBatcher.
		active    batcher.Batcher
		sizes     batcher.SizeSource
		want      []string
		wantStats [][2]any
	}{
		{
			name:      "no state returns the active batchifier's grouping",
			sizes:     linesSizes(10),
			want:      []string{"01-a+02-b+03-c"},
			wantStats: [][2]any{{"size", 7.0}},
		},
		{
			name:      "no state follows the size source",
			sizes:     linesSizes(500),
			want:      []string{"01-a", "02-b", "03-c"},
			wantStats: [][2]any{{"size", 3.0}, {"size", 3.0}, {"size", 3.0}},
		},
		{
			name:      "a recorded partition is kept when the size source changes between calls",
			state:     recorded,
			sizes:     linesSizes(500),
			want:      []string{"01-a+02-b+03-c"},
			wantStats: [][2]any{{"size", 7.0}},
		},
		{
			name:      "a state without a partition batches with identity while a grouping batchifier is active",
			state:     &websterengine.State{},
			sizes:     linesSizes(10),
			want:      []string{"01-a", "02-b", "03-c"},
			wantStats: [][2]any{{"identity", 0.0}, {"identity", 0.0}, {"identity", 0.0}},
		},
		{
			name:      "no state tells the batchifier no batch runs ahead of the plan's cards",
			active:    beforeBatcher{},
			sizes:     linesSizes(10),
			want:      []string{"01-a+02-b+03-c"},
			wantStats: [][2]any{{"before", 0.0}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var active batcher.Batcher = sizeBatcher{}
			if tc.active != nil {
				active = tc.active
			}
			got, err := websterengine.ExecutionBatches(plan, tc.state, active, tc.sizes)
			if err != nil {
				t.Fatalf("ExecutionBatches() error = %v; want nil", err)
			}
			if !slices.Equal(batchIDs(got), tc.want) {
				t.Errorf("ExecutionBatches() grouping = %v; want %v", batchIDs(got), tc.want)
			}
			for i, b := range got {
				if b.Profile != tc.wantStats[i][0] || b.Estimate != tc.wantStats[i][1] {
					t.Errorf("batch %d profile and estimate = %q, %v; want %q, %v", i, b.Profile, b.Estimate, tc.wantStats[i][0], tc.wantStats[i][1])
				}
			}
		})
	}
}

// TestExecutionBatches_Refusals asserts a recorded partition that names a card the plan lacks, leaves a plan card out, or runs a card before the card it uses is refused with the card named and the way forward.
func TestExecutionBatches_Refusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plan      *planparser.Plan
		partition []websterengine.PartitionBatch
		wantErr   error
		want      []string
	}{
		{
			name:      "a recorded id the plan lacks",
			plan:      partitionPlan(),
			partition: []websterengine.PartitionBatch{{Cards: []string{"01-a", "02-b", "03-c", "04-gone"}}},
			wantErr:   websterengine.ErrPartitionMismatch,
			want:      []string{"recorded cards 04-gone are not cards of the plan", "`lyx webster rebaseline --card NN`", "`lyx webster restore-plan`"},
		},
		{
			name:      "a plan card outside the partition",
			plan:      partitionPlan(),
			partition: []websterengine.PartitionBatch{{Cards: []string{"01-a"}}, {Cards: []string{"02-b"}}},
			wantErr:   websterengine.ErrPartitionMismatch,
			want:      []string{"plan cards 03-c are in no recorded batch", "`lyx webster rebaseline --card NN`", "`lyx webster restore-plan`"},
		},
		{
			name: "a backward dependency",
			plan: &planparser.Plan{Dir: "plan", Cards: []planparser.Card{
				{Number: 1, Slug: "a", Uses: []string{"FooFunc"}},
				{Number: 2, Slug: "b", Targets: []string{"FooFunc"}},
			}},
			partition: []websterengine.PartitionBatch{{Cards: []string{"01-a"}}, {Cards: []string{"02-b"}}},
			wantErr:   websterengine.ErrBatchOrder,
			want:      []string{`batch 1 (card 1) uses "FooFunc", which batch 2 (card 2) targets`, "`lyx webster rebaseline --card NN`", "`lyx webster run --fresh`"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := websterengine.ExecutionBatches(tc.plan, &websterengine.State{Partition: tc.partition}, sizeBatcher{}, linesSizes(10))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ExecutionBatches() error = %v; want errors.Is(err, %v)", err, tc.wantErr)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ExecutionBatches() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}
