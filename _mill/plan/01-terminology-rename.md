# Batch: terminology-rename

```yaml
task: "Weft repo — companion-repo overlay for lyx"
batch: terminology-rename
number: 1
cards: 5
verify: go test ./internal/paths/... ./internal/ide/... ./internal/worktree/...
depends-on: []
```

## Batch Scope

Rename `Layout.Container → Hub`, `Layout.MainWorktree → Prime`, and `HubName() → PrimeName()` across all Go source and test files. This is a pure identifier rename — zero logic changes. Every test that currently passes must still pass after this batch. The batch delivers a compilable, green codebase with the new field/method names, which batch 03 (docs) documents in parallel.

## Cards

### Card 1: Rename Layout fields and method in paths.go

- **Context:** none
- **Edits:**
  - `internal/paths/paths.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/paths/paths.go`: (1) rename struct field `Container string` → `Hub string` on the `Layout` struct; (2) rename struct field `MainWorktree string` → `Prime string` on the `Layout` struct; (3) rename method `HubName() string` → `PrimeName() string`; (4) update the method body from `filepath.Base(l.MainWorktree)` → `filepath.Base(l.Prime)`; (5) update all internal references within the file body — every `l.Container` → `l.Hub`, `l.MainWorktree` → `l.Prime`, and the `Resolve` function assignments `MainWorktree: mainWorktree` → `Prime: mainWorktree`; (6) update all doc comments on the struct fields and method to reflect the new names; (7) update the `Layout` type godoc to describe `Hub` and `Prime` correctly: Hub is the parent of WorktreeRoot (the container directory, not a git repo), Prime is the path to the main/first worktree.
- **Commit:** `paths: rename Layout fields Container→Hub, MainWorktree→Prime; HubName→PrimeName`

### Card 2: Update paths_test.go for renamed fields/method

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/paths/paths_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update every reference in `internal/paths/paths_test.go`: (1) all `layout.Container` → `layout.Hub`; (2) all `layout.MainWorktree` → `layout.Prime`; (3) all `HubName()` calls → `PrimeName()`; (4) all `expectedContainer` variable uses are fine — local variable names need not change; (5) the local variable `hub` (used as the test repo path) is a local name and does NOT need to change; (6) verify no `Container` or `MainWorktree` identifier remains after edit. No new test cases.
- **Commit:** `paths: update tests for Hub/Prime rename`

### Card 3: Update ide/color.go and color_test.go

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/ide/color.go`
  - `internal/ide/color_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/ide/color.go`: (1) `l.Container` → `l.Hub` at lines 49 and 70; (2) `l.MainWorktree` → `l.Prime` at line 55; (3) update any struct-literal field usages or comments. In `internal/ide/color_test.go`: update all `Container:` and `MainWorktree:` struct-literal fields in `paths.Layout` literals to `Hub:` and `Prime:` respectively. No logic changes.
- **Commit:** `ide: update color.go for Layout Hub/Prime rename`

### Card 4: Update ide/spawn_test.go and menu_test.go

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/ide/spawn_test.go`
  - `internal/ide/menu_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/ide/spawn_test.go`: rename all `Container:` → `Hub:` and `MainWorktree:` → `Prime:` in `paths.Layout` struct literals — there are 8 sites (4 struct literals each with Container and MainWorktree fields). In `internal/ide/menu_test.go`: same rename — 10 sites (5 struct literals). Verify no `Container:` or `MainWorktree:` remains after edit.
- **Commit:** `ide: update spawn_test and menu_test for Layout Hub/Prime rename`

### Card 5: Update worktree test files

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/worktree/portals_test.go`
  - `internal/worktree/remove_test.go`
  - `internal/worktree/launchers_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/worktree/portals_test.go`: rename `l.Container` → `l.Hub` at lines 37, 99, 139, 189. In `internal/worktree/remove_test.go`: rename `l.Container` → `l.Hub` at lines 105, 128. In `internal/worktree/launchers_test.go`: update the one comment that references `Container` to say `Hub`. No struct literals or method calls to update in these files beyond the identified sites.
- **Commit:** `worktree: update tests for Layout Hub/Prime rename`

## Batch Tests

`verify: go test ./internal/paths/... ./internal/ide/... ./internal/worktree/...` runs the full test suites for the three affected packages. This scope is justified: the rename touches only these three packages and the changes are behavior-preserving; other packages (board, config, cmd) are unaffected by this batch.
