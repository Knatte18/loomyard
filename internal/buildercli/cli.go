// cli.go builds the cobra command tree for the builder module and the
// RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "builder" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> builder config -> model
// registry -> resolved roles -> mux engine -> claude engine ->
// shuttleengine.Runner exactly once per invocation, storing the resolved
// ingredients on builderCLI, mirroring perchcli's Cwd-anchoring rationale
// (internal/perchcli/cli.go): every _lyx/plan and _lyx/builder path this
// module touches is anchored at layout.Cwd -- the directory lyx init ran
// in, never WorktreeRoot or a weft sibling.
//
// Unlike perchcli (which stores only the resolved config ingredients and
// constructs a fresh *perchengine.Engine per invocation), builderCLI keeps
// the constructed shuttle Runner AND its two underlying engines directly:
// poll's terminal classification needs to call the claude engine's
// ParseEvents and the mux engine's Status directly, and Runner's own
// engine/mux fields are unexported, so builderCLI keeps its own handles to
// both rather than re-deriving them.

package buildercli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// builderCLI is the receiver every builder verb hangs off of, so their RunE
// bodies read the same PersistentPreRunE-populated state. The zero
// builderCLI is not valid until PersistentPreRunE has populated it (or a
// test has populated the same fields directly -- see each verb's own
// _test.go file for the package-local fake-injection pattern this enables).
type builderCLI struct {
	// starter and blockingRunner default to runner in production. A test
	// overrides one or the other with a fake to exercise spawn-batch/run
	// without a live psmux/claude substrate, mirroring how builderengine's
	// own spawn/run tests fake these same seams (spawn_test.go,
	// runlevel_test.go).
	runner         *shuttleengine.Runner
	starter        builderengine.Starter
	blockingRunner builderengine.BlockingRunner

	// engine and mux are the constructed claude and mux engines Runner
	// itself holds unexported: poll's turnEnded/strandLive-equivalent
	// gatherers need to call them directly (ParseEvents, Status), which
	// Runner's own surface does not expose.
	engine shuttleengine.Engine
	mux    shuttleengine.MuxOps

	layout     *hubgeometry.Layout
	shuttleCfg shuttleengine.Config
	cfg        builderengine.Config
	roles      map[builderengine.Role]modelspec.Resolved

	// planDir, builderDir, and reportsDir are the hubgeometry-resolved
	// _lyx/plan, _lyx/builder, and _lyx/builder/reports directories, all
	// anchored at layout.Cwd -- never WorktreeRoot -- per the Hub Geometry
	// Invariant and this package's own Cwd-anchoring rationale (see the
	// package doc above).
	planDir    string
	builderDir string
	reportsDir string
}

// Command returns the cobra command tree for the builder module.
//
// The parent "builder" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> builder config -> model
// registry -> resolved roles -> mux engine -> claude engine ->
// shuttleengine.Runner into c, skipping that resolution entirely when the
// group command itself is invoked (bare "lyx builder" listing or an
// unknown-subcommand error via GroupRunE) so neither path requires a git
// repository. Role resolution runs against every one of the config's four
// roles as a pre-flight -- deliberately uniform across all six verbs, per
// the discussion's role-selection decision (the discussion's
// run/spawn-batch scoping is the minimum, not a ceiling) -- so a typo'd
// role alias in builder.yaml aborts every verb here, before any agent
// spawns, never surfacing only hours into a run when that role first
// spawns.
func Command() *cobra.Command {
	c := &builderCLI{}

	parent := &cobra.Command{
		Use:   "builder",
		Short: "drive a pinned plan-format plan through implementer sessions, batch by batch",
		Long: `builder takes a pinned plan-format v1 plan (see docs/modules/plan-format.md)
and drives it through implementer sessions, batch by batch, until the plan
is built or the run reports stuck or paused. A long-lived orchestrator
session (spawned by "run") holds the batch loop; the Go verbs below
provide the fat, file-contract-backed primitives it drives.

Verbs:
  lyx builder validate                        lint the plan without running anything
  lyx builder run --fresh                     spawn/resume the orchestrator and block until terminal
  lyx builder spawn-batch 3 --role recovery    spawn one batch's implementer
  lyx builder poll --wait 8m                   long-poll the in-flight batch for its terminal digest
  lyx builder status                           an instant snapshot of state.json + reports
  lyx builder pause                            request a pause at the next batch boundary`,
		// RunE is set so that bare "lyx builder" lists subcommands and "lyx
		// builder bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the builder group command itself is invoked (bare
			// listing or unknown-subcommand error path via GroupRunE), skip
			// cwd/layout/config/engine resolution so that neither path
			// requires a git repository to be present.
			if cmd.Name() == "builder" {
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

			// Every config is anchored at layout.Cwd, matching perchcli's
			// own resolution: the worktree the operator is actually
			// standing in, never WorktreeRoot or any weft sibling.
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

			builderCfg, err := builderengine.LoadConfig(layout.Cwd, "builder")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			registry, err := modelspec.LoadRegistry(layout.Cwd)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// The fail-pre-flight surface: a typo'd role alias in
			// builder.yaml aborts every verb here, before any agent spawns.
			roles, err := builderengine.ResolveRoles(builderCfg, registry)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			muxEngine := muxengine.New(muxCfg, layout)
			claudeEngine := claudeengine.New()
			runner := shuttleengine.NewRunner(muxEngine, claudeEngine, layout, shuttleCfg)

			c.runner = runner
			c.starter = runner
			c.blockingRunner = runner
			c.engine = claudeEngine
			c.mux = muxEngine
			c.layout = layout
			c.shuttleCfg = shuttleCfg
			c.cfg = builderCfg
			c.roles = roles
			// Anchored at layout.Cwd, like every config load above and
			// like perchcli's own runDirBase: the initialized _lyx (the
			// weft junction) lives at the directory lyx init ran in, which
			// is Cwd -- not necessarily the git worktree root. Anchoring at
			// WorktreeRoot would, in a nested-initialized repo, resolve
			// these dirs outside the junctioned _lyx the weft commit's
			// RelPath-scoped pathspec never includes, silently stranding
			// every builder artifact outside the weft.
			c.planDir = hubgeometry.PlanDir(layout.Cwd)
			c.builderDir = hubgeometry.BuilderDir(layout.Cwd)
			c.reportsDir = hubgeometry.BuilderReportsDir(layout.Cwd)
			return nil
		},
	}

	parent.AddCommand(c.validateCmd())
	parent.AddCommand(c.statusCmd())

	return parent
}

// RunCLI is the public seam for the builder module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
