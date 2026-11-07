// validate.go implements the validate-discussion, validate-plan, validate-parts and validate-description loom verbs.
// Each verb calls the same package function the matching recipe row's gate calls.
// No verb re-implements or re-derives a check.
// Each maps its package's result onto the envelope and exit contract.

package loomcli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/spf13/cobra"
)

// renderFindings turns a slice of values carrying an Error() string method into the []string
// payload placed under an envelope's "findings" key.
// validate-discussion and validate-plan share it so they render their findings identically.
func renderFindings[T interface{ Error() string }](items []T) []string {
	rendered := make([]string, 0, len(items))
	for _, item := range items {
		rendered = append(rendered, item.Error())
	}
	return rendered
}

// validateDiscussionCmd builds the `validate-discussion` subcommand: the standalone form of the
// checks the Discussion-Write and Discussion-Burler-Fix gates run, callable by the writer agent
// before handoff.
// It checks every loom record with the one check the gates run, and commits nothing.
func (c *loomCLI) validateDiscussionCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "validate-discussion",
		Short:       "run the checks Discussion-Write's and Discussion-Burler-Fix's own gate runs standalone against the current discussion files",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `validate-discussion runs the loom gate's check against the current
worktree's discussion files -- the identical check Discussion-Write's and
Discussion-Burler-Fix's own gate runs: the decision record and support log
check, the decision-id and acceptance checks, and the check that the record
keeps every "Added after Discussion" entry and appended PATTERN bullet its
commit history held -- and reports the result as one JSON envelope. It takes
no arguments and no flags; it always checks the worktree's own discussion
files, and it commits nothing.

On a run whose record Discussion-Write writes, it also runs the citation check,
which refuses a path or symbol the decision record cites that does not exist in
the worktree, unless marked (new) or (removed), and the relevant list check when
relevant.md exists.

Example:
  lym loom validate-discussion`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			findings, err := loomshed.ValidateDiscussion(loomshed.DiscussionCheck{
				RecordPath:     c.env.DecisionRecordPath,
				SupportLogPath: c.env.SupportLogPath,
				RelevantPath:   c.env.RelevantPath,
				WorktreeRoot:   c.env.WorktreeRoot,
				Index:          c.env.PlanIndex,
				History:        c.env.DiscussionHistory,
				Citations:      c.env.RecordEngine == "DiscussionWrite",
			})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate discussion: "+err.Error()))
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
		Use:         "validate-description",
		Short:       "run the check the Describe row's own gate runs standalone against the current change description",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `validate-description runs summaryparser.ValidateDescription against the
current worktree's change description -- the identical check the Describe
row's own gate runs -- and reports the result as one JSON envelope. It takes
no arguments and no flags; it always checks the worktree's own description.
It sits beside validate-discussion and validate-plan.

Example:
  lym loom validate-description`,
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

// planFindingsHaveBlocking reports whether findings carries at least one entry that is not informational.
// Severity, not finding count, decides the verdict.
// The test is not-informational rather than equals-blocking, because planglyph.Severity is an open string type,
// and an unrecognized or zero value must not silently pass a gate.
// Every gate that decides a plan's verdict must test the same way.
func planFindingsHaveBlocking(findings []planglyph.Finding) bool {
	for _, f := range findings {
		if f.Severity != planglyph.SeverityInformational {
			return true
		}
	}
	return false
}

