# Batch: cli-definition-and-symbol

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: cli-definition-and-symbol
number: 9
cards: 5
verify: go test -count=1 ./internal/codeintelcli/... ./cmd/lyx/...
depends-on: [8]
```

## Batch Scope

Adds the `definition` and `symbol` CLI verbs and, in the same batch,
retrofits the **existing** `refs` verb with the 0/1/2 exit-code contract
`_mill/discussion.md`'s `exit-code-contract-vs-envelope` decision
describes — today's shipped `refsCommand` maps every engine error
(including `ErrAmbiguousSymbol`) uniformly to `output.Err`/exit 1, so
the ambiguous-means-exit-2 behavior does not exist yet anywhere in this
codebase; this batch is where it is actually built, then reused by
`definition` (which shares the same ambiguity-producing
`resolvePosition` path). `symbol` never produces `ErrAmbiguousSymbol`
(per `symbol-semantics`) and keeps today's simple uniform-error-mapping
shape.

This batch is single-arg only — every verb still takes exactly one
positional argument (`cobra.ExactArgs(1)`, unchanged for `refs`, new but
identical for `definition`/`symbol`). Batch mode (`MinimumNArgs(1)`, the
per-symbol array shape, the 4th `error` status) is batch 10, layered on
top of what this batch establishes.

## Cards

### Card 33: The 0/1/2 exit-code contract — shared helper, retrofit `refs`

- **Context:**
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `func emitLookupResult(ctx context.Context, out io.Writer, resultsField
  string, results []codeintelengine.Reference, err error)`: if `err !=
  nil`, first check `var ambiguous *codeintelengine.ErrAmbiguousSymbol;
  if errors.As(err, &ambiguous)` — on match, call `output.Ok(out,
  map[string]any{"candidates": ambiguous.Candidates})` (its return value
  discarded — `output.Ok` always returns `0`, which would be a no-op
  through `clihelp.SetExit` anyway) then `clihelp.SetExit(ctx, 2)` as a
  **separate** call to force the exit code, exactly matching
  `exit-code-contract-vs-envelope`'s "`output.Ok`... but the CLI layer
  overrides the process exit code to 2 via a direct
  `clihelp.SetExit(ctx, 2)` call after emitting" wording; return. On any
  other non-nil error, keep today's exact behavior:
  `clihelp.SetExit(ctx, output.Err(out, err.Error())); return` — no
  other error type gets special-cased, `ErrSymbolNotFound` and every
  other engine error already fall through correctly to `output.Err`'s
  hardcoded exit 1, which is already the design's "not found" contract
  value, per the decision's own "found/not-found fall out of the
  existing `output.Ok`/`output.Err` behavior for free" reasoning. On a
  nil error, `clihelp.SetExit(ctx, output.Ok(out, map[string]any{resultsField:
  referenceFields(results)}))` — `resultsField` is the caller-supplied
  JSON key (`"references"` for `refs`, `"definitions"` for `definition`,
  card 34), so this one helper serves both verbs' identical result
  shape. Retrofit `refsCommand`'s `RunE`: replace the existing
  `results, err := codeintelengine.References(ctx, opts); if err != nil {
  clihelp.SetExit(ctx, output.Err(out, err.Error())); return nil };
  clihelp.SetExit(ctx, output.Ok(out, map[string]any{"references":
  referenceFields(results)}))` block with `results, err :=
  codeintelengine.References(ctx, opts); emitLookupResult(ctx, out,
  "references", results, err); return nil`. `refsCommand`'s `Long`
  doc-comment example prose is unaffected — no CLI surface change, only
  the exit code on an ambiguous bare-symbol query changes from `1` to
  `2`, with the response body's shape also changing from an error-string
  message to `{"ok":true,"candidates":[...]}` on that one path.
- **Commit:** `feat(codeintelcli): add the 0/1/2 exit-code contract via emitLookupResult, retrofit refs`

### Card 34: `definition` verb

- **Context:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/definition.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `definitionCommand() *cobra.Command`, placed
  immediately after `refsCommand` and structurally identical to it (same
  three flags — `--target-dir`, `--lang`, `--timeout` — same `cwd`/
  registry-resolution preamble, same `parseQuery(args[0])` call, same
  `Args: cobra.ExactArgs(1)`): `Use: "definition
  <symbol|file:line:col>"`, non-empty `Short` (e.g. "show the
  definition of a symbol or source position"), and a `Long` mirroring
  `refs`'s exact structure and both example lines (`lyx codeintel
  definition MyFunction` / `lyx codeintel definition
  internal/foo/bar.go:42:8`) — CONSTRAINTS.md's CLI/Cobra Invariant
  requires concrete examples in `Long` for every self-discoverable
  command. Call `codeintelengine.Definition(ctx, opts)` (not
  `References`) and finish with `emitLookupResult(ctx, out,
  "definitions", results, err)` (card 33's helper, `resultsField =
  "definitions"` per `batch-mode-cli`'s field-naming decision, which
  names the single-arg field too — "definitions" is the array key for
  both shapes, this batch just doesn't have an array-of-results-per-symbol
  yet). Register it: `cmd.AddCommand(definitionCommand())` in `Command()`,
  alongside the existing `cmd.AddCommand(refsCommand())` line. In that
  same `Command()` function, update the parent `codeintel` command's
  `Short` — currently "code intelligence lookups (references) across
  supported languages", which under-describes the module the moment
  `definition`/`symbol` exist — to something naming all three
  capabilities (e.g. "code intelligence lookups (references,
  definitions, symbol search) across supported languages"), per
  CONSTRAINTS.md's CLI/Cobra Invariant help-accuracy obligation. Also
  wire `opts.WorktreeRoot` (the `Options` field batch 7's card 26 added)
  in every verb's `opts` construction (`refsCommand`, `definitionCommand`,
  and `symbolCommand` in card 35): `opts.WorktreeRoot = layout.WorktreeRoot`
  when the same `hubgeometry.Resolve(cwd)` call already made for the
  `servers.yaml` overlay resolution succeeds, left as the zero value
  `""` otherwise — this makes card 26's `Options.WorktreeRoot` doc
  comment ("The CLI layer populates it from a resolved
  `hubgeometry.Layout.WorktreeRoot` when available") actually true the
  moment it lands, rather than a promise no card fulfills; the value has
  no observable effect in V1 (only `supervised` reads it, and no V1
  registry entry selects `supervised`), so this is a forward-compatibility
  wire, not new user-visible behavior.
- **Commit:** `feat(codeintelcli): add the definition verb, update parent Short, wire WorktreeRoot`

### Card 35: `symbol` verb

- **Context:**
  - `internal/codeintelengine/symbol.go`
  - `internal/codeintelcli/cli.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `symbolCommand() *cobra.Command`: same
  `--target-dir`/`--lang`/`--timeout` flags and `cwd`/registry-resolution
  preamble as `refsCommand`, but **do not call `parseQuery`** — per
  `symbol-semantics`, `symbol`'s positional argument is always a plain
  search string, never position-parsed, so build
  `opts.Query = codeintelengine.Query{Symbol: args[0]}` directly. `Use:
  "symbol <query>"`, non-empty `Short` (e.g. "search workspace symbols by
  name"), `Long` stating plainly that unlike `refs`/`definition`, the
  argument is always treated as a literal search string — even one that
  happens to look like `file:line:col` — with one example
  (`lyx codeintel symbol MyFunction`). Call `codeintelengine.Symbol(ctx,
  opts)`, which returns `[]codeintelengine.SymbolMatch`, not
  `[]codeintelengine.Reference` — add a small local
  `symbolMatchFields(matches []codeintelengine.SymbolMatch)
  []map[string]any` (mirroring `referenceFields`'s exact shape/style)
  producing `{"name": ..., "kind": ..., "file": ..., "line": ...,
  "character": ...}` per match. On error, `clihelp.SetExit(ctx,
  output.Err(out, err.Error()))` — **do not** call `emitLookupResult`
  here; `Symbol` never returns `*ErrAmbiguousSymbol` (per
  `symbol-semantics`, it has no ambiguous state), so the simple
  uniform-error-mapping `refsCommand` used before card 33's retrofit is
  the correct, sufficient shape for this verb, and reusing
  `emitLookupResult` would not even type-check against `[]SymbolMatch`
  anyway. On success, `clihelp.SetExit(ctx, output.Ok(out,
  map[string]any{"symbols": symbolMatchFields(results)}))`. Register it:
  `cmd.AddCommand(symbolCommand())`.
- **Commit:** `feat(codeintelcli): add the symbol verb`

### Card 36: Update the pinned help-tree subcommand set

- **Context:**
  - `internal/codeintelcli/cli.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestHelpTree_VerbModuleSubcommands`'s `tests`
  table, change the `"codeintel"` entry's `wantSubs` from
  `[]string{"refs"}` to `[]string{"refs", "definition", "symbol"}` — the
  CLI/Cobra Invariant's registration guard (`registration_test.go`)
  needs no change (it checks package-level `Command()` wiring, which is
  unaffected — `codeintelcli.Command()` is already registered in
  `cmd/lyx/main.go`, only its own subcommand tree grew), and
  `longlist_test.go` needs no change either (it checks module names, not
  subcommand names). No other pinned set in `cmd/lyx/*_test.go`
  references codeintel's subcommand list.
- **Commit:** `test(lyx): pin definition and symbol in the codeintel help tree`

### Card 37: CLI tests for `definition`/`symbol` and the exit-code-2 path

- **Context:**
  - `internal/codeintelcli/cli.go`
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/definition.go`
  - `internal/codeintelengine/symbol.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `internal/codeintelcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestCommand_EveryCommandHasShort` (existing,
  generic tree walk) automatically covers the two new commands with no
  edit needed — leave it untouched, its passing is itself part of this
  card's proof. Add four new tests, mirroring
  `TestRunCLI_Refs_NoLanguageError`'s exact `t.Chdir(t.TempDir())` +
  empty-target-dir pattern: (1)
  `TestRunCLI_Definition_NoLanguageError` — same assertions as the `refs`
  version, substituting `"definition"` for the subcommand and checking
  the error message still mentions `"no language detected"`. (2)
  `TestRunCLI_Symbol_NoLanguageError` — same, for `"symbol"`. (3)
  `TestRunCLI_Symbol_TreatsFileLineColArgumentAsLiteralSearchString` — call
  `RunCLI(&out, []string{"symbol", "foo.go:1:1", "--target-dir",
  emptyTargetDir})` and assert the resulting `ErrNoLanguage` envelope's
  error message contains the **literal** string `"foo.go:1:1"` (proving
  the argument was passed through as `Query.Symbol` unparsed, not
  silently swallowed as a position) — this is the one behavior specific
  to `symbol`'s CLI layer this card must pin, since nothing else in the
  test suite would catch a regression that accidentally re-enables
  `parsePosition` for this verb. (4)
  `TestEmitLookupResult_AmbiguousSymbolExitsTwo` — test
  `emitLookupResult` directly rather than through the full `RunCLI` tree
  (`cli_test.go` is `package codeintelcli`, the same package
  `emitLookupResult` is defined in, so a direct call is legal, and is the
  right layer for this: reaching `ErrAmbiguousSymbol` through a live
  `refs`/`definition` call would require a real language server, which
  this file's header comment explicitly keeps out of scope). Build
  `ambiguousErr := &codeintelengine.ErrAmbiguousSymbol{Symbol: "Foo",
  Candidates: []string{"a.go:1:1", "b.go:2:2"}}`; seed a context via
  `ctx, es := clihelp.NewExitContext(context.Background())` (the
  package's own exported exit-state seam); call `emitLookupResult(ctx,
  &out, "references", nil, ambiguousErr)`; assert `es.Code() == 2`; and
  parse `out`'s single JSON line, asserting `"ok":true` and a
  `"candidates"` field equal to `["a.go:1:1", "b.go:2:2"]`. Add a second,
  twin case in the same test (table-driven, one row per `resultsField`)
  proving the non-ambiguous path still behaves as `refsCommand` did
  before card 33's retrofit: `emitLookupResult(ctx2, &out2, "definitions",
  nil, &codeintelengine.ErrSymbolNotFound{Symbol: "Bar", TargetDir:
  "/tmp"})` yields `es2.Code() == 1` and an `"ok":false` envelope whose
  `"error"` field mentions `"Bar"` — the not-found case must still fall
  through to plain `output.Err`, not be swept into the ambiguous branch.
- **Commit:** `test(codeintelcli): cover definition, symbol, and the ambiguous exit-2 path`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelcli/...
./cmd/lyx/...` — `cmd/lyx/...` because card 36 edits a pinned set that
lives there. This is the first batch where `cmd/lyx`'s own tests are
part of a `verify:` scope; every other `cmd/lyx/*_test.go` guard
(`registration_test.go`, `longlist_test.go`, `sandbox_coverage_test.go`,
`drift_test.go`) runs in the same `go test ./cmd/lyx/...` invocation and
must stay green, since none of their pinned expectations change here.
</content>
