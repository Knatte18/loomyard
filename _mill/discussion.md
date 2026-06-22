# Discussion: Prune and consolidate the test suite (board first)

```yaml
task: Prune and consolidate the test suite (board first)
slug: prune-board-tests
status: discussing
parent: main
```

## Problem

Test-suite **wall-clock** is already solved: Tier 1 (offline `go test ./...`) runs
repo-wide in ~3.5 s after `optimize-test-suite` and `optimize-remaining-test-suites`.
But those two tasks ran under a strict **no-drop (superset) guardrail** — they only
folded, relocated, and tagged tests; they never removed redundant coverage. So the
**count** was never pruned: ~236 top-level test funcs repo-wide (~350 counting
subtests), with a lot of one-assertion-per-func sprawl and the same behavior asserted
at multiple layers.

This task prunes the **count** by (a) folding groups of single-shape funcs into
table-driven tests and (b) dropping coverage that is *proven redundant* by another
test. Time is not the target here; readability and signal-to-noise are. `internal/board`
is the worst offender and sets the pattern; the four lighter packages
(`worktree`, `weft`, `ide`, `muxpoc`) get the same pass in this task too.

## Scope

**In:**

- Fold + dedup the test **functions** in five packages, each as its own batch:
  - `internal/board` (8 unit files, 61 funcs) — the pattern-setter, do first.
  - `internal/worktree` (9 files, ~22 funcs).
  - `internal/weft` (5 files, 20 funcs).
  - `internal/ide` (5 files, 20 funcs).
  - `internal/muxpoc` (4 files, 20 funcs).
- Convert clusters of single-shape funcs to table-driven tests with the **same**
  assertions, preserving each original func name as the `t.Run` subtest name.
- Drop only assertions/funcs proven redundant (a strict subset of, or fully covered
  by, another test), per the layer-ownership rule below.
- Append a new **count-focused** history block to
  `docs/benchmarks/test-suite-timing.md` documenting before/after func counts,
  unchanged coverage %, and the justified subset/superset (folded + dropped) name list.

**Out:**

- `internal/board/boardtest` (the 19 integration-tagged git/bench/concurrency funcs) —
  **not touched**. Git-heavy Tier-2 integration with a different character; the
  proposal's detailed plan only covers the `internal/board` unit files.
- `internal/muxpoc/muxpoc_smoke_test.go` (`//go:build smoke`) — separate tier, untouched.
- **No assertion deletion that loses real coverage.** This is function consolidation,
  not coverage deletion. An assertion is dropped only when another test proves the same
  thing.
- No production-code changes. No new test dependencies. No wall-clock optimization work
  (already done). No cosmetic-only rewrites that don't reduce func count (e.g. converting
  an inline bad-input loop to `t.Run` is optional, not required).
- No cross-package shared-helper refactors beyond what a single package's fold needs.

## Decisions

### scope-board-plus-sweep

- Decision: Do all five packages in this one task (`board` first, then the four
  sweep packages), each as an independent batch. Exclude `board/boardtest` and the
  `muxpoc` smoke test.
