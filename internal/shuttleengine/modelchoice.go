// modelchoice.go defines ModelChoice, the engine-neutral form of one resolved model choice.

package shuttleengine

import "github.com/Knatte18/loomyard/internal/modelspec"

// ModelChoice is one resolved choice of engine and model, with the provider's effort and version.
// An empty field means the provider default.
type ModelChoice struct {
	Engine  string
	Model   string
	Effort  string
	Version string
}

// NewModelChoice returns the ModelChoice of resolved: its engine and model, and the effort and version params.
// An absent param leaves its field empty.
func NewModelChoice(resolved modelspec.Resolved) ModelChoice {
	return ModelChoice{
		Engine:  resolved.Engine,
		Model:   resolved.Model,
		Effort:  resolved.Params["effort"],
		Version: resolved.Params["version"],
	}
}
