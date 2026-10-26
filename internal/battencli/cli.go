// cli.go builds the cobra command tree for the batten module and the RunCLI/RunCLIIn seams that
// wire it into the standard io.Writer-based call contract.
//
// Package battencli is the module that drives one task worktree's whole lifecycle as a single
// Shed run from the hub's prime worktree. It imports internal/battenshed and
// internal/battenrecipe, neither of which imports cobra, and it is outside the Fabric Vocabulary
// Invariant's owner set, so no identifier, literal, or comment in this package -- or in either of
// those two it imports -- may name either side of the pair: write "the task worktree", "the pair",
// and "the hub's prime worktree" instead.
//
// "lyx batten cleanup [--apply]" (cleanup.go) removes done runs' records and scratch and aged orphan scratch, as a dry run unless given --apply.
//
// "lyx batten cancel <slug> [--remove]" (cancel.go, cancelcmd.go) ends a run for good from the prime, whatever its state.
// It refuses first, with nothing ended: a hub pause, work that reached the parent, an owner on another machine, a stopped origin entry, a cancel already under way and a slug with nothing left.
// It then takes the cancel marker, a record whose writer's liveness is a lock it holds, which every driving verb refuses on and a second cancel reads to finish an interrupted one.
// The order is: end the registered processes, take batten's locks, stop the pair's strands and the tackler, remove the pair and its task branches with --remove, remove the records, leave the claim draft, lift the slug's own pauses, push and notify, and remove the marker last.
// Without --remove the pair is kept and the cancel names "lyx batten cancel <slug> --remove" as the step before the slug runs again.
// An unreachable origin ends only the processes and strands, leaves the marker and posts a notice naming the cancel to run once origin answers.
// The cancel of a beam project first runs the hub's origin reconcile and refuses while the beam has members, naming each to land or cancel first, or, once the beam's pawl run holds its entry, to cancel and move off the beam; with origin unreachable it refuses naming the reconcile's way forward.
// A beam with no members is ended like a task, its dobby with its run, and its orch and watcher are then stopped, a failure there only a warning on the report; `lyx beam create <slug>` starts the run again.
//
// "lyx batten overview" (overview.go) is the hidden pane verb `lyx beam start` runs in the runs strand of the prime and of each beam, whose window reed names `beam: prime` or `beam: <slug>`.
// It draws the window's dobby section, then a blank line and its orch section; each run shows once, in the dobby section.
// The dobby section is the lines of the dobby of the worktree the pane runs in (overviewdobby.go), rendered by `DobbyLines` from the `dobbyengine.View` a `DobbyViewReader` reads:
// a header saying whether the dobby is alive, with its `max_runs`, or not running with `lyx beam start` as the way forward;
// one line per run it drives that is not done, in slug order, with the run's cells `DobbyRunCells` reads, then the pass cell: a pass in flight while the driver lock is held, else the minutes since the last pass exited, then a full pass owed, an owner elsewhere and a failing count from the dobby's state record;
// and one line per released task it has not started, `ready to start` or `waits on` each of open dependencies, another machine's ownership, a pause, a refused start with its text and the minutes to its retry, and `max_runs`.
// A dobby view that does not read shows `dobby <label>: unreadable`.
// A run's cells are batten's state, the stage and the time in that stage.
// A run with a seed and no status shows as `starting`; the stage is the task worktree's current producer while batten is in Run-Shed and that status is found, and batten's own producer otherwise.
// The stage cell carries the stage's entry number from the recipe, `7. Land` for a batten stage and `6.5 Plan-Write` for a child stage under Run-Shed's number 6; a stage whose number cannot be read shows its name alone.
// A running, batten-paused or starting run a live pause covers shows `paused <minutes>` in its state cell, the minutes since that pause began.
// A status file unreadable on two polls in a row shows `status unreadable` in place of the cells; one failed poll keeps the cells of the previous status.
// The orch section holds one line per orch the window shows: the prime's window shows every orch of the hub, and a beam's its own orch and the orch of each beam whose parent chain, read through `hubgeom.ParentOf`, reaches it.
// An orch line carries the session's latest context tokens against the soft and hard thresholds, its cycle phase and cycle count, with `watcher down` appended once the watcher has exited; no state file draws no orch line, and a state file unreadable on two polls in a row shows `orch  state unreadable`.
// The pane polls each second and backs off while nothing changes, which holds because every time renders in whole minutes.
// It only reads the orch state file, the orch config (loaded once at start), the run records, the status files, the board, beam.yaml and the dobby's state record, without the records' locks.
// It learns whether the dobby and each driver is live by taking its lock for an instant and releasing it; the acquirers wait `lock.ProbeGrace`, so a poll never refuses one and never slows a run.
//
// dobbyreads.go holds the reads of what a worktree's dobby drives, exported for the dobby's own wiring and for the overview: `DobbyRuns`, `DobbyLandings` and `DobbyTasks` over a `DobbyParents` cache, and `DobbyViewReader`, which builds the dobby's view from outside its process.
//
// "lyx batten run <slug>" is one pass: it runs the run to its next halt or yield and exits, started by its parent worktree's dobby or by hand.
// After the pass it makes sure the parent worktree's dobby is live.
// `StartFirstPass` is the in-process start of a first pass for the verbs that take a run over or start a beam project's:
// it runs the status check, the pause refusal and the owner pre-flight of a plain run, refuses a done slug, a covered one or one another machine owns before starting anything, then starts the pass detached.
// Its `FirstPassOptions.Anchor` is recorded as the seed's `anchor` param on the first pass only, so it rides in the run record and a resume keeps it.
//
// A beam project runs as a task does, with the pawl recipe's own rows past Worktree-Create.
// The start guard lets a beam project start or resume: a fresh start with no marked pair here gets a task's dirty-source and left-parts checks, the dirty-source check only when it forks from the prime;
// a marked beam pair, a legacy beam's or a resumed run's, passes them, and a beam project skips the draft check, since a beam's cancel leaves its entry draft and `lyx beam create` starts its run again.
//
// The Worktree-Create row opens by claiming the slug's board entry with "running · Worktree-Create", before the pair exists, so the board's run lock starts at the claim.
// The claim is one Bolt commit that also records this machine as the entry's owner, and an absent or done entry is skipped.
// A failed owner write or push refuses the row, and so does an entry another machine owns, which the claim never takes.
// The claim stays when the row then fails, and the stopped run resumes with the same verb.
// No other row writes a board status; loom's transitions are the only later writer.
// A pair is forked from the entry's parent beam, or from the prime outside a beam; a beam project's pair is forked unmarked at the anchor the seed records, since the pawl run's Beam-Up marks it.
// A member's pair is judged against its beam's hold again once it is made, and when the row finds it already complete: a beam whose run holds its entry reads as held, so the row removes the member's pair and refuses, the member keeping its claim.
//
// A machine starts, resumes and claims only what it owns.
// A start (no status file, or one still at Worktree-Create whose pair this machine does not hold) pulls the board, which needs origin, claims the hub's prime when none is recorded, and refuses a task another machine owns.
// A resume refuses a task the origin reconcile or, with origin unreachable, the local board shows owned elsewhere; with origin answering it records this machine as the owner of a run-held entry with none.
// Every such refusal names the owner and "lyx adopt <slug>" as the way forward.
// "lyx batten status" reports the slug's effective owner as owner_machine and whether this machine holds it as owned_here.
//
// The Inner-Run row's state-changed notice is held while the child's arbiter is live:
// the row reads the child's in-flight arbiter marker against the pair's reed directory.
// Its strand revive ends the pair's dead arbiter strands through shuttle's stop verb before reed's resume, so an arbiter strand is never relaunched and its attempt reads as hand-on;
// a live arbiter strand is left to finish.
package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// battenCLI carries the fields wire (wire.go) populates; every batten verb hangs off this
// receiver.
type battenCLI struct {
	// location is the resolved *lyxcwd.Location for the hub's prime worktree. wire reads it to
	// anchor every path constructor and every seam.
	location *lyxcwd.Location
	// env is the assembled shedrecipe.Env the run verb passes to battenrecipe.New.
	env shedrecipe.Env
	// shedPaths carries the five told values shedengine.Shed itself reads, which the run verb
	// passes alongside env, and which the status verb reads directly.
	shedPaths shedbuild.ShedPaths
	// slug is the task slug read from the command's own arguments.
	slug string
	// caller is the caller label armAt classifies from the strand variable and specFor copies onto the Spec;
	// empty on a hand-populated receiver, which records none.
	caller string
	// routing is the recipe's producer-graph projection armAt loads (loadRouting) and specFor copies
	// onto the Spec; zero on a hand-populated receiver, which reports no progress.
	routing shedengine.Routing
	// untilStop, when non-nil, replaces the shared --until-stop loop as what the run verb's driver steps the run through, so the run verb is testable without a detached loop.
	untilStop func(ctx context.Context, attempt string) ([]byte, error)
	// spec is the shedverbs.Spec the pre-run fills in place (arm.go) and the shedverbs verbs read
	// at run time. It is always non-nil after construction, so Command() can hand the same
	// pointer to shedverbs.Verbs before the pre-run has ever run.
	spec *shedverbs.Spec
	// driverFlag carries "run"'s and "step"'s own --driver value: a closure cannot carry it, since
	// cobra parses flags after Command() has already built and returned the whole tree, so arm.go's
	// armSeed reads it back off the receiver instead. Empty means "unset"; battenDriver (arm.go)
	// resolves that to shedrun.DriverGo.
	driverFlag string
	// childDriverFlag is driverFlag's sibling for --child-driver, the value armSeed writes into the
	// auto-seeded seed's params.child_driver, and only when childDriverFlagSet. Empty means "unset":
	// the child then takes its recipe's driver_kind, then shed.yaml's.
	childDriverFlag string
	// driverFlagSet records whether the operator typed --driver, rather than cobra filling the
	// field with its StringVar default.
	// The value alone cannot carry that distinction, since the default is itself a legal driver
	// name, and refuseAdoptedSeed (arm.go) needs it to tell an operator's intent from a default.
	driverFlagSet bool
	// childDriverFlagSet is driverFlagSet's sibling for --child-driver; armSeed records child_driver only when it is set.
	childDriverFlagSet bool
	// parentOwner names the machine, other than this one, that owns the worktree parent on the board's owners, where the empty parent is the prime; it is empty when no other machine owns it.
	// Nil selects the board's owners through the owner seams.
	parentOwner func(parent string) (string, error)
	// landingHeld reports whether the landing hold of the worktree parent stands, the prime never being held.
	// Nil reads the local board's entry of parent through boardengine.RunHolds.
	landingHeld func(parent string) (bool, error)
	// passSpawn starts `<lyx> <args>` detached in its own process group with dir as its working directory, and returns the started process's pid.
	// Nil selects spawnDetachedPass.
	passSpawn func(dir string, args []string) (int, error)
	// dobbyLive reports whether the dobby described by the geometry is running; nil selects dobbyengine.Live.
	dobbyLive func(dobbyengine.Geometry) (bool, error)
	// dobbySpawn starts the dobby described by the geometry as a detached process; nil selects dobbyengine.SpawnDetached.
	dobbySpawn func(dobbyengine.Geometry) error
	// reconcile runs the run-scope origin reconcile of a slug the pre-run resumes; Command wires it to reconcileRun, and nil reads as no reconcile, which only a hand-populated receiver has.
	reconcile func(slug string) (fabricengine.OriginReport, error)
	// continuePass carries "run"'s hidden --continue-pass value: the pass is one dobby schedules into a run it already drives, so the pre-run skips the resume's origin reconcile, the Open-Window re-run and the pause refusal.
	continuePass bool
	// noReopen carries "run"'s hidden --no-reopen value, which skips only the Open-Window re-run.
	noReopen bool
	// originReached records that this pass's resume reconcile reached origin, which lets a pass that exits 0 remove the origin-owed marker.
	originReached bool
	// owner carries the seams of the owner checks; armAt fills it with hubOwnerSeams when it is zero, and a receiver that calls no arm leaves it zero, which reads as no owner check.
	owner ownerSeams
	// guard carries the pre-run's refusals of a cancel under way, a dirty source and a left pair; armAt fills it with hubStartGuard when its Marker is nil, and a hand-populated receiver leaves it zero, which checks nothing.
	guard startGuard
	// beam carries the reads Worktree-Create makes for a beam project's run; wire fills it with hubLandingSeams, and a receiver that calls no wire leaves it zero, which reads every entry as a task.
	beam landingSeams
	// beamHold reads the board entry of a beam that a member's pair was just made under: its status, and whether its run holds the entry, which keeps the member from joining.
	// Nil reads the local board through boardengine.RunHolds.
	beamHold func(beam string) (status string, held bool, err error)
	// firstPassAnchor is the anchor StartFirstPass records as the seed's anchor param when armSeed writes the seed; empty records none.
	firstPassAnchor string
	// preDrive re-reads the cancel marker and the seed at the seeding of an absent status and under the shed's run lock; armAt sets it to preDriveCheck, and nil checks nothing, which only a hand-populated receiver has.
	preDrive func() error
	// clientAttached reports whether any client is attached to the task worktree's reed session; wire fills it, and nil reads as no reopening, which only a hand-populated receiver has.
	clientAttached func() (bool, error)
}

