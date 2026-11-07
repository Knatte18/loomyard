// boundary.go derives the batch boundaries two rules impose on a grouping batchifier:
// a later card renaming or deleting an earlier card's target; a later card using a C# member an earlier card re-signs.

package batcher

import (
	"slices"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
)

// renameBoundaries returns one entry per card: the index of the first later card that conflicts with it.
// The entry is len(cards) when no later card does.
// A card's targets are the Refs of its Create, Edit, Extends and Delete groups and the New side of each Rename pair;
// a later card covers them with the Old side of each Rename pair and the Refs of each Delete group.
// Whether a cover reaches a target follows planparser's cover rule.
// A later card conflicts with a card when it covers one of the card's targets or names a C# member the card re-signs.
// A card re-signs a C# member with an Edit group arrow whose target parses as a C# glyph,
// and a later card names it with that target's exact string among its Targets or Uses.
// The re-sign is rewritten to the member's new glyph only once its batch is recorded.
// The naming card therefore cannot share the batch.
// The rules only add batch boundaries:
// they never merge batches, reorder cards or move a card across a dependency,
// and every grouping batchifier calls them.
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

// csharpResignTargets returns the re-sign targets in card's Edit groups that plan's alphabet parses as C# glyphs.
// Each target is kept as the card spells it.
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

// cardTargets returns the refs card writes:
// its Create, Edit, Extends and Delete refs and the New side of each Rename pair.
func cardTargets(card planparser.Card) []string {
	var targets []string
	for _, group := range card.TargetGroups {
		switch group.Type {
		case planparser.CardTypeCreate, planparser.CardTypeEdit, planparser.CardTypeExtends, planparser.CardTypeDelete:
			targets = append(targets, group.Refs...)
		case planparser.CardTypeRename:
			for _, pair := range group.Pairs {
				targets = append(targets, pair.New)
			}
		}
	}
	return targets
}
