// validate.go implements the `validate-discussion` and `validate-plan` loom verbs: standalone,
// zero-argument callers of the identical package functions the Discussion-Validate and
// Plan-Validate mechanical gates call, per the shared-implementation-is-the-whole-point Shared
// Decision. Neither verb re-implements or re-derives any check; both map their package's result
// onto the envelope-and-exit-contract Shared Decision.

package loomcli

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/spf13/cobra"
)

// renderFindings turns a slice of values carrying an Error() string method into the []string
// payload placed under an envelope's "findings" key, so validate-discussion and validate-plan
// render their findings identically, per the findings-render-as-error-strings Shared Decision.
func renderFindings[T interface{ Error() string }](items []T) []string {
	rendered := make([]string, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, item.Error())
	}
	return rendered
}

// validateDiscussionCmd builds the `validate-discussion` subcommand: the standalone form of the
// Discussion-Validate mechanical gate, callable by the writer agent before handoff.
func (c *loomCLI) validateDiscussionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-discussion",
		Short: "run the Discussion-Validate gate's checks standalone against the current discussion files",
		Long: `validate-discussion runs discussionparser.Validate against the current
worktree's decision record and support log -- the identical check the
Discussion-Validate mechanical gate runs -- and reports the result as one
JSON envelope. It takes no arguments and no flags; it always checks the
worktree's own discussion files.

Example:
  lyx loom validate-discussion`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			findings, err := discussionparser.Validate(c.env.DecisionRecordPath, c.env.SupportLogPath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate discussion record "+c.env.DecisionRecordPath+": "+err.Error()))
				return nil
			}
			if len(findings) > 0 {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: discussion is not yet valid", map[string]any{
					"findings": renderFindings(findings),
				}))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"decision_record": c.env.DecisionRecordPath,
				"support_log":     c.env.SupportLogPath,
			}))
			return nil
		},
	}
}

// planFindingsHaveBlocking reports whether findings carries at least one planglyph.SeverityBlocking
// entry, mirroring internal/loomshed/planvalidate.go's own hasBlockingFinding: severity, not finding
// count, decides the verdict on both sides of this parity pair.
func planFindingsHaveBlocking(findings []planglyph.Finding) bool {
	for _, f := range findings {
		if f.Severity == planglyph.SeverityBlocking {
			return true
		}
	}
	return false
}

// validatePlanCmd builds the `validate-plan` subcommand: the standalone form of the Plan-Validate/
// Plan-Revalidate mechanical gate, callable by the writer agent before handoff.
func (c *loomCLI) validatePlanCmd() *cobra.Command {
	var requireApproved bool

	cmd := &cobra.Command{
		Use:   "validate-plan",
		Short: "run the Plan-Validate gate's checks standalone against the current plan",
		Long: `validate-plan parses the current worktree's plan and checks it in one of
two modes. With no flags, it runs planglyph.ValidateFormat -- the same
format-only check set the Plan-Validate mechanical gate runs before review,
and the mode the plan writer calls before handoff. With --require-approved,
it runs planglyph.Validate -- the same full check set, including the
plan-unapproved approval gate, that the Plan-Revalidate mechanical gate runs
after review settles. Either way it reports the result as one JSON
envelope, carrying any informational findings under their own envelope key
even on the success path. It takes no arguments.

Example:
  lyx loom validate-plan
  lyx loom validate-plan --require-approved`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			planDir := planparser.PlanDir(c.env.AnchorPath)
			plan, err := planparser.ParsePlan(planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: parse plan at "+planDir+": "+err.Error()))
				return nil
			}

			var findings []planglyph.Finding
			if requireApproved {
				findings, err = planglyph.Validate(plan, c.env.WorktreeRoot)
			} else {
				findings, err = planglyph.ValidateFormat(plan, c.env.WorktreeRoot)
			}
			if err != nil {
				// Named for quarry, not the plan: an operator reading this envelope must never be
				// told the plan is invalid when quarry simply could not answer -- this is the
				// CLI-side half of loomshed's producer-side disposition, and the two must agree.
				// Any OTHER validator error still fails the verb rather than being dropped, with
				// its own wording: reporting "valid" over a validation that did not finish is the
				// failure mode internal/planglyph/repo.go's rationale rejects outright.
				if errors.Is(err, planglyph.ErrQuarryUnavailable) {
					clihelp.SetExit(cmd.Context(), output.Err(out, "loom: quarry could not answer validating plan at "+planDir+": "+err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validating plan at "+planDir+" failed: "+err.Error()))
				return nil
			}

			if planFindingsHaveBlocking(findings) {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: plan is not yet valid", map[string]any{
					"findings": renderFindings(findings),
				}))
				return nil
			}

			fields := map[string]any{
				"plan_dir": planDir,
			}
			if len(findings) > 0 {
				// Informational-only: surfaced for visibility on the pass path, under its own key
				// rather than dropped, so a plan that introduces a new package is never silently
				// reported as if nothing had been said about it.
				fields["findings"] = renderFindings(findings)
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}

	cmd.Flags().BoolVar(&requireApproved, "require-approved", false, "also run the plan-unapproved approval gate, matching the Plan-Revalidate row")

	return cmd
}
