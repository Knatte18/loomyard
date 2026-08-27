// sequence_test.go covers SequenceBatches' four internal stages end to end through its exported
// surface: edge derivation (producer-before-consumer on Targets/Uses matches, lower-card-number-wins
// on shared Targets, no edge from a shared Uses), SCC condensation and its Kahn ordering (acyclic
// topological order, the no-op property on an already dependency-correct plan, cycle condensation),
// the structural guarantees (length-preservation, no mutation, determinism across repeated and
// shuffled calls), and Cycle.Warning's rendering.
// Tier 1: package websterengine_test, no git, no disk — every fixture is a hand-built []batcher.Batch
// literal; nothing goes through planparser.ParsePlan.

package websterengine_test

import (
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
// determinism tests use in place of a random source.
func reversed(batches []batcher.Batch) []batcher.Batch {
	out := make([]batcher.Batch, len(batches))
	for i, b := range batches {
		out[len(batches)-1-i] = b
	}
	return out
}

func TestSequenceBatches_EdgeDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []batcher.Batch
		want []int
		nCyc int
	}{
		{
			name: "symbol-shaped Uses/Targets match orders producer before consumer",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"FooFunc"}),
				oneCardBatch(2, "producer", []string{"FooFunc"}, nil),
			},
			want: []int{2, 1},
			nCyc: 0,
		},
		{
			name: "path-shaped Uses/Targets match also orders producer before consumer",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"internal/foo/foo.go"}),
				oneCardBatch(2, "producer", []string{"internal/foo/foo.go"}, nil),
			},
			want: []int{2, 1},
			nCyc: 0,
		},
		{
			name: "two cards sharing a Targets entry orders lower number before higher",
			in: []batcher.Batch{
				oneCardBatch(5, "later-writer", []string{"shared.Ref"}, nil),
				oneCardBatch(2, "earlier-writer", []string{"shared.Ref"}, nil),
			},
			want: []int{2, 5},
			nCyc: 0,
		},
		{
			// Unconstrained batches — no edges among them at all — sequence purely by their own
			// (number, index) sort key, which is what makes the no-op property hold whenever a
			// plan's declared order already matches ascending batch number, as a real plan's does.
			// If a shared Uses/Uses match wrongly produced an edge here, condensation would fold
			// these two batches into a cycle (nCyc > 0); it does not.
			name: "two cards sharing only a Uses entry produce no ordering constraint",
			in: []batcher.Batch{
				oneCardBatch(1, "first", nil, []string{"shared.Ref"}),
				oneCardBatch(2, "second", nil, []string{"shared.Ref"}),
			},
			want: []int{1, 2},
			nCyc: 0,
		},
		{
			name: "a card matching nothing else keeps its declared position",
			in: []batcher.Batch{
				oneCardBatch(1, "first", []string{"A"}, nil),
				oneCardBatch(2, "second", []string{"B"}, nil),
				oneCardBatch(3, "third", []string{"C"}, nil),
			},
			want: []int{1, 2, 3},
			nCyc: 0,
		},
		{
			name: "a ref in one card's own Targets and Uses produces no self-edge or cycle",
			in: []batcher.Batch{
				oneCardBatch(1, "self-referential", []string{"shared.Ref"}, []string{"shared.Ref"}),
				oneCardBatch(2, "other", []string{"other.Ref"}, nil),
			},
			want: []int{1, 2},
			nCyc: 0,
		},
		{
			name: "a Rename group's Pairs endpoints, already projected into Targets, order as targets",
			in: []batcher.Batch{
				oneCardBatch(1, "consumer", nil, []string{"pkg.New"}),
				oneCardBatch(2, "rename", []string{"pkg.Old", "pkg.New"}, nil),
			},
			want: []int{2, 1},
			nCyc: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, cycles := websterengine.SequenceBatches(tt.in)

			if diff := diffOrder(t, got, tt.want); diff {
				return
			}
			if len(cycles) != tt.nCyc {
				t.Errorf("SequenceBatches() cycles = %d; want %d", len(cycles), tt.nCyc)
			}
		})
	}
}

