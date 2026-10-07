// batcher.go defines the package's two core types: Batch, one ordered group of cards, and Batcher,
// the interface every batchifier implements.
// Both types are deliberately minimal — the registry (registry.go) and the library members
// (identity.go, and future grouping batchifiers) build on top of them.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// Batch is one ordered group of cards — webster's execution unit.
type Batch struct {
	Cards []planparser.Card

	// Profile is the Name of the batchifier that formed this batch.
	Profile string

	// Estimate is this batch's segment cost under that batchifier's weights;
	// zero when the batchifier does not estimate.
	Estimate float64
}

// Batcher groups a plan's flat card list into Batches.
type Batcher interface {
	// Batch groups cards, a contiguous run of plan.Cards, into an ordered list of Batches in card order.
	// plan carries the glyph language the estimator maps refs to files with, and sizes reads the worktree facts it weighs.
	Batch(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource) ([]Batch, error)

	// Name reports this batchifier's registry key.
	Name() string
}
