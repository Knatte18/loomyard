// cli.go builds the cobra command tree for the reed module and the RunCLI seam that wires it into
// the standard io.Writer-based call contract.
// The parent "reed" command carries a PersistentPreRunE that resolves
// cwd -> location -> config -> geometry -> *reedengine.Engine exactly once per invocation,
// into a receiver every verb (up.go, add.go, remove.go, status.go, resume.go, attach.go,
// statusline.go, watchdog.go) closes over, so no subcommand re-resolves geometry or config itself.
// The geometry step is hubgeom.ReedGeometry: this file is where the resolved Location becomes the
// reedengine.Geometry the engine is told, and the engine never sees the Location.
// The resolved *lyxcwd.Location is named "location" throughout, never "layout": "layout" is a live
// first-class term in this very module (the tmux window_layout string, planLayout,
// applyLayoutLocked, select-layout), and it is also the name of the Engine field that held a
// Location before the told-geometry refactor replaced it with geom.

package reedcli

import (
	"io"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// reedCLI carries the resolved *reedengine.Engine for each reed verb; the zero value is invalid until PersistentPreRunE populates eng.
type reedCLI struct {
	eng *reedengine.Engine

	// hubPath is the hub path PersistentPreRunE stored off the resolved *lyxcwd.Location — the hub
	// path alone, deliberately not the whole Location, since ensureWatchdogSpawned needs nothing
	// else from it and a stored Location would invite other code to re-read geometry the engine was
	// already told.
	hubPath string

	// suppressWatchdogSpawn, when true, makes ensureWatchdogSpawned a no-op. It is initialised from
	// testing.Testing() in Command(): re-exec'ing os.Executable() from a test binary runs the whole
	// suite recursively, the same hazard and the same shape as the Engine.suppressHeaderLaunch field
	// batch 3 deleted, relocated to the layer that now owns the spawn. An in-package test flips it
	// back off to drive the real spawn path.
	suppressWatchdogSpawn bool
}

// Command returns the cobra command tree for the reed module.
//
// The parent "reed" command carries a PersistentPreRunE that resolves cwd -> location -> config -> geometry ->
// *reedengine.Engine into c, skipping that resolution entirely when the group command itself is
// invoked (bare "lyx reed" listing or an unknown-subcommand error via GroupRunE) so neither path
// requires a git repository.
// Every verb card (22-27) creates its own (c *reedCLI) xCmd() builder and registers it here via
// parent.AddCommand — this card registers no subcommands itself.
func Command() *cobra.Command {
	c := &reedCLI{suppressWatchdogSpawn: testing.Testing()}

	parent := &cobra.Command{
		Use:   "reed",
		Short: "manage the tmux strand overlay for this worktree",
		Long: `reed drives a per-hub tmux server that lays out one strand per pane for
this worktree's session: adding, removing, resuming, and attaching to
strands, plus rendering their layout on every mutation.

The tmux session is named after this worktree's directory, so that name must
carry none of ".", ":", "\" or any control character or invalid UTF-8 — tmux
silently rewrites all of these and would then create a session reed can never
address or tear down. Every reed verb refuses up front, naming the directory,
rather than booting substrate it cannot reach.`,
		// RunE is set so that bare "lyx reed" lists subcommands and "lyx reed bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the reed group command itself is invoked (bare listing or
			// unknown-subcommand error path via GroupRunE), or when the watchdog daemon is
			// invoked, skip cwd/location/config resolution entirely. The daemon is told its hub
			// path on its own flags and must never be reached through c.eng — letting it run the
			// normal pre-run would make it refuse to start outside a worktree and hold a geometry
			// it must not use.
			if cmd.Name() == "reed" || cmd.Name() == "watchdog" {
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
				// lyxcwd.Resolve's error is already self-describing (it IS the
				// "not a git repository" sentinel); pass it through bare rather than
				// doubling that same text on top of it.
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// The _lyx/config/ root is anchored at location.AnchorPath(), not WorktreeRoot or
			// any fabric sibling — reed config lives with the worktree the operator is
			// actually standing in.
			cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			reedGeom := hubgeom.ReedGeometry(location)
			c.eng = reedengine.New(cfg, reedGeom)
			c.hubPath = location.HubPath
			return nil
		},
	}

	parent.AddCommand(c.upCmd(), c.downCmd(), c.addCmd(), c.removeCmd(), c.statusCmd(), c.resumeCmd(), c.attachCmd(), c.statuslineCmd(), c.watchdogCmd())

	return parent
}

// RunCLI is the public seam for the reed module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the capture writer
// for all output (including cobra's error text).
// This preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
// The branch exists because lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to
// ExecuteIn would panic on every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
