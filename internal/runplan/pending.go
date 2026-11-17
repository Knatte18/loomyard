// pending.go partitions the cards a run has not yet built, and records that partition beside the batches already done.

package runplan

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// PendingPartition partitions the cards of plan that no terminal done batch record of st holds.
// A card is held by slug: each record's card ids are matched on the slug after their number,
// so a card renumbered since its batch ran still counts as done.
// It returns the done batches as st.Partition records them, in order,
// and the fresh batches b forms over the remaining cards.
// b is told that the done batches run ahead of the remaining cards.
// A plan with no remaining card returns no fresh batch.
func PendingPartition(plan *planparser.Plan, st *State, b batcher.Batcher, sizes batcher.SizeSource) ([]PartitionBatch, []batcher.Batch, error) {
	doneSlugs := doneCardSlugs(st)

	var done []PartitionBatch
	for _, pb := range st.Partition {
		if len(pb.Cards) > 0 && allSlugsDone(pb.Cards, doneSlugs) {
			done = append(done, pb)
		}
	}

	var remaining []planparser.Card
	for _, c := range plan.Cards {
		if !doneSlugs[c.Slug] {
			remaining = append(remaining, c)
		}
	}
	if len(remaining) == 0 {
		return done, nil, nil
	}

	fresh, err := b.Batch(plan, remaining, sizes, len(done), batcher.StartBase{})
	if err != nil {
		return nil, nil, fmt.Errorf("runplan: batch the remaining cards of plan %s: %w", plan.Dir, err)
	}
	return done, fresh, nil
}

// RecordBeginPartition replaces st.Partition with the done batches as given, followed by the fresh batches.
// The fresh batches are recorded as RecordPartition records them.
func RecordBeginPartition(st *State, done []PartitionBatch, fresh []batcher.Batch) {
	RecordPartition(st, fresh)
	st.Partition = append(append([]PartitionBatch{}, done...), st.Partition...)
}

// doneCardSlugs returns the slugs of every card a terminal done batch record of st holds.
// A record written before card ids existed names the single card of its own slug.
func doneCardSlugs(st *State) map[string]bool {
	slugs := make(map[string]bool)
	for _, bs := range st.Batches {
		if bs == nil || !bs.Terminal || bs.Status != DigestStatusDone {
			continue
		}
		if len(bs.Cards) == 0 {
			slugs[bs.Slug] = true
			continue
		}
		for _, id := range bs.Cards {
			_, slug, _ := strings.Cut(id, "-")
			slugs[slug] = true
		}
	}
	return slugs
}

// allSlugsDone reports whether the slug of every card id is in done.
func allSlugsDone(ids []string, done map[string]bool) bool {
	for _, id := range ids {
		_, slug, _ := strings.Cut(id, "-")
		if !done[slug] {
			return false
		}
	}
	return true
}