// battenVerbTexts carries batten's four shedverbs-driven verbs' Use/Short/Long text.
// run's and status's are lifted verbatim from their original hand-written constructors (run.go,
// status.go, since deleted) before shedverbs.Verbs took over their bodies -- including every
// Example: block and every embedded newline, byte-for-byte -- plus new text for pause and step in
// the same shape.
//
// Every Use string reads "[<run-id>]", not "<slug>": card 24's self-address refusal means an
// omitted positional is a legal cobra parse that reaches a named refusal, not an arity error, so
// the run-id is documented as optional here even though "self" itself always refuses for batten.
var battenVerbTexts = shedverbs.VerbTexts{
	Run: shedverbs.VerbText{
		Use:      "run [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "run one pass of a task worktree's whole lifecycle run, to its next halt or yield",
		Long: `run drives the pair's create/run/teardown lifecycle one pass, to its next
halt or yield: done, blocked, paused, or a yield while the child's own run goes
on. A run is one pass after another, started by its parent worktree's dobby or
by hand; after its pass, run makes sure that dobby is live. Invoked against a
slug with no persisted status, it starts a fresh run. Invoked against one
already in progress, it resumes
silently from the persisted current producer -- there is no re-seed flag,
because every operator-fixable refusal this task raises lands as blocked,
which is the everyday resume path. A slug already done refuses on the
envelope, naming "lyx batten cleanup --apply" and the leftover branches to
delete to run it again.

The Go driver meets an unexpected stop with its rule table and a cold tackler
agent before the orch sees it; a second run for the slug is refused while the
first holds the driver lock.

The run-id is required in practice: an omitted run-id refuses by name
rather than defaulting to "self".

The Open-Window, Seed-IDE and Open-IDE rows are Command rows: each runs its
batten.yaml command (open_window_command, seed_ide_command, open_ide_command)
only when its entry gate (the open_window, seed_ide or open_ide setting) is on.
Open-Window's default command opens one desktop terminal window attached to the
task worktree's reed session; Seed-IDE's writes the worktree's VS Code config,
and Open-IDE's opens VS Code. A run resumed past that row opens a
window again when no client is attached to the task worktree's reed session and
open_window is on, through the same command.

Example:
  lyx batten run some-slug`,
	},
	Status: shedverbs.VerbText{
		Use:      "status [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "report a task worktree's persisted lifecycle status",
		Long: `status reports a slug's persisted lifecycle status: the current producer, the
state, the error field, "pause_reason" (why a paused run is paused), the activity, and the history, plus the resolved
status path so an operator can find the file. A slug with no seed at all
refuses, listing the seeded run-ids; a seeded slug whose status file is
absent -- hand-seeded, or never stepped yet -- is reported as a determined
answer on the success envelope, not as an error. While a pause covers the
slug, the envelope also carries "paused_since" (when the pause began, RFC
3339) and "paused_by" (the pause's scope); an uncovered slug carries neither.

With --watch, it performs the same read once as a pre-flight, then tails
the file and never exits, printing a line only when the composed activity
actually changes rather than once per poll.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lyx batten status some-slug
  lyx batten status some-slug --watch`,
	},
	Pause: shedverbs.VerbText{
		Use:      "pause [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "request a pause at a task worktree's next lifecycle producer boundary",
		Long: `pause sets a request the running lifecycle consumes at its next producer
boundary. It does not kill anything -- the machine itself clears the flag
in the persist that records the paused state.

--by <label> records a free-text caller label, such as operator or the orch's
strand name, beside the request. The label is recorded in the paused run's
pause_reason; shed is reserved for lyx's own automatic steps and refused.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lyx batten pause some-slug`,
	},
	Step: shedverbs.VerbText{
		Use:      "step [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "drive a task worktree's lifecycle one producer forward",
		Long: `step drives the pair's create/run/teardown lifecycle exactly one producer
forward from its persisted current producer, seeding a fresh run first when
none is persisted yet -- the single-producer primitive an external
supervisor drives one call at a time.
With --until-stop, step runs the step loop to the run's next stop, the entry
"lyx batten run" drives.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lyx batten step some-slug`,
	},
	Goto: shedverbs.VerbText{
		Use:      "goto [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "move a halted lifecycle onto a named row, paused, keeping its spent budget",
		Long: `goto moves a halted lifecycle onto the row named by --to and leaves it
paused. It keeps the segment's spent bounce budget, and resets only what the
target row needs to run: the error, the transient mark, a pending resume label
and the pause conditions. It records a "goto" history entry and itself as the
run's pause reason. A bounce-budget-exhausted halt is lifted only by
"lyx batten run", never by goto. It refuses while a driver holds the run lock and on a done run,
and never re-opens a finished run. goto moves a halted run only back, to a row at or before its current row (strictly before when the run is awaiting), and refuses a running run.
Worktree-Teardown is reachable only once the child run is done.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lyx batten goto some-slug --to Run-Shed`,
	},
}

