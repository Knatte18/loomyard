// containment.go implements resolveContainment, the resolve-backed tier of the cross-granularity
// containment check: the half string prefixes cannot see, because a package's symbols are spread
// across its files. planparser's own syntacticContainment (internal/planparser/containment.go)
// catches unit-level overlap by string comparison; this tier catches the member-vs-file overlap
// that needs the actual Resolve answer, and reports it under its own check ID so the two tiers
// never collide.

package planglyph

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// resolveContainment emits check ID containment-file-overlap: for each member glyph's own
// resolved files — read from ResolveResult.Symbols' own File field, never derived, since a
// member's own file need not equal its unit's directory path — it matches against every OTHER
// card's file self glyph, whose own path comes from Glyph.UnitPath(), the one glyph->path call
// the glyph-conversion-chokepoint Shared Decision allows. A multipart result carries several
// symbols and therefore several files; the overlap is reported when any of them matches, since
// editing any part of a multipart symbol touches that file. Findings are blocking, matching the
// syntactic tier's own severity.
func resolveContainment(plan *planparser.Plan, results []quarry.ResolveResult) []Finding {
	lang, ok := resolveLanguage(plan)
	if !ok {
		return nil
	}

	index := resultByTarget(results)
	byTarget := targetCards(plan)

	type memberEntry struct {
		card  planparser.Card
		files map[string]bool
	}
	type selfEntry struct {
		card planparser.Card
		file string
	}

	var members []memberEntry
	var selves []selfEntry

	for target, cards := range byTarget {
		g, err := glyph.Parse(lang, target)
		if err != nil {
			continue // not glyph-shaped: a path, a symbol, or a plan: handle.
		}

		if g.IsSelf() {
			file, ok := g.UnitPath()
			if !ok {
				continue
			}
			for _, c := range cards {
				selves = append(selves, selfEntry{card: c, file: file})
			}
			continue
		}

		r, resolved := index[target]
		if !resolved || (r.Status != quarry.StatusFound && r.Status != quarry.StatusMultipart) {
			continue
		}
		files := make(map[string]bool)
		for _, s := range r.Symbols {
			if s.File != "" {
				files[s.File] = true
			}
		}
		if len(files) == 0 {
			continue
		}
		for _, c := range cards {
			members = append(members, memberEntry{card: c, files: files})
		}
	}

	var findings []Finding
	for _, m := range members {
		for _, s := range selves {
			if cardIDOf(m.card) == cardIDOf(s.card) {
				continue // a card cannot conflict with itself.
			}
			if !m.files[s.file] {
				continue
			}
			findings = append(findings, Finding{
				Check: "containment-file-overlap",
				Card:  cardIDOf(m.card),
				Detail: fmt.Sprintf(
					"card %d's member glyph physically overlaps card %s's own file self glyph naming %q",
					m.card.Number, cardIDOf(s.card), s.file,
				),
				Severity: SeverityBlocking,
			})
		}
	}

	return findings
}
