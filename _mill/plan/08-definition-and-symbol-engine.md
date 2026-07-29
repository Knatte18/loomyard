# Batch: definition-and-symbol-engine

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: definition-and-symbol-engine
number: 8
cards: 5
verify: go test -count=1 ./internal/codeintelengine/...
depends-on: [7]
```

## Batch Scope

Adds the two new engine entry points batch 9's CLI verbs will call:
`Definition` (shares `Query`/`resolvePosition` with `References`, per
`definition-semantics`, and reuses batch 7's `lookup` pipeline
verbatim) and `Symbol` (its own dedicated entry point that does **not**
call `resolvePosition` at all, per `symbol-semantics` — it calls
`workspace/symbol` directly and returns every candidate as success,
never collapsing to "ambiguous").

The external interfaces batch 9 consumes: `Definition(ctx
context.Context, opts Options) ([]Reference, error)` (new file
`definition.go`) and `Symbol(ctx context.Context, opts Options)
([]SymbolMatch, error)` (new file `symbol.go`), plus the new exported
`SymbolMatch` type.

## Cards

### Card 28: `lspClient.definition` — multi-shape `textDocument/definition` unmarshal

- **Context:**
  - `internal/codeintelengine/position.go`
- **Edits:**
  - `internal/codeintelengine/lspclient.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `func (c *lspClient) definition(ctx context.Context, fileURI string,
  pos lspPosition) ([]lspLocation, error)`, placed immediately after
  `references` (same section — this is its sibling LSP call). Body sends
  `textDocument/definition` via `c.call` with `{"textDocument": {"uri":
  fileURI}, "position": pos}` (no `context.includeDeclaration` — that
  parameter is `textDocument/references`-specific and does not exist on
  `textDocument/definition`'s request shape), then passes the raw result
  to a new helper, `parseDefinitionResult(raw json.RawMessage)
  ([]lspLocation, error)`, because the LSP spec's response type for this
  method is `Location | Location[] | LocationLink[] | null` — three
  distinct wire shapes one Go type cannot `json.Unmarshal` into
  directly. `parseDefinitionResult`: trim whitespace; `"null"` or empty
  returns `(nil, nil)` (zero definitions is a legitimate, non-error
  result at this layer — `definition.go`'s `Definition`, card 29, is
  where an empty result becomes a caller-visible outcome, not here).
  When the trimmed bytes start with `{`, unmarshal as a single
  `lspLocation` and return a one-element slice (the bare-`Location`
  case). Otherwise unmarshal as `[]json.RawMessage` and, per element,
  unmarshal into an anonymous probe struct carrying both possible
  per-element shapes at once — `URI string `+"`json:\"uri\"`"+`, Range
  lspRange `+"`json:\"range\"`"+`` (the `Location` fields) and
  `TargetURI string `+"`json:\"targetUri\"`"+`, TargetSelectionRange
  lspRange `+"`json:\"targetSelectionRange\"`"+`` (the `LocationLink`
  fields LSP recommends servers report a definition's precise identifier
  range in) — then build the resulting `lspLocation` from whichever pair
  is non-empty: `if probe.URI != "" { use URI/Range } else { use
  TargetURI/TargetSelectionRange }`. Document on the helper that `gopls`
  is known to report `Location[]` for `textDocument/definition` (not
  `LocationLink[]`), so the `LocationLink` branch is defensive per the
  LSP spec's stated possible shapes, not something this task can
  empirically confirm exercises against the one real server V1 ships
  with — the unit tests (card 31) are what actually cover it.
- **Commit:** `feat(codeintelengine): add lspClient.definition with multi-shape response parsing`

### Card 29: `Definition` — thin wrapper over the shared `lookup` pipeline

- **Context:**
  - `internal/codeintelengine/refs.go`
- **Creates:**
  - `internal/codeintelengine/definition.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the file's header doc comment mirroring
  `refs.go`'s own style: state that `Definition` is `References`'s
  sibling, differing only in which LSP method it calls once
  `lookup`'s shared detect→acquire→resolve pipeline has a position in
  hand, per `definition-semantics`. Add
  `func Definition(ctx context.Context, opts Options) ([]Reference,
  error) { return lookup(ctx, opts, func(ctx context.Context, client
  *lspClient, fileURI string, pos lspPosition) ([]lspLocation, error) {
  return client.definition(ctx, fileURI, pos) }) }`. `Definition` takes
  the identical `Options`/`Query` shape `References` does — a bare
  symbol name resolved via `resolvePosition`'s existing
  ambiguity-collapsing `workspace/symbol` call (returning
  `ErrAmbiguousSymbol` on more than one candidate, exactly as it already
  does for `References`), or an explicit `file:line:col` position
  bypassing resolution entirely — no new input-handling code is needed
  here at all, which is the whole point of sharing `lookup`.
- **Commit:** `feat(codeintelengine): add Definition sharing the lookup pipeline with References`

### Card 30: `Symbol` — dedicated entry point, `SymbolMatch`, and the `Kind` wire field

- **Context:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/position.go`
- **Edits:**
  - `internal/codeintelengine/lspclient.go`
- **Creates:**
  - `internal/codeintelengine/symbol.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `lspclient.go`, add `Kind int `+"`json:\"kind\"`"+``
  to the existing `symbolInformation` struct (currently `Name`+`Location`
  only) — this is the one wire field `workspace/symbol` already returns
  that nothing in the package captures today; LSP's `SymbolKind` is a
  1-indexed integer enum (`File`=1, `Module`=2, `Function`=12, …) and
  this package does not decode it into named constants, only passes the
  raw integer through — decoding meaning belongs to whatever eventually
  displays it (the CLI layer, batch 9), not the engine. No other change
  to `lspClient.workspaceSymbol` is needed — it already returns the full
  `symbolInformation` slice; adding a struct field with no method
  signature change is the only edit. In the new `symbol.go`, add
  `type SymbolMatch struct { Name string; Kind int; File string; Line
  int; Character int }` (1-based `Line`/`Character`, matching
  `Reference`'s existing display convention). Factor the
  post-connection candidate-handling logic into its own unexported
  helper — mirroring `finalizeConnection`'s extraction (batch 5, card
  18) precisely so it is independently testable against a fake
  transport, the same reason that extraction exists —
  `func symbolFromClient(ctx context.Context, client *lspClient, lang
  string, entry Entry, query string, timeout time.Duration)
  ([]SymbolMatch, error)`: **do not call `resolvePosition`** — that is
  the whole point of `symbol-semantics`'s decision, `Symbol` bypasses
  `resolvePosition`'s ambiguity-collapsing behavior entirely, since
  collapsing to "ambiguous" is wrong for a verb whose job *is* returning
  every match; if `!client.supportsWorkspaceSymbol()`, return
  `&ErrResolverUnsupported{Language: lang, Server: entry.Command[0]}`
  (same check `resolvePosition` already makes for `References`/
  `Definition`, duplicated here rather than shared, because
  `resolvePosition`'s other behavior — the position-vs-symbol branch, the
  ambiguity collapse — is exactly what `Symbol` must not inherit); call
  `client.workspaceSymbol(symbolCtx, query)` under its own
  `context.WithTimeout(ctx, timeout)`, timeout-detection-and-return
  identical to `lookup`'s pattern; zero candidates returns
  `&ErrSymbolNotFound{Symbol: query, TargetDir: ""}` (the `TargetDir`
  field is populated by `Symbol` itself, below, which is the only caller
  that actually has `opts.TargetDir` in scope — `symbolFromClient` takes
  no `Options`, only the bare values its logic needs, which is what
  makes it callable from a test with a hand-built client and no real
  `Options`/registry/target-dir at all); one-or-more candidates maps
  every single one into a `SymbolMatch` (`Name`, `Kind` straight from
  the wire struct; `File` via `trimFileURI(c.Location.URI)`;
  `Line`/`Character` via `c.Location.Range.Start.Line + 1`/
  `c.Location.Range.Start.Character + 1`, the same
  0-based-to-1-based promotion `toSortedReferences` already applies) and
  returns the full slice — **never** an `ErrAmbiguousSymbol`, regardless
  of how many candidates came back. Add
  `func Symbol(ctx context.Context, opts Options) ([]SymbolMatch,
  error)` as the thin public wrapper: `DetectLanguage`; `acquireConnection`
  (reused from `refs.go`, batch 7 — identical connection-acquisition
  semantics to `References`/`Definition`); `defer func() {
  teardownConnection(client, kind, timedOut) }()` with `timedOut :=
  false` exactly as `lookup` does; call `symbolFromClient(ctx, client,
  lang, entry, opts.Query.Symbol, opts.Timeout)`, and on an
  `ErrSymbolNotFound` result, overwrite its `TargetDir` field with
  `opts.TargetDir` before returning (the only place that value is
  available) — every other error or the success slice passes through
  unchanged; set `timedOut = true` when the result is an
  `ErrServerTimeoutSentinel`-matching error, mirroring every other
  connection-using function in this package. `Symbol`'s CLI argument
  handling (accepting only a plain search string, never a
  `file:line:col` bypass) is batch 9's concern, not this one —
  `opts.Query.Symbol` here is just whatever string the caller supplies.
- **Commit:** `feat(codeintelengine): add Symbol with its own non-collapsing engine entry point`

### Card 31: Tests for `lspClient.definition` and `Definition`

- **Context:**
  - `internal/codeintelengine/lspclient_test.go`
  - `internal/codeintelengine/refs_test.go`
- **Edits:**
  - `internal/codeintelengine/lspclient_test.go`
- **Creates:**
  - `internal/codeintelengine/definition_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** **In `lspclient_test.go`**, add table-driven coverage
  for `parseDefinitionResult` (or, if kept unexported and not
  independently table-friendly, three focused sub-tests) using the
  existing `newPipeTransportPair`/`fakeServer` harness through
  `client.definition(...)`, one fake-server response per LSP shape: (1)
  a bare `Location` object — one-element result; (2) a `Location[]` array
  with two elements — two-element result, field values preserved in
  order; (3) a `LocationLink[]` array (using `targetUri`/
  `targetSelectionRange` field names, not `uri`/`range`) — assert the
  parsed `lspLocation`s pull from the `target*` fields correctly; (4) a
  `null` response — `(nil, nil)`, no error. **In `definition_test.go`**
  (new, mirroring `refs_test.go`'s exact untagged/spawn-free style and
  header-comment convention), add
  `TestDefinition_NonExistentServerBinaryYieldsErrServerNotFound`, a
  direct copy of `refs_test.go`'s equivalent test but calling
  `Definition` instead of `References` — proving `Definition` goes
  through `acquireConnection`'s same error-mapping for the legacy path,
  since `lookup` is shared code and this is the cheapest possible
  regression proof that the sharing actually works for `Definition` too.
- **Commit:** `test(codeintelengine): cover lspClient.definition's multi-shape parsing and Definition's error path`

### Card 32: Tests for `Symbol`

- **Context:**
  - `internal/codeintelengine/refs_test.go`
  - `internal/codeintelengine/lspclient_test.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/symbol_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror `definition_test.go`'s
  `TestDefinition_NonExistentServerBinaryYieldsErrServerNotFound`
  pattern for `Symbol` first (same legacy-path regression proof — this
  one does exercise the full `Symbol` function, since it never reaches a
  connection at all). Then add the tests that are `Symbol`-specific and
  are the actual point of this card, calling `symbolFromClient` directly
  (card 30's extracted helper) against a hand-built client built over the
  `newPipeTransportPair`/`fakeServer` harness (mirror
  `TestLSPClient_InitializeCapturesCapabilities`'s shape) — not through
  `Symbol`'s full `DetectLanguage`/`acquireConnection` machinery, which
  would require a real spawn; `Symbol`'s interesting behavior is entirely
  in how `symbolFromClient` interprets `workspaceSymbol`'s result, and
  connection acquisition is already covered generically elsewhere: (1) a
  fake server returning **two** `workspace/symbol` candidates for the
  same query — assert `symbolFromClient` returns a two-element
  `[]SymbolMatch` (not an error, and specifically
  not `ErrAmbiguousSymbol` — assert `errors.Is(err,
  ErrAmbiguousSymbolSentinel)` is `false` on this path, since a bug that
  accidentally reintroduces `resolvePosition`-style collapsing is exactly
  the regression this test exists to catch) with `Kind`/`Name`/
  `File`/`Line`/`Character` all populated correctly from the fake
  server's response shape; (2) zero candidates — assert
  `errors.Is(err, ErrSymbolNotFoundSentinel)`; (3) `!
  client.supportsWorkspaceSymbol()` (a fake server whose `initialize`
  response omits `workspaceSymbolProvider`) — assert
  `errors.Is(err, ErrResolverUnsupportedSentinel)`, and that
  `workspaceSymbol` is never called at all (the fake server's read loop
  should see no `workspace/symbol` request arrive — assert this the same
  way `lspclient_test.go`'s existing tests prove a request was or was
  not sent, by having the fake server goroutine fail the test via
  `t.Errorf` if it unexpectedly receives one).
- **Commit:** `test(codeintelengine): cover Symbol's non-collapsing candidate handling`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...`. No
integration tag needed — every card in this batch is either pure
wire-shape parsing against a fake in-memory server or a
`newLSPClient`-with-a-fake-binary-name error path, neither of which
needs a real `gopls`.
</content>
