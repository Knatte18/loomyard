# Plan: Spike: structured Go reference/call-graph lookup (go/packages / gopls)

```yaml
task: 'Spike: structured Go reference/call-graph lookup (go/packages / gopls)'
slug: codeintel-spike
approved: true
started: '20260717-150903'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches._

```yaml
batches:
  - number: 1
    name: poc-scaffold-gopackages
    file: 01-poc-scaffold-gopackages.md
    depends-on: []
    verify: go build ./...
  - number: 2
    name: poc-gopls-callgraph
    file: 02-poc-gopls-callgraph.md
    depends-on: [1]
    verify: go build ./...
  - number: 3
    name: measure-and-writeup
    file: 03-measure-and-writeup.md
    depends-on: [1, 2]
    verify: go build ./...
  - number: 4
    name: revert-and-verify
    file: 04-revert-and-verify.md
    depends-on: [3]
    verify: go build ./...
```

## Shared Decisions

### Decision: throwaway-discipline

- **Decision:** Every file under `tools/codeintel-poc/` and the repo-root `.lsp.json` is
  **disposable instrumentation**, committed on the task branch and **fully reverted in
  batch 4**. The single committed *product* is `docs/research/codeintel-spike.md`. This
  mirrors the `session-fork-spike` precedent (throwaway `tools/fork-poc/` + a
  `docs/research/*.md` findings doc).
- **Rationale:** The spike's value is the findings doc; the harness only exists to produce
  real numbers. Shipping the harness or its `golang.org/x/tools` dependency to `main` is out
  of scope (see `_mill/discussion.md` → Scope → Out).
- **Applies to:** all batches.

### Decision: no-production-module-conventions

- **Decision:** The harness is a standalone `tools/` `main.go` program (like `tools/sandbox`
  and `tools/deploy`) — **no** `<module>cli`/`<module>engine` split, **no** Cobra, **no**
  registration in `cmd/lyx/main.go`, and **no** `*_test.go` files. It is run manually via
  `go run ./tools/codeintel-poc ...` / a built binary, never via `go test`.
- **Rationale:** Avoids the CLI/Cobra Invariant, help-tree/registration/drift guards, and the
  Test Tier Purity Invariant entirely (see `_mill/discussion.md` → Constraints). A throwaway
  POC is explicitly blessed by CONSTRAINTS.md (the `muxpoc` precedent).
- **Applies to:** all batches.

### Decision: network-prerequisites

- **Decision:** Two network fetches are prerequisites: (a) `golang.org/x/tools` added to
  `go.mod` (**load-bearing** — the in-process arm cannot build without it); (b)
  `go install golang.org/x/tools/gopls@latest` (**best-effort** — the gopls-subprocess arm
  and CC-native LSP baseline need it). If the `gopls` install is blocked by network/policy,
  those two arms **degrade to a docs-based characterization** — an accepted, non-blocking
  outcome (see `_mill/discussion.md` → Technical context → Dependencies). If the `x/tools`
  fetch itself is blocked, the task cannot proceed and mill-go halts as stuck.
- **Rationale:** The in-process `go/packages` arm is the primary measurement and must build;
  the external-`gopls` arms are comparisons that the discussion explicitly allows to fall back
  to characterization.
- **Applies to:** poc-scaffold-gopackages, poc-gopls-callgraph, measure-and-writeup.

### Decision: measurement-artifacts-to-scratch

- **Decision:** All raw measurement output (timing logs, per-symbol reference/caller dumps,
  ground-truth grep results) is written under `.scratch/codeintel/` (gitignored — see
  `.gitignore` `**/.scratch/`). Only the distilled tables and verdict land in the committed
  findings doc. Never commit raw dumps.
- **Rationale:** `.scratch/` is the sanctioned ephemeral location; the findings doc carries
  the distilled result, matching `docs/benchmarks/` house style ("this file holds the
  numbers").
- **Applies to:** measure-and-writeup.

### Decision: findings-doc-is-the-deliverable

- **Decision:** `docs/research/codeintel-spike.md` is the primary deliverable. It carries: the
  adopt/defer/drop verdict per the `recommendation-rubric` in `_mill/discussion.md`; inline
  cost tables (warm-up-once-per-run tax + per-query steady-state, separately, per mechanism);
  an inline precision table (per benchmark symbol, false-negative/false-positive counts vs
  hand-verified ground truth); the CHA/RTA/VTA transitive sub-verdict with the callgraph roots
  recorded; and — if the verdict is adopt — a runnable how-to recipe (exact imports / call
  sequence / gopls request) verified by actually running it.
- **Rationale:** The user stated the doc is the more important artifact; it is what the
  follow-up adoption task consumes.
- **Applies to:** measure-and-writeup.

## All Files Touched

- `.lsp.json`
- `docs/research/codeintel-spike.md`
- `go.mod`
- `go.sum`
- `tools/codeintel-poc/callers.go`
- `tools/codeintel-poc/callgraph.go`
- `tools/codeintel-poc/gopackages.go`
- `tools/codeintel-poc/gopls.go`
- `tools/codeintel-poc/main.go`
