# Batch: prune-cleanup

```yaml
task: "Speed up internal/warp integration tests"
batch: "prune-cleanup"
number: 4
cards: 3
verify: go test -tags integration -run TestPrune ./internal/warp/ && go test -tags integration -run TestCleanup ./internal/warp/
depends-on: [1]
```

## Batch Scope

Merges the prune and cleanup tests that share identical setup but differ only in flags,
running multiple operations sequentially against one fixture. Delivers groups F, G, H.
Net: 3 fewer fixture builds across `prune_test.go` and `cleanup_test.go`, no coverage lost
(the prefix-mismatch regression guard from `_NonEmptyBranchPrefix` is preserved by merging,
not deleting). Batch-local: sequential operations on a shared fixture (no `t.Parallel`
between the steps), each asserting the intermediate state.

## Cards

### Card 11: Group F — merge Prune dry-run + apply into one sequential test

- **Context:** none
- **Edits:**
  - `internal/warp/prune_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestPrune_DryRunReportsStaleWeft` and `TestPrune_ApplyRemovesStaleWeft`
  share identical setup (Add a paired worktree via `setupPruneFixture`, then
  `git worktree remove` the host to leave a stale weft). The dry-run leaves the stale weft
  intact, so apply can run on the same post-dry-run fixture. Replace the two with one
  `TestPrune_StaleWeft` that: (1) sets up the stale state once; (2) runs `Prune(apply=false)`
  and asserts the stale weft is reported and NOT removed (dir intact); (3) runs
  `Prune(apply=true)` and asserts `Removed=true` and the weft dir is gone. Steps run
  sequentially (no `t.Parallel` between them). Keep `TestPrune_LivePairNeverTouched`.
- **Commit:** `test(warp): merge Prune dry-run and apply into one sequential test`

### Card 12: Group G — merge Cleanup report-only modes onto one fixture

- **Context:** none
- **Edits:**
  - `internal/warp/cleanup_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestCleanup_DryRunReportsOrphanBranch` (apply=false, force=false) and
  `TestCleanup_ForceAloneReportsOnly` (apply=false, force=true) both report-only — neither
  deletes — so both `Cleanup` calls can run on one fixture. Replace the two with one
  `TestCleanup_ReportOnlyModes` that: (1) adds one orphan branch via `createOrphanWeftBranch`
  on a single `setupCleanupFixture`; (2) calls `Cleanup(false, false)` and asserts the
  orphan is reported, not deleted; (3) calls `Cleanup(false, true)` and asserts still not
  deleted. Sequential (no `t.Parallel` between calls). Keep
  `TestCleanup_ApplySkipsProtectedBranch` and `TestCleanup_ApplyForceDeletesTaskBranch`
  separate — they assert contradictory delete outcomes and cannot share a fixture.
- **Commit:** `test(warp): merge Cleanup report-only modes onto one fixture`

### Card 13: Group H — combine Cleanup live-branch tests (no-prefix + prefix)

- **Context:** none
- **Edits:**
  - `internal/warp/cleanup_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestCleanup_LiveBranchNeverDeleted` and
  `TestCleanup_LiveBranchNeverDeleted_NonEmptyBranchPrefix` both assert a live pair is never
  touched by `Cleanup`, differing only in branch prefix (none vs `"hanf/"`). Combine into one
  `TestCleanup_LiveBranchNeverDeleted` that, on one fixture: (1) `Add`s a live pair with no
  prefix; (2) `Add`s a live pair with the `"hanf/"` prefix; (3) runs `Cleanup(true, true)`
  once; (4) asserts neither live branch was reported or deleted. This preserves the cited
  prefix-mismatch regression coverage — do not drop the prefixed case.
- **Commit:** `test(warp): combine Cleanup live-branch never-deleted prefix cases`

## Batch Tests

`verify` runs `TestPrune` (the merged stale-weft test + `TestPrune_LivePairNeverTouched`)
and `TestCleanup` (the merged report-only + combined live-branch tests + the two protected/
force tests), chained with `&&`. Confirms the sequential merges assert each intermediate
state and that the prefix regression coverage survives.
