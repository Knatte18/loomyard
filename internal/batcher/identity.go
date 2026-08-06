// identity.go implements identityBatcher, the library's baseline Batcher: one card per Batch, in input order.
// It self-registers into the package registry at package init so webster's call sites never construct it directly.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// identityBatcher is the simplest Batcher: one card per Batch.
type identityBatcher struct{}

// Batch returns one single-card Batch per card, preserving input order.
func (identityBatcher) Batch(cards []planparser.Card) []Batch {
	batches := make([]Batch, len(cards))
	for i, card := range cards {
		batches[i] = Batch{Cards: []planparser.Card{card}}
	}
	return batches
}

// Name reports this batchifier's registry key.
func (identityBatcher) Name() string {
	return "identity"
}

// init registers identityBatcher in the package registry.
func init() {
	register(identityBatcher{})
}
