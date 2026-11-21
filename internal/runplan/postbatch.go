// postbatch.go holds the post-batch checks every terminal batch owes.
// They are the done checks, the re-sign rebind, handle binding and the glyph scope guard.
// They also include drift detection with the renames that rewrite the cards not yet begun.

package runplan

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrCardNotDone is the sentinel the post-batch checks return when a done-check reports a blocking finding.
// Such a finding concerns the batch's own cards.
// It is a Create target that still does not resolve, or a Delete target that still does.
// The implementer turns it into a failed batch, with the findings as its reasons.
var ErrCardNotDone = errors.New("runplan: the post-batch done-checks reported a blocking finding")

// ErrLaterTargetRemoved is the sentinel the post-batch checks return for a removed symbol a later card names.
// A batch causes it by removing that symbol.
// The batch is handed back to its attempt once, while that attempt still holds the batch's context.
var ErrLaterTargetRemoved = errors.New("runplan: the batch removed a symbol a later card names")

// LaterTargetRemovedError is the refusal the post-batch checks return, once per bracket.
// It covers a batch that removes a symbol a later card names without reporting a rename.
// The batch stays non-terminal with its report in place.
// The next record of the batch records it instead of refusing again.
type LaterTargetRemovedError struct {
	// Batch names the batch as NN-<slug>.
	Batch string
	// Number is the batch's number.
	Number int
	// Reasons holds one line per later card and removed symbol.
	Reasons []string
	// WayForward is the implementer's way forward through the batch's attempt.
	WayForward string
}

// Error names each removed symbol and the later card that names it, then the way forward.
func (e *LaterTargetRemovedError) Error() string {
	return fmt.Sprintf("%s: batch %s: %s; way forward: %s", ErrLaterTargetRemoved, e.Batch, strings.Join(e.Reasons, "; "), e.WayForward)
}

// Unwrap returns the later-target-removed sentinel, so errors.Is matches it.
func (e *LaterTargetRemovedError) Unwrap() error { return ErrLaterTargetRemoved }

// CardNotDoneError is the error the post-batch done-checks return, and it unwraps to ErrCardNotDone.
// Its message carries the findings' text.
// The flags record which kinds of finding it holds, so the implementer can word the way forward.
type CardNotDoneError struct {
	// Reasons is each finding's text.
	Reasons []string
	// DeleteNotDone is set for a Delete target that still resolves.
	DeleteNotDone bool
	// GlyphRejected is set when quarry rejects a card's target before resolving it.
	GlyphRejected bool
	// CreateNotDone is set for a Create target that still does not resolve, or for a Rename pair that did not happen.
	CreateNotDone bool
	// DroppableCreate is set for a Create target that still does not resolve.
	// A decision can drop such a target, and it cannot drop a Rename pair.
	DroppableCreate bool
	// SignatureMismatch is set when a target's member is declared only with another parameter list.
	SignatureMismatch bool
}

func (e *CardNotDoneError) Error() string {
	return fmt.Sprintf("%s: %s", ErrCardNotDone, strings.Join(e.Reasons, "; "))
}

func (e *CardNotDoneError) Unwrap() error { return ErrCardNotDone }

