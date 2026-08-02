# Batch: bolt-handle

```yaml
task: 'fabric: collapse external API surface onto Commit — stop leaking warp/weft'
batch: bolt-handle
number: 1
cards: 5
verify: go test -tags integration ./internal/fabricengine/ ./internal/boardengine/ ./internal/fabriccli/
depends-on: []
```

## Batch Scope

Introduce the Fabric-owned `Bolt` handle (methods `Commit`/`Push`/`Sync`) that names the unpaired weft:main area without exposing warp/weft, migrate board and clone onto it, then unexport the two primitives that lose their last external caller as a result (`CoalescePush`→`coalescePush`, `CommitWeftAt`→`commitWeftAt`). This batch delivers the `Bolt` type the rest of the collapse leans on and proves board's coalescing still holds through it. `PushWeftAt` is NOT unexported here — the three round-loop CLIs still call it (that happens in batch 2). `CoalescePushBothAt` stays exported. Batch-local decision: `Bolt.Sync` reuses the (now-private) `coalescePush` loop rather than reimplementing it, so there is one coalescing implementation.

## Cards

### Card 1: Create the `Bolt` handle

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/boardweft.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:**
  - `internal/fabricengine/bolt.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/bolt.go` with an exported type `Bolt` wrapping a single weft:main-backed repo path (a `path string` field), constructed by an exported `NewBolt(repoPath string) *Bolt` (or `Bolt{...}` literal helper). Give it three methods: `Commit(message string, opts SyncOptions) (sha string, committed bool, err error)` delegating to the existing package-level `CommitWeftAt(b.path, message, opts)` logic; `Push(opts SyncOptions) error` delegating to `PushWeftAt(b.path, opts)`; and `Sync(step func() (progressed bool, err error)) error` that runs the absorbing-lock loop by calling `CoalescePush(<b's push-lock path>, step)`. The push-lock path MUST be byte-identical to what `boardengine/sync.go` uses today (`filepath.Join(b.path, "board.push.lock")`) so board's existing `board.push.lock` serialization is preserved — do not invent a new lock filename. `Bolt` must not construct any geometry token itself; it receives its already-resolved repo path from the caller (Hub Geometry Invariant). Keep the new file's comments in the trimmed `golang-comments` shape (what+why doc comment on `Bolt` and each method, why-only inline). This card leaves `CoalescePush`/`CommitWeftAt`/`PushWeftAt` exported (still are) — the weftgit.go edit is only to add any small helper needed, or may be empty; if no weftgit.go change is needed, drop it from Edits.
- **Commit:** `feat(fabric): add Bolt handle for the unpaired weft:main area`

### Card 2: Migrate boardengine.Sync onto Bolt

- **Context:**
  - `internal/fabricengine/bolt.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/boardengine/board.go`
