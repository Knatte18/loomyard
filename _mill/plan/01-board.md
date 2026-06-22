# Batch: board

```yaml
task: Prune and consolidate the test suite (board first)
batch: board
number: 1
cards: 5
verify: go test ./internal/board/
depends-on: []
```

## Batch Scope

Prune `internal/board` (the worst offender, 61 top-level funcs → ~40) and establish the
table-driven + layer-ownership pattern the other four packages follow. All eight test
files are `package board_test` (black-box), all untagged (Tier 1), sharing one func
namespace, so they are edited together in one batch. This batch sets the conventions for
folded subtest naming and cross-layer dedup that batches 2–5 reuse. `layer_test.go` and
`task_test.go` are already clean and are **not** edited. `internal/board/boardtest` is out
of scope and untouched. Coverage floor: **62.5%** (default build).

## Cards

### Card 1: Fold render_test.go into table-driven tests

- **Context:**
  - `internal/board/render.go`
  - `internal/board/layer.go`
  - `_mill/plan/baseline/board.txt`
- **Edits:**
  - `internal/board/render_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Reduce render_test.go from 16 top-level funcs to ~5 with identical
  assertions. (a) Fold the single-shape `board.Render()` assertions —
  `TestRenderDependencies`, `TestRenderSpecialBucketTask`, `TestRenderIsolatedTask`,
  `TestRenderTaskIDFormatting`, `TestRenderBrief`, `TestRenderMissingDependency`,
  `TestRenderStatusVariants`, `TestRenderLayerBuckets` — into one (or two) table-driven
  funcs keyed by task-shape → expected `Home.md` substrings, each case named after its
  original func. (b) **Drop** `TestRenderTaskStatus` — it asserts `[test-task] [active]`,
  a strict subset of `TestRenderStatusVariants` (which covers `active`); record the drop
  in this batch's name-map notes. (c) Fold `TestRenderSingleTask` and
  `TestRenderOrphanDetection` into one proposal-key table (body→proposal key present;
  no-body / second-render-without-body→absent), names preserved. (d) Fold
  `TestRenderExtendedTitle` and `TestRenderSidebarBlanks` into one sidebar table, keeping
  one assertion that `Render` wires `board.ExtendedTitle` into `_Sidebar.md`. (e) Keep
  `TestRenderToDisk` (disk I/O + orphan-file cleanup), `TestRenderCustomOutputs` (custom
  Home filename + proposal prefix), and `TestRenderEmptyTaskList` as distinct funcs.
  Preserve the `getKeys` helper. No assertion may be lost except the proven-subset drop
  in (b). Note: `TestRenderDeferredTask` is a new row in the shape table (not folded from
  baseline) covering the deferred-task bucket path; included to preserve coverage floor.
- **Commit:** `test(board): fold render_test into table-driven tests`

### Card 2: Fold cli_test.go to a JSON-contract table + error table

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/store_test.go`
  - `_mill/plan/baseline/board.txt`
- **Edits:**
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Reduce cli_test.go from 8 funcs to ~3, asserting only the CLI's
  unique value (JSON envelope shape + exit codes), not business logic owned by
  store_test.go. Fold the happy-path verbs `TestCLIUpsertTask`, `TestCLIListTasks`,
  `TestCLIGetTask`, `TestCLISetPhase`, `TestCLIRerender` into one table-driven
  `TestCLIContract` (each case named after its original func) asserting exit 0 + the
  `ok=true` envelope + the verb's distinctive field (e.g. `task`, `tasks[].layer`/
  `has_proposal`, Home.md written for rerender). Keep an error/edge table folding
  `TestCLIGetNonexistentTask` (null task), `TestCLIRemoveNonexistentTask` (exit 1 +
  error), `TestCLINotInitialized` (exit 1 + "not initialized"). All tests use
  `t.Setenv("BOARD_SKIP_GIT","1")` + `seedCwd`/`t.Chdir` and stay **serial** (no
  `t.Parallel`). Preserve the `seedCwd` and `runCLI` helpers.
- **Commit:** `test(board): fold cli_test into contract + error tables`

### Card 3: Dedup board_test.go against the store/cli layers

- **Context:**
  - `internal/board/board.go`
  - `internal/board/store_test.go`
  - `_mill/plan/baseline/board.txt`
- **Edits:**
  - `internal/board/board_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Apply layer-ownership. In `TestUpsertTask`, keep the facade-unique
  assertions (tasks.json written + Home.md written) and **drop** the "update preserves
  fields" re-assertion (owned by `store_test.go:TestUpsertTaskPreservesFields`). **Drop**
  `TestRemoveTask` entirely (missing-slug error owned by
  `store_test.go:TestRemoveTaskMissing` and asserted again in cli's error table). Keep
  `TestRerender` (facade persistence wiring: Home + _Sidebar written) and all four
  `TestHealthCheck*` funcs (facade-unique). Record every dropped name in the name-map
  notes with its owning test.
- **Commit:** `test(board): drop store/cli-duplicated facade assertions`

### Card 4: Fold config_test.go LoadConfig + Outputs

- **Context:**
  - `internal/board/config.go`
  - `_mill/plan/baseline/board.txt`
- **Edits:**
  - `internal/board/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold the `board.LoadConfig` variants — `TestDefaultsReturned`,
  `TestErrorNotInitialized`, `TestRelativePathResolution`, `TestAbsolutePathPassthrough`,
  `TestMalformedYAMLError`, `TestLoadConfig_FallbackPathResolution` — into one
  table-driven `TestLoadConfig` (`{name, writeYAML, mkLyx, wantPathSuffix/wantPath,
  wantErrSubstr}`), each case named after its original func. Merge `TestOutputsFromConfig`
  and `TestDefaultOutputs` into one `Outputs()` test with both named subtests. Preserve
  every assertion (path resolution, error substrings, Outputs field values).
- **Commit:** `test(board): fold config_test LoadConfig and Outputs`

### Card 5: Fold init_test.go first-run checks

- **Context:**
  - `internal/board/init.go`
  - `_mill/plan/baseline/board.txt`
- **Edits:**
  - `internal/board/init_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold the three first-run checks `TestInitCreatesStructure`,
  `TestInitGitignoreBlock`, `TestInitJSONShape` into one func that runs `board.RunInit`
  once and asserts structure (_lyx/config + board.yaml/worktree.yaml fully commented) +
  gitignore managed-block markers + JSON shape, with the three original names as
  subtests. Keep `TestInitIdempotent` separate (second-run semantics). Preserve the
  `runInit` helper.
- **Commit:** `test(board): fold init_test first-run checks`

## Batch Tests

`verify: go test ./internal/board/` runs the full board unit package (all eight
`board_test` files; `./internal/board/` does **not** recurse into the out-of-scope
`boardtest` subpackage). After the batch, run `go test ./internal/board/ -cover` and
confirm coverage **≥ 62.5%**; diff `go test ./internal/board/ -list '.*'` against
`_mill/plan/baseline/board.txt` and confirm every removed top-level name maps to a
surviving `t.Run` subtest or to a documented drop (`TestRenderTaskStatus`,
`TestRemoveTask`, and the "update preserves fields" assertion). The name-map produced here
feeds batch 6's doc block.