// Command returns the cobra command tree for the batten module.
func Command() *cobra.Command {
	c := &battenCLI{spec: &shedverbs.Spec{}}
	c.reconcile = func(slug string) (fabricengine.OriginReport, error) { return reconcileRun(c.location, slug, false) }

	parent := &cobra.Command{
		Use:   "batten",
		Short: "drive one task worktree's whole lifecycle as a single Shed run",
		Long: `batten drives one task worktree's whole lifecycle -- create, seed the task
worktree's own run, run it to a terminal state, and tear down -- as a single
Shed run over a per-slug status.json. "run" starts or resumes that run for a
slug; "step" drives it exactly one producer forward; "status" reports its
current state; "pause" requests a pause at the run's next producer boundary;
"goto" moves a halted run onto a named row, paused, keeping its spent budget;
"lock list" and "lock release" show and free the hub's parent locks.

"cleanup" takes no slug and, as a dry run unless given --apply, removes the
records and scratch of every done run, and the scratch of aged orphans.

"cancel" ends one slug's run for good, whatever its state: it ends the run's
processes and strands, leaves its board claim draft and removes its records, and with
--remove also its pair and task branches.

Every verb runs from the hub's prime worktree and from any beam: they refuse when
invoked from a task worktree that is not a beam, from the pair's fabric sibling, or
from _board.

Example:
  lyx batten run some-slug
  lyx batten step some-slug
  lyx batten status some-slug
  lyx batten pause some-slug
  lyx batten goto some-slug --to Run-Shed
  lyx batten lock list
  lyx batten cleanup --apply
  lyx batten cancel some-slug --remove`,
		// RunE is set so that bare "lyx batten" lists subcommands and "lyx batten bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(battenVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb, gotoVerb := verbs[0], verbs[1], verbs[2], verbs[3], verbs[4]
	// MaximumNArgs(1), not ExactArgs(1): all three surfaces -- "lyx shed", "lyx batten", "lyx loom"
	// -- land on this one arity contract. "lyx batten run" with no argument stops being an arity
	// error and becomes a "self" address, which arm.go's refuseSelfAddress rejects by name with a
	// better message than cobra's own count error.
	runVerb.Args = cobra.MaximumNArgs(1)
	stepVerb.Args = cobra.MaximumNArgs(1)
	statusVerb.Args = cobra.MaximumNArgs(1)
	pauseVerb.Args = cobra.MaximumNArgs(1)
	gotoVerb.Args = cobra.MaximumNArgs(1)

	// --driver and --child-driver answer different questions now: --driver names the process the
	// operator typed, and batten has no bootstrap verb, so armSeed's refuseBattenOwnDriverLLM still
	// refuses "llm" there. --child-driver names the driver the task worktree's own bootstrap will
	// honour, which is a fact about that worktree's recipe, not batten's, so it accepts both "go" and
	// "llm" -- the child's own recipe capability is checked when that child's seed is written, not
	// here.
	runVerb.Flags().StringVar(&c.driverFlag, "driver", shedrun.DriverGo, "the run's own driver (batten has no bootstrap verb, so \"llm\" is refused)")
	runVerb.Flags().StringVar(&c.childDriverFlag, "child-driver", "", "the driver the task worktree's own inner run uses; unset, the child recipe's driver_kind, then shed.yaml's")
	runVerb.Flags().BoolVar(&c.continuePass, "continue-pass", false, "run the continue pass dobby schedules into a run it already drives: skip the origin reconcile, the Open-Window re-run and the pause refusal, and refuse a run that is not one dobby may continue")
	runVerb.Flags().BoolVar(&c.noReopen, "no-reopen", false, "skip the Open-Window re-run of a resumed run")
	_ = runVerb.Flags().MarkHidden("continue-pass")
	_ = runVerb.Flags().MarkHidden("no-reopen")
	runVerb.RunE = func(cmd *cobra.Command, args []string) error { return c.driveRun(cmd) }
	stepVerb.Flags().StringVar(&c.driverFlag, "driver", shedrun.DriverGo, "the run's own driver (batten has no bootstrap verb, so \"llm\" is refused)")
	stepVerb.Flags().StringVar(&c.childDriverFlag, "child-driver", "", "the driver the task worktree's own inner run uses; unset, the child recipe's driver_kind, then shed.yaml's")

	parent.AddCommand(runVerb, stepVerb, statusVerb, pauseVerb, gotoVerb, lockCommand(), c.cleanupCmd(), c.cancelCmd(), c.overviewCmd())

	return parent
}

// resolvePersistentPreRun resolves cwd via lyxcwd.CwdFrom, resolves it into a *lyxcwd.Location via
// lyxcwd.Resolve, resolves the prime name via fabricengine.PrimeName, and applies the non-prime
// refusal (refusal.go) before resolving anything further. It then reads the slug from the command's
// own arguments and calls wire.
//
// Skips resolution entirely when the batten group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present, and for "cleanup", "cancel" and "overview", which arm no shed spec and resolve their own location.
// The lock group is skipped too: its verbs address no slug, and resolve the prime in lockStore.
func (c *battenCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "batten" || cmd.Name() == "cleanup" || cmd.Name() == "cancel" || cmd.Name() == "overview" || inLockGroup(cmd) {
		return nil
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	// Recorded here because arm never sees the *cobra.Command.
	// Changed reports false for a flag the command does not declare, so status and pause are
	// unaffected.
	c.driverFlagSet = cmd.Flags().Changed("driver")
	c.childDriverFlagSet = cmd.Flags().Changed("child-driver")

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	armed, err := c.arm(cwd, cmd.Name(), args)
	if err != nil {
		// arm's own lyxcwd.Resolve error is already self-describing (it IS the "not a git
		// repository" sentinel), and its non-prime refusal already names both worktrees; both
		// pass through bare rather than doubling text on top of them.
		output.Err(out, err.Error())
		code := 1
		if cmd.Name() == "run" {
			code = passExitCode(err)
		}
		clihelp.Abort(ctx, code)
		return nil
	}
	*c.spec = armed
	return nil
}

// startFirstPass is StartFirstPass's body: it runs the pre-checks of a plain run, starts `lyx batten run <slug>` detached with the prime's anchor as its working directory, and makes sure the parent worktree's dobby is live.
// A slug whose status is done, undecodable or in an unrecognized state is refused before anything starts, with the same refusal a plain run gives, and so is a slug a pause covers or another machine owns, naming the owner and "lyx adopt <slug>";
// seeding a fresh status stays inside the started pass.
// The envelope names the started pass's pid and the parent worktree whose dobby drives the run, and never a started batten: a second batten for the slug is refused by the run's own lock inside the pass.
// A parent dobby that cannot be started is a warning on the envelope, not a refusal, since the pass is started either way.
func (c *battenCLI) startFirstPass(cmd *cobra.Command) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if err := c.refuseCovered(); err != nil {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}
	if _, err := c.checkRunStatus(); err != nil {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}
	if err := c.preflightRunOwner(); err != nil {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}

	passArgs := []string{"batten", "run", c.slug}
	if c.driverFlagSet {
		passArgs = append(passArgs, "--driver", c.driverFlag)
	}
	if c.childDriverFlagSet {
		passArgs = append(passArgs, "--child-driver", c.childDriverFlag)
	}
	spawnPass := c.passSpawn
	if spawnPass == nil {
		spawnPass = spawnDetachedPass
	}
	pid, err := spawnPass(c.location.AnchorPath(), passArgs)
	if err != nil {
		clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("battencli: could not start the first pass of %q: %v; way forward: run `lyx batten run %s` in a terminal instead", c.slug, err, c.slug)))
		return nil
	}

	fields := map[string]any{"pid": pid}
	if parent, err := parentWorktree(c.location, c.slug); err == nil {
		fields["parent_worktree"] = parent
	}
	if err := c.ensureDobby(); err != nil {
		logger.Warn("battencli: the first pass is started but its parent's dobby is not", "slug", c.slug, "error", err.Error())
		fields["warning"] = err.Error()
	}
	clihelp.SetExit(ctx, output.Ok(out, fields))
	return nil
}

