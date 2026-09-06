// cli.go exposes the cobra command tree for the quarry module: toc, glyphs, resolve, and expand,
// the planner's only source of glyph spellings.
// The repository root is resolved once in the parent's PersistentPreRunE via
// preflight.ResolveMode(cwd), skipping resolution when cmd.Name() is "quarry" so a bare "lyx
// quarry" listing never requires a git repository, following the pattern
// internal/stencilcli/cli.go and internal/webstercli/wiring.go both establish.
//
// quarrycli is a named deviation from the CLI/Cobra Invariant's package-naming rule: its kernel is
// internal/planglyph, not quarryengine. See CONSTRAINTS.md.

package quarrycli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// Command returns the cobra command tree for the quarry module.
func Command() *cobra.Command {
	var root string

	cmd := &cobra.Command{
		Use:   "quarry",
		Short: "Query the repository's glyph alphabet -- the planner's only source of glyph spellings",
		Long: `quarry answers four read-only repository queries against the current worktree's own
glyph alphabet: glyphs, resolve, toc and expand. Every answer is quarry's own
rendering, emitted verbatim -- these verbs never re-shape, filter, or re-key an
answer, and none of them takes a repository-path flag, so a copied spelling is
always from the tree the plan validator resolves against.

Examples:
  lyx quarry glyphs internal/planglyph
  lyx quarry resolve internal/planglyph#Validate
  lyx quarry toc internal/planglyph
  lyx quarry expand internal/planglyph#Repo`,
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "quarry" {
				return nil
			}

			ctx := cmd.Context()

			cwd, err := lyxcwd.CwdFrom(ctx)
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

			// ResolveMode returns a nil *lyxcwd.Location at every one of its three ModeStandalone
			// return sites, so loc is read only on the hub branch; the standalone branch takes the
			// root from cwd itself, mirroring the two-mode split internal/webstercli/wiring.go
			// already makes.
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

// RunCLI is the public seam for the quarry module CLI.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
// The branch exists because lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to
// ExecuteIn would panic on every existing RunCLI call. RunCLIIn is required here, not optional: the
// group's root resolution is cwd-dependent.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
