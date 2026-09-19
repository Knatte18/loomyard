// cli.go builds the cobra command tree for the shed subtree and the RunCLI/RunCLIIn seams that wire
// it into the standard io.Writer-based call contract, matching internal/loomcli's and
// internal/battencli's own shapes so all three modules read identically at the call site.
//
// resolvePersistentPreRun is the whole reason this package resolves cwd at all, unlike
// internal/shedverbs' own no-resolver leaf: it must read a seed before it knows which recipe to
// arm, and a seed read is a path read that needs an anchor. Its sequence, pinned as this batch's own
// refusal precedence, is: short-circuit on the bare group ("shed") and on "seed" (card 29's command,
// exempt because it is the one site invoked when no seed yet exists); resolve cwd via
// lyxcwd.CwdFrom; resolve it into a *lyxcwd.Location via lyxcwd.Resolve, whose own
// not-a-git-repository sentinel passes through bare; resolve the run-id from the command's
// positional argument, defaulting to shedrun.SelfRunID; read the seed at that run-id via
// shedrun.ReadSeed, refusing via shedrun.MissingSeedMessage naming "lyx shed seed <run-id> --recipe
// <name>" as the remedy when none is found; look the seed's recipe up through the table's own
// lookup, whose unknown-name error already names the available recipes; apply the verb gate against
// the resolved entry's Verbs set; and call the entry's Arm (the Location-taking ArmAt shape) with
// the Location and run-id already in hand.

package shedcli

