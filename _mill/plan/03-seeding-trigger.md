# Batch: seeding-trigger

```yaml
task: "Relocate producer prompt files into a stencils/ directory"
batch: "seeding-trigger"
number: 3
cards: 5
verify: go build ./... && go test ./internal/fabricengine/... ./internal/boardengine/... ./cmd/lyx/... ./tools/...
depends-on: [2]
```

## Batch Scope

This batch makes seeding actually happen: once per process, at `cmd/lyx`'s root pre-run, committed into the board repo under `board.lock` with an explicit positive pathspec.
It lands before the remaining three engine families move (batches 4, 5, 6) so that every stencil is seeded the moment its engine starts reading from disk, rather than leaving a window where a relocated prompt has no copy on the board.

Three pieces: the `board.lock` filename becomes single-declarer in `internal/fabricengine` with `internal/boardengine` aliasing it, so a package outside `boardengine` can take the same lock at all; a new mutating `fabricengine` verb acquires that lock and commits the stencils subtree via `gitrepo.StageAndCommit`; and `cmd/lyx` gains the `buildChannel` ldflags var plus the reconcile call in `newRoot()`'s existing `PersistentPreRunE`.

Batch-local decisions.
The seeding commit does **not** use `Bolt`: `Bolt.Sync` takes `board.push.lock` while board's own file writes take `board.lock`, so seeding under `Bolt` would not exclude a concurrent `boardCriticalSection`, and `Bolt.Commit` stages everything in the board repo via `StageAllAndCommit`, which could capture a half-written board.
The verb **commits only and never pushes** — the commit rides board's next push through the existing coalescing path, and pushing per run would fire a push on nearly every `lyx` invocation.
The mutation record is logged rather than enveloped: a pre-run seed emits no verb-outcome envelope, and the Mutation Record Invariant scopes its fixed `mutations`/`partial` keys to envelopes emitted from a verb outcome, so a `lyx board list` that happened to seed fifteen files must not grow `board`'s key set.

## Cards

### Card 12: Make `internal/fabricengine` the single declarer of the `board.lock` filename

- **Context:**
  - `internal/fabricengine/bolt.go`
  - `internal/boardengine/board.go`
- **Edits:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/boardengine/sync.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `package fabricengine`, beside `BoardDirName` in `junctionnames.go`:

  - `const BoardWriteLockFile = "board.lock"` — the exported single declarer, doc-commented as the lock serialising every write to the board directory, held by `internal/boardengine`'s own critical section and by this package's stencil-seeding commit verb.
  - `func BoardWriteLockPath(hub string) string` returning `filepath.Join(BoardDir(hub), BoardWriteLockFile)`.

  `fabricengine` is the right owner because it already owns the board directory itself through `BoardDirName`/`BoardDir`, and the literal is unexported inside `boardengine` today (`writeLockFile`), so it is unreachable from a new package without this move.
  There is no cycle risk: `internal/boardengine/sync.go:19` already imports `internal/fabricengine`.
  This mirrors the shape fabric's clone-time guard already uses for the anchor-marker names — alias, never re-declare.

  In `internal/boardengine/sync.go`, replace the `writeLockFile = "board.lock"` line inside the existing `const` block with an alias `writeLockFile = fabricengine.BoardWriteLockFile`, keeping the identifier `writeLockFile` so its two existing use sites — `sync.go:63` in `commitDirty` and `board.go:109` in `boardCriticalSection` — are untouched.
  Leave `pushLockFile` exactly as it is; it is a separate lock with a separate name and is out of this task's scope.

- **Commit:** `refactor(fabricengine): own the board.lock filename, boardengine aliases it`

