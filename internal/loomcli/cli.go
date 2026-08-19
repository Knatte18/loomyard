// cli.go builds the cobra command tree for the loom module and the RunCLI seam that wires it into
// the standard io.Writer-based call contract.
// The parent "loom" command carries a PersistentPreRunE that resolves cwd once into a
// *lyxcwd.Location and delegates the whole engine-stack construction to c.wire (wiring.go), storing
// the resolved ingredients on loomCLI exactly once per invocation.
// loom is hub-only: it resolves through lyxcwd.Resolve the way internal/reedcli does, never through
// preflight.ResolveMode's degrade-to-standalone probe -- loom needs a wired fabric and a real weft
// sibling to commit its seed into, so there is no meaningful standalone mode (see the plan's
// hub-only-resolution-for-loom decision). The resolved value is named "location" throughout, never
// "loc" or "cwd".
package loomcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// loomCLI carries the fields wire (wiring.go) populates; every loom verb hangs off this receiver.
type loomCLI struct {
	// location is the resolved *lyxcwd.Location for this invocation. wire reads it to anchor every
	// config load and to build the reed/webster geometries; no verb needs it directly.
	location *lyxcwd.Location
	// cwd is the resolved cwd string the pre-run read via lyxcwd.CwdFrom. loomshed.NewPreflightProducer
	// reads it -- Preflight is the one row that spawns git, over this exact cwd.
	cwd string
	// cfg is the loaded loom.yaml config. Batch 5's run/bootstrap verb reads its Discussion/Plan
	// role model-specs and timeout knobs.
	cfg loomengine.Config
	// reed is the constructed reed engine. Batch 5's bootstrap spawn machinery reads it to add and
	// resolve the driver's own strand.
	reed *reedengine.Engine
	// deps is the assembled loomshed.Deps. driveCmd (drive.go) passes it to loomshed.New to build
	// the phase machine, and statusCmd/pauseCmd read its StatusPath/StatusLockPath pair.
	deps loomshed.Deps
	// runDeps is the assembled websterengine.RunDeps, embedded verbatim as deps.WebsterDeps. It is
	// also kept here directly so a test can inspect it without unwrapping deps.
	runDeps websterengine.RunDeps
}

// runnerMasterStarter adapts *shuttleengine.Runner to websterengine.MasterStarter.
//
// This is a deliberate duplication of the identical adapter in internal/webstercli, per the plan's
// duplicated-cli-adapters-over-a-cli-to-cli-import decision: both types are unexported in their home
// packages, and a <module>cli importing another <module>cli has no precedent in this repo and would
// couple two independent cobra seams. Two small duplications are cheaper than that coupling.
type runnerMasterStarter struct {
	runner *shuttleengine.Runner
}

// StartMaster implements websterengine.MasterStarter.
func (s runnerMasterStarter) StartMaster(spec shuttleengine.Spec) (websterengine.MasterHandle, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// resolvePersistentPreRun resolves cwd via lyxcwd.CwdFrom, resolves it into a *lyxcwd.Location via
// lyxcwd.Resolve, and delegates the whole engine-stack construction to c.wire (wiring.go), storing
// the resolved ingredients on c.
// Skips resolution entirely when the loom group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present.
func (c *loomCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "loom" {
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

	if err := c.wire(location, cwd); err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	return nil
}

// Command returns the cobra command tree for the loom module.
func Command() *cobra.Command {
	c := &loomCLI{}

	parent := &cobra.Command{
		Use:   "loom",
		Short: "bootstrap and drive one loom task's phase machine for this worktree",
		Long: `loom drives one task's phase machine (Preflight, Discussion, Plan,
Batchifier, Webster, Publish, Finalize) over a per-worktree status.json,
the same shed engine that already drives Hardener. "run" is the bootstrap
verb: it seeds the status file, commits the seed, and spawns/attaches the
detached driver session; "drive" is the no-tmux escape hatch that runs the
phase machine in the foreground for debugging and CI; "status" reports the
current phase and, with --watch, tails it one line at a time; "pause"
requests a pause at the next producer boundary.

Example:
  lyx loom run
  lyx loom drive
  lyx loom status
  lyx loom status --watch
  lyx loom pause`,
		// RunE is set so that bare "lyx loom" lists subcommands and "lyx
		// loom bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.AddCommand(c.runCmd(), c.driveCmd(), c.statusCmd(), c.pauseCmd())

	return parent
}

// RunCLI is the public seam for the loom module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the capture writer
// for all output (including cobra's error text).
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