func TestSequenceBatches_MultiCardBatch(t *testing.T) {
	t.Parallel()

	// Batch 10 holds two cards: card 10 targets X, card 11 uses X. Batch 20 holds a single card
	// that uses Y, which nothing targets. The card-level edge between card 10 and card 11 is
	// INSIDE one batch, so it must not become a self-loop or a spurious cycle in the batch-level
	// output, and batch 20 (no cross-batch edges) keeps its declared position.
	multi := batcher.Batch{
		Cards: []planparser.Card{
			{Number: 10, Slug: "producer-in-batch", Targets: []string{"X"}},
			{Number: 11, Slug: "consumer-in-batch", Uses: []string{"X"}},
		},
	}
	solo := oneCardBatch(20, "unrelated", nil, []string{"Y"})

	in := []batcher.Batch{multi, solo}
	got, cycles := websterengine.SequenceBatches(in)

	if len(got) != 2 {
		t.Fatalf("SequenceBatches() len = %d; want 2", len(got))
	}
	if len(got[0].Cards) != 2 {
		t.Errorf("SequenceBatches()[0] card count = %d; want 2 (the multi-card batch, kept intact)", len(got[0].Cards))
	}
	if len(cycles) != 0 {
		t.Errorf("SequenceBatches() cycles = %d; want 0 (an intra-batch card edge is not a cycle)", len(cycles))
	}
}

func TestSequenceBatches_AcyclicOrdering(t *testing.T) {
	t.Parallel()

	in := []batcher.Batch{
		oneCardBatch(1, "c", nil, []string{"b.Out"}),
		oneCardBatch(2, "b", []string{"b.Out"}, []string{"a.Out"}),
		oneCardBatch(3, "a", []string{"a.Out"}, nil),
	}
	want := []int{3, 2, 1}

	got, cycles := websterengine.SequenceBatches(in)
	diffOrder(t, got, want)
	if len(cycles) != 0 {
		t.Errorf("SequenceBatches() cycles = %d; want 0", len(cycles))
	}
}

func TestSequenceBatches_NoOpOnDeclaredCorrectOrder(t *testing.T) {
	t.Parallel()

	// Already dependency-correct: 1 targets what nothing needs, 2 uses 1's target, 3 uses 2's
	// target. Declared order already satisfies every derived edge, so the no-op property applies.
	in := []batcher.Batch{
		oneCardBatch(1, "first", []string{"one.Out"}, nil),
		oneCardBatch(2, "second", []string{"two.Out"}, []string{"one.Out"}),
		oneCardBatch(3, "third", nil, []string{"two.Out"}),
	}
	want := []int{1, 2, 3}

	got, cycles := websterengine.SequenceBatches(in)
	diffOrder(t, got, want)
	if len(cycles) != 0 {
		t.Errorf("SequenceBatches() cycles = %d; want 0", len(cycles))
	}
}

func TestSequenceBatches_ConsumerDeclaredBeforeProducer(t *testing.T) {
	t.Parallel()

	// 1 uses what 3 produces, declared before it; 2 has no relation to either. The producer (3)
	// must move ahead of its consumer (1). Batch 2 carries no edges at all, so it is ready from the
	// start and — per the min-heap's own (number, index) key — is popped as soon as its number
	// makes it the lowest-keyed ready component, ahead of batch 3, whose own readiness is
	// immediate too but whose number is higher.
	in := []batcher.Batch{
		oneCardBatch(1, "consumer", nil, []string{"three.Out"}),
		oneCardBatch(2, "unrelated", []string{"two.Out"}, nil),
		oneCardBatch(3, "producer", []string{"three.Out"}, nil),
	}
	want := []int{2, 3, 1}

	got, cycles := websterengine.SequenceBatches(in)
	diffOrder(t, got, want)
	if len(cycles) != 0 {
		t.Errorf("SequenceBatches() cycles = %d; want 0", len(cycles))
	}
}

