// cli.go builds the cobra command tree for the burler module and the
// RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "burler" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> mux engine -> claude
// engine -> shuttleengine.Runner -> burlerengine.Engine exactly once per
// invocation, into a receiver the run verb closes over, so the debug CLI
// wires the real substrate exactly like shuttlecli — burlercli is the
// module's claudeengine wiring point, mirroring the Provider-Seam Invariant.

package burlercli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// burlerCLI is the receiver the run verb hangs off of, so its RunE reads the
// same PersistentPreRunE-populated state. engine is the domain handle the
// verb calls into (Run) — the zero burlerCLI is not valid until
// PersistentPreRunE has populated engine.
type burlerCLI struct {
	engine *burlerengine.Engine
}

// Command returns the cobra command tree for the burler module.
//
// The parent "burler" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> mux engine -> claude
// engine -> shuttleengine.Runner -> burlerengine.Engine into c, skipping
// that resolution entirely when the group command itself is invoked (bare
// "lyx burler" listing or an unknown-subcommand error via GroupRunE) so
// neither path requires a git repository. The run verb creates its own
// (c *burlerCLI) runCmd() builder and registers it here via
// parent.AddCommand.
func Command() *cobra.Command {
	c := &burlerCLI{}

	parent := &cobra.Command{
		Use:   "burler",
		Short: "run one review+fix round over an artifact (the burler round worker)",
		Long: `burler drives one review+fix round over an artifact: an A phase reviews
the target against a fasit (a source of truth) and writes a structured review
file (verdict + findings), then a B phase fixes what A found and writes a
fixer report. What to review, what to judge it against, and how the round is
allowed to write its fixes are all supplied as a profile YAML file — burler
itself carries zero domain logic about the artifact under review.

Example:
  lyx burler run --profile profile.yaml`,
		// RunE is set so that bare "lyx burler" lists subcommands and "lyx
		// burler bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the burler group command itself is invoked (bare
			// listing or unknown-subcommand error path via GroupRunE), skip
			// cwd/layout/config/engine resolution so that neither path
			// requires a git repository to be present.
			if cmd.Name() == "burler" {
				return nil
			}

			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := hubgeometry.Getwd()
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			layout, err := hubgeometry.Resolve(cwd)
			if err != nil {
				// hubgeometry.Resolve's error is already self-describing (it
				// IS the "not a git repository" sentinel); pass it through
				// bare rather than doubling that same text on top of it.
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// Both configs are anchored at layout.Cwd, matching shuttlecli's
			// own resolution: the worktree the operator is actually standing
			// in, never WorktreeRoot or any weft sibling.
			shuttleCfg, err := shuttleengine.LoadConfig(layout.Cwd, "shuttle")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			muxCfg, err := muxengine.LoadConfig(layout.Cwd, "mux")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			muxEngine := muxengine.New(muxCfg, layout)
			runner := shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)
			c.engine = burlerengine.New(runner, layout)
			return nil
		},
	}

	parent.AddCommand(c.runCmd())

	return parent
}

// RunCLI is the public seam for the burler module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
