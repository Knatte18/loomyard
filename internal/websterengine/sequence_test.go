// sequence_test.go covers SequenceBatches' four internal stages end to end through its exported
// surface: edge derivation (producer-before-consumer on Targets/Uses matches, lower-card-number-wins
// on shared Targets, no edge from a shared Uses), SCC condensation and its Kahn ordering (acyclic
// topological order, the no-op property on an already dependency-correct plan, cycle condensation),
// the structural guarantees (no mutation, determinism across repeated and reversed calls), and
// Cycle.Warning naming every member.
// Tier 1: package websterengine_test, no git, no disk — every fixture is a hand-built []batcher.Batch
// literal; nothing goes through planparser.ParsePlan.

package websterengine_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// oneCardBatch builds a single-card batcher.Batch from a number, slug, Targets list, and Uses list,
// so the table rows below stay readable.
func oneCardBatch(number int, slug string, targets, uses []string) batcher.Batch {
	return batcher.Batch{
		Cards: []planparser.Card{
			{
				Number:  number,
				Slug:    slug,
				Targets: targets,
				Uses:    uses,
			},
		},
	}
}

// batchNumbers extracts each batch's own first-card number, in slice order — the observable
// identity SequenceBatches' output is asserted against.
func batchNumbers(batches []batcher.Batch) []int {
	out := make([]int, len(batches))
	for i, b := range batches {
		out[i] = b.Cards[0].Number
	}
	return out
}

// reversed returns a new slice holding batches in reverse order — the deterministic "shuffle" the
// determinism check uses in place of a random source.
func reversed(batches []batcher.Batch) []batcher.Batch {
	out := make([]batcher.Batch, len(batches))
	for i, b := range batches {
		out[len(batches)-1-i] = b
	}
	return out
}

