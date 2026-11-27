// keynames.go renders the full name of a beam.yaml key.

package dobbyengine

import (
	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
)

// taskHeadroomKey is the beam.yaml key a start hold names.
const taskHeadroomKey = "task_headroom"

// The beam.yaml keys of the gate-hold notice's bound.
const (
	gateHoldFactorKey = "gate_hold_factor"
	gateHoldMinKey    = "gate_hold_min"
)

// keyName returns the full name of the beam.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(beamsetting.Module, path)
}
