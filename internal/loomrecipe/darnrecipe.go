// darnrecipe.go implements NewDarn and DarnRouting, the builder and routing projection of the embedded darn recipe, and DarnRetiredRows, the row renames a darn run is carried across.
// Neither of the recipe's budgets is declared in it: NewDarn stamps the verify budget `darn.yaml` supplies onto the verify gates of Darn-Write and Darn-Burler-Fix, and both apply its review budget to the Darn-Review perch, at every call.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// verifyGateName is the gate entry whose attempts the verify budget sets.
const verifyGateName = "verify"

// darnVerifyGatedRows are the darn recipe's rows whose verify gate runs darn.yaml's verify command.
var darnVerifyGatedRows = []string{loomshed.NameDarnWrite, loomshed.NameDarnBurlerFix}

// DarnRetiredRows maps each row name a darn run's status file may still carry from an earlier recipe onto the row that replaced it.
func DarnRetiredRows() map[string]string {
	return map[string]string{"Darn": loomshed.NameDarnWrite}
}

// NewDarn parses recipes.DarnRecipe, stamps env.DarnVerifyAttempts onto the verify gates of Darn-Write and Darn-Burler-Fix, builds it against env, applies env.Segments["Darn-Review"].MaxBounces to the Darn-Review perch, and returns the assembled *shedengine.Shed.
// A review budget of 0 routes Darn-Write's Done straight to Merge-In, so neither the perch nor Describe runs.
// It makes the status-path coherence checks New makes, then refuses an env.DarnVerifyAttempts below 1, a missing Darn-Review entry in env.Segments, naming the darn wiring's entry, and a Darn-Review budget below 0 before any row builds.
func NewDarn(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}
	if err := checkVerifyAttempts(env.DarnVerifyAttempts); err != nil {
		return nil, err
	}
	segments, err := reviewSegments(recipes.DarnRecipe)
	if err != nil {
		return nil, err
	}
	budgets, err := segmentBudgets(segments, env, darnEntryWayForward)
	if err != nil {
		return nil, err
	}

	parsed, err := shedbuild.Parse(recipes.DarnRecipe)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}
	for _, row := range darnVerifyGatedRows {
		if err := applyVerifyAttempts(&parsed, row, env.DarnVerifyAttempts); err != nil {
			return nil, err
		}
	}

	shed, err := shedbuild.NewShedFrom(parsed, env, paths)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}
	if err := applyDarnReviewBudget(shed.Producers, budgets); err != nil {
		return nil, err
	}

	return shed, nil
}

// darnEntryWayForward is the way forward of a missing Darn-Review budget.
const darnEntryWayForward = "add the Darn-Review budget entry the darn wiring builds from darn.yaml's review_max_bounces"

// DarnRouting projects the embedded darn recipe's routing without building any engine, with budgets' Darn-Review entry applied to the Darn-Review perch as NewDarn applies it.
// A budgets map lacking Darn-Review is refused naming the segment and the darn wiring's budget entry, and a negative entry is refused with the runnable way forward.
func DarnRouting(budgets map[string]int) (shedengine.Routing, error) {
	segments, err := reviewSegments(recipes.DarnRecipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	if err := checkSegmentBudgets(segments, budgets, darnEntryWayForward); err != nil {
		return shedengine.Routing{}, err
	}

	routing, err := shedbuild.RoutingOf(recipes.DarnRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}
	if err := applyDarnReviewBudget(routing.Producers, budgets); err != nil {
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

// applyDarnReviewBudget sets MaxBounces to budgets' Darn-Review entry on every row of the darn recipe's review segments, or, for a budget of 0, routes Darn-Write's Done to Merge-In so neither a review row nor Describe is reached and the merge-in still runs under the parent lock.
func applyDarnReviewBudget(producers []shedengine.ProducerDef, budgets map[string]int) error {
	if budgets[darnReviewSegment] == 0 {
		for i := range producers {
			if producers[i].Name == loomshed.NameDarnWrite {
				producers[i].OnDone = loomshed.NameMergeIn
			}
		}
		return nil
	}

	segments, err := reviewSegments(recipes.DarnRecipe)
	if err != nil {
		return err
	}

	applyReviewBudget(producers, segments, budgets)

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
