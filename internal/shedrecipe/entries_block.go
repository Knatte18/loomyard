// entries_block.go implements the Block registry entry and the producer of an expanded block row.
// It also holds the closed table of block gate checks that a block row's gate is chosen from.

package shedrecipe

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// EngineBlock is the engine name a block row carries once its recipe is expanded.
// A recipe file never writes it: the block row's keys name the recipe to inline and the check that gates it.
const EngineBlock = "Block"

// BlockGateAlways is the block gate check that always holds, so the block runs on every pass.
const BlockGateAlways = "always"

// BlockGateUndiscussed is the block gate check that holds unless the run's board entry is flagged discussed.
// For a discussed entry it seeds the entry's stored decision record into the run through Env.SeedDiscussedRecord.
// It then passes over the block.
const BlockGateUndiscussed = "undiscussed"

// undiscussedSkipReason is the reason the undiscussed gate records when it passes over its block.
const undiscussedSkipReason = "the board entry is flagged discussed: its stored decision record is the run's, so the block's Discussion is passed over"

// BlockGate builds a block row's entry gate from the Env it is given.
type BlockGate func(env Env) (shedengine.EntryGate, error)

// BlockGates is the closed table of block gate checks, keyed by the check name a block row's block_gate key writes.
type BlockGates map[string]BlockGate

// blockGates is the one table of block gate checks lym ships.
// A new check is a row here, never code in a recipe or its builder.
var blockGates = BlockGates{
	BlockGateAlways:      alwaysBlockGate,
	BlockGateUndiscussed: undiscussedBlockGate,
}

// alwaysBlockGate builds the gate of the BlockGateAlways check, which holds with an empty reason.
func alwaysBlockGate(Env) (shedengine.EntryGate, error) {
	return func() (bool, string, error) { return true, "", nil }, nil
}

// undiscussedBlockGate builds the gate of the BlockGateUndiscussed check.
// It requires env.SeedDiscussedRecord to be non-nil.
// The gate holds with an empty reason when the seam seeds nothing.
// When the seam seeded the stored record, the gate passes over the block with undiscussedSkipReason.
// The seam's error is the gate's.
func undiscussedBlockGate(env Env) (shedengine.EntryGate, error) {
	if err := requireSeam("block gate "+BlockGateUndiscussed, "SeedDiscussedRecord", env.SeedDiscussedRecord); err != nil {
		return nil, err
	}
	return func() (bool, string, error) {
		seeded, err := env.SeedDiscussedRecord()
		if err != nil {
			return false, "", err
		}
		if seeded {
			return false, undiscussedSkipReason, nil
		}
		return true, "", nil
	}, nil
}

// LookupBlockGate returns the block gate check that name selects.
// An unknown name is refused, and the error lists the known names.
func LookupBlockGate(name string) (BlockGate, error) {
	gate, ok := blockGates[name]
	if !ok {
		return nil, fmt.Errorf("shedrecipe: unknown block gate %q; way forward: name one of: %s", name, strings.Join(BlockGateNames(), ", "))
	}
	return gate, nil
}

// BlockGateNames returns every block gate check name, sorted, in a freshly allocated slice each call.
func BlockGateNames() []string {
	names := make([]string, 0, len(blockGates))
	for name := range blockGates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// blockEntry is the Constructor for the "Block" registry row. It accepts no Config key and validates no Env field.
func blockEntry(_ RowIdentity, cfg Config, _ Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	return blockProducer{}, nil
}

// blockProducer is the producer of a block row.
// The row does no work of its own: its Done marks the run entering the block.
type blockProducer struct{}

// Call returns Done with no output at once.
func (blockProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
