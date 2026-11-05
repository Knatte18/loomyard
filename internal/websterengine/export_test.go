// export_test.go exposes the unexported done-check failure seams of record-batch to the package's external tests.

package websterengine

import "github.com/Knatte18/loomyard/internal/planindex"

// MaxRecoveries is how many counted recovery spawns one batch gets.
const MaxRecoveries = maxRecoveries

// CardNotDoneInputs is the input failCardNotDone takes.
type CardNotDoneInputs = cardNotDoneInputs

// FailCardNotDone exposes failCardNotDone.
var FailCardNotDone = failCardNotDone

// CardNotDoneFor returns the error cardNotDoneFor builds from findings, or nil when findings is empty.
func CardNotDoneFor(findings []planindex.Finding) error {
	notDone := cardNotDoneFor(postBatchInputs{}, findings)
	if notDone == nil {
		return nil
	}
	return notDone
}
