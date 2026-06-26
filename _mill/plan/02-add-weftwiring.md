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
groups A, B, E and four aggressive deletions (two `TestAdd` table cases, two weftwiring
functions). Net: 9 fewer fixture builds across these two files, no coverage lost (the
KEEP-list tests `TestWeftSpawnPushesWeftBranch` and `TestWeftRollbackOnPostHostCreateFailure`
are untouched). Batch-local: introduces a per-case `extraAssert` hook on the `TestAdd`
table (group A mechanism).

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
- **Requirements:** `TestWeftPrechecksHardRequireWeftRepo` (`weftwiring_test.go`) is a
  byte-for-byte duplicate of the `TestAdd/NoWeftRepo` table case (same rename-weft-prime
  setup, same `"no weft repo"` error, `wantNoTargetDir`, `wantResultZero`) — the
  `add_test.go` `NoWeftRepo` case comment notes it was migrated from here. Delete
  `TestWeftPrechecksHardRequireWeftRepo`. Do NOT delete
  `TestWeftPrechecksRejectExistingWeftWorktree` — it covers a unique error path
  (`"weft worktree directory already exists"`) with no `TestAdd` equivalent.
- **Commit:** `test(warp): delete TestWeftPrechecksHardRequireWeftRepo (duplicate of TestAdd/NoWeftRepo)`

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

### Card 6: Delete low-value weftwiring tests (ForkPointMirrorsHost, MissingParentBranch)

- **Context:**
  - `internal/warp/weftwiring_test.go`
- **Edits:**
  - `internal/warp/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete `TestWeftForkPointMirrorsHost` — it is a subset of
  `TestWeftForkPointSubtaskIsolation`, which also asserts the fork point differs from the
  main tip. Delete `TestWeftMissingParentBranch` — it is the third rollback test; rollback
  is already covered by `TestAddRollback` (live path) and
  `TestWeftRollbackOnPostHostCreateFailure` (white-box `rollbackAdd`). Keep
  `TestWeftForkPointSubtaskIsolation` and `TestWeftRollbackOnPostHostCreateFailure`.
- **Commit:** `test(warp): drop redundant weftwiring fork-point and missing-parent tests`

## Batch Tests

`verify` runs three pipe-free `-run` prefixes covering every function in the two edited
files: `TestAdd` (the consolidated table + `TestAddRollback`/`TestAddAdoptExistingWeftBranch`),
`TestWeft` (all remaining weft-spawn/precheck/forkpoint/rollback tests), and `TestWire`
(the merged `TestWireJunctionsIdempotent`). Chained with `&&`. This confirms the folded
HappyPath assertions pass and that the deletions left the files compiling and green.
