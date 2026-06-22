# Batch: worktree

```yaml
task: Prune and consolidate the test suite (board first)
batch: worktree
number: 2
cards: 2
verify: go test -tags integration ./internal/worktree/
depends-on: [1]
```

## Batch Scope

Apply the board pattern to `internal/worktree` (22 → ~16). The fold targets live in the
white-box `package worktree`, integration-tagged files `weft_test.go`, `add_test.go`, and
`portals_test.go`. The already-clean tables (`add_test.go:TestAdd`,
`remove_test.go:TestRemove`, `list_test.go:TestList`, `config_test.go:TestLoadConfig`,
`prune_test.go:TestPruneEmptyAncestors`, `launchers_test.go`) and the wiring-unique
`cli_test.go` are **not** edited. Depends on batch 1 (board sets the convention). Coverage
floor: **68.6%** (`-tags integration`).

## Cards

### Card 6: Fold weft prechecks and migrate the no-weft-repo redundancy

- **Context:**
  - `internal/worktree/add.go`
  - `internal/worktree/weft.go`
  - `_mill/plan/baseline/worktree.txt`
- **Edits:**
  - `internal/worktree/weft_test.go`
  - `internal/worktree/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** (a) Fold `TestWeftPrechecksRejectExistingWeftWorktree`,
  `TestWeftPrechecksRejectExistingWeftBranch`, and `TestWeftHostPristineEnforced` into one
  table-driven `TestWeftPrechecks` (`{name, setup, wantErrContains}`; shared shape:
  `lyxtest.CopyPaired` → setup → `w.Add(..., SkipPush:true)` → assert error substring +
  zero residue), each case named after its original func; all are `t.Parallel`, so the
  table runs parallel. (b) **Drop** `TestWeftPrechecksHardRequireWeftRepo`: it duplicates
  `add_test.go:TestAdd/NoWeftRepo` (same "no weft repo" error + no target dir); first
  migrate its one extra assertion (`result.Slug == ""`) into the `NoWeftRepo` case of the
  `TestAdd` table in `add_test.go`, then remove the func. (c) Trim
  `TestWeftSpawnPairedWorktrees`: drop its branch-prefix and host-worktree assertions
  (covered by `TestAdd/HappyPath`+`/BranchPrefix`); keep only the weft-side assertions
  (weft worktree dir at `WeftWorktreePath`, weft branch via `WeftRepoRoot()`). Keep
  `TestWeftSpawnCreatesJunction`, `TestWeftSpawnSeedsExclude`,
  `TestWeftRollbackOnPostHostCreateFailure` (distinct: direct `rollbackAdd` unit). Record
  each dropped/folded name in the name-map.
- **Commit:** `test(worktree): fold weft prechecks, migrate no-weft-repo assertion`

### Card 7: Extract shared portal-setup helper

- **Context:**
  - `internal/worktree/portals.go`
  - `_mill/plan/baseline/worktree.txt`
- **Edits:**
  - `internal/worktree/portals_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Remove the duplicated createPortal setup boilerplate across
  `TestCreatePortal`, `TestCreatePortalMultipleSubpaths`, `TestCreatePortalRootRelPath`
  by extracting a `setupPortalTarget(t, dir)` file-local helper (resolve layout, mkdir
  `<Hub>/<slug>/<RelPath>/_lyx`, skip-on-unsupported). The three funcs assert genuinely
  distinct invariants (link resolves / links distinct / flat root layout) — keep them as
  three funcs (or one table with a per-case `verify` closure), preserving every
  assertion and the original names. This is a boilerplate dedup, not a coverage drop.
- **Commit:** `test(worktree): extract setupPortalTarget helper`

## Batch Tests

`verify: go test -tags integration ./internal/worktree/` runs the full Tier-2 worktree
package (the integration build is the superset, so it exercises both the tagged folds and
the untagged `config_test`/`prune_test`). After the batch, run
`go test -tags integration ./internal/worktree/ -cover` and confirm coverage **≥ 68.6%**;
diff `go test -tags integration ./internal/worktree/ -list '.*'` against
`_mill/plan/baseline/worktree.txt`. Note the serial/parallel split: `cli_test.go` and
`remove_test.go:TestRemoveSubpathJunction` use `t.Chdir` and stay serial (not edited
here); the `TestWeftPrechecks` fold members are all `t.Parallel` and merge safely.
