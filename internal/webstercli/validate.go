// validate.go implements the `validate` webster verb: the standalone, lint-without-run half of the
// automatic gate websterengine.Run runs itself before ever spawning Master.
// It parses the plan and runs every plan-format machine check against it, printing exactly one
// JSON envelope: ok with {"valid": true, "cards": <n>, "scope": <scope>} for a clean plan, or an
// error envelope carrying every finding for a plan with findings -- exit non-zero either way a
// blocking finding exists, never plain text.
// The check set is SCOPED the way Run scopes its own, and the "scope" key names which answer the
// call gave: planglyph.Validate's whole-plan answer while no batch has completed, and
// planglyph.ValidateDispatch's pending-cards answer once a run has recorded a terminal batch.
// See scopedValidate for why the two are not interchangeable.
// webster's own Run pre-flight ALSO refuses a zero-batch plan outright
// (nothing-to-build is a malformed plan, never a vacuous outcome: done, per websterengine's
// runlevel.go);
// validate surfaces that same emptiness through the findings set rather than a distinct check,
// since a zero-card plan already fails planparser's index-file-consistency checks.
package webstercli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// scopeWholePlan and scopePending are the two values validate's envelopes report under "scope",
// naming which of planglyph's two entry points answered this call. They are spelled out as
// constants because the value is part of the verb's output contract, read by a Planner deciding
// whether an absent finding means "clean" or "not re-checked on this call".
const (
	scopeWholePlan = "whole-plan"
	scopePending   = "pending"
)

// findingsHaveBlocking reports whether findings carries at least one planglyph.SeverityBlocking
// entry, mirroring internal/loomshed/planvalidate.go's own hasBlockingFinding and
// internal/loomcli/validate.go's own planFindingsHaveBlocking: severity, not finding count, decides
// the verdict on every side of this parity pair.
func findingsHaveBlocking(findings []planglyph.Finding) bool {
	for _, f := range findings {
		if f.Severity == planglyph.SeverityBlocking {
			return true
		}
	}
	return false
}

// findingsEntries renders findings as the structured array both findingsEnvelope and validateCmd's
// own informational-only pass path place under an envelope's "findings" key, each entry carrying
// its own severity alongside check, card, and detail so an informational create-new-unit is
// distinguishable from a blocking glyph-not-found in the one place this record exists.
func findingsEntries(findings []planglyph.Finding) []map[string]string {
	entries := make([]map[string]string, len(findings))
	for i, f := range findings {
		entries[i] = map[string]string{"check": f.Check, "card": f.Card, "detail": f.Detail, "severity": string(f.Severity)}
	}
	return entries
}

// findingsEnvelope writes a JSON error envelope carrying findings via findingsEntries, plus the
// scope those findings were collected under -- the refusal envelope needs it as much as the success
// one does, since which cards were re-resolved is what decides whether a missing finding means
// "clean" or "out of scope on this call".
func findingsEnvelope(out io.Writer, findings []planglyph.Finding, scope string) int {
	data, _ := json.Marshal(map[string]any{
		"ok":       false,
		"error":    fmt.Sprintf("webster: plan validation found %d finding(s)", len(findings)),
		"findings": findingsEntries(findings),
		"scope":    scope,
	})
	fmt.Fprintln(out, string(data))
	return 1
}

// completedCards returns the cards of every batch the run recorded at c.geom has already driven to
// a terminal classification, in the execution sequencer's own order -- or nil when no run has
// started yet.
//
// It is the scoping value planglyph.ValidateDispatch takes, and it is what makes this verb's answer
// agree with the automatic gate it advertises: a plan describes intended CHANGE, so a card whose
// work already landed necessarily contradicts the tree it would be re-resolved against -- its
// Create target now exists (create-already-exists under the Create inversion), its Delete target
// and its Rename's old side are gone (glyph-not-found). Every one of those is the plan working
// exactly as designed, reported as a blocking defect.
//
// The batch-terminality walk is spelled out here rather than called on websterengine, whose own
// completedCards is package-private and whose exported surface offers no replacement. The two must
// stay in step; TestValidateCmd_MidRunScopesToPendingCards pins this one against a state.json
// shaped exactly as a run writes it.
//
// A nil batcher is a wiring bug rather than a state, and it is reported as one: reading it as "no
// completed cards" would silently hand back the whole-plan answer this function exists to avoid.
func (c *websterCLI) completedCards(plan *planparser.Plan) ([]planparser.Card, error) {
	state, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	if c.batcher == nil {
		return nil, websterengine.ErrNilBatcher
	}

	// Every batch-computation site sequences, so all of them agree on one order by construction
	// rather than by comment.
	batches, _ := websterengine.SequenceBatches(c.batcher.Batch(plan.Cards))

	var completed []planparser.Card
	for _, batch := range batches {
		if len(batch.Cards) == 0 {
			continue
		}
		batchState, ok := state.Batches[batch.Cards[0].Number]
		if !ok || batchState == nil || !batchState.Terminal {
			continue
		}
		completed = append(completed, batch.Cards...)
	}
	return completed, nil
}