import (
	"errors"
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// shedCLI carries the one field every shed verb hangs off: spec, the shedverbs.Spec the pre-run
// fills in place (resolvePersistentPreRun) and the four shedverbs verbs read at run time.
type shedCLI struct {
	// spec is always non-nil after construction, so Command() can hand the same pointer to
	// shedverbs.Verbs before the pre-run has ever run.
	spec *shedverbs.Spec
}

// shedVerbTexts carries this subtree's own generic Use/Short/Long text for the four shedverbs
// verbs: it is not loom's and not batten's, since one "lyx shed status --help" must serve every
// recipe rather than describing only one of them.
var shedVerbTexts = shedverbs.VerbTexts{
	Run: shedverbs.VerbText{
		Use:   "run [<run-id>]",
		Short: "run the addressed run's phase machine in the foreground",
		Long: `run arms the recipe named by the addressed run's own seed and runs its phase
machine in the foreground, exactly as that recipe's own "run" verb does. The
run-id positional defaults to "self" when omitted.

Example:
  lyx shed run
  lyx shed run some-slug`,
	},
	Step: shedverbs.VerbText{
		Use:   "step [<run-id>]",
		Short: "bootstrap idempotently and drive exactly one producer of the addressed run",
		Long: `step arms the recipe named by the addressed run's own seed and drives exactly
one producer, reporting a JSON envelope, exactly as that recipe's own "step"
verb does. Not every recipe supports step: a recipe whose table entry
excludes it is refused before arming. The run-id positional defaults to
"self" when omitted.

Example:
  lyx shed step
  lyx shed step some-slug`,
	},
	Status: shedverbs.VerbText{
		Use:   "status [<run-id>]",
		Short: "report the addressed run's current phase, once or as a live-tailed watch",
		Long: `status arms the recipe named by the addressed run's own seed and reports its
current phase, exactly as that recipe's own "status" verb does. The run-id
positional defaults to "self" when omitted.

Example:
  lyx shed status
  lyx shed status some-slug`,
	},
	Pause: shedverbs.VerbText{
		Use:   "pause [<run-id>]",
		Short: "request a pause at the addressed run's next producer boundary",
		Long: `pause arms the recipe named by the addressed run's own seed and requests a
pause at its next producer boundary, exactly as that recipe's own "pause"
verb does. The run-id positional defaults to "self" when omitted.

Example:
  lyx shed pause
  lyx shed pause some-slug`,
	},
}

// Command returns the cobra command tree for the shed subtree.
func Command() *cobra.Command {
	c := &shedCLI{spec: &shedverbs.Spec{}}

	parent := &cobra.Command{
		Use:   "shed",
		Short: "drive an addressed run's generic run/step/status/pause verbs",
		Long: `shed drives an addressed run's phase machine through the same four generic
verbs internal/shedverbs builds for every module: "run", "step", "status",
and "pause". Each verb takes an optional run-id positional (defaulting to
"self") naming the run to address; the run's own seed.json, written by "lyx
shed seed", names the recipe that arms the invocation. Not every recipe
supports every verb, and an unsupported verb/recipe pair is refused before
arming.

Example:
  lyx shed run
  lyx shed run some-slug
  lyx shed status
  lyx shed pause`,
		// RunE is set so that bare "lyx shed" lists subcommands and "lyx shed bogus" emits a JSON
		// error envelope instead of falling through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	verbs := shedverbs.Verbs(shedVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb := verbs[0], verbs[1], verbs[2], verbs[3]

	// MaximumNArgs(1), not a per-recipe contract read off the table: an addressed run is named by
	// run-id, not by a per-recipe argument shape, so there is nothing left for a per-recipe
	// contract to vary now that --recipe is gone.
	runVerb.Args = cobra.MaximumNArgs(1)
	stepVerb.Args = cobra.MaximumNArgs(1)
	statusVerb.Args = cobra.MaximumNArgs(1)
	pauseVerb.Args = cobra.MaximumNArgs(1)

	parent.AddCommand(runVerb, stepVerb, statusVerb, pauseVerb, newSeedCommand())

	return parent
}

// resolvePersistentPreRun is this subtree's own equivalent of each module's existing group guard,
// extended to also exempt "lyx shed seed" (card 29's command): it short-circuits when the bare
// "shed" group command or "seed" itself is invoked, so neither needs a git repository or an
// already-existing seed. Otherwise it resolves cwd, resolves it into a *lyxcwd.Location -- the one
// resolution stage armFromSeed below performs no part of, and therefore the one stage this
// package's own untagged tests never drive directly -- and delegates the rest of the pinned
// sequence to armFromSeed. See this file's own header comment for the full pinned sequence.
func (c *shedCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "shed" || cmd.Name() == "seed" {
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

	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		// lyxcwd.Resolve's error is already self-describing (it IS the "not a git repository"
		// sentinel); pass it through bare rather than doubling that same text on top of it.
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	armed, err := armFromSeed(location, cmd.Name(), args)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	*c.spec = armed
	return nil
}

// armFromSeed resolves the addressed run-id from args -- args[0] when present,
// shedrun.SelfRunID otherwise -- reads that run's seed, looks the seed's recipe up through the
// table's own lookup, applies the verb gate against the resolved entry's Verbs set, and calls the
// entry's Arm (the Location-taking ArmAt shape) with location and the resolved run-id. It performs
// no lyxcwd.Resolve of its own: location is already resolved, told rather than derived, by
// resolvePersistentPreRun -- which is what lets cli_test.go drive this directly against a hand-built
// *lyxcwd.Location, with no real git repository behind it, and stay Tier 1.
//
// Every error it returns is a plain error, never a fields-carrying envelope: resolvePersistentPreRun
// reports every one of them through output.Err, never output.ErrFields, so the missing-run refusal
// this function returns can never pick up a "kind" field by accident -- unlike shedverbs' own step
// body, whose PreStep errors round-trip through output.ErrFields with a "kind" key. A missing run is
// not a sixth value in that closed vocabulary, and this function's own return type is what keeps it
// that way structurally rather than by review discipline alone.
func armFromSeed(location *lyxcwd.Location, verb string, args []string) (shedverbs.Spec, error) {
	runID := shedrun.SelfRunID
	if len(args) > 0 {
		runID = args[0]
	}

	seed, found, err := shedrun.ReadSeed(location, runID)
	if err != nil {
		return shedverbs.Spec{}, err
	}
	if !found {
		existing, listErr := shedrun.List(location)
		if listErr != nil {
			return shedverbs.Spec{}, listErr
		}
		return shedverbs.Spec{}, errors.New(shedrun.MissingSeedMessage("shedcli", runID, existing, `run "lyx shed seed `+runID+` --recipe <name>" first`))
	}

	e, err := lookup(seed.Recipe)
	if err != nil {
		return shedverbs.Spec{}, err
	}

	if !verbSupported(e.Verbs, verb) {
		return shedverbs.Spec{}, errors.New(unsupportedVerbMessage(verb, seed.Recipe, e.Verbs))
	}

	armed, err := e.Arm(location, verb, runID)
	if err != nil {
		// The arming module's own error is already self-describing (either lyxcwd.Resolve's
		// "not a git repository" sentinel or a non-prime refusal naming both worktrees); pass it
		// through bare rather than doubling text on top of it, exactly as both modules' own
		// pre-runs already do.
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
// It delegates to RunCLIIn with an empty cwd, exactly as loomcli.RunCLI and battencli.RunCLI do.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute, while any other value seeds cwd into the execution context via
// clihelp.ExecuteIn, because lyxcwd.WithCwd panics on an empty directory.
//
// RunCLIIn is carried rather than skipped because this package's own parity tests need an
// injectable cwd.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
