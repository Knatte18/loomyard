// resolve.go declares the "resolve" subcommand: the check that a copied spelling names something
// real.

package quarrycli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Knatte18/quarry/quarry"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
)

// newResolveCmd returns the "resolve" subcommand, reading the resolved repository root through
// root at call time.
func newResolveCmd(root func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <glyph>...",
		Short: "Check whether one or more copied glyph spellings name something real",
		Long: `resolve resolves one or more glyphs, positionally, in one call against the current
worktree, and answers each glyph separately with quarry's own JSON rendering,
unchanged, in argument order.
A glyph that is not found, ambiguous, or rejected by the grammar gets its own answer
(carrying "status", or "error" and "reason" for a grammar rejection) and makes the exit non-zero,
but every answer is still printed.
The {"ok":false,...} envelope appears only when quarry could not answer at all.

Example:
  lyx quarry resolve internal/planglyph#Validate internal/planglyph#Repo`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			results, err := planglyph.Resolve(root(), args)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			rendered := make([][]byte, 0, len(results))
			negative := false
			for _, r := range results {
				data, err := quarry.RenderResolveJSON(r)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				rendered = append(rendered, data)
				if r.Rejected() || (r.Status != quarry.StatusFound && r.Status != quarry.StatusMultipart) {
					negative = true
				}
			}

			for _, data := range rendered {
				if _, err := out.Write(data); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}
			if negative {
				clihelp.SetExit(cmd.Context(), 1)
			}
			return nil
		},
	}
}

// describeRejectedResolve formats r's own fields into one diagnostic line for a resolve result
// that is not found, is ambiguous, or was rejected by the glyph grammar before resolution -- the
// pre-resolution rejection case, where Error carries the message and Status is empty.
func describeRejectedResolve(r quarry.ResolveResult) string {
	if r.Error != "" {
		return fmt.Sprintf("%s: %s", r.Target, r.Error)
	}
	return fmt.Sprintf("%s: %s", r.Target, r.Status)
}
