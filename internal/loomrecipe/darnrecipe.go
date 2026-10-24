// darnrecipe.go implements NewDarn and DarnRouting, the builder and routing projection of the embedded darn recipe, and DarnRetiredRows, the row renames a darn run is carried across.
// Neither of the recipe's budgets is declared in it: NewDarn stamps the verify budget `darn.yaml` supplies onto the verify gates of Darn-Write and Code-Burler-Fix, and both apply the Code-Review segment's budget to the perch, at every call.

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

// codeReviewSegment is the review segment of the code review perch, the darn recipe's only one.
const codeReviewSegment = "Code-Review"

// darnVerifyGatedRows are the darn recipe's rows whose verify gate runs the tree-scoped command, or the command impactset derives from the merge base, and runs darn.yaml's verify where impactset falls back.
var darnVerifyGatedRows = []string{loomshed.NameDarnWrite, loomshed.NameCodeBurlerFix}

// DarnRetiredRows maps each row name a darn run's status file may still carry from an earlier recipe onto the row that replaced it.
func DarnRetiredRows() map[string]string {
	return map[string]string{"Darn": loomshed.NameDarnWrite}
}

// NewDarn parses recipes.DarnRecipe, stamps env.DarnVerifyAttempts onto the verify gates of Darn-Write and Code-Burler-Fix, builds it against env, applies env.Segments["Code-Review"].MaxBounces to the Code-Review perch, and returns the assembled *shedengine.Shed.
// A review budget of 0 routes Darn-Write's Done and Finalize's reroute straight to PR-Review, so neither the perch nor Describe runs.
// It makes the status-path coherence checks New makes, then refuses an env.DarnVerifyAttempts below 1, a missing Code-Review entry in env.Segments, naming the loom wiring's segment table, and a Code-Review budget below 0 before any row builds.
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
	budgets, err := segmentBudgets(segments, env, loomSegmentTableWayForward)
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

// DarnRouting projects the embedded darn recipe's routing without building any engine, with budgets' Code-Review entry applied to the perch as NewDarn applies it.
// A budgets map lacking Code-Review is refused naming the segment and the loom wiring's segment table, and a negative entry is refused naming the segment.
func DarnRouting(budgets map[string]int) (shedengine.Routing, error) {
	segments, err := reviewSegments(recipes.DarnRecipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	if err := checkSegmentBudgets(segments, budgets, loomSegmentTableWayForward); err != nil {
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

// applyDarnReviewBudget sets MaxBounces to budgets' Code-Review entry on every row of the darn recipe's review segments, or, for a budget of 0, routes Darn-Write's Done and Finalize's reroute to PR-Review so neither a review row nor Describe is reached.
func applyDarnReviewBudget(producers []shedengine.ProducerDef, budgets map[string]int) error {
	if budgets[codeReviewSegment] == 0 {
		for i := range producers {
			switch producers[i].Name {
			case loomshed.NameDarnWrite:
				producers[i].OnDone = loomshed.NamePRReview
			case loomshed.NameFinalize:
				producers[i].OnReroute = loomshed.NamePRReview
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
