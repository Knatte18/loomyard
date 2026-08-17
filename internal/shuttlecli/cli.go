// cli.go builds the cobra command tree for the shuttle module and the RunCLI seam that wires it
// into the standard io.Writer-based call contract.
// The parent "shuttle" command carries a PersistentPreRunE that resolves cwd -> layout -> shuttle
// config -> reed config -> reed engine -> claude engine -> shuttleengine.Runner exactly once per
// invocation, into a receiver every verb (run.go, interrupt.go, send.go) closes over, so no
// subcommand re-resolves geometry, config, or engine construction itself.

package shuttlecli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// shuttleCLI is the receiver every shuttle verb hangs off of;
// runner is populated by PersistentPreRunE.
type shuttleCLI struct {
	runner *shuttleengine.Runner
}

// Command returns the cobra command tree for the shuttle module.
// The parent command carries a PersistentPreRunE that resolves geometry and engines, skipping
// resolution when the group command itself is invoked (no git repo required).
func Command() *cobra.Command {
	c := &shuttleCLI{}

	parent := &cobra.Command{
		Use:   "shuttle",
		Short: "run one LLM agent over the file contract via a swappable engine",
		Long: `shuttle drives one LLM agent through a single run: a prompt goes in, the
agent's output files (the file contract) and a classified outcome
(done/asking/died/timeout) come out. The provider itself is swappable behind
an engine seam (Claude Code today) so the run loop and CLI never depend on
provider specifics.`,
		// RunE is set so that bare "lyx shuttle" lists subcommands and "lyx
		// shuttle bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the shuttle group command itself is invoked (bare
			// listing or unknown-subcommand error path via GroupRunE), skip
			// cwd/layout/config/engine resolution so that neither path
			// requires a git repository to be present.
			if cmd.Name() == "shuttle" {
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

			layout, err := lyxcwd.Resolve(cwd)
			if err != nil {
				// lyxcwd.Resolve's error is already self-describing (it
				// IS the "not a git repository" sentinel); pass it through
				// bare rather than doubling that same text on top of it.
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// Both configs are anchored at layout.AnchorPath(), matching reedcli's own
			// resolution: the worktree the operator is actually standing in,
			// never WorktreeRoot or any fabric sibling.
			shuttleCfg, err := shuttleengine.LoadConfig(layout.AnchorPath(), "shuttle")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			reedCfg, err := reedengine.LoadConfig(layout.AnchorPath(), "reed")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			reedGeom := hubgeom.ReedGeometry(layout)
			reedEngine := reedengine.New(reedCfg, reedGeom)
			c.runner = shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
			return nil
		},
	}

	parent.AddCommand(c.runCmd(), c.interruptCmd(), c.sendCmd())

	return parent
}

// RunCLI is the public seam for the shuttle module CLI, delegating to clihelp.Execute for output
// capture.
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
