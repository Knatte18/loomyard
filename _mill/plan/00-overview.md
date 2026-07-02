# Plan: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant

```yaml
task: "Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant"
slug: "sandbox-suite-expand"
approved: false
started: "20260702-071523"
parent: "main"
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
    name: sandbox-suite-doc
    file: 01-sandbox-suite-doc.md
    depends-on: []
    verify: go test ./tools/sandbox/...
  - number: 2
    name: coverage-invariant
    file: 02-coverage-invariant.md
    depends-on: [1]
    verify: go test ./cmd/lyx/...
```

## Shared Decisions

### Decision: Coverage is declared by explicit `**Covers:**` tags, never inferred from prose

- **Decision:** Each `SANDBOX-SUITE.md` scenario that drives a specific lyx
  module declares it with a `**Covers:** <module>[, <module>...]` line, in the
  same bold-label style as the scenario's existing `**Goal:**`/`**Watch:**`/
  `**Verdict:**` lines. The coverage test parses ONLY these lines — it never
  greps scenario prose for module names. S0 (discovery) and S1 (hub
  orientation) drive no single module and carry **no `Covers:` line at all**
  (no `(discovery)` sentinel — a literal `(discovery)` token would trip the
  drift-guard assertion). The renamed S5 (error ergonomics) is cross-cutting
  and also carries no `Covers:` line.
- **Rationale:** module names ("config", "ide", "warp") appear incidentally in
  prose without the scenario exercising the module; substring-grepping would
  produce false positives and negatives. An explicit tag is the same
  "structured fact, not prose" discipline the Cobra guards use — tag *presence*
  is machine-checked, scenario *content* accuracy stays a review obligation.
- **Applies to:** all batches.

### Decision: Coverage is checked at module granularity, against the live cobra root

- **Decision:** The invariant checks whole modules, not subcommands. The set of
  registered modules is derived from `newRoot().Commands()` (skipping cobra's
  `help`/`completion`), exactly as `cmd/lyx/longlist_test.go` already does — not
  a separately hand-maintained list. A module counts as covered if any scenario
  tags it. The eight registered modules are `init`, `board`, `config`, `ide`,
  `muxpoc`, `weft`, `warp`, `selfreport`.
- **Rationale:** matches CONSTRAINTS.md's phrasing for the parallel guard and
  reuses the existing enumeration pattern. `init --undo` and `config --set` are
  flags on existing modules, not new registered commands, so they do not change
  the module set.
- **Applies to:** all batches.

### Decision: Allowlist excludes `muxpoc`, `ide`, `selfreport` (whole modules, with reasons)

- **Decision:** The coverage test's exclusion allowlist is exactly
  `{muxpoc, ide, selfreport}`, each with a one-line reason: `muxpoc` (PoC,
  slated for replacement by the mux module); `ide` (side-effect heavy — `spawn`
  opens a real VS Code window, `menu` is an interactive stdin picker);
  `selfreport` (`create` files a real GitHub issue). Coverage is module-level,
  so each is a whole-module exclusion, not a per-subcommand one.
- **Rationale:** reading the actual command implementations, every subcommand of
  all three is side-effect-heavy or throwaway end-to-end; under module-level
  granularity there is no partial-credit path, so whole-module exclusion is the
  only option that matches their behaviour.
- **Applies to:** batch `coverage-invariant`.

### Decision: The renumber is forward-only

- **Decision:** The current S6 (wrong-directory/error-ergonomics) is renamed to
  S5, closing the S4→S6 gap. New scenarios take S6 (subfolder init), S7 (weft
  lifecycle), S8 (warp introspection). Past run reports and the frozen
  `sandbox-cli-ergonomics` task keep their historical "S6" references — this
  document does NOT rewrite external references, only the embedded scheme.
- **Rationale:** the task specifies a forward-only renumber; mid-sequence
  insertion was never in scope.
- **Applies to:** batch `sandbox-suite-doc`.

### Decision: Go verify commands, no `PYTHONPATH=` prefix

- **Decision:** This is a Go repo; every `verify:` uses the native `go test`
  runner directly (`go test ./tools/sandbox/...`, `go test ./cmd/lyx/...`) with
  no `PYTHONPATH= ` prefix. The prefix rule applies only to Python/mill projects.
- **Rationale:** the `verify-not-isolated` validator check is language-conditional.
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/sandbox_coverage_test.go`
- `tools/sandbox/SANDBOX-SUITE.md`
