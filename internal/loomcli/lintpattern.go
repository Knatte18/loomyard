// lintpattern.go implements the `lint-pattern` loom verb, the standalone form of the PATTERN headroom lint every card gate runs.
// It is a caller of the same patternlint.Lint the round gate calls.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/patternlint"
	"github.com/spf13/cobra"
)

// lintPatternCmd builds the `lint-pattern` subcommand.
// It only reads git and files; a finding fails the gate whose command runs it, never the run.
func (c *loomCLI) lintPatternCmd() *cobra.Command {
	var base string

	cmd := &cobra.Command{
		Use:         "lint-pattern",
		Short:       "find growth of the PATTERN overview past its headroom line in the current change",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `lint-pattern runs patternlint.Lint over the current worktree -- the check
every card gate runs after lint-comments -- and reports the result as one JSON
envelope. Without --base it compares the working tree's PATTERN.md with
HEAD's; with --base it compares HEAD's with that commit's. A finding is an
overview that grew past the headroom line below its size cap; the envelope
lists it with the bytes to cut. An overview that stays above the line or
shrinks passes. It takes no arguments.

Example:
  lyx loom lint-pattern
  lyx loom lint-pattern --base 1a2b3c4`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			finding, found, err := patternlint.Lint(c.env.WorktreeRoot, base)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: lint pattern in "+c.env.WorktreeRoot+": "+err.Error()))
				return nil
			}
			if found {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, "loom: PATTERN.md grew past its headroom line; "+patternlint.WayForward, map[string]any{
					"findings": []string{finding.Message},
				}))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"worktree": c.env.WorktreeRoot,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&base, "base", "", "compare this commit's overview with HEAD's instead of the working tree's with HEAD's")

	return cmd
}
