# Batch: ide

```yaml
task: Prune and consolidate the test suite (board first)
batch: ide
number: 4
cards: 4
verify: go test -tags integration ./internal/ide/
depends-on: [1]
```

## Batch Scope

Apply the board pattern to `internal/ide` (20 → ~10). Fold targets span the untagged
`color_test.go`/`spawn_test.go` (Tier 1) and the integration-tagged
`cli_test.go`/`menu_test.go` (Tier 2). No test in this package is currently table-driven
and none uses `t.Parallel`; several share the package-global `codeLauncher` stub or use
`t.Setenv`/`os.Chdir`, so **all folded groups run serially**. `vscode_test.go` and the
distinct menu funcs (`TestMenuHardErrorOnMissingBoard`, `TestMenuExcludesMain`,
`TestMenuNumericSelection`) and `cli_test.go:TestRunCLISpawnDispatch` are **not** edited.
Depends on batch 1. Coverage floor: **75.4%** (`-tags integration`).

## Cards

### Card 10: Fold pickColor palette tests

- **Context:**
  - `internal/ide/color.go`
  - `_mill/plan/baseline/ide.txt`
- **Edits:**
  - `internal/ide/color_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold `TestPickColorNeverReturnsGreen`,
  `TestPickColorFirstUnusedNonGreen`, `TestPickColorWrapAroundAllUsed`,
  `TestPickColorIgnoresUnreadable` into one table-driven `TestPickColor`
  (`{name, seedColors, RelPath, wantColor/wantNot}`), each case named after its original
  func. **Carry `RelPath` per row** — Never/FirstUnused use `RelPath:"."`,
  WrapAround/IgnoresUnreadable use `RelPath:".vscode"`; do not silently unify. Keep
  serial (no `t.Parallel`; shared scaffold). Preserve every color/wantNot assertion.
- **Commit:** `test(ide): fold pickColor palette tests`

### Card 11: Fold ide CLI error envelopes

- **Context:**
  - `internal/ide/cli.go`
  - `_mill/plan/baseline/ide.txt`
- **Edits:**
  - `internal/ide/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold `TestRunCLIUnknownSubcommand`, `TestRunCLIMissingSlug`,
  `TestRunCLINoArgs` into one table-driven `TestRunCLIErrors`
  (`{name, args, wantSubstring}`; assert exit 1 + substring), each case named after its
  original func, keeping `TestRunCLINoArgs`'s extra JSON `ok=false` assertion as a row
  flag or always-on check. Keep `TestRunCLISpawnDispatch` (success path) separate. All
  use `os.Chdir` → keep **serial**; chdir once per row or at parent scope.
- **Commit:** `test(ide): fold CLI error-envelope tests`

### Card 12: Fold Spawn tests, drop redundant color-selection test

- **Context:**
  - `internal/ide/spawn.go`
  - `internal/ide/vscode_test.go`
  - `internal/ide/color_test.go`
  - `_mill/plan/baseline/ide.txt`
- **Edits:**
  - `internal/ide/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fold `TestSpawnGeneratesConfig`, `TestSpawnCallsCodeLauncher`,
  `TestSpawnDoesNotClobber` into one table-driven `TestSpawn` with a `relpath` column
  (the non-`"."` relpath join in `TestSpawnCallsCodeLauncher` is the only unique
  launcher-path coverage; the Spawn-level no-clobber becomes a thin row), each case named
  after its original func. **Drop** `TestSpawnColorSelection` — it only asserts the
  `workbench.colorCustomizations` key exists, already implied by `TestSpawnGeneratesConfig`
  + `vscode_test.go:TestWriteVSCodeConfigCreatesFilesWhenAbsent`, and does not verify the
  chosen color (color_test's job). Keep serial (shared `codeLauncher` global). Record the
  drop in the name-map.
- **Commit:** `test(ide): fold Spawn tests, drop redundant color-selection`

### Card 13: Dedup menu zero-worktree path

- **Context:**
  - `internal/ide/menu.go`
  - `_mill/plan/baseline/ide.txt`
- **Edits:**
  - `internal/ide/menu_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestMenuZeroWorktreeMessage` and `TestMenuRequiresLyxDir` end on the
  identical assertion ("no active worktrees" + `Menu` returns nil); `RequiresLyxDir` also
  covers the `_lyx`-filter path, so it is the keeper. **Drop**
  `TestMenuZeroWorktreeMessage` — its empty-children case adds no unique coverage over
  `TestMenuRequiresLyxDir`. (Deterministic choice: drop, not fold, so the name-map diff is
  unambiguous.) Keep
  `TestMenuHardErrorOnMissingBoard`, `TestMenuExcludesMain`, `TestMenuNumericSelection`
  unchanged (distinct behaviors, heavy unique git setup). All menu tests use
  `t.Setenv("BOARD_SKIP_GIT","1")` → serial; preserve `mustRunMenu` /
  `newTestGitRepoWithWorktrees` helpers. Record the drop/fold in the name-map.
- **Commit:** `test(ide): dedup menu zero-worktree path`

## Batch Tests

`verify: go test -tags integration ./internal/ide/` runs the full Tier-2 ide package
(superset of the untagged color/spawn folds and the integration cli/menu folds). After
the batch, run `go test -tags integration ./internal/ide/ -cover` and confirm coverage
**≥ 75.4%**; diff `go test -tags integration ./internal/ide/ -list '.*'` against
`_mill/plan/baseline/ide.txt`. Every folded group stays serial — do not introduce
`t.Parallel` (shared `codeLauncher` global, `t.Setenv`, and `os.Chdir` all forbid it).