func TestSequenceBatches_TwoCardCycle(t *testing.T) {
	t.Parallel()

	// 1 uses what 2 produces, and 2 uses what 1 produces: a two-batch cycle. Batch 3 has no
	// relation to either and stays correctly ordered around the condensed component.
	in := []batcher.Batch{
		oneCardBatch(1, "one", []string{"one.Out"}, []string{"two.Out"}),
		oneCardBatch(2, "two", []string{"two.Out"}, []string{"one.Out"}),
		oneCardBatch(3, "unrelated", nil, nil),
	}

	got, cycles := websterengine.SequenceBatches(in)

	if len(got) != 3 {
		t.Fatalf("SequenceBatches() len = %d; want 3", len(got))
	}
	// The cycle's members run in declared order (1 then 2); batch 3 is unconstrained and keeps
	// its own declared position relative to the condensed group, which sorts by its lowest
	// member's key (batch 1) — so the emitted order is 1, 2, 3.
	diffOrder(t, got, []int{1, 2, 3})

	if len(cycles) != 1 {
		t.Fatalf("SequenceBatches() cycles = %d; want 1", len(cycles))
	}
	wantMembers := []int{1, 2}
	if diff := diffIntSlice(cycles[0].Batches, wantMembers); diff {
		t.Errorf("cycles[0].Batches = %v; want %v", cycles[0].Batches, wantMembers)
	}
}

func TestSequenceBatches_ThreeCardCycle(t *testing.T) {
	t.Parallel()

	in := []batcher.Batch{
		oneCardBatch(1, "one", []string{"one.Out"}, []string{"three.Out"}),
		oneCardBatch(2, "two", []string{"two.Out"}, []string{"one.Out"}),
		oneCardBatch(3, "three", []string{"three.Out"}, []string{"two.Out"}),
	}

	got, cycles := websterengine.SequenceBatches(in)

	if len(got) != 3 {
		t.Fatalf("SequenceBatches() len = %d; want 3", len(got))
	}
	if len(cycles) != 1 {
		t.Fatalf("SequenceBatches() cycles = %d; want 1", len(cycles))
	}
	wantMembers := []int{1, 2, 3}
	if diff := diffIntSlice(cycles[0].Batches, wantMembers); diff {
		t.Errorf("cycles[0].Batches = %v; want %v", cycles[0].Batches, wantMembers)
	}
}

func TestSequenceBatches_TwoDisjointCycles(t *testing.T) {
	t.Parallel()

	in := []batcher.Batch{
		oneCardBatch(1, "one", []string{"one.Out"}, []string{"two.Out"}),
		oneCardBatch(2, "two", []string{"two.Out"}, []string{"one.Out"}),
		oneCardBatch(3, "three", []string{"three.Out"}, []string{"four.Out"}),
		oneCardBatch(4, "four", []string{"four.Out"}, []string{"three.Out"}),
	}

	got, cycles := websterengine.SequenceBatches(in)

	if len(got) != 4 {
		t.Fatalf("SequenceBatches() len = %d; want 4", len(got))
	}
	if len(cycles) != 2 {
		t.Fatalf("SequenceBatches() cycles = %d; want 2", len(cycles))
	}
	if diff := diffIntSlice(cycles[0].Batches, []int{1, 2}); diff {
		t.Errorf("cycles[0].Batches = %v; want [1 2]", cycles[0].Batches)
	}
	if diff := diffIntSlice(cycles[1].Batches, []int{3, 4}); diff {
		t.Errorf("cycles[1].Batches = %v; want [3 4]", cycles[1].Batches)
	}
}

func TestSequenceBatches_AcyclicPlanReportsNoCycles(t *testing.T) {
	t.Parallel()

	in := []batcher.Batch{
		oneCardBatch(1, "a", []string{"a.Out"}, nil),
		oneCardBatch(2, "b", nil, []string{"a.Out"}),
	}
	_, cycles := websterengine.SequenceBatches(in)
	if len(cycles) != 0 {
		t.Errorf("SequenceBatches() cycles = %d; want 0", len(cycles))
	}
}