- **Edits:**
  - `internal/boardengine/sync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `boardengine.Sync(boardPath, skipGit, skipPush)` (`sync.go:46`) to compose its coalescing through `fabricengine.NewBolt(boardPath).Sync(step)` instead of calling `fabricengine.CoalescePush(filepath.Join(boardPath, pushLockFile), step)` directly. Board KEEPS its per-iteration step closure exactly as today: `ensureLockfilesIgnored(boardPath)` + `commitDirty(boardPath)` (which holds board's own `board.lock`/`writeLockFile` and calls the commit primitive) + the `skipPush`-gated push, returning `committed` as the `progressed` signal. Replace board's direct `fabricengine.CommitWeftAt`/`fabricengine.PushWeftAt` calls inside the step closure with `bolt.Commit(...)`/`bolt.Push(...)` on the same `Bolt` value. `board.push.lock` (`pushLockFile` const, `sync.go:36`) must remain the absorbing lock held once for the whole loop. Trim `sync.go`'s long how-it-works doc comments to the `golang-comments` shape.
- **Commit:** `refactor(board): route Sync coalescing through fabric.Bolt`

### Card 3: Migrate fabric clone's board-dir ops onto Bolt

- **Context:**
  - `internal/fabricengine/bolt.go`
  - `internal/fabricengine/clone.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabriccli/clone.go`, replace the one-shot `fabricengine.CommitWeftAt(res.BoardDir, "fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{})` (`clone.go:77`) and `fabricengine.PushWeftAt(res.BoardDir, fabricengine.SyncOptions{})` (`clone.go:80`) with `b := fabricengine.NewBolt(res.BoardDir)` then `b.Commit("fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{})` and `b.Push(fabricengine.SyncOptions{})`. Preserve the existing `output.Err(out, err.Error())` handling on each. `res.BoardDir` continues to come from `fabricengine.CloneHub(...)`'s `CloneResult`.
- **Commit:** `refactor(fabric): clone board-dir writes via Bolt`

### Card 4: Unexport CoalescePush and CommitWeftAt

- **Context:**
  - `internal/boardengine/sync.go`
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/bolt.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the exported `CoalescePush` (`coalesce.go:30`) to package-private `coalescePush`, updating its two remaining callers, both in-package: `CoalescePushBothAt` (`coalesce.go:162`) and `Bolt.Sync` (`bolt.go`). Rename the exported package-level `CommitWeftAt` (`weftgit.go:564`) to `commitWeftAt`, updating its sole remaining caller `Bolt.Commit` (`bolt.go`). Confirm via grep that no caller outside the `fabricengine` package references either symbol after cards 2–3 (board and clone now go through `Bolt`). `CoalescePushBothAt` and `PushWeftAt` stay exported. Update doc-comment mentions of `CoalescePush`/`CommitWeftAt` in the edited files to the new casing AND reword any now-false rationale: `CoalescePush`'s doc comment (`coalesce.go:27-29`) currently says "It is exported because boardengine … calls it directly" — after card 2 (board routes through `Bolt.Sync`) and this unexport, both clauses are false; reword to reflect that it is unexported because `Bolt` (and in-package `CoalescePushBothAt`) is now the sole coalescing entry point. Trim to the `golang-comments` shape.
- **Commit:** `refactor(fabric): unexport coalescePush and commitWeftAt`

### Card 5: Port coalescing tests onto Bolt

- **Context:**
  - `internal/fabricengine/bolt.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/fabricengine/coalesce_test.go`
  - `internal/fabricengine/commitweftat_test.go`
  - `internal/boardengine/sync_integration_test.go`
- **Creates:**
  - `internal/fabricengine/bolt_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `coalesce_test.go` (the untagged loop-skeleton tests `TestCoalescePush_LoopsWhileProgressed`/`_StepErrorAbortsImmediately`/`_ReleasesLockOnReturn`) to call `coalescePush` (new casing). Update `commitweftat_test.go` (`//go:build integration`) to call `commitWeftAt` (new casing) — it stays a valid in-package test of that primitive. Create `internal/fabricengine/bolt_integration_test.go` (`//go:build integration`) porting the coalescing expectations from the discussion onto `Bolt.Commit`/`Bolt.Push`/`Bolt.Sync`: a dirty repo commits+pushes, a clean repo is a no-op, `SkipGit` short-circuits, and `Bolt.Sync` holds its single absorbing lock across a burst. Confirm `boardengine/sync_integration_test.go` still passes unchanged in intent — in particular `TestSync_ConcurrentCallSerializesOnBoardPushLock` must still serialize on `board.push.lock`; adjust only if the lock path assertion needs re-pointing at the same literal `Bolt` now constructs. Add a `TestMain` calling `lyxtest.HermeticGitEnv()` in the new test file's package only if the package does not already have one (fabricengine already does — verify, do not duplicate).
- **Commit:** `test(fabric): cover Bolt commit/push/sync coalescing`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/ ./internal/boardengine/ ./internal/fabriccli/` runs the ported `coalesce_test.go`/`commitweftat_test.go`/new `bolt_integration_test.go` plus `boardengine/sync_integration_test.go` (the board coalescing/serialization suite that proves the migration preserved behaviour) and the fabriccli clone tests. The `-tags integration` is required because the real-git coalescing and board-push-lock tests carry that build tag. Scope is the three packages this batch edits.
