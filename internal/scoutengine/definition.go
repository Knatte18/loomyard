// definition.go implements Definition, References's sibling: it shares the same
// detect->acquire->resolve pipeline (lookup, in refs.go) and differs only in which LSP method it
// calls once that pipeline has a position in hand — textDocument/definition instead of
// textDocument/references, per the plan's definition-semantics decision.

package scoutengine

import "context"

// Definition resolves opts.Query against the language server for opts.TargetDir and returns the
// location(s) of its definition.
// It uses the same resolution pipeline as References, differing only in the LSP method called.
func Definition(ctx context.Context, opts Options) ([]Reference, error) {
	return lookup(ctx, opts, func(ctx context.Context, client *lspClient, fileURI string, pos lspPosition) ([]lspLocation, error) {
		return client.definition(ctx, fileURI, pos)
	})
}
