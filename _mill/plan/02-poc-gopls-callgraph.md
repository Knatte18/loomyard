# Batch: poc-gopls-callgraph

```yaml
task: 'Spike: structured Go reference/call-graph lookup (go/packages / gopls)'
batch: poc-gopls-callgraph
number: 2
cards: 2
verify: go build ./tools/codeintel-poc/
depends-on: [1]
```

## Batch Scope

Adds the two remaining measurement arms onto the batch-1 harness: (a) the **`gopls`
held-open LSP subprocess** arm (the warm-server comparison, plus a cold `gopls` CLI path for
the cold-per-call penalty), and (b) the **`callgraph` CHA/RTA/VTA transitive** arm. Both are
new `-mode` handlers registered in the existing `dispatch` switch and reuse the batch-1 flag
parsing and `loadPackages`. Batch-local decision: the gopls arm speaks the minimum LSP
JSON-RPC needed (`initialize`, `initialized`, `textDocument/references`) over stdio and holds
the process open across `-n` queries to measure warm steady-state — it does **not** implement
full LSP. Consumed by batch 3, which runs every arm to gather numbers.

## Cards

### Card 4: gopls held-open LSP subprocess arm

- **Context:**
  - `tools/codeintel-poc/main.go`
  - `tools/codeintel-poc/gopackages.go`
- **Edits:**
  - `tools/codeintel-poc/main.go`
- **Creates:**
  - `tools/codeintel-poc/gopls.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `tools/codeintel-poc/gopls.go`, implement two modes. **`gopls-refs`**
  (warm): spawn `gopls` (from `$PATH`) as a subprocess speaking LSP JSON-RPC over stdin/stdout
  using the standard `Content-Length:`-framed message envelope; send `initialize`
  (rootUri = the `-dir` module root, default workspace capabilities), `initialized`, then hold
  the process open and issue `textDocument/references` for the target symbol's position `-n`
  times, measuring the **warm-up** (spawn + `initialize` + first query, since gopls loads the
  package graph lazily on first request) separately from **steady-state** (subsequent
  queries in the same live process); shut down cleanly (`shutdown` + `exit`). Resolve the
  symbol's file/line/character position by reusing batch-1's `resolveSymbol` + `token.Position`
  (LSP positions are 0-based UTF-16; convert). **`gopls-cli-refs`** (cold baseline): shell out
  to `gopls references <file>:<line>:<col>` once **per query** (`-n` invocations, each a fresh
  process) to capture the cold-per-call penalty for comparison. Both report the same warm-up +
  steady-state timing shape as batch 1 and the returned position set. If `gopls` is not found
  on `$PATH`, exit non-zero with a clear message naming the `go install
  golang.org/x/tools/gopls@latest` prerequisite (per Shared Decision `network-prerequisites`
  the *measurement* card, not this build, handles the docs-fallback). Register both handlers
  in `dispatch`.
- **Commit:** `feat(codeintel-poc): gopls held-open LSP + cold-cli reference arms`

### Card 5: CHA/RTA/VTA callgraph transitive arm

- **Context:**
  - `tools/codeintel-poc/main.go`
  - `tools/codeintel-poc/gopackages.go`
- **Edits:**
  - `tools/codeintel-poc/main.go`
- **Creates:**
  - `tools/codeintel-poc/callgraph.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `tools/codeintel-poc/callgraph.go`, implement the `callgraph` mode
  selected by `-algo` (`cha` | `rta` | `vta`). Build SSA via
  `golang.org/x/tools/go/ssa/ssautil.AllPackages` (with `ssa.InstantiateGenerics`) over the
  batch-1-loaded packages and `prog.Build()`. For `cha`: `cha.CallGraph(prog)`. For `rta`:
  collect **seed roots** — `main.main` of `cmd/lyx` plus every package `init` and every
  reachable `func TestMain`/exported test entry if present — and call `rta.Analyze(roots,
  true)`; expose the exact root set used in the output. For `vta`: build an initial CHA graph
  then `vta.CallGraph(allFuncsReachable, chaGraph)` (per the `go/callgraph/vta` doc), also
  reporting roots. For the target function `obj`, walk incoming edges transitively to produce
  the **transitive caller set** (deduplicated function set) and report: the algorithm, the
  root set used, the caller-set size, and the SSA-build + analysis durations separately. When
  the target is a method reachable only through an interface, this is exactly where the three
  algorithms diverge — the output must make the per-algorithm set difference inspectable
  (e.g. emit the caller sets so batch 3 can diff them). Register the handler in `dispatch`.
- **Commit:** `feat(codeintel-poc): CHA/RTA/VTA callgraph transitive-caller arm`

## Batch Tests

`verify: go build ./tools/codeintel-poc/` (Go native runner, no `PYTHONPATH=` prefix)
compiles the two new arms against the batch-1 harness. No unit tests per Shared Decision
`no-production-module-conventions`; the arms are exercised for real in batch 3. The build
check is the batch's failure surface (LSP envelope struct correctness, SSA/callgraph API
usage). `gopls` availability is not required to *build* card 4 — only to *run* it in batch 3.
