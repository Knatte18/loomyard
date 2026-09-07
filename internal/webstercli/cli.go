// cli.go builds the cobra command tree for the webster module and the RunCLI seam that wires it
// into the standard io.Writer-based call contract.
// The parent "webster" command carries a PersistentPreRunE that resolves cwd, runs one
// preflight.ResolveMode probe, and delegates to c.wire (wiring.go), which selects hub or standalone
// mode and builds the whole engine stack for whichever mode wins, storing the resolved ingredients
// on websterCLI exactly once per invocation.
// A non-nil ResolveMode error means refuse: it aborts the pre-run right there, before c.wire is ever
// called, rather than selecting a mode -- refusal is a resolution verdict, not a wiring choice.
// The module holds no *lyxcwd.Location: every path it touches -- every _lyx/plan and _lyx/webster
// path included -- arrives as a websterengine.Geometry, told by hubgeom.WebsterGeometry(loc) in hub
// mode or internal/standalonegeom.WebsterGeometry(target, stateDir) in standalone, never re-derived
// from a stored Location.
// Fabric is reached only through the lazy c.openFabric closure, which is nil in standalone mode --
// standalone has no fabric repo by construction, and the closure is never called during wiring in
// either mode.
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
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

	// reedUp brings the standalone reed session up, idempotently, and is set by wireStandalone
	// alone — run calls it immediately before spawning Master and recover-batch immediately before
	// spawning its cold recovery strand, because standalone mode has no other way to a live session:
	// `lyx reed up` is hub-only (its pre-run requires lyxcwd.Resolve), so the session on standalone's
	// own geometry (socket "lyx-<hash8>", state under the derived state directory) can only be booted
	// in-process, mirroring what internal/loomcli's run/drive verbs already do for hub mode. It stays
	// nil in hub mode, where bringing reed up remains the operator's (or loom's) own act.
	//
	// It is called by those two spawning verbs alone, never from wiring, so validate, status, pause
	// and await-batch still boot no tmux server. The membership rule is "does this verb start an OS
	// process of its own", not "does it write": begin-batch and record-batch mutate state but only
	// inject into or read around a pane Master already owns, so a session they could reach exists by
	// construction whenever they are legitimately called.
	reedUp func() error

	// standalonePlanDirOverridden reports whether --plan-dir moved the plan off standalone's
	// default (<stateDir>/_lyx/plan). Set by wireStandalone, read by the run verb alone, which
	// refuses to spawn Master over a moved plan: Master's in-pane verb invocations are flagless and
	// resolve the default, so they could never see the override (see wireStandalone).
	// standalonePlanDirDefault carries that default path for the refusal's own recourse text.
	standalonePlanDirOverridden bool
	standalonePlanDirDefault    string

	shuttleCfg shuttleengine.Config
	cfg        websterengine.Config
	roles      map[websterengine.Role]modelspec.Resolved

	// anchorRel is the hub-mode Location's own anchor-relative path (loc.AnchorRel), empty in
	// standalone. It is the one thing the deleted layout field held that no other told replacement
	// supplies, and fabricSync's own told scope (card 37) needs it for fabricengine.ScopedPathspec.
	anchorRel string

	// geom is the told websterengine.Geometry every verb's Deps construction, and every path this
	// module touches, reads from -- hubgeom.WebsterGeometry(loc) in hub mode,
	// standalonegeom.WebsterGeometry(target, stateDir) in standalone.
	geom websterengine.Geometry
	// refMatcher is the injected fabric-reference class matcher record-batch's and run's own audit
	// consult — a real *fabricengine.RefScanner in hub mode, built eagerly because NewRefScanner only
	// compiles a regexp and cannot fail.
	refMatcher websterengine.RefMatcher
	// openFabric is the lazy fabric-handle opener RunDeps.OpenBisector is built from. It must NOT be
	// opened during PersistentPreRunE: fabricengine.Open stat-checks the paired sibling and would fail
	// the pre-run in the three healthy-but-unwired locations that run validate and status today,
	// which never reach the integration bisect.
	openFabric func() (*fabricengine.Fabric, error)

	// batcher is the load-time-resolved, config-selected batchifier.
	batcher batcher.Batcher

	// stencilsDirFlag, planDirFlag, and targetDirFlag hold the raw, as-parsed values of the three
	// standalone-entry persistent flags (--stencils-dir, --plan-dir, --target-dir), each bound by
	// Command() and read by the wiring function (wiring.go) inside resolvePersistentPreRun. An empty
	// value means the flag was not passed; the wiring function computes each mode's own default
	// rather than a zero-value fallback landing here.
	stencilsDirFlag string
	planDirFlag     string
	targetDirFlag   string
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

