// validate.go implements the `validate` webster verb.
// It is the lint-without-run half of the automatic gate that websterengine.Run applies before it spawns Master.
// It parses the plan and runs every plan-format machine check against it.
// It prints exactly one JSON envelope.
// An ok envelope, {"valid": true, "cards": <n>, "scope": <scope>}, marks a clean plan.
// An error envelope carries every finding.
// It exits non-zero whenever a blocking finding exists, and it never prints plain text.
// The check set is scoped the way Run scopes its own, and the "scope" key names which answer the call gave:
// the index's Validate, the whole-plan answer, while no batch has begun;
// or planglyph.ValidateDispatch's pending-cards answer once a run has begun any batch.
// See scopedValidate for why the two are not interchangeable.
// Run's pre-flight also refuses a zero-batch plan outright:
// nothing to build is a malformed plan, never a vacuous done outcome (websterengine's runlevel.go).
// validate surfaces that emptiness through the findings set rather than a distinct check,
// since a zero-card plan already fails planparser's index-file-consistency checks.
// With --batches the ok envelope also carries "profile" and "batches":
// the fresh partition the active batchifier forms for the plan.
// Each batch lists its card numbers and estimated peak context.
// It is computed without state, so it shows what a fresh run would form even while a run holds its recorded partition.
package webstercli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// scopeWholePlan and scopePending are the two values validate's envelopes report under "scope",
// naming which of planglyph's two entry points answered the call.
// They are constants because the value is part of the verb's output contract:
// a Planner reads it to tell whether an absent finding means "clean" or "not re-checked on this call".
const (
	scopeWholePlan = "whole-plan"
	scopePending   = "pending"
)

// findingsHaveBlocking reports whether findings holds at least one planglyph.SeverityBlocking entry.
// Severity, not finding count, decides the verdict: an informational finding never blocks.
func findingsHaveBlocking(findings []planglyph.Finding) bool {
	for _, f := range findings {
		if f.Severity == planglyph.SeverityBlocking {
			return true
		}
	}
	return false
}

// findingsEntries renders findings as the array an envelope places under its "findings" key.
// Each entry carries its severity beside check, card and detail,
// so an informational finding can be told apart from a blocking one.
func findingsEntries(findings []planglyph.Finding) []map[string]string {
	entries := make([]map[string]string, len(findings))
	for i, f := range findings {
		entries[i] = map[string]string{"check": f.Check, "card": f.Card, "detail": f.Detail, "severity": string(f.Severity)}
	}
	return entries
}

// findingsEnvelope writes a JSON error envelope carrying findings and the scope they were collected under.
// The scope tells a reader whether a missing finding means "clean" or "not checked on this call",
// so the refusal envelope carries it just as the success envelope does.
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

// scopedValidate runs the check set this call's scope calls for.
// It returns the scope name with the findings, so every envelope can report it.
// st is the state validateCmd loaded under the lease; nil means no run has started.
//
// With no begun card, it runs the index's Validate:
// the whole plan, including the plan-unapproved approval gate.
// That is the honest answer for the pre-flight case this verb serves.
//
// Once a run has begun any batch, it runs planglyph.ValidateDispatch scoped by websterengine.DispatchScope.
// A begun card's own targets are not resolved, since its work may or may not have landed.
// A forthcoming card's Create and Rename New targets are excluded from the status check,
// so a later card that Uses them passes.
// Approval is not re-checked on that branch:
// a run cannot have begun a batch without first passing websterengine's entry-time approval refusal.
//
// With no state, it first computes the fresh partition and returns its batch-order error, as a first init would.
// A nil batcher with a state on disk is a wiring bug, reported as websterengine.ErrNilBatcher.
// Reading it as "no begun cards" would silently return the whole-plan answer this function exists to avoid.
func (c *websterCLI) scopedValidate(plan *planparser.Plan, st *websterengine.State) ([]planglyph.Finding, string, error) {
	if st == nil {
		if c.batcher != nil {
			if _, err := c.executionBatches(plan, nil); err != nil {
				return nil, "", err
			}
		}
		findings, err := c.geom.Index.Validate(plan, c.geom.WorktreeRoot)
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
		findings, err := c.geom.Index.Validate(plan, c.geom.WorktreeRoot)
		return findings, scopeWholePlan, err
	}
	findings, err := planglyph.ValidateDispatch(plan, c.geom.WorktreeRoot, begun, forthcoming)
	return findings, scopePending, err
}

// batchEntries renders batches as the array that validate's --batches output places under "batches".
// Each entry carries the batch's card numbers and its estimated peak context.
func batchEntries(batches []batcher.Batch) []map[string]any {
	entries := make([]map[string]any, len(batches))
	for i, b := range batches {
		cards := make([]int, len(b.Cards))
		for j, card := range b.Cards {
			cards[j] = card.Number
		}
		entries[i] = map[string]any{"cards": cards, "estimate": b.Estimate}
	}
	return entries
}

