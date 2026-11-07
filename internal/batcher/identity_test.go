// identity_test.go covers identityBatcher's one-card-per-batch contract.
// It checks N cards in, N single-card batches out, in order, including the empty-input and one-card cases.
// Tier-1 (pure logic, no git, no TestMain).

package batcher

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestIdentityBatcher_Batch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		cards  []planparser.Card
		before int
	}{
		{"empty", nil, 0},
		{"oneCard", []planparser.Card{{Number: 1, Slug: "a"}}, 0},
		{"threeCards", []planparser.Card{
			{Number: 1, Slug: "a"},
			{Number: 2, Slug: "b"},
			{Number: 3, Slug: "c"},
		}, 0},
		{"aLaterBeforeLeavesTheOutputUnchanged", []planparser.Card{{Number: 4, Slug: "d"}, {Number: 5, Slug: "e"}}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Identity().Batch(nil, tt.cards, nil, tt.before, StartBase{})
			if err != nil {
				t.Fatalf("Batch: %v", err)
			}
			if len(got) != len(tt.cards) {
				t.Fatalf("Batch(%d cards) returned %d batches; want %d", len(tt.cards), len(got), len(tt.cards))
			}
			for i, batch := range got {
				if len(batch.Cards) != 1 {
					t.Errorf("batch %d has %d cards; want 1", i, len(batch.Cards))
					continue
				}
				if batch.Profile != "identity" || batch.Estimate != 0 {
					t.Errorf("batch %d has profile %q and estimate %v; want %q and 0", i, batch.Profile, batch.Estimate, "identity")
				}
				if batch.Cards[0].Number != tt.cards[i].Number {
					t.Errorf("batch %d carries card %d; want card %d (order not preserved)", i, batch.Cards[0].Number, tt.cards[i].Number)
				}
			}
		})
	}
}

func TestIdentityBatcher_Name(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if got := (identityBatcher{}).Name(); got != "identity" {
		t.Errorf("identityBatcher{}.Name() = %q; want %q", got, "identity")
	}
}
