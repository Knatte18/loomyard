// cli.go builds the cobra command tree for the shed subtree.
// It also holds the RunCLI and RunCLIIn seams that run that tree against an io.Writer and an argument list.

package shedcli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// shedCLI holds the shedverbs.Spec that every shed verb reads at run time.
// The pre-run fills the Spec in place once the addressed run is armed.
type shedCLI struct {
	// spec is never nil: the verbs hold this pointer from construction, before the pre-run fills it in.
	spec *shedverbs.Spec
}

// shedVerbTexts holds the generic Use, Short and Long text of the five shedverbs verbs.
// The text describes every recipe, so one "lym shed status --help" serves all of them.
var shedVerbTexts = shedverbs.VerbTexts{
	Run: shedverbs.VerbText{
		Use:      "run [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "run the addressed run's phase machine in the foreground",
		Long: `run arms the recipe named by the addressed run's own seed and runs its phase
machine in the foreground, exactly as that recipe's own "run" verb does. The
run-id positional defaults to "self" when omitted.

Example:
  lym shed run
  lym shed run some-slug`,
	},
	Step: shedverbs.VerbText{
		Use:      "step [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "bootstrap idempotently and drive exactly one producer of the addressed run",
		Long: `step arms the recipe named by the addressed run's own seed and drives exactly
one producer, reporting a JSON envelope, exactly as that recipe's own "step"
verb does. Not every recipe supports step: a recipe whose table entry
excludes it is refused before arming. The run-id positional defaults to
"self" when omitted.

Example:
  lym shed step
  lym shed step some-slug`,
	},
	Status: shedverbs.VerbText{
		Use:      "status [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "report the addressed run's current phase, once or as a live-tailed watch",
		Long: `status arms the recipe named by the addressed run's own seed and reports its
current phase, exactly as that recipe's own "status" verb does. The run-id
positional defaults to "self" when omitted.

Example:
  lym shed status
  lym shed status some-slug`,
	},
	Pause: shedverbs.VerbText{
		Use:      "pause [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "record a pause for the addressed run, at its next producer boundary or a named one",
		Long: `pause arms the recipe named by the addressed run's own seed and records a
pause, exactly as that recipe's own "pause" verb does. The run-id positional
defaults to "self" when omitted.

--before <producer> records a stop condition: the run pauses before that
producer runs. --after <producer> pauses after that producer returns a
routed running outcome, at the row it routed to. Either replaces a recorded
one. --clear removes both conditions, and refuses beside --before or
--after.

--by <label> records a free-text caller label, such as operator, beside the
request it accompanies. The label is recorded in the paused run's
pause_reason; shed is reserved for lym's own automatic steps and refused.

A bare pause, with no flag, requests a pause at the next producer boundary
for a recipe that allows it, and refuses for a run lym loom drives, naming "lym tender
pause <slug>" to stop its agents now and "--after <current_producer>" to
halt it at its next producer boundary.

Example:
  lym shed pause some-slug --after Plan-Write
  lym shed pause --before Plan-Write
  lym shed pause --clear`,
	},
	Goto: shedverbs.VerbText{
		Use:      "goto [<run-id>]",
		Audience: clihelp.AudienceOperator,
		Short:    "move a halted run onto a named row, paused, keeping its settled reviews and spent budget",
		Long: `goto arms the recipe named by the addressed run's own seed and moves a halted
run onto the row named by --to, leaving it paused. It keeps the segment's
settled verdicts and spent bounce budget, and resets only what the target row
needs to run: the error, the transient mark, a pending resume label and the
pause conditions. It records a "goto" history entry and itself as the run's
pause reason, and is refused onto a review or fix row of a settled review
segment where the recipe tells that guard. A bounce-budget-exhausted halt is
lifted only by the recipe's resume, never by goto. It refuses while a driver
holds the run lock and on a done run, and never re-opens a finished run. The
run-id positional defaults to "self" when omitted. goto moves a halted run only back, to a row at or before its current row (strictly before when the run is awaiting), and refuses a running run.

Example:
  lym shed goto --to Plan-Write
  lym shed goto some-slug --to Plan-Write`,
	},
}