// LaterDeleteReferenceReasons returns one reason per blocking reference to a symbol the cards in own delete.
// The reference comes from an unbegun card.
// batches is the run's recorded partition mapped onto the current plan, so the unbegun cards are the pending ones.
// A method matched by name alone is informational and gives no reason:
// its receiver is not resolved, and the later card's verify build catches a real call.
// An infrastructure error is wrapped in planindex.ErrQuarryUnavailable.
func LaterDeleteReferenceReasons(plan *planparser.Plan, batches []batcher.Batch, st *State, own []planparser.Card, geom Geometry) ([]string, error) {
	index, err := geom.index()
	if err != nil {
		return nil, err
	}
	findings, err := index.LaterDeleteReferences(plan, own, UnbegunCards(batches, st), geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	reasons := make([]string, 0, len(findings))
	for _, f := range findings {
		if f.Severity == planindex.SeverityBlocking {
			reasons = append(reasons, f.Error())
		}
	}
	return reasons, nil
}

// PostBatchInputs carries everything the post-batch checks need.
type PostBatchInputs struct {
	// Plan is the parsed plan the checks read and rewrite.
	Plan *planparser.Plan
	// State is the run-plan state the plan re-baselines record into.
	State *State
	// Batch is the record being finished; a removed later target sets its LaterTargetsReturned.
	Batch *BatchState
	// Geom is the told geometry the checks read the worktree, plan and run directories from.
	Geom Geometry
	// Batches is the run's recorded partition mapped onto the current plan, in execution order.
	// Drift detection scopes to the cards of the batches not yet terminal, this one excluded.
	Batches []batcher.Batch
	// Number is the batch's number, and Cards its own cards.
	Number int
	Cards  []planparser.Card
	// StartSHA is the bracket record's captured start SHA and HeadSHA the reconciled report head.
	StartSHA string
	HeadSHA  string
	// DecisionRecordPath is the run's decision record, whose dropped Create targets skip their done-check.
	// Empty reads no decisions.
	DecisionRecordPath string
	// Label names the batch in warnings.
	Label string
	// Renames are the glyph renames the batch's report lists, which drift detection repairs in the cards not yet begun.
	Renames []ReportRename
	// HandBackRemovedTargets asks the checks to refuse with *LaterTargetRemovedError for a removed later target.
	// The refusal applies when the batch removed a symbol a later card names.
	// The refusal comes before any plan rewrite past the re-sign rebind.
	HandBackRemovedTargets bool
	// HandBackWayForward is the way forward that refusal names.
	HandBackWayForward string
	// RecordWarning records a later card's drift finding once on the batch, keyed by id.
	// It returns the warning's text and whether it was newly recorded.
	RecordWarning func(id, class, detail string) (text string, added bool)
}

// Bind reasons that PostBatchChecks decides itself.
const (
	// bindCardNotDone marks a handle of a card the done-checks reported findings for.
	bindCardNotDone planindex.BindReason = "card-not-done"
	// bindBatchNotDone marks a handle of a card whose batch-mate failed the done-checks, so the pass never bound it.
	bindBatchNotDone planindex.BindReason = "batch-not-done"
	// bindDeltaUnavailable marks a handle the pass could not bind because the batch's delta was unavailable.
	bindDeltaUnavailable planindex.BindReason = "delta-unavailable"
	// bindLanguageNone marks a handle of a plan whose language opts out of glyphs.
	bindLanguageNone planindex.BindReason = "language-none"
)

// failedBindOutcomes returns one failed outcome for each handle the cards own, in card and body order.
// reasonFor picks the reason each card's handles fail with.
func failedBindOutcomes(cards []planparser.Card, reasonFor func(planparser.Card) planindex.BindReason) []planindex.BindOutcome {
	var outcomes []planindex.BindOutcome
	for _, c := range cards {
		for _, handle := range planparser.OwnedHandles(c) {
			outcomes = append(outcomes, planindex.BindOutcome{Card: c.Number, Handle: handle, Reason: reasonFor(c)})
		}
	}
	return outcomes
}

// logBindOutcomes writes one Info line per handle outcome, so the bind rate is countable by grep.
func logBindOutcomes(batch int, outcomes []planindex.BindOutcome) {
	for _, o := range outcomes {
		if o.Reason == "" {
			logger.Info("webster: handle bind", "batch", batch, "card", o.Card, "handle", o.Handle, "outcome", "bound", "glyph", o.Glyph)
			continue
		}
		logger.Info("webster: handle bind", "batch", batch, "card", o.Card, "handle", o.Handle, "outcome", "failed", "reason", string(o.Reason))
	}
}

// CardNotDoneFor returns the *CardNotDoneError that findings make against batch number's own cards.
// It returns nil when findings is empty.
// It logs every handle of the batch as failed,
// so the bind trace names the card that was not done and the batch-mates held back with it.
func CardNotDoneFor(number int, cards []planparser.Card, findings []planindex.Finding) *CardNotDoneError {
	if len(findings) == 0 {
		return nil
	}
	notDone := &CardNotDoneError{}
	notDoneCards := map[string]bool{}
	for _, f := range findings {
		notDone.Reasons = append(notDone.Reasons, f.Error())
		notDoneCards[f.Card] = true
		switch f.Check {
		case "delete-not-done":
			notDone.DeleteNotDone = true
		case "glyph-rejected":
			notDone.GlyphRejected = true
		case "create-not-done":
			notDone.CreateNotDone = true
			notDone.DroppableCreate = true
		case "rename-not-done":
			notDone.CreateNotDone = true
		case "signature-mismatch":
			notDone.SignatureMismatch = true
		}
	}
	logBindOutcomes(number, failedBindOutcomes(cards, func(c planparser.Card) planindex.BindReason {
		if notDoneCards[c.ID()] {
			return bindCardNotDone
		}
		return bindBatchNotDone
	}))
	return notDone
}

// rereadCards returns cards as plan holds them now, matched by card ID; a card plan no longer holds stays as given.
func rereadCards(plan *planparser.Plan, cards []planparser.Card) []planparser.Card {
	reread := make([]planparser.Card, len(cards))
	for i, c := range cards {
		reread[i] = c
		for _, current := range plan.Cards {
			if current.ID() == c.ID() {
				reread[i] = current
				break
			}
		}
	}
	return reread
}

// withoutDroppedCreates returns cards with every Create target in dropped removed from its target groups.
// A dropped target that a card declares through a `plan:` handle loses its declaration too.
// The card then owns no handle for it.
// Each such target adds a warning naming the card, the target and the decision that dropped it.
// A card it changes is a copy: the caller's cards and their slices stay as given.
func withoutDroppedCreates(cards []planparser.Card, dropped map[string]string) (kept []planparser.Card, warnings []string) {
	kept = slices.Clone(cards)
	for i, c := range kept {
		groups := slices.Clone(c.TargetGroups)
		droppedHandles := map[string]bool{}
		for j, g := range groups {
			if g.Type != planparser.CardTypeCreate {
				continue
			}
			var refs []string
			for _, ref := range g.Refs {
				title, isDropped := dropped[ref]
				if !isDropped {
					refs = append(refs, ref)
					continue
				}
				droppedHandles[ref] = true
				warnings = append(warnings, fmt.Sprintf("card %s: Create target %q is dropped by the decision %q, so its done-check did not run", c.ID(), ref, title))
			}
			groups[j].Refs = refs
			groups[j].Declarations = declarationsWithout(g.Declarations, droppedHandles)
		}
		kept[i].TargetGroups = groups
		kept[i].Declarations = declarationsWithout(c.Declarations, droppedHandles)
	}
	return kept, warnings
}

// declarationsWithout returns a copy of declarations without those whose handle is in handles.
func declarationsWithout(declarations []planparser.CardDeclaration, handles map[string]bool) []planparser.CardDeclaration {
	return slices.DeleteFunc(slices.Clone(declarations), func(d planparser.CardDeclaration) bool { return handles[d.Handle] })
}

// PostBatchChecks runs the mechanical pass every terminal batch owes, whichever path takes it terminal.
// It runs the card done-checks and carries a recorded C# re-sign into the plan.
// The done-checks skip each Create target that an `Added after Discussion` decision in in.DecisionRecordPath drops,
// and a warning names each such target.
// Binding owns no handle for a dropped target either; drift detection and the scope guard still read the cards as given.
// A decision record that cannot be read is an error with a way forward.
// It computes the single delta that the rest of the pass shares.
// It then binds handles, runs the glyph scope guard, and runs drift detection with its exact-tier auto-repair.
// It also runs the plan-staleness re-baseline each of those rewrites requires.
//
// The scope guard's findings come back as deviations.
// They are the deduplicated, sorted files the batch changed outside its targets.
// They are informational and never join the warnings.
// When the delta cannot be computed, there are no deviations, and the could-not-run warning is returned instead.
//
// A batch that reached done through a recovery is exactly as done as one its first attempt finished:
// its Create targets have to resolve, its plan: handles have to bind,
// and the drift its work caused has to be repaired or reported.
// Skipping this pass for a recovered batch would leave its plan: handles unbound.
// Every later card would then reference an unbound handle.
// Drift detection would not see those references, since its reference index keys on the ref as the card spells it.
//
// Findings about the batch's own cards (done-checks, blocking bind findings) come back as an error.
// That error wraps ErrCardNotDone.
// Drift findings concern later cards, so they become `later card:` warnings recorded once through in.RecordWarning.
// Informational findings ride out on warnings too.
// Warnings come back alongside any error, so a caller never loses them.
func PostBatchChecks(in PostBatchInputs) (deviations, warnings []string, err error) {
	// A card's completion has a mechanical verdict.
	// A Create target that still does not resolve, or a Delete target that still does, blocks the card.
	// This runs its own batched Resolve against the post-card tree, distinct from the single delta call below.
	index, err := in.Geom.index()
	if err != nil {
		return nil, nil, err
	}
	dropped, err := discussionparser.DroppedCreates(in.DecisionRecordPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read the dropped Create targets of the decision record %s: %w; way forward: restore the decision record at that path, or have the run's parent replace it with `lym loom decision replace`, then re-run the verb that failed", in.DecisionRecordPath, err)
	}
	cardsToCheck, droppedWarnings := withoutDroppedCreates(in.Cards, dropped)
	// Every return below carries the dropped-target warnings, so a refusal never hides them.
	defer func() { warnings = append(droppedWarnings, warnings...) }()
	doneFindings, err := index.DoneChecks(in.Plan, cardsToCheck, in.Geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}
	if notDone := CardNotDoneFor(in.Number, in.Cards, doneFindings); notDone != nil {
		return nil, nil, notDone
	}

	// A recorded C# re-sign is carried into the plan once its edit is in the tree:
	// every later card naming the member by its old glyph then names the minted id.
	// This rewrite runs before the delta, so everything after it reads the re-read plan,
	// and the plan is re-baselined before the guard and drift detection run.
	fingerprintBefore, err := Fingerprint(in.Geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	resignFindings, err := index.RebindResigns(in.Plan, in.Cards, in.Geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}
	if notDone := CardNotDoneFor(in.Number, in.Cards, resignFindings); notDone != nil {
		return nil, nil, notDone
	}
	fingerprintAfter, err := Fingerprint(in.Geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	recordedCards := in.Cards
	if fingerprintAfter != fingerprintBefore {
		if err := RestampFingerprint(in.State, in.Geom.PlanDir, in.Geom.RunDir); err != nil {
			return nil, nil, err
		}
		reparsed, err := in.Plan.Reparse()
		if err != nil {
			return nil, nil, fmt.Errorf("re-parse plan %s after re-sign rebinding: %w", in.Geom.PlanDir, err)
		}
		in.Plan = reparsed
		in.Cards = rereadCards(reparsed, in.Cards)
	}
	completed := CompletedCards(in.Batches, in.State, in.Number)

	// One planindex.Delta serves the whole batch: BindHandles, ScopeGuard and DetectDrift all consume it.
	// HeadSHA is the reconciled report head, which the caller has already cross-checked against the worktree's real HEAD.
	// That cross-check is why the delta can be trusted here and nowhere earlier.
	// A DeltaGit infrastructure error does not abort the sequence.
	// The scope guard degrades to an informational notice on this same deltaErr.
	// The done-checks above ran on their own Resolve and are unaffected.
	// delta is the zero value on error, so BindHandles cannot confirm any handle bound.
	// It reports bind-count-mismatch for every card that declared one.
	// An unconfirmed Create is exactly a not-done card.
	// The floor error of a git below lym's minimum is not a quarry degrade,
	// so it stops the batch here like any other non-quarry error.
	delta, deltaErr := index.Delta(in.Geom.WorktreeRoot, in.StartSHA, in.HeadSHA)
	if deltaErr != nil && !errors.Is(deltaErr, planindex.ErrQuarryUnavailable) {
		return nil, nil, deltaErr
	}

	// The report's renames are what drift detection repairs further down,
	// so a removal the batch reported is no hand-back.
	reported := make([]planindex.Rename, len(in.Renames))
	for i, r := range in.Renames {
		reported[i] = planindex.Rename{Old: r.Old, New: r.New}
	}

	// A removed later target is handed back before binding,
	// so the plan carries no bind or drift rewrite of a batch its attempt is still to change.
	if in.HandBackRemovedTargets && deltaErr == nil && !in.Batch.LaterTargetsReturned {
		if removed := delta.RemovedTargets(in.Plan, completed, in.Cards, reported); len(removed) > 0 {
			reasons := make([]string, len(removed))
			for i, f := range removed {
				reasons[i] = f.Error()
			}
			in.Batch.LaterTargetsReturned = true
			return nil, nil, &LaterTargetRemovedError{Batch: in.Label, Number: in.Number, Reasons: reasons, WayForward: in.HandBackWayForward}
		}
	}

	// Binding runs after the done-checks above, so a card that already failed create-not-done is never bound.
	// It applies its whole batch of substitutions in one RewriteRefs call.
	// A dropped Create declares nothing the delta could create, so its handle is left out of the cards binding reads.
	bindCards, _ := withoutDroppedCreates(in.Cards, dropped)
	bindFindings, bindOutcomes, bindErr := delta.BindHandles(in.Plan, in.Geom.PlanDir, bindCards)
	// The trace is logged before the restamp below, so a restamp failure still reports the bind that landed.
	// The trace names the reason the pass decided.
	// That reason overrides the outcomes BindHandles computed when there is no glyph language or no delta.
	if bindErr == nil {
		if !in.Plan.GlyphLanguage() {
			bindOutcomes = failedBindOutcomes(in.Cards, func(planparser.Card) planindex.BindReason { return bindLanguageNone })
		} else if deltaErr != nil {
			bindOutcomes = failedBindOutcomes(in.Cards, func(planparser.Card) planindex.BindReason { return bindDeltaUnavailable })
		}
		logBindOutcomes(in.Number, bindOutcomes)
	}
	// BindHandles rewrites plan references on disk before it reports a finding or an error.
	// The substitutions come from delta.Created;
	// a bind-count mismatch comes from a card whose handle matched nothing.
	// One call can therefore do both.
	// The staleness re-baseline runs here, not once after every refusal below.
	// A restamp failure never masks bindErr: the caller is already returning for that reason.
	if rebaseErr := RestampFingerprint(in.State, in.Geom.PlanDir, in.Geom.RunDir); rebaseErr != nil && bindErr == nil {
		return nil, nil, rebaseErr
	}
	if bindErr != nil {
		return nil, nil, bindErr
	}
	var bindBlocking []string
	for _, f := range bindFindings {
		bindBlocking = append(bindBlocking, f.Error())
	}
	if len(bindBlocking) > 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrCardNotDone, strings.Join(bindBlocking, "; "))
	}

	// The informational glyph scope guard never blocks.
	// It degrades explicitly on the deltaErr above rather than running against a zero-value delta;
	// such a delta would look like a real, empty one.
	// An unavailable diff costs visibility, not correctness,
	// and the done-checks above have already blocked on the same infrastructure error.
	if deltaErr != nil {
		warnings = append(warnings, fmt.Sprintf("glyph scope guard could not run for batch %s: %v", in.Label, deltaErr))
	} else {
		deviationSet := map[string]bool{}
		// The guard's targets are the cards as recorded and as re-read.
		// A re-signed member's old glyph and its minted id both count as the batch's own.
		for _, f := range delta.ScopeGuard(in.Plan, append(slices.Clone(recordedCards), in.Cards...)) {
			if f.File != "" && !deviationSet[f.File] {
				deviationSet[f.File] = true
				deviations = append(deviations, f.File)
			}
		}
		sort.Strings(deviations)
	}

	// Drift detection runs after binding and before the digest is persisted.
	// It works on the delta's own deleted-symbols-still-referenced signal.
	// HeadSHA is the triggering SHA that every exact-tier repair's own amendment records.
	// Drift runs against the remaining plan:
	// the delta's deleted symbols intersected with what the plan still has to do.
	// Every already-built card is excluded, and so is this batch's own.
	// Without that exclusion, a Delete card's own deletion is reported against the very card that asked for it,
	// and no Delete card could ever be recorded.
	// in.Plan rides along as DetectDrift's fullPlan so gate one can recognize this batch's own declared Rename outcome:
	// the pending view excludes exactly the cards whose renames the delta reports.
	// The report's renames ride along,
	// so a later card naming a glyph the batch renamed is rewritten even where quarry cannot prove the rename.
	driftFindings, driftErr := delta.DetectDrift(in.Plan, completed, reported, in.Geom.PlanDir, in.Geom.WorktreeRoot, in.HeadSHA, time.Now().UTC().Format(time.RFC3339))
	// The exact-tier repair's RewriteRefs writes to disk before this call reports anything.
	// Its blocking plan-references-deleted-symbol finding comes from another part of the same delta,
	// so the re-baseline runs here, for the reason the bind step above gives.
	if rebaseErr := RestampFingerprint(in.State, in.Geom.PlanDir, in.Geom.RunDir); rebaseErr != nil && driftErr == nil {
		return nil, warnings, rebaseErr
	}
	if driftErr != nil {
		return nil, warnings, driftErr
	}
	// DetectDrift is the one index call in this sequence that returns a mixed severity set:
	// its plan-references-deleted-symbol finding is blocking;
	// the evidence tier's rename-candidate is informational by construction.
	// The rename-versus-genuine-delete decision is the reviewer's, never the pipeline's.
	// Failing the batch on it would destroy the very tier it belongs to.
	// So the split is by whose card the finding concerns.
	// A blocking finding concerns a later card, still to be built;
	// it is recorded as a "later card:" warning rather than failing this batch, and refuses at that card's own dispatch.
	// An informational one rides out on warnings.
	for _, f := range driftFindings {
		if f.Severity != planindex.SeverityBlocking {
			warnings = append(warnings, f.Error())
			continue
		}
		detail := "later card: " + f.Error()
		if text, added := in.RecordWarning("later-card-drift:"+f.Error(), "later-card-drift", detail); added {
			warnings = append(warnings, text)
		}
	}

	// No third restamp is needed.
	// The two above cover every rewrite this pass performs,
	// and each runs right after its own rewriting call rather than past the refusals between them.
	// The amendment log the exact-tier repair writes is excluded from the fingerprint,
	// so it needs no re-baseline.
	return deviations, warnings, nil
}