### Card 13: Add the stencil-seeding commit verb to `internal/fabricengine`

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/lock/lock.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/stencilcommit.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  New file in `package fabricengine` declaring one mutating verb and its result type:

  - `type StencilSeedResult struct { MutationRecord; SHA string; Committed bool }` — the `MutationRecord` embed is required of every mutating verb's result type by the Mutation Record Invariant.
  - `func CommitSeededStencils(hub string, writtenRelPaths []string, message string, rec *Mutations) (res StencilSeedResult, err error)`

  Behaviour:
  - When `writtenRelPaths` is empty, return the zero result with `Committed: false` and no error, taking no lock and running no git at all. This is the common case on an ordinary run, and it is what keeps the seeding pass free: the verb must never fire a commit when nothing was written.
  - Otherwise acquire the board write lock via `lock.AcquireWriteLock(BoardWriteLockPath(hub))`, wrapping a failure as `fmt.Errorf("fabricengine: acquire board write lock: %w", err)`, and release it with `defer func() { _ = l.Release() }()` — the same idiom already used at `commit.go:186` and `weftgit.go:260`.
  - Build the pathspec by prefixing each entry of `writtenRelPaths` (which arrive relative to the stencils directory, e.g. `loom/loom-template-discussion.md` and `.gitattributes`) with `path.Join(lyxdirs.LyxDirName, "stencils")`, producing board-repo-relative slash-separated paths such as `_lyx/stencils/loom/loom-template-discussion.md`. Use `ScopedPathspec` for this join so the construction stays on the package's own existing helper.
  - Commit via `gitrepo.New(BoardDir(hub)).StageAndCommit(message, pathspec)`. It must be `StageAndCommit` with that explicit positive pathspec, never `StageAllAndCommit` and never `Bolt.Commit` — a stage-all would sweep an unrelated dirty file elsewhere in the board into the seeding commit.
  - Never push. Do not call `Push`, `PushCoalesced`, `pushWeftAt`, or `Bolt.Sync`; the commit rides board's next push through the existing coalescing path.
  - Record mutations on `rec` only after the primitive observably changed state, per the Mutation Record Invariant: one `rec.Append(KindFileWritten, ...)` per entry in `writtenRelPaths`, and a single `rec.Append(KindCommitCreated, BoardDir(hub), sha)` only when `StageAndCommit` reported `committed == true`. No new `Kind` member is needed — `KindFileWritten` and `KindCommitCreated` already exist at `mutation.go:45,50`.
  - Snapshot the record into the result with `defer func() { res.Mutations = rec.Snapshot() }()`, matching `Fabric.Commit`'s existing shape at `commit.go:109`.

  The doc comment must state plainly why this verb exists rather than reusing `Bolt`: `Bolt.Sync` takes `board.push.lock` while board's own writes take `board.lock`, so `Bolt` would not exclude a concurrent `boardCriticalSection`, and `Bolt.Commit` stages everything.

- **Commit:** `feat(fabricengine): add locked, pathspec-scoped stencil seeding commit verb`

### Card 14: Run the reconcile pass once per process at `cmd/lyx`'s root pre-run

- **Context:**
  - `internal/stencilstore/reconcile.go`
  - `internal/fabricengine/stencilcommit.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/mutation.go`
  - `stencils/stencils.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:**
  - `cmd/lyx/stencilseed.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `cmd/lyx/main.go`, add a package-level `var buildChannel string` with a doc comment stating it is set by `tools/deploy -dev` via `-ldflags "-X main.buildChannel=dev"`, that an unstamped binary (a plain `go build`, a `go install`, or a `go test` binary) leaves it empty, and that empty means production.
  Production is the conservative default because it keeps shipped defaults converging; dev is the exception and must opt in explicitly.

  In `newRoot()`'s existing `PersistentPreRunE`, after the `logger.SetVerbosity(verbosity)` and trace-arming block, add a single call `seedStencils(cmd.Context())`, returning `nil` regardless of its outcome — seeding must never block a command from running.

  `cmd/lyx/stencilseed.go` declares `func seedStencils(ctx context.Context)` in `package main`:
  - Resolve cwd via `lyxcwd.CwdFrom(ctx)` and the location via `lyxcwd.Resolve(cwd)`. On **either** error, return immediately without logging an error: the root pre-run resolves no hub for commands that legitimately have none — `lyx fabric clone` and friends — so the pass is skipped there rather than failing. That is not in tension with the missing-board hard error, which belongs to the producer read path where a stencil is genuinely required.
  - Compute `baseDir := fabricengine.StencilsDir(l.HubPath)`.
  - Compute `sourceDir` as `filepath.Join(l.WorktreePath(), "stencils")` when that directory exists on disk, and the empty string otherwise. The empty string means "no source tree here", which is what makes the port-back drift warning silent in a consumer repo instead of firing on every run forever.
  - Compute the mode: `stencilstore.ModeDev` when `buildChannel == "dev"`, `stencilstore.ModeProduction` otherwise.
  - Call `stencilstore.Reconcile(baseDir, stencils.Registry(), mode, sourceDir)`. On error, emit one `logger.Warn` naming the failure and return.
  - When the returned slice is empty, return without calling the commit verb at all.
  - Otherwise call `fabricengine.CommitSeededStencils(l.HubPath, written, "lyx: seed stencils", fabricengine.NewMutations(filepath.Dir(l.HubPath)))` and log the outcome via `logger.Info` on success, `logger.Warn` on error. Log the mutation record — do not surface it in any command's output envelope, and do not add any key to any envelope. A pre-run seed emits no verb-outcome envelope, and widening every command's key set was already rejected.

  Do not make `seedStencils` return an error, and do not call it from anywhere other than the root pre-run — a lazy pass inside `stencilstore.Read` would drag `fabricengine` onto treadle's stack through `runTriage`/`runTargeting` and their siblings, defeating the allowlist amendment batch 5 makes.
