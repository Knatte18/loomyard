// gateslots.go implements GateSlots, the hub-mode constructor of the gate-slot pool every Go-side gate site is told.

package hubgeom

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// GateSlots returns the gate-slot pool of l's hub: its slot directory is the board dir's, and its limits are read strictly from the board dir's gate.yaml at every acquire.
// A gate.yaml that cannot be loaded surfaces at acquire as an error naming the file.
func GateSlots(l *lyxcwd.Location) *gateslot.Pool {
	boardDir := fabricengine.BoardDir(l.HubPath)
	return &gateslot.Pool{
		Dir: gateslot.Dir(boardDir),
		Limits: func() (gateslot.Limits, error) {
			cfg, err := gateslot.LoadConfig(boardDir)
			if err != nil {
				return gateslot.Limits{}, fmt.Errorf("load %s: %w", configengine.ConfigFile(boardDir, "gate"), err)
			}
			return cfg.Limits(), nil
		},
	}
}
