# Batch: fabric-commit-lock-and-wiring

```yaml
task: 'fabric: warp-side commit lock + push coalescing'
batch: fabric-commit-lock-and-wiring
number: 3
cards: 4
verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...
depends-on: [2]
```

## Batch Scope

Closes the headline asymmetry and wires the coalescing loop into the async push path. Three code changes plus the package doc: (1) `Fabric.Commit` acquires the combined write lock whenever it will commit anything (not only on the weft side) and releases it before spawning the async push child; (2) the fabric CLI bypass push handler (the code the detached child re-enters) runs `CoalescePushBothAt` instead of one single push per side; (3) `internal/fabricengine/doc.go`'s package comment is updated in the same batch as the observable behavior changes it describes, per the Documentation Lifecycle. All of this depends on batch 2's `CoalescePushBothAt`. There is no file overlap with batch 4 (board), so the two may run in parallel.

Batch-local decision: `PushWarpAt` (`spawn.go`) loses its only production caller when the bypass handler switches to `CoalescePushBothAt`. It is RETAINED (not deleted) — it remains the exported synchronous warp-push primitive and the sibling of `PushWeftAt`, has its own `spawn_test.go` coverage, and removing it is out of this slice's scope. Card 7 notes this retention rather than churning `spawn.go`/`spawn_test.go`.

## Cards

### Card 6: Combined commit lock in `Fabric.Commit`, released before the async push

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/spawn.go`
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/fabricengine/commit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/fabricengine/commit.go`, change `Fabric.Commit` so the write lock (`filepath.Join(lockDir, weftWriteLockFile)` obtained via `f.ensureWeftLockDir()`) is acquired whenever the call will commit something, not only on the weft side. Compute `weftSide := len(weftFiles) > 0 && !opts.SkipGit` and `committing := len(warpFiles) > 0 || weftSide`; acquire the lock iff `committing`. A degenerate no-op call (`!committing`) takes no lock and calls no `ensureWeftLockDir`, exactly as today (Shared Decision `combined-commit-lock`).
  - Structure the code so the lock is RELEASED before `spawnDetachedPushFn(f.warpPath, f.weftPath)` is called (Shared Decision `commit-lock-scoped-to-commit-only`). A function-scoped `defer l.Release()` holds the lock across the spawn, which the design forbids and a test asserts against — so scope the locked section explicitly. Suggested shape: extract a helper `func (f *Fabric) commitBothSides(warpFiles, weftFiles []string, weftSide bool, msg string, snapshotTags []string, opts SyncOptions) (CommitResult, *PartialCommitError, error)` that acquires the lock under `committing` with a `defer`-release local to the helper, performs the existing warp-then-weft commit + `*PartialCommitError` mapping, and returns `(result, partialErr, err)`; `Commit` then calls it (lock released on return), and only after that runs the `if result.WarpCommitted || result.WeftCommitted { _ = spawnDetachedPushFn(...) }` gate and the `partialErr`/nil return. Preserve every existing behavior: warp-first ordering, the three-outcome `*PartialCommitError` mapping (`commit.go` lines ~131-145), the early hard-error return on warp commit failure, and the "spawn only when something landed" gate.
  - Update `Fabric.Commit`'s own doc comment so it states the lock is taken for any committing call (`len(warpFiles) > 0 || weftSide`), released before the async push spawn, and no longer implies the lock is weft-only. Keep the existing accurate statements (warp-only drops `snapshotTags`, etc.).
- **Commit:** `fix(fabricengine): take combined write lock for any committing Fabric.Commit, release before push`

### Card 7: Wire the CLI bypass push handler to the coalescing loop

