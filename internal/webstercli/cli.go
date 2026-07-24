// cli.go builds the cobra command tree for the webster module and the
// RunCLI seam that wires it into the standard io.Writer-based call contract.
// The parent "webster" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> webster config -> model
// registry -> resolved roles -> mux engine -> claude engine ->
// shuttleengine.Runner exactly once per invocation, storing the resolved
// ingredients on websterCLI -- buildercli's own cli.go
// (internal/buildercli/cli.go) is the proven shape this file mirrors file
// for file, per the discussion's cli-shape decision: every _lyx/plan and
// _lyx/webster path this module touches is anchored at layout.Cwd -- the
// directory lyx init ran in, never WorktreeRoot or a weft sibling.
//
// Unlike buildercli (which stores only a builderengine.Starter and a
// builderengine.OrchestratorStarter adapter over the same Runner),
// websterCLI stores THREE adapted views of the one constructed Runner:
// starter (builderengine.Starter, consumed by recover-batch's cold-strand
// spawn, reused by import per the reuse-by-import-never-copy decision),
// injector (websterengine.Injector, consumed by begin-batch's model-switch
// choreography), and masterStarter (websterengine.MasterStarter, behind the
// runnerMasterStarter adapter, consumed by run's Master spawn) -- because
// webster's three verbs each need a distinct narrow seam onto the same
// underlying *shuttleengine.Runner, none of which the others expose.
package webstercli

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
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// websterCLI is the receiver every webster verb hangs off of, so their RunE
// bodies read the same PersistentPreRunE-populated state. The zero
// websterCLI is not valid until PersistentPreRunE has populated it (or a
// test has populated the same fields directly -- see each verb's own
// _test.go file for the package-local fake-injection pattern this enables).
type websterCLI struct {
	// runner is the constructed shuttle Runner every one of the three
	// adapted seams below is derived from.
	runner *shuttleengine.Runner

	// starter, injector, and masterStarter are the three narrow seams
	// webster's verbs spawn/inject through, all backed by runner in
	// production: starter is recover-batch's cold-strand spawn seam
	// (builderengine.Starter, reused by import); injector is begin-batch's
	// model-switch seam (websterengine.Injector); masterStarter is run's
	// Master spawn seam (websterengine.MasterStarter), reached through the
	// runnerMasterStarter adapter below. A test overrides any one
	// independently with a fake to exercise a verb without a live
	// tmux/claude substrate.
	starter       builderengine.Starter
	injector      websterengine.Injector
	masterStarter websterengine.MasterStarter

	// engine and mux are the constructed claude and mux engines Runner
	// itself holds unexported: record-batch and recover-batch call
	// builderengine.TurnEnded/builderengine.StrandLive directly with these,
	// and both gatherers need to call ParseEvents/Status on them, which
	// Runner's own surface does not expose.
	engine shuttleengine.Engine
	mux    shuttleengine.MuxOps

	layout     *hubgeometry.Layout
	shuttleCfg shuttleengine.Config
	cfg        websterengine.Config
	roles      map[websterengine.Role]modelspec.Resolved

	// planDir, websterDir, reportsDir, and promptsDir are the
	// hubgeometry-resolved _lyx/plan, _lyx/webster, _lyx/webster/reports, and
	// _lyx/webster/prompts directories, all anchored at layout.Cwd -- never
	// WorktreeRoot -- per the Hub Geometry Invariant and buildercli's own
	// Cwd-anchoring rationale (see the package doc above).
	planDir    string
	websterDir string
	reportsDir string
	promptsDir string
}

// runnerMasterStarter adapts *shuttleengine.Runner to
// websterengine.MasterStarter: Runner.Start returns the concrete
// *shuttleengine.Run, which satisfies websterengine.MasterHandle
// structurally (StrandGUID + Wait), but Go's typed method sets keep Runner
// itself from satisfying the interface directly — this thin adapter bridges
// that, so websterengine.Run can persist Master's strand identity between
// the start and the blocking wait, mirroring buildercli's own
// runnerOrchestratorStarter (internal/buildercli/cli.go).
type runnerMasterStarter struct {
	runner *shuttleengine.Runner
}

// StartMaster implements websterengine.MasterStarter by delegating to the
// shuttle Runner's non-blocking Start.
func (s runnerMasterStarter) StartMaster(spec shuttleengine.Spec) (websterengine.MasterHandle, error) {
	run, err := s.runner.Start(spec)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// Command returns the cobra command tree for the webster module.
//
// The parent "webster" command carries a PersistentPreRunE that resolves
// cwd -> layout -> shuttle config -> mux config -> webster config -> model
// registry -> resolved roles -> mux engine -> claude engine ->
// shuttleengine.Runner into c, skipping that resolution entirely when the
// group command itself is invoked (bare "lyx webster" listing or an
// unknown-subcommand error via GroupRunE) so neither path requires a git
// repository. Role resolution runs against every one of the config's three
// roles as a pre-flight -- deliberately uniform across all eight verbs,
// mirroring buildercli's own uniform role pre-flight -- so a typo'd role
// alias in webster.yaml aborts every verb here, before any agent ever forks
// or spawns.
func Command() *cobra.Command {
	c := &websterCLI{}

	parent := &cobra.Command{
		Use:   "webster",
		Short: "drive a pinned plan-format plan through a long-lived Master session that forks one implementer per batch",
		Long: `webster takes a pinned plan-format v2 plan (see docs/reference/plan-format.md)
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

			// Every config is anchored at layout.Cwd, matching buildercli's
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

			websterCfg, err := websterengine.LoadConfig(layout.Cwd, "webster")
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
			// webster.yaml aborts every verb here, before any agent ever
			// forks or spawns.
			roles, err := websterengine.ResolveRoles(websterCfg, registry)
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
			c.injector = runner
			c.masterStarter = runnerMasterStarter{runner: runner}
			c.engine = claudeEngine
			c.mux = muxEngine
			c.layout = layout
			c.shuttleCfg = shuttleCfg
			c.cfg = websterCfg
			c.roles = roles
			// Anchored at layout.Cwd, like every config load above and like
			// buildercli's own planDir/builderDir/reportsDir: the
			// initialized _lyx (the weft junction) lives at the directory
			// lyx init ran in, which is Cwd -- not necessarily the git
			// worktree root. Anchoring at WorktreeRoot would, in a
			// nested-initialized repo, resolve these dirs outside the
			// junctioned _lyx the weft commit's RelPath-scoped pathspec
			// never includes, silently stranding every webster artifact
			// outside the weft.
			c.planDir = hubgeometry.PlanDir(layout.Cwd)
			c.websterDir = hubgeometry.WebsterDir(layout.Cwd)
			c.reportsDir = hubgeometry.WebsterReportsDir(layout.Cwd)
			c.promptsDir = hubgeometry.WebsterPromptsDir(layout.Cwd)
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
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
