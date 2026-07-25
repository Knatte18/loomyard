// identity.go implements identityBatcher, the library's baseline Batcher: one card
// per Batch, in input order. It self-registers into the package registry at
// package init so webster's call sites never construct it directly.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// identityBatcher is the simplest possible Batcher: it makes no grouping decision
// at all, forking every card in its own single-card Batch. It is the registry's
// DefaultName entry, not an interim or "v0" implementation — production plans run
// under it unless webster.yaml's batcher: key names a different registered
// batchifier.
type identityBatcher struct{}

// Batch implements Batcher by returning one single-card Batch per element of
// cards, preserving input order: N cards in, N Batches out.
func (identityBatcher) Batch(cards []planparser.Card) []Batch {
	batches := make([]Batch, len(cards))
	for i, card := range cards {
		batches[i] = Batch{Cards: []planparser.Card{card}}
	}
	return batches
}

// Name implements Batcher, reporting this batchifier's registry key.
func (identityBatcher) Name() string {
	return "identity"
}

// init registers identityBatcher under its own Name() so it is resolvable via
// Select as soon as the package is imported.
func init() {
	register(identityBatcher{})
}
