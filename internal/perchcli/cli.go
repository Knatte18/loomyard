// cli.go builds the cobra command tree for the perch module and the RunCLI seam that wires it into
// the standard io.Writer-based call contract.
// The parent "perch" command carries a PersistentPreRunE that resolves cwd, runs one
// preflight.ResolveMode probe, and delegates to c.wire (wiring.go), which selects hub or standalone
// mode and builds the whole engine stack for whichever mode wins, storing the resolved ingredients
// on perchCLI rather than a constructed *perchengine.Engine: the pause seam
// (perchengine.Options.PauseRequested) closes over a per-run runDir that is only known once the run
// verb has resolved --profile and --run-id, so the run verb calls perchengine.New itself, per
// invocation.
// A non-nil ResolveMode error means refuse: it aborts the pre-run right there, before c.wire is ever
// called, rather than selecting a mode -- refusal is a resolution verdict, not a wiring choice.
// perchcli is the module's claudeengine wiring point, mirroring the Provider-Seam Invariant.

package perchcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// perchCLI stores PersistentPreRunE-resolved state shared by run and pause verbs.
type perchCLI struct {
	burlerEngine *burlerengine.Engine
	runner       *shuttleengine.Runner
	perchCfg     perchengine.Config
	// modelReg is the models.yaml registry, loaded exactly once per
	// invocation in wire and reused for both perchCfg's judge_model
	// resolution and decodeProfile's judge-model/model resolution — no
	// second models.yaml read anywhere in the same run.
	modelReg modelspec.Registry
	// perchGeom is the told perch geometry wire resolves via
	// hubgeom.PerchGeometry (hub mode) or standalonegeom.PerchGeometry
	// (standalone mode). Its old comment named layout as the reason the
	// three fabric call sites in run.go survived; those now read
	// stencilsDir, anchorRel, and openFabric instead.
	perchGeom      perchengine.Geometry
	runDirBase     string
	scratchDirBase string

	// stencilsDir is the one told stencils directory both consumers read:
	// the nested burlerEngine (built in wire) and the value run.go passes to
	// perchengine.Engine.Run. It also doubles as the third reporting field,
	// alongside mode/stateDir below, so perch carries the same envelope
	// values burler does without a fourth field.
	stencilsDir string
	// anchorRel is loc.AnchorRel in hub mode and "" in standalone -- the one
	// thing the deleted layout field held that no other told replacement
	// supplies.
	anchorRel string
	// openFabric is the lazy fabric opener: a closure in hub mode, nil in
	// standalone (which has no fabric repo by construction). It must not be
	// called during the pre-run itself -- fabricengine.Open stat-checks the
	// paired sibling and would fail in healthy-but-unwired locations.
	openFabric func() (*fabricengine.Fabric, error)

	// stencilsDirFlag and targetDirFlag hold the raw, as-parsed values of the two standalone-entry
	// persistent flags (--stencils-dir, --target-dir). An empty value means the flag was not passed;
	// each mode's own default is computed by the wiring function (wiring.go) rather than a
	// zero-value fallback landing here.
	stencilsDirFlag string
	targetDirFlag   string

	// mode and stateDir are CLI-level facts about how the stack was wired, read off this receiver by
	// run.go's success envelope. They are not results of a review round, which is why they are not
	// threaded through burlerengine.Result.
	mode     string
	stateDir string
}

// resolvePersistentPreRun resolves cwd, calls preflight.ResolveMode(cwd), and delegates the mode
// decision and the whole engine stack construction to c.wire (wiring.go), storing the resolved
// ingredients on c.
// A non-nil ResolveMode error is the refuse case, and it is handled right here rather than inside
// wire: the refusal deliberately stays in resolvePersistentPreRun because it is a resolution
// verdict, not a wiring choice, so it is surfaced verbatim and aborts the pre-run before c.wire is
// ever called -- wire's own two-row truth table never sees a third value.
// Extracted from Command()'s PersistentPreRunE assignment so a test can invoke it directly against a
// *perchCLI it holds a reference to and inspect the populated fields afterward.
// Skips resolution entirely when the group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present -- preserved exactly as today because TestRunCLI_GroupGuard_OutsideGitRepo pins it.
func (c *perchCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "perch" {
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

// Command returns the cobra command tree for the perch module.
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

Modes:
  perch runs in hub mode inside a lyx hub worktree, and in standalone mode
  anywhere else -- a plain git checkout with no lyx hub beside it. Two
  persistent flags cross that boundary: --stencils-dir is optional and
  read-only in BOTH modes (hub default: the hub's own stencils dir;
  standalone default: the derived state directory's own _lyx/stencils);
  --target-dir is standalone-only, defaults to the current directory, and is
  refused in hub mode, where the worktree itself is structurally the target.

Example:
  lyx perch run --profile profile.yaml

Example (standalone, outside any lyx hub):
  lyx perch run --profile profile.yaml --target-dir /path/to/repo`,
		// RunE is set so that bare "lyx perch" lists subcommands and "lyx
		// perch bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.PersistentFlags().StringVar(&c.stencilsDirFlag, "stencils-dir", "",
		"override the stencils directory read at call time (read-only in both modes; hub default: the hub's own stencils dir; standalone default: the derived state directory's _lyx/stencils)")
	parent.PersistentFlags().StringVar(&c.targetDirFlag, "target-dir", "",
		"standalone-only: the directory perch reviews against; defaults to the current directory; refused in hub mode, where the worktree is already the target")

	parent.AddCommand(c.runCmd())
	parent.AddCommand(c.pauseCmd())

	return parent
}

// RunCLI is the public seam for the perch module CLI.
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
