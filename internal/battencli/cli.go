// cli.go builds the cobra command tree for the batten module.
// It also holds the `RunCLI` and `RunCLIIn` seams that wire the tree into the standard io.Writer call contract.
//
// The battencli package drives one task worktree's whole lifecycle as a single Shed run from the hub's prime worktree.
// It imports `internal/battenshed` and `internal/battenrecipe`, neither of which imports cobra.
// The package is outside the Fabric Vocabulary Invariant's owner set.
// So no identifier, literal, or comment in it, or in either of those two packages, may name either side of the pair.
// Write "the task worktree", "the pair", or "the hub's prime worktree" instead.
//
// "lym batten cleanup [--apply]" (cleanup.go) removes done runs' records and scratch, and aged orphan scratch.
// Without --apply it is a dry run.
// `ClearMotifRun` (cleanup.go) removes one slug's records the same way.
// It acts only if its run is done and the child-recipe record Seed-Child writes in batten's run directory names motif.
// A run or step of a task with recipe raddle has the start guard judge the start as a fresh one first (startguard.go).
// `claimStart` (startguard.go) runs once the start's owner check has passed.
// A refused start therefore leaves the run in place.
// A promoted Sketch starts over the motif run that made it, with no hand step.
// The dobby's read holds no start for that run.
//
// "lym batten cancel <slug> [--remove]" (cancel.go, cancelcmd.go) ends a run for good from the prime, in any state.
// It refuses first, with nothing ended:
// a hub pause; work that reached the parent; an owner on another machine; a stopped origin entry;
// a cancel already under way; and a slug with nothing left.
// It then takes the cancel marker: a record whose writer's liveness is a lock it holds.
// Every driving verb refuses on it, and a second cancel reads it to finish an interrupted one.
// The order is: end the registered processes; take batten's locks; stop the pair's strands and the tackler;
// remove the pair and its task branches with --remove; remove the records; leave the claim draft;
// lift the slug's own pauses; push and notify; and remove the marker last.
// Without --remove, the pair is kept.
// The cancel names "lym batten cancel <slug> --remove" as the step before the slug runs again.
// Records already removed and untracked at the records HEAD count as removed.
// No records commit is made, so "--remove" after a plain cancel succeeds.
// An unreachable origin ends only the processes and strands.
// It leaves the marker and posts a notice naming the cancel to run once origin answers.
// The cancel of a beam project first runs the hub's origin reconcile.
// It refuses while the beam has members, and names the way out for each one:
// to land or cancel first, or, once the beam's pawl run holds its entry, to cancel and move off the beam.
// With origin unreachable it refuses and names the reconcile's way forward.
// A beam with no members is ended like a task, its dobby with its run.
// Its orch and watcher are then stopped; a failure there is only a warning on the report.
// `lym beam create <slug>` starts the run again.
//
// "lym batten overview" (overview.go) is the hidden pane verb that `lym beam start` runs.
// `lym beam start` runs it in the runs strand of the prime and of each beam.
// Reed names that window `beam: prime` or `<handle>: beam: <slug>`.
// It draws the window's dobby section, then a blank line and its orch section;
// each run shows once, in the dobby section.
// The dobby section lists the dobby's lines for the worktree the pane runs in (overviewdobby.go).
// `DobbyLines` renders them from the `dobbyengine.View` that a `DobbyViewReader` reads.
// Its first line is a header that says whether the dobby is alive, with its `max_runs` and `land_notice`;
// or, when it is not running, `lym beam start` as the way forward.
// It then has one line per run it drives that is not done, in slug order.
// Each run line carries the run's cells that `DobbyRunCells` reads, then its pass cell.
// The pass cell is a pass in flight while the driver lock is held, else the minutes since the last pass exited.
// The line then adds a full pass owed, an owner elsewhere and a failing count, from the dobby's state record.
// It also has one line per released task it has not started.
// That line reads `ready to start`, with the last refused start's text when one is recorded;
// or it reads `waits on`, followed by each of these:
// open dependencies; another machine's ownership; a pause; a start batten refuses, with its text;
// a refused start, with its text and the minutes to its retry; and `max_runs`.
// A run or task whose board entry asks for a landing notice ends its line with `notify_on_land`.
// A dobby view that does not read shows `dobby <label>: unreadable`.
// A run's cells are batten's state, the stage and the time in that stage.
// A run with a seed and no status shows as `starting`.
// While batten is in Run-Shed and that status is found, the stage is the task worktree's current producer.
// Otherwise the stage is batten's own producer.
// The stage cell carries the stage's entry number from the recipe ahead of its name, as in `N. Name`.
// A child stage under Run-Shed carries a decimal number.
// A stage whose number cannot be read shows its name alone.
// A running, batten-paused or starting run that a live pause covers shows `paused <minutes>` in its state cell.
// The minutes count from when that pause began.
// A status file unreadable on two polls in a row shows `status unreadable` in place of the cells;
// one failed poll keeps the cells of the previous status.
// The orch section holds one line per orch the window shows.
// The prime's window shows every orch of the hub.
// A beam's window shows its own orch and the orch of each beam whose parent chain reaches it.
// The chain is read through `hubgeom.ParentOf`.
// An orch line carries the session's latest context tokens against the soft and hard thresholds.
// It also carries its cycle phase and cycle count, with `watcher down` appended once the watcher has exited.
// An orch with no state file draws no orch line.
// A state file unreadable on two polls in a row shows `orch  state unreadable`.
// The pane polls each second and backs off while nothing changes.
// The back-off holds because every time renders in whole minutes.
// It reads only these, without the records' locks:
// the orch state file; the orch config, loaded once at start; the run records; the status files;
// the board; beam.yaml; and the dobby's state record.
// It learns whether the dobby and each driver is live by taking its lock for an instant and releasing it.
// The acquirers wait `lock.ProbeGrace`, so a poll never refuses one and never slows a run.
//
// dobbyreads.go holds the reads of what a worktree's dobby drives.
// They are exported for the dobby's own wiring and for the overview.
// `DobbyRuns`, `DobbyLandings` and `DobbyTasks` read over a `DobbyParents` cache.
// `DobbyViewReader` builds the dobby's view from outside its process.
//
// "lym batten run <slug>" is one pass: it runs the run to its next halt or yield and exits.
// Its parent worktree's dobby starts it, or it is started by hand.
// After the pass it makes sure the parent worktree's dobby is live.
// It brings up a dobby that is down, with its orch, through `orchcli.BeamUpWorktree`, as `lym beam start` does.
// It never brings up the dobby alone.
// `StartFirstPass` is the in-process start of a first pass.
// It serves the verbs that take a run over, or start a beam project's.
// It runs the status check, the pause refusal and the owner pre-flight of a plain run.
// It refuses a done slug, a covered one, or one another machine owns before starting anything.
// Then it starts the pass detached.
// Its `FirstPassOptions.Anchor` is recorded as the seed's `anchor` param on the first pass only.
// So it rides in the run record, and a resume keeps it.
//
// A beam project runs as a task does, with the pawl recipe's own rows past Worktree-Create.
// The start guard lets a beam project start or resume.
// A fresh start with no marked pair here gets a task's dirty-source and left-parts checks.
// The dirty-source check applies only when the start forks from the prime.
// A marked beam pair, a legacy beam's or a resumed run's, passes the checks.
// A beam project skips the draft check, since a beam's cancel leaves its entry draft.
// `lym beam create` starts its run again.
// The guard reads a marked pair as the leftover of a stopped promote.
// It does so only while the board holds the slug as a task and the run's work has not reached its parent.
// A beam's own landing run, whose entry its Land pruned, resumes its Worktree-Teardown over the mark its Beam-Up wrote.
// The promote refusal never names a cancel that refuses back.
//
// The Worktree-Create row opens by claiming the slug's board entry with "running · Worktree-Create".
// The claim comes before the pair exists, so the board's run lock starts at the claim.
// The claim is one Bolt commit that also records this machine as the entry's owner.
// An absent or done entry is skipped.
// A failed owner write or push refuses the row.
// So does an entry another machine owns, which the claim never takes.
// The claim stays when the row then fails, and the stopped run resumes with the same verb.
// Each later transition at a row before Run-Shed writes "<state> · <row>" on the entry through the status seam.
// From Run-Shed on, the inner run's own transitions are the only writer.
// A pair is forked from the entry's parent beam, or from the prime outside a beam.
// A beam project's pair is forked at the anchor the seed records, unmarked, since the pawl run's Beam-Up marks it.
// A member's pair is judged against its beam's hold again once it is made.
// The row judges it again when it finds the pair already complete.
// A beam whose run holds its entry reads as held, so the row removes the member's pair and refuses.
// The member keeps its claim.
//
// A machine starts, resumes and claims only what it owns.
// A start (no status file, or one still at Worktree-Create whose pair this machine does not hold) pulls the board.
// The pull needs origin.
// It claims the hub's prime when none is recorded, and refuses a task another machine owns.
// A resume refuses a task that is owned elsewhere.
// The origin reconcile shows this, or, with origin unreachable, the local board does.
// With origin answering, a resume records this machine as the owner of a run-held entry with none.
// A resume at Worktree-Teardown whose task worktree is gone runs no origin reconcile.
// Nothing of the pair is left to judge, so it judges the owner on the local board alone.
// The teardown retries what is left of the pair, such as its remote branch deletion.
// It retries on any machine that owns the run.
// A task worktree is gone when nothing stands at its path or the path does not resolve as a worktree.
// One example is a directory that a removal left holding ignored files.
// The verbs that gate on or resolve the task worktree read it that way.
// By contrast, the overview and the beam-land member listing only look for the directory.
// Every such refusal names the owner and "lym adopt <slug>" as the way forward.
// "lym batten status" reports the slug's effective owner as `owner_machine`.
// It also reports whether this machine holds it as `owned_here`.
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
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// battenCLI carries the state of one batten command tree.
// Every batten verb hangs off this receiver.
type battenCLI struct {
	// location is the resolved *lymcwd.Location of the hub's prime worktree.
	// Every path constructor and seam is anchored on it.
	location *lymcwd.Location
	// env is the assembled shedrecipe.Env the run verb passes to battenrecipe.New.
	env shedrecipe.Env
	// shedPaths carries the values shedengine.Shed reads itself.
	// The run verb passes them alongside env, and the status verb reads them directly.
	shedPaths shedbuild.ShedPaths
	// slug is the task slug read from the command's own arguments.
	slug string
	// caller is the caller label classified from the strand variable and recorded on the Spec.
	// It is empty on a hand-populated receiver, which records none.
	caller string
	// routing is the recipe's producer-graph projection that the run loads and records on the Spec.
	// It is zero on a hand-populated receiver, which reports no progress.
	routing shedengine.Routing
	// untilStop, when non-nil, replaces the shared --until-stop loop that the run verb's driver steps the run through.
	// It makes the run verb testable without a detached loop.
	untilStop func(ctx context.Context, attempt string) ([]byte, error)
	// recordsDeps, when non-nil, replaces the fabric calls that commit the run's records.
	// It makes the run verb testable without a git pair.
	recordsDeps func() commitStatusDeps
	// spec is the shedverbs.Spec that the pre-run fills in place and that the shedverbs verbs read at run time.
	// It is always non-nil after construction.
	// So Command() can hand the same pointer to shedverbs.Verbs before the pre-run first runs.
	spec *shedverbs.Spec
	// driverFlag carries the --driver value of "run" and "step".
	// A closure cannot hold it: cobra parses flags after Command() has built and returned the whole tree,
	// so the seeding step reads the value back off the receiver instead.
	// Empty means "unset", which resolves to shedrun.DriverGo.
	driverFlag string
	// childDriverFlag carries the --child-driver value.
	// The auto-seeded seed records it as params.child_driver, and only when childDriverFlagSet is true.
	// Empty means "unset": the child then takes its recipe's driver_kind, then shed.yaml's.
	childDriverFlag string
	// driverFlagSet records whether the operator typed --driver, rather than cobra filling in the default.
	// The value alone cannot carry that distinction, since the default is itself a legal driver name.
	// An adopted seed's refusal needs the distinction to tell an operator's intent from a default.
	driverFlagSet bool
	// childDriverFlagSet records whether the operator typed --child-driver.
	// The seed records child_driver only in that case.
	childDriverFlagSet bool
	// parentOwner names the machine, other than this one, that owns the worktree parent on the board.
	// The empty parent is the prime. The result is empty when no other machine owns it.
	// Nil selects the board's owners through the owner seams.
	parentOwner func(parent string) (string, error)
	// landingHeld reports whether the landing hold of the worktree parent stands, the prime never being held.
	// Nil reads the local board's entry of parent through boardengine.RunHolds.
	landingHeld func(parent string) (bool, error)
	// passSpawn starts `<lym> <args>` detached in its own process group, with dir as its working directory.
	// It returns the started process's pid. Nil selects the default spawn.
	passSpawn func(dir string, args []string) (int, error)
	// dobbyLive reports whether the dobby described by the geometry is running; nil selects dobbyengine.Live.
	dobbyLive func(dobbyengine.Geometry) (bool, error)
	// pairStart brings up the worktree named parent, together with its dobby and its orch.
	// Nil selects orchcli.BeamUpWorktree over this receiver's prime worktree.
	pairStart func(parent string) error
	// reconcile runs the run-scope origin reconcile of a slug that the pre-run resumes.
	// Nil reads as no reconcile, which only a hand-populated receiver has.
	reconcile func(slug string) (fabricengine.OriginReport, error)
	// readopt carries the seams that re-make a live pair whose task worktree is gone.
	// A receiver that skips the arm leaves it zero, which reads as no re-adoption.
	readopt readoptSeams
	// continuePass carries the hidden --continue-pass value of "run".
	// The pass is one that dobby schedules into a run it already drives,
	// so the pre-run skips the resume's origin reconcile, the Open-Window re-run and the pause refusal.
	continuePass bool
	// noReopen carries "run"'s hidden --no-reopen value, which skips only the Open-Window re-run.
	noReopen bool
	// originReached records that this pass's resume reconcile reached origin, which lets a pass that exits 0 remove the origin-owed marker.
	originReached bool
	// owner carries the seams of the owner checks.
	// The arm fills it with the hub's owner seams when it is zero.
	// A receiver that skips the arm keeps it zero, which reads as no owner check.
	owner ownerSeams
	// guard carries the pre-run's refusals of a cancel under way, a dirty source and a left pair.
	// The arm fills it with the hub's start guard when its Marker is nil.
	// A hand-populated receiver leaves it zero, which checks nothing.
	guard startGuard
	// motifRunClearPending records that the start replaces a done motif run.
	// That run is removed once the owner check passes.
	motifRunClearPending bool
	// beam carries the reads that Worktree-Create makes for a beam project's run.
	// The hub's landing seams fill it. A hand-populated receiver leaves it zero, which reads every entry as a task.
	beam landingSeams
	// beamHold reads the board entry of the beam that a member's pair was made under:
	// its status, and whether its run holds the entry, which keeps the member from joining.
	// Nil reads the local board through boardengine.RunHolds.
	beamHold func(beam string) (status string, held bool, err error)
	// firstPassAnchor is the anchor the first pass records as the seed's anchor param; empty records none.
	firstPassAnchor string
	// preDrive re-reads the cancel marker and the seed when an absent status is seeded, under the shed's run lock.
	// Nil checks nothing, which only a hand-populated receiver has.
	preDrive func() error
	// clientAttached reports whether any client is attached to the task worktree's reed session.
	// Nil reads as no reopening, which only a hand-populated receiver has.
	clientAttached func() (bool, error)
	// machineName reads this machine's name, which is passed to the shed being built.
	// Nil selects machinename.Name, which only a hand-populated receiver leaves nil.
	machineName func() (string, error)
}

