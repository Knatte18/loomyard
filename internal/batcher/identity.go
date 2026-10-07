// identity.go implements identityBatcher, the library's baseline Batcher: one card per Batch, in
// input order.
// It self-registers into the package registry at package init, and Identity returns it without a
// registry lookup.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// identityBatcher is the simplest Batcher: one card per Batch.
type identityBatcher struct{}

// Identity returns the identity batchifier, whatever profile is active.
func Identity() Batcher {
	return identityBatcher{}
}

// Batch returns one single-card Batch per card, preserving input order.
// It ignores plan and sizes and never fails.
func (identityBatcher) Batch(_ *planparser.Plan, cards []planparser.Card, _ SizeSource) ([]Batch, error) {
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

// init registers identityBatcher in the package registry.
func init() {
	register(identityBatcher{})
}
