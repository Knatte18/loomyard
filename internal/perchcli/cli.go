// cli.go builds the cobra command tree for the perch module and the
// RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "perch" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> mux engine -> claude
// engine -> shuttleengine.Runner -> burlerengine.Engine exactly once per
// invocation, storing the resolved ingredients on perchCLI rather than a
// constructed *perchengine.Engine: the pause seam (perchengine.Options.
// PauseRequested) closes over a per-run runDir that is only known once the
// run verb has resolved --profile and --run-id, so the run verb calls
// perchengine.New itself, per invocation (card 15). perchcli is the
// module's claudeengine wiring point, mirroring the Provider-Seam Invariant.

package perchcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// perchCLI is the receiver the run and pause verbs hang off of, so their
// RunE bodies read the same PersistentPreRunE-populated state. Unlike
// burlerCLI (which stores a constructed engine), perchCLI stores the
// resolved ingredients a fresh *perchengine.Engine is built from: the run
// verb (card 15) calls perchengine.New per invocation, closing its pause
// seam over the concrete runDir it resolves from --profile/--run-id. The
// zero perchCLI is not valid until PersistentPreRunE has populated it.
type perchCLI struct {
	burlerEngine *burlerengine.Engine
	runner       *shuttleengine.Runner
	perchCfg     perchengine.Config
	layout       *hubgeometry.Layout
	runDirBase   string
}

// Command returns the cobra command tree for the perch module.
//
// The parent "perch" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> perch config -> mux
// engine -> claude engine -> shuttleengine.Runner -> burlerengine.Engine
// into c, skipping that resolution entirely when the group command itself
// is invoked (bare "lyx perch" listing or an unknown-subcommand error via
// GroupRunE) so neither path requires a git repository. The run and pause
// verbs register their own builders via parent.AddCommand.
func Command() *cobra.Command {
	c := &perchCLI{}

	parent := &cobra.Command{
		Use:   "perch",
		Short: "run a profile-driven gate loop over burler rounds until APPROVED or STUCK",
		Long: `perch drives burler rounds over an artifact until the artifact is either
APPROVED (a clean round with zero blocking findings on top of the prior
round's fixes) or STUCK (a milestone-gated round cap plus a progress judge
determined the artifact is not converging). What to review, what to judge it
against, the convergence gate, and the round-cap ladder are all supplied as a
profile YAML file — perch itself carries zero domain logic about the
artifact under review.

Example:
  lyx perch run --profile profile.yaml`,
		// RunE is set so that bare "lyx perch" lists subcommands and "lyx
		// perch bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the perch group command itself is invoked (bare
			// listing or unknown-subcommand error path via GroupRunE), skip
			// cwd/layout/config/engine resolution so that neither path
			// requires a git repository to be present.
			if cmd.Name() == "perch" {
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

			// Every config is anchored at layout.Cwd, matching
			// burlercli/shuttlecli's own resolution: the worktree the
			// operator is actually standing in, never WorktreeRoot or any
			// weft sibling.
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

			perchCfg, err := perchengine.LoadConfig(layout.Cwd, "perch")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			muxEngine := muxengine.New(muxCfg, layout)
			runner := shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)
			c.burlerEngine = burlerengine.New(runner, layout)
			c.runner = runner
			c.perchCfg = perchCfg
			c.layout = layout
			c.runDirBase = hubgeometry.PerchRunsDir(layout.WorktreeRoot)
			return nil
		},
	}

	return parent
}

// RunCLI is the public seam for the perch module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
