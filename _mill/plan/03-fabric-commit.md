# Batch: fabric-commit

```yaml
task: 'fabric: Fabric.Commit classify+dispatch + unified diff/status'
batch: fabric-commit
number: 3
cards: 4
verify: go test -tags integration ./internal/fabricengine/
depends-on: [1, 2]
```

## Batch Scope

This batch adds the centerpiece `Fabric.Commit` and its integration coverage. It composes the foundations from batch 1 (`classifyPaths`, `commitWeftLocked`, the `Snapshot:` trailer) and the async-push helper from batch 2 (`SpawnDetachedPush`), which is why it depends on both. It delivers the `CommitResult`/`*PartialCommitError` result surface (`_mill/discussion.md`'s `commit-result-and-message` open item pins them in a new `commit.go`), the warp-first two-sided commit under a caller-held weft lock, the three-outcome partial-failure story, and the fire-and-forget both-sides push through the `spawnDetachedPushFn` test seam. Batch-local decisions: `Fabric.Commit` passes `relPath == "."` (shared `relpath-is-dot-for-slice-2` decision) and obtains the wired name-set via `WiredNames(f.weftPath)`, so its integration tests must seed a fabric config into the weft fixture (`lyxtest.SeedConfig(t, weftPath, map[string]string{"fabric": "branch_prefix: \"\"\npathspec: _lyx _pattern\n"})`); every integration test swaps `spawnDetachedPushFn` for a recorder (per the `push-invocation-seam-for-tests` decision) so no real detached child is launched from the test binary.

## Cards

### Card 7: CommitResult, PartialCommitError, and Fabric.Commit

- **Context:**
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/index.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commit.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In a new `internal/fabricengine/commit.go` add `type CommitResult struct { WarpSHA string; WarpCommitted bool; WeftSHA string; WeftCommitted bool }`; `type PartialCommitError struct { WarpSHA string; WeftSHA string; WeftCommitted bool; Err error }` with an `Error() string` and an `Unwrap() error` returning `Err` (semantics: `WeftCommitted == true` → the committed-but-unrecorded outcome, the weft commit landed but `RecordCorrespondence` failed to persist an index entry — because the landed weft commit still carries its `Warp-SHA` trailer (the correspondence index's sole source of truth), an explicit `RebuildIndex` reconstructs the entry; note `WeftSHAForWarpSHA`'s own one-shot rebuild fires only on a *stale hit* (an entry exists but its weft SHA no longer resolves), never on the index *miss* a never-written entry produces, so recovery here is a direct `RebuildIndex` call, not an automatic self-heal on the next lookup; `WeftCommitted == false` → the weft commit itself failed while the warp commit stays); and a package-level test seam `var spawnDetachedPushFn = SpawnDetachedPush`. Add `func (f *Fabric) Commit(files []string, msg string, snapshotTags []string, opts SyncOptions) (CommitResult, error)`: load `wiredNames, err := WiredNames(f.weftPath)` (return the error on failure); `warpFiles, weftFiles := classifyPaths(".", wiredNames, files)`; set `weftSide := len(weftFiles) > 0 && !opts.SkipGit`. When `weftSide`, acquire the weft write lock **before** the warp commit and hold it across both — `f.ensureWeftLockDir()` then `lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))` with a deferred `Release` (this is why `commitWeftLocked` exists — do not call the public `CommitWeft`, which would re-acquire the non-reentrant lock and self-deadlock). Warp first: when `len(warpFiles) > 0`, call `f.Warp.StageAndCommit(msg, warpFiles)` with the **bare** `msg` (no trailer, no correspondence — the plain-git property); on its error return `CommitResult{}` and the wrapped warp error immediately (nothing landed, **before** the push step). Populate `result.WarpSHA`/`result.WarpCommitted`. Then, when `weftSide`, call `f.commitWeftLocked(weftFiles, msg, opts, snapshotTags...)` and map the three outcomes: `err != nil && committed` → set `result.WeftSHA`/`result.WeftCommitted=true` and build a `*PartialCommitError{WarpSHA: result.WarpSHA, WeftSHA: sha, WeftCommitted: true, Err: err}`; `err != nil && !committed` → `*PartialCommitError{WarpSHA: result.WarpSHA, WeftCommitted: false, Err: err}`; `err == nil && committed` → set `result.WeftSHA`/`result.WeftCommitted=true`; `err == nil && !committed` → weft no-op (leave `WeftCommitted=false`, no error). A warp-only degenerate commit (`weftSide == false`) takes no lock, runs only the warp commit, and silently drops any `snapshotTags` (they ride the weft commit; an optional debug log is fine, no error). After the commit step (reached only when no early warp-failure return happened), fire the async push for whatever landed via `_ = spawnDetachedPushFn(f.warpPath, f.weftPath)`, then return `result` plus the `*PartialCommitError` if one was built (else nil). The push is not opts-gated at this call site — gating stays helper-internal in `SpawnDetachedPush` (the `async-push-both-sides-detached` decision).
- **Commit:** `feat(fabric): add Fabric.Commit classify-and-dispatch two-sided commit`

### Card 8: Integration tests — two-sided, warp-only, weft-only, trailers

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commit_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` file (package `fabricengine`) reusing the existing fixtures (`newPlainWarpRepo`, `commitWarp`, `newFabric`, `currentSHA`, `commitMessageAt`, `writeWeftConfigContent`, `lyxtest.CopyWeft`). Each test seeds the fabric config into the weft fixture via `lyxtest.SeedConfig(t, weftFixture.WeftPath, map[string]string{"fabric": "branch_prefix: \"\"\npathspec: _lyx _pattern\n"})` so `WiredNames` resolves, and swaps `spawnDetachedPushFn` for a no-op/recorder with a deferred restore (tests that swap it must not call `t.Parallel()`). A warp-side file is a plain path written into the warp repo (e.g. `README`); a weft-side file is a `_lyx/...` path written into the weft worktree via `writeWeftConfigContent`. Assert: (1) **warp-first ordering** — for a two-sided `Fabric.Commit`, the weft commit's `Warp-SHA` trailer (via `parseWarpSHATrailer` on `commitMessageAt`) equals the warp SHA `Fabric.Commit` just created (`CommitResult.WarpSHA`), not the prior warp HEAD; (2) correspondence is recorded for the weft SHA (`WeftSHAForWarpSHA(result.WarpSHA) == result.WeftSHA`); (3) `CommitResult` fields are populated correctly for two-sided, warp-only, and weft-only inputs; (4) the warp commit carries **no** trailer and creates **no** correspondence entry (plain-git property); (5) a `Snapshot: <tag>` trailer is present on the weft commit for each `snapshotTags` entry and absent when `snapshotTags` is empty; (6) message handling — warp commit message is bare `msg`, weft commit message carries `msg` + `Warp-SHA` + `Snapshot` trailers; (7) a successful `Fabric.Commit` invokes the `spawnDetachedPushFn` recorder with `(f.warpPath, f.weftPath)`.
- **Commit:** `test(fabric): cover Fabric.Commit ordering, results, and trailers`

### Card 9: Integration tests — partial failure and push-on-partial

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commit_partial_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` file covering the three partial-failure outcomes, each seeding the fabric config and swapping `spawnDetachedPushFn` for a recorder (non-parallel). (1) **warp lands, weft commit fails**: induce a weft-side failure after the warp commit (e.g. pre-create the weft gitdir's `index.lock`, the lever `TestRevertWithWeft_WeftResetFailure_RollsWarpBack` uses); assert the warp commit stays (`CommitResult.WarpSHA`/`WarpCommitted` set), the returned error is a `*PartialCommitError` (via `errors.As`) naming the warp SHA, wrapping the weft error, with `WeftCommitted == false`; and the push recorder **was** called (durable warp commit still pushed). (2) **warp commit fails**: induce a warp-side failure (e.g. pre-create the warp gitdir's `index.lock`); assert nothing landed on weft, an error is returned, and the push recorder was **not** called (Fabric.Commit returns before the push step). (3) **committed-but-unrecorded**: force `RecordCorrespondence` to fail after the weft commit lands (e.g. pre-create a directory at the correspondence-index path — `f.corrIndexPath()` — so its JSON write fails); assert `CommitResult.WeftCommitted == true` with `WeftSHA` set, the error is a `*PartialCommitError` with `WeftCommitted == true` (a correspondence-recording failure, distinct from a commit failure), the push recorder **was** called, and after clearing the block an explicit `f.RebuildIndex()` (rescanning the landed weft commit's `Warp-SHA` trailer, the index's sole source of truth) followed by `f.WeftSHAForWarpSHA(result.WarpSHA)` resolves to `result.WeftSHA` (no data lost). Do **not** assert that `f.WeftSHAForWarpSHA` alone self-heals: the blocked `RecordCorrespondence` persisted no entry, so the lookup takes the index-*miss* path (`ErrNoCorrespondence`) whose one-shot `RebuildIndex` never fires — that auto-rebuild is reached only on a *stale hit*, per the `WeftSHAForWarpSHA` correspondence-lookup logic.
- **Commit:** `test(fabric): cover Fabric.Commit partial-failure outcomes and push-on-partial`

### Card 10: Integration tests — skip-git commit-side gating

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commit_gating_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` file (seed fabric config, swap `spawnDetachedPushFn` for a recorder, non-parallel) asserting the commit-side skip gating: with `opts.SkipGit == true`, a two-sided input commits the warp side (`CommitResult.WarpCommitted == true`) but the weft side no-ops (`CommitResult.WeftCommitted == false`, weft HEAD unchanged, no weft lock acquired); and a warp-only input under `opts.SkipGit` still lands its warp commit. Assert a two-sided commit under normal `opts` DOES land both sides (control case). The "async push forks no child under `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`" property is a `SpawnDetachedPush` behavior covered by batch 2's `spawn_test.go`, not re-asserted here (the seam is swapped in these tests).
- **Commit:** `test(fabric): cover Fabric.Commit skip-git commit-side gating`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/` runs all four new integration files (`commit_integration_test.go`, `commit_partial_integration_test.go`, `commit_gating_integration_test.go`) plus the full existing `fabricengine` suite, regression-guarding the batch-1 `commitWeftLocked` refactor once `Fabric.Commit` exercises the held-lock-across-both-commits path. The `spawnDetachedPushFn` seam keeps every test offline and deterministic (no real detached child from the test binary); the env-gated no-fork behavior of the real `SpawnDetachedPush` is covered by batch 2's Tier-1 `spawn_test.go`. Fabric-config seeding via `lyxtest.SeedConfig` is required in every test because `Fabric.Commit` resolves `WiredNames(f.weftPath)`.
