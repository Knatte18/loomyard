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
- **Requirements:** In `emitLookupResult` (the refs/definition single-arg mapper), add `"resolution": "complete"` to the success-branch map so the emitted envelope becomes `{resultsField: referenceFields(results), "resolution": "complete"}`. Leave the `*ErrAmbiguousSymbol` (exit-2 `candidates`) branch and the fall-through `output.Err` (exit-1) branch untouched — the marker appears only on a successful lookup. In `classifyLookupError` (the refs/definition batch-mode classifier), add `"resolution": "complete"` to the `statusFound` branch's fields map alongside `resultsField`; leave the `statusAmbiguous`, `statusNotFound` (nil fields), and `statusError` branches unchanged. Do **not** touch `classifySymbolError`, `symbolMatchFields`, `symbolQuery`, or `symbolCommand` — `symbol` output stays byte-for-byte unchanged (a "complete" marker is meaningless for a fuzzy search). Update the `Long` help of both `refsCommand` and `definitionCommand` to name the machine-readable `"resolution":"complete"` field emitted on a successful single-arg lookup (and per-entry `found` in batch mode), so help stays accurate per the CLI/Cobra help-accuracy obligation. Note the two commands' existing trust-note wording **differs** — `refsCommand`'s `Long` says "The result set is complete and semantically resolved by the language server ...", while `definitionCommand`'s says "The result is semantically resolved by the language server, not text-matched ..." — so extend each command's own existing sentence to mention the field, rather than assuming a shared verbatim sentence across both.
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
- **Requirements:** Add `func buildOptions(registry codeintelengine.Registry, targetDir, worktreeRoot, lang string, query codeintelengine.Query, timeout time.Duration) codeintelengine.Options` to `cli.go`, returning `codeintelengine.Options{Registry: registry, TargetDir: targetDir, WorktreeRoot: worktreeRoot, Lang: lang, Query: query, Timeout: timeout}`. Replace all six `codeintelengine.Options{...}` literals with a `buildOptions(...)` call, passing the `worktreeRoot` local (resolved from `hubgeometry.Resolve(cwd)`'s `layout.WorktreeRoot`) into every one: refs single-arg (currently sets `WorktreeRoot`), refs batch closure (currently **omits** it), definition single-arg (sets it), definition batch closure (**omits**), symbol single-arg (sets it), symbol batch closure (**omits**). After this card, all three verbs' batch closures thread `WorktreeRoot` identically to their single-arg paths. **Also close the empty-`WorktreeRoot`-outside-a-hub gap** (see the "Supervised daemon anchoring outside a lyx hub" Shared Decision): today each verb leaves `worktreeRoot` `""` when `hubgeometry.Resolve(cwd)` fails (the supported outside-a-hub path that degrades to `BuiltinRegistry()`), which is harmless under native but, once batch 4 flips Go to supervised, makes `ensureSupervised` anchor its state/lock/socket at a cwd-**relative** `.lyx/codeintel/go/` path (littering/colliding across cwds). Fix it here, before the flip: in each verb's resolution block add an `else` branch that sets `worktreeRoot` to the **absolute** `--target-dir` (`filepath.Abs(dir)`, falling back to `dir` on error) so the supervised daemon anchors deterministically at `<abs target dir>/.lyx/codeintel/go/`, stable across cwds for the same `--target-dir`. Factor the resolution into a small testable helper (e.g. `func resolveWorktreeRoot(cwd, targetDir string) string` returning `layout.WorktreeRoot` when `hubgeometry.Resolve(cwd)` succeeds, else `filepath.Abs(targetDir)`) so it can be asserted without a live daemon; add a `cli_test.go` assertion that from a `t.TempDir()` with no `_lyx` (outside a hub) the resolved `worktreeRoot` is the absolute target dir, never `""`. The **DRY collapse itself is the regression guard**: once every one of the six sites is a single `buildOptions(..., worktreeRoot, ...)` call, a batch closure can no longer silently omit `WorktreeRoot` the way a hand-written literal did — that is what prevents the batch-mode drift, not the unit test. Add a `cli_test.go` assertion that documents `buildOptions`'s field-threading contract — call it once and assert the returned `Options` has every field set from the corresponding argument (`WorktreeRoot` included and non-empty) — but do **not** frame it as the batch-vs-single-arg regression guard: comparing `buildOptions(...)` to itself with the same args is tautological and cannot catch a call site passing the wrong local. (An end-to-end same-daemon assertion across the two arg shapes would need a live daemon and belongs to the integration tier, not this untagged unit test.)
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

`verify: go test ./internal/codeintelcli/...` runs the whole `codeintelcli` package untagged. It exercises the item-3 marker via direct `emitLookupResult`/`classifyLookupError` assertions (the package already has `TestEmitLookupResult_AmbiguousSymbolExitsTwo` and `TestClassifySymbolError_*` as precedent), the `buildOptions` field-threading contract test (card 8 — the batch-mode `WorktreeRoot` drift is guarded structurally by the DRY collapse of the six sites, not by this test), and the `inFileQuery`/flag-registration assertions (card 9) — all without a live language server (the existing `TestRunCLI_*_NoLanguageError` tests reach `ErrNoLanguage` against an empty `--target-dir`, and the new query-construction helpers are tested directly, exactly as `symbolQuery`/`parseQuery` are). `TestCommand_EveryCommandHasShort` (already in this package) re-confirms the CLI/Cobra `Short` invariant. End-to-end `--in-file` resolution against a real gopls is deferred to the out-of-band `-tags integration` engine suite (batch 2/4 territory), not this CLI gate.
