// cli.go builds the cobra command tree for the shed subtree and the RunCLI/RunCLIIn seams that wire
// it into the standard io.Writer-based call contract, matching internal/loomcli's and
// internal/battencli's own shapes so all three modules read identically at the call site.

package shedcli

import (
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
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
		Use:   "run",
		Short: "run the named recipe's phase machine in the foreground",
		Long: `run arms the recipe named by --recipe and runs its phase machine in the
foreground, exactly as that recipe's own "run" verb does.

Example:
  lyx shed run --recipe loom`,
	},
	Step: shedverbs.VerbText{
		Use:   "step",
		Short: "bootstrap idempotently and drive exactly one producer of the named recipe",
		Long: `step arms the recipe named by --recipe and drives exactly one producer,
reporting a JSON envelope, exactly as that recipe's own "step" verb does.
Not every recipe supports step: a recipe whose table entry excludes it is
refused before arming.

Example:
  lyx shed step --recipe loom`,
	},
	Status: shedverbs.VerbText{
		Use:   "status",
		Short: "report the named recipe's current phase, once or as a live-tailed watch",
		Long: `status arms the recipe named by --recipe and reports its current phase,
exactly as that recipe's own "status" verb does.

Example:
  lyx shed status --recipe loom
  lyx shed status --recipe batten some-slug`,
	},
	Pause: shedverbs.VerbText{
		Use:   "pause",
		Short: "request a pause at the named recipe's next producer boundary",
		Long: `pause arms the recipe named by --recipe and requests a pause at its next
producer boundary, exactly as that recipe's own "pause" verb does.

Example:
  lyx shed pause --recipe loom`,
	},
}

// Command returns the cobra command tree for the shed subtree.
func Command() *cobra.Command {
	c := &shedCLI{spec: &shedverbs.Spec{}}

	var recipeFlag string

	parent := &cobra.Command{
		Use:   "shed",
		Short: "drive a named recipe's generic run/step/status/pause verbs",
		Long: `shed drives a named recipe's phase machine through the same four generic
verbs internal/shedverbs builds for every module: "run", "step", "status",
and "pause". The --recipe flag (default "loom") selects which recipe's
arming function resolves the invocation; not every recipe supports every
verb, and an unsupported verb/recipe pair is refused before arming.

Example:
  lyx shed run --recipe loom
  lyx shed run --recipe batten some-slug
  lyx shed status --recipe loom
  lyx shed pause --recipe loom`,
		// RunE is set so that bare "lyx shed" lists subcommands and "lyx shed bogus" emits a JSON
		// error envelope instead of falling through to cobra's plain-text help.
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: c.resolvePersistentPreRun,
	}

	parent.PersistentFlags().StringVar(&recipeFlag, "recipe", "loom", "the named recipe to arm and drive")

	verbs := shedverbs.Verbs(shedVerbTexts, c.spec)
	runVerb, stepVerb, statusVerb, pauseVerb := verbs[0], verbs[1], verbs[2], verbs[3]

	// Args is a closure rather than a static assignment: cobra parses flags before it validates
	// Args, so --recipe is readable at that point, but the table entry is not known when the tree
	// is built. Returning nil for an unresolvable recipe lets the unknown-recipe refusal come from
	// the pre-run's own envelope rather than from an argument-count error.
	argsFor := func() cobra.PositionalArgs {
		return func(cmd *cobra.Command, args []string) error {
			e, err := lookup(recipeFlag)
			if err != nil {
				return nil
			}
			return e.Args(cmd, args)
		}
	}
	runVerb.Args = argsFor()
	stepVerb.Args = argsFor()
	statusVerb.Args = argsFor()
	pauseVerb.Args = argsFor()

	parent.AddCommand(runVerb, stepVerb, statusVerb, pauseVerb)

	return parent
}

// resolvePersistentPreRun is this subtree's own equivalent of each module's existing group guard:
// it short-circuits when the bare "shed" group command itself is invoked, so a bare listing needs
// no git repository. Otherwise it reads cwd through lyxcwd.CwdFrom, looks the --recipe value up
// through lookup, refuses a verb the resolved recipe does not support, and then calls that entry's
// Arm -- never resolving cwd into a *lyxcwd.Location itself, since each module's Arm owns its own
// resolution.
func (c *shedCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "shed" {
		return nil
	}

	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	recipeFlag, err := cmd.Flags().GetString("recipe")
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	e, err := lookup(recipeFlag)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	if !verbSupported(e.Verbs, cmd.Name()) {
		output.Err(out, unsupportedVerbMessage(cmd.Name(), recipeFlag, e.Verbs))
		clihelp.Abort(ctx, 1)
		return nil
	}

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}

	armed, err := e.Arm(cwd, cmd.Name(), args)
	if err != nil {
		// The arming module's own error is already self-describing (either lyxcwd.Resolve's
		// "not a git repository" sentinel or a non-prime refusal naming both worktrees); pass it
		// through bare rather than doubling text on top of it, exactly as both modules' own
		// pre-runs already do.
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	*c.spec = armed
	return nil
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
