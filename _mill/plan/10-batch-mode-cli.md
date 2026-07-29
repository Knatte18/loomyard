# Batch: batch-mode-cli

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: batch-mode-cli
number: 10
cards: 5
verify: go test -count=1 ./internal/codeintelcli/...
depends-on: [9]
```

## Batch Scope

Adds batch mode to all three verbs (`refs Foo Bar Baz`), per
`batch-mode-cli`: arg count is the shape discriminant — exactly 1
positional argument keeps today's legacy single-symbol envelope
unchanged (byte-for-byte, including the exit-code-2-on-ambiguous
behavior batch 9 just built), 2+ arguments switches to a per-symbol
array with a 4th `error` status and worst-outcome-wins exit codes 0–3.
Pure `internal/codeintelcli` work — no engine (`internal/codeintelengine`)
changes are needed, since batch mode is just "call the same
`References`/`Definition`/`Symbol` once per argument and aggregate."

**Naming gap this batch resolves, stated explicitly because
`_mill/discussion.md`'s `batch-mode-cli` decision describes each
per-symbol entry's shape (`{"symbol": ..., "status": ..., ...}`) but
never names the top-level array's own JSON key:** this batch uses
`"results"` as that key for all three verbs
(`{"ok":true,"results":[{"symbol":"Foo","status":"found",...}, ...]}`)
— a generic, verb-independent name, since the per-entry field that
actually varies by verb (`"references"`/`"definitions"`/`"symbols"`)
already lives one level down, inside each entry. No verb needs its own
top-level array key.

## Cards

### Card 38: Batch-status classification and the generic batch runner

- **Context:**
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `type batchStatus string` with
  `const (statusFound batchStatus = "found"; statusNotFound batchStatus =
  "not_found"; statusAmbiguous batchStatus = "ambiguous"; statusError
  batchStatus = "error")`, and
  `var statusRank = map[batchStatus]int{statusFound: 0, statusNotFound:
  1, statusAmbiguous: 2, statusError: 3}` (found=0 < not_found=1 <
  ambiguous=2 < error=3, per `batch-mode-cli`'s exact ranking). Add
  `func classifyLookupError(err error, resultsField string, results
  []codeintelengine.Reference) (batchStatus, map[string]any)`, shared by
  `refs`/`definition` batch mode: `err == nil` → `(statusFound,
  map[string]any{resultsField: referenceFields(results)})`;
  `errors.As(err, &ambiguous)` (`*codeintelengine.ErrAmbiguousSymbol`) →
  `(statusAmbiguous, map[string]any{"candidates": ambiguous.Candidates})`;
  `errors.Is(err, codeintelengine.ErrSymbolNotFoundSentinel)` →
  `(statusNotFound, nil)` — the only case with no extra fields, per
  `batch-mode-cli`'s "confirmed absent" framing, nothing more to report;
  anything else → `(statusError, map[string]any{"error": err.Error()})`
  — every non-`ErrSymbolNotFound` engine error (`ErrNoLanguage`,
  `ErrServerNotFound`, `ErrServerTimeout`, `ErrResolverUnsupported`, a
  toolchain-install failure) falls into this branch, since none of them
  mean "confirmed absent." Add the `symbol`-specific twin,
  `func classifySymbolError(err error, results
  []codeintelengine.SymbolMatch) (batchStatus, map[string]any)`: same
  `nil`/not-found/else structure but **no `ambiguous` branch at all** —
  `nil` → `(statusFound, map[string]any{"symbols":
  symbolMatchFields(results)})`; not-found → `(statusNotFound, nil)`;
  else → `(statusError, map[string]any{"error": err.Error()})` — per
  `batch-mode-cli`'s explicit "`symbol` has no `ambiguous` status and no
  exit code 2" rule. Add the generic runner,
  `func runBatch(ctx context.Context, out io.Writer, args []string,
  lookupOne func(symbol string) (batchStatus, map[string]any))`: build
  one entry per `arg` — `map[string]any{"symbol": arg, "status":
  string(status)}` merged with `lookupOne(arg)`'s returned fields map (a
  `nil` fields map, the not-found case, merges nothing extra); track the
  worst status seen via `statusRank`; call `output.Ok(out,
  map[string]any{"results": entries})` (return value discarded, same
  reasoning as `emitLookupResult`'s `output.Ok` call — it always returns
  0); then, only if the worst rank is non-zero,
  `clihelp.SetExit(ctx, statusRank[worst])` to override the exit code —
  when every symbol is `found`, the rank is already 0 and no override
  call is needed (or harmful; `SetExit` is a no-op for 0 regardless).
  `runBatch` has no opinion on how `lookupOne` resolves a symbol — that
  is each verb's own closure (cards 39, 40).
- **Commit:** `feat(codeintelcli): add batch-status classification and the generic batch runner`

### Card 39: Batch mode for `refs` and `definition`

- **Context:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/definition.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change both `refsCommand` and `definitionCommand`'s
  `Args:` from `cobra.ExactArgs(1)` to `cobra.MinimumNArgs(1)`. In each
  `RunE`, after the existing `cwd`/registry-resolution preamble (which is
  arg-count-independent and stays exactly where it is), branch on
  `len(args)`: `== 1` keeps the **exact** existing single-arg body
  unchanged (the `parseQuery`/`codeintelengine.References` (or
  `Definition`)/`emitLookupResult` sequence batch 9 already built — do
  not touch this branch's code at all, only its `if` guard is new).
  `> 1` calls `runBatch(ctx, out, args, func(symbol string) (batchStatus,
  map[string]any) { query, err := parseQuery(symbol); if err != nil {
  return statusError, map[string]any{"error": err.Error()} }; results,
  err := codeintelengine.References(ctx, codeintelengine.Options{Registry:
  registry, TargetDir: dir, Lang: lang, Query: query, Timeout: timeout});
  return classifyLookupError(err, "references", results) })` for `refs`
  (substituting `codeintelengine.Definition` and `"definitions"` for
  `definition`), then `return nil`. Each batch-mode symbol argument is
  still run through `parseQuery` exactly like the single-arg case — a
  batch argument may itself be a bare name or a `file:line:col` position,
  no capability is lost relative to single-arg mode, `batch-mode-cli`
  never restricts batch entries to names-only (that restriction is
  `symbol`-specific, per `symbol-semantics`, and unrelated to this
  arg-count discriminant).
- **Commit:** `feat(codeintelcli): add batch mode to refs and definition`

### Card 40: Batch mode for `symbol`

- **Context:**
  - `internal/codeintelengine/symbol.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `symbolCommand`'s `Args:` from
  `cobra.ExactArgs(1)` to `cobra.MinimumNArgs(1)`. Branch on
  `len(args)`: `== 1` keeps the exact existing single-arg body (batch 9's
  `codeintelengine.Symbol` + `symbolMatchFields` + plain
  `output.Err`-on-error sequence) unchanged. `> 1` calls `runBatch(ctx,
  out, args, func(symbol string) (batchStatus, map[string]any) { results,
  err := codeintelengine.Symbol(ctx, codeintelengine.Options{Registry:
  registry, TargetDir: dir, Lang: lang, Query: codeintelengine.Query{
  Symbol: symbol}, Timeout: timeout}); return classifySymbolError(err,
  results) })`, then `return nil`. Note in a comment above this branch
  that every batch entry is built directly from the raw `arg` string as
  `Query.Symbol` — exactly like the single-arg path, `symbol`'s batch
  mode never calls `parseQuery`/position-parsing either, so
  `lyx codeintel symbol foo.go:1:1 bar.go:2:2` treats both arguments as
  literal search strings, not positions, consistent across both arg-count
  shapes.
