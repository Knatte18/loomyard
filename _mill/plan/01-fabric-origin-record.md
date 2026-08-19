# Batch: fabric-origin-record

```yaml
task: 'loom: session bootstrap'
batch: fabric-origin-record
number: 1
cards: 4
verify: go test ./internal/fabricengine/
depends-on: []
```

## Batch Scope

This batch adds the two new `internal/fabricengine` primitives loom needs, with no caller yet: the `_lyx/fabric/origin.json` provenance record (its type, its two anchor-aware path accessors, its anchor-relative form, and its read/write functions) and `CommitWeftPaths`, the narrow positive-pathspec, no-push weft-commit helper.
It is one batch because both pieces are new files in one package with no behaviour change anywhere else — nothing calls either until batch 2 (`Add`) and batch 5 (`loomcli`) do.

The external interface batch 2 and batch 5 consume: `fabricengine.Origin`, `fabricengine.OriginRecordRel`, `fabricengine.OriginRecordPath`, `fabricengine.OriginRecordPathFor`, `fabricengine.ReadOrigin`, `fabricengine.WriteOrigin`, and `fabricengine.CommitWeftPaths`.

Batch-local decisions beyond `## Shared Decisions` in the overview:

- `WriteOrigin` takes a `slug` rather than being split into two functions.
  Loom passes `l.WorktreeName` for its own pair, which resolves to the same physical file `ReadOrigin` reaches through the warp junction, so one write function serves both callers and loom still supplies no path.
- `CommitWeftPaths` orders its two early returns — `SkipGit`, then an empty `relPaths` — ahead of the lock acquisition, which is what keeps both testable without git or a `.weft` directory.

## Cards