- **Commit:** `feat(cmd/lyx): seed and commit stencils once per process at root pre-run`

### Card 15: Stamp the dev build channel in `tools/deploy`

- **Context:**
  - `cmd/lyx/main.go`
  - `tools/internal/devbin/devbin.go`
- **Edits:**
  - `tools/deploy/main.go`
- **Creates:** none
- **Moves:** none
- **Deletes:** none
- **Requirements:**
  `run(dev bool, destArg string) error` currently assembles its build command as a fixed argument list:

```go
	build := exec.Command("go", "build", "-o", dest, "./cmd/lyx")
```

  Change it to build the argument slice first and append `-ldflags` only on the `-dev` path, so a production deploy's arguments are byte-for-byte what they are today:
  build `args := []string{"build", "-o", dest}`, then when `dev` is true append `"-ldflags"` and `"-X main.buildChannel=dev"`, then append `"./cmd/lyx"`, and construct `exec.Command("go", args...)`.
  `tools/deploy/main.go` passes no `-ldflags` today, so this is a new flag on that build path.
  Leave `build.Dir`, `build.Stdout`, `build.Stderr`, and every other line of `run` unchanged.
  The `-dev` and `-dest` mutual exclusion in `resolveDest` is unaffected — do not touch that function.
- **Commit:** `build(deploy): stamp buildChannel=dev on the -dev build path`

### Card 16: Test the seeding commit and the untouched envelope key set

- **Context:**
  - `internal/fabricengine/stencilcommit.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/mutation.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/hermetic.go`
  - `cmd/lyx/stencilseed.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/stencilcommit_integration_test.go`
  - `cmd/lyx/stencilenvelope_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Both files carry a `//go:build integration` constraint as their first non-empty line: they build a real hub through `internal/hubforge` and spawn git, so they are Tier 2 by the Test Tier Purity Invariant.
  Both live in packages that already have a `TestMain` calling `gitkit.HermeticGitEnv()` — verify that is true for each and add one only if it is missing, per the Hermetic Git Test Environment Invariant.

  `stencilcommit_integration_test.go` asserts, against a `hubforge` hub:
  - an empty `writtenRelPaths` returns `Committed: false`, produces no commit, and leaves the record empty
  - a non-empty `writtenRelPaths` produces exactly one commit whose changed-file set is confined to the stencils subtree, **including** the seeded `.gitattributes`
  - an unrelated dirty file elsewhere in the board repo is **not** swept into that commit — this is the regression guard against reverting to a stage-all commit, and it is the assertion that distinguishes this verb from `Bolt.Commit`
  - the returned record is empty on the no-op run and carries `file_written` plus `commit_created` entries on the run that actually seeded
  - the verb pushes nothing: assert the board repo still reports unpushed commits after it returns

  `stencilenvelope_integration_test.go` asserts that a non-`stencil` command's envelope key set is unchanged by a run that seeded: drive a read-only command through the root against a hub whose stencils directory is absent, so the pre-run seeds all of them, and assert the emitted JSON object carries neither a `mutations` nor a `partial` key.
  This is the guard against the pre-run path quietly widening every command's envelope.
- **Commit:** `test(fabricengine): pin the scoped seeding commit and the untouched envelope keys`

## Batch Tests

`verify: go build ./... && go test ./internal/fabricengine/... ./internal/boardengine/... ./cmd/lyx/... ./tools/...`

The scope is the four packages this batch edits.
`internal/boardengine` is included because card 12 changes how its `writeLockFile` const is declared, and its existing lock tests are what prove the alias resolves to the same filename.
`cmd/lyx` is included because its guard tests (`tierpurity_test.go`, `hermeticenv_test.go`, `destructiveguard_test.go`, `notransients_test.go`) are the machine checks that react to the new `stencilseed.go` file and the new `fabricengine` verb, and they run cheaply.
`./tools/...` covers the deploy change.
The two new integration-tagged tests in card 16 do not run under this untagged `verify:`; they are exercised by the repo-wide `pipeline.done_gate`, which already runs `go test ./... && go test -tags integration ./...`.
`go build ./...` guards the cross-package compile of the new `cmd/lyx` file against the `stencils` and `stencilstore` packages.
