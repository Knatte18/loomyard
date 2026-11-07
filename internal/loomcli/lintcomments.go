// lintcomments.go implements the `lint-comments` loom verb: a lint of the comment lines in the current change.
// It calls `commentlint.Lint`, the check the gate's `comments` lint step also runs inside `lym gate test`.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/commentlint"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// renderCommentFindings renders each finding as `file:line: rule: text`, the payload under the envelope's "findings" key.
func renderCommentFindings(findings []commentlint.Finding) []string {
	rendered := make([]string, 0, len(findings))
	for _, f := range findings {
		rendered = append(rendered, f.String())
	}
	return rendered
}

// lintCommentsCmd builds the `lint-comments` subcommand.
// It only reads git and files; a finding fails the gate whose command runs it, never the run.
func (c *loomCLI) lintCommentsCmd() *cobra.Command {
	var base string

	cmd := &cobra.Command{
		Use:         "lint-comments",
		Short:       "find fixed-column-wrapped breaks and over-long lines in the comments the current change creates",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `lint-comments runs commentlint.Lint over the current worktree -- the check
a gate key's comments lint step also runs inside lym gate test -- and reports the result as one JSON
envelope. Without --base it lints the working tree against HEAD, untracked Go
files included; with --base it lints that commit against HEAD. A finding is
a line of a // comment block that a new break leaves ending in the middle of
a sentence, or a new or changed line whose text reaches the comment
line-length limit; the envelope lists each as file:line: rule: text. It
takes no arguments.

Example:
  lym loom lint-comments
  lym loom lint-comments --base 1a2b3c4`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			findings, err := commentlint.Lint(c.env.WorktreeRoot, base)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: lint comments in "+c.env.WorktreeRoot+": "+err.Error()))
				return nil
			}
			if len(findings) > 0 {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: comment lines wrap at a fixed column or run past the line-length limit", map[string]any{
					"findings": renderCommentFindings(findings),
				}))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"worktree": c.env.WorktreeRoot,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&base, "base", "", "lint this commit against HEAD instead of the working tree against HEAD")

	return cmd
}
