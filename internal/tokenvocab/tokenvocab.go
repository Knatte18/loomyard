// tokenvocab.go defines the token vocabulary's core types (Ctx, Token) and the
// registry of always-resolvable tokens, plus Build, which resolves the whole
// registry into a flat map a template renderer (internal/stencil) can consume.

package tokenvocab

import "github.com/Knatte18/loomyard/internal/hubgeometry"

// Ctx carries the context a Token.Resolve needs to produce its value. It is a
// struct — not a bare *hubgeometry.Layout — so a future token (e.g. one keyed on
// a task slug) can add a field here without changing every Resolve signature.
type Ctx struct {
	// Layout is the resolved worktree/Hub geometry every current token reads from.
	Layout *hubgeometry.Layout
}

// Token is one named, resolvable entry in the vocabulary: a template marker name
// paired with the function that produces its value from a Ctx.
type Token struct {
	// Name is the template marker name (the "X" in a template's {{.X}}).
	Name string
	// Resolve computes this token's value from c. It must be always-resolvable —
	// tokens that can fail or be conditionally absent are out of scope for this
	// registry (see doc.go's extension rule).
	Resolve func(Ctx) string
}

// registry is the single source of truth for the token vocabulary. Adding a
// token is exactly one entry here; Build and Render both iterate registry
// rather than switching on token names, so neither needs to change when a
// token is added.
var registry = []Token{
	{Name: "repo", Resolve: func(c Ctx) string { return c.Layout.Repo }},
	{Name: "hub", Resolve: func(c Ctx) string { return c.Layout.Hub }},
}

// Build resolves every token in the registry against c and returns the result
// as a flat map keyed by token name, the shape internal/stencil.Fill consumes
// as its values argument.
func Build(c Ctx) map[string]string {
	values := make(map[string]string, len(registry))
	for _, token := range registry {
		values[token.Name] = token.Resolve(c)
	}
	return values
}
