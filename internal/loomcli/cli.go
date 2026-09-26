// cli.go builds the cobra command tree for the loom module and the RunCLI seam that wires it into
// the standard io.Writer-based call contract.
// The parent "loom" command carries a PersistentPreRunE that resolves cwd once into a
// *lyxcwd.Location and delegates the whole engine-stack construction to c.wire (wiring.go), storing
// the resolved ingredients on loomCLI exactly once per invocation.
// loom is hub-only: it resolves through lyxcwd.Resolve the way internal/reedcli does, never through
// preflight.ResolveMode's degrade-to-standalone probe -- loom needs a wired fabric to commit its
// seed into, so there is no meaningful standalone mode (see the plan's
// hub-only-resolution-for-loom decision). The resolved value is named "location" throughout, never
// "loc" or "cwd".
package loomcli

import (
	"io"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// loomCLI carries the fields wire (wiring.go) populates; every loom verb hangs off this receiver.
type loomCLI struct {
	// location is the resolved *lyxcwd.Location for this invocation. wire reads it to anchor every
	// config load and to build the reed/webster geometries; no verb needs it directly.
	location *lyxcwd.Location
	// cwd is the resolved cwd string the pre-run read via lyxcwd.CwdFrom. It travels to
	// preflightEntry as Env.Cwd, and that entry makes the same preflightshed.NewPreflight(name,
	// env.Cwd) call over this exact cwd -- Preflight is the one row that spawns git.
	cwd string
	// cfg is the loaded loom.yaml config. Batch 5's start/bootstrap verb reads its Discussion/Plan
	// role model-specs and timeout knobs.
	cfg loomengine.Config
	// reed is the constructed reed engine. Batch 5's bootstrap spawn machinery reads it to add and
	// resolve the driver's own strand.
	reed *reedengine.Engine
	// env is the assembled shedrecipe.Env that runCmd (run.go) passes to loomrecipe.New.
	env shedrecipe.Env
	// shedPaths carries the five told values shedengine.Shed itself reads, which runCmd passes
	// alongside env, and which statusCmd, pauseCmd, and startCmd read directly.
	shedPaths shedbuild.ShedPaths
	// runDeps is the assembled websterengine.RunDeps, embedded verbatim as env.WebsterDeps. It is
	// also kept here directly so a test can inspect it without unwrapping env.
	runDeps websterengine.RunDeps
	// registry is the resolved model-spec registry, carried onto the struct so run.go can pass
	// it to landingDeps without a second modelspec.LoadRegistry call.
	registry modelspec.Registry
	// frictionDir is the absolute Tier 2 friction directory resolved once in wire, empty when Tier 2
	// is off. It is carried on the struct so start.go and run.go read it without re-resolving it or
	// re-reading loom.yaml a second time. Left at its zero value by wireLightweight, whose verbs are
	// all read-only and load no module config.
	frictionDir string
	// runner is the constructed shuttle runner, carried onto the struct so run.go can pass it to
	// landingDeps as the landing seam's Shuttle value.
	runner *shuttleengine.Runner
	// landingCfg is the loaded landing.yaml configuration, loaded once in wire() per the
	// landing-config-loads-in-wire decision, so an unreconciled hub's absent-config error reaches
	// the operator's own terminal on every verb, not only inside run's detached driver log.
	landingCfg landingshed.Config
	// suppressWatchdogSpawn, initialised from testing.Testing(), makes spawnWatchdog's call a no-op
	// under a test binary. It exists to enforce the Live-Substrate Spawn Observability invariant's
	// "never re-exec os.Executable() under go test" clause -- without it, every test reaching the
	// bootstrap would re-exec the test binary and run the whole suite recursively.
	suppressWatchdogSpawn bool
	// spawnWatchdog defaults to reedengine.SpawnWatchdog and is an injection point so a test can
	// assert the call site's arguments -- under go test the real call returns immediately and
	// leaves no process, no log line, and nothing else to assert against. Named after
	// awaitRunLock's own four injected seams (bootstrap.go) and this file's own doc comment
	// principle that the verb body is assembly over judgment already under test.
	spawnWatchdog func(hubPath, tmuxPath, shellPath string, suppress bool)
	// spec is the shedverbs.Spec the pre-run fills in place (arm.go) and the four shedverbs
	// verbs read at run time. It is always non-nil after newLoomCLI, so Command() can hand the
	// same pointer to shedverbs.Verbs before the pre-run has ever run.
	spec *shedverbs.Spec
	// parentFlag carries "run"'s and "step"'s own --parent value: a closure cannot carry it,
	// since the flag variable lives in Command() while the PreStep hook that reads it is built
	// inside arm, which sees only (cwd, verb, args) -- and a PersistentPreRunE's args are
	// positional only, never parsed flags.
	parentFlag string
	// runID is the run-id arm (arm.go) resolves from args[0] when present, shedrun.SelfRunID
	// otherwise, and records here before either wiring call -- wireLightweight and wire (wiring.go)
	// read it back to build every shedrun.* path, in place of a hardcoded shedrun.SelfRunID, so a
	// later card and batch 7's addressing surface can read it too.
	runID string
	// entryObservation carries loomPreRun's entry observation forward to loomPostRun, since
	// PreRun returns no envelope map of its own.
	entryObservation loomengine.EntryObservation
	// driverStarter is the seam through which the llm arm starts the ly-drive session's shuttle run,
	// wrapping the same *shuttleengine.Runner c.runner already carries. The seam exists because the
	// Test Tier Purity Invariant bars a real spawn from an untagged file and *shuttleengine.Runner is
	// a concrete type.
	driverStarter driverStarter
	// driverPaneProbe is the seam through which the llm arm reads and removes driver strands,
	// wrapping the same *reedengine.Engine c.reed already carries. The seam exists for the same
	// reason driverStarter does: *reedengine.Engine is a concrete type and the Test Tier Purity
	// Invariant bars a real reed call from an untagged file.
	driverPaneProbe driverPaneProbe
	// armedVerb is the verb armAt was called with. reflectFrictionRow reads it at call time so the
	// Friction-Reflect row reflects only under "run", a fact of this invocation rather than the
	// recorded seed driver, which the Driver Choice Single-Site Invariant bars any code path from
	// gating behaviour on.
	armedVerb string
	// rowFrictionStatus is the status the Friction-Reflect row's closure recorded in this process;
	// empty when the row did not run here. loomPostRun reports it on RunDone.
	rowFrictionStatus string
}

// newLoomCLI is the only place production code may build a *loomCLI: it is what keeps
// suppressWatchdogSpawn and spawnWatchdog set identically on every constructed receiver, so neither
// of this package's two constructors (Command, StartAliasCommand) can forget one and leave a nil
// spawnWatchdog to panic rather than degrade.
func newLoomCLI() *loomCLI {
	return &loomCLI{
		suppressWatchdogSpawn: testing.Testing(),
		spawnWatchdog:         reedengine.SpawnWatchdog,
		spec:                  &shedverbs.Spec{},
	}
}

// runnerMasterStarter adapts *shuttleengine.Runner to websterengine.MasterStarter.
//
// This is a deliberate duplication of the identical adapter in internal/webstercli, per the plan's
// duplicated-cli-adapters-over-a-cli-to-cli-import decision: both types are unexported in their home
// packages, and a <module>cli importing another <module>cli has no precedent in this repo and would
// couple two independent cobra seams. Two small duplications are cheaper than that coupling.
type runnerMasterStarter struct {
	runner *shuttleengine.Runner
}

// StartMaster implements websterengine.MasterStarter.
//
// It spends Runner.StartGated rather than Runner.Start so the gate reaches the run before the
// caller blocks on it; a zero gate makes StartGated behave exactly as Start.
func (s runnerMasterStarter) StartMaster(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
	run, err := s.runner.StartGated(spec, gate)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// resolvePersistentPreRun resolves cwd via lyxcwd.CwdFrom, resolves it into a *lyxcwd.Location via
// lyxcwd.Resolve, and delegates the whole engine-stack construction to c.wire (wiring.go), storing
// the resolved ingredients on c.
// Skips resolution entirely when the loom group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present.
func (c *loomCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "loom" {
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

	armed, err := c.arm(cwd, cmd.Name(), args)
	if err != nil {
		// arm's own lyxcwd.Resolve error is already self-describing (it IS the "not a git
		// repository" sentinel); pass it through bare rather than doubling that same text on
		// top of it -- exactly as every other arm/wire error is reported.
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	*c.spec = armed
	return nil
}

// verbUsesLightweightWiring reports whether the named loom subcommand needs nothing beyond the
// resolved location, the two status-file paths, and a handful of cheap path accessors -- no module
// config, no engine, no producer.
//
// The set is the two read-only status verbs (status, pause) plus the two standalone format
// self-checks (validate-discussion, validate-plan), which read only c.env's path fields and never a
// loaded config -- crucible round sonnet5-xhigh-r8's F2 extended the set from the original two after
// finding the writer agents' own stencil-mandated pre-handoff self-check failed on an unrelated
// module's broken config, the identical hazard that got status/pause this lightweight path in the
// first place (see wireLightweight's own doc comment for that history). Every other verb builds or
// drives producers and keeps the full wire(), including its early config refusal. "step" is
// deliberately excluded from this set for that same reason: it drives a producer through
// shedengine.Shed's own Step, so it needs the full wire() and its early config refusal exactly as
// "start" and "run" do.
func verbUsesLightweightWiring(name string) bool {
	switch name {
	case "status", "pause", "validate-discussion", "validate-plan":
		return true
	default:
		return false
	}
}

// loomVerbTexts carries loom's four shedverbs-driven verbs' Use/Short/Long text, lifted verbatim
// from their original hand-written constructors (run.go, step.go, status.go, pause.go, since
// deleted) before shedverbs.Verbs took over their bodies -- including every Example: block and
// every embedded newline, byte-for-byte.
var loomVerbTexts = shedverbs.VerbTexts{
	Run: shedverbs.VerbText{
		Use:   "run [run-id]",
		Short: "run loom's phase machine in the foreground, with no status strand and no terminal handover",
		Long: `run runs loom's phase machine in the foreground: no status strand and
no terminal handover. It is the escape hatch for debugging and CI.

run is NOT tmux-free. Every LLM row underneath it -- Discussion-Write,
Plan-Write, and all three review segments -- spawns its agent through
shuttle into a reed pane, so a live tmux session is required. run
ensures that session itself, exactly as "lyx loom start" does, rather than
failing several producers deep once a row first tries to add a strand.
What run does not do is add the status strand or hand the terminal over.

run never seeds a status file and never commits anything -- only
"lyx loom start" seeds, because only it owns the commit-before-precondition
ordering the bootstrap needs. run itself never seeds either: an optional
run-id positional addresses a run other than this worktree's own default
("self"), and run refuses when no seed already exists at that run-id.

Example:
  lyx loom run
  lyx loom run <run-id>`,
	},
	Step: shedverbs.VerbText{
		Use:   "step [run-id]",
		Short: "bootstrap idempotently and drive exactly one producer, reporting a JSON envelope",
		Long: `step bootstraps this worktree's loom task exactly as "lyx loom start" does --
seeding the status file when absent and committing it into the fabric --
and then drives exactly one producer through shedengine.Shed's own Step.

step spawns no detached driver, hands the terminal to nothing, and loops
over nothing: it is the single-producer primitive an external supervisor
drives, one invocation at a time. The envelope's "continue" and
"next_interrupt_policy" fields say what to do next; it also names
"trace_file" (the durable trace this invocation wrote), "friction_dir" and
"scratch_dir". Every error envelope carries the same three keys beside
"kind", so a supervisor can read what the step did and repair from it.

An optional run-id positional addresses a run other than this worktree's
own default ("self"); step refuses when no seed already exists at that
run-id, since it never writes one itself.

Example:
  lyx loom step
  lyx loom step --parent main
  lyx loom step <run-id>`,
	},
	Status: shedverbs.VerbText{
		Use:   "status [run-id]",
		Short: "report loom's current phase, once or as a live-tailed watch",
		Long: `status reports the current phase-machine state.

Without --watch, it reads the status file once and emits a single JSON
envelope carrying the current producer, state, error text, pause flag,
composed activity, history length, the task's slug/parent, and the
interrupt policy for the current producer.

With --watch, it performs the same read once as a pre-flight, then tails
the file and never exits, printing a line only when the composed activity
actually CHANGES rather than once per poll -- so a quiet pane means the
current producer is still working, not that the tail has stopped. This is
the one documented interactive-handoff exception on this verb, taken
narrowly on the tail only, after every fallible step has already run
pre-flight.

An optional run-id positional addresses a run other than this worktree's
own default ("self"); status refuses when no seed already exists at that
run-id.

Example:
  lyx loom status
  lyx loom status --watch
  lyx loom status --watch --interval 200ms
  lyx loom status <run-id>`,
	},
	Pause: shedverbs.VerbText{
		Use:   "pause [run-id]",
		Short: "request a pause at loom's next producer boundary",
		Long: `pause sets a request the running phase machine consumes at its next
producer boundary. It does not kill anything -- the machine itself clears
the flag in the persist that records the paused state.

An optional run-id positional addresses a run other than this worktree's
own default ("self"); pause refuses when no seed already exists at that
run-id.

Example:
  lyx loom pause
  lyx loom pause <run-id>`,
	},
}

// Command returns the cobra command tree for the loom module.
func Command() *cobra.Command {
	c := newLoomCLI()

	parent := &cobra.Command{
		Use:   "loom",
		Short: "bootstrap and drive one loom task's phase machine for this worktree",
		Long: `loom drives one task's phase machine over a per-worktree status.json,
on the generic shed engine. The machine walks its producer rows: a
two-row preflight, then Discussion, Plan, and Webster, each of the three
followed by its own LLM review segment that loops until it approves or
escalates, then Publish and Finalize, and last Friction-Reflect, which runs
"run"'s Tier 2 friction reflection before the run records done. "start" is
the bootstrap verb: it seeds the status file, commits the seed, and spawns/attaches the
detached driver session; "run" is the no-tmux escape hatch that runs the
phase machine in the foreground for debugging and CI; "step" bootstraps
idempotently and drives exactly one producer, reporting a JSON envelope --
the single-producer primitive an external supervisor drives; "status"
reports the current phase and, with --watch, tails it, printing a line
only when the activity changes; "pause" requests a pause at the next
producer boundary. "validate-discussion" and "validate-plan" are the
standalone form of the mechanical gates Discussion-Write's and Plan-Write's
own rows carry, callable by the writer agent before handoff.

Example:
  lyx loom start
  lyx loom run
  lyx loom step
  lyx loom status
  lyx loom status --watch
  lyx loom pause
  lyx loom validate-discussion
  lyx loom validate-plan`,
		// RunE is set so that bare "lyx loom" lists subcommands and "lyx
		// loom bogus" emits a JSON error envelope instead of falling
		// through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(loomVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb := verbs[0], verbs[1], verbs[2], verbs[3]
	stepVerb.Flags().StringVar(&c.parentFlag, "parent", "", "write the pair's provenance record once for a worktree created before that record existed; refused when it disagrees with an already-recorded value")

	// Each of the four generic verbs takes at most one positional argument -- the run-id arm
	// resolves (arm.go). None declared an Args validator before this card, so a second positional
	// was silently swallowed and the command addressed "self" regardless -- the worst of the
	// available behaviours. "lyx loom start" keeps its own arity (no positional at all): it takes
	// no run-id.
	runVerb.Args = cobra.MaximumNArgs(1)
	stepVerb.Args = cobra.MaximumNArgs(1)
	statusVerb.Args = cobra.MaximumNArgs(1)
	pauseVerb.Args = cobra.MaximumNArgs(1)

	parent.AddCommand(c.startCmd(), runVerb, stepVerb, statusVerb, pauseVerb, c.validateDiscussionCmd(), c.validatePlanCmd())

	return parent
}

// RunCLI is the public seam for the loom module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the capture writer
// for all output (including cobra's error text).
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
