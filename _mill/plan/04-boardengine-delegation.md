# Batch: boardengine-delegation

```yaml
task: 'fabric: warp-side commit lock + push coalescing'
batch: boardengine-delegation
number: 4
cards: 2
verify: go test -tags integration ./internal/boardengine/...
depends-on: [2]
```

## Batch Scope

Makes `boardengine.Sync` delegate its absorbing-lock loop-until-clean coalescing to `fabricengine.CoalescePush`, removing board's parallel implementation of git-level concurrency serialization (the layering inversion the brief calls out). Board keeps its own `board.lock` (via `commitDirty`), its `board.push.lock` path, its `ensureLockfilesIgnored` `.gitignore` seeding, and its rebase-retry push (`PushWeftAt` → `PushCoalesced`) — only the acquire site of the absorbing push lock and the loop skeleton move into `fabricengine`. This depends only on batch 2's `CoalescePush` (not batch 3); it shares no files with batch 3 and may run in parallel with it.

Batch-local decision: `ensureLockfilesIgnored` moves INSIDE the step closure (running once per iteration) rather than staying a pre-loop call. It is idempotent and cheap (reads `.gitignore`, returns early when the patterns are present), and running it as the first action of each iteration preserves the original ordering guarantee — ignore patterns in place before the first `git add -A` — now that the absorbing lock is acquired by the primitive rather than by `Sync` itself.

## Cards

### Card 10: Delegate `boardengine.Sync` to `fabricengine.CoalescePush`

- **Context:**
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/boardengine/board.go`
  - `cmd/lyx/boardguard_test.go`
- **Edits:**
  - `internal/boardengine/sync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/boardengine/sync.go`, rewrite `Sync(boardPath string, skipGit, skipPush bool) error` to delegate its loop to `fabricengine.CoalescePush`. Keep the `if skipGit { return nil }` early return. Remove the direct `flock.AcquireWriteLock(filepath.Join(boardPath, pushLockFile))` acquire and its `defer pushLock.Release()` from `Sync` — the primitive now owns that acquire, at the SAME path `filepath.Join(boardPath, pushLockFile)` (`board.push.lock`, unchanged name/location). Build a step closure that: (1) calls `ensureLockfilesIgnored(boardPath)` (return `(false, err)` on error); (2) calls `committed, err := commitDirty(boardPath)` (return `(false, err)` on error); (3) if `!skipPush`, calls `fabricengine.PushWeftAt(boardPath, fabricengine.SyncOptions{})`, wrapping any error as `fmt.Errorf("sync push: %w", err)` and returning `(false, err)`; (4) returns `(committed, nil)`. Then `return fabricengine.CoalescePush(filepath.Join(boardPath, pushLockFile), step)`.
  - Leave `commitDirty` (still acquires `board.lock` via `flock.AcquireWriteLock`), `ensureLockfilesIgnored`, and the `writeLockFile`/`pushLockFile` constants unchanged. Confirm the `flock` and `filepath` imports remain used (they do — `commitDirty` uses both) so no import churn is needed.
  - Update `sync.go`'s package/`Sync` doc comment to say the absorbing-lock loop is now provided by `fabricengine.CoalescePush`, board supplying the commit+push step; keep the description of coalescing semantics (a burst of writes collapses into as few pushes as possible) accurate.
  - Do NOT introduce any `internal/gitrepo` or `internal/gitexec` import into `boardengine` — `cmd/lyx/boardguard_test.go` forbids it; board reaches git only through `fabricengine`, which `CoalescePush`/`PushWeftAt` preserve.
- **Commit:** `refactor(boardengine): delegate Sync coalescing loop to fabricengine.CoalescePush`

### Card 11: Board sync parity integration test + hermetic TestMain

- **Context:**
  - `internal/boardengine/sync.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/gitrepo/push_test.go`
- **Edits:** none
- **Creates:**
  - `internal/boardengine/sync_integration_test.go`
  - `internal/boardengine/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `internal/boardengine/testmain_test.go` (package `boardengine`) with a `TestMain(m *testing.M)` that calls `lyxtest.HermeticGitEnv()` before `os.Exit(m.Run())` — required because the new integration test spawns git and `boardengine` has no hermetic `TestMain` today (Hermetic Git Test Environment Invariant). Mirror `internal/fabricengine/testmain_test.go`.
  - Create `internal/boardengine/sync_integration_test.go` (package `boardengine`, `//go:build integration`). Build a board worktree as a git repo on `main` with a bare `origin` and upstream tracking (use `lyxtest.MustRun` for the git setup, following the fixture shape in `internal/gitrepo/push_test.go`'s `newBareRemote`/`newRepoWithRemote` — reimplemented locally since those helpers live in a different test package). Assert board's post-delegation parity: (a) `Sync(boardPath, false, false)` on a dirty board commits the pending change and advances the bare origin's HEAD (coalescing still pushes); (b) `Sync` seeds `.gitignore` with the lock/manifest patterns via `ensureLockfilesIgnored` (the committed `.gitignore` contains `*.lock`, `*.swaplock`, and the render-manifest pattern); (c) the absorbing push lock file is created at `filepath.Join(boardPath, "board.push.lock")` (unchanged name/location) — assert the path board uses, e.g. by confirming a concurrent second `Sync` serializes rather than erroring; (d) `Sync(boardPath, false, true)` (skipPush) commits locally but does not advance the origin. Do not over-specify assertion shapes.
- **Commit:** `test(boardengine): cover Sync parity after delegating to fabricengine.CoalescePush`

## Batch Tests

`verify: go test -tags integration ./internal/boardengine/...` runs the new tagged `sync_integration_test.go` under the new hermetic `TestMain`. `boardengine`'s existing untagged tests continue to run in the same invocation and must stay green (the `Sync` signature and board's public surface are unchanged). The board git fixture is built inline with `lyxtest.MustRun` because `internal/gitrepo`'s bare-remote helpers are not importable across the test-package boundary.