// TestSequenceBatches asserts the order SequenceBatches emits and the cycles it reports for each
// shape of Targets/Uses relation: producers ahead of consumers, lower card number first on a shared
// Target, no constraint from a shared Uses, intra-batch edges kept inside their batch, and each
// strongly connected component condensed into one reported cycle.
func TestSequenceBatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []batcher.Batch
		want []int
		// wantCycles lists each reported cycle's member batch numbers, in report order.
		wantCycles [][]int
		// wantCardCounts, when set, is each output batch's card count, in output order.
		wantCardCounts []int
	}{
		{
			name: "symbol-shaped Uses/Targets match orders producer before consumer",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"FooFunc"}),
				oneCardBatch(2, "producer", []string{"FooFunc"}, nil),
			},
			want: []int{2, 1},
		},
		{
			name: "path-shaped Uses/Targets match also orders producer before consumer",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"internal/foo/foo.go"}),
				oneCardBatch(2, "producer", []string{"internal/foo/foo.go"}, nil),
			},
			want: []int{2, 1},
		},
		{
			name: "two cards sharing a Targets entry orders lower number before higher",
			in: []batcher.Batch{
				oneCardBatch(5, "later-writer", []string{"shared.Ref"}, nil),
				oneCardBatch(2, "earlier-writer", []string{"shared.Ref"}, nil),
			},
			want: []int{2, 5},
		},
		{
			// Unconstrained batches — no edges among them at all — sequence purely by their own
			// (number, index) sort key, which is what makes the no-op property hold whenever a
			// plan's declared order already matches ascending batch number, as a real plan's does.
			// If a shared Uses/Uses match wrongly produced an edge here, condensation would fold
			// these two batches into a cycle; it does not.
			name: "two cards sharing only a Uses entry produce no ordering constraint",
			in: []batcher.Batch{
				oneCardBatch(1, "first", nil, []string{"shared.Ref"}),
				oneCardBatch(2, "second", nil, []string{"shared.Ref"}),
			},
			want: []int{1, 2},
		},
		{
			name: "a card matching nothing else keeps its declared position",
			in: []batcher.Batch{
				oneCardBatch(1, "first", []string{"A"}, nil),
				oneCardBatch(2, "second", []string{"B"}, nil),
				oneCardBatch(3, "third", []string{"C"}, nil),
			},
			want: []int{1, 2, 3},
		},
		{
			name: "a ref in one card's own Targets and Uses produces no self-edge or cycle",
			in: []batcher.Batch{
				oneCardBatch(1, "self-referential", []string{"shared.Ref"}, []string{"shared.Ref"}),
				oneCardBatch(2, "other", []string{"other.Ref"}, nil),
			},
			want: []int{1, 2},
		},
		{
			name: "a Rename group's Pairs endpoints, already projected into Targets, order as targets",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"pkg.New"}),
				oneCardBatch(2, "rename", []string{"pkg.Old", "pkg.New"}, nil),
			},
			want: []int{2, 1},
		},
		{
			// Batch 10 holds two cards: card 10 targets X, card 11 uses X. Batch 20 holds a single
			// card that uses Y, which nothing targets. The card-level edge between card 10 and
			// card 11 is INSIDE one batch, so it must not become a self-loop or a spurious cycle in
			// the batch-level output, and batch 20 (no cross-batch edges) keeps its declared position.
			name: "an intra-batch card edge keeps the multi-card batch intact and is not a cycle",
			in: []batcher.Batch{
				{Cards: []planparser.Card{
					{Number: 10, Slug: "producer-in-batch", Targets: []string{"X"}},
					{Number: 11, Slug: "consumer-in-batch", Uses: []string{"X"}},
				}},
				oneCardBatch(20, "unrelated", nil, []string{"Y"}),
			},
			want:           []int{10, 20},
			wantCardCounts: []int{2, 1},
		},
		{
			name: "a producer chain declared in reverse sorts to dependency order",
			in: []batcher.Batch{
				oneCardBatch(1, "c", nil, []string{"b.Out"}),
				oneCardBatch(2, "b", []string{"b.Out"}, []string{"a.Out"}),
				oneCardBatch(3, "a", []string{"a.Out"}, nil),
			},
			want: []int{3, 2, 1},
		},
		{
			// Already dependency-correct: 1 targets what nothing needs, 2 uses 1's target, 3 uses
			// 2's target. Declared order already satisfies every derived edge, so the no-op
			// property applies.
			name: "an already dependency-correct plan keeps its declared order",
			in: []batcher.Batch{
				oneCardBatch(1, "first", []string{"one.Out"}, nil),
				oneCardBatch(2, "second", []string{"two.Out"}, []string{"one.Out"}),
				oneCardBatch(3, "third", nil, []string{"two.Out"}),
			},
			want: []int{1, 2, 3},
		},
		{
			// 1 uses what 3 produces, declared before it; 2 has no relation to either. The producer
			// (3) must move ahead of its consumer (1). Batch 2 carries no edges at all, so it is
			// ready from the start and — per the min-heap's own (number, index) key — is popped as
			// soon as its number makes it the lowest-keyed ready component, ahead of batch 3, whose
			// own readiness is immediate too but whose number is higher.
			name: "a consumer declared before its producer moves behind it",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"three.Out"}),
				oneCardBatch(2, "unrelated", []string{"two.Out"}, nil),
				oneCardBatch(3, "producer", []string{"three.Out"}, nil),
			},
			want: []int{2, 3, 1},
		},
		{
			// 1 uses what 2 produces, and 2 uses what 1 produces: a two-batch cycle. Batch 3 has no
			// relation to either and keeps its declared position relative to the condensed group,
			// which sorts by its lowest member's key (batch 1), so the emitted order is 1, 2, 3.
			name: "a two-batch cycle is condensed and reported",
			in: []batcher.Batch{
				oneCardBatch(1, "one", []string{"one.Out"}, []string{"two.Out"}),
				oneCardBatch(2, "two", []string{"two.Out"}, []string{"one.Out"}),
				oneCardBatch(3, "unrelated", nil, nil),
			},
			want:       []int{1, 2, 3},
			wantCycles: [][]int{{1, 2}},
		},
		{
			name: "a three-batch cycle is condensed and reported",
			in: []batcher.Batch{
				oneCardBatch(1, "one", []string{"one.Out"}, []string{"three.Out"}),
				oneCardBatch(2, "two", []string{"two.Out"}, []string{"one.Out"}),
				oneCardBatch(3, "three", []string{"three.Out"}, []string{"two.Out"}),
			},
			want:       []int{1, 2, 3},
			wantCycles: [][]int{{1, 2, 3}},
		},
		{
			name: "two disjoint cycles are reported separately",
			in: []batcher.Batch{
				oneCardBatch(1, "one", []string{"one.Out"}, []string{"two.Out"}),
				oneCardBatch(2, "two", []string{"two.Out"}, []string{"one.Out"}),
				oneCardBatch(3, "three", []string{"three.Out"}, []string{"four.Out"}),
				oneCardBatch(4, "four", []string{"four.Out"}, []string{"three.Out"}),
			},
			want:       []int{1, 2, 3, 4},
			wantCycles: [][]int{{1, 2}, {3, 4}},
		},
		{
			name: "nil input yields nothing",
			in:   nil,
			want: []int{},
		},
		{
			name: "empty input yields nothing",
			in:   []batcher.Batch{},
			want: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := batchNumbers(tt.in)
			got, cycles := websterengine.SequenceBatches(tt.in)

			if gotNumbers := batchNumbers(got); !equalInts(gotNumbers, tt.want) {
				t.Errorf("SequenceBatches() order = %v; want %v", gotNumbers, tt.want)
			}
			if after := batchNumbers(tt.in); !equalInts(before, after) {
				t.Errorf("input mutated: before %v, after %v", before, after)
			}
			// The order is deterministic: a repeated call and a reversed input give the same one.
			again, _ := websterengine.SequenceBatches(tt.in)
			fromReversed, _ := websterengine.SequenceBatches(reversed(tt.in))
			for _, other := range [][]batcher.Batch{again, fromReversed} {
				if !equalInts(batchNumbers(other), batchNumbers(got)) {
					t.Errorf("SequenceBatches() order = %v on a repeated or reversed call; want %v", batchNumbers(other), batchNumbers(got))
				}
			}
			if len(cycles) != len(tt.wantCycles) {
				t.Fatalf("SequenceBatches() cycles = %d; want %d", len(cycles), len(tt.wantCycles))
			}
			for i, members := range tt.wantCycles {
				if !equalInts(cycles[i].Batches, members) {
					t.Errorf("cycles[%d].Batches = %v; want %v", i, cycles[i].Batches, members)
				}
				warning := cycles[i].Warning()
				for _, member := range members {
					if !strings.Contains(warning, strconv.Itoa(member)) {
						t.Errorf("cycles[%d].Warning() = %q; want it to name member batch %d", i, warning, member)
					}
				}
			}
			if tt.wantCardCounts != nil {
				counts := make([]int, len(got))
				for i, batch := range got {
					counts[i] = len(batch.Cards)
				}
				if !equalInts(counts, tt.wantCardCounts) {
					t.Errorf("SequenceBatches() card counts = %v; want %v", counts, tt.wantCardCounts)
				}
			}
		})
	}
}

// equalInts reports whether a and b have the same length and the same element at every position.
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
