// tokenvocab.go defines the token vocabulary's core types (Ctx, Token) and the
// registry of always-resolvable tokens, plus Build, which resolves the whole
// registry into a flat map a template renderer (internal/stencil) can consume.

package tokenvocab

import "github.com/Knatte18/loomyard/internal/lyxcwd"

// Ctx carries the context a Token.Resolve needs.
type Ctx struct {
	// Layout is the resolved geometry every token reads from.
	Layout *lyxcwd.Location
}

// Token is one named, resolvable entry in the vocabulary.
type Token struct {
	// Name is the template marker name (the "X" in {{.X}}).
	Name string
	// Resolve computes this token's value from c.
	Resolve func(Ctx) string
}

// registry is the single source of truth for the token vocabulary.
var registry = []Token{
	{Name: "repo", Resolve: func(c Ctx) string { return c.Layout.RepoName }},
	{Name: "hub", Resolve: func(c Ctx) string { return c.Layout.HubPath }},
}

// Build resolves every token in the registry against c.
func Build(c Ctx) map[string]string {
	values := make(map[string]string, len(registry))
	for _, token := range registry {
		values[token.Name] = token.Resolve(c)
	}
	return values
}
