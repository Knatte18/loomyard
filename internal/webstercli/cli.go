// cli.go builds the cobra command tree for the webster module and the RunCLI seam that wires it
// into the standard io.Writer-based call contract.
// The parent "webster" command carries a PersistentPreRunE that resolves cwd -> layout -> shuttle
// config -> reed config -> webster config -> model registry -> resolved roles -> reed engine ->
// claude engine -> shuttleengine.Runner exactly once per invocation, storing the resolved
// ingredients on websterCLI, per the discussion's cli-shape decision: every _lyx/plan
// and _lyx/webster path this module touches is anchored at layout.AnchorPath() -- the directory lyx
// init ran in, never WorktreeRoot or a fabric sibling.
//
// websterCLI stores THREE adapted
// views of the one constructed Runner: starter (websterengine.Starter, webster's own local copy of
// the spawn seam, consumed by recover-batch's cold-strand spawn), injector (websterengine.Injector,
// consumed by begin-batch's model-switch choreography), and masterStarter
// (websterengine.MasterStarter, behind the runnerMasterStarter adapter, consumed by run's Master
// spawn) -- because webster's three verbs each need a distinct narrow seam onto the same underlying
// *shuttleengine.Runner, none of which the others expose.
package webstercli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// websterCLI is the receiver every webster verb hangs off of.
type websterCLI struct {
	// runner is the constructed shuttle Runner the three adapted seams below are derived from.
	runner *shuttleengine.Runner

	// starter, injector, and masterStarter are the three narrow seams webster's verbs spawn/inject through.
	starter       websterengine.Starter
	injector      websterengine.Injector
	masterStarter websterengine.MasterStarter

	// engine and reed are the constructed claude and reed engines record-batch and recover-batch need directly.
	engine shuttleengine.Engine
	reed   shuttleengine.ReedOps

	layout     *lyxcwd.Location
	shuttleCfg shuttleengine.Config
	cfg        websterengine.Config
	roles      map[websterengine.Role]modelspec.Resolved

	// batcher is the load-time-resolved, config-selected batchifier.
	batcher batcher.Batcher

	// planDir is planparser's own told-anchor path, built from layout.AnchorPath() in
	// PersistentPreRunE; websterDir and reportsDir remain the lyxcwd-resolved _lyx dirs;
	// promptsDir and websterScratchDir remain the lyxcwd-resolved .lyx dirs.
	planDir           string
	websterDir        string
	reportsDir        string
	promptsDir        string
	websterScratchDir string
}

// runnerMasterStarter adapts *shuttleengine.Runner to websterengine.MasterStarter.
type runnerMasterStarter struct {
	runner *shuttleengine.Runner
}

// StartMaster implements websterengine.MasterStarter.
func (s runnerMasterStarter) StartMaster(spec shuttleengine.Spec) (websterengine.MasterHandle, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// Command returns the cobra command tree for the webster module.
//
// The parent "webster" command carries a PersistentPreRunE that resolves cwd and configs into c,
// skipping resolution when the group command itself is invoked.
func Command() *cobra.Command {
	c := &websterCLI{}

	parent := &cobra.Command{
		Use:   "webster",
		Short: "drive a pinned plan-format plan through a long-lived Master session that forks one implementer per batch",
		Long: `webster takes a pinned plan-format plan (see contracts/specs/loom-plan-spec.md)
and drives it through a long-lived Master session that reads the plan once
and forks one implementer per batch in-session, bracketing each fork with
begin-batch/record-batch calls, until the plan is built or the run reports
stuck or paused. A fork that reports stuck (or never reports at all) is
escalated to a cold recovery strand via recover-batch. The Go verbs below
are the fat, file-contract-backed primitives Master's own prompt drives.

Verbs:
  lyx webster validate                       lint the plan without running anything
  lyx webster run --fresh                    spawn/resume Master and block until terminal
  lyx webster status                         an instant snapshot of state.json + reports
  lyx webster pause                          request a pause at the next batch boundary
  lyx webster begin-batch 3                  Master's bracket call immediately before forking batch 3
  lyx webster await-batch 3                  block until batch 3's report lands (forks are backgrounded)
  lyx webster record-batch 3                 Master's bracket call once batch 3's fork has delivered
  lyx webster recover-batch 3 --wait 8m      escalate batch 3 to a cold recovery strand`,
		// RunE is set so that bare "lyx webster" lists subcommands and "lyx
		// webster bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the webster group command itself is invoked (bare
			// listing or unknown-subcommand error path via GroupRunE), skip
			// cwd/layout/config/engine resolution so that neither path
			// requires a git repository to be present.
			if cmd.Name() == "webster" {
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
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

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

			websterCfg, err := websterengine.LoadConfig(layout.AnchorPath(), "webster")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			activeBatcher, err := batcher.Active(layout.AnchorPath())
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

			roles, err := websterengine.ResolveRoles(websterCfg, registry)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			reedGeom := hubgeom.ReedGeometry(layout)
			reedEngine := reedengine.New(reedCfg, reedGeom)
			claudeEngine := claudeengine.New()
			runner := shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

			c.runner = runner
			c.starter = runner
			c.injector = runner
			c.masterStarter = runnerMasterStarter{runner: runner}
			c.engine = claudeEngine
			c.reed = reedEngine
			c.layout = layout
			c.shuttleCfg = shuttleCfg
			c.cfg = websterCfg
			c.roles = roles
			c.batcher = activeBatcher
			c.planDir = planparser.PlanDir(layout.AnchorPath())
			c.websterDir = websterengine.Dir(layout)
			c.reportsDir = websterengine.ReportsDir(layout)
			c.websterScratchDir = websterengine.ScratchDir(layout)
			c.promptsDir = websterengine.PromptsDir(layout)
			return nil
		},
	}

	parent.AddCommand(c.validateCmd())
	parent.AddCommand(c.runCmd())
	parent.AddCommand(c.statusCmd())
	parent.AddCommand(c.pauseCmd())
	parent.AddCommand(c.beginBatchCmd())
	parent.AddCommand(c.awaitBatchCmd())
	parent.AddCommand(c.recordBatchCmd())
	parent.AddCommand(c.recoverBatchCmd())

	return parent
}

// RunCLI is the public seam for the webster module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the capture writer
// for all output (including cobra's error text).
// This preserves the existing call contract so that callers and tests are unchanged.
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
