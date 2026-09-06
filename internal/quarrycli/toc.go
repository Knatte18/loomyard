// toc.go declares the "toc" subcommand: the structure query the stencil's replaced lookup
// instructions point the planner at.

package quarrycli

import (
	"github.com/spf13/cobra"

	"github.com/Knatte18/quarry/quarry"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
)

// newTOCCmd returns the "toc" subcommand, reading the resolved repository root through root at
// call time.
func newTOCCmd(root func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "toc <path>",
		Short: "Report the table-of-contents structure under a repository-relative path",
		Long: `toc answers a table-of-contents query for path -- a repository-relative directory or
file path, with "" and "." both meaning the repository root -- and emits quarry's
own JSON rendering unchanged.

Example:
  lyx quarry toc internal/planglyph`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			answer, err := planglyph.TOC(root(), args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			data, err := quarry.RenderJSON(answer)
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