// battenVerbTexts carries the Use, Short and Long text of batten's shedverbs-driven verbs.
//
// Every Use string reads "[<run-id>]", not "<slug>".
// An omitted positional is a legal cobra parse that reaches a named refusal, not an arity error,
// so the run-id is documented as optional even though "self" always refuses for batten.
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
envelope, naming "lym batten cleanup --apply" and the leftover branches to
delete to run it again, unless it is the done motif run of a task whose recipe the board resolves to
raddle, which the start removes.

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
  lym batten run some-slug`,
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
  lym batten status some-slug
  lym batten status some-slug --watch`,
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
pause_reason; shed is reserved for lym's own automatic steps and refused.

A paused run whose work already reached its parent is not held: dobby
finishes it through Land and Worktree-Teardown. Hold such a run with
lym tender pause <slug>.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lym batten pause some-slug`,
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
"lym batten run" drives.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lym batten step some-slug`,
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
"lym batten run", never by goto. It refuses while a driver holds the run lock and on a done run,
and never re-opens a finished run. goto moves a halted run only back, to a row at or before its current row (strictly before when the run is awaiting), and refuses a running run.
Worktree-Teardown is reachable only once the child run is done.

A paused run whose work already reached its parent is not held: dobby
finishes it through Land and Worktree-Teardown. Hold such a run with
lym tender pause <slug>.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lym batten goto some-slug --to Run-Shed`,
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
from ` + fabricengine.BoltDirName + `.

Example:
  lym batten run some-slug
  lym batten step some-slug
  lym batten status some-slug
  lym batten pause some-slug
  lym batten goto some-slug --to Run-Shed
  lym batten lock list
  lym batten cleanup --apply
  lym batten cancel some-slug --remove`,
		// RunE is set so that bare "lym batten" lists its subcommands.
		// It also makes "lym batten bogus" emit a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(battenVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb, gotoVerb := verbs[0], verbs[1], verbs[2], verbs[3], verbs[4]
	// MaximumNArgs(1), not ExactArgs(1), makes "lym batten run" with no argument stop being an arity error.
	// The omitted run-id then becomes a "self" address, which the verb refuses by name.
	// That refusal reads more clearly than cobra's count error.
	runVerb.Args = cobra.MaximumNArgs(1)
	stepVerb.Args = cobra.MaximumNArgs(1)
	statusVerb.Args = cobra.MaximumNArgs(1)
	pauseVerb.Args = cobra.MaximumNArgs(1)
	gotoVerb.Args = cobra.MaximumNArgs(1)

	// --driver names the process the operator typed; batten has no bootstrap verb, so "llm" is refused there.
	// --child-driver names the driver the task worktree's own bootstrap will honour.
	// That is a fact about the worktree's recipe, not batten's, so both "go" and "llm" are accepted.
	// The child's own recipe capability is checked when that child's seed is written.
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

