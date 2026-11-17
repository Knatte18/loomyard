// export_test.go exposes the package's unexported values to its external tests.

package weaverengine

import "github.com/Knatte18/loomyard/internal/runplan"

// RunPlanOf returns the shared machinery's view of g.
func RunPlanOf(g Geometry) runplan.Geometry { return g.runPlan() }
