# Plan: Build internal/stencil: fill markdown prompt templates

```yaml
task: 'Build internal/stencil: fill markdown prompt templates'
slug: 'internal-stencil'
approved: false
started: '20260707-112325'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: stencil-leaf
    file: 01-stencil-leaf.md
    depends-on: []
    verify: go test ./internal/stencil/
```

## Shared Decisions

### Decision: leaf discipline — stdlib-only, domain-blind, I/O-free

- **Decision:** `internal/stencil` is a shared-infra *leaf*. It imports only the standard
  library (`bytes`, `fmt`, `sort`, `strings`, `text/template`, `text/template/parse`). It
  performs no file I/O (caller passes bytes), constructs no paths, exposes no cobra
  `Command()`/`RunCLI`, and never learns any domain word ("review", "cluster", "phase",
  "bulk"). The `Type`-discriminator idiom is a caller convention, not baked into the leaf.
- **Rationale:** matches the `internal/yamlengine` / `internal/output` / `internal/state`
  leaf pattern and the lyxtest Leaf Invariant. Keeps the CLI/Cobra and Sandbox-Coverage
  invariants inapplicable (stencil is never a registered module).
- **Applies to:** all batches.

### Decision: error style — plain `fmt.Errorf`, `%w` on wraps

- **Decision:** the unfilled-marker error is a plain `fmt.Errorf` (no sentinel/typed
  error); parse and execute failures are wrapped with `%w` (`fmt.Errorf("parse template:
  %w", err)`, `fmt.Errorf("execute template: %w", err)`).
- **Rationale:** leaf convention — `yamlengine`/`output` use plain errors; no consumer needs
  to branch on the error programmatically.
- **Applies to:** all batches.

### Decision: test style — pure, table-driven, black-box

- **Decision:** tests are pure and table-driven with no substrate, in a black-box
  `package stencil_test`, exercising only the exported `Fill`.
- **Rationale:** every scenario in the discussion's Testing section is observable through
  `Fill`; black-box matches `internal/output/output_test.go` and keeps the tests a contract
  check rather than an implementation mirror.
- **Applies to:** all batches.

## All Files Touched

- `docs/shared-libs/README.md`
- `docs/shared-libs/stencil.md`
- `internal/stencil/stencil.go`
- `internal/stencil/stencil_test.go`
