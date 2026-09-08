// containment.go implements syntacticContainment, the pure half of the cross-granularity
// containment check: the hole a symbol-granular DAG would otherwise hide.
// websterengine.deriveEdges matches refs by exact string equality, which correctly sees that two
// cards touching different members do not serialize, but two refs at different granularities can
// overlap physically with no string equality between them: no edge, blind parallel dispatch, merge
// conflict. This tier catches the two halves lexical work can see — a member glyph against another
// card's self glyph of the same unit, and a file self glyph against another card's self glyph of
// the directory that file sits in; the member->file half needs the Resolve answer and is
// internal/planglyph's business, added as a separately-identified finding there.

package planparser

import (
	"fmt"
	"path"

	"github.com/Knatte18/quarry/glyph"
)

// syntacticContainment emits check ID containment-unit-overlap, in two lexical pairings over every
// card's own Targets. First, member-vs-self: a member glyph on one card compared against every self
// glyph on every OTHER card, reported when the other card's self glyph names that same unit.
// Comparing parsed Glyph.Unit values rather than doing string-prefix arithmetic on the raw ref is
// deliberate, so the rule cannot drift from the alphabet.
// Second, file-self-vs-unit-self: a FILE self glyph on one card compared against every extensionless
// (directory/package) self glyph on every other card, reported when the file's own parent directory
// IS that unit's directory. Both refs being self glyphs, neither the member-vs-self pairing above
// nor planglyph's resolve-backed member-vs-file tier ever compared them, so a card targeting a
// whole package (`internal/foo#` — the spelling prosa-symbol-target explicitly admits) and a card
// targeting one of its files (`internal/foo/bar.go#`) dispatched blind in parallel on physically
// overlapping targets — the exact hazard both containment tiers exist to close (crucible round
// fable-high-r10, F4). A Go unit's files are its direct children, so parent-directory equality is
// exact; both paths come from Glyph.UnitPath(), the one glyph->path call the
// glyph-conversion-chokepoint Shared Decision allows.
// A card cannot conflict with itself: two glyphs on the same card are never compared. Two member
// glyphs in one unit produce no finding either -- symbol granularity is exactly the case this tier
// does not flag -- and neither do two file self glyphs (distinct files) or two extensionless self
// glyphs (distinct directories, since Go units do not nest their files).
func syntacticContainment(plan *Plan, lang glyph.Language) []ValidationError {
	var findings []ValidationError

	type unitOnCard struct {
		card     Card
		unit     string
		unitPath string
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
				unitPath, ok := g.UnitPath()
				if !ok {
					continue
				}
				selves = append(selves, unitOnCard{card: c, unit: g.Unit, unitPath: unitPath})
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

	for _, f := range selves {
		if !hasFileExtension(f.unitPath) {
			continue
		}
		for _, s := range selves {
			if cardID(f.card) == cardID(s.card) || hasFileExtension(s.unitPath) {
				continue
			}
			if path.Dir(f.unitPath) != s.unitPath {
				continue
			}
			findings = append(findings, ValidationError{
				Check: "containment-unit-overlap",
				Card:  cardID(f.card),
				Detail: fmt.Sprintf(
					"card %d's file self glyph naming %q physically overlaps card %s's own self glyph naming that file's whole unit %q",
					f.card.Number, f.unitPath, cardID(s.card), s.unit,
				),
			})
		}
	}

	return findings
}
