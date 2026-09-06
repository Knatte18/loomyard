// containment.go implements syntacticContainment, the pure half of the cross-granularity
// containment check: the hole a symbol-granular DAG would otherwise hide.
// websterengine.deriveEdges matches refs by exact string equality, which correctly sees that two
// cards touching different members do not serialize, but a card targeting a member glyph and a
// card targeting the file self glyph of the file that member lives in overlap physically with no
// string equality between them: no edge, blind parallel dispatch, merge conflict. This tier
// catches the half that string prefixes can see; the member->file half needs the Resolve answer
// and is internal/planglyph's business, added as a separately-identified finding there.

package planparser

import (
	"fmt"

	"github.com/Knatte18/quarry/glyph"
)

// syntacticContainment emits check ID containment-unit-overlap: for every member glyph on one
// card's own Targets, it compares that glyph's Glyph.Unit against every self glyph on every other
// card's own Targets, and reports the pair when the other card's self glyph names that same unit.
// Comparing parsed Glyph.Unit values rather than doing string-prefix arithmetic on the raw ref is
// deliberate, so the rule cannot drift from the alphabet.
// A card cannot conflict with itself: two glyphs on the same card are never compared. Two member
// glyphs in one unit produce no finding either -- symbol granularity is exactly the case this tier
// does not flag.
func syntacticContainment(plan *Plan, lang glyph.Language) []ValidationError {
	var findings []ValidationError

	type unitOnCard struct {
		card Card
		unit string
	}

	var members []unitOnCard
	var selves []unitOnCard

	for _, c := range plan.Cards {
		for _, t := range c.Targets {
			if classifyRef(t) != refKindGlyph {
				continue
			}
			g, err := parseGlyph(lang, t)
			if err != nil {
				continue
			}
			if g.IsSelf() {
				selves = append(selves, unitOnCard{card: c, unit: g.Unit})
			} else {
				members = append(members, unitOnCard{card: c, unit: g.Unit})
			}
		}
	}

	for _, m := range members {
		for _, s := range selves {
			if cardID(m.card) == cardID(s.card) {
				continue
			}
			if m.unit != s.unit {
				continue
			}
			findings = append(findings, ValidationError{
				Check: "containment-unit-overlap",
				Card:  cardID(m.card),
				Detail: fmt.Sprintf(
					"card %d's member glyph in unit %q physically overlaps card %s's own self glyph naming the same unit",
					m.card.Number, m.unit, cardID(s.card),
				),
			})
		}
	}

	return findings
}
