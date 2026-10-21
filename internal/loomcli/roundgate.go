// roundgate.go builds the told sources of the hub's gate lists that loom's and darn's wiring hand the verify gate and the Publish failure note.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/loomshed"
)

// roundGateSource returns the told source of the hub's `round` gate over dirs.
// The config is loaded at each call, so a gate.yaml edit between two arrivals takes effect, and a config or `round` list refusal is returned with its own way forward.
func roundGateSource(dirs configengine.Dirs) func() (loomshed.RoundGate, error) {
	return func() (loomshed.RoundGate, error) {
		cfg, err := gateslot.LoadConfig(dirs)
		if err != nil {
			return loomshed.RoundGate{}, err
		}
		tiers, err := cfg.TierList("round")
		if err != nil {
			return loomshed.RoundGate{}, err
		}
		return loomshed.RoundGate{
			Tiers: tiers,
			Render: func(scope ...string) string {
				// TierList accepted the round list above, so GateCall has no refusal left.
				call, _ := cfg.GateCall("round", scope...)
				return call
			},
			GateTiers: cfg.TierList,
		}, nil
	}
}

// gateTiersLookup returns a lookup of a named gate's tier list that loads the config over dirs at each call.
func gateTiersLookup(dirs configengine.Dirs) func(gate string) ([]int, error) {
	return func(gate string) ([]int, error) {
		cfg, err := gateslot.LoadConfig(dirs)
		if err != nil {
			return nil, err
		}
		return cfg.TierList(gate)
	}
}
