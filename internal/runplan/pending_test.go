// pending_test.go covers the partition of a run's remaining cards.
// Tier 0: a fake batcher, no files.

package runplan

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// onePerCardBatcher forms one batch per card and records the batch count it was told ran before its cards.
type onePerCardBatcher struct {
	before *int
}

func (o onePerCardBatcher) Batch(_ *planparser.Plan, cards []planparser.Card, _ batcher.SizeSource, before int, _ batcher.StartBase) ([]batcher.Batch, error) {
	*o.before = before
	batches := make([]batcher.Batch, 0, len(cards))
	for _, c := range cards {
		batches = append(batches, batcher.Batch{Cards: []planparser.Card{c}, Profile: "fake"})
	}
	return batches, nil
}

func (onePerCardBatcher) Name() string { return "fake" }

func TestPendingPartition(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	plan := &planparser.Plan{Dir: "plan", Cards: []planparser.Card{
		{Number: 1, Slug: "one"}, {Number: 2, Slug: "two"}, {Number: 3, Slug: "three"},
	}}
	doneState := func(ids ...string) *State {
		st := &State{Batches: map[int]*BatchState{}}
		for i, id := range ids {
			st.Batches[i+1] = &BatchState{Cards: []string{id}, Terminal: true, Status: DigestStatusDone}
			st.Partition = append(st.Partition, PartitionBatch{Cards: []string{id}})
		}
		return st
	}
	slugsOf := func(batches []batcher.Batch) []string {
		var slugs []string
		for _, b := range batches {
			for _, c := range b.Cards {
				slugs = append(slugs, c.Slug)
			}
		}
		return slugs
	}

	tests := []struct {
		name       string
		st         *State
		wantDone   int
		wantFresh  []string
		wantBefore int
	}{
		{"fresh run partitions every card", &State{}, 0, []string{"one", "two", "three"}, 0},
		{"done batch is dropped by slug and kept", doneState("01-one"), 1, []string{"two", "three"}, 1},
		{"done batch is matched after the card was renumbered", doneState("07-two"), 1, []string{"one", "three"}, 1},
		{"every card done returns no fresh batch", doneState("01-one", "02-two", "03-three"), 3, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := -1
			done, fresh, err := PendingPartition(plan, tt.st, onePerCardBatcher{&before}, nil)
			if err != nil {
				t.Fatalf("PendingPartition() error = %v", err)
			}
			if len(done) != tt.wantDone {
				t.Errorf("PendingPartition() kept %d done batches; want %d", len(done), tt.wantDone)
			}
			if got := slugsOf(fresh); !slices.Equal(got, tt.wantFresh) {
				t.Errorf("PendingPartition() fresh cards = %v; want %v", got, tt.wantFresh)
			}
			if len(tt.wantFresh) > 0 && before != tt.wantDone {
				t.Errorf("PendingPartition() told the batcher %d batches ran before; want %d", before, tt.wantDone)
			}
		})
	}

	t.Run("RecordBeginPartition keeps the done batches first", func(t *testing.T) {
		t.Parallel()

		st := doneState("01-one")
		done, fresh, err := PendingPartition(plan, st, onePerCardBatcher{new(int)}, nil)
		if err != nil {
			t.Fatalf("PendingPartition() error = %v", err)
		}
		RecordBeginPartition(st, done, fresh)

		var got []string
		for _, pb := range st.Partition {
			got = append(got, pb.Cards...)
		}
		if want := []string{"01-one", "02-two", "03-three"}; !slices.Equal(got, want) {
			t.Errorf("st.Partition cards = %v; want %v", got, want)
		}
	})
}
