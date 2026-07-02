// remove.go implements the `remove` mux verb: deletes a strand by guid,
// requiring --recursive for a non-leaf so children are never silently
// orphaned, and reports every strand actually removed.

package muxcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// removeCmd builds the `remove <guid>` subcommand: deletes the strand
// identified by the single positional guid argument. Removing a non-leaf
// without --recursive is rejected by the engine ("strand has children, use
// --recursive"); with --recursive the whole descendant subtree cascades
// away. The success envelope lists every strand actually removed.
func (c *muxCLI) removeCmd() *cobra.Command {
	var recursive bool

	cmd := &cobra.Command{
		Use:   "remove <guid>",
		Short: "remove a strand from the mux layout",
		Long: `remove deletes the strand identified by <guid>. Removing a strand that
has children requires --recursive, which cascades the removal through the
strand's whole descendant subtree; without --recursive a non-leaf remove is
rejected outright, so children are never silently orphaned.

Example:
  lyx mux remove 3fae21ac9b1d4c0e --recursive`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			removed, err := c.eng.RemoveStrand(args[0], recursive)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			list := make([]map[string]any, len(removed.Strands))
			for i, s := range removed.Strands {
				list[i] = map[string]any{"guid": s.GUID, "name": s.Name}
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"removed": list,
			}))
			return nil
		},
	}

	cmd.Flags().BoolVar(&recursive, "recursive", false, "cascade removal through the strand's whole descendant subtree")

	return cmd
}
