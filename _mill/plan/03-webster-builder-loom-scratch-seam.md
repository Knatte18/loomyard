# Batch: webster-builder-loom-scratch-seam

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
batch: webster-builder-loom-scratch-seam
number: 3
cards: 8
verify: go test ./internal/websterengine/... ./internal/webstercli/... ./internal/builderengine/... ./internal/buildercli/... ./internal/loomengine/... ./cmd/lyx/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/... ./internal/buildercli/... ./internal/loomengine/...
depends-on: [1]
```

## Batch Scope

This batch relocates webster's, builder's and loom's never-tracked transients out of `_lyx` into the mirrored `.lyx` subpath, behind an explicit scratch-dir seam per module: `websterengine.ScratchDir(l)`, `builderengine.ScratchDir(l)`, and — since loom has no `Dir(l)` to mirror — a re-pointed `loomengine.LoomStatusLock(l)` stated outright rather than derived by analogy.
The relocated artifacts are webster's `pause`, `prompts/*`, `run.lock`, `mutate.lock` and `state.json.lock`;
builder's `pause`, `run.lock`, `mutate.lock` and `state.json.lock`;
and loom's `status.json.lock`.
`state.json`, `status.json`, `outcome.yaml`, `summary.md` and every report stay under `_lyx`.

It is one batch because webster and builder are structural twins — the same five accessor shapes, the same `RunDeps`/`BeginDeps` threading, the same CLI verb set — so an implementer holding one holds the other, and because all three modules' path constructors are asserted together in the single `cmd/lyx/constructoranchoring_test.go` table, which cannot be split across parallel batches without a write conflict.

**External interface batch 5 consumes:** `websterengine.ScratchDir(l)`, `builderengine.ScratchDir(l)`, `loomengine.LoomStatusLock(l)`.

**Batch-local decision — the two-tree accessors take a second parameter, not a struct.**
`LoadState`/`SaveState` in both engines become `(durableDir, scratchDir string, ...)`, because each derives `state.json` *and* `state.json.lock` from one argument today and this task splits exactly those two.
Accessors naming only a transient (`AcquireStateMutation`, `RequestPause`, `PauseRequested`, `ClearPause`, `PauseFlagPath`, `RunActive`, `PromptsDir`) keep their single-`dir` shape and simply receive the scratch dir instead;
their parameter is renamed from `websterDir`/`builderDir` to `scratchDir` so a call site passing the wrong tree reads wrong.

**Batch-local decision — `RunDeps`/`BeginDeps` gain `ScratchDir`, and every deps-constructing site supplies it.**
Webster's engine-internal `AcquireStateMutation`/`ClearPause`/`PauseRequested`/`LoadState`/`SaveState` calls all read `deps.WebsterDir` today;
builder's read `deps.BuilderDir`.
Adding the field is what makes those internal calls re-keyable without the engines deriving anything.

## Cards

### Card 15: add websterengine.ScratchDir and re-point PromptsDir

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/perchengine/identity.go`
- **Edits:**
  - `internal/websterengine/state.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add `func ScratchDir(l *lyxcwd.Location) string` returning `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, websterDirName)`, documented as `Dir`'s never-tracked sibling holding the pause flag, the rendered fork prompts, and every `*.lock`, at the mirrored subpath of the `_lyx/webster` content each relates to.
  Re-point `PromptsDir` to `filepath.Join(ScratchDir(l), "prompts")` and update its godoc: prompts are machine-local re-renderable artifacts, and they now live under `.lyx` rather than being held out of weft commits by an exclude pattern.
  Update `websterDirName`'s godoc to say the segment is joined onto both `lyxdirs.LyxDirName` and `lyxdirs.DotLyxDirName`.
  `Dir` and `ReportsDir` are unchanged.
- **Commit:** `feat(websterengine): add ScratchDir and move PromptsDir under .lyx`

### Card 16: re-key webster's state lock, mutate lock and pause flag

- **Context:**
  - `internal/websterengine/state.go`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
- **Edits:**
  - `internal/websterengine/pause.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/beginbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** this card re-signatures webster's transient accessors and re-keys every in-engine caller.
  In `state.go` (also edited by card 15, same batch): `AcquireStateMutation(scratchDir string)` MkdirAlls and locks `filepath.Join(scratchDir, stateMutateLockName)`;
  `LoadState(websterDir, scratchDir string)` and `SaveState(websterDir, scratchDir string, st *State)` keep `path = filepath.Join(websterDir, stateFileName)` but resolve `lockPath = filepath.Join(scratchDir, stateFileName+".lock")` and `os.MkdirAll(scratchDir, 0o755)` before writing.
  Update `stateMutateLockName`'s godoc, which today says "Excluded from weft commits like every other `*.lock` (see webstercli's websterWeftPathspec)" — that mechanism is gone;
  it now lives under `.lyx` and is never in a weft worktree at all.
  In `pause.go`, rename the parameter of `pauseFlagPath`, `RequestPause`, `PauseRequested` and `ClearPause` from `websterDir` to `scratchDir` and update their godocs and error strings accordingly ("create webster scratch dir %s").
  In `runlevel.go`: add `ScratchDir string` to `RunDeps` with a godoc naming it the `.lyx/webster` tree, update the struct's summary comment that enumerates `PlanDir, WebsterDir, ReportsDir, PromptsDir, WorktreeRoot`;
  change `RunActive`'s parameter to `scratchDir` and resolve `run.lock` from it;
  in `Run`, `os.MkdirAll(deps.ScratchDir, 0o755)` beside the existing `MkdirAll(deps.WebsterDir, ...)`, take the run lock at `filepath.Join(deps.ScratchDir, runLockName)`, and re-key both `ClearPause(deps.WebsterDir)` calls and the `AcquireStateMutation(deps.WebsterDir)` calls to `deps.ScratchDir`, and every `LoadState`/`SaveState` call to the two-argument form `(deps.WebsterDir, deps.ScratchDir)`.
  Update `runLockName`'s godoc to say it lives in the scratch dir.
  In `beginbatch.go`: add `ScratchDir string` to `BeginDeps`, update the struct comment that enumerates `WebsterDir, ReportsDir, PromptsDir`, change `PauseRequested(deps.WebsterDir)` to `PauseRequested(deps.ScratchDir)`, and update `ErrPaused`'s godoc, which names "deps.WebsterDir's pause flag".
  `OutcomePath`, `SummaryPath`, `verifyEveryBatchDone`, `archiveStateFile`, `archiveStaleOutcome` and `ArchiveStaleSummary` all keep taking `websterDir` and stay under `_lyx` — do not touch them.
- **Commit:** `refactor(websterengine): resolve every transient from the scratch dir`

### Card 17: thread webster's scratch dir through webstercli

- **Context:**
  - `internal/websterengine/state.go`
  - `internal/websterengine/pause.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/beginbatch.go`
- **Edits:**
  - `internal/webstercli/cli.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/awaitbatch.go`
  - `internal/webstercli/status.go`
  - `internal/webstercli/pause.go`
  - `internal/webstercli/ownership.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `cli.go`, add a `websterScratchDir string` field beside `websterDir`, populate it in `PersistentPreRunE` with `websterengine.ScratchDir(layout)` immediately after `c.websterDir = websterengine.Dir(layout)`, and extend the field-group comment that says "planDir, websterDir, reportsDir, and promptsDir are the lyxcwd-resolved _lyx dirs" to distinguish the `.lyx`-resolved scratch dir.
  Retarget every call site: `AcquireStateMutation`, `RequestPause` and `PauseRequested` take `c.websterScratchDir`;
  `LoadState`/`SaveState` take `(c.websterDir, c.websterScratchDir)`;
  `RunDeps`/`BeginDeps` literals gain `ScratchDir: c.websterScratchDir`;
  `ownerlessRunWarnings`'s parameter is renamed to `scratchDir` and its `websterengine.RunActive` call is fed the scratch dir, so every caller in `beginbatch.go`, `awaitbatch.go`, `recordbatch.go` and `recoverbatch.go` passes `c.websterScratchDir`.
  `OutcomePath(c.websterDir)`/`SummaryPath(c.websterDir)` in `recordbatch.go` keep the durable dir.
  In `weft.go` no change is needed beyond what batch 1 already did — its pathspec still names `_lyx` only, which is now correct by construction rather than by exclusion.
- **Commit:** `refactor(webstercli): pass websterengine.ScratchDir to every transient accessor`

### Card 18: add builderengine.ScratchDir and re-key builder's transients

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/pause.go`
  - `internal/state/state.go`
- **Edits:**
  - `internal/builderengine/state.go`
  - `internal/builderengine/pause.go`
  - `internal/builderengine/runlevel.go`
  - `internal/builderengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** mirror cards 15 and 16 for builder, artifact for artifact.
  In `state.go`: add `func ScratchDir(l *lyxcwd.Location) string` returning `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, builderDirName)`;
  `AcquireStateMutation(scratchDir string)` locks `filepath.Join(scratchDir, stateMutateLockName)`;
  `LoadState(builderDir, scratchDir string)` / `SaveState(builderDir, scratchDir string, st *State)` keep `state.json` in `builderDir` and resolve the lock from `scratchDir`, MkdirAlling it first;
  update `builderDirName`'s godoc to name both tokens, and `stateMutateLockName`'s godoc, which today says "Excluded from weft commits like every other `*.lock` (see buildercli's builderWeftPathspec)".
  In `pause.go`: rename the parameter of `PauseFlagPath`, `RequestPause`, `PauseRequested` and `ClearPause` from `builderDir` to `scratchDir`, updating godocs and error strings.
  In `runlevel.go`: add `ScratchDir string` to `RunDeps`;
  `os.MkdirAll(deps.ScratchDir, 0o755)` beside the existing builder-dir MkdirAll;
  take the run lock at `filepath.Join(deps.ScratchDir, runLockName)`;
  re-key both `ClearPause(deps.BuilderDir)` calls, the `AcquireStateMutation(deps.BuilderDir)` call, and every `LoadState`/`SaveState` call;
  update `runLockName`'s godoc.
  In `spawn.go`: add `ScratchDir string` to `SpawnDeps` beside `BuilderDir`, extend the struct's own doc where it enumerates its path fields, change `PauseRequested(deps.BuilderDir)` to `PauseRequested(deps.ScratchDir)`, and change both `SaveState(deps.BuilderDir, deps.State)` calls to `SaveState(deps.BuilderDir, deps.ScratchDir, deps.State)`.
  `ReportsDir`, `ArchiveStateFile`, `ArchiveStaleOutcome` and the `outcomeFileName` join keep the durable dir.
- **Commit:** `feat(builderengine): add ScratchDir and resolve every transient from it`

### Card 19: thread builder's scratch dir through buildercli

- **Context:**
  - `internal/builderengine/state.go`
  - `internal/builderengine/pause.go`
  - `internal/builderengine/runlevel.go`
- **Edits:**
  - `internal/buildercli/cli.go`
  - `internal/buildercli/run.go`
  - `internal/buildercli/spawnbatch.go`
  - `internal/buildercli/poll.go`
  - `internal/buildercli/status.go`
  - `internal/buildercli/pause.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `cli.go`, add a `builderScratchDir string` field beside `builderDir` and populate it with `builderengine.ScratchDir(layout)` immediately after `c.builderDir = builderengine.Dir(layout)`.
  Retarget: `RequestPause` and `PauseRequested` take `c.builderScratchDir`;
  `AcquireStateMutation` takes `c.builderScratchDir`;
  every `LoadState`/`SaveState` in `poll.go`, `spawnbatch.go` and `status.go` takes `(c.builderDir, c.builderScratchDir)`;
  the `RunDeps` literal in `run.go` and the `SpawnDeps`-style literal in `spawnbatch.go` gain `ScratchDir: c.builderScratchDir`.
  Leave `weft.go`'s pathspec naming `_lyx` only.
- **Commit:** `refactor(buildercli): pass builderengine.ScratchDir to every transient accessor`

### Card 20: move loom's status.json.lock under .lyx

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `internal/reedengine/lock.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/preflight.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** re-point `LoomStatusLock` to `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "status.json.lock")` and rewrite its godoc: it no longer "shares LoomStatusFile's AnchorPath anchoring" as a joint `_lyx` path — it is stated outright as the mirrored `.lyx` sibling, because `loomengine` has no `Dir(l)` accessor to mirror and deriving it by analogy would be exactly the kind of implicit geometry this task removes.
  `LoomStatusFile` stays under `lyxdirs.LyxDirName` — loom's orchestration status is durable and weft-synced by design.
  In `preflight.go`, the `state.ReadJSONStrict[Status](LoomStatusFile(l), LoomStatusLock(l))` call needs no argument change, but it **does** need an explicit `os.MkdirAll(filepath.Dir(LoomStatusLock(l)), 0o755)` immediately before it, because nothing on that path creates the lock's new parent: `state.ReadJSONStrict`'s own doc comment in `internal/state/state.go` states outright that, unlike `ReadJSON`, it does **not** create missing parent directories, and `internal/lock`'s `AcquireReadLock`/`AcquireWriteLock` perform no `MkdirAll` either (gofrs/flock opens the lock file with `O_CREATE` but never creates parents — the same gap `internal/reedengine/lock.go` already works around with its own `MkdirAll`-before-lock).
  Until now the lock sat beside `status.json` under `_lyx`, so its parent always existed by the time the guarding `os.Stat(LoomStatusFile(l))` succeeded;
  moving it to `.lyx` breaks that coincidence, and without the `MkdirAll` Preflight would escalate a missing-`.lyx` worktree to a hard infra error instead of honouring its report-not-error contract.
  Add a comment recording exactly that.
- **Commit:** `refactor(loomengine): move status.json.lock under .lyx`

### Card 21: update the cross-module constructor anchoring table

- **Context:**
  - `internal/websterengine/state.go`
  - `internal/builderengine/state.go`
  - `internal/loomengine/config.go`
  - `internal/perchengine/identity.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in both `TestConstructorAnchoring_Unanchored` and `TestConstructorAnchoring_SubpathAnchored`, move the `loomengine.LoomStatusLock` and `websterengine.PromptsDir` assertions out of the `_lyx`-durable group and into the `.lyx` group, with expected values `filepath.Join(dotLyxBase, "status.json.lock")` and `filepath.Join(dotLyxBase, "webster", "prompts")`.
  Add three new assertions to the `.lyx` group in both tests: `websterengine.ScratchDir(l)` → `filepath.Join(dotLyxBase, "webster")`, `builderengine.ScratchDir(l)` → `filepath.Join(dotLyxBase, "builder")`, and `perchengine.ScratchDir(l)` → `filepath.Join(dotLyxBase, "perch")`.
  In this batch `dotLyxBase` is still `filepath.Join(worktree, ".lyx")` — batch 4 is what re-anchors it onto `anchor`;
  so in `TestConstructorAnchoring_SubpathAnchored` the five `.lyx` entries listed above must be asserted against an **`anchor`**-based base while `logger.WorktreeLogsDir`/`scoutengine.DaemonStateFile`/`DaemonLock` keep the `worktree`-based one.
  Introduce a second local, `dotLyxAnchorBase := filepath.Join(anchor, ".lyx")`, for the already-migrated group and leave `dotLyxBase` for the not-yet-migrated one, with a comment stating batch 4 collapses the two back into one.
  Update the file-header comment to describe the three groups as they now stand.
- **Commit:** `test(cmd/lyx): record the scratch-dir constructors in the anchoring table`

### Card 22: update webster's, builder's and loom's own tests

- **Context:**
  - `internal/websterengine/state.go`
  - `internal/websterengine/pause.go`
  - `internal/websterengine/runlevel.go`
  - `internal/builderengine/state.go`
  - `internal/builderengine/pause.go`
  - `internal/builderengine/runlevel.go`
  - `internal/loomengine/config.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/websterengine/state_test.go`
  - `internal/websterengine/pause_test.go`
  - `internal/websterengine/runlevel_test.go`
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/integration_test.go`
  - `internal/websterengine/webstergeom_test.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/verbs_test.go`
  - `internal/webstercli/weft_integration_test.go`
  - `internal/builderengine/state_test.go`
  - `internal/builderengine/pause_test.go`
  - `internal/builderengine/runlevel_test.go`
  - `internal/builderengine/spawn_test.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/pause_test.go`
  - `internal/buildercli/spawnbatch_test.go`
  - `internal/buildercli/status_test.go`
  - `internal/buildercli/smoke_test.go`
  - `internal/buildercli/weft_integration_test.go`
  - `internal/loomengine/loomstatus_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** update every call to the re-signatured functions, and add the per-module path and locking coverage the seam needs:
  (a) in `webstergeom_test.go` and `loomstatus_test.go`, assert each relocated artifact resolves under `.lyx` at the mirrored subpath (`ScratchDir`, `PromptsDir`, `LoomStatusLock`) while the durable ones (`Dir`, `ReportsDir`, `LoomStatusFile`, `PlanDir`) stay under `_lyx`, for both an unanchored and a subpath-anchored `*lyxcwd.Location`;
  (b) in `state_test.go` for both engines, assert `SaveState` writes `state.json` into the durable dir and `state.json.lock` into the scratch dir, that the durable dir ends up containing **no** `.lock` file, and that a `LoadState` after a `SaveState` round-trips when the two directories differ;
  (c) in `state_test.go` for both engines, assert `AcquireStateMutation` serialises correctly — a second acquire blocks or fails as the existing test already asserts — with the lease file in the scratch dir;
  (d) in `pause_test.go` for both engines, assert `RequestPause`/`PauseRequested`/`ClearPause` all agree against a scratch dir that is not the durable dir, and that no `pause` file appears in the durable dir;
  (e) in `runlevel_test.go` for both engines, assert `run.lock` is taken in the scratch dir and that `ErrRunBusy` still fires for a second concurrent `Run`;
  (f) in `buildercli/pause_test.go` and `webstercli/verbs_test.go`, assert the CLI pause verb and the engine's own pause check resolve the same file — this is the regression the whole seam exists for, so assert it through the CLI, not by calling the engine accessor twice.
  Where a test fixture builds a webster/builder dir by hand, give it a sibling scratch dir under a `.lyx` path rather than reusing the same temp dir, or the split is untested.
  `internal/buildercli/smoke_test.go` carries `//go:build smoke` and `internal/websterengine/runlevel_test.go`, `internal/websterengine/integration_test.go`, `internal/webstercli/weft_integration_test.go`, `internal/buildercli/weft_integration_test.go` and `internal/loomengine/preflight_integration_test.go` carry `//go:build integration` — keep each tag line first in its file.
- **Commit:** `test: cover the webster, builder and loom scratch-dir split`

## Batch Tests

`verify: go test ./internal/websterengine/... ./internal/webstercli/... ./internal/builderengine/... ./internal/buildercli/... ./internal/loomengine/... ./cmd/lyx/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/... ./internal/buildercli/... ./internal/loomengine/...` — the five edited packages plus `cmd/lyx` for card 21's table, and a tagged run for the five `//go:build integration` files card 22 edits.
`internal/buildercli/smoke_test.go` (`//go:build smoke`) is deliberately **not** run: it drives a real substrate spawn and is out of scope for a per-batch gate;
card 22 only re-signatures its calls, and the overview's module-wide `go vet -tags integration ./...` does not compile it either, so its compile is confirmed by the repo-wide done gate rather than here.
This is the batch's one acknowledged coverage gap and is stated deliberately rather than left silent.

Covered files: the twenty-one test files in card 22 plus `cmd/lyx/constructoranchoring_test.go`.

The assertions that matter are (b), (d) and (f): a test that only checks "the lock is in scratch" passes an implementation writing to both trees, so the negative half — the durable dir holds no `.lock` and no `pause` — is what actually pins the move.
And (f) is the one that catches the failure mode the discussion names: a CLI pause verb still passing the durable dir writes `_lyx/<module>/pause` while the engine reads `.lyx/<module>/pause`, and pause silently stops working with no test on either side alone failing.
