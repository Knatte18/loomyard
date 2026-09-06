// drift.go implements DetectDrift: noticing when the code moved out from under the plan, and
// repairing only what quarry itself asserts. The signal is deterministic — the delta's deleted
// symbols intersected with the remaining plan's own references — and every rewrite this file
// performs is gated twice before it ever touches a byte: a rename matching a declared Rename
// card's own pair is that card's expected outcome, never drift, and a renamed symbol nothing in
// the plan references is logged only. Only the exact tier auto-repairs; the evidence tier (card
// 37) never calls RewriteRefs or AppendAmendment at all.

package planglyph

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// renameCardPairs indexes every declared Rename card's own Old->New pair, plan-wide, so
// DetectDrift's gate one can recognize a rename that is a card's own expected outcome.
func renameCardPairs(plan *planparser.Plan) map[string]string {
	pairs := make(map[string]string)
	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != planparser.CardTypeRename {
				continue
			}
			for _, p := range g.Pairs {
				pairs[p.Old] = p.New
			}
		}
	}
	return pairs
}

// driftRepair is one exact-tier auto-repair DetectDrift queues: the old->new substitution and the
// cards that referenced the old glyph, so exactly one Amendment can be appended per repair once the
// plan-wide rewrite and revalidation both succeed.
type driftRepair struct {
	oldID, newID string
	cards        []planparser.Card
}

// DetectDrift computes its signal deterministically — the delta's deleted symbols intersected with
// the remaining plan's own references — and applies two gates before any rewrite. Gate one: a
// rename that matches a declared Rename card's own pair is that card's expected outcome, never
// drift, and produces no finding and no repair. Gate two: a renamed symbol nothing in the
// remaining plan references is logged only, never rewritten.
//
// Past the gates, an exact-tier detection — an entry of delta.Renamed, which quarry populates only
// under its own AST-exact conditions with no threshold — auto-repairs: the old->new substitution is
// applied plan-wide through one planparser.RewriteRefs call, the reloaded plan is revalidated with
// one batched resolve against worktreeRoot, and exactly one planparser.Amendment is appended per
// repair, carrying now, the (first, sorted) referencing card, the old and new glyph, the tier word
// "exact", and sha. Auto-repairing only the tier quarry itself asserts is what keeps loomyard from
// deciding what quarry deliberately returns as undecided.
//
// worktreeRoot is not part of this function's signature in the batch plan's own prose, which
// describes only the mechanical rewrite; it is added here because "revalidate with one batched
// resolve" is not otherwise performable — planglyph never derives a worktree root of its own (the
// told-geometry-for-planglyph Shared Decision), so the caller must tell it exactly as every other
// entry point in this package already does.
//
// A deleted symbol the remaining plan still references with no rename pair at all is a blocking
// finding, check ID plan-references-deleted-symbol. This never collides with the exact-tier
// handling above: quarry's own delta engine removes an exact pair's constituents from Deleted
// entirely, so a symbol reaching this check was never renamed under quarry's own AST-exact
// conditions.
//
// now and sha are taken as parameters rather than read from a clock or a repository inside this
// function, so the whole detector is deterministic and testable without a fixture.
func DetectDrift(plan *planparser.Plan, planDir, worktreeRoot string, delta quarry.GitDeltaAnswer, sha, now string) ([]Finding, error) {
	renamePairs := renameCardPairs(plan)
	refCards := targetCards(plan)

	var findings []Finding
	subs := make(map[string]string)
	var repairs []driftRepair

	for _, rp := range delta.Renamed {
		oldID, newID := rp.From.ID, rp.To.ID

		if want, ok := renamePairs[oldID]; ok && want == newID {
			continue // Gate one: the declared Rename card's own expected outcome.
		}

		cards := refCards[oldID]
		if len(cards) == 0 {
			logger.Debug("planglyph: renamed symbol has no plan reference; logging only, no repair", "old_id", oldID, "new_id", newID)
			continue // Gate two: nothing in the remaining plan references it.
		}

		subs[oldID] = newID
		repairs = append(repairs, driftRepair{oldID: oldID, newID: newID, cards: cards})
	}

	for _, s := range delta.Deleted {
		cards := refCards[s.ID]
		if len(cards) == 0 {
			continue
		}
		for _, c := range sortedCards(cards) {
			findings = append(findings, Finding{
				Check:    "plan-references-deleted-symbol",
				Card:     cardIDOf(c),
				Detail:   fmt.Sprintf("card %d references %q, which the delta reports deleted with no corresponding rename", c.Number, s.ID),
				Severity: SeverityBlocking,
			})
		}
	}

	// Evidence-tier candidates: gate two applies here too — a deleted symbol nothing in the
	// remaining plan references gets no finding, exactly as an unreferenced exact-tier rename gets
	// none above. This tier never calls RewriteRefs or AppendAmendment: an amendment records a
	// repair, and no repair happens here — the rename-versus-genuine-delete decision is the
	// reviewer's, never the pipeline's. Candidate order is quarry's own deterministic ordering,
	// carried through unchanged: never re-sorted, re-ranked, or truncated.
	for _, entry := range delta.RenameCandidates {
		cards := refCards[entry.ID]
		if len(cards) == 0 {
			continue
		}

		var candidateParts []string
		for _, cand := range entry.Candidates {
			candidateParts = append(candidateParts, fmt.Sprintf(
				"%s (file=%s, signature_identical_modulo_name=%v, body_token_similarity=%.4f, body_tokens_before=%d, body_tokens_after=%d, doc_identical=%v)",
				cand.ID, cand.File, cand.Signals.SignatureIdenticalModuloName, cand.Signals.BodyTokenSimilarity,
				cand.Signals.BodyTokensBefore, cand.Signals.BodyTokensAfter, cand.Signals.DocIdentical,
			))
		}

		for _, c := range sortedCards(cards) {
			findings = append(findings, Finding{
				Check: "rename-candidate",
				Card:  cardIDOf(c),
				Detail: fmt.Sprintf(
					"card %d references %q, deleted with %d evidence-tier rename candidate(s) — mechanical evidence only, the rename-versus-genuine-delete decision is the reviewer's, never the pipeline's: %s",
					c.Number, entry.ID, len(entry.Candidates), strings.Join(candidateParts, "; "),
				),
				Severity: SeverityInformational,
			})
		}
	}

	if len(subs) == 0 {
		return findings, nil
	}

	if err := planparser.RewriteRefs(planDir, subs); err != nil {
		return findings, err
	}

	reloaded, err := planparser.ParsePlan(planDir)
	if err != nil {
		return findings, err
	}
	if lang, ok := resolveLanguage(reloaded); ok {
		repo, err := openRepo(worktreeRoot)
		if err != nil {
			return findings, err
		}
		if _, err := resolveTargets(repo, collectGlyphTargets(reloaded, lang)); err != nil {
			return findings, err
		}
	}

	for _, r := range repairs {
		sorted := sortedCards(r.cards)
		card := ""
		if len(sorted) > 0 {
			card = cardIDOf(sorted[0])
		}
		if err := planparser.AppendAmendment(planDir, planparser.Amendment{
			Timestamp: now,
			Card:      card,
			OldGlyph:  r.oldID,
			NewGlyph:  r.newID,
			Tier:      "exact",
			SHA:       sha,
		}); err != nil {
			return findings, err
		}
	}

	return findings, nil
}
