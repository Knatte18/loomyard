// batcher.go defines the package's two core types: Batch, one ordered group of
// cards, and Batcher, the interface every batchifier implements. Both types are
// deliberately minimal — the registry (registry.go) and the library members
// (identity.go, and future grouping batchifiers) build on top of them.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// Batch is one ordered group of at least one card — webster's own execution unit.
// A fork owns exactly one Batch: it is spawned with that Batch's cards and nothing
// else. Batch carries only the card grouping itself; the webster-side run-state
// record for a batch (slug, start SHA, terminal status, carried-forward digest) is
// a separate type, BatchState, owned by websterengine — batcher has no reason to
// know about a batch's run-time lifecycle.
type Batch struct {
	// Cards is this batch's cards, in the order webster forks them.
	Cards []planparser.Card
}

// Batcher groups a plan's flat card list into Batches. Each library member
// implements one grouping policy (identity.go's one-card-per-batch policy is the
// first) and self-registers into the package registry via its own init(), so
// webster's call sites never name a concrete batchifier type directly — they
// resolve one by name through Select.
type Batcher interface {
	// Batch groups cards into an ordered list of Batches. Implementations own the
	// entire grouping decision: card order within a Batch, Batch order within the
	// returned slice, and how many cards land in each Batch.
	Batch(cards []planparser.Card) []Batch

	// Name reports this batchifier's registry key, the same string
	// webster.yaml's batcher: config key and Select's name argument use to select
	// it.
	Name() string
}
