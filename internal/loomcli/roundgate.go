// roundgate.go builds the told sources of the hub's gate keys that loom's and darn's wiring hand the verify gate, the landing producers and the Publish failure note.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
)

// gateCallSource returns a reader of the call of a gate key, rendered from the config loaded over dirs at each call, so a gate.yaml edit between two calls is seen.
// A config or gate-key refusal is returned with its own way forward.
func gateCallSource(dirs configengine.Dirs) func(key string) (string, error) {
	return func(key string) (string, error) {
		cfg, err := gateslot.LoadConfig(dirs)
		if err != nil {
			return "", err
		}
		return cfg.GateCall(key)
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
