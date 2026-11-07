// registry.go implements the pinned built-in fallback registry and Registry.Resolve.
// Resolve is the alias-lookup and default-merge step of the model-spec contract.
// Grammar checking lives in parse.go.

package modelspec

import (
	"fmt"
	"sort"
)

// builtins returns the pinned fallback registry.
// It holds the Claude aliases sonnet, opus, haiku and fable with no defaults.
// Their effort defaults live only in models.yaml, so the registry holds none.
// It also holds the Codex aliases sol, astra and luna at effort medium,
// so a Codex spec naming no effort never launches at the provider's own default.
func builtins() Registry {
	codexDefaults := func() map[string]string { return map[string]string{"effort": "medium"} }
	return Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus"},
		"haiku":  {Engine: "claude", Model: "haiku"},
		"fable":  {Engine: "claude", Model: "fable"},
		"sol":    {Engine: "codex", Model: "gpt-6.1-sol", Defaults: codexDefaults()},
		"astra":  {Engine: "codex", Model: "gpt-6-astra", Defaults: codexDefaults()},
		"luna":   {Engine: "codex", Model: "gpt-6-luna", Defaults: codexDefaults()},
	}
}

// Resolve resolves s against r and returns the fully realized Resolved.
// For alias form, s.Alias is looked up in r, and an unknown alias is an error.
// Resolved.Params starts as the entry's Defaults overlaid by s.Params.
// For escape form, r is unused.
// Resolved.Params is never nil.
func (r Registry) Resolve(s Spec) (Resolved, error) {
	// Escape form carries its own engine and model and bypasses the registry: there is nothing to look up.
	if s.Alias == "" {
		return Resolved{
			Engine: s.Engine,
			Model:  s.Model,
			Params: copyParams(s.Params),
		}, nil
	}

	entry, ok := r[s.Alias]
	if !ok {
		return Resolved{}, fmt.Errorf("modelspec: unknown alias %q; known aliases: %v", s.Alias, sortedKeys(r))
	}

	// Start from the registry entry's defaults, then let the spec's bracket params override per key.
	// This is the contract's precedence rule.
	params := copyParams(entry.Defaults)
	for k, v := range s.Params {
		params[k] = v
	}

	return Resolved{
		Engine: entry.Engine,
		Model:  entry.Model,
		Params: params,
	}, nil
}

// copyParams returns a fresh, non-nil copy of m.
func copyParams(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// sortedKeys returns the sorted alias keys of r (for unknown-alias error messages).
func sortedKeys(r Registry) []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
