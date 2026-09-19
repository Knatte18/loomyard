// cli.go builds the cobra command tree for the lifecycle module and the RunCLI/RunCLIIn seams that
// wire it into the standard io.Writer-based call contract.
//
// Package lifecyclecli is the module that drives one task worktree's whole lifecycle as a single
// Shed run from the hub's prime worktree. It imports internal/lifecycleshed and
// internal/lifecyclerecipe, neither of which imports cobra, and it is outside the Fabric Vocabulary
// Invariant's owner set, so no identifier, literal, or comment in this package -- or in either of
// those two it imports -- may name either side of the pair: write "the task worktree", "the pair",
// and "the hub's prime worktree" instead.
package lifecyclecli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// lifecycleCLI carries the fields wire (wire.go) populates; every lifecycle verb hangs off this
// receiver.
type lifecycleCLI struct {
	// location is the resolved *lyxcwd.Location for the hub's prime worktree. wire reads it to
	// anchor every path constructor and every seam.
	location *lyxcwd.Location
	// env is the assembled shedrecipe.Env the run verb passes to lifecyclerecipe.New.
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
}

// lifecycleVerbTexts carries lifecycle's three shedverbs-driven verbs' Use/Short/Long text.
// run's and status's are lifted verbatim from their original hand-written constructors (run.go,
// status.go, since deleted) before shedverbs.Verbs took over their bodies -- including every
// Example: block and every embedded newline, byte-for-byte -- plus new text for pause in the same
// shape. step's fields are left at their zero value: lifecycle has no analogue for it, and an
// unregistered returned command with a blank Short never reaches the live tree
// cmd/lyx/drift_test.go walks.
var lifecycleVerbTexts = shedverbs.VerbTexts{
	Run: shedverbs.VerbText{
		Use:   "run <slug>",
		Short: "start or resume a task worktree's whole lifecycle run to its next halt",
		Long: `run drives the pair's create/run/teardown lifecycle to its next halt: done,
blocked, or paused. Invoked against a slug with no persisted status, it
starts a fresh run. Invoked against one already in progress, it resumes
silently from the persisted current producer -- there is no re-seed flag,
because every operator-fixable refusal this task raises lands as blocked,
which is the everyday resume path. A slug already done refuses on the
envelope, naming the per-slug directory to delete to run it again.

Example:
  lyx lifecycle run some-slug`,
	},
	Status: shedverbs.VerbText{
		Use:   "status <slug>",
		Short: "report a task worktree's persisted lifecycle status",
		Long: `status reports a slug's persisted lifecycle status: the current producer, the
state, the error field, the activity, and the history, plus the resolved
status path so an operator can find the file. A slug that has never been
run on this machine is reported as a determined answer on the success
envelope, not as an error.

With --watch, it performs the same read once as a pre-flight, then tails
the file and never exits, printing a line only when the composed activity
actually changes rather than once per poll.

Example:
  lyx lifecycle status some-slug
  lyx lifecycle status some-slug --watch`,
	},
	Pause: shedverbs.VerbText{
		Use:   "pause <slug>",
		Short: "request a pause at a task worktree's next lifecycle producer boundary",
		Long: `pause sets a request the running lifecycle consumes at its next producer
boundary. It does not kill anything -- the machine itself clears the flag
in the persist that records the paused state.

Example:
  lyx lifecycle pause some-slug`,
	},
}

// Command returns the cobra command tree for the lifecycle module.
func Command() *cobra.Command {
	c := &lifecycleCLI{spec: &shedverbs.Spec{}}

	parent := &cobra.Command{
		Use:   "lifecycle",
		Short: "drive one task worktree's whole lifecycle as a single Shed run",
		Long: `lifecycle drives one task worktree's whole lifecycle -- create, run the loom
session to a terminal state, and tear down -- as a single Shed run over a
per-slug status.json. "run" starts or resumes that run for a slug; "status"
reports its current state; "pause" requests a pause at the run's next
producer boundary.

All three verbs run from the hub's prime worktree only: they refuse when
invoked from a task worktree.

Example:
  lyx lifecycle run some-slug
  lyx lifecycle status some-slug
  lyx lifecycle pause some-slug`,
		// RunE is set so that bare "lyx lifecycle" lists subcommands and "lyx lifecycle bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(lifecycleVerbTexts, c.spec)
	runVerb, statusVerb, pauseVerb := verbs[0], verbs[2], verbs[3]
	runVerb.Args = cobra.ExactArgs(1)
	statusVerb.Args = cobra.ExactArgs(1)
	pauseVerb.Args = cobra.ExactArgs(1)

	parent.AddCommand(runVerb, statusVerb, pauseVerb)

	return parent
}

// resolvePersistentPreRun resolves cwd via lyxcwd.CwdFrom, resolves it into a *lyxcwd.Location via
// lyxcwd.Resolve, resolves the prime name via fabricengine.PrimeName, and applies the non-prime
// refusal (refusal.go) before resolving anything further. It then reads the slug from the command's
// own arguments and calls wire.
//
// Skips resolution entirely when the lifecycle group command itself is invoked (bare listing or
// unknown-subcommand error path via clihelp.GroupRunE), so neither path requires a git repository to
// be present.
func (c *lifecycleCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "lifecycle" {
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
		// repository" sentinel), and its non-prime refusal already names both worktrees; both
		// pass through bare rather than doubling text on top of them.
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	*c.spec = armed
	return nil
}

// RunCLI is the public seam for the lifecycle module CLI.
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
// as the Lifecycle Bookend Invariant's mechanical proxy and the non-prime refusal test both need an
// injectable cwd, and neither is reachable through RunCLI alone. The branch exists because
// lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to ExecuteIn would panic on
// every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
