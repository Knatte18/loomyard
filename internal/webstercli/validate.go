// validate.go implements the `validate` webster verb: the standalone, lint-without-run half of the
// automatic gate websterengine.Run runs itself before ever spawning Master.
// It parses the plan and runs every plan-format machine check against it, printing exactly one
// JSON envelope: ok with {"valid": true, "cards": <n>, "scope": <scope>} for a clean plan, or an
// error envelope carrying every finding for a plan with findings -- exit non-zero either way a
// blocking finding exists, never plain text.
// The check set is SCOPED the way Run scopes its own,
// and the "scope" key names which answer the call gave: planglyph.Validate's whole-plan answer while no batch has begun, and planglyph.ValidateDispatch's pending-cards answer once a run has begun any batch.
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
// msg is the envelope's error text.
func findingsEnvelope(out io.Writer, msg string, findings []planglyph.Finding, scope string) int {
	data, _ := json.Marshal(map[string]any{
		"ok":       false,
		"error":    msg,
		"findings": findingsEntries(findings),
		"scope":    scope,
	})
	fmt.Fprintln(out, string(data))
	return 1
}

// scopedValidate runs the check set this call's own scope calls for, and returns the scope name alongside the findings so every envelope can report it.
// st is the state validateCmd already loaded under the lease; nil means no run has started.
//
// With no begun card -- no run at all, or a run that has not begun a batch yet --
// it runs planglyph.Validate: the whole plan, including the plan-unapproved approval gate, which is the honest answer for the pre-flight case this verb exists to serve.
//
// Once a run has begun any batch it runs planglyph.ValidateDispatch scoped by websterengine.DispatchScope,
// the same call begin-batch and websterengine.Run make.
// A begun card's own targets are not resolved, since its work may have landed or not;
// a forthcoming card's Create and Rename New targets are excluded from the status check, so a later card that Uses them passes.
// Approval is deliberately not re-checked on that branch, and nothing is lost by it:
// a run cannot have begun a batch without having passed Run's own entry-time approval refusal first.
//
// With no state it computes the fresh partition first and refuses its batch-order error, as the first init would.
// A nil batcher with a state on disk is a wiring bug rather than a state, and it is reported as one:
// reading it as "no begun cards" would silently hand back the whole-plan answer this function exists to avoid.
func (c *websterCLI) scopedValidate(plan *planparser.Plan, st *websterengine.State) ([]planglyph.Finding, string, error) {
	if st == nil {
		if c.batcher != nil {
			if _, err := c.executionBatches(plan, nil); err != nil {
				return nil, "", err
			}
		}
		findings, err := planglyph.Validate(plan, c.geom.WorktreeRoot)
		return findings, scopeWholePlan, err
	}
	if c.batcher == nil {
		return nil, "", websterengine.ErrNilBatcher
	}

	batches, err := c.executionBatches(plan, st)
	if err != nil {
		return nil, "", err
	}
	begun, forthcoming := websterengine.DispatchScope(batches, st)
	if len(begun) == 0 {
		findings, err := planglyph.Validate(plan, c.geom.WorktreeRoot)
		return findings, scopeWholePlan, err
	}
	findings, err := planglyph.ValidateDispatch(plan, c.geom.WorktreeRoot, begun, forthcoming)
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
plan-fingerprint crash/resume guard afterward, so a rewrite it performs is
never later mistaken for a foreign edit. With a run in progress, validate
first checks the plan against the fingerprint the run recorded: a plan edited
since then is still linted, but validate skips the re-baseline, leaves
state.json untouched and exits non-zero naming "lyx webster rebaseline" and
"lyx webster restore-plan", so an edit is never adopted unseen.

Which cards are checked follows the run's own progress, exactly as the
automatic gate "lyx webster run" applies before forking an implementer does,
and every envelope reports the answer it gave under a "scope" key:

  whole-plan  no batch has been begun yet (no run at all, or a run that
              has begun none). Every card is checked, including the
              plan-unapproved approval gate.
  pending     a run has begun at least one batch. The cards of every begun
              batch are excluded from resolving: their work may have landed,
              so a Create target may exist and a Delete or Rename-old target
              may be gone. The Create and Rename-new targets of begun
              batches that are not yet terminal count as forthcoming, so a
              later card that Uses one is not reported as missing. Approval
              is not re-checked, being already an established fact of that
              run.

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

			// The edit check runs before scopedValidate's own rewrite:
			// the restamp below exists to adopt webster's own rewrites, so any difference seen here is someone else's edit and is refused, never adopted into the plan hashes.
			var editErr error
			if st != nil {
				editErr = websterengine.PlanEditError(st, plan.Dir)
			}

			findings, scope, validateErr := c.scopedValidate(plan, st)

			// Re-baseline regardless of validateErr, exactly as begin-batch re-baselines ahead of
			// every refusal below it: a sanctioned rewrite that already landed on disk is a durable
			// fact about the plan whether or not a LATER step of this same call then fails. A nil
			// st (no run in progress) means there is no state.json to desync, so this is a no-op.
			// An edited plan skips it and leaves state.json untouched.
			var rebaseErr error
			if st != nil && editErr == nil {
				if fpErr := websterengine.RestampPlanBaseline(st, plan.Dir, c.geom.WebsterDir); fpErr != nil {
					rebaseErr = fpErr
				} else {
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
					msg = fmt.Sprintf("%s (additionally, persisting the plan-fingerprint re-baseline this call had already earned failed: %v; way forward: re-run `lyx webster validate`)", msg, rebaseErr)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}
			if editErr != nil {
				clihelp.SetExit(cmd.Context(), findingsEnvelope(out, editErr.Error(), findings, scope))
				return nil
			}
			if rebaseErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: validate finished but persisting the plan-fingerprint re-baseline failed: %v -- state.json may now be stale; the next begin-batch/record-batch/run may refuse the plan as foreign; way forward: re-run `lyx webster validate`", rebaseErr)))
				return nil
			}

			if findingsHaveBlocking(findings) {
				clihelp.SetExit(cmd.Context(), findingsEnvelope(out, fmt.Sprintf("webster: plan validation found %d finding(s)", len(findings)), findings, scope))
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
