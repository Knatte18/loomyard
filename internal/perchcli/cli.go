// cli.go builds the cobra command tree for the perch module and the RunCLI seam that wires it into
// the standard io.Writer-based call contract.
// The parent "perch" command carries a PersistentPreRunE that resolves cwd -> layout -> perch
// geometry -> shuttle config -> reed config -> models registry -> perch config -> burler config ->
// reed engine -> claude engine -> shuttleengine.Runner -> burlerengine.Engine exactly once per
// invocation, storing the resolved ingredients on perchCLI rather than a constructed
// *perchengine.Engine: the pause seam (perchengine.Options.
// PauseRequested) closes over a per-run runDir that is only known once the run verb has resolved
// --profile and --run-id, so the run verb calls perchengine.New itself, per invocation.
// perchcli is the module's claudeengine wiring point, mirroring the Provider-Seam Invariant.

package perchcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// perchCLI stores PersistentPreRunE-resolved state shared by run and pause verbs.
type perchCLI struct {
	burlerEngine *burlerengine.Engine
	runner       *shuttleengine.Runner
	perchCfg     perchengine.Config
	// modelReg is the models.yaml registry, loaded exactly once per
	// invocation in PersistentPreRunE and reused for both perchCfg's
	// judge_model resolution and decodeProfile's judge-model/model
	// resolution — no second models.yaml read anywhere in the same run.
	modelReg modelspec.Registry
	layout   *lyxcwd.Location
	// perchGeom is the told perch geometry PersistentPreRunE resolves via
	// hubgeom.PerchGeometry, alongside layout: layout survives for the three
	// fabric call sites in run.go, which genuinely need the Location and are
	// genuinely hub-mode-only.
	perchGeom      perchengine.Geometry
	runDirBase     string
	scratchDirBase string
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

			// Every config is anchored at layout.AnchorPath(), matching
			// burlercli/shuttlecli's own resolution: the worktree the
			// operator is actually standing in, never WorktreeRoot or any
			// fabric sibling.
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

			// Loaded ONCE here, at the same layout.AnchorPath() anchor every config
			// load above uses, and reused for both perchCfg's judge_model
			// resolution (via LoadConfigWithRegistry) and decodeProfile's
			// profile-field resolution in runCmd — models.yaml is read
			// exactly once per invocation.
			modelReg, err := modelspec.LoadRegistry(layout.AnchorPath())
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			perchCfg, err := perchengine.LoadConfigWithRegistry(layout.AnchorPath(), "perch", modelReg)
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

			reedGeom := hubgeom.ReedGeometry(layout)
			reedEngine := reedengine.New(reedCfg, reedGeom)
			runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
			c.burlerEngine = burlerengine.New(runner, hubgeom.BurlerGeometry(layout), burlerCfg, fabricengine.StencilsDir(layout.HubPath))
			c.runner = runner
			c.perchCfg = perchCfg
			c.modelReg = modelReg
			c.layout = layout
			c.perchGeom = hubgeom.PerchGeometry(layout)
			// Anchored at perchGeom.AnchorPath, like the config loads above and
			// like Layout.LyxDir itself: the initialized _lyx (the fabric
			// junction, mirrored at <fabric>/<RelPath>/_lyx) lives at the
			// directory lyx init ran in, which is Cwd — not necessarily the git
			// worktree root. Anchoring at WorktreeRoot would, in a
			// nested-initialized repo, write run dirs into an un-junctioned
			// _lyx the fabric commit's RelPath-scoped pathspec never includes,
			// silently stranding every artifact outside fabric.
			c.runDirBase = perchengine.RunsDir(c.perchGeom.AnchorPath)
			// Anchored at the same perchGeom.AnchorPath as runDirBase — the
			// never-tracked half of the pair: run.lock, state.json.lock, and
			// the pause flag live here instead of inside _lyx, so a block's
			// two directories can never disagree about which anchor they
			// belong to.
			c.scratchDirBase = perchengine.ScratchDir(c.perchGeom.AnchorPath)
			return nil
		},
	}

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
