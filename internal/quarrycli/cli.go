// cli.go builds the cobra command tree for the quarry module: toc, glyphs, resolve, and expand.
// The quarry group resolves the repository root once, in its PersistentPreRunE, through preflight.ResolveMode(cwd).
// It skips that resolution for a bare "lym quarry" listing, so the listing needs no git repository.
//
// quarrycli is a named deviation from the CLI/Cobra Invariant's package-naming rule (PATTERN-cli-cobra):
// its kernel is internal/planglyph, not quarryengine.

package quarrycli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// Command returns the cobra command tree for the quarry module.
func Command() *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "quarry",
		Short: "query the repository's glyph alphabet -- the planner's only source of glyph spellings",
		Long: `quarry answers four read-only repository queries against the current worktree's own
glyph alphabet: glyphs, resolve, toc and expand. Every answer is quarry's own
rendering, emitted verbatim -- these verbs never re-shape, filter, or re-key an
answer, and none of them takes a repository-path flag, so a copied spelling is
always from the tree the plan validator resolves against.

Examples:
  lym quarry glyphs internal/planglyph
  lym quarry resolve internal/planglyph#Validate
  lym quarry toc internal/planglyph
  lym quarry expand internal/planglyph#Repo`,
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "quarry" {
				return nil
			}

			ctx := cmd.Context()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			loc, mode, err := preflight.ResolveMode(cwd)
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// ResolveMode returns a nil *lymcwd.Location in standalone mode, so loc is read only on the hub branch.
			// The standalone branch takes the root from cwd itself.
			if mode == preflight.ModeHub {
				root = loc.WorktreePath()
			} else {
				root = cwd
			}
			return nil
		},
	}

	rootFn := func() string { return root }

	cmd.AddCommand(newTOCCmd(rootFn), newGlyphsCmd(rootFn), newResolveCmd(rootFn), newExpandCmd(rootFn))
	return cmd
}

// RunCLI runs the quarry command tree with args, writes its output to out, and returns the exit code.
// It resolves the repository root from the process working directory.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the quarry command tree with args, writes its output to out, and returns the exit code.
// The group resolves the repository root from cwd.
// A non-empty cwd is seeded into the execution context through clihelp.ExecuteIn;
// an empty cwd reads the process working directory through clihelp.Execute.
// The branch exists because lymcwd.WithCwd panics on an empty directory, so ExecuteIn cannot take an empty cwd.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
