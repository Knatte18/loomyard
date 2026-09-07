// validate.go implements the `validate` webster verb: the standalone pre-flight half of the
// automatic gate websterengine.Run runs itself before ever spawning Master.
// It parses the plan and runs every plan-format machine check against it, printing exactly one
// JSON envelope: ok with {"valid": true, "cards": <n>} for a clean plan, or an error envelope
// carrying every finding for a plan with findings -- exit non-zero either way a blocking finding
// exists, never plain text.
// webster's own Run pre-flight ALSO refuses a zero-batch plan outright
// (nothing-to-build is a malformed plan, never a vacuous outcome: done, per websterengine's
// runlevel.go);
// validate surfaces that same emptiness through planglyph.Validate's own findings set rather than
// a distinct check, since a zero-card plan already fails planparser's index-file-consistency
// checks.
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
	"github.com/spf13/cobra"
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

// findingsEnvelope writes a JSON error envelope carrying findings via findingsEntries.
func findingsEnvelope(out io.Writer, findings []planglyph.Finding) int {
	data, _ := json.Marshal(map[string]any{
		"ok":       false,
		"error":    fmt.Sprintf("webster: plan validation found %d finding(s)", len(findings)),
		"findings": findingsEntries(findings),
	})
	fmt.Fprintln(out, string(data))
	return 1
}

// validateCmd builds the `validate` subcommand.
func (c *websterCLI) validateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "lint the plan against the plan-format machine checks without running anything",
		Long: `validate parses the plan at _lyx/plan and runs the full planglyph.Validate
check set against it -- see contracts/specs/loom-plan-spec.md's own
"Validation checks" section for the complete list, since it is the source of
truth rather than a count pinned here. A clean plan, or one carrying only
informational findings, prints {"valid": true, "cards": N}, with any
informational findings carried under their own key for visibility. A plan
carrying at least one blocking finding prints an error envelope carrying
every finding (check, card, detail, severity) and exits non-zero -- this is
the SAME gate "lyx webster run" runs automatically before ever forking an
implementer; validate is the lint-without-run pre-flight for a Planner or
human.

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

			findings, err := planglyph.Validate(plan, c.geom.WorktreeRoot)
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
				clihelp.SetExit(cmd.Context(), findingsEnvelope(out, findings))
				return nil
			}

			fields := map[string]any{
				"valid": true,
				"cards": len(plan.Cards),
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
