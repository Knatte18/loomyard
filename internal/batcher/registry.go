// registry.go maps each batchifier kind a batcher.yaml profile can name to the constructor that
// builds it from that profile; Active (config.go) resolves the active profile through it.

package batcher

import "fmt"

// DefaultName is the profile an empty active: key resolves to.
const DefaultName = "identity"

// constructors maps a profile's batchifier kind to the constructor that builds it, given the
// profile's name and settings.
var constructors = map[string]func(name string, p profile) (Batcher, error){
	"identity": func(string, profile) (Batcher, error) {
		return Identity(), nil
	},
	"cost": newCostFromProfile,
}

// newCostFromProfile builds the cost batchifier a `batchifier: cost` profile describes, erroring
// naming batcher.yaml and the profile when a parameter is missing or out of range.
func newCostFromProfile(name string, p profile) (Batcher, error) {
	if p.AloneAbove == nil {
		return nil, fmt.Errorf("batcher.yaml profile %q is missing alone_above", name)
	}
	if p.Budget == nil {
		return nil, fmt.Errorf("batcher.yaml profile %q is missing budget", name)
	}
	if p.MaxCards == nil {
		return nil, fmt.Errorf("batcher.yaml profile %q is missing max_cards", name)
	}
	if *p.AloneAbove <= 0 {
		return nil, fmt.Errorf("batcher.yaml profile %q alone_above must be positive: %v", name, *p.AloneAbove)
	}
	if *p.Budget <= 0 {
		return nil, fmt.Errorf("batcher.yaml profile %q budget must be positive: %v", name, *p.Budget)
	}
	if *p.MaxCards < 2 {
		return nil, fmt.Errorf("batcher.yaml profile %q max_cards must be at least 2: %d", name, *p.MaxCards)
	}
	weights, err := profileWeights(name, p)
	if err != nil {
		return nil, err
	}
	return NewCost(name, CostParams{AloneAbove: *p.AloneAbove, Budget: *p.Budget, MaxCards: *p.MaxCards, Weights: weights}), nil
}