- **Commit:** `feat(codeintelcli): add batch mode to symbol`

### Card 41: Fix the pre-existing 2-arg test — batch mode is now valid syntax

- **Context:**
  - `internal/codeintelcli/cli.go`
- **Edits:**
  - `internal/codeintelcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestRunCLI_Refs_RequiresExactlyOneArg`'s `"two_args"`
  sub-test (`RunCLI(&out, []string{"refs", "one", "two"})`) currently
  asserts a **non-zero** exit code, on the premise that 2 args is an
  arg-count violation — that premise is false as of this batch: `refs
  one two` is now valid batch-mode syntax (two symbols, each almost
  certainly resolving to `not_found` against whatever registry/target-dir
  the test's ambient environment provides, since `"one"`/`"two"` are not
  real symbols — but the *call itself* is no longer a `cobra.MinimumNArgs`
  rejection). Rename the test to
  `TestRunCLI_Refs_RequiresAtLeastOneArg` and remove the `"two_args"` row
  from its table entirely — the `"bare"` row (0 args) is the only
  arg-count-violation case left standing, since `cobra.MinimumNArgs(1)`
  still rejects it exactly as `cobra.ExactArgs(1)` did. In its place, add
  a **new**, separately named test proving the opposite point:
  `TestRunCLI_Refs_TwoArgsIsBatchMode` — call `RunCLI(&out, []string{
  "refs", "one", "two", "--target-dir", t.TempDir()})` against an empty
  temp dir (so `DetectLanguage` fails identically for both symbols,
  keeping the assertion deterministic and gopls-independent) and assert
  the JSON envelope's top-level `"results"` array has exactly two
  entries, each with `"status":"error"` (an `ErrNoLanguage` failure is
  not "confirmed absent," so it classifies as `error`, not `not_found` —
  this is itself a useful regression pin distinguishing the two
  statuses) and `"symbol"` equal to `"one"`/`"two"` respectively, and
  that the process exit code is `3` (the worst-outcome rank for an
  all-`error` batch).
