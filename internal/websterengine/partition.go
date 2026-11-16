// partition.go resolves the batches every webster verb runs.
// It covers the partition the active batchifier forms once per run, and the copy of it that state.json records.
// It also covers the fallback for a run that started before partitions were recorded.

package websterengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
)

// formBatches batches the plan's whole card list with active.
// It reads file sizes from sizes and prices the master session's start at base.
// It asserts that the result runs in dependency order.
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

// batchCards batches cards, a contiguous run of plan.Cards, with active.
// It tells active that `before` batches run ahead of these cards, and that the master session starts at base.
// A batchifier failure is refused as transient, naming the plan.
func batchCards(plan *planparser.Plan, cards []planparser.Card, active batcher.Batcher, sizes batcher.SizeSource, before int, base batcher.StartBase) ([]batcher.Batch, error) {
	batches, err := active.Batch(plan, cards, sizes, before, base)
	if err != nil {
		return nil, fmt.Errorf("webster: batch the cards of plan %s: %w; way forward: transient, re-run the verb", plan.Dir, err)
	}
	return batches, nil
}

// ExecutionBatches returns the batches every webster verb runs, in execution order.
// It asserts the order with CheckBatchOrder. The source of the batches depends on the state:
//   - with no state, the active batchifier's grouping of the whole plan, the partition validate and first init compute;
//   - with a recorded partition, its batches mapped onto the current plan's cards by id;
//     each keeping its recorded profile and estimate, so a changed size source or start base never regroups the run;
//   - with a state that records no partition, the identity batchifier over the whole plan, whatever profile is active;
//     so a run started before partitions were recorded runs on as it started.
func ExecutionBatches(plan *planparser.Plan, st *State, active batcher.Batcher, sizes batcher.SizeSource, base batcher.StartBase) ([]batcher.Batch, error) {
	switch {
	case st == nil:
		return formBatches(plan, active, sizes, 0, base)
	case len(st.Partition) == 0:
		return formBatches(plan, batcher.Identity(), sizes, 0, base)
	}

	batches, err := runplan.MapPartition(plan, st.Partition, partitionMismatchWayForward)
	if err != nil {
		return nil, err
	}
	if err := CheckBatchOrder(batches, ""); err != nil {
		return nil, err
	}
	return batches, nil
}
