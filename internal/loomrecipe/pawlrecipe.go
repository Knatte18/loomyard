// pawlrecipe.go implements NewPawl and PawlRouting, the builder and routing projection of the embedded pawl recipe, a beam's life.
// Both apply the Code-Review segment's budget to the recipe's perch.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// NewPawl parses recipes.PawlRecipe, builds it against env, applies env.Segments["Code-Review"].MaxBounces to the Code-Review perch and returns the assembled *shedengine.Shed.
// A review budget of 0 stamps `zero_rounds` on Code-Bouncer, which then settles approved at once; Beam-Describe carries no entry gate, so it writes whatever the budget.
// It makes the status-path coherence checks New makes, then refuses a missing Code-Review entry in env.Segments, naming the loom wiring's segment table, and a budget below 0 before any row builds.
func NewPawl(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}
	segments, err := reviewSegments(recipes.PawlRecipe)
	if err != nil {
		return nil, err
	}
	budgets, err := segmentBudgets(segments, env, loomSegmentTableWayForward)
	if err != nil {
		return nil, err
	}

	parsed, err := shedbuild.Parse(recipes.PawlRecipe)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}
	stampZeroRounds(&parsed, segments, budgets)

	shed, err := shedbuild.NewShedFrom(parsed, env, paths)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}
	applyReviewBudget(shed.Producers, segments, budgets)

	return shed, nil
}

// PawlRouting projects the embedded pawl recipe's routing without building any engine, with budgets' Code-Review entry applied to the perch as NewPawl applies it and every edge kept at any budget.
// A budgets map lacking Code-Review is refused naming the segment and the loom wiring's segment table, and a negative entry is refused naming the segment.
func PawlRouting(budgets map[string]int) (shedengine.Routing, error) {
	segments, err := reviewSegments(recipes.PawlRecipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	if err := checkSegmentBudgets(segments, budgets, loomSegmentTableWayForward); err != nil {
		return shedengine.Routing{}, err
	}

	routing, err := shedbuild.RoutingOf(recipes.PawlRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}
	applyReviewBudget(routing.Producers, segments, budgets)

	return routing, nil
}
