// export_test.go exposes the package's unexported values to its external tests.

package weaverengine

import (
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// RunPlanOf returns the shared machinery's view of g.
func RunPlanOf(g Geometry) runplan.Geometry { return g.runPlan() }

// BatchGateSpec returns the must-pass gates of one batch attempt's strand.
func BatchGateSpec(geom Geometry, cards []planparser.Card, last bool, rowGates shuttleengine.GateSpec) (shuttleengine.GateSpec, error) {
	return batchGateSpec(geom, cards, last, rowGates)
}
