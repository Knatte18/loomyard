# Batch: engine-documentsymbol-infile

```yaml
task: "Give codeintel a persistent, session-long daemon"
batch: "engine-documentsymbol-infile"
number: 2
cards: 4
verify: go test ./internal/codeintelengine/...
depends-on: []
```

## Batch Scope

This batch delivers item 4's **engine** half: a new `textDocument/documentSymbol` LSP method and the third `Query` form (`InFile`) that resolves a bare symbol name to a position exhaustively within one file, with no fuzzy matching and no column math. It is a DAG root (independent of proc and item 5). The external interface batch 3's CLI half consumes is the exported `InFileQuery` type and the `Query.InFile *InFileQuery` field — batch 3 constructs `Query{InFile: &InFileQuery{...}}`, so it depends on this batch for those types to exist and compile. All work is in `internal/codeintelengine` (`lspclient.go`, `refs.go`, `doc.go`) plus untagged unit tests. This batch touches `refs.go` and `doc.go`, which batch 4 also edits; batch 4 depends on this batch, so the two are ordered, never parallel.

## Cards

### Card 3: Add the `documentSymbol` LSP method and `lspDocumentSymbol` wire type

- **Context:**
  - `internal/codeintelengine/position.go`
- **Edits:**
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `lspclient.go`, add a wire type `lspDocumentSymbol` (following the existing `lsp*`-prefixed wire-type convention — `lspPosition`, `lspRange`, `lspLocation`) with fields `Name string` (json `name`), `Kind int` (json `kind`), `Range lspRange` (json `range`), `SelectionRange lspRange` (json `selectionRange`), and `Children []lspDocumentSymbol` (json `children`) — the hierarchical LSP `DocumentSymbol` shape (methods nest under types as children). Add a method `func (c *lspClient) documentSymbol(ctx context.Context, fileURI string) ([]lspDocumentSymbol, error)` that issues `textDocument/documentSymbol` via the existing `c.call(ctx, phase, method, params)` plumbing (phase `"documentSymbol"`, method `"textDocument/documentSymbol"`, params `map[string]any{"textDocument": map[string]any{"uri": fileURI}}`), then `json.Unmarshal`s the raw result into `[]lspDocumentSymbol` and returns it — the same plumbing/error-wrapping shape `references`/`definition`/`workspaceSymbol` already use. gopls returns the hierarchical `DocumentSymbol[]` shape; parsing that shape is sufficient (the LSP spec's alternative flat `SymbolInformation[]` shape is out of scope — note the assumption in the method doc comment). Update `doc.go`'s "speaks exactly seven methods" sentence and its enumerated method list to include `textDocument/documentSymbol` (now eight methods).
- **Commit:** `feat(codeintelengine): add textDocument/documentSymbol LSP method`

### Card 4: Add the `InFile` query form and the `documentSymbol` resolve branch

- **Context:**
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/symbol.go`
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `refs.go`, add an exported type `type InFileQuery struct { File string; Name string }` and a third mutually-exclusive field `InFile *InFileQuery` on the `Query` struct (alongside `Symbol` and `Pos`). Update `Query`'s doc comment to describe all three forms and the `resolvePosition` precedence (InFile checked first, then Pos, then Symbol; callers still set exactly one). In `resolvePosition`, add a new branch **before** the `opts.Query.Pos != nil` check: when `opts.Query.InFile != nil`, call `client.documentSymbol(ctx, "file://"+opts.Query.InFile.File)` (bounded by a fresh `context.WithTimeout(ctx, opts.Timeout)`, mirroring the workspace/symbol branch), then search the returned `[]lspDocumentSymbol` recursively — descending into each node's `Children` — for nodes whose `Name` exactly equals `opts.Query.InFile.Name`. Factor the recursive exact-name collection into a separate unexported helper (e.g. `func collectInFileMatches(syms []lspDocumentSymbol, name string) []lspDocumentSymbol`) so it is unit-testable with no transport. Map the match count exactly as the workspace/symbol branch maps its candidate count: zero matches → `&ErrSymbolNotFound{Symbol: opts.Query.InFile.Name, TargetDir: opts.TargetDir}`; more than one → `&ErrAmbiguousSymbol{Symbol: opts.Query.InFile.Name, Candidates: [...]}` where each candidate is `formatLocation(lspLocation{URI: "file://"+File, Range: match.SelectionRange})`; exactly one → return `"file://"+opts.Query.InFile.File` and the match's `SelectionRange.Start` used directly as the `lspPosition` (no round-trip through `toLSPPosition`/the byte-column `Position` type — use `SelectionRange` because it spans just the identifier, not the whole symbol body). Update `doc.go`'s resolver section to note the exhaustive-per-file `documentSymbol` resolve mode (`--in-file`) alongside the existing workspace/symbol resolver, contrasting it as fuzzy-free and cap-free.
- **Commit:** `feat(codeintelengine): resolve --in-file names via exhaustive documentSymbol search`

### Card 5: Unit-test `documentSymbol` parse and child recursion

- **Context:**
  - `internal/codeintelengine/lspclient.go`
- **Edits:**
  - `internal/codeintelengine/lspclient_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a test driving `lspClient.documentSymbol` over the existing fake-transport harness (`newPipeTransportPair()` + `newFakeServer()` + `server.readMessage(t)`/`server.respond(t, id, result)`), following the exact pattern of `TestLSPClient_ReferencesSendsIncludeDeclarationAndParsesResult`. The fake server responds to the `textDocument/documentSymbol` request with a hierarchical `DocumentSymbol[]` JSON payload containing at least one top-level symbol (e.g. a type) with a nested `children` entry (e.g. a method), each carrying `name`, `kind`, `range`, and `selectionRange`. Assert the method: sends the request to `textDocument/documentSymbol` with the correct `textDocument.uri`; parses top-level names/kinds/ranges; and preserves the `children` subtree (so the later recursion can reach nested methods).
- **Commit:** `test(codeintelengine): cover documentSymbol parse and child recursion`

### Card 6: Unit-test the `InFile` resolve branch and exact-match helper

- **Context:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/lspclient_test.go`
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelengine/refs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add tests for item 4's resolve logic to `refs_test.go`. Test the pure `collectInFileMatches` helper directly (no transport): a name matching a top-level symbol; a name matching a nested child (proving recursion); zero matches → empty slice; two same-name matches (e.g. a method named `Open` on two types) → both collected. Add at least one end-to-end `resolvePosition` test over the fake transport (as in `lspclient_test.go`/`daemonstate_test.go`) asserting: exactly one match returns the match's `SelectionRange.Start` as the position and `"file://"+File` as the URI; zero matches → error satisfying `errors.Is(err, ErrSymbolNotFoundSentinel)`; multiple matches → error satisfying `errors.Is(err, ErrAmbiguousSymbolSentinel)` whose `Candidates` are formatted `file:line:col` strings. Keep every case untagged and process-free.
- **Commit:** `test(codeintelengine): cover the InFile documentSymbol resolve branch`

## Batch Tests

`verify: go test ./internal/codeintelengine/...` runs the whole engine package untagged, exercising the new `documentSymbol` parse (card 5) and the `InFile` resolve branch + `collectInFileMatches` helper (card 6) over the package's established fake-transport harness (`newPipeTransportPair`/`newFakeServer`) — no real gopls, no spawn, satisfying the Test Tier Purity Invariant. The `leaf_enforcement_test.go` import-allowlist check also runs here and confirms the new code adds no disallowed import (all additions use stdlib types already in the package). Integration coverage of `--in-file` end-to-end against a real gopls (the same-name-in-two-types ambiguity case) is deferred to the out-of-band `-tags integration` suite per the Test Tier Purity Invariant and is not part of this batch's gate.
