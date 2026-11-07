// seed.go implements "lym shed seed", which writes a run's first seed.json and belongs to no recipe's Verbs set.
// It holds the testable workers behind the command's RunE:
// writeSeed, which validates and writes the seed, and parseSeedParams, which parses the --param flags.

package shedcli

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// resolveSeedDriver returns flagVal when it is set;
// otherwise it returns shedrun.DriverLLM for a recipe with a bootstrap verb and shedrun.DriverGo for one without.
// An llm driver is the intended default, but a recipe without a bootstrap verb cannot honour it.
func resolveSeedDriver(flagVal string, hasBootstrapVerb bool) string {
	if flagVal != "" {
		return flagVal
	}
	if hasBootstrapVerb {
		return shedrun.DriverLLM
	}
	return shedrun.DriverGo
}

// parseSeedParams parses raw into a map, each entry a "key=value" pair.
// It refuses an entry with no "=" or with an empty key.
// A nil raw yields a nil map, never an empty one.
// A seed written with no --param flags omits its params key, as the json:",omitempty" tag on shedrun.Seed intends.
func parseSeedParams(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	params := make(map[string]string, len(raw))
	for _, kv := range raw {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			return nil, fmt.Errorf("shedcli: malformed --param %q; want key=value", kv)
		}
		key := kv[:idx]
		if key == "" {
			return nil, fmt.Errorf("shedcli: malformed --param %q; key must not be empty", kv)
		}
		params[key] = kv[idx+1:]
	}
	return params, nil
}

// seedHereWayForward is the trailing clause on both "refuses to write here" wraps.
const seedHereWayForward = "way forward: run \"lym shed seed\" from the worktree the recipe drives, which the cause names"

// writeSeed validates runID and the driver, looks recipeName up in this package's table, and writes the seed.
// The lookup means a seed names only a recipe that can be armed.
// It returns the driver the seed records.
// The caller resolves location, so writeSeed performs no location lookup of its own.
func writeSeed(location *lymcwd.Location, runID, recipeName, driverFlag string, params map[string]string) (string, error) {
	// This check runs first: a seed written into fabric's own checkouts lands in a repository no run is driven from.
	// The checkouts are the Board and a pair's other side.
	// The Board's commit stages everything in that checkout, so it would sweep the seed onto its branch.
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return "", fmt.Errorf("shedcli: seed refuses to write here: %w; %s", err, seedHereWayForward)
	}
	if err := shedrun.ValidateRunID(runID); err != nil {
		return "", err
	}
	e, err := lookup(recipeName)
	if err != nil {
		return "", err
	}
	// The recipe's own location rule runs after fabric's:
	// a seed written where the recipe's verbs refuse is one no verb can consume.
	if e.RefuseSeedAt != nil {
		if err := e.RefuseSeedAt(location); err != nil {
			return "", fmt.Errorf("shedcli: seed refuses to write here: %w; %s", err, seedHereWayForward)
		}
	}
	driver := resolveSeedDriver(driverFlag, e.BootstrapVerb != "")
	if err := shedrun.ValidateDriver(driver); err != nil {
		return "", err
	}
	// The capability check runs only for a legal driver value: a misspelled value gets the vocabulary error above;
	// it does not get a capability error about a recipe that would have accepted the value meant.
	if driver == shedrun.DriverLLM && e.BootstrapVerb == "" {
		return "", fmt.Errorf("shedcli: recipe %q has no bootstrap verb, so it cannot be driven by an LLM; way forward: re-run with \"--driver go\"", recipeName)
	}
	if err := shedrun.WriteSeed(location, runID, shedrun.Seed{
		Recipe: recipeName,
		Driver: driver,
		Params: params,
	}); err != nil {
		return "", err
	}
	return driver, nil
}

// newSeedCommand builds "lym shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]".
// The command belongs to no recipe's Verbs set, so the verb gate would refuse it for every recipe.
// The seed-read pre-run skips it: that pre-run needs an existing seed, and seed writes the first one.
func newSeedCommand() *cobra.Command {
	var recipeFlag string
	var driverFlag string
	var paramFlags []string

	cmd := &cobra.Command{
		Use:         "seed <run-id> --recipe <name>",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Short:       "write the first seed for a run, naming its recipe, driver, and parameters",
		Long: `seed writes run-id's seed.json, naming the recipe that arms every subsequent
"lym shed" invocation against it. It is not itself a generic verb: it
belongs to no recipe's Verbs set, and it is exempt from the seed-read
pre-run every other "lym shed" command goes through, because that whole
sequence is predicated on a seed already existing -- "seed" is the command
invoked when one does not.

--recipe is required and validated against the same table every "lym shed"
invocation arms through. --driver defaults to "llm" for a recipe that has
a bootstrap verb and to "go" for one that does not; an explicit "llm" is
refused for a recipe with no bootstrap verb, and "go" stays selectable for
every recipe. An llm-driven run's driver session launches from the driver
stencil. --param is repeatable and sets a seed
parameter as key=value.

seed is idempotent against a byte-identical existing seed and refuses a
disagreeing one. A recipe's own bootstrap verb writes the seed it expects
and refuses to start over a disagreeing one, so a pre-seed must match it:
loom's run-id is "self" and its only param is the pair's recorded parent
branch, "parent".

Example:
  lym shed seed some-slug --recipe batten
  lym shed seed self --recipe tapestry --param parent=main
  lym shed seed self --recipe tapestry --driver go --param parent=main`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}

			runID := args[0]

			params, err := parseSeedParams(paramFlags)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				// lymcwd.Resolve's error is already self-describing (it IS the "not a git
				// repository" sentinel); pass it through bare rather than doubling that same
				// text on top of it.
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			driver, err := writeSeed(location, runID, recipeFlag, driverFlag, params)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"run_id": runID,
				"recipe": recipeFlag,
				"driver": driver,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&recipeFlag, "recipe", "", "the recipe this run's seed names (required)")
	// A MarkFlagRequired error becomes a JSON envelope through RunRootCtx's cobra-error wrapping;
	// every other cobra-level validation failure on this tree takes the same path.
	_ = cmd.MarkFlagRequired("recipe")
	// Empty by default, since the default depends on the recipe:
	// resolveSeedDriver picks it once --recipe is known.
	cmd.Flags().StringVar(&driverFlag, "driver", "", `the driver this run's child uses (default "llm", or "go" for a recipe with no bootstrap verb); "llm" is refused for a recipe with no bootstrap verb`)
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "a seed parameter as key=value (repeatable)")

	return cmd
}
