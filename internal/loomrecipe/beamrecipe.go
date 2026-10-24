// beamrecipe.go implements NewBeam and BeamRouting, the builder and routing projection of the embedded beam recipe.
// The recipe has no review segment, so neither takes a budget.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// NewBeam parses recipes.BeamRecipe, builds it against env and returns the assembled *shedengine.Shed.
// It makes the status-path coherence checks New makes before any row builds.
func NewBeam(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}

	parsed, err := shedbuild.Parse(recipes.BeamRecipe)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}
	shed, err := shedbuild.NewShedFrom(parsed, env, paths)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	return shed, nil
}

// BeamRouting projects the embedded beam recipe's routing without building any engine.
func BeamRouting() (shedengine.Routing, error) {
	routing, err := shedbuild.RoutingOf(recipes.BeamRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}

	return routing, nil
}
