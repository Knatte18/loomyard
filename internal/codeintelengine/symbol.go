// symbol.go implements Symbol, a dedicated engine entry point for
// workspace/symbol search. Unlike References/Definition (refs.go,
// definition.go), Symbol does not share the lookup pipeline and never calls
// resolvePosition at all — per the plan's symbol-semantics decision, it
// calls workspace/symbol directly and returns every candidate as a success,
// never collapsing multiple matches down to an ErrAmbiguousSymbol failure,
// since returning every match is the entire point of a symbol search.

package codeintelengine

import (
	"context"
	"errors"
	"time"
)

// SymbolMatch is one workspace/symbol search result: the symbol's display
// name, its LSP SymbolKind (a 1-indexed integer enum this package does not
// decode into named constants — decoding meaning belongs to whatever
// eventually displays it, the CLI layer, not the engine), and its
// declaration location. Line and Character are 1-based, matching
// Reference's existing display convention.
type SymbolMatch struct {
	Name      string
	Kind      int
	File      string
	Line      int
	Character int
}

// symbolFromClient runs workspace/symbol against client for query and maps
// every candidate to a SymbolMatch. It deliberately does not call
// resolvePosition and does not collapse multiple candidates to
// ErrAmbiguousSymbol — that ambiguity-collapsing behavior is exactly what
// Symbol must not inherit from References/Definition's resolution path.
// It takes no Options, only the bare values its logic needs, so it is
// callable from a test against a hand-built client with no real
// Options/registry/target-dir at all: TargetDir is filled in by Symbol
// itself, below, the only caller that actually has opts.TargetDir in
// scope.
func symbolFromClient(ctx context.Context, client *lspClient, lang string, entry Entry, query string, timeout time.Duration) ([]SymbolMatch, error) {
	if !client.supportsWorkspaceSymbol() {
		return nil, &ErrResolverUnsupported{Language: lang, Server: entry.Command[0]}
	}

	symbolCtx, symbolCancel := context.WithTimeout(ctx, timeout)
	defer symbolCancel()
	candidates, err := client.workspaceSymbol(symbolCtx, query)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, &ErrSymbolNotFound{Symbol: query}
	}

	// Every candidate becomes a result, unlike resolvePosition's
	// single-candidate-or-ambiguous handling — a symbol search caller wants
	// the full candidate set, not a forced disambiguation.
	matches := make([]SymbolMatch, len(candidates))
	for i, c := range candidates {
		matches[i] = SymbolMatch{
			Name:      c.Name,
			Kind:      c.Kind,
			File:      trimFileURI(c.Location.URI),
			Line:      c.Location.Range.Start.Line + 1,
			Character: c.Location.Range.Start.Character + 1,
		}
	}
	return matches, nil
}

// Symbol resolves opts.Query.Symbol via workspace/symbol against the
// language server for opts.TargetDir and returns every candidate match.
// Symbol's CLI argument handling (accepting only a plain search string,
// never a file:line:col bypass) is the CLI layer's concern, not this
// function's — opts.Query.Symbol here is just whatever string the caller
// supplies.
func Symbol(ctx context.Context, opts Options) ([]SymbolMatch, error) {
	lang, entry, err := DetectLanguage(opts.TargetDir, opts.Registry, opts.Lang)
	if err != nil {
		return nil, err
	}

	client, kind, err := acquireConnection(ctx, lang, entry, opts)
	if err != nil {
		// No deferred teardown needed here: acquireConnection already tears
		// down any partial connection itself on its own error path, exactly
		// as lookup relies on for References/Definition.
		return nil, err
	}

	timedOut := false
	defer func() {
		teardownConnection(client, kind, timedOut)
	}()

	matches, err := symbolFromClient(ctx, client, lang, entry, opts.Query.Symbol, opts.Timeout)
	if err != nil {
		if errors.Is(err, ErrServerTimeoutSentinel) {
			timedOut = true
		}
		// opts.TargetDir is only available here, not inside
		// symbolFromClient, so ErrSymbolNotFound's TargetDir field is filled
		// in at this one call site rather than threaded through as an extra
		// parameter symbolFromClient's test callers would otherwise have to
		// supply.
		var notFound *ErrSymbolNotFound
		if errors.As(err, &notFound) {
			notFound.TargetDir = opts.TargetDir
		}
		return nil, err
	}

	return matches, nil
}
