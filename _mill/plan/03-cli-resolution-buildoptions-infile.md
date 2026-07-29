# Batch: cli-resolution-buildoptions-infile

```yaml
task: "Give codeintel a persistent, session-long daemon"
batch: "cli-resolution-buildoptions-infile"
number: 3
cards: 3
verify: go test ./internal/codeintelcli/...
depends-on: [2]
```

## Batch Scope

This batch delivers all three of the task's CLI-layer changes in `internal/codeintelcli/cli.go`: item 3's flat `"resolution":"complete"` trust marker, the shared `buildOptions(...)` helper that threads `WorktreeRoot` uniformly through all six `Options` construction sites (item 5's prerequisite — without it batch-mode lookups would anchor a different daemon once dispatch flips to supervised), and item 4's `--in-file` flag on refs/definition. It depends on batch 2 because the `--in-file` card constructs `codeintelengine.Query{InFile: &codeintelengine.InFileQuery{...}}`, whose types batch 2 adds. Cards are ordered so card 8 (`buildOptions`) lands before card 9 (`--in-file`), which routes its new query form through `buildOptions`. This batch touches no engine files, so it never conflicts with batches 2 or 4.

## Cards

### Card 7: Add the `"resolution":"complete"` trust marker to refs/definition

- **Context:**
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
  - `internal/codeintelcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `emitLookupResult` (the refs/definition single-arg mapper), add `"resolution": "complete"` to the success-branch map so the emitted envelope becomes `{resultsField: referenceFields(results), "resolution": "complete"}`. Leave the `*ErrAmbiguousSymbol` (exit-2 `candidates`) branch and the fall-through `output.Err` (exit-1) branch untouched — the marker appears only on a successful lookup. In `classifyLookupError` (the refs/definition batch-mode classifier), add `"resolution": "complete"` to the `statusFound` branch's fields map alongside `resultsField`; leave the `statusAmbiguous`, `statusNotFound` (nil fields), and `statusError` branches unchanged. Do **not** touch `classifySymbolError`, `symbolMatchFields`, `symbolQuery`, or `symbolCommand` — `symbol` output stays byte-for-byte unchanged (a "complete" marker is meaningless for a fuzzy search). Update the `Long` help of both `refsCommand` and `definitionCommand` to name the machine-readable `"resolution":"complete"` field emitted on a successful single-arg lookup (and per-entry `found` in batch mode), so help stays accurate per the CLI/Cobra help-accuracy obligation; keep the wording consistent with the existing "result set is complete and semantically resolved" sentence already in those `Long` blocks.
- **Commit:** `feat(codeintelcli): add resolution:complete trust marker to refs/definition`

### Card 8: Extract the shared `buildOptions` helper threading `WorktreeRoot`

- **Context:**
  - `internal/codeintelengine/refs.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
  - `internal/codeintelcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func buildOptions(registry codeintelengine.Registry, targetDir, worktreeRoot, lang string, query codeintelengine.Query, timeout time.Duration) codeintelengine.Options` to `cli.go`, returning `codeintelengine.Options{Registry: registry, TargetDir: targetDir, WorktreeRoot: worktreeRoot, Lang: lang, Query: query, Timeout: timeout}`. Replace all six `codeintelengine.Options{...}` literals with a `buildOptions(...)` call, passing the `worktreeRoot` local (resolved from `hubgeometry.Resolve(cwd)`'s `layout.WorktreeRoot`) into every one: refs single-arg (currently sets `WorktreeRoot`), refs batch closure (currently **omits** it), definition single-arg (sets it), definition batch closure (**omits**), symbol single-arg (sets it), symbol batch closure (**omits**). After this card, all three verbs' batch closures thread `WorktreeRoot` identically to their single-arg paths. Add `cli_test.go` assertions: for each of refs/definition/symbol, `buildOptions` produces `Options` with the same non-empty `WorktreeRoot` (and same `Registry`/`TargetDir`/`Lang`/`Timeout`) for the single-arg and batch query shapes — a table over the three verbs comparing the two constructed `Options` values, guarding against the batch-mode `WorktreeRoot` regression.
- **Commit:** `refactor(codeintelcli): thread WorktreeRoot via shared buildOptions helper`

### Card 9: Add the `--in-file` exact-lookup flag to refs/definition

- **Context:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
  - `internal/codeintelcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `--in-file <path>` string flag (`StringVar`, default `""`) to `refsCommand` and `definitionCommand` only — **not** `symbolCommand`. Add a named helper `func inFileQuery(inFilePath, name string) (codeintelengine.Query, error)` (mirroring the existing `symbolQuery`/`parseQuery` pattern so the contract is unit-testable without a live server): it resolves `inFilePath` to an absolute path via `filepath.Abs` (the CLI layer owns path interpretation, exactly as `parseQuery` resolves `Pos.File`) and returns `codeintelengine.Query{InFile: &codeintelengine.InFileQuery{File: absPath, Name: name}}`. In both commands' `RunE`, when `inFile != ""`: build each positional argument's query via `inFileQuery(inFile, arg)` instead of `parseQuery(arg)` — so the positional is always treated as a bare name and never position-parsed, even when it looks like `file:line:col`. Compose with batch mode: `--in-file` must **not** reject 2+ positional args; each positional resolves independently in the same `<path>` through the existing `runBatch`/`classifyLookupError` per-entry contract (build the per-entry query via `inFileQuery(inFile, symbol)` inside the batch closure, routed through `buildOptions`). Update both commands' `Long` help with an `--in-file` section: one bare-name single-arg example (`lyx codeintel refs --in-file internal/foo/bar.go MyFunc`) and one batch example (`... Open Close` resolving both names in the same file). Add `cli_test.go` assertions: `inFileQuery` produces a `Query` with `InFile` set (absolute `File`, bare `Name`) and never `Pos`, even for a `file:line:col`-shaped name; a relative `--in-file` path resolves to absolute; the `--in-file` flag is registered on refs and definition but absent from `symbol`. If any exact-help-text golden/snapshot assertion exists in `cli_test.go`, update it to match the new `Long` text.
- **Commit:** `feat(codeintelcli): add --in-file exact symbol lookup to refs/definition`

## Batch Tests

`verify: go test ./internal/codeintelcli/...` runs the whole `codeintelcli` package untagged. It exercises the item-3 marker via direct `emitLookupResult`/`classifyLookupError` assertions (the package already has `TestEmitLookupResult_AmbiguousSymbolExitsTwo` and `TestClassifySymbolError_*` as precedent), the `buildOptions` single-vs-batch `WorktreeRoot` equality guard (card 8), and the `inFileQuery`/flag-registration assertions (card 9) — all without a live language server (the existing `TestRunCLI_*_NoLanguageError` tests reach `ErrNoLanguage` against an empty `--target-dir`, and the new query-construction helpers are tested directly, exactly as `symbolQuery`/`parseQuery` are). `TestCommand_EveryCommandHasShort` (already in this package) re-confirms the CLI/Cobra `Short` invariant. End-to-end `--in-file` resolution against a real gopls is deferred to the out-of-band `-tags integration` engine suite (batch 2/4 territory), not this CLI gate.