func TestSequenceBatches_StructuralGuarantees(t *testing.T) {
	t.Parallel()

	fixtures := [][]batcher.Batch{
		{
			oneCardBatch(1, "c", nil, []string{"b.Out"}),
			oneCardBatch(2, "b", []string{"b.Out"}, []string{"a.Out"}),
			oneCardBatch(3, "a", []string{"a.Out"}, nil),
		},
		{
			oneCardBatch(1, "one", []string{"one.Out"}, []string{"two.Out"}),
			oneCardBatch(2, "two", []string{"two.Out"}, []string{"one.Out"}),
			oneCardBatch(3, "unrelated", nil, nil),
		},
		{
			oneCardBatch(5, "x", nil, nil),
		},
	}

	for i, in := range fixtures {
		before := batchNumbers(in)

		got, _ := websterengine.SequenceBatches(in)
		if len(got) != len(in) {
			t.Errorf("fixture %d: len(out) = %d; want %d", i, len(got), len(in))
		}
		if diff := diffMultiset(batchNumbers(got), before); diff {
			t.Errorf("fixture %d: output multiset %v; want %v", i, batchNumbers(got), before)
		}

		got2, _ := websterengine.SequenceBatches(in)
		if diff := diffIntSlice(batchNumbers(got), batchNumbers(got2)); diff {
			t.Errorf("fixture %d: SequenceBatches() called twice gave different orders: %v vs %v", i, batchNumbers(got), batchNumbers(got2))
		}

		shuffled := reversed(in)
		gotShuffled, _ := websterengine.SequenceBatches(shuffled)
		if diff := diffIntSlice(batchNumbers(got), batchNumbers(gotShuffled)); diff {
			t.Errorf("fixture %d: shuffled input gave different order: %v vs %v", i, batchNumbers(got), batchNumbers(gotShuffled))
		}

		after := batchNumbers(in)
		if diff := diffIntSlice(before, after); diff {
			t.Errorf("fixture %d: input mutated: before %v, after %v", i, before, after)
		}
	}
}

func TestSequenceBatches_NilAndEmptyInput(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		got, cycles := websterengine.SequenceBatches(nil)
		if len(got) != 0 {
			t.Errorf("SequenceBatches(nil) len = %d; want 0", len(got))
		}
		if len(cycles) != 0 {
			t.Errorf("SequenceBatches(nil) cycles = %d; want 0", len(cycles))
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		got, cycles := websterengine.SequenceBatches([]batcher.Batch{})
		if len(got) != 0 {
			t.Errorf("SequenceBatches([]) len = %d; want 0", len(got))
		}
		if len(cycles) != 0 {
			t.Errorf("SequenceBatches([]) cycles = %d; want 0", len(cycles))
		}
	})
}

func TestCycle_Warning(t *testing.T) {
	t.Parallel()

	c := websterengine.Cycle{Batches: []int{2, 5, 9}}
	msg := c.Warning()
	for _, want := range []string{"2", "5", "9"} {
		if !containsSubstring(msg, want) {
			t.Errorf("Cycle.Warning() = %q; want it to name member batch %q", msg, want)
		}
	}
}

func TestCycle_WarningZeroValueAndEmptyBatchesDoNotPanic(t *testing.T) {
	t.Parallel()

	t.Run("zero value", func(t *testing.T) {
		t.Parallel()
		var c websterengine.Cycle
		_ = c.Warning()
	})

	t.Run("empty Batches", func(t *testing.T) {
		t.Parallel()
		c := websterengine.Cycle{Batches: []int{}}
		_ = c.Warning()
	})
}

// diffOrder asserts got's batch numbers, in order, equal want, reporting a t.Error (not Fatal) and
// returning whether a mismatch was reported.
func diffOrder(t *testing.T, got []batcher.Batch, want []int) bool {
	t.Helper()
	gotNumbers := batchNumbers(got)
	if diffIntSlice(gotNumbers, want) {
		t.Errorf("SequenceBatches() order = %v; want %v", gotNumbers, want)
		return true
	}
	return false
}

// diffIntSlice reports whether a and b differ in length or in any element at the same position.
func diffIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// diffMultiset reports whether a and b hold the same elements irrespective of order.
func diffMultiset(a, b []int) bool {
	if len(a) != len(b) {
		return true
	}
	counts := map[int]int{}
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
	}
	for _, c := range counts {
		if c != 0 {
			return true
		}
	}
	return false
}

// containsSubstring reports whether s contains sub — a tiny local helper so this file needs no
// "strings" import for a single call site.
func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
