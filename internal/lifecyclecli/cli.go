// cli.go builds the cobra command tree for the lifecycle module and the RunCLI/RunCLIIn seams that
// wire it into the standard io.Writer-based call contract.
//
// Package lifecyclecli is the module that drives one task worktree's whole lifecycle as a single
// Shed run from the hub's prime worktree. It imports internal/lifecycleshed and
// internal/lifecyclerecipe, neither of which imports cobra, and it is outside the Fabric Vocabulary
// Invariant's owner set, so no identifier, literal, or comment in this package -- or in either of
// those two it imports -- may name either side of the pair: write "the task worktree", "the pair",
// and "the hub's prime worktree" instead.
package lifecyclecli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/spf13/cobra"
)

// lifecycleCLI carries the fields wire (wire.go) populates; every lifecycle verb hangs off this
// receiver.
type lifecycleCLI struct {
	// location is the resolved *lyxcwd.Location for the hub's prime worktree. wire reads it to
	// anchor every path constructor and every seam.
	location *lyxcwd.Location
	// env is the assembled shedrecipe.Env the run verb passes to lifecyclerecipe.New.
	env shedrecipe.Env
	// shedPaths carries the five told values shedengine.Shed itself reads, which the run verb
	// passes alongside env, and which the status verb reads directly.
	shedPaths lifecyclerecipe.ShedPaths
	// slug is the task slug read from the command's own arguments.
	slug string
	// abandonedSession is the value the Teardown.Shutdown seam records on the receiver -- carried
	// here rather than only returned from the seam, so the run verb can surface it on the envelope
	// after the Shed's own Run has returned.
	abandonedSession string
}

// Command returns the cobra command tree for the lifecycle module.
func Command() *cobra.Command {
	c := &lifecycleCLI{}

	parent := &cobra.Command{
		Use:   "lifecycle",
		Short: "drive one task worktree's whole lifecycle as a single Shed run",
		Long: `lifecycle drives one task worktree's whole lifecycle -- create, run the loom
session to a terminal state, and tear down -- as a single Shed run over a
per-slug status.json. "run" starts or resumes that run for a slug; "status"
reports its current state.

Both verbs run from the hub's prime worktree only: they refuse when invoked
from a task worktree.

Example:
  lyx lifecycle run some-slug
  lyx lifecycle status some-slug`,
		// RunE is set so that bare "lyx lifecycle" lists subcommands and "lyx lifecycle bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.AddCommand(c.runCmd(), c.statusCmd())

	return parent
}

// resolvePersistentPreRun resolves cwd via lyxcwd.CwdFrom, resolves it into a *lyxcwd.Location via
// lyxcwd.Resolve, resolves the prime name via fabricengine.PrimeName, and applies the non-prime
// refusal (refusal.go) before resolving anything further. It then reads the slug from the command's
// own arguments and calls wire.
//
// Skips resolution entirely when the lifecycle group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present.
func (c *lifecycleCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "lifecycle" {
		return nil
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		// lyxcwd.Resolve's error is already self-describing (it IS the "not a git repository"
		// sentinel); pass it through bare rather than doubling that same text on top of it.
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	primeName, primeNameErr := fabricengine.PrimeName(location)
	if refusalErr := refuseNonPrime(location.WorktreeName, primeName, primeNameErr); refusalErr != nil {
		output.Err(out, refusalErr.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}
	c.location = location
	c.slug = slug

	if err := c.wire(location, slug); err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	return nil
}

// RunCLI is the public seam for the lifecycle module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the capture writer
// for all output (including cobra's error text).
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
//
// RunCLIIn is carried rather than skipped for a concrete reason: the path-derivation tests serving
// as the Lifecycle Bookend Invariant's mechanical proxy and the non-prime refusal test both need an
// injectable cwd, and neither is reachable through RunCLI alone. The branch exists because
// lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to ExecuteIn would panic on
// every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