// spawnDetachedPass starts `<lyx> args...` as a detached process in its own process group with dir as its working directory, and returns its pid.
// The child's output is discarded, and the child is started but never waited for.
// It returns an error, starting nothing, in a test binary, where forking the running binary would re-run the test suite.
func spawnDetachedPass(dir string, args []string) (int, error) {
	if testing.Testing() {
		return 0, errors.New("battencli: pass not spawned under test")
	}
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locate the lyx executable: %w", err)
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("battencli: spawn pass failed", "exe", executable, "args", args, "dir", dir, "error", err)
		return 0, err
	}
	logger.Info("battencli: spawned pass", "exe", executable, "args", args, "dir", dir, "pid", cmd.Process.Pid)
	return cmd.Process.Pid, nil
}

// parentWorktree returns the name of the worktree whose dobby drives slug's run: the beam slug's pair was forked from, or the prime's name for a run with none.
func parentWorktree(prime *lyxcwd.Location, slug string) (string, error) {
	parent, err := hubgeom.ParentOf(prime, slug)
	if err != nil {
		return "", err
	}
	if parent == "" {
		return prime.WorktreeName, nil
	}
	return parent, nil
}

// ensureParentDobby makes sure the dobby of slug's parent worktree is running, so a run started by hand or in a first pass is driven on.
// ownerElsewhere names the other machine that owns the parent worktree on the board's owners, or is empty;
// when another machine owns it, nothing is spawned and the error names `lyx adopt <parent>`, or `lyx adopt root` for the prime, since that dobby would exit at once on its owner read and leave the run undriven.
// landingHeld reports whether the parent beam's landing hold stands;
// when it does, nothing is spawned and the error names `lyx batten status <parent>`.
// Otherwise live reads whether the dobby runs and spawn starts it when it does not;
// a failed geometry, read or spawn is returned naming `lyx beam start <parent>`, or `lyx beam start root` for the prime.
func ensureParentDobby(prime *lyxcwd.Location, slug string, ownerElsewhere func(parent string) (string, error), landingHeld func(parent string) (bool, error), live func(dobbyengine.Geometry) (bool, error), spawn func(dobbyengine.Geometry) error) error {
	parent, err := parentWorktree(prime, slug)
	if err != nil {
		return fmt.Errorf("battencli: resolve the parent worktree of %q: %w; way forward: fix the cause, then run `lyx batten run %s` again", slug, err, slug)
	}
	startWay := "run `lyx beam start " + parent + "` to start it"
	if parent == prime.WorktreeName {
		startWay = "run `lyx beam start root` to start it"
	}
	owner, err := ownerElsewhere(parent)
	if err != nil {
		return fmt.Errorf("battencli: read the owner of the worktree %q: %w; way forward: %s", parent, err, startWay)
	}
	if owner != "" {
		adopt := "lyx adopt " + parent
		if parent == prime.WorktreeName {
			adopt = "lyx adopt root"
		}
		return fmt.Errorf("battencli: machine %q owns the worktree %q whose dobby drives %q, so no dobby is started here and the run stays undriven; way forward: run `%s` from this machine's prime", owner, parent, slug, adopt)
	}
	held, err := landingHeld(parent)
	if err != nil {
		return fmt.Errorf("battencli: read whether the landing hold of %q stands: %w; way forward: %s", parent, err, startWay)
	}
	if held {
		return fmt.Errorf("battencli: the landing hold of %q stands, so no dobby is started and the run stays undriven; way forward: read the landing with `lyx batten status %s`", parent, parent)
	}
	geometry, err := hubgeom.DobbyGeometry(prime, parent)
	if err != nil {
		return fmt.Errorf("battencli: %w; way forward: %s", err, startWay)
	}
	running, err := live(geometry)
	if err != nil {
		return fmt.Errorf("battencli: read whether the dobby of %q is live: %w; way forward: %s", parent, err, startWay)
	}
	if running {
		return nil
	}
	if err := spawn(geometry); err != nil {
		return fmt.Errorf("battencli: start the dobby of %q: %w; way forward: %s", parent, err, startWay)
	}
	return nil
}

