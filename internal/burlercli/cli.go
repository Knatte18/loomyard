// cli.go builds the cobra command tree for the burler module and the RunCLI seam that wires it into
// the standard io.Writer-based call contract.
// The parent "burler" command carries a PersistentPreRunE that resolves cwd -> layout -> shuttle
// config -> burler config -> reed config -> reed engine -> claude engine -> shuttleengine.Runner ->
// burlerengine.Engine exactly once per invocation, into a receiver the run verb closes over, so the
// debug CLI wires the real substrate exactly like shuttlecli — burlercli is the module's
// claudeengine wiring point, mirroring the Provider-Seam Invariant.

package burlercli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// burlerCLI is the receiver the run verb hangs off of.
type burlerCLI struct {
	engine *burlerengine.Engine
}

// Command returns the cobra command tree for the burler module.
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
			// Guard: when the group command itself is invoked, skip resolution
			// so neither path requires a git repository.
			if cmd.Name() == "burler" {
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
				// lyxcwd.Resolve's error is already self-describing.
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// Both configs are anchored at layout.AnchorPath(), matching shuttlecli's
			// own resolution: the worktree the operator is actually standing
			// in, never WorktreeRoot or any fabric sibling.
			shuttleCfg, err := shuttleengine.LoadConfig(layout.AnchorPath(), "shuttle")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// burlerengine.LoadConfig's only error today is a read/decode
			// failure — an absent burler.yaml is not an error, it decodes to
			// the zero Config (clustering then fails later, at fan
			// resolution, with a message naming `lyx config reconcile`).
			burlerCfg, err := burlerengine.LoadConfig(layout.AnchorPath())
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
			runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), layout, shuttleCfg)
			c.engine = burlerengine.New(runner, layout, burlerCfg, fabricengine.StencilsDir(layout.HubPath))
			return nil
		},
	}

	parent.AddCommand(c.runCmd())

	return parent
}

// RunCLI is the public seam for the burler module CLI.
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
