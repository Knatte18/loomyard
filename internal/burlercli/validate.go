// validate.go implements the `validate-review` burler verb.
// The verb is a read-only self-check of a review file: it runs the same parse every burler round's review gate runs.
// It also runs the round checks on a review that sits in a run directory.

package burlercli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/spf13/cobra"
)

// validateReviewVerb is the cobra name of the validate-review verb.
// The verb skips hub, mode and engine wiring.
// It resolves the worktree itself, from the seam cwd, when it runs the round checks.
const validateReviewVerb = "validate-review"

// validateReviewCmd builds `lym burler validate-review <review-file>`.
// It prints ok with the absolute review file when the file parses and passes the round checks it is due.
// Otherwise it prints the refusal verbatim and exits with 1.
// A relative path resolves against the seam cwd.
func (c *burlerCLI) validateReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:         validateReviewVerb + " <review-file>",
		Short:       "check that a review file passes the checks a round's review gate runs",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `validate-review runs the checks a burler round's review gate runs on its
review file. It first runs the parse (burlerengine.CheckReviewFile, over
ParseCurrentReview): YAML frontmatter carrying a legal verdict and well-formed
findings, each with a non-empty fix key. The frontmatter's top-level keys are
verdict, findings and prior, and the body holds exactly one "### [SEVERITY]"
prose block per finding, matching the frontmatter's count per severity.

When the file is a round-<n>-review.md in a run directory under the review
runs of the worktree the current directory resolves to, it then runs the round
checks (burlerengine.CheckReviewRound) against the round before it: the prior
list, the carries links and the BLOCKING bound of an incremental round. The
round before is read from the run directory, and the decision record is read as
it stands, so an entry added while the reviewer works counts here and not in
the round's own gate. Any other file, and a current directory that resolves to
no worktree, gets the parse alone. A round context that cannot be built is
named in the round_checks field of the ok answer, and the parse decides.

It prints ok with the absolute review file, and the refusal verbatim with exit
1 when a check refuses. It is read-only.

Example:
  lym burler validate-review review.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			reviewFile := args[0]
			if !filepath.IsAbs(reviewFile) {
				reviewFile = filepath.Join(cwd, reviewFile)
			}
			reviewFile = filepath.Clean(reviewFile)

			if err := burlerengine.CheckReviewFile(reviewFile); err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			refusal, builderNote := checkRoundInRunDirectory(cwd, reviewFile)
			if refusal != nil {
				output.Err(out, refusal.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			answer := map[string]any{"review_file": reviewFile}
			if builderNote != "" {
				answer["round_checks"] = builderNote
			}
			output.Ok(out, answer)
			return nil
		},
	}
}

// checkRoundInRunDirectory runs the round checks on the review at reviewFile when it sits in a run directory.
// The worktree comes from cwd alone, never from reviewFile's path, since `_lym` is a link out of the worktree.
// The checks run only when cwd resolves, the file is named round-<n>-review.md,
// and its directory lies under the worktree's review runs.
// refusal is the first check the review fails.
// builderNote is set when the round context could not be built, so only the parse decided.
func checkRoundInRunDirectory(cwd, reviewFile string) (refusal error, builderNote string) {
	location, err := lymcwd.ResolveWithin(cwd)
	if err != nil {
		return nil, ""
	}
	round, ok := shedadapters.ParseRoundReviewName(filepath.Base(reviewFile))
	runDir := filepath.Dir(reviewFile)
	if !ok || !isBelow(runDir, loomengine.LoomReviewsDir(location)) {
		return nil, ""
	}

	paths := shedadapters.RoundContextSources{DecisionRecordPath: loomengine.DiscussionDecisionRecord(location), AnchorPath: location.AnchorPath()}
	// The plan segment's review and fix rows share the run_subdir of its Bouncer row, which the loom recipe gives.
	planSubdir, found, err := loomrecipe.BouncerRunSubdir(loomshed.NamePlanBouncer)
	if err != nil {
		return nil, fmt.Sprintf("the plan review run directory could not be read from the recipe, so the parse alone decided: %s", err)
	}
	if found && filepath.Clean(runDir) == filepath.Join(loomengine.LoomReviewsDir(location), planSubdir) {
		amendmentPath := loomengine.LoomAmendmentRecord(location)
		paths.AmendmentTitles = func() ([]string, error) {
			record, _, err := loomshed.ReadAmendment(amendmentPath)
			return record.Titles, err
		}
		paths.ConflictDir = runplan.ConflictDir(location.AnchorPath())
	}
	roundContext, err := shedadapters.BuildRoundContext(runDir, round, paths)
	if err != nil {
		return nil, fmt.Sprintf("the round context could not be built, so the parse alone decided: %s", err)
	}
	if roundContext != nil {
		roundContext.WorktreeRoot = location.WorktreePath()
	}
	content, err := os.ReadFile(reviewFile)
	if err != nil {
		return fmt.Errorf("burler: read review file %q: %w", reviewFile, err), ""
	}
	return burlerengine.CheckReviewRound(content, roundContext), ""
}

// isBelow reports whether dir is a directory strictly below root, comparing the cleaned paths without resolving links.
func isBelow(dir, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(dir))
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
