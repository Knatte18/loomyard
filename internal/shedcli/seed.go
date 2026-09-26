// seed.go implements "lyx shed seed", the one command that writes a run's first seed.json, plus the
// two testable workers behind its RunE: writeSeed (the whole resolution-free body: recipe lookup,
// driver default/validation, and the shedrun.WriteSeed call) and parseSeedParams (the --param k=v
// parser). It is a shedcli-only command, not a shedverbs verb: shedverbs is a leaf that derives no
// path and knows no recipe table, and seeding needs both.

package shedcli

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// resolveSeedDriver returns flagVal, defaulting to shedrun.DriverGo when flagVal is empty --
// mirroring battencli's own battenDriver helper.
func resolveSeedDriver(flagVal string) string {
	if flagVal == "" {
		return shedrun.DriverGo
	}
	return flagVal
}

// parseSeedParams parses raw, each entry a "key=value" pair, into a map. It refuses an entry with
// no "=" or an empty key. A nil raw yields a nil map, never an empty non-nil one, so an invocation
// with no --param flags writes a seed with an omitted params key exactly as shedrun.Seed's own
// json:",omitempty" tag intends.
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

// writeSeed is seed's whole resolution-free body: it validates runID, looks recipeName up through
// this package's own table (lookup) so a seed can only ever name an armable recipe, resolves and
// validates the driver, and calls shedrun.WriteSeed. It performs no lyxcwd.Resolve and no cwd read
// of its own -- location is already resolved, told rather than derived, by whichever caller holds
// it -- which is what lets seed_test.go drive it directly against a hand-built *lyxcwd.Location,
// with no real git repository behind it, and stay Tier 1.
func writeSeed(location *lyxcwd.Location, runID, recipeName, driverFlag string, params map[string]string) error {
	// Refused first, as every other shed verb over a batten seed already refuses through
	// battencli's ArmAt: a seed written into one of fabric's own checkouts (the Board checkout or
	// a pair's other side) lands in a repository no run is ever driven from, where the Board's own
	// commit, which stages everything in that checkout, would sweep it onto its branch.
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return fmt.Errorf("shedcli: seed refuses to write here: %w", err)
	}
	if err := shedrun.ValidateRunID(runID); err != nil {
		return err
	}
	e, err := lookup(recipeName)
	if err != nil {
		return err
	}
	// The recipe's own location rule, after fabric's: a batten seed is drivable from prime alone,
	// and a seed written where its recipe's verbs refuse is dirt no verb can ever consume.
	if e.RefuseSeedAt != nil {
		if err := e.RefuseSeedAt(location); err != nil {
			return fmt.Errorf("shedcli: seed refuses to write here: %w", err)
		}
	}
	driver := resolveSeedDriver(driverFlag)
	if err := shedrun.ValidateDriver(driver); err != nil {
		return err
	}
	// The capability check is reached only for a legal driver value: an operator who typed a
	// misspelling gets the vocabulary error above, not a capability error about a recipe that
	// would have accepted the value they meant.
	if driver == shedrun.DriverLLM && e.BootstrapVerb == "" {
		return fmt.Errorf("shedcli: recipe %q has no bootstrap verb, so it cannot be driven by an LLM", recipeName)
	}
	return shedrun.WriteSeed(location, runID, shedrun.Seed{
		Recipe: recipeName,
		Driver: driver,
		Params: params,
	})
}

// newSeedCommand builds "lyx shed seed <run-id> --recipe <name> [--driver <name>] [--param k=v]".
// It is exempt from resolvePersistentPreRun by that pre-run's own cmd.Name() == "seed"
// short-circuit, for the reason that pre-run's whole sequence is predicated on a seed already
// existing while "seed" is by definition the command invoked when one does not -- and it belongs to
// no recipe's Verbs set, so the verb gate would refuse it for every recipe.
func newSeedCommand() *cobra.Command {
	var recipeFlag string
	var driverFlag string
	var paramFlags []string

	cmd := &cobra.Command{
		Use:   "seed <run-id> --recipe <name>",
		Short: "write the first seed for a run, naming its recipe, driver, and parameters",
		Long: `seed writes run-id's seed.json, naming the recipe that arms every subsequent
"lyx shed" invocation against it. It is not itself a generic verb: it
belongs to no recipe's Verbs set, and it is exempt from the seed-read
pre-run every other "lyx shed" command goes through, because that whole
sequence is predicated on a seed already existing -- "seed" is the command
invoked when one does not.

--recipe is required and validated against the same table every "lyx shed"
invocation arms through. --driver defaults to "go", unchanged; "llm" is
accepted for a recipe that has a bootstrap verb, and refused for one that
does not; an llm-driven run's driver session needs the ly-drive skill from
loomyard's "ly" plugin installed. --param is repeatable and sets a seed
parameter as key=value.

seed is idempotent against a byte-identical existing seed and refuses a
disagreeing one.

Example:
  lyx shed seed some-slug --recipe batten
  lyx shed seed some-slug --recipe loom --driver go --param slug=some-slug`,
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

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				// lyxcwd.Resolve's error is already self-describing (it IS the "not a git
				// repository" sentinel); pass it through bare rather than doubling that same
				// text on top of it.
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			if err := writeSeed(location, runID, recipeFlag, driverFlag, params); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"run_id": runID,
				"recipe": recipeFlag,
				"driver": resolveSeedDriver(driverFlag),
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&recipeFlag, "recipe", "", "the recipe this run's seed names (required)")
	// MarkFlagRequired's own error surfaces through RunRootCtx's cobra-error wrapping as a JSON
	// envelope, exactly as every other cobra-level validation failure does on this tree.
	_ = cmd.MarkFlagRequired("recipe")
	cmd.Flags().StringVar(&driverFlag, "driver", shedrun.DriverGo, `the driver this run's child uses; "llm" is refused for a recipe with no bootstrap verb`)
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "a seed parameter as key=value (repeatable)")

	return cmd
}
