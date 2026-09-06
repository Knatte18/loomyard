// planglyph.go declares ValidateFormat and Validate, the composed validation entry points batch 5
// names in the Gate Self-Check Parity Invariant, plus resolvePass, the resolve-backed half both
// share.

package planglyph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// ValidateFormat runs planparser.ValidateFormat's pure checks against plan, converts every
// finding, and appends the resolve-backed findings resolvePass collects on top. Its second return
// is a non-nil ErrQuarryUnavailable-wrapped error exactly when resolvePass could not get a clean
// answer from quarry — the pure findings already collected are still returned alongside it, since
// the gate cannot certify a plan as valid against code it failed to read, and the error must
// report as a gate/infrastructure failure rather than as a plan finding so nobody mistakes "quarry
// broke" for "the plan is wrong".
func ValidateFormat(plan *planparser.Plan, worktreeRoot string) ([]Finding, error) {
	findings := convertAll(planparser.ValidateFormat(plan, worktreeRoot))
	resolveFindings, err := resolvePass(plan, worktreeRoot)
	findings = append(findings, resolveFindings...)
	return findings, err
}

// Validate runs planparser.Validate's pure checks against plan, including the plan-unapproved
// approval gate, converts every finding, and appends the same resolve-backed findings on top,
// with the same error contract ValidateFormat documents.
func Validate(plan *planparser.Plan, worktreeRoot string) ([]Finding, error) {
	findings := convertAll(planparser.Validate(plan, worktreeRoot))
	resolveFindings, err := resolvePass(plan, worktreeRoot)
	findings = append(findings, resolveFindings...)
	return findings, err
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
// returns nil findings and a nil error immediately, opening no repository at all — language: none
// degrades to today's path-only behaviour, with no quarry call whatsoever.
//
// Otherwise it canonicalizes every draft plan: handle first (card 19's CanonicalizeHandles), so the
// later passes — the resolve status policy (card 17's statusFindings), the Create inversion (card
// 18's createFindings), and the resolve-backed containment tier (card 20's resolveContainment) —
// see the canonical spellings rather than the draft ones, over the single batched Resolve call
// every one of those passes shares.
//
// Its second return is a non-nil, ErrQuarryUnavailable-wrapped error whenever openRepo,
// resolveTargets, or CanonicalizeHandles' own use of RewriteRefs fails — a category distinct from
// every per-target verdict those passes report, so a quarry outage or a disk failure is never
// mistaken for a clean answer.
func resolvePass(plan *planparser.Plan, worktreeRoot string) ([]Finding, error) {
	lang, ok := resolveLanguage(plan)
	if !ok {
		return nil, nil
	}

	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return nil, err
	}

	targets := collectGlyphTargets(plan, lang)
	results, err := resolveTargets(repo, targets)
	if err != nil {
		return nil, err
	}

	handleFindings, err := CanonicalizeHandles(plan, plan.Dir, results)
	if err != nil {
		return handleFindings, fmt.Errorf("%w: canonicalize handles: %v", ErrQuarryUnavailable, err)
	}

	// The reload is not optional and its failure is not recoverable: CanonicalizeHandles has just
	// rewritten planDir, so a plan that no longer parses means the three passes below would run
	// against the stale in-memory copy and report a clean verdict over bytes that are no longer on
	// disk -- the "a plan looks validated and was not" failure mode repo.go's own ErrQuarryUnavailable
	// rationale names as rejected. It reports as an infrastructure failure rather than a plan finding
	// for the same reason: the gate could not read the artifact, it did not find a defect in it.
	current, rerr := planparser.ParsePlan(plan.Dir)
	if rerr != nil {
		return handleFindings, fmt.Errorf("%w: re-parse plan after handle canonicalization: %v", ErrQuarryUnavailable, rerr)
	}

	findings := append([]Finding{}, handleFindings...)

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

	return findings, nil
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
