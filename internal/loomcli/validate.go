// validate.go implements the `validate-discussion`, `validate-plan` and `validate-description` loom
// verbs: standalone, zero-argument callers of the identical package functions the Discussion,
// Plan, PR-Rework, Describe and Darn-Write rows' own gates call, per the shared-implementation-is-the-whole-point Shared
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
// checks Discussion-Write's and Discussion-Burler-Fix's own gate runs, callable by the writer agent
// before handoff.
func (c *loomCLI) validateDiscussionCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "validate-discussion",
		Short:       "run the checks Discussion-Write's and Discussion-Burler-Fix's own gate runs standalone against the current discussion files",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `validate-discussion runs discussionparser.Validate against the current
worktree's decision record and support log -- the identical check
Discussion-Write's and Discussion-Burler-Fix's own gate runs -- and reports the
result as one JSON envelope. It takes no arguments and no flags; it always
checks the worktree's own discussion files.

On a darn run it is the Darn-Write row's gate check: it also refuses a record
missing an "Added after Discussion" entry its commit history held, and a clean
run commits the decision record and support log.

Example:
  lyx loom validate-discussion`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			darn := c.env.DiscussionHistory != nil
			var findings []discussionparser.Finding
			var err error
			if darn {
				findings, err = loomshed.ValidateDarnDiscussion(c.env.DecisionRecordPath, c.env.SupportLogPath, c.env.DiscussionHistory)
			} else {
				findings, err = discussionparser.Validate(c.env.DecisionRecordPath, c.env.SupportLogPath)
			}
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
			if darn {
				if err := c.env.CommitDiscussion(); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate discussion: commit the discussion files: "+err.Error()+`; way forward: re-run "lyx loom validate-discussion"`))
					return nil
				}
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
		Use:         "validate-description",
		Short:       "run the check the Describe row's own gate runs standalone against the current change description",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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
// Plan-Write's, a twill Plan-Write run's, Plan-Burler-Fix's and PR-Rework's own gates run, plus the plan-unapproved approval
// check no gate runs at all, callable by the writer agent before handoff.
func (c *loomCLI) validatePlanCmd() *cobra.Command {
	var requireApproved, rework, overlay, lensReview bool

	cmd := &cobra.Command{
		Use:         "validate-plan",
		Short:       "run the checks Plan-Write's, a twill Plan-Write run's, Plan-Burler-Fix's or PR-Rework's own gate runs standalone against the current plan",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `validate-plan parses the current worktree's plan and checks it in one of
five modes, reporting the result as one JSON envelope that carries
informational findings under their own key even on success. It takes no
arguments.

No flag: the index's ValidateFormat, the format-only check set Plan-Write's and
Plan-Burler-Fix's own gate runs, which the plan writer calls before handoff.
The cards of batches webster's run record holds done are history, and their
tree-dependent checks are skipped. --require-approved: the index's Validate,
the same set plus the plan-unapproved check no gate runs, which this flag alone
reaches standalone. --rework: the set PR-Rework's own gate runs, the
format-only checks over the whole new plan plus a check that its first_card
equals the card number the rework session was told. The default and --rework
modes include the card-fabric-reference and fabric-vocabulary-name checks;
--require-approved leaves both out.

--lens-review: the no-flag checks plus the check that the plan directory's
lens-review.md holds a recorded lens review, the check set of a twill
Plan-Write run's own gate. The no-flag mode ignores that file.

--overlay: the no-flag checks over the plan with proposed contents laid over
its files, in memory; nothing is written. Pipe one JSON object on stdin, keys
the file names and values their full content. A name is 00-overview.md or a
card file the Card Index on disk lists; adding, removing or renumbering a card
still goes through insert-card and remove-card. Handles are canonicalized in
memory, and {} checks the plan as it is. It validates target-list,
verify-command and card-text changes, never a proposed code change.

The four flags are mutually exclusive.

Example:
  lyx loom validate-plan
  lyx loom validate-plan --require-approved
  lyx loom validate-plan --rework
  lyx loom validate-plan --lens-review
  lyx loom validate-plan --overlay <<'EOF'
  {"01-slug.md": "<full card file text>"}
  EOF`,
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

			if lensReview && (requireApproved || rework) {
				flag := "--rework"
				if !rework {
					flag = "--require-approved"
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-plan: --lens-review and "+flag+" are mutually exclusive; way forward: run --lens-review alone, which runs the flag-less check set plus the lens-review check"))
				return nil
			}

			if overlay && (requireApproved || rework || lensReview) {
				flag := "--rework"
				if lensReview {
					flag = "--lens-review"
				} else if requireApproved {
					flag = "--require-approved"
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-plan: --overlay and "+flag+" are mutually exclusive; way forward: run --overlay alone, which runs the flag-less check set"))
				return nil
			}

			planDir := planparser.PlanDir(c.env.AnchorPath)
			var plan *planparser.Plan
			var err error
			if overlay {
				var refusal string
				plan, refusal, err = overlaidPlan(cmd.InOrStdin(), planDir)
				if refusal != "" {
					clihelp.SetExit(cmd.Context(), output.Err(out, refusal))
					return nil
				}
			} else {
				plan, err = planparser.ParsePlan(planDir)
			}
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: parse plan at "+planDir+": "+err.Error()))
				return nil
			}

			var findings []planglyph.Finding
			switch {
			case requireApproved:
				findings, err = c.env.PlanIndex.Validate(plan, c.env.WorktreeRoot)
			case rework:
				findings, err = loomshed.ValidateReworkPlan(plan, c.env.WorktreeRoot, c.env.PlanIndex, c.env.Rework.ReadCommitted)
			case lensReview:
				findings, err = loomshed.ValidatePlanLensReview(plan, c.env.AnchorPath, c.env.WorktreeRoot, c.env.PlanIndex)
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
	cmd.Flags().BoolVar(&lensReview, "lens-review", false, "run the flag-less check set plus the check that the plan directory's lens-review.md holds a recorded lens review, as a twill Plan-Write run's own gate does")
	cmd.Flags().BoolVar(&overlay, "overlay", false, "run the no-flag check set over the plan with the proposed plan file contents on stdin (one JSON object, file name to full content) laid over it, in memory, writing nothing")

	return cmd
}
