// registry.go maps each batchifier kind a batcher.yaml profile can name to the constructor that builds it from that profile;
// Active (config.go) resolves the active profile through it.

package batcher

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// DefaultName is the profile an empty active: key resolves to; the template's own active: names the cautious profile instead.
const DefaultName = "identity"

// constructors maps a profile's batchifier kind to the constructor that builds it, given the profile's name and settings.
var constructors = map[string]func(name string, p profile) (Batcher, error){
	"identity": func(string, profile) (Batcher, error) {
		return Identity(), nil
	},
	"cost": newCostFromProfile,
}

// newCostFromProfile builds the cost batchifier a `batchifier: cost` profile describes, erroring naming batcher.yaml and the profile when a parameter is missing or out of range, or when it still carries the retired alone_above.
func newCostFromProfile(name string, p profile) (Batcher, error) {
	if p.AloneAbove != nil {
		return nil, retiredKeyError(name, "alone_above")
	}
	if p.Budget == nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q is missing budget", name))
	}
	if p.MaxCards == nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q is missing max_cards", name))
	}
	if *p.Budget <= 0 {
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q budget must be positive: %v", name, *p.Budget))
	}
	if *p.MaxCards < 2 {
		return nil, configengine.MarkInvalid(fmt.Errorf("batcher.yaml profile %q max_cards must be at least 2: %d", name, *p.MaxCards))
	}
	weights, err := profileWeights(name, p)
	if err != nil {
		return nil, err
	}
	return NewCost(name, CostParams{Budget: *p.Budget, MaxCards: *p.MaxCards, Weights: weights}), nil
}
