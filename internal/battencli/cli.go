// cli.go builds the cobra command tree for the batten module and the RunCLI/RunCLIIn seams that
// wire it into the standard io.Writer-based call contract.
//
// Package battencli is the module that drives one task worktree's whole lifecycle as a single
// Shed run from the hub's prime worktree. It imports internal/battenshed and
// internal/battenrecipe, neither of which imports cobra, and it is outside the Fabric Vocabulary
// Invariant's owner set, so no identifier, literal, or comment in this package -- or in either of
// those two it imports -- may name either side of the pair: write "the task worktree", "the pair",
// and "the hub's prime worktree" instead.
package battencli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedbuild"
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
	// abandonedSession is the value the Teardown.Shutdown seam records on the receiver -- carried
	// here rather than only returned from the seam, so the run verb can surface it on the envelope
	// after the Shed's own Run has returned.
	abandonedSession string
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
	// auto-seeded seed's params.child_driver.
	childDriverFlag string
	// driverFlagSet records whether the operator typed --driver, rather than cobra filling the
	// field with its StringVar default.
	// The value alone cannot carry that distinction, since the default is itself a legal driver
	// name, and refuseAdoptedSeed (arm.go) needs it to tell an operator's intent from a default.
	driverFlagSet bool
	// childDriverFlagSet is driverFlagSet's sibling for --child-driver.
	childDriverFlagSet bool
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
		Use:   "run [<run-id>]",
		Short: "start or resume a task worktree's whole lifecycle run to its next halt",
		Long: `run drives the pair's create/run/teardown lifecycle to its next halt: done,
blocked, or paused. Invoked against a slug with no persisted status, it
starts a fresh run. Invoked against one already in progress, it resumes
silently from the persisted current producer -- there is no re-seed flag,
because every operator-fixable refusal this task raises lands as blocked,
which is the everyday resume path. A slug already done refuses on the
envelope, naming the per-slug directory to delete to run it again.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lyx batten run some-slug`,
	},
	Status: shedverbs.VerbText{
		Use:   "status [<run-id>]",
		Short: "report a task worktree's persisted lifecycle status",
		Long: `status reports a slug's persisted lifecycle status: the current producer, the
state, the error field, the activity, and the history, plus the resolved
status path so an operator can find the file. A slug with no seed at all
refuses, listing the seeded run-ids; a seeded slug whose status file is
absent -- hand-seeded, or never stepped yet -- is reported as a determined
answer on the success envelope, not as an error.

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
		Use:   "pause [<run-id>]",
		Short: "request a pause at a task worktree's next lifecycle producer boundary",
		Long: `pause sets a request the running lifecycle consumes at its next producer
boundary. It does not kill anything -- the machine itself clears the flag
in the persist that records the paused state.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lyx batten pause some-slug`,
	},
	Step: shedverbs.VerbText{
		Use:   "step [<run-id>]",
		Short: "drive a task worktree's lifecycle one producer forward",
		Long: `step drives the pair's create/run/teardown lifecycle exactly one producer
forward from its persisted current producer, seeding a fresh run first when
none is persisted yet -- the single-producer primitive an external
supervisor drives one call at a time.

The run-id positional is required in practice, even though cobra accepts
its absence: prime hosts many slug-addressed batten runs, so an omitted
run-id refuses by name rather than defaulting to "self".

Example:
  lyx batten step some-slug`,
	},
}

// Command returns the cobra command tree for the batten module.
func Command() *cobra.Command {
	c := &battenCLI{spec: &shedverbs.Spec{}}

	parent := &cobra.Command{
		Use:   "batten",
		Short: "drive one task worktree's whole lifecycle as a single Shed run",
		Long: `batten drives one task worktree's whole lifecycle -- create, seed the task
worktree's own run, run it to a terminal state, and tear down -- as a single
Shed run over a per-slug status.json. "run" starts or resumes that run for a
slug; "step" drives it exactly one producer forward; "status" reports its
current state; "pause" requests a pause at the run's next producer boundary.

All four verbs run from the hub's prime worktree only: they refuse when invoked
from a task worktree, from the pair's fabric sibling, or from _board.

Example:
  lyx batten run some-slug
  lyx batten step some-slug
  lyx batten status some-slug
  lyx batten pause some-slug`,
		// RunE is set so that bare "lyx batten" lists subcommands and "lyx batten bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(battenVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb := verbs[0], verbs[1], verbs[2], verbs[3]
	// MaximumNArgs(1), not ExactArgs(1): all three surfaces -- "lyx shed", "lyx batten", "lyx loom"
	// -- land on this one arity contract. "lyx batten run" with no argument stops being an arity
	// error and becomes a "self" address, which arm.go's refuseSelfAddress rejects by name with a
	// better message than cobra's own count error.
	runVerb.Args = cobra.MaximumNArgs(1)
	stepVerb.Args = cobra.MaximumNArgs(1)
	statusVerb.Args = cobra.MaximumNArgs(1)
	pauseVerb.Args = cobra.MaximumNArgs(1)

	// --driver and --child-driver answer different questions now: --driver names the process the
	// operator typed, and batten has no bootstrap verb, so armSeed's refuseBattenOwnDriverLLM still
	// refuses "llm" there. --child-driver names the driver the task worktree's own bootstrap will
	// honour, which is a fact about that worktree's recipe, not batten's, so it accepts both "go" and
	// "llm" -- the child's own recipe capability is checked when that child's seed is written, not
	// here.
	runVerb.Flags().StringVar(&c.driverFlag, "driver", shedrun.DriverGo, "the run's own driver (batten has no bootstrap verb, so \"llm\" is refused)")
	runVerb.Flags().StringVar(&c.childDriverFlag, "child-driver", shedrun.DriverGo, "the driver the task worktree's own inner run uses")
	stepVerb.Flags().StringVar(&c.driverFlag, "driver", shedrun.DriverGo, "the run's own driver (batten has no bootstrap verb, so \"llm\" is refused)")
	stepVerb.Flags().StringVar(&c.childDriverFlag, "child-driver", shedrun.DriverGo, "the driver the task worktree's own inner run uses")

	parent.AddCommand(runVerb, stepVerb, statusVerb, pauseVerb)

	return parent
}

// resolvePersistentPreRun resolves cwd via lyxcwd.CwdFrom, resolves it into a *lyxcwd.Location via
// lyxcwd.Resolve, resolves the prime name via fabricengine.PrimeName, and applies the non-prime
// refusal (refusal.go) before resolving anything further. It then reads the slug from the command's
// own arguments and calls wire.
//
// Skips resolution entirely when the batten group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present.
func (c *battenCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "batten" {
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
		clihelp.Abort(ctx, 1)
		return nil
	}
	*c.spec = armed
	return nil
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