- **Commit:** `test(codeintelcli): retire the stale 2-args-is-invalid assumption, pin 2-args-is-batch-mode`

### Card 42: Batch-mode coverage — mixed outcomes, worst-outcome exit codes, `symbol` never ambiguous

- **Context:**
  - `internal/codeintelcli/cli.go`
- **Edits:**
  - `internal/codeintelcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `TestBatchRunner_WorstOutcomeWinsExitCode`, testing `runBatch` (card
  38) directly rather than through a live language server — build a
  `lookupOne` closure driven by a small table keyed on the input symbol
  string (e.g. `"a"` → `(statusFound, map[string]any{"references": []})`,
  `"b"` → `(statusNotFound, nil)`, `"c"` → `(statusAmbiguous,
  map[string]any{"candidates": []string{"x"}})`, `"d"` → `(statusError,
  map[string]any{"error": "boom"})`), so this test needs no engine call
  or subprocess at all. Table-drive four sub-tests, one per possible
  "worst status present" combination (`{"a"}` → exit 0; `{"a","b"}` →
  exit 1; `{"a","b","c"}` → exit 2; `{"a","b","c","d"}` → exit 3),
  asserting both the exit code (via `clihelp.NewExitContext`'s
  `exitState.Code()`, mirroring card 37's pattern) and that `"results"`
  has one entry per input symbol with the expected `"status"`. Add
  `TestClassifySymbolError_MultipleMatchesIsFoundNotAmbiguous` — call
  `classifySymbolError(nil, []codeintelengine.SymbolMatch{{Name: "Foo"},
  {Name: "FooBar"}})` (two matches, the multi-candidate case that
  *would* be `ambiguous` for `refs`/`definition`) and assert it returns
  `statusFound`, not `statusAmbiguous`, with both matches present in the
  `"symbols"` field — the actual regression this test exists to catch
  is a future edit that makes `classifySymbolError` reuse
  `classifyLookupError`'s ambiguity branch by mistake.
- **Commit:** `test(codeintelcli): cover batch worst-outcome exit codes and symbol's non-ambiguous classification`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelcli/...`. Every
card in this batch is pure CLI-layer logic with no engine change and no
subprocess spawn, so no integration tag is needed anywhere.
</content>
