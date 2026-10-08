// identity.go implements identityBatcher, the library's baseline Batcher: one card per Batch, in
// input order.
// Identity returns it, and the registry's identity kind builds it.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// identityBatcher is the simplest Batcher: one card per Batch.
type identityBatcher struct{}

// Identity returns the identity batchifier, whatever profile is active.
func Identity() Batcher {
	return identityBatcher{}
}

// Batch returns one single-card Batch per card, preserving input order.
// It ignores plan, sizes, before and base, and never fails.
func (identityBatcher) Batch(_ *planparser.Plan, cards []planparser.Card, _ SizeSource, _ int, _ StartBase) ([]Batch, error) {
	batches := make([]Batch, len(cards))
	for i, card := range cards {
		batches[i] = Batch{Cards: []planparser.Card{card}, Profile: identityBatcher{}.Name()}
	}
	return batches, nil
}

// Name reports this batchifier's registry key.
func (identityBatcher) Name() string {
	return "identity"
}