// ensureDobby is ensureParentDobby over this receiver's seams, the board's owners and landing hold and dobbyengine's liveness read and detached spawn where a seam is nil.
func (c *battenCLI) ensureDobby() error {
	ownerElsewhere := c.parentOwner
	if ownerElsewhere == nil {
		ownerElsewhere = c.otherMachineOwning
	}
	landingHeld := c.landingHeld
	if landingHeld == nil {
		landingHeld = func(parent string) (bool, error) { return hubgeom.LandingHeld(c.location, parent) }
	}
	live := c.dobbyLive
	if live == nil {
		live = dobbyengine.Live
	}
	spawn := c.dobbySpawn
	if spawn == nil {
		spawn = dobbyengine.SpawnDetached
	}
	return ensureParentDobby(c.location, c.slug, ownerElsewhere, landingHeld, live, spawn)
}

// otherMachineOwning names the machine other than this one that owns the worktree parent on the local board, the prime's name reading the prime owner.
// A receiver with no owner seams reads every worktree as owned by no other machine.
func (c *battenCLI) otherMachineOwning(parent string) (string, error) {
	if c.owner.Machine == nil {
		return "", nil
	}
	machine, err := c.thisMachine()
	if err != nil {
		return "", err
	}
	owners, err := c.owner.Read()
	if err != nil {
		return "", err
	}
	owner := owners.Prime
	if parent != c.location.WorktreeName {
		owner, err = owners.Effective(parent, c.owner.ParentOf)
		if err != nil {
			return "", err
		}
	}
	if !boardengine.OwnedElsewhere(owner, machine) {
		return "", nil
	}
	return owner, nil
}

