// boundary.go derives the batch boundaries a later card's rename or delete of an earlier card's target imposes on a grouping batchifier.

package batcher

import "github.com/Knatte18/loomyard/internal/planparser"

// renameBoundaries returns one entry per card: the index in cards of the first later card that covers one of the card's targets, or len(cards) when none does.
// A card's targets are the Refs of its Create, Edit and Delete groups and the New side of each Rename pair;
// a later card covers them with the Old side of each of its Rename pairs and the Refs of its Delete groups, as planparser.RefCovers decides.
// A card is never compared with itself.
// The rule only adds batch boundaries: it never merges batches, reorders cards or moves a card across a dependency, and every grouping batchifier calls it.
func renameBoundaries(plan *planparser.Plan, cards []planparser.Card) []int {
	boundaries := make([]int, len(cards))
	for i, card := range cards {
		boundaries[i] = len(cards)
		targets := cardTargets(card)
		for j := i + 1; j < len(cards) && boundaries[i] == len(cards); j++ {
			for _, cover := range planparser.CardCovers(cards[j]) {
				if coversAny(plan, cover, targets) {
					boundaries[i] = j
					break
				}
			}
		}
	}
	return boundaries
}

// coversAny reports whether cover covers at least one of targets.
func coversAny(plan *planparser.Plan, cover string, targets []string) bool {
	for _, target := range targets {
		if planparser.RefCovers(plan, cover, target) {
			return true
		}
	}
	return false
}

// cardTargets returns the refs card writes: its Create, Edit and Delete refs and the New side of each Rename pair.
func cardTargets(card planparser.Card) []string {
	var targets []string
	for _, group := range card.TargetGroups {
		switch group.Type {
		case planparser.CardTypeCreate, planparser.CardTypeEdit, planparser.CardTypeDelete:
			targets = append(targets, group.Refs...)
		case planparser.CardTypeRename:
			for _, pair := range group.Pairs {
				targets = append(targets, pair.New)
			}
		}
	}
	return targets
}
