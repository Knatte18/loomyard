# Batch: poc-scaffold-gopackages

```yaml
task: 'Spike: structured Go reference/call-graph lookup (go/packages / gopls)'
batch: poc-scaffold-gopackages
number: 1
cards: 3
verify: go build ./tools/codeintel-poc/
depends-on: []
```

## Batch Scope

Delivers the throwaway harness skeleton and the **primary (in-process `go/packages` +
`go/types`) measurement arm**. After this batch, `go run ./tools/codeintel-poc -mode=refs
-symbol=<spec>` and `-mode=callers` return a structured reference / direct-caller list for a
symbol on this repo, and print separate **warm-up** (first package load) and **steady-state**
(subsequent same-process queries) timings. This is the load-bearing arm; batches 2–3 add the
comparison arms and consume this harness. Batch-local decision: the symbol spec format is
`<import-path>.<Name>` for funcs/types/vars and `<import-path>.<Type>.<Method>` for methods —
documented in `-help` and reused by every later mode.

## Cards

### Card 1: Harness skeleton + x/tools dependency

- **Context:**
  - `tools/sandbox/main.go`
  - `docs/research/session-fork-spike.md`
- **Edits:**
  - `go.mod`
  - `go.sum`
- **Creates:**
  - `tools/codeintel-poc/main.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `package main` at `tools/codeintel-poc/main.go` with a `main()`
  that parses flags via the stdlib `flag` package: `-mode` (string; one of `refs`, `callers`,
  and — added in later cards — `gopls-refs`, `gopls-cli-refs`, `callgraph`), `-symbol`
  (string, spec form `<import-path>.<Name>` or `<import-path>.<Type>.<Method>`), `-dir`
  (string, module root to analyze; default `.`), `-n` (int, number of steady-state query
  repeats; default 5), `-algo` (string, for callgraph mode; default `cha`), `-json` (bool,
  emit JSON). Include a `-help`/usage string documenting the symbol spec form and every mode.
  Define a `dispatch(mode string) error` switch that returns `fmt.Errorf("unknown mode %q",
  mode)` for unimplemented modes so later cards register their handlers by extending it. Add
  `golang.org/x/tools` to `go.mod` by running `go get golang.org/x/tools@latest && go mod
  tidy` (this also updates `go.sum`); the import is exercised starting in Card 2. The file
  must compile with `go build ./tools/codeintel-poc/` even before later modes exist (unknown
  modes error at runtime, not compile time). No `*_test.go`. This is throwaway per Shared
  Decision `no-production-module-conventions`.
- **Commit:** `chore(codeintel-poc): scaffold harness cli + add x/tools dep`

### Card 2: In-process go/packages reference finder

- **Context:**
  - `tools/codeintel-poc/main.go`
  - `internal/state/state.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/shuttleengine/engine.go`
- **Edits:**
  - `tools/codeintel-poc/main.go`
- **Creates:**
  - `tools/codeintel-poc/gopackages.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `tools/codeintel-poc/gopackages.go`, implement the `refs` mode. Add a
  `loadPackages(dir string) ([]*packages.Package, time.Duration, error)` that calls
  `packages.Load` with `Mode: packages.NeedName | NeedFiles | NeedSyntax | NeedTypes |
  NeedTypesInfo | NeedDeps | NeedImports | NeedModule` and `&packages.Config{Dir: dir, Tests:
  false}`, returning the packages plus the **warm-up** load duration; fail via
  `packages.PrintErrors`. Add a `resolveSymbol(pkgs, spec)` that finds the `types.Object` for
  the spec (func/type/var by `pkg.Types.Scope().Lookup`, method by looking up the receiver
  type then `types.NewMethodSet`/`LookupFieldOrMethod`). Add `findReferences(pkgs, obj)
  []token.Position` that scans every package's `TypesInfo.Uses` (and `Defs` for the
  definition site) for identifiers whose object is `obj`, converting each to a
  `token.Position` via the shared `token.FileSet`. The `refs` handler: load once (record
  warm-up), then run `findReferences` `-n` times recording each call's duration; report
  warm-up duration, per-query steady-state durations (min/median), the total reference count,
  and the list of `file:line:col` positions (JSON when `-json`). Register the handler in
  `main.go`'s `dispatch` switch. The three Context files are read only as realistic example
  targets to sanity-check the resolver against (a generic func, a high-fan-in package, an
  interface) — do not edit them.
- **Commit:** `feat(codeintel-poc): in-process go/packages reference finder`

### Card 3: In-process direct-caller (call-hierarchy) finder

- **Context:**
  - `tools/codeintel-poc/main.go`
  - `tools/codeintel-poc/gopackages.go`
- **Edits:**
  - `tools/codeintel-poc/main.go`
- **Creates:**
  - `tools/codeintel-poc/callers.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `tools/codeintel-poc/callers.go`, implement the `callers` mode
  (incoming direct callers, the "who calls this function" call-hierarchy slice). Reuse
  `loadPackages` and `resolveSymbol` from `gopackages.go`. Add `findDirectCallers(pkgs,
  obj) []CallerInfo` that walks each package's syntax with `ast.Inspect`, and for every
  `*ast.CallExpr` whose callee identifier resolves (via `TypesInfo.Uses`) to the target
  `obj`, records the **enclosing function** (`*ast.FuncDecl` / `*ast.FuncLit`) and the call
  site `token.Position`. A `CallerInfo` carries the enclosing function's qualified name and
  the call-site position. Report the same warm-up + steady-state timing shape as Card 2 and
  the deduplicated caller-function set with counts. Register the handler in `dispatch`.
  Document in a top-of-file comment that this is *direct* callers only (syntactic call sites
  resolved by type info); *transitive* callers are the callgraph arm (Card 5).
- **Commit:** `feat(codeintel-poc): in-process direct-caller finder`

## Batch Tests

`verify: go build ./tools/codeintel-poc/` compiles the harness (Go project — native runner,
no `PYTHONPATH=` prefix). Per Shared Decision `no-production-module-conventions` there are no
unit tests: the harness is disposable and is validated by being *run* in batch 3, not by a
test suite. The build check catches type errors in the `go/packages`/`go/types` usage, which
is the batch's only failure surface. Requires the `golang.org/x/tools` fetch from Card 1 to
have succeeded (Shared Decision `network-prerequisites`).
