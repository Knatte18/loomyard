// validate.go implements the `validate-discussion`, `validate-plan` and `validate-description` loom
// verbs: standalone, zero-argument callers of the identical package functions the Discussion,
// Plan, PR-Rework and Describe rows' own gates call, per the shared-implementation-is-the-whole-point Shared
// Decision. No verb re-implements or re-derives any check; each maps its package's result onto the
// envelope-and-exit-contract Shared Decision.

package loomcli

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/summaryparser"
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
// checks Discussion-Write's and Discussion-Burler's own gate runs, callable by the writer agent
// before handoff.
func (c *loomCLI) validateDiscussionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-discussion",
		Short: "run the checks Discussion-Write's and Discussion-Burler's own gate runs standalone against the current discussion files",
		Long: `validate-discussion runs discussionparser.Validate against the current
worktree's decision record and support log -- the identical check
Discussion-Write's and Discussion-Burler's own gate runs -- and reports the
result as one JSON envelope. It takes no arguments and no flags; it always
checks the worktree's own discussion files.

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

// validateDescriptionCmd builds the `validate-description` subcommand: the standalone form of the
// check the Describe row's own gate runs, callable by the Describe agent before handoff.
func (c *loomCLI) validateDescriptionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-description",
		Short: "run the check the Describe row's own gate runs standalone against the current change description",
		Long: `validate-description runs summaryparser.ValidateDescription against the
current worktree's change description -- the identical check the Describe
row's own gate runs -- and reports the result as one JSON envelope. It takes
no arguments and no flags; it always checks the worktree's own description.
It sits beside validate-discussion and validate-plan.

Example:
  lyx loom validate-description`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			findings, err := summaryparser.ValidateDescription(c.env.DescriptionPath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate change description "+c.env.DescriptionPath+": "+err.Error()))
				return nil
			}
			if len(findings) > 0 {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: change description is not yet valid", map[string]any{
					"findings": renderFindings(findings),
				}))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"description": c.env.DescriptionPath,
			}))
			return nil
		},
	}
}

// planFindingsHaveBlocking reports whether findings carries at least one entry that is not
// explicitly informational, mirroring internal/loomshed/gates.go's own hasBlockingFinding:
// severity, not finding count, decides the verdict on both sides of this parity pair.
// It tests NOT-informational rather than equals-blocking for the reason that function's own doc
// gives -- planglyph.Severity is an open string type, and an unrecognized or zero value must not
// silently pass a gate (crucible round opus-medium-r6, R6-27). The two halves of the pair must keep
// agreeing about that, so neither is "the equals-blocking one".
func planFindingsHaveBlocking(findings []planglyph.Finding) bool {
	for _, f := range findings {
		if f.Severity != planglyph.SeverityInformational {
			return true
		}
	}
	return false
}

// validatePlanCmd builds the `validate-plan` subcommand: the standalone form of the checks
// Plan-Write's, Plan-Burler's and PR-Rework's own gates run, plus the plan-unapproved approval
// check no gate runs at all, callable by the writer agent before handoff.
func (c *loomCLI) validatePlanCmd() *cobra.Command {
	var requireApproved, rework bool

	cmd := &cobra.Command{
		Use:   "validate-plan",
		Short: "run the checks Plan-Write's, Plan-Burler's or PR-Rework's own gate runs standalone against the current plan",
		Long: `validate-plan parses the current worktree's plan and checks it in one of
three modes. With no flags, it runs the index's ValidateFormat -- the same
format-only check set Plan-Write's and Plan-Burler's own gate runs before
handoff, and the mode the plan writer calls before handoff; the cards of
batches webster's run record holds done are history, and their
tree-dependent checks are skipped. With
--require-approved, it runs planglyph.Validate -- the same format-only set
plus the plan-unapproved approval check, which no gate runs at all: this
flag is the one place an operator can still reach that check standalone,
since its own guarantee otherwise rests on the review segment's approve
seam failing loudly if it is ever wired nil. With --rework, it runs the
check set PR-Rework's own gate runs: the format-only set over the whole
new plan, plus a check that its first_card equals the card number the
rework session was told to start at -- the mode the rework session calls
before handoff. The default and --rework modes include the
card-fabric-reference check, which refuses a command in the plan that
reaches the fabric repo; --require-approved leaves it out. The two flags
are mutually exclusive. Every mode reports the result as one JSON
envelope, carrying any informational findings under their own envelope
key even on the success path. It takes no arguments.

Example:
  lyx loom validate-plan
  lyx loom validate-plan --require-approved
  lyx loom validate-plan --rework`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if requireApproved && rework {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-plan: --require-approved and --rework are mutually exclusive"))
				return nil
			}

			planDir := planparser.PlanDir(c.env.AnchorPath)
			plan, err := planparser.ParsePlan(planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: parse plan at "+planDir+": "+err.Error()))
				return nil
			}

			var findings []planglyph.Finding
			switch {
			case requireApproved:
				findings, err = planglyph.Validate(plan, c.env.WorktreeRoot)
			case rework:
				findings, err = loomshed.ValidateReworkPlan(plan, c.env.WorktreeRoot, c.env.PlanIndex, c.env.Rework.ReadCommitted)
			default:
				findings, err = loomshed.ValidatePlan(plan, c.env.AnchorPath, c.env.WorktreeRoot, c.env.PlanIndex)
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

	cmd.Flags().BoolVar(&requireApproved, "require-approved", false, "also run the plan-unapproved approval check, the one check no gate runs -- this flag is the sole way to reach it standalone")
	cmd.Flags().BoolVar(&rework, "rework", false, "check the whole new plan and its first_card against the told number, as PR-Rework's own gate does")

	return cmd
}
