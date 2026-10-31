// keymove.go — the declared key moves between registered modules, and their application to a task's run settings.

package configreg

// KeyMove names one key that moved from a source module's config file to a target module's, with its key unchanged in meaning.
type KeyMove struct {
	// FromModule and FromKey name the key in its old module.
	FromModule, FromKey string
	// ToModule and ToKey name the key in its new module.
	ToModule, ToKey string
	// KeepTarget makes a target file that holds the key, and was not started from the template in the same pass, keep its value whatever it is, the template default included.
	KeepTarget bool
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
		{FromModule: "burler", FromKey: "lenses", ToModule: "lenses", ToKey: "lenses", KeepTarget: true},
		{FromModule: "burler", FromKey: "fans", ToModule: "lenses", ToKey: "fans", KeepTarget: true},
		// The darn recipe's review perch is loom's Code-Review perch, so its darn.yaml keys moved to loom.yaml's Code-Review keys;
		// review_fan and review_fix_fan are the fan keys' names before darn_reviewer_fan and darn_fixer_fan.
		{FromModule: "darn", FromKey: "darn_reviewer_model", ToModule: "loom", ToKey: "code_reviewer_model"},
		{FromModule: "darn", FromKey: "darn_fixer_model", ToModule: "loom", ToKey: "code_fixer_model"},
		{FromModule: "darn", FromKey: "darn_reviewer_fan", ToModule: "loom", ToKey: "code_reviewer_fan"},
		{FromModule: "darn", FromKey: "darn_fixer_fan", ToModule: "loom", ToKey: "code_fixer_fan"},
		{FromModule: "darn", FromKey: "review_fan", ToModule: "loom", ToKey: "code_reviewer_fan"},
		{FromModule: "darn", FromKey: "review_fix_fan", ToModule: "loom", ToKey: "code_fixer_fan"},
		{FromModule: "darn", FromKey: "review_max_bounces", ToModule: "loom", ToKey: "code_max_bounces"},
		{FromModule: "darn", FromKey: "review_hard_max_bounces", ToModule: "loom", ToKey: "code_hard_max_bounces"},
		{FromModule: "darn", FromKey: "review_amend_max_bounces", ToModule: "loom", ToKey: "review_amend_max_bounces"},
		// The plan perch and the implementer are the tapestry recipe's alone, so loom.yaml's plan keys moved to tapestry.yaml under the same names.
		{FromModule: "loom", FromKey: "plan", ToModule: "tapestry", ToKey: "plan"},
		{FromModule: "loom", FromKey: "plan_timeout_min", ToModule: "tapestry", ToKey: "plan_timeout_min"},
		{FromModule: "loom", FromKey: "implementer", ToModule: "tapestry", ToKey: "implementer"},
		{FromModule: "loom", FromKey: "plan_lenses", ToModule: "tapestry", ToKey: "plan_lenses"},
		{FromModule: "loom", FromKey: "plan_reviewer_model", ToModule: "tapestry", ToKey: "plan_reviewer_model"},
		{FromModule: "loom", FromKey: "plan_fixer_model", ToModule: "tapestry", ToKey: "plan_fixer_model"},
		{FromModule: "loom", FromKey: "plan_reviewer_fan", ToModule: "tapestry", ToKey: "plan_reviewer_fan"},
		{FromModule: "loom", FromKey: "plan_fixer_fan", ToModule: "tapestry", ToKey: "plan_fixer_fan"},
		{FromModule: "loom", FromKey: "plan_judge", ToModule: "tapestry", ToKey: "plan_judge"},
		{FromModule: "loom", FromKey: "plan_seed", ToModule: "tapestry", ToKey: "plan_seed"},
		{FromModule: "loom", FromKey: "plan_max_bounces", ToModule: "tapestry", ToKey: "plan_max_bounces"},
		{FromModule: "loom", FromKey: "plan_hard_max_bounces", ToModule: "tapestry", ToKey: "plan_hard_max_bounces"},
		{FromModule: "loom", FromKey: "plan_auto_accept", ToModule: "tapestry", ToKey: "plan_auto_accept"},
	}
}

// KeyCopy names one key copied, not moved, from a source module's config file to a target module's, under a key name of its own.
// The copy reads and writes the hub's base layer only.
type KeyCopy struct {
	// FromModule and FromKey name the key in the source module, which keeps it.
	FromModule, FromKey string
	// ToModule and ToKey name the key in the target module.
	ToModule, ToKey string
}

// KeyCopies returns every declared key copy, in the order reconcile applies them.
// A reconcile places the value in the target module's hub file when the source file sets it and the target file does not.
func KeyCopies() []KeyCopy {
	return []KeyCopy{}
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
