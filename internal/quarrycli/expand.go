// expand.go declares the "expand" subcommand: the detail query over a type glyph that replaces
// the stencil's old lookups.

package quarrycli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Knatte18/quarry/quarry"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
)

// newExpandCmd returns the "expand" subcommand, reading the resolved repository root through root
// at call time.
func newExpandCmd(root func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "expand <glyph>",
		Short: "Report a type glyph's own head plus every member whose owner chain begins with it",
		Long: `expand answers target -- the type's own head plus every member whose owner chain
begins with it -- and emits quarry's own JSON rendering unchanged when target is
found. A target that is not found, is ambiguous, is rejected by the glyph grammar,
or does not name a type fails the whole invocation with a JSON error envelope and
a non-zero exit.

Example:
  lyx quarry expand internal/planglyph#Repo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			answer, err := planglyph.Expand(root(), args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if answer.Status != quarry.StatusFound {
				msg := fmt.Sprintf("%s: %s", args[0], answer.Status)
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}

			data, err := quarry.RenderExpandJSON(answer)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if _, err := out.Write(data); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
			}
			return nil
		},
	}
}
