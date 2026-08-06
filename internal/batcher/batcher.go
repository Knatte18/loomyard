// batcher.go defines the package's two core types: Batch, one ordered group of cards, and Batcher,
// the interface every batchifier implements.
// Both types are deliberately minimal — the registry (registry.go) and the library members
// (identity.go, and future grouping batchifiers) build on top of them.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// Batch is one ordered group of cards — webster's execution unit.
type Batch struct {
	Cards []planparser.Card
}

// Batcher groups a plan's flat card list into Batches.
type Batcher interface {
	// Batch groups cards into an ordered list of Batches.
	Batch(cards []planparser.Card) []Batch

	// Name reports this batchifier's registry key.
	Name() string
}
