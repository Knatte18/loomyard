// export_test.go exposes the unexported done-check failure seams of record-batch to the package's external tests.
// It also exposes the composed prompt templates.

package websterengine

import (
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// ImplementerBodyTemplate returns the shared implementer job body read from stencilsDir.
func ImplementerBodyTemplate(stencilsDir string) ([]byte, error) {
	return stencilstore.Read(stencilsDir, "webster-body-implementer")
}

// ForkTemplate returns the composed thin in-session fork prompt template, read from stencilsDir.
var ForkTemplate = composeForkTemplate

// RecoveryTemplate returns the composed full cold-start recovery prompt template, read from stencilsDir.
var RecoveryTemplate = composeRecoveryTemplate

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
