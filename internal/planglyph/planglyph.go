// planglyph.go declares ValidateFormat and Validate, the composed validation entry points batch 5
// names in the Gate Self-Check Parity Invariant, plus resolvePass, the resolve-backed half both
// share.

package planglyph

import (
	"fmt"
	"sort"

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
//
// This is the WHOLE-plan form, correct before execution starts (loom's Plan-Validate row and the
// standalone verbs). Once execution is under way, use ValidateDispatch instead.
func ValidateFormat(plan *planparser.Plan, worktreeRoot string) ([]Finding, error) {
	findings := convertAll(planparser.ValidateFormat(plan, worktreeRoot))
	resolveFindings, err := resolvePass(plan, worktreeRoot, nil)
	findings = append(findings, resolveFindings...)
	return findings, err
}

// Validate runs planparser.Validate's pure checks against plan, including the plan-unapproved
// approval gate, converts every finding, and appends the same resolve-backed findings on top,
// with the same error contract ValidateFormat documents.
func Validate(plan *planparser.Plan, worktreeRoot string) ([]Finding, error) {
	findings := convertAll(planparser.Validate(plan, worktreeRoot))
	resolveFindings, err := resolvePass(plan, worktreeRoot, nil)
	findings = append(findings, resolveFindings...)
	return findings, err
}

// ValidateDispatch is ValidateFormat's mid-execution form: the same check set, scoped to the cards
// whose work has NOT landed yet. completed names every card already built, and a caller with none
// gets exactly ValidateFormat's answer.
//
// The scoping is not an optimisation, it is correctness. A plan describes intended change, so a card
// whose work already landed necessarily contradicts the tree it is re-resolved against: a completed
// Delete target reads glyph-not-found, a completed Rename's Old side reads glyph-not-found and
// leaves CanonicalizeHandles nothing to derive its to-side declaration from, and a completed Create
// target reads create-already-exists under the Create inversion. Every one of those is the plan
// working exactly as designed, reported as a blocking defect. Re-resolving the whole plan on every
// batch therefore wedged any multi-batch plan carrying a Create, Delete or Rename card at its second
// batch.
//
// The two halves are scoped differently, deliberately. The resolve-backed pass runs over the pending
// cards ALONE, so a completed card's targets are never resolved and never paired against a pending
// card for containment — there is no race left to prevent with work that already landed. The pure
// pass runs over the WHOLE plan and has only its card-scoped findings dropped, because several pure
// checks are plan-level and would misreport against a filtered plan: index-file-mismatch would see
// every completed card's file as orphaned, card-numbering would see gaps, and path-missing's
// satisfied-by-another-card union would lose the Create and Rename destinations completed cards
// contribute to still-pending ones.
func ValidateDispatch(plan *planparser.Plan, worktreeRoot string, completed []planparser.Card) ([]Finding, error) {
	done := cardIDSet(completed)

	var findings []Finding
	for _, f := range convertAll(planparser.ValidateFormat(plan, worktreeRoot)) {
		if f.Card != "" && done[f.Card] {
			continue
		}
		findings = append(findings, f)
	}

	resolveFindings, err := resolvePass(plan, worktreeRoot, done)
	findings = append(findings, resolveFindings...)
	return findings, err
}

// cardIDSet indexes cards by their own ID.
func cardIDSet(cards []planparser.Card) map[string]bool {
	set := make(map[string]bool, len(cards))
	for _, c := range cards {
		set[c.ID()] = true
	}
	return set
}

// PendingPlan returns the view of plan carrying only the cards whose work has not landed yet: a
// shallow copy with completed cards dropped from Cards, or plan itself when none are completed.
// Every other field — Dir, Format, Language, Approved, the overview's own sections — is shared
// unchanged, since none of them is per-card.
//
// It is exported because websterengine's record-batch boundary needs the same view for drift
// detection that ValidateDispatch builds for its own resolve pass: DetectDrift's whole signal is
// "the delta's deleted symbols intersected with the REMAINING plan's own references", and a card
// that already ran is not remaining. Without it a Delete card's own batch could never be recorded —
// the card references the symbol it just deleted, so its own success reported as
// plan-references-deleted-symbol.
func PendingPlan(plan *planparser.Plan, completed []planparser.Card) *planparser.Plan {
	return pendingCardsByID(plan, cardIDSet(completed))
}

// pendingCardsByID is PendingPlan over an already-built card-ID set, so resolvePass can re-scope a
// plan it reloaded from disk without rebuilding the set.
func pendingCardsByID(plan *planparser.Plan, done map[string]bool) *planparser.Plan {
	if len(done) == 0 {
		return plan
	}
	scoped := *plan
	kept := make([]planparser.Card, 0, len(plan.Cards))
	for _, c := range plan.Cards {
		if done[c.ID()] {
			continue
		}
		kept = append(kept, c)
	}
	scoped.Cards = kept
	return &scoped
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
// Its second return is a non-nil error whenever the pass could not reach a verdict at all — a
// category distinct from every per-target verdict those passes report, so an outage is never
// mistaken for a clean answer. It is ErrQuarryUnavailable-wrapped when QUARRY is what could not
// answer (openRepo, resolveTargets); a failure to rewrite or re-read the PLAN on disk is returned
// unwrapped, naming the plan directory, because calling that a quarry outage sends an operator at
// the wrong subsystem.
//
// done is the set of already-built card IDs ValidateDispatch supplies, empty for the whole-plan
// entry points. Every pass below sees only the pending cards, so a card whose work already landed is
// never resolved against a tree it deliberately changed. planDir is still the whole plan's
// directory: RewriteRefs re-parses it itself, so a handle bound in a pending card is still spelled
// consistently across every card file, including the completed ones.
func resolvePass(plan *planparser.Plan, worktreeRoot string, done map[string]bool) ([]Finding, error) {
	lang, ok := plan.GlyphLanguage()
	if !ok {
		return nil, nil
	}

	planDir := plan.Dir
	pending := pendingCardsByID(plan, done)

	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return nil, err
	}

	targets := collectGlyphTargets(pending, lang)
	results, err := resolveTargets(repo, targets)
	if err != nil {
		return nil, err
	}

	handleFindings, rewrote, err := CanonicalizeHandles(pending, planDir, results)
	if err != nil {
		// NOT wrapped in ErrQuarryUnavailable: CanonicalizeHandles' error comes from
		// planparser.RewriteRefs, which parses the plan and writes card files, so a read-only
		// _lyx/plan or a card file deleted mid-run was reported to the operator as "quarry could not
		// answer" and sent them at the wrong subsystem entirely (crucible round opus-medium-r6,
		// R6-12). Both callers' non-quarry branch already fails the gate with an accurate,
		// plan-named message, which is the right disposition for a plan artifact lyx could not
		// rewrite.
		return handleFindings, fmt.Errorf("canonicalize handles in plan %s: %w", planDir, err)
	}

	// The reload happens only when canonicalization actually rewrote the plan on disk -- otherwise
	// there is nothing new to read and the in-memory plan is already current.
	//
	// When it does happen its failure is NOT recoverable. Falling back to the stale in-memory copy
	// would run the three passes below against bytes that are no longer on disk and report a clean
	// verdict over them: the "a plan looks validated and was not" failure mode repo.go's own
	// ErrQuarryUnavailable rationale names as deliberately rejected. It reports as an infrastructure
	// failure rather than a plan finding for the same reason -- the gate could not read the artifact,
	// it did not find a defect in it.
	current := pending
	if rewrote {
		reloaded, rerr := planparser.ParsePlan(planDir)
		if rerr != nil {
			// Same reasoning as the RewriteRefs failure above: this is the PLAN that could not be
			// read back, not quarry that could not answer.
			return handleFindings, fmt.Errorf("re-parse plan %s after handle canonicalization: %w", planDir, rerr)
		}
		current = pendingCardsByID(reloaded, done)
	}

	findings := append([]Finding{}, handleFindings...)

	// Rename pairs' New sides are excluded from the status policy exactly as Create targets are,
	// and for the same reason: both name something that only exists AFTER the card runs. A symbol
	// rename's New side is a plan: handle and never reaches the resolve set at all, but a
	// FILE-rename pair's New side is a self glyph of the destination file, which resolves
	// not_found against the pre-rename tree — the pure layer's path-missing check deliberately
	// never checks Pairs.New (and satisfies later refs via its renameTargetsUnion), so reporting
	// blocking glyph-not-found here wedged every plan carrying a file rename.
	createTargets := createTargetSet(current)
	renameNewTargets := renameNewTargetSet(current)
	var nonCreateResults []quarry.ResolveResult
	for _, r := range results {
		if createTargets[r.Target] || renameNewTargets[r.Target] {
			continue
		}
		nonCreateResults = append(nonCreateResults, r)
	}

	// The Create inversion needs one answer per Create target INCLUDING the handle-shaped ones, whose
	// expected glyph only became knowable once CanonicalizeHandles computed it above — so its index
	// is the batched glyph answers plus one further batched call for the handles, keyed by the ref
	// each card actually spells.
	createIndex := resultByTarget(results)
	handleResults, err := createHandleResults(repo, current)
	if err != nil {
		return findings, err
	}
	for handle, r := range handleResults {
		createIndex[handle] = r
	}

	findings = append(findings, statusFindings(current, nonCreateResults)...)
	findings = append(findings, createFindings(current, createIndex)...)
	findings = append(findings, resolveContainment(current, results)...)

	return findings, nil
}

// renameNewTargetSet returns the set of every Rename pair's New-side ref across plan, matching the
// spelling the card carries. resolvePass excludes these from statusFindings, mirroring the pure
// layer's own rule that a Rename's New side is never existence-checked (planparser's path-missing
// check skips Pairs.New): the destination only exists once the rename lands, so a not_found answer
// against the pre-rename tree is the plan working as designed, not a defect.
func renameNewTargetSet(plan *planparser.Plan) map[string]bool {
	set := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			set[p.New] = true
		}
	}
	return set
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
		if planparser.IsHandleRef(raw) {
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
