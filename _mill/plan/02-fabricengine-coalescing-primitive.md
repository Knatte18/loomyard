# Batch: fabricengine-coalescing-primitive

```yaml
task: 'fabric: warp-side commit lock + push coalescing'
batch: fabricengine-coalescing-primitive
number: 2
cards: 3
verify: go test -tags integration ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

Adds the generic loop-until-clean coalescing primitive `CoalescePush` to `internal/fabricengine`, plus the fabric-side two-sided rebase-free push entry `CoalescePushBothAt` that batch 3's CLI bypass handler wires in and that board (batch 4) reuses the generic half of. Also adds the `.weft/`-based push-lock constant and refactors the lock-dir helper so a no-`Fabric`-instance caller (the detached push child) can create/seed the lock dir. This batch delivers the pure coordination skeleton and the fabric push policy; it does NOT yet rewire any caller (that is batches 3 and 4). External interface downstream batches consume: `CoalescePush(lockPath string, step func() (bool, error)) error` (board) and `CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) error` (fabric CLI bypass).

Batch-local decisions: (1) `CoalescePush` is exported because `boardengine` (a separate package that already imports `fabricengine`) calls it directly. (2) The primitive is pure coordination — it acquires/holds the absorbing lock and drives the step, and contains NO commit, stage, ensure-ignored, or push logic (that all lives in each caller's step). (3) A new file `coalesce.go` hosts both the generic primitive and the fabric step so the loop and fabric's push policy sit together; the generic primitive stays caller-agnostic.

## Cards

### Card 3: Add `weftPushLockFile` constant and extract package-level `ensureWeftLockDirAt`

- **Context:**
  - `internal/fabricengine/commit.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/fabricengine/weftgit.go`, add `weftPushLockFile = "fabric.push.lock"` to the existing `const (...)` block that defines `weftLockDirName` and `weftWriteLockFile`, with a comment noting it names the absorbing push lock the coalescing loop holds, living inside `.weft/` alongside the write lock (already git-excluded by `seedWeftArtifactExcludes`'s whole-directory `weftLockDirName + "/"` entry — no new exclude entry is added, per Shared Decision `lock-artifact-under-weft`).
  - Extract a package-level `func ensureWeftLockDirAt(weftPath string) (string, error)` containing the current body of the `(f *Fabric) ensureWeftLockDir()` method (mkdir `filepath.Join(weftPath, weftLockDirName)`, then `seedWeftArtifactExcludes(weftPath)`, returning the dir path). Rewrite `(f *Fabric) ensureWeftLockDir()` to `return ensureWeftLockDirAt(f.weftPath)`. This lets the detached push child (which has no `Fabric` instance) create and exclude-seed the lock dir. Preserve `ensureWeftLockDir`'s existing godoc on the method; give `ensureWeftLockDirAt` a short godoc stating it is the no-`Fabric`-instance form.
  - Do not change `seedWeftArtifactExcludes`, the exclude entry list, or any other behavior.
- **Commit:** `refactor(fabricengine): add fabric.push.lock constant and package-level ensureWeftLockDirAt`

### Card 4: `CoalescePush` primitive and fabric two-sided rebase-free push step

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/commit.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/lock/lock.go`
  - `internal/logger/logger.go`
  - `internal/boardengine/sync.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/coalesce.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `internal/fabricengine/coalesce.go` (package `fabricengine`).
  - Add `func CoalescePush(lockPath string, step func() (progressed bool, err error)) error`: `l, err := lock.AcquireWriteLock(lockPath)` (wrap acquire error as `fmt.Errorf("fabricengine: acquire push lock: %w", err)`); `defer func() { _ = l.Release() }()`; then `for { progressed, err := step(); if err != nil { return err }; if !progressed { return nil } }`. The primitive holds the one absorbing lock across the whole loop and contains no other logic. Godoc must state the exit contract (loop while `progressed`, exit on first no-progress or on error, which propagates) per Shared Decision `coalescing-loop-in-fabricengine-via-closures`.
  - Add `func CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) error`: honor `opts.SkipGit || opts.SkipPush` by returning nil immediately (match `PushWeftAt`/`PushWarpAt` gating). Guard an empty `weftPath` FIRST: the absorbing push lock has its only sanctioned home under `weftPath`'s `.weft/` (a host-root lock is forbidden by Shared Decisions `lock-artifact-under-weft` / `no-host-root-gitrepo-push-lock`), so `if weftPath == ""` return `fmt.Errorf("fabricengine: CoalescePushBothAt requires a weft path for the absorbing push lock")` rather than falling back to `warpPath` (which would put a lock at the pristine host root) or defaulting to cwd (which `ensureWeftLockDirAt("")` would do — `mkdir .weft` + `git rev-parse` relative to the process cwd). This is a latent edge only: the detached push child always supplies both `warpPath` and `weftPath` (see `SpawnDetachedPush` and `Fabric.Commit`'s `spawnDetachedPushFn(f.warpPath, f.weftPath)` call), so production never hits the guard. Then build the absorbing lock path: `lockDir, err := ensureWeftLockDirAt(weftPath)` then `lockPath := filepath.Join(lockDir, weftPushLockFile)`. Return `CoalescePush(lockPath, step)` where `step` is the two-sided rebase-free push described below. (A warp-only push, `warpPath != "" && weftPath == ""`, is not a supported coalescing entry — the guard rejects it; `warpPath` may still be empty when `weftPath` is present, which pushes only the weft side.)
  - The `step` closure, per Shared Decisions `push-only-loop-exit-on-head-stability` and `rebase-free-async-push`: read `beforeWarp` and `beforeWeft` via a helper `headOrEmpty(path string) (string, error)` that calls `gitrepo.New(path).CurrentSHA()` and maps `errors.Is(err, gitrepo.ErrNoCommits)` to `("", nil)`, propagating any other error. For each side with a non-empty path AND a non-empty (born) HEAD, call a helper `pushRebaseFreeLogged(path string) error` that runs `gitrepo.New(path).PushRebaseFree()`, maps `errors.Is(err, gitrepo.ErrPushRejected)` to a `logger.Warn(...)` line (naming the diverged path, stating commits are left unpushed) + `return nil`, and propagates any other error. After pushing, re-read `afterWarp`/`afterWeft` via `headOrEmpty`. Return `progressed = (afterWarp != beforeWarp) || (afterWeft != beforeWeft), nil`. A side that is unborn is skipped for pushing (nothing to push) but still participates in the before/after HEAD comparison (its empty-string HEAD is stable).
  - Use imports: `errors`, `fmt`, `path/filepath`, and the `gitrepo`, `lock`, and `logger` packages (their source files are already listed in this card's Context). Do NOT call `gitrepo.PushCoalesced` or `PushWarpAt`/`PushWeftAt` from the fabric step — the async loop is rebase-free and lock-free per side (serialization is the absorbing lock), which is exactly what eliminates the host-root `.gitrepo-push.lock` (Shared Decision `no-host-root-gitrepo-push-lock`).
- **Commit:** `feat(fabricengine): coalescing push primitive and two-sided rebase-free push step`

### Card 5: Tests for the loop skeleton and the fabric two-sided push

- **Context:**
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/gitrepo/push.go`
  - `internal/lock/lock.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/commit_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/coalesce_test.go`
  - `internal/fabricengine/coalesce_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `internal/fabricengine/coalesce_test.go` (package `fabricengine`, UNTAGGED — it spawns no git, only a `flock` on a `t.TempDir()` path, which the Test Tier Purity Invariant permits): drive `CoalescePush` with scripted `step` closures and assert the exit contract: (a) a step returning `progressed=true` N times then `false` runs exactly N+1 times; (b) a step returning `false` on the first call runs exactly once (proves the no-spin property — the loop does NOT depend on `hasUnpushed`); (c) a step returning a non-nil error aborts immediately and `CoalescePush` returns that error; (d) the lock file at `lockPath` is released after `CoalescePush` returns (a second `lock.AcquireWriteLock` on the same path succeeds without blocking). Use a call counter captured in the closure.
  - `internal/fabricengine/coalesce_integration_test.go` (package `fabricengine`, `//go:build integration`): exercise `CoalescePushBothAt` against a real warp+weft pair with bare origins, reusing this package's existing fixture helpers (`newPlainWarpRepo`, `commitWarp`, `currentSHA`, `newFabric` from `index_integration_test.go`; `seedFabricConfig`/`writeWarpFile` from `commit_integration_test.go`) plus `lyxtest.MustRun` for setting up bare remotes and upstream tracking. Assert: (a) with an unpushed commit on each side, `CoalescePushBothAt` advances both bare upstreams to match local HEAD and returns nil; (b) after a warp-via-fabric push, `.gitrepo-push.lock` does NOT exist at the warp worktree root (Shared Decision `no-host-root-gitrepo-push-lock`); (c) with a diverged warp remote (a second clone pushed a commit warp lacks), `CoalescePushBothAt` returns nil (not an error), leaves the warp bare's HEAD unadvanced by this call, mutates no warp working-tree file, and does not spin (the call returns promptly). Do not over-specify assertion shapes.
- **Commit:** `test(fabricengine): cover CoalescePush loop-exit and two-sided rebase-free push`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/...` runs the untagged `coalesce_test.go` loop-skeleton tests and the tagged `coalesce_integration_test.go` real-git tests together (the `-tags integration` build includes untagged files too). `fabricengine` already has `testmain_test.go` calling `lyxtest.HermeticGitEnv()` (Hermetic Git Test Environment Invariant), which the new integration test inherits. The untagged `coalesce_test.go` uses only a `flock` on a temp path, spawning no git, so it satisfies the Test Tier Purity Invariant.