// scopedValidate runs the check set this call's own scope calls for, and returns the scope name
// alongside the findings so every envelope can report it.
//
// With no completed cards -- no run at all, or a run that has not recorded a terminal batch yet --
// it runs planglyph.Validate: the whole plan, including the plan-unapproved approval gate, which is
// the honest answer for the pre-flight case this verb exists to serve.
//
// Once a batch has landed it runs planglyph.ValidateDispatch scoped by exactly that set, which is
// the call websterengine.Run makes. Approval is deliberately not re-checked on that branch, and
// nothing is lost by it: a run cannot have recorded a terminal batch without having passed Run's
// own entry-time approval refusal first, so approval is an established fact of the run rather than
// an open question. Before the fix this branch did not exist, and the verb answered mid-run with
// the whole-plan check set -- exiting 1 over the very plan `lyx webster run` resumes without
// complaint, the exact wedge websterengine/runlevel.go documents having fixed for Run.
func (c *websterCLI) scopedValidate(plan *planparser.Plan) ([]planglyph.Finding, string, error) {
	completed, err := c.completedCards(plan)
	if err != nil {
		return nil, "", err
	}
	if len(completed) == 0 {
		findings, err := planglyph.Validate(plan, c.geom.WorktreeRoot)
		return findings, scopeWholePlan, err
	}
	findings, err := planglyph.ValidateDispatch(plan, c.geom.WorktreeRoot, completed)
	return findings, scopePending, err
}

// validateCmd builds the `validate` subcommand.
func (c *websterCLI) validateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "lint the plan against the plan-format machine checks without running anything",
		Long: `validate parses the plan at _lyx/plan and runs the plan-format machine
check set against it -- see contracts/specs/loom-plan-spec.md's own
"Validation checks" section for the complete list, since it is the source of
truth rather than a count pinned here. A clean plan, or one carrying only
informational findings, prints {"valid": true, "cards": N}, with any
informational findings carried under their own key for visibility. A plan
carrying at least one blocking finding prints an error envelope carrying
every finding (check, card, detail, severity) and exits non-zero. validate is
the lint-without-run pre-flight for a Planner or human; it never spawns
anything -- but it is not read-only: like every bracket verb, its own
resolve pass can canonicalize a not-yet-canonical plan: handle, rewriting
the affected card files on disk, and validate re-baselines state.json's
plan-fingerprint crash/resume guard afterward exactly as begin-batch and
record-batch already do, so a rewrite it performs is never later mistaken
for a foreign edit.

Which cards are checked follows the run's own progress, exactly as the
automatic gate "lyx webster run" applies before forking an implementer does,
and every envelope reports the answer it gave under a "scope" key:

  whole-plan  no batch has reached a terminal classification yet (no run
              at all, or a run that has recorded none). Every card is
              checked, including the plan-unapproved approval gate.
  pending     a run has recorded at least one terminal batch. Only the
              cards whose work has NOT landed are re-resolved: a completed
              Create target now exists and a completed Delete or Rename-old
              target is gone, so re-resolving them reports the plan working
              as designed as a blocking defect. Approval is not re-checked,
              being already an established fact of that run.

Example:
  lyx webster validate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			plan, err := planparser.ParsePlan(c.geom.PlanDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// scopedValidate's own resolve pass can canonicalize a not-yet-canonical plan: handle,
			// rewriting the plan on disk exactly as begin-batch's own ValidateDispatch call can
			// (CanonicalizeHandles, resolve.go) -- but unlike every bracket verb, validate never
			// restamped state.json's PlanFingerprint afterward, silently desyncing webster's own
			// crash/resume guard: the plan on disk carried validate's own sanctioned edit while
			// state.json kept the pre-rewrite fingerprint, so the next begin-batch/record-batch/run
			// refused it as a foreign edit and forced --fresh, discarding a live run's progress over
			// what looked like a read-only lint (crucible round sonnet-xhigh-r8, WS-1). The lease and
			// reload-then-restamp shape below mirror begin-batch's own restamp discipline exactly —
			// see persistPlanFingerprintRebaseline's own doc comment for why the restamp reloads
			// state fresh rather than trusting an in-memory copy.
			mutateLock, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			defer func() { _ = mutateLock.Release() }()

			st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			var fingerprintBefore string
			if st != nil {
				fingerprintBefore = st.PlanFingerprint
			}

			findings, scope, validateErr := c.scopedValidate(plan)

			// Re-baseline regardless of validateErr, exactly as begin-batch re-baselines ahead of
			// every refusal below it: a sanctioned rewrite that already landed on disk is a durable
			// fact about the plan whether or not a LATER step of this same call then fails. A nil
			// st (no run in progress) means there is no state.json to desync, so this is a no-op.
			var rebaseErr error
			if st != nil {
				if fp, fpErr := websterengine.Fingerprint(plan.Dir); fpErr != nil {
					rebaseErr = fpErr
				} else {
					st.PlanFingerprint = fp
					rebaseErr = persistPlanFingerprintRebaseline(c.geom, st, fingerprintBefore)
				}
			}

			if validateErr != nil {
				// Named for quarry, not the plan: an operator reading this envelope must never be
				// told the plan is invalid when quarry simply could not answer. Any OTHER validator
				// error still fails the verb rather than being dropped — printing "valid": true over
				// a validation that did not finish is the failure mode internal/planglyph/repo.go's
				// own rationale rejects outright.
				msg := "webster: validating plan failed: " + validateErr.Error()
				if errors.Is(validateErr, planglyph.ErrQuarryUnavailable) {
					msg = "webster: quarry could not answer validating plan: " + validateErr.Error()
				}
				if rebaseErr != nil {
					msg = fmt.Sprintf("%s (additionally, persisting the plan-fingerprint re-baseline this call had already earned failed: %v)", msg, rebaseErr)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}
			if rebaseErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: validate finished but persisting the plan-fingerprint re-baseline failed: %v -- state.json may now be stale; the next begin-batch/record-batch/run may refuse the plan as foreign", rebaseErr)))
				return nil
			}

			if findingsHaveBlocking(findings) {
				clihelp.SetExit(cmd.Context(), findingsEnvelope(out, findings, scope))
				return nil
			}

			fields := map[string]any{
				"valid": true,
				"cards": len(plan.Cards),
				"scope": scope,
			}
			if len(findings) > 0 {
				fields["findings"] = findingsEntries(findings)
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}

	return cmd
}
