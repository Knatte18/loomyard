// cli.go builds the cobra command tree for the shuttle module and the
// RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "shuttle" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> mux engine -> claude
// engine -> shuttleengine.Runner exactly once per invocation, into a
// receiver every verb (run.go, interrupt.go, send.go) closes over, so no
// subcommand re-resolves geometry, config, or engine construction itself.

package shuttlecli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// shuttleCLI is the receiver every shuttle verb hangs off of, so each
// subcommand's RunE reads the same PersistentPreRunE-populated state. runner
// is the domain handle every verb calls into (Run/Interrupt/Send) — the
// zero shuttleCLI is not valid until PersistentPreRunE has populated runner.
type shuttleCLI struct {
	runner *shuttleengine.Runner
}

// Command returns the cobra command tree for the shuttle module.
//
// The parent "shuttle" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> mux engine -> claude
// engine -> shuttleengine.Runner into c, skipping that resolution entirely
// when the group command itself is invoked (bare "lyx shuttle" listing or an
// unknown-subcommand error via GroupRunE) so neither path requires a git
// repository. Every verb card creates its own (c *shuttleCLI) xCmd() builder
// and registers it here via parent.AddCommand — this card registers only
// run.
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

			// Both configs are anchored at layout.Cwd, matching muxcli's own
			// resolution: the worktree the operator is actually standing in,
			// never WorktreeRoot or any weft sibling.
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
			c.runner = shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)
			return nil
		},
	}

	parent.AddCommand(c.runCmd())

	return parent
}

// RunCLI is the public seam for the shuttle module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
