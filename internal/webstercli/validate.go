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
anything.

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

			findings, scope, err := c.scopedValidate(plan)
			if err != nil {
				// Named for quarry, not the plan: an operator reading this envelope must never be
				// told the plan is invalid when quarry simply could not answer. Any OTHER validator
				// error still fails the verb rather than being dropped — printing "valid": true over
				// a validation that did not finish is the failure mode internal/planglyph/repo.go's
				// own rationale rejects outright.
				if errors.Is(err, planglyph.ErrQuarryUnavailable) {
					clihelp.SetExit(cmd.Context(), output.Err(out, "webster: quarry could not answer validating plan: "+err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, "webster: validating plan failed: "+err.Error()))
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
