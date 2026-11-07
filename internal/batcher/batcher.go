// batcher.go defines Batch, one ordered group of cards, and Batcher, the interface every batchifier implements.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// Batch is one ordered group of cards, the unit one implementer executes.
type Batch struct {
	Cards []planparser.Card

	// Profile is the registry key of the batchifier that formed this batch.
	Profile string

	// Estimate is this batch's estimated peak fork context under that batchifier's weights;
	// zero when the batchifier does not estimate.
	Estimate float64

	// Breakdown records the components of the batch's estimate;
	// nil when the batchifier does not estimate.
	Breakdown *Breakdown
}

// Batcher groups a plan's flat card list into Batches.
type Batcher interface {
	// Batch groups cards, a contiguous run of plan.Cards, into an ordered list of Batches in card order.
	// plan carries the glyph language the estimator maps refs to files with.
	// sizes reads the worktree facts it weighs.
	// before is the number of batches the run executes ahead of cards[0],
	// so the k-th returned batch is the run's batch at position before + k.
	// base is the part of the orchestrating session's start context computed from what it loads;
	// the zero value adds nothing.
	Batch(plan *planparser.Plan, cards []planparser.Card, sizes SizeSource, before int, base StartBase) ([]Batch, error)

	// Name reports this batchifier's registry key.
	Name() string
}
