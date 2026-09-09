// containment.go implements resolveContainment, the resolve-backed tier of the cross-granularity
// containment check: the half string prefixes cannot see, because a package's symbols are spread
// across its files. planparser's own syntacticContainment (internal/planparser/containment.go)
// catches unit-level overlap by string comparison; this tier catches the member-vs-file overlap
// that needs the actual Resolve answer, and reports it under its own check ID so the two tiers
// never collide.

package planglyph

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// writingTargetCards indexes plan's cards by the glyph targets they WRITE — c.Targets alone, which
// already carries both endpoints of every Pairs entry (normalizeCard projects them there) — and never
// by the refs they merely read.
//
// It exists because containment is a WRITE hazard: two cards editing overlapping granularity in
// parallel produce a merge conflict, and two cards reading the same file produce nothing.
// websterengine.deriveEdges encodes exactly that asymmetry, the sibling syntactic tier
// (planparser.syntacticContainment) walks c.Targets alone for the same reason, and both this file's
// own godoc and manifest/designs/quarry-glyph-plan-alphabet.md specify the rule in terms of what a
// card TARGETS.
//
// resolveContainment used targetCards instead, which also indexes Uses and both Pairs endpoints, so a
// card that merely READ a file contributed a self entry and a card that merely READ a symbol
// contributed a member entry: two read-only cards emitted a SeverityBlocking finding, refusing
// `lyx webster run` outright and every dispatch after it, on a plan that was correct (crucible round
// opus-medium-r6, R6-3). targetCards stays as it is for statusFindings and DetectDrift, where
// attributing a resolve answer to every referencing card is the right rule.
func writingTargetCards(plan *planparser.Plan) map[string][]planparser.Card {
	index := make(map[string][]planparser.Card)
	seen := make(map[string]map[string]bool)
	for _, c := range plan.Cards {
		for _, t := range c.Targets {
			id := c.ID()
			if seen[t][id] {
				continue
			}
			if seen[t] == nil {
				seen[t] = make(map[string]bool)
			}
			seen[t][id] = true
			index[t] = append(index[t], c)
		}
	}
	return index
}

// resolveContainment emits check ID containment-file-overlap: for each member glyph's own
// resolved files — read from ResolveResult.Symbols' own File field, never derived, since a
// member's own file need not equal its unit's directory path — it matches against every OTHER
// card's file self glyph, whose own path comes from Glyph.UnitPath(), the one glyph->path call
// the glyph-conversion-chokepoint Shared Decision allows. A multipart result carries several
// symbols and therefore several files; the overlap is reported when any of them matches, since
// editing any part of a multipart symbol touches that file. Findings are blocking, matching the
// syntactic tier's own severity.
//
// The member-glyph target loop below disposes of an unresolved or unreadable answer in two
// deliberately different ways. A target absent from index (!resolved) keeps a silent continue:
// absence is structurally legitimate here, because canonicalization can rewrite a plan: handle into
// a glyph ref that was never part of this pass's own resolve batch, so "no answer" is not a defect
// to report. A target that DID get an answer is a fail-closed vocabulary guard instead, in the same
// shape as doneCheckVerdicts (donecheck.go): StatusFound and StatusMultipart proceed to read
// Symbols; StatusNotFound and StatusAmbiguous keep today's silent skip of the member entry — a
// target that plainly does not resolve, or resolves to several candidates nothing chose between, is
// not a containment hazard; and anything outside that four-value vocabulary, the zero-value Status
// included, raises the blocking finding glyph-rejected (via unreadableStatusDetail, resolve.go)
// instead of silently dropping the member from the containment index, one finding per referencing
// card. statusFindings' own default arm already reports the same anomalous target under the same
// check ID; the double report is accepted by design here, exactly as createFindings and
// doneCheckVerdicts already double-report against the same fail-closed policy.
func resolveContainment(plan *planparser.Plan, results []quarry.ResolveResult) []Finding {
	lang, ok := plan.GlyphLanguage()
	if !ok {
		return nil
	}

	index := resultByTarget(results)
	byTarget := writingTargetCards(plan)

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
	var findings []Finding

	// byTarget is a map, and a Go map range is randomised, so walking it directly made the finding
	// order differ between two runs over an identical plan whenever more than one overlap existed.
	// Every sibling pass in this package sorts deliberately; this one now does too.
	targets := make([]string, 0, len(byTarget))
	for target := range byTarget {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	for _, target := range targets {
		cards := sortedCards(byTarget[target])
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
		if !resolved {
			// Deliberately silent: see this function's own doc comment above.
			continue
		}
		switch r.Status {
		case quarry.StatusFound, quarry.StatusMultipart:
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
		case quarry.StatusNotFound, quarry.StatusAmbiguous:
			// Not a containment hazard: nothing resolved to overlap against.
		default:
			// Fail closed: see this function's own doc comment above.
			for _, c := range cards {
				findings = append(findings, Finding{
					Check:    "glyph-rejected",
					Card:     c.ID(),
					Detail:   unreadableStatusDetail("member target", target, r),
					Severity: SeverityBlocking,
				})
			}
		}
	}

	for _, m := range members {
		for _, s := range selves {
			if m.card.ID() == s.card.ID() {
				continue // a card cannot conflict with itself.
			}
			if !m.files[s.file] {
				continue
			}
			findings = append(findings, Finding{
				Check: "containment-file-overlap",
				Card:  m.card.ID(),
				Detail: fmt.Sprintf(
					"card %d's member glyph physically overlaps card %s's own file self glyph naming %q",
					m.card.Number, s.card.ID(), s.file,
				),
				Severity: SeverityBlocking,
			})
		}
	}

	return findings
}
