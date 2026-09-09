// resolve.go implements statusFindings, the resolve status policy layered onto every batched
// Resolve answer that is not a Create group's own target — Create's inversion is card 18's, in
// create.go, and is deliberately not woven in here so this policy stays readable on its own.

package planglyph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// targetCards indexes plan's cards by every glyph target they reference — Targets, Uses, and both
// Pairs endpoints alike — so a resolve-backed finding can be attributed to every card that
// references the target, one finding per referencing card, rather than one unattributed finding.
// A card referencing the same target more than once (both endpoints of every Pairs entry are also
// projected into Targets, so a Rename card always does) is indexed once per target, never once per
// occurrence — otherwise one defect reported as two identical findings against the same card.
func targetCards(plan *planparser.Plan) map[string][]planparser.Card {
	index := make(map[string][]planparser.Card)
	seen := make(map[string]map[string]bool)
	add := func(c planparser.Card, ref string) {
		id := c.ID()
		if seen[ref][id] {
			return
		}
		if seen[ref] == nil {
			seen[ref] = make(map[string]bool)
		}
		seen[ref][id] = true
		index[ref] = append(index[ref], c)
	}
	for _, c := range plan.Cards {
		for _, t := range c.Targets {
			add(c, t)
		}
		for _, u := range c.Uses {
			add(c, u)
		}
		for _, p := range c.Pairs {
			add(c, p.Old)
			add(c, p.New)
		}
	}
	return index
}

// sortedCards returns cards sorted by their own ID, for deterministic finding order.
func sortedCards(cards []planparser.Card) []planparser.Card {
	sorted := make([]planparser.Card, len(cards))
	copy(sorted, cards)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID() < sorted[j].ID() })
	return sorted
}

// statusFindings turns each of results into the finding its Status (or, absent a Status, its
// Error/Reason pre-resolution rejection) calls for, attributed to every card that references the
// target: found and multipart both pass with no finding — multipart marks one symbol the language
// lets be declared in several places, not a defect; ambiguous is the blocking finding
// glyph-ambiguous, listing every ResolveResult.Candidates entry by its ID; not_found is the
// blocking finding glyph-not-found, whose detail branches on ResolveResult.Unit — found means the
// unit is there and only the member is missing (a misspelled member), not_found means the unit
// itself is missing (a misspelled unit); and EVERY other answer is the blocking finding
// glyph-rejected — a result carrying no Status at all, which is quarry's pre-resolution rejection
// carrying Error and Reason instead, and equally a status outside quarry's four-value vocabulary,
// so the policy fails closed rather than passing an answer it has not been taught to read.
//
// This function does not special-case a Create group's targets: resolvePass excludes those before
// calling statusFindings, and create.go's createFindings handles them instead, so this policy
// stays readable on its own.
func statusFindings(plan *planparser.Plan, results []quarry.ResolveResult) []Finding {
	var findings []Finding
	index := targetCards(plan)

	for _, r := range results {
		cards := sortedCards(index[r.Target])

		switch r.Status {
		case quarry.StatusFound, quarry.StatusMultipart:
			// Both pass with no finding.
		case quarry.StatusAmbiguous:
			ids := make([]string, 0, len(r.Candidates))
			for _, cand := range r.Candidates {
				ids = append(ids, cand.ID)
			}
			for _, c := range cards {
				findings = append(findings, Finding{
					Check:    "glyph-ambiguous",
					Card:     c.ID(),
					Detail:   fmt.Sprintf("target %q is ambiguous among candidates: %s", r.Target, strings.Join(ids, ", ")),
					Severity: SeverityBlocking,
				})
			}
		case quarry.StatusNotFound:
			var detail string
			if r.Unit == quarry.StatusFound {
				detail = fmt.Sprintf("target %q's unit exists but the member is missing — check for a misspelled member", r.Target)
			} else {
				detail = fmt.Sprintf("target %q's unit does not exist — check for a misspelled unit", r.Target)
			}
			for _, c := range cards {
				findings = append(findings, Finding{
					Check:    "glyph-not-found",
					Card:     c.ID(),
					Detail:   detail,
					Severity: SeverityBlocking,
				})
			}
		default:
			// Fail closed: an absent Status is quarry's pre-resolution rejection of the target
			// string itself, and any other value is a vocabulary this package has not been taught.
			// Written as the switch's own default rather than as a pre-switch "" test, so widening
			// quarry's four-value vocabulary can never silently pass here either (crucible round
			// opus-high-r9, R9-6).
			for _, c := range cards {
				findings = append(findings, Finding{
					Check:    "glyph-rejected",
					Card:     c.ID(),
					Detail:   unreadableStatusDetail("target", r.Target, r),
					Severity: SeverityBlocking,
				})
			}
		}
	}

	return findings
}

// unreadableStatusDetail renders the glyph-rejected detail for a result whose Status neither
// statusFindings nor createFindings can read: an absent Status is quarry's pre-resolution
// rejection of the target string itself, carried by Error and Reason instead, while any other
// value is a resolve status outside the four-value vocabulary quarry documents. noun names what
// the target is to the caller ("target", "Create target", "done-check target"), so one renderer
// serves every fail-closed status policy in the package.
func unreadableStatusDetail(noun, target string, r quarry.ResolveResult) string {
	if r.Status == "" {
		return fmt.Sprintf("%s %q was rejected before resolution: error %s, reason %q", noun, target, r.Error, r.Reason)
	}
	return fmt.Sprintf("%s %q answered the unrecognized resolve status %q", noun, target, r.Status)
}
