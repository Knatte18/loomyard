# Plan: Prune and consolidate the test suite (board first)

```yaml
task: Prune and consolidate the test suite (board first)
slug: prune-board-tests
approved: false
started: 20260622-174317
parent: main
root: ""
verify: null
```

## Batch Index

```yaml
batches:
  - number: 1
    name: board
    file: 01-board.md
    depends-on: []
    verify: go test ./internal/board/
  - number: 2
    name: worktree
    file: 02-worktree.md
    depends-on: [1]
    verify: go test -tags integration ./internal/worktree/
  - number: 3
    name: weft
    file: 03-weft.md
    depends-on: [1]
    verify: go test -tags integration ./internal/weft/
  - number: 4
    name: ide
    file: 04-ide.md
    depends-on: [1]
    verify: go test -tags integration ./internal/ide/
  - number: 5
    name: muxpoc
    file: 05-muxpoc.md
    depends-on: [1]
    verify: go test ./internal/muxpoc/
  - number: 6
    name: doc
    file: 06-doc.md
    depends-on: [1, 2, 3, 4, 5]
    verify: null
```

## Shared Decisions

### Decision: function-consolidation-only

- **Decision:** This task changes only test code. It folds clusters of single-shape
  top-level test funcs into table-driven tests with the **same assertions**, and drops
  only assertions/funcs proven redundant (a strict subset of, or fully owned by, another
  test). No production code changes; no new test dependencies; no wall-clock optimization.
- **Rationale:** Wall-clock is already solved (~3.5 s Tier 1); the target is func count
  and signal-to-noise. Prior tasks ran a no-drop superset guardrail and never pruned count.
- **Applies to:** all batches.

### Decision: layer-ownership

- **Decision:** When the same behavior is asserted at multiple layers, one layer owns it:
  unit tests own business logic + validation; facade tests own persistence wiring (files
  written, HealthCheck); CLI tests own the JSON envelope shape + exit codes + edge paths
  (not-initialized, unknown subcommand). Drop business-logic re-assertions from the
  facade/CLI layers, keeping exactly one assertion of each layer's unique value.
- **Rationale:** Removes the 3-way duplication (e.g. "remove missing errors" asserted in
  store + board + cli) without losing per-layer coverage.
- **Applies to:** board (store↔board↔cli), worktree (cli↔unit), weft (cli↔unit), ide
  (cli↔unit), muxpoc (cli↔state).

### Decision: preserve-names-as-subtests

- **Decision:** Each folded case keeps its original top-level func name as its `t.Run`
  subtest name (e.g. `name: "TestRenderDoneTask"`). Where a fold absorbs an inline check
  that had no top-level func name, that subtest gets a descriptive name and the doc
  name-map records which subtest now carries it. Tests that are *dropped* (not folded)
  are listed in the doc name-map with their drop justification.
- **Rationale:** Keeps `-list`/`=== RUN` traceability so the name-set diff against the
  frozen baseline stays auditable.
- **Applies to:** all batches.

### Decision: coverage-guardrail-with-frozen-baselines

- **Decision:** Per-package test-name baselines are frozen under
  `_mill/plan/baseline/<pkg>.txt` (captured before any edit). After each batch, the
  package's coverage % must be **≥ the floor below**, and every removed name from the
  baseline must map to a surviving subtest or to a documented drop. Coverage floors
  (statement coverage, the tier used for that package's verify):
  - `board` 62.5% (default build)
  - `worktree` 68.6% (`-tags integration`)
  - `weft` 64.6% (`-tags integration`)
  - `ide` 75.4% (`-tags integration`)
  - `muxpoc` 33.0% (default build)
- **Rationale:** Drops are allowed here (unlike prior superset-only tasks), so a name diff
  alone cannot catch a uniquely-covered assertion being dropped; coverage is the objective
  net. A measured drop below the floor blocks the batch until the lost assertion is
  restored or proven dead.
- **Applies to:** all batches; the floors are re-stated per batch in `## Batch Tests`.

### Decision: respect-test-build-and-package-constraints

- **Decision:** Folds must stay within one build-tag bucket (never merge a
  `//go:build integration` func with an untagged one) and within one package boundary
  (white-box `package <pkg>` vs black-box `package <pkg>_test`). A folded table that
  contains any `t.Setenv` or `os.Chdir`/`t.Chdir` member must set env/chdir at parent
  scope and run subtests **serially** — never add `t.Parallel` to such a group, and never
  parallelize a test that is serial today.
- **Rationale:** `t.Setenv` panics under `t.Parallel`; integration/unit tiers and
  white/black-box packages cannot see each other's symbols; preserving serial/parallel
  posture avoids flakiness.
- **Applies to:** all batches (specific serial members named per batch).

### Decision: counts-are-not-quotas

- **Decision:** Every `→ ~N` func-count estimate is an expected outcome, not a target. A
  batch stops folding/dropping where further reduction would lose a uniquely-covered
  assertion; a count above the estimate is not a failure.
- **Rationale:** Principle-driven prune (per discussion); reviewer must not treat a miss
  of the estimate as a defect.
- **Applies to:** all batches.

### Decision: path-and-link-invariants

- **Decision:** Test edits must not introduce raw `os.Getwd` or
  `git rev-parse --show-toplevel` (banned outside `internal/paths` and `cmd/lyx/main.go`,
  enforced by `internal/paths/enforcement_test.go`). `t.Chdir` is allowed. Do not change
  `internal/fslink` or `internal/lyxtest` helpers.
- **Rationale:** CONSTRAINTS.md path invariant + fslink directory-junction contract.
- **Applies to:** all batches (most relevant to worktree/weft junction tests).

## All Files Touched

- `docs/benchmarks/test-suite-timing.md`
- `internal/board/board_test.go`
- `internal/board/cli_test.go`
- `internal/board/config_test.go`
- `internal/board/init_test.go`
- `internal/board/render_test.go`
- `internal/ide/cli_test.go`
- `internal/ide/color_test.go`
- `internal/ide/menu_test.go`
- `internal/ide/spawn_test.go`
- `internal/muxpoc/cli_test.go`
- `internal/muxpoc/cmd_test.go`
- `internal/muxpoc/state_test.go`
- `internal/weft/config_test.go`
- `internal/weft/weft_integration_test.go`
- `internal/worktree/add_test.go`
- `internal/worktree/portals_test.go`
- `internal/worktree/weft_test.go`
