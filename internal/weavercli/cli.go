// cli.go builds the cobra command tree of the weaver module and its pre-run, which admits a hub's task worktree alone.

package weavercli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// outsideHubRefusal is the refusal of every verb run outside a hub's task worktree.
const outsideHubRefusal = "weaver: lym weaver reset runs only in a hub's task worktree, where Weaver runs as a loom row; way forward: run it from the task worktree of the run to reset"

// weaverCLI is the receiver of every weaver verb.
// Its fields are the seams the verbs run through, which tests replace.
type weaverCLI struct {
	// resolve reports the location and mode of cwd.
	resolve func(cwd string) (*lymcwd.Location, preflight.Mode, error)
	// resetDepsFor builds the seams Reset is told from the hub location the pre-run resolved.
	resetDepsFor func(loc *lymcwd.Location) (weaverengine.ResetDeps, error)
	// runReset runs the reset and prints its envelope.
	runReset func(out io.Writer, deps weaverengine.ResetDeps, stopStrands bool) int
	// loc is the hub location the pre-run resolved.
	loc *lymcwd.Location
}

// Command returns the cobra command tree for the weaver module.
func Command() *cobra.Command {
	return newCommand(&weaverCLI{resolve: preflight.ResolveMode, resetDepsFor: resetDepsFor, runReset: resetVerb})
}

// newCommand builds the weaver command tree over c.
func newCommand(c *weaverCLI) *cobra.Command {
	parent := &cobra.Command{
		Use:   "weaver",
		Short: "the weaver implementer's verbs",
		Long: `weaver holds the verbs of Weaver, the cold-start implementer that builds a plan's
batches as cold strands and climbs a model ladder when an attempt fails.
Weaver runs as a loom row, so its verbs run only in a hub's task worktree.

Reach for "lym weaver reset" to start a stopped run over.

Example:
  lym weaver reset`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}
	parent.AddCommand(c.resetCmd())
	return parent
}

// resolvePersistentPreRun resolves the working directory and refuses outside a hub's task worktree.
// It skips resolution when the group command itself runs, a bare listing or an unknown subcommand.
func (c *weaverCLI) resolvePersistentPreRun(cmd *cobra.Command, _ []string) error {
	if cmd.Name() == "weaver" {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	loc, mode, err := c.resolve(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	if mode != preflight.ModeHub {
		output.Err(out, outsideHubRefusal)
		clihelp.Abort(ctx, 1)
		return nil
	}
	c.loc = loc
	return nil
}