- **Context:**
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabriccli/pushbypass_integration_test.go`
  - `internal/fabriccli/cli_test.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/fabriccli/weft_verbs.go`, replace the bypass-mode body of the `push` subcommand's `RunE` (currently lines ~206-225: the sequential `if warpPath != "" { PushWarpAt(...) }` then `if weftPath != "" { PushWeftAt(...) }`) with a single call to `fabricengine.CoalescePushBothAt(warpPath, weftPath, fabricengine.SyncOptions{})`. On error, surface it via `clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))` and return; on success emit `output.Ok(out, map[string]any{})` exactly as today. Leave normal (non-bypass) mode, the `--warp-path`-only non-push rejection path, and all other subcommands untouched.
  - Update the file's package-doc/handler comments that currently say bypass mode "calls `PushWarpAt` then `PushWeftAt` (single push each)" to describe the coalescing-loop delegation.
  - Do NOT delete `PushWarpAt` (see Batch Scope retention decision).
  - Re-run and confirm `internal/fabriccli/pushbypass_integration_test.go`'s `TestRunCLI_BypassPushAdvancesBothUpstreams` (both bare upstreams advance) and `TestRunCLI_WarpPathPushOnly` (exit 1 for a non-push verb) still pass unchanged — the coalescing loop pushes both sides at least once, so a quiescent pair still advances both upstreams, and the bypass-mode verb gating is unchanged. If a stale in-comment reference (e.g. "wired in card 5") no longer matches, it may be left as-is (historical) — do not edit test files in this card unless an assertion genuinely fails.
- **Commit:** `feat(fabriccli): run the coalescing push loop in the bypass push handler`

### Card 8: Update the fabricengine package doc for the coalescing async push

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabriccli/weft_verbs.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/fabricengine/doc.go`, update the async-push paragraph (the one describing `Commit` firing "an unconditional, detached, fire-and-forget push of both repos via `SpawnDetachedPush`") to state that the detached child now runs a loop-until-clean coalescing push (via `CoalescePushBothAt` in the CLI bypass handler) under a separate absorbing push lock in `.weft/` (`fabric.push.lock`), that the push is rebase-free (a diverged remote leaves commits unpushed and logs, never `pull --rebase`), and that the combined write lock is now taken for any committing `Fabric.Commit` call (warp-only included) and released before the spawn. Keep the existing accurate `opts.SkipGit`/`opts.SkipPush` scoping statements; adjust only where they interact with the changed behavior.
  - Also update the `.gitrepo-push.lock` asymmetry note (the paragraph stating `Fabric.Status` may surface a host-side `.gitrepo-push.lock`) to reflect that the warp-via-fabric async push no longer creates that artifact at the host root (it is now lock-free rebase-free), while noting a host lock could still appear only from a non-fabric host-side `PushCoalesced` caller. Do not overstate — `Fabric.Status`'s behavior of not suppressing a host-side artifact is unchanged; only the fabric async path no longer produces one.
- **Commit:** `docs(fabricengine): document coalescing rebase-free async push in package doc`

### Card 9: Integration tests for the combined lock and async-push behavior

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/commit_partial_integration_test.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/lock/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commit_lock_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `internal/fabricengine/commit_lock_integration_test.go` (package `fabricengine`, `//go:build integration`), reusing existing fixture helpers (`newPlainWarpRepo`, `commitWarp`, `currentSHA`, `newFabric`, `seedFabricConfig`, `writeWarpFile`) and the `spawnDetachedPushFn` seam swap pattern already used by `commit_integration_test.go` (swap it for a recorder/no-op with a deferred restore; no `t.Parallel()`).
  - Cover: (a) **Warp-only commit serialization** — two concurrent warp-only `Fabric.Commit` calls (weft-empty inputs) against one warp+weft pair serialize on `.weft/weft.write.lock`; assert both warp commits land, neither corrupts the other's index, and history is linear. This is the test that fails against today's unlocked warp-only path. Use goroutines; `gofrs/flock` contends even in-process (two `AcquireWriteLock` handles on one path), so no process spawn is needed. (b) **Combined-lock coverage across sides** — a warp-only, a weft-only, and a two-sided `Fabric.Commit` all contend on the same lock file; assert mutual exclusion via an observable ordering or a lock-held probe. (c) **Commit lock released before push** — assert the write lock is NOT held when `spawnDetachedPushFn` fires: e.g. inside the swapped seam, a non-blocking `lock.TryAcquireWriteLock` on `.weft/weft.write.lock` succeeds (the lock is already released). (d) **Push-on-partial-failure** — on a `*PartialCommitError` where the warp landed but weft failed, assert the seam still fired (`WarpCommitted || WeftCommitted` gate), preserving current behavior; reuse `commit_partial_integration_test.go`'s failure-injection approach.
  - Do not over-specify assertion shapes; mill owns those.
- **Commit:** `test(fabricengine): cover combined commit lock, release-before-push, and partial-failure push`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/...` runs the new `commit_lock_integration_test.go` (tagged) and re-runs `fabricengine`'s existing commit/coalesce suites, plus `fabriccli`'s bypass-push integration tests (`pushbypass_integration_test.go`, `cli_test.go`) that card 7's wiring change must keep green. Both packages already satisfy the Hermetic Git Test Environment Invariant via their own `TestMain`. The scope spans two packages because the wiring change straddles the `fabricengine`/`fabriccli` boundary.
