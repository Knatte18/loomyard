// identity_test.go covers identityBatcher's one-card-per-batch contract: N cards
// in, N single-card Batches out, in order, including the empty-input and
// one-card boundary cases. Tier-1 (pure logic, no git, no TestMain), per the
// go-test-tiers-and-hermetic-git Shared Decision. The table is written generally
// enough (asserting only "one Batch per input card, in order") that a future
// grouping batchifier can reuse the same shape for its own contract test.

package batcher

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestIdentityBatcher_Batch(t *testing.T) {
	tests := []struct {
		name  string
		cards []planparser.Card
	}{
		{"empty", nil},
		{"oneCard", []planparser.Card{{Number: 1, Slug: "a"}}},
		{"threeCards", []planparser.Card{
			{Number: 1, Slug: "a"},
			{Number: 2, Slug: "b"},
			{Number: 3, Slug: "c"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identityBatcher{}.Batch(tt.cards)
			if len(got) != len(tt.cards) {
				t.Fatalf("Batch(%d cards) returned %d batches; want %d", len(tt.cards), len(got), len(tt.cards))
			}
			for i, batch := range got {
				if len(batch.Cards) != 1 {
					t.Errorf("batch %d has %d cards; want 1", i, len(batch.Cards))
					continue
				}
				if batch.Cards[0].Number != tt.cards[i].Number {
					t.Errorf("batch %d carries card %d; want card %d (order not preserved)", i, batch.Cards[0].Number, tt.cards[i].Number)
				}
			}
		})
	}
}

func TestIdentityBatcher_Name(t *testing.T) {
	if got := (identityBatcher{}).Name(); got != "identity" {
		t.Errorf("identityBatcher{}.Name() = %q; want %q", got, "identity")
	}
}
