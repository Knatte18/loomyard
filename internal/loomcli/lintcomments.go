// lintcomments.go implements the `lint-comments` loom verb: the standalone form of the comment line-break lint every card gate runs,
// a zero-positional caller of the same commentlint.Lint the round gate calls.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/commentlint"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// renderCommentFindings renders each finding as `file:line: text`, the payload under the envelope's "findings" key.
func renderCommentFindings(findings []commentlint.Finding) []string {
	rendered := make([]string, 0, len(findings))
	for _, f := range findings {
		rendered = append(rendered, fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Text))
	}
	return rendered
}

// lintCommentsCmd builds the `lint-comments` subcommand.
// It only reads git and files; a finding fails the gate whose command runs it, never the run.
func (c *loomCLI) lintCommentsCmd() *cobra.Command {
	var base string

	cmd := &cobra.Command{
		Use:   "lint-comments",
		Short: "find fixed-column-wrapped line breaks in the comments the current change creates",
		Long: `lint-comments runs commentlint.Lint over the current worktree -- the check
every card gate runs after its tests -- and reports the result as one JSON
envelope. Without --base it lints the working tree against HEAD, untracked Go
files included; with --base it lints that commit against HEAD. A finding is
a line of a // comment block that a new break leaves ending in the middle of
a sentence; the envelope lists each as file:line: text. It takes no
arguments.

Example:
  lyx loom lint-comments
  lyx loom lint-comments --base 1a2b3c4`,
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
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: comments break a sentence at a fixed column", map[string]any{
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
