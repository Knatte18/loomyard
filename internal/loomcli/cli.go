// cli.go builds the cobra command tree for the loom module and the RunCLI seam, which takes an io.Writer.
// The parent "loom" command's pre-run resolves cwd into a *lymcwd.Location; wire builds the engine stack from it.
// loom is hub-only: it has no standalone mode, because it commits its seed into a wired fabric.

package loomcli

import (
	"io"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// loomCLI carries the state that every loom verb shares, filled in once per invocation.
type loomCLI struct {
	// location is the resolved *lymcwd.Location for this invocation.
	// wire reads it to anchor every config load and to build the reed/webster geometries; no verb needs it directly.
	location *lymcwd.Location
	// cwd is the resolved cwd string the pre-run reads via lymcwd.CwdFrom.
	// It reaches the preflight row as Env.Cwd,
	// and that row runs its checks over this exact cwd.
	// That row is the only one that spawns git.
	cwd string
	// cfg is the loaded loom.yaml config.
	// The start/bootstrap verb reads its Discussion and Plan role model-specs and timeout knobs.
	cfg loomengine.Config
	// reed is the constructed reed engine.
	// The bootstrap spawn machinery reads it to add and resolve the driver's own strand.
	reed *reedengine.Engine
	// env is the assembled shedrecipe.Env that the recipe constructor loomrecipe.New takes with the shed paths.
	env shedrecipe.Env
	// shedPaths carries the shed paths that shedengine.Shed reads itself.
	// loomrecipe.New takes it alongside env, and the shed verbs read it directly.
	shedPaths shedbuild.ShedPaths
	// languageSkills are the hub's language skills that wire loads from language.yaml for the agent sessions.
	languageSkills []string
	// language is the whole language.yaml wire loaded, whose rename tools the Webster prompts render.
	language languageconfig.Language
	// runDeps is the assembled websterengine.RunDeps, embedded verbatim as env.WebsterDeps.
	// It is also kept here directly so a test can inspect it without unwrapping env.
	runDeps websterengine.RunDeps
	// registry is the resolved model-spec registry.
	// It is carried on the struct so run.go can pass it to landingDeps without a second modelspec.LoadRegistry call.
	registry modelspec.Registry
	// frictionDir is the absolute Tier 2 friction directory, resolved once in wire.
	// It is empty when Tier 2 is off.
	// It is carried on the struct, so start.go and run.go read it without re-resolving it or re-reading loom.yaml.
	// Left at its zero value by wireLightweight, whose verbs are all read-only and load no module config.
	frictionDir string
	// parentName is the name of the session that spawned this worktree's run.
	// wire takes it from the reed geometry's origin record; it is empty when the worktree has no resolvable parent.
	// Every role loom wires renders its parent directive from it.
	parentName string
	// lymBinary resolves this lym binary for the loop's step children; nil means selfexec.LymBinary.
	// A test sets it to a stand-in path, since selfexec.LymBinary refuses a test binary.
	lymBinary func() (string, error)
	// endFixers, when set, is what an admitted goto calls to end the pair's gate fixers; nil means endRunFixers.
	// Tests set it to record the call.
	endFixers func() error
	// runner is the constructed shuttle runner.
	// It is carried on the struct so run.go can pass it to landingDeps as the landing seam's Shuttle value.
	runner *shuttleengine.Runner
	// shuttleCfg is the loaded shuttle config; the loop's activity reading uses it to find live runs in the worktree.
	// It is zero under the lightweight wiring, whose verbs never run the loop.
	shuttleCfg shuttleengine.Config
	// stepIdleTimeout is the window after which a --until-stop step showing no activity is killed.
	// It comes from loom.yaml's step_idle_timeout_min and is zero under the lightweight wiring.
	// A step at a row waiting on the operator (Beam-Await, PR-Review, Sampler-Session) reads as active and is never killed.
	stepIdleTimeout time.Duration
	// landingCfg is the loaded landing.yaml configuration, loaded once in wire().
	// Loading it there lets an unreconciled hub's absent-config error reach the operator's own terminal on every verb;
	// otherwise it would show only in run's detached driver log.
	landingCfg landingshed.Config
	// suppressWatchdogSpawn, initialised from testing.Testing(), makes spawnWatchdog's call a no-op under a test binary.
	// It enforces the Live-Substrate Spawn Observability invariant, which bars re-exec of os.Executable() under go test.
	// Without it, every test that reaches the bootstrap would re-exec the test binary,
	// and the whole suite would run recursively.
	suppressWatchdogSpawn bool
	// spawnWatchdog defaults to reedengine.SpawnWatchdog.
	// It is an injection point,
	// so a test can assert the call's arguments.
	// Under go test the real call returns at once,
	// and it leaves no process, no log line, and nothing else to assert against.
	spawnWatchdog func(hubPath, tmuxPath, shellPath string, suppress bool)
	// midMerge is the seam through which start reads the pair's merge state before spawning or resuming a driver.
	// The Test Tier Purity Invariant bars real mid-merge pairs in untagged tests; tests substitute a canned state or error.
	midMerge func(*lymcwd.Location) (fabricengine.MidMergeState, error)
	// recordedParentBranch is the seam through which start and resume read the pair's recorded parent branch.
	// It tells a parked merge-in of that branch from any other merge state; a test substitutes a canned branch or error.
	recordedParentBranch func(*lymcwd.Location) (string, error)
	// parentCodeDirt is the seam through which the Finalize goto guard reads the parent pair's code checkout.
	// It reports the tracked dirty paths and whether fabric holds a merge record; untagged tests substitute canned answers.
	parentCodeDirt func() (checkout string, paths []string, mergeRecord bool, err error)
	// spec is the shedverbs.Spec that the pre-run fills in place (arm.go).
	// The shedverbs verbs read it at run time.
	// It is non-nil after newLoomCLI, so Command() can hand the same pointer to shedverbs.Verbs before the pre-run has run.
	spec *shedverbs.Spec
	// parentFlag carries the --parent value of the run and step verbs.
	// A closure cannot carry it, since the flag variable lives in Command().
	// The PreStep hook that reads it is built inside arm, which sees only the cwd, verb and args.
	// A PersistentPreRunE's args are positional only, never parsed flags.
	parentFlag string
	// runID is the run id that arm (arm.go) resolves from args[0] when present.
	// Otherwise it is shedrun.SelfRunID.
	// arm records it here before either wiring call.
	// wireLightweight and wire (wiring.go) read it back to build every shedrun.* path.
	runID string
	// recipe is the name of the recipe that the run's seed records.
	// armAt reads it once, right after the run id, through resolveRecipe.
	// It is shedrun.RecipeTapestry until the run has a seed; routing, the shed builder, wiring and landing read it.
	recipe string
	// loomPath is true when the `lym loom` subtree armed the receiver, which serves loom runs only.
	// The arming `lym shed` shares leaves it false, so a refusal of a non-loom recipe never sits in `lym shed`'s path.
	loomPath bool
	// caller is the caller label armAt classifies from the strand variable and specFor copies onto the Spec;
	// empty on a hand-populated receiver, which records none.
	caller string
	// routing is the recipe's producer-graph projection, which armAt loads through loadRouting.
	// specFor copies it onto the Spec; it is zero on a hand-populated receiver, which reports no progress.
	routing shedengine.Routing
	// driverStarter is the seam through which the llm arm starts the loom driver session's shuttle run.
	// It wraps a *shuttleengine.Runner.
	// The seam exists because *shuttleengine.Runner is a concrete type.
	// The Test Tier Purity Invariant bars a real spawn from an untagged file.
	driverStarter driverStarter
	// driverSender is the seam through which `start` types the resume line into a parked, live driver's pane.
	driverSender driverSender
	// driverResumeWait is the pause between resume send attempts;
	// a test substitutes a counting fake so no real sleep happens.
	// A nil value means no pause.
	driverResumeWait func()
	// driverPaneProbe is the seam through which the llm arm reads and removes driver strands.
	// It wraps the *reedengine.Engine that c.reed holds.
	// The engine is a concrete type, and the Test Tier Purity Invariant bars a real reed call from an untagged file.
	// The seam lets a test substitute a fake.
	driverPaneProbe driverPaneProbe
	// driverDirectory is the seam through which `resume` reads the driver strand from reed's sessionless directory.
	driverDirectory driverDirectoryProbe
	// bouncerSubdir is the seam through which `resume` reads a row's run subdirectory and whether it is a Bouncer row.
	// It is loomrecipe.BouncerRunSubdir in production and a test substitutes a canned answer.
	bouncerSubdir func(row string) (subdir string, isBouncer bool, err error)
	// reflectionShuttle is the shuttle every friction reflection goes through, wired to the shuttle runner in production.
	// It is the seam through which tests fake the reflection agent and its filing.
	// It stays nil on the lightweight wiring path, where a reflection reports a missing shuttle as a failure.
	reflectionShuttle frictionengine.Shuttle
	// rowFrictionStatus is the status the Friction-Reflect row's closure recorded in this process.
	// It is empty when the row did not run here, and loomPostRun reports it on RunDone.
	rowFrictionStatus string
	// spawnGoDriver is the seam through which a Go-driver spawn starts a detached `lym loom drive` and returns its pid.
	// A test substitutes a counting fake, because the Test Tier Purity Invariant bars a real spawn from an untagged file.
	spawnGoDriver func() (int, error)
	// spawnGoDriverFrom starts a detached drive from the executable a caller resolved, for a handover after a deploy.
	spawnGoDriverFrom func(executable string) (int, error)
	// ownerOf judges who owns the run of slug, and a nil ownerOf reads the run as owned here without calling anything.
	ownerOf func(l *lymcwd.Location, slug string) (ownerVerdict, error)
}

// newLoomCLI is the only place production code may build a *loomCLI.
// It sets suppressWatchdogSpawn and spawnWatchdog on every receiver it builds,
// so that no constructor can forget one and leave a nil spawnWatchdog, which would panic rather than degrade.
func newLoomCLI() *loomCLI {
	c := &loomCLI{
		suppressWatchdogSpawn: testing.Testing(),
		spawnWatchdog:         reedengine.SpawnWatchdog,
		midMerge:              fabricengine.MidMerge,
		recordedParentBranch:  readRecordedParentBranch,
		spec:                  &shedverbs.Spec{},
	}
	c.parentCodeDirt = c.parentCodeDirtFromFabric
	c.ownerOf = func(l *lymcwd.Location, slug string) (ownerVerdict, error) {
		return runOwnerVerdict(l, slug, machinename.Name)
	}
	c.spawnGoDriverFrom = func(executable string) (int, error) { return spawnDetachedDrive(c.location, executable) }
	c.spawnGoDriver = func() (int, error) {
		executable, err := selfexec.LymBinary()
		if err != nil {
			return 0, err
		}
		return c.spawnGoDriverFrom(executable)
	}
	return c
}

// runnerMasterStarter adapts *shuttleengine.Runner to websterengine.MasterStarter.
//
// The identical adapter in internal/webstercli is a deliberate duplicate.
// Both types are unexported, and sharing one would need a cli package to import another.
// That import would couple two independent cobra seams.
type runnerMasterStarter struct {
	runner *shuttleengine.Runner
}

// StartMaster implements websterengine.MasterStarter.
//
// It starts the run with the gate attached, so the gate is in place before the caller blocks on the run.
// A zero gate makes the start behave exactly as an ungated one.
func (s runnerMasterStarter) StartMaster(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
	run, err := s.runner.StartGated(spec, gate)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// resolvePersistentPreRun reads cwd through lymcwd.CwdFrom, arms the invoked verb, and stores the result on c.spec.
// Arming resolves the location and builds the engine stack through c.wire (wiring.go).
// The pre-run skips the loom group command, so a bare listing or an unknown subcommand needs no git repository.
// It also skips the circling, decision and peers groups and their verbs.
// Those resolve their own target worktree in their own PersistentPreRunE.
// cmd/lym sets cobra.EnableTraverseRunHooks, so cobra runs this pre-run for them too;
// without the skip it would arm them and read a slug as a run-id.
// The checklist pane verb is skipped as well, because arming refuses a missing seed and a pane must outlive one.
func (c *loomCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "loom" || cmd.Name() == "checklist" || inReviewGroup(cmd) {
		return nil
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	armed, err := c.arm(cwd, cmd.Name(), args)
	if err != nil {
		// The error arm returns from lymcwd.Resolve already says it is not a git repository.
		// Report it as it is, so that text is not printed twice.
		clihelp.Abort(ctx, c.reportArmFailure(out, cmd, args, err))
		return nil
	}
	*c.spec = armed
	return nil
}

// reportArmFailure prints the refusal arm returned and gives the exit code.
// A `step` with `--until-stop` and a resolved location is a loop stop, which writes the run's status file as failed.
// Every other verb, a step without the flag, and a failed resolve, which names no run, get the plain arming refusal.
func (c *loomCLI) reportArmFailure(out io.Writer, cmd *cobra.Command, args []string, err error) int {
	untilStop, _ := cmd.Flags().GetBool(shedverbs.UntilStopFlag)
	if cmd.Name() != "step" || !untilStop || c.location == nil {
		return shedverbs.ReportArmError(out, cmd.Name(), err)
	}
	runID := shedrun.SelfRunID
	if len(args) > 0 {
		runID = args[0]
	}
	loopID, _ := cmd.Flags().GetString(shedverbs.LoopDetachedFlag)
	return shedverbs.ReportLoopArmError(out, shedverbs.ArmStop{
		RunID:          shedrun.ResolveRunID(c.location, runID),
		StatusPath:     shedrun.StatusFile(c.location, runID),
		RunLockPath:    shedrun.RunLock(c.location, runID),
		StatusLockPath: shedrun.StatusLock(c.location, runID),
		LoopLockPath:   shedrun.LoopLock(c.location, runID),
		PIDPath:        shedrun.LoopPIDFile(c.location, runID),
		EnvelopePath:   shedrun.LoopEnvelope(c.location, runID),
		LoopID:         loopID,
	}, err)
}

// inReviewGroup reports whether cmd is loom's circling, decision or peers group, or a command under any of them.
// They resolve their own target worktree, from an optional slug or the caller's cwd.
// The loom pre-run must never arm them as a run-id.
func inReviewGroup(cmd *cobra.Command) bool {
	for p := cmd; p.HasParent(); p = p.Parent() {
		if (p.Name() == "circling" || p.Name() == decisionGroup || p.Name() == peersGroup) && p.Parent().Name() == "loom" {
			return true
		}
	}
	return false
}

// verbUsesLightweightWiring reports whether the named loom subcommand needs only cheap path accessors.
// Such a verb needs no module config, engine or producer, so a broken module config must not refuse it.
// It reads only paths, never a loaded config.
// Every other verb builds or drives a producer, so it keeps the full wire() and its early config refusal.
// That includes `step`, which drives a producer through `shedengine.Shed.Step`.
func verbUsesLightweightWiring(name string) bool {
	switch name {
	case "status", "pause", "goto", "validate-discussion", "validate-plan", "validate-parts", "validate-description", "lint-comments", "lint-pattern", "fmt", "commit-records", "insert-card", "remove-card", "restore-renumber", "raddle-propose", "raddle-approve":
		return true
	default:
		return false
	}
}

// loomVerbTexts carries the help text and audience of loom's shedverbs-driven verbs under the loom mount.
var loomVerbTexts = shedverbs.VerbTexts{
	Run: shedverbs.VerbText{
		Use:      "run [<run-id>]",
		Audience: clihelp.AudienceRole,
		Short:    "run loom's phase machine in the foreground, with no status strand and no detached driver",
		Long: `run runs loom's phase machine in the foreground: no status strand and
no detached driver. It is the escape hatch for debugging and CI.

run is NOT tmux-free. Every LLM row underneath it -- Discussion-Write,
Plan-Write, Describe, and all three review segments -- spawns its agent through
shuttle into a reed pane, so a live tmux session is required. run
ensures that session itself, exactly as "lym loom start" does, rather than
failing several producers deep once a row first tries to add a strand.
What run does not do is add the status strand or spawn a detached driver.

run needs a seed and a status file, writes neither and commits nothing.
The seed comes from "lym loom start", which seeds and spawns the LLM driver,
or from "lym shed seed self --recipe tapestry --driver go --param
parent=<parent-branch>", which writes a driverless seed; <parent-branch> is
the pair's recorded parent branch, "parent_branch" in fabric's origin record.
Only "lym loom start" or a first "lym loom step" writes the status file, so
after a driverless seed "lym loom step" comes first. An optional run-id
positional addresses a run other than this worktree's own default ("self"),
and run refuses when no seed already exists at that run-id.

Example:
  lym loom run
  lym loom run <run-id>`,
	},
	Step: shedverbs.VerbText{
		Use:      "step [<run-id>]",
		Audience: clihelp.AudienceRole,
		Short:    "drive exactly one producer of a seeded run, reporting a JSON envelope",
		Long: `step needs a seed at the addressed run-id and refuses without one, since it
never writes a seed itself. With one, it runs the seed-agreement,
status-seeding and commit block "lym loom start" runs, then drives exactly one
producer through shedengine.Shed's own Step.

Two routes write the seed: "lym loom start", which seeds and spawns the LLM
driver, and "lym shed seed self --recipe tapestry --driver go --param
parent=<parent-branch>", which writes a driverless seed; <parent-branch> is the
pair's recorded parent branch, "parent_branch" in fabric's origin record. After
a driverless seed the first driving call is "lym loom step", whose bootstrap
seeds the status file; from then on step or run drives.

step spawns no detached driver and loops over nothing: it is the single-producer primitive an external supervisor
drives, one invocation at a time. The printed envelope is short: its
"continue" field says whether to step again, and it names "trace_file" (the
durable trace this invocation wrote) and "envelope_path" (the full
envelope, holding "friction_dir", "scratch_dir" and
"next_interrupt_policy"). Every error envelope carries "kind", and also
"transient": the transient class name, or empty when the failure is not
transient, so a supervisor can read what the step did and repair from it.
--full prints the full envelope on stdout instead; the record, exit code
and run state are unchanged.

An optional run-id positional addresses a run other than this worktree's
own default ("self").

Example:
  lym loom step
  lym loom step --parent main
  lym loom step <run-id>`,
	},
	Status: shedverbs.VerbText{
		Use:      "status [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "report loom's current phase, once or as a live-tailed watch",
		Long: `status reports the current phase-machine state.

Without --watch, it reads the status file once and emits a single JSON
envelope carrying the current producer, state, error text, pause flag,
"pause_reason" (why a paused run is paused), composed activity, history length, the task's slug/parent, and the
interrupt policy for the current producer. While a pause covers the run,
the envelope also carries "paused_since" (when the pause began, RFC 3339)
and "paused_by" (the pause's scope); a run no pause covers carries neither.

With --watch, it performs the same read once as a pre-flight, then tails
the file and never exits, printing a line only when the composed activity
actually CHANGES rather than once per poll: the row, the state, or a wait's
kind or detail, never a wait's elapsed time alone. A quiet pane thus means the
current producer is still working, not that the tail has stopped. This is
the one documented interactive-handoff exception on this verb, taken
narrowly on the tail only, after every fallible step has already run
pre-flight.

An optional run-id positional addresses a run other than this worktree's
own default ("self"); status refuses when no seed already exists at that
run-id.

Example:
  lym loom status
  lym loom status --watch
  lym loom status --watch --interval 200ms
  lym loom status <run-id>`,
	},
	Pause: shedverbs.VerbText{
		Use:      "pause [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "record a stop condition that pauses loom at a named producer boundary",
		Long: `pause records a stop condition the running phase machine fires at a named
producer boundary. It does not kill anything.

--before <producer> pauses the run before that producer runs. --after
<producer> pauses after that producer returns a routed running outcome, at
the row it routed to. Either replaces a recorded one. --clear removes both
conditions, and refuses beside --before or --after.

--by <label> records a free-text caller label, such as operator or the orch's
strand name, beside the condition it accompanies. The label is recorded in
the paused run's pause_reason; shed is reserved for lym's own automatic steps
and refused.

A bare pause, with no flag, refuses for a run: it no longer stops
anything. The refusal names the two ways forward: "lym tender pause <slug>"
stops the run's agents now, and "--after <current_producer>" halts the run at
its next producer boundary.

--after naming a row that waits for the parent lock also ends that wait
mid-row, halting the run at the row; the condition stays recorded until
"lym loom goto" clears it, and "lym loom resume" then moves the run.

An optional run-id positional addresses a run other than this worktree's
own default ("self"); pause refuses when no seed already exists at that
run-id.

Example:
  lym loom pause --after Plan-Write
  lym loom pause <run-id> --before Plan-Write
  lym loom pause --clear`,
	},
	Goto: shedverbs.VerbText{
		Use:      "goto [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "move a halted run onto a named row, paused, keeping its settled reviews and spent budget",
		Long: `goto moves a halted run onto the row named by --to and leaves it paused.
It keeps the segment's settled verdicts and spent bounce budget,
and resets only what the target row needs to run:
the error, the transient mark, a pending resume label and the pause conditions.
It records a "goto" history entry and itself as the run's pause reason.
It is refused onto a review or fix row of a review segment whose verdict has settled.
A bounce-budget-exhausted halt is lifted only by "lym loom resume", never by goto.
It refuses while a driver holds the run lock and on a done run, and
never re-opens a finished run. goto moves a halted run only back, to a row at or before its current row (strictly before when the run is awaiting), and refuses a running run.

An optional run-id positional addresses a run other than this worktree's
own default ("self"); goto refuses when no seed already exists at that
run-id.

Example:
  lym loom goto --to Plan-Write
  lym loom goto <run-id> --to Plan-Write`,
	},
}

// Command returns the cobra command tree for `lym loom`:
// the generic shed verbs that act on a run, and the module's other subcommands.
func Command() *cobra.Command {
	c := newLoomCLI()

	parent := &cobra.Command{
		Use:   "loom",
		Short: "bootstrap and drive one task's phase machine for this worktree",
		Long: `loom drives one task's phase machine over a per-worktree status.json, on
the generic shed engine. The machine walks its producer rows: a two-row
preflight, then Discussion, Plan and Webster, each followed by its own LLM
review segment that loops until it approves or escalates, then Describe,
Publish and Finalize, and last Friction-Reflect, before the run records done.

Reach for it to start a task in its worktree, watch and steer the run, and
answer the gates that wait on the operator or the run's parent.

A verb taking an optional <slug> addresses the task worktree it runs in; from
the prime the slug is required.

Example:
  lym loom start
  lym loom status --watch
  lym loom circling accept <slug>`,
		// RunE makes bare "lym loom" list subcommands; "lym loom bogus" gets a JSON error envelope, not cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(loomVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb, gotoVerb := verbs[0], verbs[1], verbs[2], verbs[3], verbs[4]
	stepVerb.Flags().StringVar(&c.parentFlag, "parent", "", "write the pair's provenance record once for a worktree created before that record existed; refused when it disagrees with an already-recorded value")

	// Each generic verb takes at most one positional argument, the run-id, which `resolveRunID` in arm.go reads.
	// Without that cap, cobra accepts a second positional and the verb silently ignores it.
	// `lym loom start` takes no positional, since it takes no run-id.
	runVerb.Args = cobra.MaximumNArgs(1)
	stepVerb.Args = cobra.MaximumNArgs(1)
	statusVerb.Args = cobra.MaximumNArgs(1)
	pauseVerb.Args = cobra.MaximumNArgs(1)
	gotoVerb.Args = cobra.MaximumNArgs(1)

	parent.AddCommand(c.startCmd(), c.resumeCmd(), runVerb, stepVerb, statusVerb, pauseVerb, gotoVerb, c.validateDiscussionCmd(), c.validatePlanCmd(), c.validatePartsCmd(), c.joinPartsCmd(), c.insertCardCmd(), c.removeCardCmd(), c.restoreRenumberCmd(), c.validateDescriptionCmd(), c.lintCommentsCmd(), c.lintPatternCmd(), c.fmtCmd(), c.commitRecordsCmd(), c.prContinueCmd(), c.samplerDoneCmd(), c.raddleProposeCmd(), c.raddleApproveCmd(), c.circlingCmd(), c.decisionCmd(), c.peersCmd(), c.arbiterCmd(), c.checklistCmd(), c.driveCmd())

	return parent
}

// RunCLI is the public seam for the loom module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree.
// All output, including cobra's error text, goes to out.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the loom module CLI in cwd, or in the process working directory when cwd is empty.
// An empty cwd goes through clihelp.Execute.
// Any other cwd is seeded into the execution context through clihelp.ExecuteIn.
// The branch exists because lymcwd.WithCwd panics on an empty directory,
// so an empty cwd must not reach clihelp.ExecuteIn.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
