# Batch: add-weftwiring

```yaml
task: "Speed up internal/warp integration tests"
batch: "add-weftwiring"
number: 2
cards: 5
verify: go test -tags integration -run TestAdd ./internal/warp/ && go test -tags integration -run TestWeft ./internal/warp/ && go test -tags integration -run TestWire ./internal/warp/
depends-on: [1]
```

## Batch Scope

Consolidates and prunes the `Add` and weft-wiring test surface — the two files are coupled
because group A folds weft-spawn structural assertions into `add_test.go`'s `TestAdd` table
while deleting their standalone homes in `weftwiring_test.go`. Delivers consolidation
groups A, B, E plus low-value prunes (two `TestAdd` table cases removed, one weftwiring
subset deleted, one weftwiring missing-parent rollback folded into a kept test). Net: 9
fewer fixture builds across these two files, no coverage lost (the KEEP-list tests
`TestWeftSpawnPushesWeftBranch` and `TestWeftRollbackOnPostHostCreateFailure` are kept;
the latter gains the missing-parent scenario). Batch-local: introduces a per-case
`extraAssert` hook on the `TestAdd` table (group A mechanism).

## Cards

### Card 2: Group A — fold weft-spawn structural assertions into TestAdd/HappyPath

- **Context:**
  - `internal/paths/paths.go`
  - `internal/fslink/fslink.go`
  - `internal/warp/weftwiring.go`
- **Edits:**
  - `internal/warp/add_test.go`
  - `internal/warp/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestAdd` (`add_test.go`) is table-driven with fields
  `name/branchPrefix/setup/opts/wantBranch/wantErrContains/wantNoTargetDir/wantResultZero`
  and no per-case assertion field. Add a field
  `extraAssert func(t *testing.T, f lyxtest.PairedFixture, res AddResult)` (nil for all
  existing rows). In the shared test body, after the existing success assertions and only
  on the no-error path, call `tc.extraAssert(t, f, result)` when non-nil. Set the
  `HappyPath` row's `extraAssert` to a closure that ports the assertions of the four
  standalone tests being removed: (a) from `TestAddDormant` — the host `_lyx` junction is
  NOT a link: `fslink.IsLink(f.Layout.HostLyxLink(slug))` returns `(false, nil)`; (b) from
  `TestWeftSpawnCreatesWeftDirectory` — `os.Stat(f.Layout.WeftLyxDirFor(slug))` succeeds;
  (c) from `TestWeftSpawnNoExcludeEntry` — the host worktree's git exclude file does NOT
  contain `_lyx` (port the exact path-resolution + read logic from that test); (d) from
  `TestWeftSpawnPairedWorktrees` — the weft worktree dir
  `f.Layout.WeftWorktreePath(slug)` exists and `refs/heads/<branch>` resolves in the weft
  repo (port its exact ref check). Then delete the standalone `TestAddDormant` from
  `add_test.go` and `TestWeftSpawnCreatesWeftDirectory`, `TestWeftSpawnNoExcludeEntry`,
  `TestWeftSpawnPairedWorktrees` from `weftwiring_test.go`. Keep the `extraAssert` body
  inside the existing per-row loop so weft-spawn assertions run ONLY for the HappyPath row.
- **Commit:** `test(warp): fold weft-spawn structural assertions into TestAdd/HappyPath`

### Card 3: Group B — merge WireJunctions PreservesBehavior into Idempotent

- **Context:** none
- **Edits:**
  - `internal/warp/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestWireJunctionsPreservesBehavior` and `TestWireJunctionsIdempotent`
  overlap ~95%. Port the one assertion unique to `TestWireJunctionsPreservesBehavior` — that
  `_lyx` appears as a complete trimmed line in the exclude file (the
  `strings.TrimSpace(line) == "_lyx"` check, not a substring match) — into
  `TestWireJunctionsIdempotent`, then delete `TestWireJunctionsPreservesBehavior`.
- **Commit:** `test(warp): merge WireJunctions PreservesBehavior into Idempotent`

### Card 4: Group E — delete duplicate weft-precheck test

- **Context:**
  - `internal/warp/add_test.go`
- **Edits:**
  - `internal/warp/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `HardRequireWeftRepo` is a **table row** (a `name:` field) inside the
  `TestWeftPrechecks` table at `weftwiring_test.go:~209`, not a standalone function. That
  row is a byte-for-byte duplicate of the `TestAdd/NoWeftRepo` table case (same
  rename-weft-prime setup, same `"no weft repo"` error, `wantNoTargetDir`, `wantResultZero`)
  — the `add_test.go` `NoWeftRepo` case comment notes it was migrated from here. Remove the
  `HardRequireWeftRepo` row from the `TestWeftPrechecks` table, leaving the single
  `RejectExistingWeftWorktree` row (a unique error path,
  `"weft worktree directory already exists"`, with no `TestAdd` equivalent — keep it).
- **Commit:** `test(warp): remove duplicate HardRequireWeftRepo row from TestWeftPrechecks`

### Card 5: Delete low-value TestAdd table cases (UnbornBranch, TargetDirExists)

- **Context:** none
- **Edits:**
  - `internal/warp/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Remove the `UnbornBranch` and `TargetDirExists` rows from the `TestAdd`
  table. `UnbornBranch` asserts the same `"detached HEAD"` guard already covered by the
  `DetachedHEAD` row (unborn is treated as detached). `TargetDirExists` is a defensive
  precheck that `git worktree add` guards structurally. Leave all other rows
  (HappyPath, BranchPrefix, DirtySource, BranchExists, NoRemote, NoWeftRepo, DetachedHEAD)
  intact.
- **Commit:** `test(warp): drop redundant TestAdd UnbornBranch and TargetDirExists cases`

### Card 6: Delete ForkPointMirrorsHost; fold MissingParentBranch trigger into a kept test

- **Context:** none
- **Edits:**
  - `internal/warp/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete `TestWeftForkPointMirrorsHost` — it is a genuine subset of
  `TestWeftForkPointSubtaskIsolation`, which also asserts the fork point differs from the
  main tip (kept). For `TestWeftMissingParentBranch`: it is the **only** test of Add's live
  paired rollback triggered by a *missing parent weft branch* (distinct from
  `TestAddRollback`'s portal-clobber trigger and `TestWeftRollbackOnPostHostCreateFailure`'s
  white-box `rollbackAdd` call), so fold rather than delete. Port its missing-parent trigger
  (the `git checkout -b <Z>` setup that makes the parent weft branch absent) and its
  zero-residue assertions into `TestWeftRollbackOnPostHostCreateFailure` as a second
  sequential scenario on the same fixture (after a rollback the worktree is clean, so the
  second trigger runs cleanly), then delete the standalone `TestWeftMissingParentBranch`.
  This removes its separate fixture build while keeping the missing-parent rollback coverage.
- **Commit:** `test(warp): drop ForkPointMirrorsHost; fold missing-parent rollback into RollbackOnPostHostCreateFailure`

## Batch Tests

`verify` runs three pipe-free `-run` prefixes covering every function in the two edited
files: `TestAdd` (the consolidated table + `TestAddRollback`/`TestAddAdoptExistingWeftBranch`),
`TestWeft` (all remaining weft-spawn/precheck/forkpoint/rollback tests), and `TestWire`
(the merged `TestWireJunctionsIdempotent`). Chained with `&&`. This confirms the folded
HappyPath assertions pass and that the deletions left the files compiling and green.