- Rationale: The user opted for the full sweep. Per-package batching keeps each diff
  self-contained (each package's test files share one package-level func namespace) and
  independently buildable/testable.
- Rejected: board-only (would leave the sweep as follow-up tasks — viable but not what
  was chosen); including boardtest/smoke (riskier integration tiers, not in the proposal's
  detailed plan).

### layer-ownership

- Decision: When the same behavior is asserted at multiple layers, assign one owner:
  - **Unit tests own business logic + validation** (`store`, `config`, `layer`, `task`
    in board; the equivalent unit funcs elsewhere).
  - **Facade tests own persistence wiring** (`board.go`: tasks.json/Home.md written,
    HealthCheck).
  - **CLI tests own the JSON envelope shape + exit codes + not-initialized/edge paths.**
  - Drop business-logic re-assertions from the facade/CLI layers, keeping exactly one
    assertion of each layer's unique value.
- Rationale: Eliminates the 3-way duplication (e.g. "remove missing errors" asserted in
  store + board + cli) while preserving genuine per-layer coverage (serialization, exit
  codes, file persistence).
- Rejected: keep one happy-path assertion per behavior at every layer (more
  conservative, less reduction) — rejected because it preserves most of the duplication
  this task exists to remove.

### preserve-names-as-subtests

- Decision: Each folded case keeps its **original top-level func name as the `t.Run`
  subtest name** (e.g. `name: "TestRenderDoneTask"`), exactly as the prior board fold
  already did (see `render_test.go` `TestRenderToDisk`/`TestRenderSpecialBucketTask`).
  Tests that are *dropped* (not folded) are listed explicitly in the doc.
- Rationale: Keeps `-list` / `=== RUN` traceability so the name-set diff stays auditable
  against the coverage guardrail; a reviewer can map every old name to a surviving
  subtest or to the documented drop list.
- Rejected: descriptive case names (cleaner reading but breaks the auditable name diff).

### coverage-guardrail

- Decision: Capture, **per package, before any edit**: (1) the test-name baseline
  (`go test -list '.*'` for the unit build and, where relevant, `-tags integration`),
  and (2) a coverage profile (`go test -coverprofile`). After the prune, the package's
  coverage % **must not drop**; any folded/dropped name must appear in the documented
  justified subset/superset note. A measured coverage drop blocks the batch until the
  lost assertion is restored or proven dead.
- Rationale: Drops are now allowed (unlike the strict-superset prior tasks), so a name
  diff alone can't catch a dropped assertion that no other test covered. The coverage
  profile is the objective safety net for deletions.
- Rejected: name-set diff only (lighter, but blind to a uniquely-covered assertion that
  gets dropped).

### per-package-batches

- Decision: Five independent batches, one per package, `board` first. Within a batch,
  all of that package's test files may be edited together (shared func namespace).
- Rationale: `board` establishes the table-driven + layer-ownership pattern the others
  follow; each package builds/tests independently so batches are cleanly orderable in the
  mill-go DAG.

### count-focused-doc-block

- Decision: Append a `2026-06-22 — after prune-board-tests` block to
  `docs/benchmarks/test-suite-timing.md` recording before/after **top-level func counts**
  per package, the per-package coverage % (shown unchanged), and a **name-map** (each
  old top-level func name → its surviving `t.Run` subtest, or → its drop justification)
  under an "Equivalence guardrail" subsection consistent with the existing pattern.
  Where a fold absorbs an inline check that had no top-level func name, the name-map
  notes which subtest now carries it.
  Update the headline count narrative; do not claim a wall-clock change (none expected).
- Rationale: The existing history blocks are wall-clock-framed; this prune's metric is
  func count. Framing it explicitly as a count prune keeps the trend log honest.
- Rejected: extend the equivalence-guardrail list only with no count table (less visible
  trend).

## Technical context

All five packages use Go's standard `testing`. Folds are mechanical: gather N funcs that
call the same function with different inputs into a `tests := []struct{…}{…}` slice +
`for _, tt := range tests { t.Run(tt.name, …) }`. The per-package maps below were
produced by reading every test file; they are the concrete work-list.

**All `→ ~N` func counts below are expected outcomes, not quotas.** Per
`coverage-guardrail` and the aggressiveness decision, a batch stops where dropping
would lose a uniquely-covered assertion; a count above the estimate is not a failure.

**Cross-cutting gotchas (apply to every package):**

- **Build-tag buckets.** Many files carry `//go:build integration`. A fold must stay
  *within* one tag bucket — never merge an integration-tagged func with an untagged one
  (it would change which tier runs the assertion). Baselines must be captured for both
  the default build and `-tags integration`.
- **Serial vs parallel.** `t.Setenv` and `os.Chdir`/`t.Chdir` force a test serial (Go
  panics on `t.Parallel` after `t.Setenv`). A folded table mixing such tests must set the
  env/chdir once at parent scope and run subtests serially; never add `t.Parallel` to a
  folded group that contains a `t.Setenv`/chdir member. Do not parallelize tests that are
  currently serial.
- **White-box vs black-box.** Some files are `package <pkg>` (white-box, can call
  unexported funcs) and others `package <pkg>_test` (black-box). A fold cannot cross this
  boundary.
- **Shared fixtures** live in `internal/lyxtest` (`CopyHostHub`, `CopyWeft`,
  `CopyPaired`, `MustRun`, …) and `internal/fslink`. They are per-test isolated; folds
  introduce no shared-state hazard. Do not change these helpers.

### board (do first) — 61 → ~40

Files all `package board_test` (black-box), all untagged (Tier 1). One func namespace.

- **`render_test.go` (16 → ~5).** Fold the single-shape `Render()` assertions into a
  table keyed by task-shape → expected `Home.md`/`_Sidebar.md` substrings:
  `TestRenderDependencies`, `TestRenderSpecialBucketTask` (already folded),
  `TestRenderIsolatedTask`, `TestRenderTaskIDFormatting`, `TestRenderBrief`,
  `TestRenderMissingDependency`, `TestRenderStatusVariants`, `TestRenderLayerBuckets`
  → one/two `TestRenderHome` tables.
  - **Drop** `TestRenderTaskStatus` (`[test-task] [active]`) — strict subset of
    `TestRenderStatusVariants` (which covers `active` among all statuses).
  - Keep distinct: `TestRenderToDisk` (disk I/O + orphan-file cleanup),
    `TestRenderCustomOutputs` (custom Home filename + proposal prefix),
    `TestRenderEmptyTaskList`. Fold `TestRenderSingleTask` (body/no-body proposal-key)
    with `TestRenderOrphanDetection` (second render without body → no proposal key — the
    in-memory analog) into one proposal-key table.
  - `TestRenderExtendedTitle` (sidebar `- Test Task [A]`) overlaps
    `layer_test.go:TestExtendedTitle` (unit on `ExtendedTitle`) and
    `TestRenderCustomOutputs` (sidebar prefix). Keep one render-level assertion that
    `Render` wires `ExtendedTitle` into the sidebar; fold with `TestRenderSidebarBlanks`
    into a small sidebar table.
- **`store_test.go` (13).** This is the **logic owner** — keep its coverage. Already
  partly table-driven (`TestValidateDependencyErrors`, `TestSetPhase`, `TestMergeTasks`,
  `TestSetDeps`, `TestUpsertTasksBatch`). Minor extra folds optional; main change is that
  board/cli stop re-asserting what lives here.
- **`board_test.go` (7).** `TestUpsertTask` — keep the facade-unique part (tasks.json +
  Home.md written); drop the "update preserves fields" re-assertion (owned by
  `store_test.go:TestUpsertTaskPreservesFields`). **Drop** `TestRemoveTask` (missing-slug
  — owned by `store_test.go:TestRemoveTaskMissing`). `TestRerender` — keep as facade
  persistence wiring (writes Home + Sidebar). Keep all four `HealthCheck*` (facade-unique).
- **`cli_test.go` (8 → ~3).** Fold the happy-path verbs (`TestCLIUpsertTask`,
  `TestCLIListTasks`, `TestCLIGetTask`, `TestCLISetPhase`, `TestCLIRerender`) into one
  table-driven `TestCLIContract` asserting JSON envelope shape + exit 0 (the CLI's unique
  value), not business logic. Keep the error/edge table:
  `TestCLIGetNonexistentTask` (null task), `TestCLIRemoveNonexistentTask` (exit 1),
  `TestCLINotInitialized` (exit 1 + "not initialized"). Note: all use
  `t.Setenv("BOARD_SKIP_GIT","1")` + `seedCwd`/`t.Chdir` → serial; fold accordingly.
- **`config_test.go` (8 → ~3).** Fold the `LoadConfig` variants
  (`TestDefaultsReturned`, `TestErrorNotInitialized`, `TestRelativePathResolution`,
  `TestAbsolutePathPassthrough`, `TestMalformedYAMLError`,
  `TestLoadConfig_FallbackPathResolution`) into one table
  `{name, writeYAML, mkLyx, wantPathSuffix, wantErrSubstr}`. Merge `TestOutputsFromConfig`
  + `TestDefaultOutputs` into one `Outputs()` test.
- **`init_test.go` (4 → ~2).** Fold the three first-run checks
  (`TestInitCreatesStructure`, `TestInitGitignoreBlock`, `TestInitJSONShape`) into one
  test that runs init once and asserts structure + gitignore markers + JSON shape; keep
  `TestInitIdempotent` separate (second-run semantics).
- **`layer_test.go` (3)** and **`task_test.go` (2)** — already table-driven/subtest and
  clean. Leave alone (they are the *owners* of `ComputeLayers`/`RenderOrder`/
  `ExtendedTitle` and `NewTask`/`ApplyPatch`).

### worktree — ~22 → ~16

Build-tag split: only `config_test.go` and `prune_test.go` are untagged; the rest are
`//go:build integration`. White-box (`worktree`): add, launchers, portals, prune,
remove, weft. Black-box (`worktree_test`): cli, config, list.

- **Fold A — `TestWeftPrechecks`** (weft_test.go): fold
  `TestWeftPrechecksHardRequireWeftRepo`, `TestWeftPrechecksRejectExistingWeftWorktree`,
  `TestWeftPrechecksRejectExistingWeftBranch`, `TestWeftHostPristineEnforced` into one
  `{name, setup, wantErrContains}` table (identical shape: CopyPaired → setup → Add
  SkipPush → assert err + zero residue). All parallel — safe.
- **Drop** `TestWeftPrechecksHardRequireWeftRepo` content as redundant with
  `add_test.go:TestAdd/NoWeftRepo` (strict superset) — migrate its only extra assertion
  (`result.Slug == ""`) into the Add table, then drop. (If kept, it becomes a row of
  Fold A instead — pick one; do not keep both.)
- **Trim** `TestWeftSpawnPairedWorktrees`: branch-prefix + host-worktree assertions are
  redundant with `TestAdd/HappyPath`+`/BranchPrefix`; keep only the weft-side assertions
  (weft worktree dir + weft branch) that Add does not cover.
- **Fold B (partial) — portals**: `TestCreatePortal`,
  `TestCreatePortalMultipleSubpaths`, `TestCreatePortalRootRelPath` share createPortal
  setup but assert distinct invariants → extract a `setupPortalTarget(t, dir)` helper or
  a table with a per-case `verify` closure. Lower-confidence; keep assertions distinct.
- **`TestRemoveHostJunctionRemoved` vs `TestRemoveSubpathJunction`**: natural 2-case pair
  but the subpath case is serial (`t.Chdir`) while the host case is `t.Parallel` — do
  **not** force-merge; document as a pair.
- **Keep (wiring/unique):** `cli_test.go:TestRunCLI` subtests (JSON shape + `--force`
  flag parsing + exit codes — the only flag-parse coverage); `TestAddRollback` and
  `TestWeftRollbackOnPostHostCreateFailure` (different entry points: real Add failure vs
  direct `rollbackAdd` unit) — optionally share an `assertNoResidue` helper.
- **Already clean:** `config_test.go:TestLoadConfig`, `prune_test.go:TestPruneEmptyAncestors`,
  `list_test.go:TestList`, `add_test.go:TestAdd`, `remove_test.go:TestRemove`,
  `launchers_test.go:TestWriteLaunchers`.

### weft — 20 → ~15

Only `config_test.go` is untagged; cli/status/sync/weft_integration are
`//go:build integration`.

- **Fold — `TestLoadConfig`** (config_test.go): fold `TestLoadConfig_DefaultWhenNoYAML`,
  `TestLoadConfig_OverrideFromYAML`, `TestLoadConfig_MissingLyx` into one table
  `{name, writeYAML, mkLyx, wantPathspec, wantErrSubstr}`. Drop the inline `Dirs()`
  re-assertion in the Override case — `Dirs()` is owned by `TestConfigDirs`.
- **Fold — `TestPushIntegration`** (weft_integration_test.go): fold
  `TestPushIntegration_CommitLandsOnBare` and `TestPushIntegration_RebaseRetryOnNFF`
  (near-identical straight-line happy paths — RebaseRetryOnNFF does not actually set up a
  non-FF remote) into one table whose strongest case is
  `TestSyncIntegration_EventuallyPushed`'s cat-file-on-bare assertion (the superset).
- **Drop** `TestPullIntegration_FastForward` — strict subset of
  `sync_test.go:TestPull_FastForward` (full FF cycle with content restore). Optionally
  keep as a trivial "no-op pull" row.
- **Keep (wiring/unique):** `cli_test.go:TestRunCLI_StatusWithMinimalFixture` (JSON
  envelope + cwd resolution — thin, must not grow logic);
  `weft_integration_test.go:TestRunCLI_EnvMapToOption` (`WEFT_SKIP_PUSH` env → option
  mapping; serial via `t.Setenv`+`t.Chdir`); `TestCommit_ScopedPathspec` (only test of
  the `scopedPathspec` pure fn).
- **Already clean:** `TestConfigDirs`, `TestStatus`, `TestStatus_Junction`,
  `TestStatus_JunctionOk_Windows` (keep separate per its comment — needs real
  `fslink.CreateDirLink`), `TestCommit`, `TestPush`, `TestDefaultConfig`.

### ide — 20 → ~10

Build-tag split: cli + menu are `//go:build integration`; color/spawn/vscode untagged.
No test currently table-driven; none use `t.Parallel`.

- **Fold A — `TestPickColor`** (color_test.go): fold all four `TestPickColor*` into a
  `{name, seedColors, RelPath, wantColor/wantNot}` table. **Preserve the RelPath per
  row** — Never/FirstUnused use `RelPath:"."`, WrapAround/IgnoresUnreadable use
  `RelPath:".vscode"`; do not silently unify. Serial (shared `codeLauncher` global +
  no existing parallelism).
- **Fold B — `TestRunCLIErrors`** (cli_test.go): fold `TestRunCLIUnknownSubcommand`,
  `TestRunCLIMissingSlug`, `TestRunCLINoArgs` into a `{name, args, wantSubstring}` table
  (assert exit 1 + substring; keep NoArgs' JSON `ok=false` as an extra). Keep
  `TestRunCLISpawnDispatch` separate (success path). All serial (`os.Chdir`).
- **Fold C — `TestSpawn`** (spawn_test.go): fold `TestSpawnGeneratesConfig`,
  `TestSpawnCallsCodeLauncher`, `TestSpawnDoesNotClobber` into one table with a `relpath`
  column (R2: the non-"." relpath join is the only unique launcher-path coverage; R3:
  Spawn-level no-clobber is a thin row). **Drop** `TestSpawnColorSelection` — only
  asserts the `workbench.colorCustomizations` key exists, already implied by
  `TestSpawnGeneratesConfig` + `vscode_test.go:TestWriteVSCodeConfigCreatesFilesWhenAbsent`;
  it does not verify the chosen color (that's color_test's job).
- **Menu** (menu_test.go): **drop or fold** `TestMenuZeroWorktreeMessage` into
  `TestMenuRequiresLyxDir` (identical final assertion: "no active worktrees" + nil
  return; RequiresLyxDir also covers the `_lyx` filter, so it's the keeper). Keep
  `TestMenuHardErrorOnMissingBoard`, `TestMenuExcludesMain`, `TestMenuNumericSelection`
  (distinct behaviors, heavy unique git setup — folding yields little). All menu tests
  use `t.Setenv("BOARD_SKIP_GIT","1")` → serial.
- **Already clean / keep:** `vscode_test.go` three funcs (golden-structure,
  no-clobber-of-both-files, gitignore registration — each unique).

### muxpoc — 19 (non-smoke) → ~14

No integration tag; smoke test is `//go:build smoke` (out of scope). White-box
(`package muxpoc`). No `t.Setenv`; `os.Chdir` only in the smoke test. No `t.Parallel`.

- **Fold A — `TestLayoutChecksum`** (cmd_test.go): fold
  `TestLayoutChecksumMatchesPsmux` + `TestLayoutChecksumIsFourHexDigits` into one table
  (two pinned-value rows + one "arbitrary" row asserting the 4-hex-digit shape as a
  per-row post-assertion).
- **Fold B — `TestSocketName`** (state_test.go): fold `TestSocketName`,
  `TestSocketNameStability`, and the inline stability re-check into one func. Per
  `preserve-names-as-subtests`, name the subtests after the originals:
  `TestSocketName` (the existing charset/prefix table) and `TestSocketNameStability`
  (the root-vs-subdir + same-input stability assertions). The inline re-check had no
  top-level name, so it folds into the `TestSocketNameStability` subtest; record this
  in the name-map column of the doc's guardrail block so the auditable diff resolves.
- **Fold C — `TestEnvFiltering`** (state_test.go): fold `TestSanitizeEnv` +
  `TestStrippedEnvKeys` (complementary halves of one behavior over the *same* input
  slice) into one func sharing one `environ` fixture. Per `preserve-names-as-subtests`,
  the two subtests keep the original names `TestSanitizeEnv` and `TestStrippedEnvKeys`
  (removes the duplicated literal, not coverage).
- **Fold D — `TestRunCLIErrors`** (cli_test.go): fold `TestRunCLINoSubcommandFails`,
  `TestRunCLIUnknownSubcommandFails`, `TestRunCLIUnknownFlagFails` into one table; give
  the flag-error row the `out.Len()==0` assertion the others have (R2).
- **Already clean:** `TestExpandTpl`, `TestParseWindowSize`, `TestParsePaneList`,
  `TestBuildColumnLayoutBottomDominatesAndAncestorsEqual` (the richest property test),
  and the distinct state funcs `TestSaveLoadRoundtrip`, `TestLoadStateMissing`,
  `TestLoadStateCorrupt`, `TestDeleteStateMissing`, `TestNewSessionID`.
  `TestParsePaneOrderSortsByTop`'s inline loop → `t.Run` is cosmetic-only, optional.

## Constraints

From `CONSTRAINTS.md`:

- **Path invariant:** all cwd/worktree-root resolution goes through `internal/paths`
  (`paths.Getwd()`, `paths.Resolve()`); raw `os.Getwd` / `git rev-parse --show-toplevel`
  are banned outside `internal/paths` and `cmd/lyx/main.go`, enforced by
  `internal/paths/enforcement_test.go` which scans the whole source tree at test time.
  Test edits must not introduce either primitive. (`t.Chdir` is fine.)
- **fslink:** cross-OS links go through `internal/fslink` (Windows directory junctions).
  Relevant to `weft`/`worktree` junction tests; do not change link primitives.

Discovered during exploration:

- Folds must respect **build-tag buckets**, **serial/parallel** rules (`t.Setenv` ⇒ no
  `t.Parallel`), and **white-box/black-box** package boundaries (enumerated per package
  above).
- Coverage profiles must be captured for **both** the default build and
  `-tags integration` where a package has integration-tagged files.

## Testing

This task's "code under change" *is* the tests; the verification is meta:

- **TDD does not apply** (no production behavior changes). The discipline instead is:
  for each package batch, capture the **baseline first** — `go test ./internal/<pkg>/...
  -list '.*'` (and again with `-tags integration` for packages that have tagged files)
  and `go test ./internal/<pkg>/... -coverprofile=before.out` (plus the tagged variant) —
  then perform the folds/drops, then re-run.
- **Pass criteria per batch:** the package compiles and `go test` (both tiers where
  applicable) is green; coverage % is **≥ baseline**; every old top-level name maps to a
  surviving `t.Run` subtest **or** appears in the documented drop list with its
  justification (the test that subsumes it).
- **Scenarios that must remain covered** after folding (do not let these vanish):
  - board: orphan proposal-file cleanup (`TestRenderToDisk`), custom Home filename +
    proposal prefix, empty task list, all status variants, every bucket header
    (Layer A/B/Z, Someday, Done), dependency depth + missing-dependency rendering;
    store validation (dangling/isolated/deferred deps, cycle detection, merge + batch
    rollback atomicity); CLI not-initialized + exit codes; init idempotency; HealthCheck
    absent-dir / absent-tasks.json / corrupt-but-readable.
  - worktree: Add happy/branch-prefix/no-weft-repo/rollback; weft-side worktree+branch
    creation; portal root vs subpath placement; remove junction cleanup; CLI `--force`
    flag + JSON shape.
  - weft: LoadConfig default/override/missing-lyx; `Dirs()` splitting; status + junction;
    commit/push tables incl. SkipPush/SkipGit; scopedPathspec; FF pull with content
    restore; `WEFT_SKIP_PUSH` env→option mapping.
  - ide: pickColor never-green / first-unused / wrap-around / unreadable (with correct
    RelPath per case); CLI error envelopes + spawn dispatch; Spawn config generation +
    non-"." relpath launcher path + no-clobber; vscode golden structure + gitignore;
    menu missing-board / excludes-main / numeric-selection / requires-lyx.
  - muxpoc: layoutChecksum pinned values + 4-hex shape; socketName charset/stability/
    root-vs-subdir; env sanitize + stripped keys; CLI error exit codes; all parse* +
    column-layout property tests; state save/load/missing/corrupt/delete; session-id
    uniqueness.
- **Final step (separate batch or appended to the last package batch):** update
  `docs/benchmarks/test-suite-timing.md` with the count-focused block (before/after func
  counts per package, coverage unchanged, folded/dropped name list).

## Q&A log

- **Q:** Scope — board-only or board + the worktree/weft/ide/muxpoc sweep? **A:** board +
  full sweep, five per-package batches, board first.
- **Q:** Include `internal/board/boardtest` (19 integration funcs)? **A:** No — exclude;
  also exclude the muxpoc `//go:build smoke` test.
- **Q:** How aggressive? **A:** Principle-driven — fold what's foldable, drop only
  proven-redundant coverage; ~40 for board is an expected outcome, not a hard quota.
- **Q:** Guardrail for drops (now allowed, unlike prior superset-only tasks)? **A:**
  Per-package coverage profile + test-name baseline before; coverage must not drop after;
  document a justified subset/superset name list.
- **Q:** Layer rule for cross-layer duplication? **A:** Unit owns business logic;
  facade owns persistence wiring; CLI owns JSON shape + exit codes. Drop logic
  re-assertions from facade/CLI.
- **Q:** Folded-name handling? **A:** Preserve each original func name as its `t.Run`
  subtest name; list dropped tests explicitly.
- **Q:** Batching? **A:** One batch per package (board first).
- **Q:** Documentation? **A:** New count-focused history block in
  `docs/benchmarks/test-suite-timing.md` (counts + unchanged coverage + name list).
