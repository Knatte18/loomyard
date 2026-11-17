// promptengine.go implements CheckPromptEngine.
// An engine's Prepare runs it so a prompt rendered for one engine never launches on another.

package shuttleengine

import "fmt"

// CheckPromptEngine reports an error unless spec's prompt was rendered for engine, the engine about to launch it.
// A spec marked HandWrittenPrompt passes, since its text was rendered for no engine.
// The error names both engines and, as the way forward, the producer's model setting for the spec's role.
func CheckPromptEngine(spec Spec, engine string) error {
	if spec.HandWrittenPrompt {
		return nil
	}
	if spec.PromptEngine == "" {
		return fmt.Errorf("shuttle: the spec for role %q names no prompt engine, so its prompt may be worded for an engine other than %q; way forward: have the producer render its prompt for the engine its model setting resolves to and set that engine as the spec's prompt engine", spec.Role, engine)
	}
	if spec.PromptEngine != engine {
		return fmt.Errorf("shuttle: the prompt for role %q was rendered for engine %q, but the launch runs on engine %q; way forward: have the producer render its prompt for the engine its model setting resolves to, or set that model setting to name engine %q", spec.Role, spec.PromptEngine, engine, spec.PromptEngine)
	}
	return nil
}
