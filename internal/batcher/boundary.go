// boundary.go derives the batch boundaries two rules impose on a grouping batchifier:
// a later card's rename or delete of an earlier card's target, and a later card's use of a C# member an earlier card re-signs.

package batcher

import (
	"slices"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
)

// renameBoundaries returns one entry per card: the index in cards of the first later card that covers one of the card's targets or names a C# member the card re-signs, or len(cards) when none does.
// A card's targets are the Refs of its Create, Edit and Delete groups and the New side of each Rename pair;
// a later card covers them with the Old side of each of its Rename pairs and the Refs of its Delete groups, as planparser.RefCovers decides.
// A card re-signs a C# member with an Edit group arrow whose target parses as a C# glyph, and a later card names it with that target's exact string among its Targets or Uses.
// The re-sign is rewritten to the member's new glyph only once its batch is recorded, so the naming card cannot share the batch.
// A card is never compared with itself.
// The rules only add batch boundaries: they never merge batches, reorder cards or move a card across a dependency, and every grouping batchifier calls them.
func renameBoundaries(plan *planparser.Plan, cards []planparser.Card) []int {
	boundaries := make([]int, len(cards))
	for i, card := range cards {
		boundaries[i] = len(cards)
		targets := cardTargets(card)
		resigned := csharpResignTargets(plan, card)
		for j := i + 1; j < len(cards) && boundaries[i] == len(cards); j++ {
			for _, cover := range planparser.CardCovers(cards[j]) {
				if coversAny(plan, cover, targets) {
					boundaries[i] = j
					break
				}
			}
			if boundaries[i] == len(cards) && namesAny(cards[j], resigned) {
				boundaries[i] = j
			}
		}
	}
	return boundaries
}

// csharpResignTargets returns the target of each re-sign arrow in card's Edit groups that plan's alphabet parses as a C# glyph, as the card spells it.
func csharpResignTargets(plan *planparser.Plan, card planparser.Card) []string {
	var targets []string
	for _, group := range card.TargetGroups {
		if group.Type != planparser.CardTypeEdit {
			continue
		}
		for _, resign := range group.Resigns {
			if g, err := planparser.ParseRef(plan, resign.Target); err == nil && g.Lang == glyph.CSharp {
				targets = append(targets, resign.Target)
			}
		}
	}
	return targets
}

// namesAny reports whether card lists one of targets, spelled exactly, among its Targets or Uses.
func namesAny(card planparser.Card, targets []string) bool {
	for _, target := range targets {
		if slices.Contains(card.Targets, target) || slices.Contains(card.Uses, target) {
			return true
		}
	}
	return false
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
