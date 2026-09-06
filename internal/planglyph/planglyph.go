// planglyph.go declares ValidateFormat and Validate, the composed validation entry points batch 5
// names in the Gate Self-Check Parity Invariant, plus resolvePass, the resolve-backed half both
// share.

package planglyph

import (
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// ValidateFormat runs planparser.ValidateFormat's pure checks against plan, converts every
// finding, and appends the resolve-backed findings resolvePass collects on top.
func ValidateFormat(plan *planparser.Plan, worktreeRoot string) []Finding {
	findings := convertAll(planparser.ValidateFormat(plan, worktreeRoot))
	findings = append(findings, resolvePass(plan, worktreeRoot)...)
	return findings
}

// Validate runs planparser.Validate's pure checks against plan, including the plan-unapproved
// approval gate, converts every finding, and appends the same resolve-backed findings on top.
func Validate(plan *planparser.Plan, worktreeRoot string) []Finding {
	findings := convertAll(planparser.Validate(plan, worktreeRoot))
	findings = append(findings, resolvePass(plan, worktreeRoot)...)
	return findings
}

// convertAll converts every planparser.ValidationError in errs into a Finding, preserving order.
func convertAll(errs []planparser.ValidationError) []Finding {
	findings := make([]Finding, 0, len(errs))
	for _, e := range errs {
		findings = append(findings, fromValidationError(e))
	}
	return findings
}

// resolvePass is the resolve-backed half both ValidateFormat and Validate delegate to: the single
// shared body that makes the parity pair one function in every mode. Under plan.Language "none" it
// returns nil immediately, opening no repository at all — language: none degrades to today's
// path-only behaviour, with no quarry call whatsoever.
//
// Otherwise it canonicalizes every draft plan: handle first (card 19's CanonicalizeHandles), so the
// later passes — the resolve status policy (card 17's statusFindings), the Create inversion (card
// 18's createFindings), and the resolve-backed containment tier (card 20's resolveContainment) —
// see the canonical spellings rather than the draft ones, over the single batched Resolve call
// every one of those passes shares.
func resolvePass(plan *planparser.Plan, worktreeRoot string) []Finding {
	lang, ok := resolveLanguage(plan)
	if !ok {
		return nil
	}

	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return nil
	}

	targets := collectGlyphTargets(plan, lang)
	results, err := resolveTargets(repo, targets)
	if err != nil {
		return nil
	}

	handleFindings, err := CanonicalizeHandles(plan, plan.Dir, results)
	if err != nil {
		return handleFindings
	}

	current := plan
	if reloaded, rerr := planparser.ParsePlan(plan.Dir); rerr == nil {
		current = reloaded
	}

	var findings []Finding
	findings = append(findings, handleFindings...)

	createTargets := createTargetSet(current)
	var nonCreateResults []quarry.ResolveResult
	for _, r := range results {
		if createTargets[r.Target] {
			continue
		}
		nonCreateResults = append(nonCreateResults, r)
	}

	findings = append(findings, statusFindings(current, nonCreateResults)...)
	findings = append(findings, createFindings(current, results)...)
	findings = append(findings, resolveContainment(current, results)...)

	return findings
}

// resolveLanguage maps plan.Language to the glyph.Language quarry's alphabet uses, mirroring
// planparser's own unexported planLanguage exactly: "" and "go" map to glyph.Go; anything else,
// including "none", reports the not-ok second return. This is a five-line switch over the same
// public Plan.Language field every other planglyph function already reads, not a second
// implementation of any validation check.
func resolveLanguage(plan *planparser.Plan) (glyph.Language, bool) {
	switch plan.Language {
	case "", "go":
		return glyph.Go, true
	default:
		return glyph.Language(""), false
	}
}

// collectGlyphTargets returns every distinct glyph-shaped ref plan's cards reference across their
// Targets, Uses, and both Pairs endpoints, sorted for determinism. A plan: handle is excluded by
// its own literal prefix — planparser.HandlePrefix, loomyard's own grammar, never glyph grammar —
// checked first because a handle's own unit half can otherwise still parse as a (wrong) glyph
// unit. Everything else is glyph-shaped when glyph.Parse(lang, ref) succeeds — the sole grammar
// call, never a local "#"-detecting regex — so a bare path and a bare symbol are excluded by
// construction, with no shape classifier of planglyph's own.
func collectGlyphTargets(plan *planparser.Plan, lang glyph.Language) []string {
	seen := make(map[string]bool)
	add := func(raw string) {
		if strings.HasPrefix(raw, planparser.HandlePrefix) {
			return
		}
		if _, err := glyph.Parse(lang, raw); err != nil {
			return
		}
		seen[raw] = true
	}
	for _, c := range plan.Cards {
		for _, t := range c.Targets {
			add(t)
		}
		for _, u := range c.Uses {
			add(u)
		}
		for _, p := range c.Pairs {
			add(p.Old)
			add(p.New)
		}
	}

	targets := make([]string, 0, len(seen))
	for t := range seen {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	return targets
}
