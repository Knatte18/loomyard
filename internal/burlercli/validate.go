// validate.go implements the `validate-review` burler verb: the read-only self-check of a review file,
// running the same parse the review gate of every burler round runs.

package burlercli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// validateReviewVerb is the cobra name of the validate-review verb.
// resolvePersistentPreRun skips hub, mode and engine wiring for it, since the check reads one told file.
const validateReviewVerb = "validate-review"

// validateReviewCmd builds `lyx burler validate-review <review-file>`.
// It prints ok with the absolute review file when the file parses, and the parse error verbatim with exit 1 otherwise.
// A relative path resolves against the seam cwd.
func (c *burlerCLI) validateReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:         validateReviewVerb + " <review-file>",
		Short:       "check that a review file parses, as a round's review gate does",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `validate-review runs the parse every burler round's review gate runs on its
review file (burlerengine.CheckReviewFile, over ParseCurrentReview): YAML
frontmatter carrying a legal verdict and well-formed findings, each with a
non-empty fix key. The frontmatter's only top-level keys are verdict and
findings, and the body holds exactly one "### [SEVERITY]" prose block per
finding, matching the frontmatter's count per severity. It prints ok with the
absolute review file when the file parses, and the parse error verbatim with
exit 1 when it does not. It is read-only and needs no hub or git repository.

Example:
  lyx burler validate-review review.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			reviewFile := args[0]
			if !filepath.IsAbs(reviewFile) {
				reviewFile = filepath.Join(cwd, reviewFile)
			}

			if err := burlerengine.CheckReviewFile(reviewFile); err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			output.Ok(out, map[string]any{"review_file": reviewFile})
			return nil
		},
	}
}
