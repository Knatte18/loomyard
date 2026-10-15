// modelchoice.go defines ModelChoice, the engine-neutral form of one resolved model choice.

package shuttleengine

import "github.com/Knatte18/loomyard/internal/modelspec"

// ModelChoice is one resolved choice of engine and model, with the provider's effort and version.
// An empty field means the provider default.
type ModelChoice struct {
	// Engine is the registered engine name the run uses, empty for the engine set's default.
	// Start and Open refuse a name the set does not hold; validate does not inspect it.
	Engine string
	// Model, when non-empty, selects a specific provider model; empty defers to the engine/provider default.
	Model string
	// Effort, when non-empty, selects a reasoning-effort override; empty defers to the engine/provider default.
	// Effort values are provider vocabulary — validate does NOT inspect this field at all (neither defaulting nor rejecting it);
	// the engine is the sole validator, since only it knows which values its provider realizes.
	// A non-empty value the engine cannot realize is a hard error from the engine (see claudeengine's validateEffort), not from Spec.validate.
	Effort string
	// Version, when non-empty, selects a version pin the provider engine realizes by translating (Model, Version) into a provider-specific model id; empty means no pin.
	// Version values are provider vocabulary — validate does NOT inspect this field at all;
	// the engine is the sole validator, since only it knows which values its provider realizes.
	// A (Model, Version) pair the engine cannot realize is a hard error from the engine (see claudeengine's resolveModelID), not from Spec.validate.
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