// validateCmd builds the `validate` subcommand.
func (c *websterCLI) validateCmd() *cobra.Command {
	var showBatches bool
	cmd := &cobra.Command{
		Use:         "validate",
		Short:       "lint the plan against the plan-format machine checks without running anything",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `validate parses the plan at _lym/plan and runs the plan-format machine checks
listed in contracts/specs/loom-plan-checks-spec.md. A
plan with no blocking finding prints {"valid": true, "cards": N}, carrying any
informational findings under their own key; a blocking finding prints an
error envelope with every finding (check, card, detail, severity) and exits
non-zero.

Reach for it before handing a plan to a run, or while editing one mid-run. It
spawns nothing, though it may canonicalize a plan: handle on disk, as every
bracket verb does. With a run in progress, a plan edited since the run
recorded it is still linted, and validate exits non-zero naming
"lym webster rebaseline" and "lym webster restore-plan".

The checked cards follow the run's progress, reported under "scope":
"whole-plan" before any batch is begun, approval gate included, and "pending"
once one is, leaving out the cards of begun batches.

--batches adds "profile", the active batchifier's name, and "batches", the
partition it forms for the plan, one {"cards": [card numbers], "estimate":
peak context} per batch in run order, formed fresh as a new run would.

Examples:
  lym webster validate
  lym webster validate --batches`,
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

			// scopedValidate's resolve pass can canonicalize a not-yet-canonical handle, rewriting the plan on disk.
			// validate then restamps state.json's PlanFingerprint, so the rewrite is not taken for a foreign edit.
			// Without the restamp, the next begin-batch, record-batch or run refuses the plan.
			// It forces --fresh, which discards a live run's progress over what looked like a read-only lint.
			// The restamp reloads state rather than trusting an in-memory copy;
			// persistPlanFingerprintRebaseline's doc comment explains why.
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

			// The edit check runs before scopedValidate rewrites the plan.
			// The restamp below adopts webster's own rewrites, so a difference seen here is someone else's edit:
			// it is refused and never adopted into the plan hashes.
			var editErr error
			if st != nil {
				editErr = websterengine.PlanEditError(st, plan.Dir, c.geom.WebsterDir)
			}

			findings, scope, validateErr := c.scopedValidate(plan, st)

			// Re-baseline regardless of validateErr.
			// A sanctioned rewrite that already landed on disk is a durable fact about the plan.
			// It stands whether or not a later step of this call fails.
			// A nil st means no run is in progress: there is no state.json to desync, so the re-baseline is a no-op.
			// An edited plan skips the re-baseline and leaves state.json untouched.
			var rebaseErr error
			if st != nil && editErr == nil {
				if fpErr := websterengine.RestampPlanBaseline(st, plan.Dir, c.geom.WebsterDir); fpErr != nil {
					rebaseErr = fpErr
				} else {
					rebaseErr = persistPlanFingerprintRebaseline(c.geom, st, fingerprintBefore)
				}
			}

			if validateErr != nil {
				// The message names quarry, not the plan, so an operator never reads the plan as invalid when quarry could not answer.
				// Any other validator error still fails the verb rather than being dropped;
				// printing "valid": true over an unfinished validation is a failure mode internal/planglyph/repo.go rejects outright.
				msg := "webster: validating plan failed: " + validateErr.Error()
				if errors.Is(validateErr, planglyph.ErrQuarryUnavailable) {
					msg = "webster: quarry could not answer validating plan: " + validateErr.Error()
				}
				if rebaseErr != nil {
					msg = fmt.Sprintf("%s (additionally, persisting the plan-fingerprint re-baseline this call had already earned failed: %v; way forward: re-run `lym webster validate`)", msg, rebaseErr)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}
			if editErr != nil {
				clihelp.SetExit(cmd.Context(), findingsEnvelope(out, editErr.Error(), findings, scope))
				return nil
			}
			if rebaseErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: validate finished but persisting the plan-fingerprint re-baseline failed: %v -- state.json may now be stale; the next begin-batch/record-batch/run may refuse the plan as foreign; way forward: re-run `lym webster validate`", rebaseErr)))
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
			if showBatches {
				if c.batcher == nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, websterengine.ErrNilBatcher.Error()))
					return nil
				}
				batches, err := c.executionBatches(plan, nil)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, "webster: forming the batch partition failed: "+err.Error()))
					return nil
				}
				fields["profile"] = c.batcher.Name()
				fields["batches"] = batchEntries(batches)
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}

	cmd.Flags().BoolVar(&showBatches, "batches", false, "also print the active batchifier's profile and the partition it forms for the plan")
	return cmd
}
