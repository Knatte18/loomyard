// darnrecipe.go implements NewDarn and DarnRouting, the builder and routing projection of the embedded darn recipe.
// The recipe's verify budget is not declared in it: NewDarn stamps the budget `darn.yaml` supplies onto the Darn row's verify gate at every build.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// verifyGateName is the gate entry of the Darn row whose attempts the verify budget sets.
const verifyGateName = "verify"

// NewDarn parses recipes.DarnRecipe, stamps env.DarnVerifyAttempts onto the Darn row's verify gate, builds it against env, and returns the assembled *shedengine.Shed.
// It makes the status-path coherence checks New makes, then refuses an env.DarnVerifyAttempts below 1 before any row builds.
// The recipe has no review segment, so no review budget applies.
func NewDarn(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}
	if err := checkVerifyAttempts(env.DarnVerifyAttempts); err != nil {
		return nil, err
	}

	parsed, err := shedbuild.Parse(recipes.DarnRecipe)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}
	if err := applyVerifyAttempts(&parsed, loomshed.NameDarn, env.DarnVerifyAttempts); err != nil {
		return nil, err
	}

	shed, err := shedbuild.NewShedFrom(parsed, env, paths)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	return shed, nil
}

// DarnRouting projects the embedded darn recipe's routing without building any engine.
// It reads no verify budget, so a caller that never builds a Shed is never refused by it.
func DarnRouting() (shedengine.Routing, error) {
	routing, err := shedbuild.RoutingOf(recipes.DarnRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}

	return routing, nil
}

// checkVerifyAttempts refuses a verify attempt budget below 1.
func checkVerifyAttempts(attempts int) error {
	if attempts < 1 {
		return fmt.Errorf("loomrecipe: verify_attempts = %d; want at least 1; way forward: run \"lyx config darn --set verify_attempts=<n>\" from the prime, then \"lyx loom resume\"", attempts)
	}

	return nil
}

// applyVerifyAttempts sets the attempts of the verify gate entry of the recipe row named row.
// A missing row, a row with no gates and a row with no verify entry are each an error.
func applyVerifyAttempts(recipe *shedbuild.Recipe, row string, attempts int) error {
	for i := range recipe.Producers {
		if recipe.Producers[i].Name != row {
			continue
		}
		gates, _ := recipe.Producers[i].Config["gates"].([]any)
		for _, raw := range gates {
			if entry, ok := raw.(map[string]any); ok && entry["name"] == verifyGateName {
				entry["attempts"] = attempts
				return nil
			}
		}
		return fmt.Errorf("loomrecipe: row %q has no %q gate entry to stamp the verify budget onto", row, verifyGateName)
	}

	return fmt.Errorf("loomrecipe: recipe has no row %q to stamp the verify budget onto", row)
}
