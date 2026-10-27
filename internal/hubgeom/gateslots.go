// gateslots.go implements GateSlots, the hub-mode constructor of the gate-slot pool every Go-side gate site is told.

package hubgeom

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// GateSlots returns the gate-slot pool of l's hub: its slot directory is the board dir's, and its limits are read strictly from the board dir's gate.yaml and this machine's override at every acquire.
// A gate config that cannot be loaded surfaces at acquire as an error naming each file in the stack.
func GateSlots(l *lymcwd.Location) *gateslot.Pool {
	boardDir := fabricengine.BoardDir(l.HubPath)
	return &gateslot.Pool{
		Dir: gateslot.Dir(boardDir),
		Limits: func() (gateslot.Limits, error) {
			dirs := fabricengine.ConfigDirs(l)
			cfg, err := gateslot.LoadConfig(dirs)
			if err != nil {
				named := configengine.ConfigFile(boardDir, "gate")
				for _, file := range gateslot.ConfigFiles(dirs)[1:] {
					named += " and " + file
				}
				return gateslot.Limits{}, fmt.Errorf("load %s: %w", named, err)
			}
			return cfg.Limits(), nil
		},
	}
}
