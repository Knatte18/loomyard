// roundgate.go builds the told sources of the hub's gate lists that loom's and darn's wiring hand the verify gate, the landing producers and the Publish failure note.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/loomshed"
)

// gateListSource returns the told source of the hub's named gate list over dirs, which the verify gate runs narrowed to a diff.
// The config is loaded at each call, so a gate.yaml edit between two arrivals takes effect, and a config or gate-list refusal is returned with its own way forward.
func gateListSource(dirs configengine.Dirs, gate string) func() (loomshed.RoundGate, error) {
	return func() (loomshed.RoundGate, error) {
		cfg, err := gateslot.LoadConfig(dirs)
		if err != nil {
			return loomshed.RoundGate{}, err
		}
		tiers, err := cfg.TierList(gate)
		if err != nil {
			return loomshed.RoundGate{}, err
		}
		return loomshed.RoundGate{
			Tiers: tiers,
			Render: func(scope ...string) string {
				// TierList accepted the list above, so GateCall has no refusal left.
				call, _ := cfg.GateCall(gate, scope...)
				return call
			},
			GateTiers: cfg.TierList,
		}, nil
	}
}

// gateCallSource returns a reader of the call of the named gate over the whole tree, rendered from the config loaded over dirs at each call.
// A config or gate-list refusal is returned with its own way forward.
func gateCallSource(dirs configengine.Dirs, gate string) func() (string, error) {
	return func() (string, error) {
		cfg, err := gateslot.LoadConfig(dirs)
		if err != nil {
			return "", err
		}
		return cfg.GateCall(gate, "--all")
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