// Command returns the cobra command tree for the shed subtree.
func Command() *cobra.Command {
	c := &shedCLI{spec: &shedverbs.Spec{}}

	parent := &cobra.Command{
		Use:   "shed",
		Short: "drive an addressed run's generic run/step/status/pause/goto verbs",
		Long: `shed drives an addressed run's phase machine through the same five generic
verbs internal/shedverbs builds for every module: "run", "step", "status",
"pause" and "goto". Each verb takes an optional run-id positional (defaulting to
"self") naming the run to address; the run's own seed.json, written by "lym
shed seed", names the recipe that arms the invocation. Not every recipe
supports every verb, and an unsupported verb/recipe pair is refused before
arming.

Example:
  lym shed run
  lym shed run some-slug
  lym shed status
  lym shed pause --after Plan-Write
  lym shed goto --to Plan-Write`,
		// RunE lists the subcommands for a bare "lym shed" and emits a JSON error envelope for "lym shed bogus";
		// without it, cobra prints plain-text help instead.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(shedVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb, gotoVerb := verbs[0], verbs[1], verbs[2], verbs[3], verbs[4]

	// The one positional is the run-id, which names an addressed run the same way for every recipe,
	// so the argument count is not read off the table.
	runVerb.Args = cobra.MaximumNArgs(1)
	stepVerb.Args = cobra.MaximumNArgs(1)
	statusVerb.Args = cobra.MaximumNArgs(1)
	pauseVerb.Args = cobra.MaximumNArgs(1)
	gotoVerb.Args = cobra.MaximumNArgs(1)

	parent.AddCommand(runVerb, stepVerb, statusVerb, pauseVerb, gotoVerb, newSeedCommand())

	return parent
}

// resolvePersistentPreRun arms the addressed run before a shed verb runs.
// It returns at once for the bare "shed" group and for "seed"; neither needs a git repository or an existing seed.
// Otherwise it resolves the working directory through lymcwd.CwdFrom;
// it resolves the git location through lymcwd.Resolve, whose not-a-git-repository error passes through bare;
// it arms the run through armFromSeed, which reads the seed and looks up its recipe;
// and it stores the armed Spec in c.spec.
func (c *shedCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "shed" || cmd.Name() == "seed" {
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

	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		// lymcwd.Resolve's error is already self-describing (it IS the "not a git repository"
		// sentinel); pass it through bare rather than doubling that same text on top of it.
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	armed, err := armFromSeed(location, cmd.Name(), args)
	if err != nil {
		clihelp.Abort(ctx, reportArmFailure(out, cmd, location, args, err))
		return nil
	}
	*c.spec = armed
	return nil
}

// reportArmFailure prints the refusal armFromSeed returned and gives the exit code.
// A step carrying the --until-stop flag is a loop stop, which writes the run's status file failed;
// every other verb, and a step without the flag, reports the plain arming refusal.
func reportArmFailure(out io.Writer, cmd *cobra.Command, location *lymcwd.Location, args []string, err error) int {
	if untilStop, _ := cmd.Flags().GetBool(shedverbs.UntilStopFlag); cmd.Name() == "step" && untilStop {
		runID := addressedRunID(args)
		loopID, _ := cmd.Flags().GetString(shedverbs.LoopDetachedFlag)
		return shedverbs.ReportLoopArmError(out, shedverbs.ArmStop{
			RunID:          shedrun.ResolveRunID(location, runID),
			StatusPath:     shedrun.StatusFile(location, runID),
			RunLockPath:    shedrun.RunLock(location, runID),
			StatusLockPath: shedrun.StatusLock(location, runID),
			LoopLockPath:   shedrun.LoopLock(location, runID),
			PIDPath:        shedrun.LoopPIDFile(location, runID),
			EnvelopePath:   shedrun.LoopEnvelope(location, runID),
			LoopID:         loopID,
		}, err)
	}
	return shedverbs.ReportArmError(out, cmd.Name(), err)
}

// addressedRunID returns the run-id that args address: args[0] when present, otherwise shedrun.SelfRunID.
func addressedRunID(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return shedrun.SelfRunID
}

// armFromSeed arms the run that args address for verb and returns the armed Spec.
// It reads the run's seed and looks up its recipe;
// it refuses a verb the recipe does not support, and it calls the recipe's Arm.
// The caller resolves location, so armFromSeed performs no git-location lookup of its own.
//
// A missing seed and an unsupported verb are wrapped in shedverbs.KindlessRefusal;
// shedverbs.ReportArmError prints them with no "kind" field, even on step.
// A missing run is not a value of the closed step vocabulary, and a driver never re-seeds a run.
// A seed naming a recipe the table does not hold is refused with the re-seed way forward.
// Every other error reaches the driver on step as a bootstrap refusal.
func armFromSeed(location *lymcwd.Location, verb string, args []string) (shedverbs.Spec, error) {
	runID := addressedRunID(args)

	seed, found, err := shedrun.ReadSeed(location, runID)
	if err != nil {
		return shedverbs.Spec{}, err
	}
	if !found {
		existing, listErr := shedrun.List(location)
		if listErr != nil {
			return shedverbs.Spec{}, listErr
		}
		return shedverbs.Spec{}, shedverbs.KindlessRefusal{Err: errors.New(shedrun.MissingSeedMessage("shedcli", runID, existing, `run "lym shed seed `+runID+` --recipe <name>" first`))}
	}

	e, err := lookup(seed.Recipe)
	if err != nil {
		return shedverbs.Spec{}, fmt.Errorf("%w; way forward: remove the run's seed.json and seed the run again with \"lym shed seed <run-id> --recipe <recipe>\"", err)
	}

	if !verbSupported(e.Verbs, verb) {
		return shedverbs.Spec{}, shedverbs.KindlessRefusal{Err: errors.New(unsupportedVerbMessage(verb, seed.Recipe, e.Verbs))}
	}

	armed, err := e.Arm(location, verb, runID)
	if err != nil {
		// The arming module's error is already self-describing;
		// it is the not-a-git-repository sentinel or a non-prime refusal naming both worktrees, so it passes through bare.
		return shedverbs.Spec{}, err
	}
	return armed, nil
}

// verbSupported reports whether verb appears in supported.
func verbSupported(supported []string, verb string) bool {
	for _, v := range supported {
		if v == verb {
			return true
		}
	}
	return false
}

// unsupportedVerbMessage builds the refusal text for a verb/recipe pair the table's Verbs set
// excludes, naming the verb, the recipe, and the verbs that recipe does support.
func unsupportedVerbMessage(verb, recipe string, supported []string) string {
	return "shedcli: recipe " + recipe + " does not support verb " + verb + "; supported verbs: " + strings.Join(supported, ", ")
}

// RunCLI is the public seam for the shed subtree.
//
// It delegates to RunCLIIn with an empty cwd, which makes RunCLIIn read the process working directory.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the shed subtree with cwd as the working directory.
// An empty cwd reads the process working directory and runs through clihelp.Execute;
// any other value is seeded into the execution context by clihelp.ExecuteIn.
// lymcwd.WithCwd panics on an empty directory, so an empty cwd must not reach clihelp.ExecuteIn.
//
// The seam exists because this package's parity tests need an injectable cwd.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
