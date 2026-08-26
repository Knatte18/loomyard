// cli.go builds the cobra command tree for the burler module and the RunCLI seam that wires it into
// the standard io.Writer-based call contract.
// The parent "burler" command carries a PersistentPreRunE that resolves cwd, runs one
// preflight.ResolveMode probe, and delegates to c.wire (wiring.go), which selects hub or standalone
// mode and builds the whole engine stack for whichever mode wins, storing the resolved ingredients
// on burlerCLI exactly once per invocation.
// A non-nil ResolveMode error means refuse: it aborts the pre-run right there, before c.wire is ever
// called, rather than selecting a mode -- refusal is a resolution verdict, not a wiring choice.
// burlercli is the module's claudeengine wiring point, mirroring the Provider-Seam Invariant.

package burlercli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/spf13/cobra"
)

// burlerCLI is the receiver the run verb hangs off of.
type burlerCLI struct {
	// engine is the constructed burlerengine.Engine the run verb closes over.
	engine *burlerengine.Engine

	// stencilsDirFlag and targetDirFlag hold the raw, as-parsed values of the two standalone-entry
	// persistent flags (--stencils-dir, --target-dir). An empty value means the flag was not passed;
	// each mode's own default is computed by the wiring function (wiring.go) rather than a
	// zero-value fallback landing here.
	stencilsDirFlag string
	targetDirFlag   string

	// mode, stateDir, and stencilsDir are CLI-level facts about how the stack was wired, read off
	// this receiver by run.go's success envelope. They are not results of a review round, which is
	// why they are not threaded through burlerengine.Result.
	mode        string
	stateDir    string
	stencilsDir string
}

// resolvePersistentPreRun resolves cwd, calls preflight.ResolveMode(cwd), and delegates the mode
// decision and the whole engine stack construction to c.wire (wiring.go), storing the resolved
// ingredients on c.
// A non-nil ResolveMode error is the refuse case, and it is handled right here rather than inside
// wire: the refusal deliberately stays in resolvePersistentPreRun because it is a resolution
// verdict, not a wiring choice, so it is surfaced verbatim and aborts the pre-run before c.wire is
// ever called -- wire's own two-row truth table never sees a third value.
// Extracted from Command()'s PersistentPreRunE assignment so a test can invoke it directly against a
// *burlerCLI it holds a reference to and inspect the populated fields afterward.
// Skips resolution entirely when the group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present -- preserved exactly as today because TestRunCLI_GroupGuard_OutsideGitRepo pins it.
func (c *burlerCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
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

	loc, mode, err := preflight.ResolveMode(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	if err := c.wire(loc, mode, cwd, c.stencilsDirFlag, c.targetDirFlag); err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	return nil
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

Modes:
  burler runs in hub mode inside a lyx hub worktree, and in standalone mode
  anywhere else -- a plain git checkout with no lyx hub beside it. Two
  persistent flags cross that boundary: --stencils-dir is optional and
  read-only in BOTH modes (hub default: the hub's own stencils dir;
  standalone default: the derived state directory's own _lyx/stencils);
  --target-dir is standalone-only, defaults to the current directory, and is
  refused in hub mode, where the anchor path is structurally the target.

Example:
  lyx burler run --profile profile.yaml

Example (standalone, outside any lyx hub):
  lyx burler run --profile profile.yaml --target-dir /path/to/repo`,
		// RunE is set so that bare "lyx burler" lists subcommands and "lyx
		// burler bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.PersistentFlags().StringVar(&c.stencilsDirFlag, "stencils-dir", "",
		"override the stencils directory read at call time (read-only in both modes; hub default: the hub's own stencils dir; standalone default: the derived state directory's _lyx/stencils)")
	parent.PersistentFlags().StringVar(&c.targetDirFlag, "target-dir", "",
		"standalone-only: the directory burler reviews against; defaults to the current directory; refused in hub mode, where the anchor path is already the target")

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