### Card 1: Origin record type and its three path accessors

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/origin.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the new file in `package fabricengine` with a file-header comment stating what the record is (fabric's provenance record for one worktree pair, the first *tracked* fabric-owned record under the durable lyx directory) and that it is committed by `Topology.Add` on the weft branch.
  Declare an exported struct `Origin` with exactly one field, `ParentBranch string`, carrying the JSON struct tag whose name is `parent_branch`, and a doc comment stating that `ParentBranch` is the warp branch the pair was forked from, recorded at creation time and never inferred.
  Declare three unexported segment constants — `originRecordDirName = "fabric"`, `originRecordFileName = "origin.json"`, `originRecordLockFileName = "origin.json.lock"` — and state in a comment that `fabricengine` is their sole declarer, per the Cwd Resolution Invariant's "a module's own durable-storage subdirectory is that module's own private relative-path constant" rule.
  Add `func OriginRecordRel() string`, returning `filepath.Join(lyxdirs.LyxDirName, originRecordDirName, originRecordFileName)` — the anchor-relative form both path accessors and every commit pathspec are built from, so the segments are joined in exactly one place.
  Add `func OriginRecordPath(l *lyxcwd.Location) string`, the read side, returning `filepath.Join(l.AnchorPath(), OriginRecordRel())`; document that `AnchorPath()` already carries `AnchorRel`, so a caller reading through the warp junction in a subpath-anchored hub needs no extra join.
  Add `func OriginRecordPathFor(l *lyxcwd.Location, slug string) string`, the write side, returning `filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, OriginRecordRel())`; document that it mirrors the existing `WeftWorktree -> AnchorRel -> durable-dir` shape and exists because during `Add` the new pair is not the acting worktree, and that a bare `WeftWorktreePath(l, slug)` root would be wrong in any subpath-anchored hub.
- **Commit:** `feat(fabricengine): add the Origin provenance record type and its path accessors`

### Card 2: ReadOrigin and WriteOrigin over state, locked under .weft

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/mutation.go`
  - `internal/state/state.go`
- **Edits:**
  - `internal/fabricengine/origin.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported `func originLockPath(weftPath string) (string, error)` that calls the already-existing package-level `ensureWeftLockDirAt(weftPath)` and joins `originRecordLockFileName` onto its return.
  Document why the lock is NOT the package's usual `path + ".lock"` idiom: that idiom is safe only for the two records that live in the weft gitdir, and a never-tracked `.lock` file sitting beside a tracked record under the durable lyx directory would violate the Durable-vs-Ephemeral State Invariant.
  Document also that `ensureWeftLockDirAt` already creates its own directory, which is what supplies the lock's parent without introducing a new raw write that `TestNoUncontainedWrite_FabricengineProductionSource` would flag.
  Add `func ReadOrigin(l *lyxcwd.Location) (Origin, bool, error)`: resolve the lock via `originLockPath(WeftWorktree(l))`, then return `state.ReadJSON[Origin](OriginRecordPath(l), lockPath)`.
  Its doc comment must state that a `false` second return means no record exists for this worktree — the legacy-worktree case — and is not an error.
  Add `func WriteOrigin(rec *Mutations, l *lyxcwd.Location, slug string, o Origin) error`: resolve the lock via `originLockPath(WeftWorktreePath(l, slug))`, call `state.WriteJSON` against `OriginRecordPathFor(l, slug)`, and — only after that call returns nil — append `KindFileWritten` to `rec` at the same path, per the Mutation Record Invariant's "after the primitive observably changed state" rule.
  Document that the `slug` parameter is what lets one function serve both callers: `Topology.Add` passes the new pair's slug, and a caller repairing its own worktree passes that worktree's own name, which resolves to the same file the read side reaches through the junction.
  Do not add any `os.`-qualified raw write call in this file.
- **Commit:** `feat(fabricengine): add ReadOrigin and WriteOrigin over the weft-side origin record`

### Card 3: CommitWeftPaths, the positive-pathspec no-push weft commit

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/lock/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commitweftpaths.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the new file in `package fabricengine`.
  Add an unexported pure helper `func weftCommitPathspec(anchorRel string, relPaths []string) []string`, returning `ScopedPathspec(anchorRel, relPaths)`, documented as the single place the anchor join happens for this helper so no caller ever joins `AnchorRel` or names the durable lyx directory in a pathspec itself.
  Add `func CommitWeftPaths(weftPath, anchorRel string, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error)` with this body order, which the doc comment must state is load-bearing:
  (1) `opts.SkipGit` returns `("", false, nil)` with no lock taken and nothing staged, exactly as `commitWeftAt` does, so an existing `SkipGit` test path can thread its opts straight through;
  (2) an empty `relPaths` returns `("", false, nil)` — nothing staged is never reported as a commit;
  (3) `ensureWeftLockDirAt(weftPath)` then `lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))`, released via a deferred call, wrapping an acquisition failure as `fabricengine: acquire weft write lock: %w`;
  (4) `return gitrepo.New(weftPath).StageAndCommit(msg, weftCommitPathspec(anchorRel, relPaths))`.
  Document that `opts.SkipPush` is accepted and ignored because this helper never pushes, and that `SyncOptions` is kept whole rather than narrowed to a bool so `Topology.Add` can pass its own opts unchanged.
  Document why neither existing path works: the pair-bound commit verb resolves its routing from its own stored warp path and cannot address a pair it did not open, and also fires a detached push unconditionally; `commitWeftAt` is a stage-all, which the Fabric Git Invariant's cross-module-exclusions rule forbids for durable-lyx commits because that tree is shared by every round-loop module.
  Document that the lock is taken here rather than pushed onto callers, unlike `commitWeftAt`'s caller-responsible contract, because this helper has two independent callers who would otherwise race on the git index lock.
- **Commit:** `feat(fabricengine): add CommitWeftPaths, a positive-pathspec no-push weft commit helper`

### Card 4: Tier-1 tests for the record accessors and the commit helper

- **Context:**
  - `internal/fabricengine/origin.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/portallauncher_test.go`
  - `internal/fabricengine/corrindex_test.go`
  - `internal/fabricengine/fabric.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/origin_test.go`
  - `internal/fabricengine/commitweftpaths_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Both files are untagged and must spawn nothing, per the Test Tier Purity Invariant — build `*lyxcwd.Location` values by hand exactly as `newPortalLauncherTestLocation` already does rather than resolving one.
  In `origin_test.go`: a `TestOrigin_JSONRoundTrip` marshalling and unmarshalling an `Origin` and asserting the wire key is `parent_branch`; a `TestOriginRecordPath_BothAnchors` asserting `OriginRecordPath` at `AnchorRel == "."` and at `AnchorRel == "backend"` against an independently computed `filepath.Join`, proving the subpath case moves the record down by the anchor; a `TestOriginRecordPathFor_BothAnchors` doing the same for the write side against a `WeftWorktreePath`-rooted expectation and asserting the anchor segment is present in the subpath case; and a `TestOriginRecordRel_IsTheSharedSuffix` asserting both accessors end in `OriginRecordRel()`.
  In `commitweftpaths_test.go`: a `TestWeftCommitPathspec` table covering `anchorRel == "."` (entries returned with no added segment) and `anchorRel == "backend"` (each entry prefixed), asserting the result is positive-only with no entry beginning with a `:(exclude)` marker; a `TestCommitWeftPaths_SkipGit` asserting `("", false, nil)` for `SyncOptions{SkipGit: true}` against a path that does not exist, which proves no lock was taken and no directory created; and a `TestCommitWeftPaths_EmptyPaths` asserting the same triple for an empty slice, likewise against a non-existent path.
- **Commit:** `test(fabricengine): cover the origin record accessors and CommitWeftPaths' git-free paths`

## Batch Tests

`verify: go test ./internal/fabricengine/` runs the package's untagged tier-1 suite, which is where both new test files land.
The scope is the one package this batch touches; the two new files are the only ones that can fail from these cards, and the rest of the package's untagged suite is the regression check that the two new production files did not break a shared helper.
The integration-tagged half of this package is exercised by batch 2, which is where the first real caller lands — running it here would spend minutes proving nothing this batch changed.
