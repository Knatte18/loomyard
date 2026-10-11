// darnrecipe.go implements NewDarn and DarnRouting, the builder and routing projection of the embedded darn recipe.
// Neither of the recipe's budgets is declared in it: NewDarn stamps the verify budget `darn.yaml` supplies onto the Darn row's verify gate, and both apply its review budget to the Darn-Review perch, at every call.

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

// NewDarn parses recipes.DarnRecipe, stamps env.DarnVerifyAttempts onto the Darn row's verify gate, builds it against env, applies env.DarnReviewMaxBounces to the Darn-Review perch, and returns the assembled *shedengine.Shed.
// A review budget of 0 routes the Darn row's Done straight to Publish, so the perch never runs.
// It makes the status-path coherence checks New makes, then refuses an env.DarnVerifyAttempts below 1 and an env.DarnReviewMaxBounces below 0 before any row builds.
func NewDarn(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}
	if err := checkVerifyAttempts(env.DarnVerifyAttempts); err != nil {
		return nil, err
	}
	if err := checkDarnReviewBudget(env.DarnReviewMaxBounces); err != nil {
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
	if err := applyDarnReviewBudget(shed.Producers, env.DarnReviewMaxBounces); err != nil {
		return nil, err
	}

	return shed, nil
}

// DarnRouting projects the embedded darn recipe's routing without building any engine, with reviewMaxBounces applied to the Darn-Review perch as NewDarn applies it.
// A reviewMaxBounces below 0 is refused.
func DarnRouting(reviewMaxBounces int) (shedengine.Routing, error) {
	if err := checkDarnReviewBudget(reviewMaxBounces); err != nil {
		return shedengine.Routing{}, err
	}

	routing, err := shedbuild.RoutingOf(recipes.DarnRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}
	if err := applyDarnReviewBudget(routing.Producers, reviewMaxBounces); err != nil {
		return shedengine.Routing{}, err
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

// checkDarnReviewBudget refuses a darn review bounce budget below 0.
func checkDarnReviewBudget(budget int) error {
	if budget < 0 {
		return fmt.Errorf("loomrecipe: darn review_max_bounces = %d; want 0 or more; way forward: run \"lyx config darn --set review_max_bounces=<n>\" from the prime, then \"lyx loom resume\"", budget)
	}

	return nil
}

// applyDarnReviewBudget sets MaxBounces to budget on every row of the darn recipe's review segments, or, for a budget of 0, routes the Darn row's Done to Publish so no review row is reached.
func applyDarnReviewBudget(producers []shedengine.ProducerDef, budget int) error {
	if budget == 0 {
		for i := range producers {
			if producers[i].Name == loomshed.NameDarn {
				producers[i].OnDone = loomshed.NamePublish
			}
		}
		return nil
	}

	segments, err := reviewSegments(recipes.DarnRecipe)
	if err != nil {
		return err
	}
	applyReviewBudget(producers, segments, budget)

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
