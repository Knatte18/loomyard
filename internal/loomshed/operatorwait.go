// operatorwait.go declares the rows a run's child may wait in with no agent working, and the predicate that reads them.

package loomshed

import (
	"slices"

	"github.com/Knatte18/loomyard/internal/pawlengine"
)

// OperatorWaitRows lists the rows at which a running child waits on something outside it with no agent working:
// Beam-Await waits on the beam's members, PR-Review on the run's pull request and Sampler-Session on the operator.
// A step at one of them is a live wait, not a stall.
var OperatorWaitRows = []string{pawlengine.NameBeamAwait, NamePRReview, NameSamplerSession}

// WaitsOnOperator reports whether row is one of OperatorWaitRows.
func WaitsOnOperator(row string) bool {
	return slices.Contains(OperatorWaitRows, row)
}
