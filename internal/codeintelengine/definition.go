// definition.go implements Definition, References's sibling: it shares the
// same detect->acquire->resolve pipeline (lookup, in refs.go) and differs
// only in which LSP method it calls once that pipeline has a position in
// hand — textDocument/definition instead of textDocument/references, per
// the plan's definition-semantics decision.

package codeintelengine

import "context"

// Definition resolves opts.Query against the language server for
// opts.TargetDir and returns the location(s) of its definition. It takes
// the identical Options/Query shape References does — a bare symbol name
// resolved via resolvePosition's existing ambiguity-collapsing
// workspace/symbol call (returning ErrAmbiguousSymbol on more than one
// candidate, exactly as it already does for References), or an explicit
// file:line:col position bypassing resolution entirely. Definition is a
// thin wrapper over the shared lookup pipeline, passing client.definition
// as the one LSP call the pipeline should make once a position is resolved
// — see lookup's own doc comment for the full step-by-step behavior.
func Definition(ctx context.Context, opts Options) ([]Reference, error) {
	return lookup(ctx, opts, func(ctx context.Context, client *lspClient, fileURI string, pos lspPosition) ([]lspLocation, error) {
		return client.definition(ctx, fileURI, pos)
	})
}