// resolvePersistentPreRun arms the command's run from the working directory and the command's own arguments.
// It applies the non-prime refusal in `refusal.go` before it resolves anything further.
// It skips the batten group command, whose bare listing and unknown-subcommand error need no git repository.
// It skips `cleanup`, `cancel` and `overview` too, since they arm no shed spec and resolve their own location.
// The lock group is skipped as well, since its verbs address no slug and resolve the prime themselves.
func (c *battenCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "batten" || cmd.Name() == "cleanup" || cmd.Name() == "cancel" || cmd.Name() == "overview" || inLockGroup(cmd) {
		return nil
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	// Recorded here because arm never sees the *cobra.Command.
	// Changed reports false for a flag the command does not declare, so status and pause are unaffected.
	c.driverFlagSet = cmd.Flags().Changed("driver")
	c.childDriverFlagSet = cmd.Flags().Changed("child-driver")

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	armed, err := c.arm(cwd, cmd.Name(), args)
	if err != nil {
		// arm's own lymcwd.Resolve error is already self-describing (it is `lymcwd.ErrNotAGitRepo`),
		// and its non-prime refusal already names both worktrees.
		// Both pass through bare rather than doubling text on top of them.
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

// startFirstPass runs the pre-checks of a plain run and starts `lym batten run <slug>` detached.
// The pass runs in the prime's anchor directory.
// It makes sure the parent worktree's dobby is live.
// A slug whose status is done, undecodable or in an unrecognized state is refused before anything starts.
// That refusal is the one a plain run gives.
// A slug a pause covers, or one another machine owns, is refused too, naming the owner and `lym adopt <slug>`.
// Seeding a fresh status stays inside the started pass.
// The envelope names the started pass's pid and the parent worktree whose dobby drives the run.
// It never names a started batten: a second batten for the slug is refused by the run's own lock inside the pass.
// A parent dobby that cannot be started is a warning on the envelope, not a refusal, since the pass starts either way.
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
		clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("battencli: could not start the first pass of %q: %v; way forward: run `lym batten run %s` in a terminal instead", c.slug, err, c.slug)))
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

// spawnDetachedPass starts `<lym> args...` as a detached process in its own process group.
// It sets dir as the child's working directory and returns its pid.
// The child's output is discarded, and the child is reaped in the background while this process lives.
// In a test binary it returns an error and starts nothing.
// Forking the running binary there would re-run the test suite.
func spawnDetachedPass(dir string, args []string) (int, error) {
	if testing.Testing() {
		return 0, errors.New("battencli: pass not spawned under test")
	}
	executable, err := selfexec.LymBinary()
	if err != nil {
		return 0, fmt.Errorf("locate the lym executable: %w", err)
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("battencli: spawn pass failed", "exe", executable, "args", args, "dir", dir, "error", err)
		return 0, err
	}
	logger.Info("battencli: spawned pass", "exe", executable, "args", args, "dir", dir, "pid", cmd.Process.Pid)
	proc.Reap(cmd)
	return cmd.Process.Pid, nil
}

// parentWorktree returns the name of the worktree whose dobby drives slug's run:
// the beam slug's pair was forked from, or the prime's name for a run with none.
func parentWorktree(prime *lymcwd.Location, slug string) (string, error) {
	parent, err := hubgeom.ParentOf(prime, slug)
	if err != nil {
		return "", err
	}
	if parent == "" {
		return prime.WorktreeName, nil
	}
	return parent, nil
}

// ensureParentDobby makes sure the dobby of slug's parent worktree is running,
// so a run started by hand or in a first pass is driven on.
// A dobby that is down is brought up with its orch, never alone, so its notices have a reader.
// When another machine owns the parent worktree on the board's owners, nothing is spawned.
// The error names `lym adopt <parent>`, or `lym adopt root` for the prime.
// Starting that dobby here would exit at once on its owner read and leave the run undriven.
// When the parent beam's landing hold stands, nothing is spawned.
// The error names `lym batten status <parent>`.
// Otherwise live reports whether the dobby runs.
// When it does not, start brings the parent worktree up, its dobby with its orch.
// A failed geometry, read or start is returned naming `lym beam start <parent>`.
// For the prime it names `lym beam start root`.
func ensureParentDobby(prime *lymcwd.Location, slug string, ownerElsewhere func(parent string) (string, error), landingHeld func(parent string) (bool, error), live func(dobbyengine.Geometry) (bool, error), start func(parent string) error) error {
	parent, err := parentWorktree(prime, slug)
	if err != nil {
		return fmt.Errorf("battencli: resolve the parent worktree of %q: %w; way forward: fix the cause, then run `lym batten run %s` again", slug, err, slug)
	}
	startWay := "run `lym beam start " + parent + "` to start it"
	if parent == prime.WorktreeName {
		startWay = "run `lym beam start root` to start it"
	}
	owner, err := ownerElsewhere(parent)
	if err != nil {
		return fmt.Errorf("battencli: read the owner of the worktree %q: %w; way forward: %s", parent, err, startWay)
	}
	if owner != "" {
		adopt := "lym adopt " + parent
		if parent == prime.WorktreeName {
			adopt = "lym adopt root"
		}
		return fmt.Errorf("battencli: machine %q owns the worktree %q whose dobby drives %q, so no dobby is started here and the run stays undriven; way forward: run `%s` from this machine's prime", owner, parent, slug, adopt)
	}
	held, err := landingHeld(parent)
	if err != nil {
		return fmt.Errorf("battencli: read whether the landing hold of %q stands: %w; way forward: %s", parent, err, startWay)
	}
	if held {
		return fmt.Errorf("battencli: the landing hold of %q stands, so no dobby is started and the run stays undriven; way forward: read the landing with `lym batten status %s`", parent, parent)
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
	if err := start(parent); err != nil {
		return fmt.Errorf("battencli: start the dobby and orch of %q: %w; way forward: %s", parent, err, startWay)
	}
	return nil
}

// ensureDobby makes sure the dobby of the slug's parent worktree is running.
// Its owner, landing hold, liveness and bring-up come from the receiver's seams,
// and each nil seam falls back to the real read or bring-up.
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
	start := c.pairStart
	if start == nil {
		start = func(parent string) error { return orchcli.BeamUpWorktree(c.location, parent) }
	}
	return ensureParentDobby(c.location, c.slug, ownerElsewhere, landingHeld, live, start)
}

// otherMachineOwning returns the local board's owner of the parent worktree when another machine owns it.
// It returns an empty string otherwise.
// The prime is read from the prime owner, and any other worktree from its effective owner.
// With a nil `owner.Machine` seam, every worktree reads as owned by no other machine.
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
	// Anchor is the directory the run's pair is anchored at, relative to the repository root.
	// A first pass records it as the seed's anchor param, so it rides in the run record and a resume keeps it;
	// empty records none.
	Anchor string
}

// StartFirstPass starts `lym batten run <slug>` as a detached first pass against the prime worktree at location.
// It returns the envelope the start prints, decoded into a map.
// Adopting verbs and beam creation call it in-process to start a slug's first pass.
// The pass is then driven on by its parent worktree's dobby.
// A refusal, such as a done slug, a covered one or one another machine owns, returns an envelope whose ok is false.
// The envelope's error names the way forward.
// The returned error is reserved for a failure to arm the run or to read the envelope back.
// The run's own driver lock, inside the pass, refuses a second driver.
func StartFirstPass(location *lymcwd.Location, slug string, opts FirstPassOptions) (map[string]any, error) {
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
// It sends all output, including cobra's error text, to out.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the batten module CLI as if run from cwd.
// An empty cwd means the process's own working directory.
//
// Tests of path derivation and of the non-prime refusal need an injectable cwd, which RunCLI does not offer.
// The empty-cwd branch exists because `lymcwd.WithCwd` panics on an empty directory.
// A uniform delegation to `clihelp.ExecuteIn` would panic on every RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
