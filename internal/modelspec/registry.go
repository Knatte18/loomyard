// registry.go implements the pinned built-in fallback registry and Registry.Resolve, the
// alias-lookup/default-merge half of the model-spec contract (Parse handles grammar only; this file
// handles resolution).

package modelspec

import (
	"fmt"
	"sort"
)

// builtins returns the pinned, default-free fallback registry: sonnet, opus,
// haiku, and fable (operator effort defaults live only in models.yaml, not here).
func builtins() Registry {
	return Registry{
		"sonnet": {Engine: "claude", Model: "sonnet"},
		"opus":   {Engine: "claude", Model: "opus"},
		"haiku":  {Engine: "claude", Model: "haiku"},
		"fable":  {Engine: "claude", Model: "fable"},
	}
}

// Resolve resolves s against r and returns the fully realized Resolved.
// For alias form, s.Alias is looked up in r (unknown alias is an error); Resolved.Params starts as
// the entry's Defaults overlaid by s.Params.
// For escape form, r is unused.
// Resolved.Params is never nil.
func (r Registry) Resolve(s Spec) (Resolved, error) {
	// Escape form carries its own engine/model and bypasses the registry
	// entirely — there is nothing to look up.
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

	// Start from the registry entry's defaults, then let the spec's bracket
	// params override per key — the contract's precedence rule.
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