// resolvePersistentPreRun resolves cwd, calls preflight.ResolveMode(cwd), and delegates the mode
// decision and the whole engine stack construction to c.wire (wiring.go), storing the resolved
// ingredients on c.
// A non-nil ResolveMode error is the refuse case, and it is handled right here rather than inside
// wire: the refusal deliberately stays in resolvePersistentPreRun because it is a resolution
// verdict, not a wiring choice, so it is surfaced verbatim and aborts the pre-run before c.wire is
// ever called -- wire's own two-row truth table never sees a third value.
// Extracted from Command()'s PersistentPreRunE assignment so a test can invoke it directly against a
// *websterCLI it holds a reference to and inspect the populated fields afterward -- most notably that
// c.openFabric is built here as a closure but never itself called.
// Skips resolution entirely when the group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present.
func (c *websterCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
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

	loc, mode, err := preflight.ResolveMode(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	if err := c.wire(loc, mode, cwd, c.stencilsDirFlag, c.planDirFlag, c.targetDirFlag); err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	return nil
}

// Command returns the cobra command tree for the webster module.
//
// The parent "webster" command carries a PersistentPreRunE (c.resolvePersistentPreRun) that resolves
// cwd and configs into c, skipping resolution when the group command itself is invoked.
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
  lyx webster recover-batch 3 --wait 8m      escalate batch 3 to a cold recovery strand

Modes:
  webster runs in hub mode inside a lyx hub worktree, and in standalone
  mode anywhere else -- a plain git checkout with no lyx hub beside it.
  Three persistent flags cross that boundary: --stencils-dir and
  --plan-dir are optional and read-only in BOTH modes (hub default: the
  hub's own stencils/plan directories; standalone default: the derived
  state directory's own _lyx/stencils and _lyx/plan); --target-dir is
  standalone-only, defaults to the current directory, and is refused in
  hub mode, where the worktree itself is structurally the target. A
  relative value for any of the three is resolved against the current
  directory, and the standalone target is lifted to the root of the git
  repository containing it, so standing in a subdirectory drives the same
  repository, state directory and reed session as standing at its root.

  In standalone mode, run boots its own private reed session (socket
  "lyx-<hash8>", state under the derived state directory) before spawning
  Master -- "lyx reed up" is a hub verb and cannot reach that geometry.

Example (standalone, outside any lyx hub):
  lyx webster run --target-dir /path/to/repo`,
		// RunE is set so that bare "lyx webster" lists subcommands and "lyx
		// webster bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.PersistentFlags().StringVar(&c.stencilsDirFlag, "stencils-dir", "",
		"override the stencils directory read at call time (read-only in both modes; hub default: the hub's own stencils dir; standalone default: the derived state directory's _lyx/stencils)")
	parent.PersistentFlags().StringVar(&c.planDirFlag, "plan-dir", "",
		"override the plan directory parsed at call time (read-only in both modes; hub default: the anchor's _lyx/plan; standalone default: the derived state directory's _lyx/plan)")
	parent.PersistentFlags().StringVar(&c.targetDirFlag, "target-dir", "",
		"standalone-only: the git repository webster drives Master and its forks against; defaults to the current directory, and either way is lifted to the containing repository's root; refused in hub mode, where the worktree is already the target")

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

// persistPlanFingerprintRebaseline saves st when the bracket verb that just failed had already
// re-baselined the plan-staleness fingerprint, and returns the save's own error.
//
// Both bracket verbs re-baseline State.PlanFingerprint the instant a sanctioned plan rewrite lands
// on disk — handle canonicalization inside begin-batch's ValidateDispatch, handle binding and
// exact-tier drift repair inside record-batch — and either verb can then fail on a later step of
// the same call. The rewrite is a durable fact about the run, so discarding the re-baseline along
// with the failed call left the plan on disk carrying webster's own edit while state.json still
// recorded the pre-rewrite fingerprint: every later bracket verb then refused that edit as a
// foreign one, and the advised `--fresh` recourse refused the run outright over the cards that had
// already landed.
//
// fingerprintBefore is the value read out of st immediately before the verb ran. An unchanged
// fingerprint means no rewrite happened, so nothing is written at all — which is what keeps a
// genuine foreign edit failing ErrFingerprintMismatch exactly as it did before.
// Callers invoke this while still holding the state-mutation lease.
func persistPlanFingerprintRebaseline(geom websterengine.Geometry, st *websterengine.State, fingerprintBefore string) error {
	if st == nil || st.PlanFingerprint == fingerprintBefore {
		return nil
	}
	return websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, st)
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
