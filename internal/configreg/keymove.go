// keymove.go — the declared key moves between registered modules, and their application to a task's run settings.

package configreg

// KeyMove names one key that moved from a source module's config file to a target module's, with its key unchanged in meaning.
type KeyMove struct {
	// FromModule and FromKey name the key in its old module.
	FromModule, FromKey string
	// ToModule and ToKey name the key in its new module.
	ToModule, ToKey string
}

// KeyMoves returns every declared key move, in the order reconcile applies them.
// A reconcile relocates a layer's source values into the target module's file of the same layer, and run settings are rewritten the same way.
func KeyMoves() []KeyMove {
	return []KeyMove{
		{FromModule: "shuttle", FromKey: "claude", ToModule: "claude", ToKey: "binary"},
		{FromModule: "shuttle", FromKey: "claude_deny_agent_tool", ToModule: "claude", ToKey: "deny_agent_tool"},
		{FromModule: "shuttle", FromKey: "claude_deny_ask_user_question", ToModule: "claude", ToKey: "deny_ask_user_question"},
		{FromModule: "shuttle", FromKey: "claude_prompt_cache_ttl", ToModule: "claude", ToKey: "prompt_cache_ttl"},
		{FromModule: "shuttle", FromKey: "claude_prompt_cache_ttl_roles", ToModule: "claude", ToKey: "prompt_cache_ttl_roles"},
	}
}

// moveRunSettings returns settings with every declared key move applied, leaving the argument untouched.
// A moved key lands in the target module's object unless that object already holds it, in which case the target's value is kept and the source key dropped.
// A module whose value is not an object is passed through for the module loop to refuse.
func moveRunSettings(settings map[string]any) map[string]any {
	moved := make(map[string]any, len(settings))
	for name, value := range settings {
		moved[name] = value
	}
	copied := map[string]bool{}
	objectOf := func(module string) (map[string]any, bool) {
		object, ok := moved[module].(map[string]any)
		if !ok {
			return nil, false
		}
		if !copied[module] {
			clone := make(map[string]any, len(object))
			for key, entry := range object {
				clone[key] = entry
			}
			moved[module] = clone
			copied[module] = true
			object = clone
		}
		return object, true
	}

	for _, move := range KeyMoves() {
		source, ok := moved[move.FromModule].(map[string]any)
		if !ok {
			continue
		}
		value, present := source[move.FromKey]
		if !present {
			continue
		}
		if existing, exists := moved[move.ToModule]; exists {
			if _, isObject := existing.(map[string]any); !isObject {
				continue
			}
		}
		source, _ = objectOf(move.FromModule)
		delete(source, move.FromKey)

		target, ok := objectOf(move.ToModule)
		if !ok {
			target = map[string]any{}
			moved[move.ToModule] = target
			copied[move.ToModule] = true
		}
		if _, held := target[move.ToKey]; !held {
			target[move.ToKey] = value
		}
	}
	return moved
}
