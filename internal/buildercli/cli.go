// cli.go builds the cobra command tree for the builder module and the
// RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "builder" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> reed config -> builder config -> model
// registry -> resolved roles -> reed engine -> claude engine ->
// shuttleengine.Runner exactly once per invocation, storing the resolved
// ingredients on builderCLI, mirroring perchcli's Cwd-anchoring rationale
// (internal/perchcli/cli.go): every _lyx/plan and _lyx/builder path this
// module touches is anchored at layout.AnchorPath() -- the directory lyx init ran
// in, never WorktreeRoot or a weft sibling.
//
// Unlike perchcli (which stores only the resolved config ingredients and
// constructs a fresh *perchengine.Engine per invocation), builderCLI keeps
// the constructed shuttle Runner AND its two underlying engines directly:
// poll's terminal classification needs to call the claude engine's
// ParseEvents and the reed engine's Status directly, and Runner's own
// engine/reed fields are unexported, so builderCLI keeps its own handles to
// both rather than re-deriving them.

package buildercli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
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
	// starter and orchestratorStarter default to runner (the latter through
	// the runnerOrchestratorStarter adapter) in production. A test overrides
	// one or the other with a fake to exercise spawn-batch/run without a
	// live tmux/claude substrate, mirroring how builderengine's own
	// spawn/run tests fake these same seams (spawn_test.go,
	// runlevel_test.go).
	runner              *shuttleengine.Runner
	starter             builderengine.Starter
	orchestratorStarter builderengine.OrchestratorStarter

	// engine and reed are the constructed claude and reed engines Runner
	// itself holds unexported: poll calls builderengine.TurnEnded/
	// builderengine.StrandLive directly with these, and both gatherers need
	// to call ParseEvents/Status on them, which Runner's own surface does
	// not expose.
	engine shuttleengine.Engine
	reed   shuttleengine.ReedOps

	layout     *lyxcwd.Location
	shuttleCfg shuttleengine.Config
	cfg        builderengine.Config
	roles      map[builderengine.Role]modelspec.Resolved

	// planDir, builderDir, and reportsDir are the lyxcwd-resolved
	// _lyx/plan, _lyx/builder, and _lyx/builder/reports directories, all
	// anchored at layout.AnchorPath() -- never WorktreeRoot -- per the Hub Geometry
	// Invariant and this package's own Cwd-anchoring rationale (see the
	// package doc above).
	planDir    string
	builderDir string
	reportsDir string
}

// runnerOrchestratorStarter adapts Runner to OrchestratorStarter via a thin bridge.
type runnerOrchestratorStarter struct {
	runner *shuttleengine.Runner
}

// StartOrchestrator implements builderengine.OrchestratorStarter.
func (s runnerOrchestratorStarter) StartOrchestrator(spec shuttleengine.Spec) (builderengine.OrchestratorHandle, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// Command returns the cobra command tree for the builder module.
// Resolution is skipped when bare "lyx builder" is invoked so help doesn't require a git repo.
// All six verbs run role resolution pre-flight to catch typos early.
func Command() *cobra.Command {
	c := &builderCLI{}

	parent := &cobra.Command{
		Use:   "builder",
		Short: "drive a pinned plan-format plan through implementer sessions, batch by batch",
		Long: `builder takes a pinned plan-format v2 plan (see docs/reference/plan-format.md)
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
			if cmd.Name() == "builder" {
				return nil
			}

			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.Getwd()
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

			// Every config is anchored at layout.AnchorPath(), matching perchcli's
			// own resolution: the worktree the operator is actually
			// standing in, never WorktreeRoot or any weft sibling.
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

			builderCfg, err := builderengine.LoadConfig(layout.AnchorPath(), "builder")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			registry, err := modelspec.LoadRegistry(layout.AnchorPath())
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

			reedEngine := reedengine.New(reedCfg, layout)
			claudeEngine := claudeengine.New()
			runner := shuttleengine.NewRunner(reedEngine, claudeEngine, layout, shuttleCfg)

			c.runner = runner
			c.starter = runner
			c.orchestratorStarter = runnerOrchestratorStarter{runner: runner}
			c.engine = claudeEngine
			c.reed = reedEngine
			c.layout = layout
			c.shuttleCfg = shuttleCfg
			c.cfg = builderCfg
			c.roles = roles
			// Anchored at layout.AnchorPath(), like every config load above and
			// like perchcli's own runDirBase: the initialized _lyx (the
			// weft junction) lives at the directory lyx init ran in, which
			// is Cwd -- not necessarily the git worktree root. Anchoring at
			// WorktreeRoot would, in a nested-initialized repo, resolve
			// these dirs outside the junctioned _lyx the weft commit's
			// RelPath-scoped pathspec never includes, silently stranding
			// every builder artifact outside the weft.
			c.planDir = lyxcwd.PlanDir(layout.AnchorPath())
			c.builderDir = lyxcwd.BuilderDir(layout.AnchorPath())
			c.reportsDir = lyxcwd.BuilderReportsDir(layout.AnchorPath())
			return nil
		},
	}

	parent.AddCommand(c.validateCmd())
	parent.AddCommand(c.statusCmd())
	parent.AddCommand(c.spawnBatchCmd())
	parent.AddCommand(c.pollCmd())
	parent.AddCommand(c.runCmd())
	parent.AddCommand(c.pauseCmd())

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
