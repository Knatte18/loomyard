// partition.go resolves the batches every webster verb runs: the partition the active batchifier forms once per run, the copy of it state.json records, and the fallback for a run that started before partitions were recorded.

package websterengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrPartitionMismatch is the sentinel ExecutionBatches' error wraps when the recorded partition and the plan's cards no longer name the same cards.
var ErrPartitionMismatch = errors.New("webster: recorded partition does not match the plan")

// PartitionBatch is one recorded batch of the run's partition: the NN-<slug> ids of its cards in card order, the profile that formed it and its estimate.
// Breakdown keeps the estimate's components, so a finished run can be fitted against the peak context its forks measured.
type PartitionBatch struct {
	Cards     []string           `json:"cards"`
	Profile   string             `json:"profile,omitempty"`
	Estimate  float64            `json:"estimate,omitempty"`
	Breakdown *batcher.Breakdown `json:"breakdown,omitempty"`
}

// cardID returns the NN-<slug> id of c.
func cardID(c planparser.Card) string {
	return fmt.Sprintf("%02d-%s", c.Number, c.Slug)
}

// formBatches batches the plan's whole card list with active, reading file sizes from sizes and pricing Merriam's start at base, and asserts the result runs in dependency order.
// A batchifier failure is refused as transient, naming the plan;
// an order violation wraps ErrBatchOrder.
func formBatches(plan *planparser.Plan, active batcher.Batcher, sizes batcher.SizeSource, before int, base batcher.StartBase) ([]batcher.Batch, error) {
	batches, err := batchCards(plan, plan.Cards, active, sizes, before, base)
	if err != nil {
		return nil, err
	}
	if err := CheckBatchOrder(batches, ""); err != nil {
		return nil, err
	}
	return batches, nil
}

// batchCards batches cards, a contiguous run of plan.Cards, with active, told that the run executes before batches ahead of them and that Merriam starts at base.
// A batchifier failure is refused as transient, naming the plan.
func batchCards(plan *planparser.Plan, cards []planparser.Card, active batcher.Batcher, sizes batcher.SizeSource, before int, base batcher.StartBase) ([]batcher.Batch, error) {
	batches, err := active.Batch(plan, cards, sizes, before, base)
	if err != nil {
		return nil, fmt.Errorf("webster: batch the cards of plan %s: %w; way forward: transient, re-run the verb", plan.Dir, err)
	}
	return batches, nil
}

// ExecutionBatches returns the batches every webster verb runs, in execution order, after asserting the order with CheckBatchOrder:
//   - with no state, the active batchifier's grouping of the whole plan, the partition validate and first init compute;
//   - with a recorded partition, its batches mapped onto the current plan's cards by id, each keeping its recorded profile and estimate, so a size source or start base that changed since never regroups a run;
//   - with a state that records no partition, the identity batchifier over the whole plan, whatever profile is active, so a run started before partitions were recorded runs on as it started.
func ExecutionBatches(plan *planparser.Plan, st *State, active batcher.Batcher, sizes batcher.SizeSource, base batcher.StartBase) ([]batcher.Batch, error) {
	switch {
	case st == nil:
		return formBatches(plan, active, sizes, 0, base)
	case len(st.Partition) == 0:
		return formBatches(plan, batcher.Identity(), sizes, 0, base)
	}

	batches, err := mapPartition(plan, st.Partition)
	if err != nil {
		return nil, err
	}
	if err := CheckBatchOrder(batches, ""); err != nil {
		return nil, err
	}
	return batches, nil
}

// mapPartition maps the recorded batches onto the plan's cards by id, refusing with ErrPartitionMismatch when a recorded id is not a card of the plan or a plan card is in no recorded batch.
func mapPartition(plan *planparser.Plan, partition []PartitionBatch) ([]batcher.Batch, error) {
	cardsByID := make(map[string]planparser.Card, len(plan.Cards))
	for _, c := range plan.Cards {
		cardsByID[cardID(c)] = c
	}

	recorded := make(map[string]bool, len(plan.Cards))
	var unknown []string
	batches := make([]batcher.Batch, 0, len(partition))
	for _, pb := range partition {
		batch := batcher.Batch{Profile: pb.Profile, Estimate: pb.Estimate, Breakdown: pb.Breakdown}
		for _, id := range pb.Cards {
			card, ok := cardsByID[id]
			if !ok {
				unknown = append(unknown, id)
				continue
			}
			recorded[id] = true
			batch.Cards = append(batch.Cards, card)
		}
		batches = append(batches, batch)
	}

	var unrecorded []string
	for _, c := range plan.Cards {
		if id := cardID(c); !recorded[id] {
			unrecorded = append(unrecorded, id)
		}
	}
	if len(unknown) == 0 && len(unrecorded) == 0 {
		return batches, nil
	}

	sort.Strings(unknown)
	var problems []string
	if len(unknown) > 0 {
		problems = append(problems, "recorded cards "+strings.Join(unknown, ", ")+" are not cards of the plan")
	}
	if len(unrecorded) > 0 {
		problems = append(problems, "plan cards "+strings.Join(unrecorded, ", ")+" are in no recorded batch")
	}
	return nil, fmt.Errorf("%w: %s; way forward: `lyx webster rebaseline --card NN` naming each card you edited, or restore the plan with `lyx webster restore-plan`",
		ErrPartitionMismatch, strings.Join(problems, "; "))
}

// RecordPartition writes the batches' card ids, profiles, estimates and estimate breakdowns into st.Partition, replacing any partition it held.
func RecordPartition(st *State, batches []batcher.Batch) {
	partition := make([]PartitionBatch, len(batches))
	for i, b := range batches {
		partition[i] = PartitionBatch{Cards: batchCardIDs(b), Profile: b.Profile, Estimate: b.Estimate, Breakdown: b.Breakdown}
	}
	st.Partition = partition
}
