// tokenvocab.go defines the token vocabulary's core types (Ctx, Token) and the registry of
// always-resolvable tokens, plus Build, which resolves the whole registry into a flat map a
// template renderer (internal/stencil) can consume.

package tokenvocab

// Ctx carries the context a Token.Resolve needs.
type Ctx struct {
	// RepoName feeds the "repo" token.
	RepoName string
	// HubPath feeds the "hub" token.
	HubPath string
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
	{Name: "repo", Resolve: func(c Ctx) string { return c.RepoName }},
	{Name: "hub", Resolve: func(c Ctx) string { return c.HubPath }},
}

// Build resolves every token in the registry against c.
func Build(c Ctx) map[string]string {
	values := make(map[string]string, len(registry))
	for _, token := range registry {
		values[token.Name] = token.Resolve(c)
	}
	return values
}
