# Batch: fabricengine: CommitWeftAt primitive

```yaml
task: 'board: move storage to weft:main'
batch: 'fabricengine: CommitWeftAt primitive'
number: 1
cards: 3
verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: []
```

## Rename mechanic

Not applicable — this batch contains no `Moves:` entries.

## Batch Scope

This batch adds the one new git-commit primitive board's detached sync process needs: `fabricengine.CommitWeftAt`, a package-level, warp-untethered wrapper around `gitrepo.Repo.StageAllAndCommit` with no `Warp-SHA` trailer and no correspondence recording (per `_mill/discussion.md`'s "Weft git routing" decision — `_board`'s `weft:main` checkout has no corresponding warp branch, so attaching a trailer would tag board commits with a meaningless pointer). It is the commit-side counterpart to the already-existing `PushWeftAt` in the same file. This batch is self-contained and root (no dependency on the `_board`-topology batch): `CommitWeftAt` operates on any weft path handed to it and does not care how that path came to exist. Batch 3 (`boardengine`'s dual-store facade) consumes this primitive directly by name (`fabricengine.CommitWeftAt`) — that is the external interface this batch hands to the next one.

## Cards

### Card 1: add `CommitWeftAt` to weftgit.go

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new package-level function `CommitWeftAt(weftPath, message string, opts SyncOptions) (sha string, committed bool, err error)` in `weftgit.go`, placed immediately after the existing `PushWeftAt` function (end of file). Mirror `PushWeftAt`'s shape (package-level, no `Fabric` receiver, no warp path) and `CommitWeft`'s early `SkipGit` gate: when `opts.SkipGit` is true, return `("", false, nil)` immediately with no git spawned. Otherwise call `gitrepo.New(weftPath).StageAllAndCommit(message)` and return its three results verbatim (no error wrapping needed beyond what `StageAllAndCommit` already returns — this primitive is deliberately bare, unlike `CommitWeft`, which layers pathspec filtering, a `Warp-SHA` trailer, and `RecordCorrespondence` on top). Do NOT acquire `ensureWeftLockDir`/`weftWriteLockFile` — that lock exists to serialize `Fabric.CommitWeft` callers sharing a pathspec-scoped commit; `CommitWeftAt`'s caller (`boardengine.Sync`, batch 3) already holds its own write lock (`board.lock`) around the equivalent critical section, so a second lock here would only add unnecessary contention with no correctness benefit. Add a doc comment on `CommitWeftAt` stating: it is the warp-untethered, wildcard-stage commit primitive for `_board`'s `weft:main` checkout (which has no corresponding warp branch to trailer against); it wraps `gitrepo.StageAllAndCommit` directly with no pathspec filtering, no `Warp-SHA` trailer, and no `RecordCorrespondence` call — unlike `Fabric.CommitWeft`; and it is `PushWeftAt`'s natural commit-side counterpart.
- **Commit:** `feat(fabricengine): add CommitWeftAt warp-untethered commit primitive`

### Card 2: update doc.go and gitrepo/doc.go's "board's opt-in exception" phrasing

- **Context:**
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/doc.go`, the package doc comment currently states (verbatim): "fabric never calls gitrepo's `StageAllAndCommit` (board's opt-in wildcard-stage exception, per gitrepo's doc.go) — all staging is explicit-list `StageAndCommit`, scoped to a configured pathspec." This becomes inaccurate once `CommitWeftAt` exists in the same package. Add a clause narrowing the claim to the `Fabric` type's own methods and naming the new exception explicitly, e.g.: "...all staging is explicit-list `StageAndCommit`, scoped to a configured pathspec. The one exception is the package-level `CommitWeftAt` function (not a `Fabric` method), which wraps board's wildcard-stage commit on its behalf — see `CommitWeftAt`'s own doc comment." In `internal/gitrepo/doc.go`, the "Scope boundaries" section states (verbatim): "`StageAllAndCommit` is a separate wildcard-stage variant provided as board's opt-in exception, not a relaxation of the explicit-list default — fabric, raddle, and codeintel keep using explicit-list `StageAndCommit`." Add a trailing clause noting the call is now routed through `fabricengine.CommitWeftAt` on board's behalf rather than `boardengine` calling `StageAllAndCommit` directly, e.g.: "...(called via `fabricengine.CommitWeftAt` on board's behalf, not `boardengine` calling `gitrepo` directly)."
- **Commit:** `docs(fabricengine,gitrepo): update board's-opt-in-exception phrasing for CommitWeftAt`

### Card 3: add CommitWeftAt test coverage

- **Context:**
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commitweftat_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file `commitweftat_test.go`, `//go:build integration` (spawns real git via `gitrepo.StageAllAndCommit`), `package fabricengine` (internal, reusing `index_integration_test.go`'s `newPlainWarpRepo(t) string` helper as a generic plain-git-repo fixture for the weft-shaped path — the helper is not warp-specific despite its name; it just `git init -b main` plus one commit). This package already has `testmain_test.go`'s `TestMain` calling `lyxtest.HermeticGitEnv()`, so no new `TestMain` is needed (per CONSTRAINTS.md's Hermetic Git Test Environment Invariant — one `TestMain` per package covers every test file in it). Mirror `CommitWeft`'s existing test shape (see `weftgit_pathspec_integration_test.go`/`weftgit_unborn_warp_test.go` for style) minus the trailer/correspondence assertions, which do not apply. Cover: (1) `TestCommitWeftAt_CommitsDirtyWorktree` — write an untracked file into a `newPlainWarpRepo(t)`-created fixture dir, call `CommitWeftAt(dir, "board sync", SyncOptions{})`, assert `committed == true`, `sha` is a valid non-empty SHA, and `err == nil`; assert the commit message is exactly `"board sync"` with no `Warp-SHA:` trailer present (`git show -s --format=%B <sha>` in the fixture dir). (2) `TestCommitWeftAt_NoopOnCleanWorktree` — call `CommitWeftAt` twice in a row on the same clean fixture (no changes between calls); assert the second call returns `committed == false, err == nil`. (3) `TestCommitWeftAt_SkipGitReturnsImmediately` — call `CommitWeftAt(dir, "msg", SyncOptions{SkipGit: true})` against a dirty fixture; assert `("", false, nil)` and that the file is still uncommitted afterward (`git status --porcelain` non-empty).
- **Commit:** `test(fabricengine): cover CommitWeftAt commit/noop/skip-git paths`

## Batch Tests

`go test ./internal/fabricengine/...` (both tagged and untagged) covers this batch: Card 1's `CommitWeftAt` is exercised by Card 3's three new integration-tagged tests in `commitweftat_test.go`. Card 2 is a doc-comment-only change with no runnable surface. Existing package tests (`weftgit_pathspec_integration_test.go`, `weftgit_unborn_warp_test.go`, etc.) must continue passing unchanged — this batch adds a new function and touches no existing one.