// FirstPassOptions carries what a first pass records into the run it starts.
type FirstPassOptions struct {
	// Anchor is the directory the run's pair takes as its anchor, relative to the repository root.
	// A first pass records it as the seed's anchor param, so it rides in the run record and a resume keeps it; empty records none.
	Anchor string
}

// StartFirstPass starts `lyx batten run <slug>` as a detached first pass against the hub's prime worktree at location, and returns the envelope that start prints.
// It is the in-process start an adopting verb runs for each slug it takes over and `lyx beam create` runs for a beam's pawl run; the pass is then driven on by its parent worktree's dobby.
// A refusal, such as a done slug, a covered one or one another machine owns, returns an envelope whose ok is false and whose error names the way forward;
// the error result is reserved for a failure to arm the run or to read the envelope back.
// The run's own driver lock, inside the pass, refuses a second driver.
func StartFirstPass(location *lyxcwd.Location, slug string, opts FirstPassOptions) (map[string]any, error) {
	c := &battenCLI{spec: &shedverbs.Spec{}, firstPassAnchor: opts.Anchor}
	c.reconcile = func(slug string) (fabricengine.OriginReport, error) { return reconcileRun(c.location, slug, false) }
	armed, err := c.armAt(location, slug, true, "run")
	if err != nil {
		return nil, err
	}
	*c.spec = armed

	cmd := &cobra.Command{Use: "run", RunE: func(cmd *cobra.Command, args []string) error { return c.startFirstPass(cmd) }}
	var out bytes.Buffer
	clihelp.Execute(cmd, &out, []string{})
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		return nil, fmt.Errorf("battencli: the first pass of %q printed no envelope: %q: %w", slug, out.String(), err)
	}
	return envelope, nil
}

// RunCLI is the public seam for the batten module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as the capture writer
// for all output (including cobra's error text).
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
//
// RunCLIIn is carried rather than skipped for a concrete reason: the path-derivation tests serving
// as the Batten Bookend Invariant's mechanical proxy and the non-prime refusal test both need an
// injectable cwd, and neither is reachable through RunCLI alone. The branch exists because
// lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to ExecuteIn would panic on
// every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