// validatePlanCmd builds the `validate-plan` subcommand.
// It is the standalone form of the checks that the Plan-Write, twill Plan-Write and Plan-Burler-Fix gates run.
// It also runs the plan-unapproved approval check, which no gate runs.
// The writer agent calls it before handoff.
func (c *loomCLI) validatePlanCmd() *cobra.Command {
	var requireApproved, rework, overlay, lensReview bool
	var part string

	cmd := &cobra.Command{
		Use:         "validate-plan",
		Short:       "run the checks Plan-Write's, a twill Plan-Write run's, or Plan-Burler-Fix's own gate runs standalone against the current plan",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `validate-plan parses the current worktree's plan and checks it in one of
six modes, reporting one JSON envelope that carries informational findings
under their own key even on success. It takes no arguments.

No flag: the format-only check set Plan-Write's and Plan-Burler-Fix's own gate
runs, which the plan writer calls before handoff. Cards of batches webster's
run record holds done are history, and their tree-dependent checks are skipped.
--require-approved: the same set plus the plan-unapproved check no gate runs.
--rework: the format-only checks over the whole new plan plus a check that
first_card is one past the highest card of the plan committed at HEAD.
--lens-review: the no-flag checks plus a check that lens-review.md holds a
recorded lens review.

--overlay: the no-flag checks over the plan with proposed contents laid over
its files, in memory. Pipe one JSON object on stdin, keys the file names and
values their full content. A name is 00-overview.md or a card file the Card
Index lists; adding, removing or renumbering a card goes through insert-card
and remove-card. {} checks the plan as it is.

--part <name>: the check a part writer's own gate runs over the named part's
plan in the parts directory, with the part's external exports and the shared
spots in scope, plus a check that every export the part's contract names is met.
It runs without the decision record and applies only to a run whose recipe has
a Parts-Plan-Write row.

The five flags are mutually exclusive.

Example:
  lym loom validate-plan
  lym loom validate-plan --part <name>
  lym loom validate-plan --require-approved
  lym loom validate-plan --rework
  lym loom validate-plan --lens-review
  lym loom validate-plan --overlay <<'EOF'
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

			if part != "" {
				for _, other := range []struct {
					set  bool
					name string
				}{{requireApproved, "--require-approved"}, {rework, "--rework"}, {lensReview, "--lens-review"}, {overlay, "--overlay"}} {
					if other.set {
						clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-plan: --part and "+other.name+" are mutually exclusive; way forward: run --part alone, which checks one part's plan"))
						return nil
					}
				}

				return c.validatePart(cmd, part)
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

			var pc loomshed.PlanContext
			if !requireApproved {
				pc, err = c.planContext()
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}

			var findings []planglyph.Finding
			switch {
			case requireApproved:
				findings, err = c.env.PlanIndex.Validate(plan, c.env.WorktreeRoot)
			case rework:
				findings, err = loomshed.ValidateReworkPlan(plan, c.env.WorktreeRoot, c.env.PlanIndex, c.env.Rework.ReadCommitted, pc)
			case lensReview:
				findings, err = loomshed.ValidatePlanLensReview(plan, c.env.AnchorPath, c.env.WorktreeRoot, c.env.PlanIndex, pc)
			default:
				findings, err = loomshed.ValidatePlan(plan, c.env.AnchorPath, c.env.WorktreeRoot, c.env.PlanIndex, pc)
			}
			if err != nil {
				// Quarry being unavailable is reported as such, never as an invalid plan:
				// an operator reading this envelope must not be told the plan is invalid when quarry simply could not answer.
				// The validator classifies this error the same way.
				// Any other error still fails the verb, with its own wording:
				// reporting the plan valid over a validation that did not finish is the failure mode this rules out.
				if errors.Is(err, planglyph.ErrQuarryUnavailable) {
					clihelp.SetExit(cmd.Context(), output.Err(out, "loom: quarry could not answer validating plan at "+planDir+": "+err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validating plan at "+planDir+" failed: "+err.Error()))
				return nil
			}

			reportPlanFindings(cmd, planDir, findings)
			return nil
		},
	}

	cmd.Flags().BoolVar(&requireApproved, "require-approved", false, "also run the plan-unapproved approval check, the one check no gate runs -- this flag is the sole way to reach it standalone")
	cmd.Flags().BoolVar(&rework, "rework", false, "check the whole new plan, and that its first_card is one past the highest card committed at HEAD")
	cmd.Flags().BoolVar(&lensReview, "lens-review", false, "run the flag-less check set plus the check that the plan directory's lens-review.md holds a recorded lens review, as a twill Plan-Write run's own gate does")
	cmd.Flags().BoolVar(&overlay, "overlay", false, "run the no-flag check set over the plan with the proposed plan file contents on stdin (one JSON object, file name to full content) laid over it, in memory, writing nothing")
	cmd.Flags().StringVar(&part, "part", "", "check the named part's plan in the parts directory, as a part writer's own gate does; only on a run whose recipe has a Parts-Plan-Write row")

	return cmd
}

// planContext builds the context the plan checks read beyond the plan itself.
// It holds the language's rename tools, the gate's raw-command prefixes and tier tags:
// the seam-test reader and the pattern record.
// An absent gate.yaml refuses no raw command, so the verbs stay usable in a repo without one.
// Every error names validate-plan and the config to fix.
func (c *loomCLI) planContext() (loomshed.PlanContext, error) {
	language, err := languageconfig.Load(fabricengine.ConfigDirs(c.location))
	if err != nil {
		return loomshed.PlanContext{}, fmt.Errorf("loom: validate-plan: %w", err)
	}
	var refuseRaw, tierTags []string
	gateCfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(c.location))
	switch {
	case err == nil:
		refuseRaw = gateCfg.RefuseRaw
		tierTags = gateTierTags(gateCfg)
	case !errors.Is(err, gateslot.ErrConfigAbsent):
		return loomshed.PlanContext{}, fmt.Errorf("loom: validate-plan: %w", err)
	}

	return loomshed.PlanContext{
		RenameTools:        language.RenameTools,
		RefuseRaw:          refuseRaw,
		TierTags:           tierTags,
		SeamTests:          loomshed.SeamTestReader(c.env.WorktreeRoot),
		DecisionRecordPath: c.env.PatternRecordPath,
		PatternBase:        loomshed.PatternBaseReader(c.env.WorktreeRoot, c.env.VerifyMergeBase),
	}, nil
}

// reportPlanFindings sets cmd's exit from a plan check's findings.
// A blocking finding makes an error envelope carrying all of them; otherwise the envelope is ok and names planDir.
// Informational findings ride the ok envelope under their own key,
// so a plan that introduces a new package is never reported as if nothing had been said about it.
func reportPlanFindings(cmd *cobra.Command, planDir string, findings []planglyph.Finding) {
	out := cmd.OutOrStdout()
	if planFindingsHaveBlocking(findings) {
		clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: plan is not yet valid", map[string]any{
			"findings": renderFindings(findings),
		}))
		return
	}

	fields := map[string]any{
		"plan_dir": planDir,
	}
	if len(findings) > 0 {
		fields["findings"] = renderFindings(findings)
	}
	clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
}

// validatePart runs `validate-plan --part`: the part check a part writer's own gate runs, over the named part.
// It refuses on a recipe with no Parts-Plan-Write row, reading the recipe's rows to decide.
// A failure of the check is reported in the error envelope with its text; its findings go to the plan report.
func (c *loomCLI) validatePart(cmd *cobra.Command, part string) error {
	out := cmd.OutOrStdout()

	hasParts, err := loomrecipe.HasEngine(c.recipe, partsPlanWriteEngine)
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-plan --part: "+err.Error()))
		return nil
	}
	if !hasParts {
		clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("loom: validate-plan --part does not apply to this run, whose %q recipe has no Parts-Plan-Write row and so no parts; way forward: run \"lym loom validate-plan\" without --part", c.recipe)))
		return nil
	}

	pc, err := c.planContext()
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
		return nil
	}
	findings, err := loomshed.ValidatePart(c.env.AnchorPath, c.env.WorktreeRoot, part, c.env.PlanIndex, pc)
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-plan --part "+part+": "+err.Error()))
		return nil
	}

	reportPlanFindings(cmd, partsparser.PartDir(c.env.AnchorPath, part), findings)
	return nil
}

// validatePartsCmd builds the `validate-parts` subcommand: the standalone form of the check the parts gate runs.
// The Parts-Write agent can call it before handoff.
// It reads the parts file and the decision record and changes nothing.
func (c *loomCLI) validatePartsCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "validate-parts",
		Short:       "run the check the parts gate runs standalone against the current parts file",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `validate-parts runs partsparser.Validate against the current worktree's
parts file -- the identical check the Parts-Write row's own gate runs: the
file's format, the part and shared-spot rules, the decision ids each part
names against the decision record, the most parts parts_max allows, and that
each export is new to the worktree, a plan: handle passing the name checks a
part's plan runs on it -- and
reports the result as one JSON envelope. It takes no arguments and no flags,
and it changes nothing. It sits beside validate-plan, whose --part checks one
part's plan.

Example:
  lym loom validate-parts`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			tapestry, err := loomengine.LoadTapestryConfig(fabricengine.ConfigDirs(c.location), "tapestry")
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-parts: "+err.Error()))
				return nil
			}
			findings, err := partsparser.Validate(c.env.AnchorPath, c.env.WorktreeRoot, c.env.DecisionRecordPath, tapestry.PartsMax)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: validate-parts: "+err.Error()))
				return nil
			}
			if len(findings) > 0 {
				rendered := make([]string, len(findings))
				for i, finding := range findings {
					label := finding.Check
					if finding.Part != "" {
						label += "/" + finding.Part
					}
					rendered[i] = label + ": " + finding.Detail
				}
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: parts file is not yet valid", map[string]any{
					"findings": rendered,
				}))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"parts_path": partsparser.File(c.env.AnchorPath),
			}))
			return nil
		},
	}
}
