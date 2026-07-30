# Batch: commit-foundations

```yaml
task: 'fabric: Fabric.Commit classify+dispatch + unified diff/status'
batch: commit-foundations
number: 1
cards: 3
verify: go test -tags integration ./internal/fabricengine/
depends-on: []
```

## Batch Scope

This batch builds the three lower-level `fabricengine` pieces `Fabric.Commit` (batch 3) will compose, none of which is `Fabric.Commit` itself: the pure warp-vs-weft path classifier, the `Snapshot:` trailer writer with tag validation, and the weft-lock refactor that lets a caller hold the weft write lock across both the warp and weft commits. The external interface batch 3 consumes: `classifyPaths(relPath, wiredNames, files) (warp, weft []string)`, `appendSnapshotTrailers(message, tags) (string, error)` + `validateSnapshotTag`, and `(f *Fabric) commitWeftLocked(pathspec, message, opts, snapshotTags ...string)` plus the now-variadic public `CommitWeft`. Batch-local decision: the classifier is a **pure, I/O-free, no-validation** function (trusts its caller) per `_mill/discussion.md`'s `classification-input-contract`; it is the batch's TDD card (impl + Tier-1 table test land together in one commit so no non-compiling intermediate commit is created).

## Cards

### Card 1: Pure warp-vs-weft path classifier

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/classify_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func classifyPaths(relPath string, wiredNames []string, files []string) (warp []string, weft []string)` to `classify.go` — a pure function with no git and no filesystem I/O and no path validation (it trusts its caller, the same posture `ScopedPathspec` takes). Compute the weft-junction prefix set as `ScopedPathspec(relPath, wiredNames)` (reuse the existing `ScopedPathspec` in `fabric.go`). Because `ScopedPathspec` builds each prefix with `filepath.Join`, which emits OS-native separators (a backslash on Windows whenever `relPath != "."`), `filepath.ToSlash`-normalize **both sides** — each computed junction prefix **and** each input path — before comparing; never compare a slash-normalized input against an un-normalized (possibly backslash-bearing) prefix. Classify each input path as **weft** iff, after that both-sides `filepath.ToSlash` normalization, it equals a junction prefix exactly or begins with that prefix plus `"/"` (a path-**segment** boundary, so `_lyxfoo` classifies as warp, not weft); everything else is **warp**. The two output slices must partition the input in input order with no path lost or duplicated. `wiredNames` is supplied by the caller (batch 3 passes `WiredNames(f.weftPath)` — the hub-reserved-filtered set from `junctionnames.go`); `classifyPaths` itself neither loads config nor filters names. Write `classify_test.go` as an untagged Tier-1 table test (no git spawn) covering: a path under `_lyx` → weft; under `_pattern` → weft; a host source path → warp; `relPath == "."` vs `relPath == "sub"` scoped prefixes; the `_lyxfoo` segment-boundary case → warp; empty file list → two empty lists; all-warp and all-weft inputs; a `wiredNames` set with more than two entries; and an assertion that the two outputs partition the input with nothing lost or duplicated.
- **Commit:** `feat(fabric): add pure warp-vs-weft path classifier`

### Card 2: Snapshot trailer writer + tag validation

- **Context:**
  - `internal/fabricengine/weftgit.go`
- **Edits:**
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/trailer_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `trailer.go` add `const SnapshotTrailerKey = "Snapshot"`, a `validateSnapshotTag(tag string) error` that rejects any tag not matching a single-line token charset `^[A-Za-z0-9._-]+$` (which already excludes newline, carriage return, and colon — the trailer-injection vector the `snapshot-trailer-written-now` decision names) with a typed/descriptive error naming the offending tag, and `appendSnapshotTrailers(message string, tags []string) (string, error)` that validates every tag first (returning the error before writing anything, so a caller fails fast rather than committing a corrupt trailer) then appends one `Snapshot: <tag>` line per tag. Reuse `endsInTrailerBlock` so the appended lines join an existing trailer block (the `Warp-SHA` trailer, already appended by the caller) directly rather than starting a new paragraph; an empty `tags` slice returns `message` unchanged with a nil error. Add pure Tier-1 unit tests to `trailer_test.go`: single tag, multiple tags (one line each), coexistence with a pre-appended `Warp-SHA` trailer (both parse back via the existing `parseWarpSHATrailer` and a `Snapshot`-key scan), empty tags → unchanged, and rejection of tags containing a newline, a carriage return, a colon, or an out-of-charset character.
- **Commit:** `feat(fabric): add Snapshot trailer writer with tag validation`

### Card 3: Weft-lock refactor for a caller-held lock across two commits

- **Context:**
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/syncweft.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Factor `CommitWeft`'s post-lock body into a new unexported `func (f *Fabric) commitWeftLocked(pathspec []string, message string, opts SyncOptions, snapshotTags ...string) (sha string, committed bool, err error)` that **assumes the weft write lock is already held and `opts.SkipGit` already handled** — it must NOT call `ensureWeftLockDir` or acquire the lock. Move the existing steps into it verbatim: `warpHeadSHA`, build `commitMessage` via `appendWarpSHATrailer` when `!unborn`, `weftPathspecFilter`, `f.Weft.StageAndCommit`, the `"did not match any files"` tolerance, and `RecordCorrespondence` (still returning `(sha, true, err)` when the commit lands but `RecordCorrespondence` fails). Extend it to append the `Snapshot:` trailers: in the `!unborn` branch, after `appendWarpSHATrailer`, call `appendSnapshotTrailers(commitMessage, snapshotTags)` and return its error if any (before staging); when `unborn`, snapshot tags are dropped (no trailer block on a trailer-less commit), matching the Warp-SHA condition. Rewrite the public `CommitWeft` to the signature `func (f *Fabric) CommitWeft(pathspec []string, message string, opts SyncOptions, snapshotTags ...string) (sha string, committed bool, err error)` as a thin wrapper: the `opts.SkipGit` early return, then `ensureWeftLockDir` + `lock.AcquireWriteLock` + `defer Release`, then `return f.commitWeftLocked(pathspec, message, opts, snapshotTags...)`. The trailing variadic keeps every existing 3-arg `CommitWeft` caller compiling unchanged and passing zero tags — the three `fabriccli` weft-verb call sites plus the `buildercli`, `webstercli`, `fabricengine` `SyncWeft`, `initengine` undo, and `perchcli` call sites; do not edit those call sites.
- **Commit:** `refactor(fabric): extract commitWeftLocked and add snapshot-tags variadic to CommitWeft`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/` runs the package's Tier-1 tests (the new `classify_test.go` and the pure `trailer_test.go` Snapshot-writer cases) plus every existing `//go:build integration` file — crucially the CommitWeft/SyncWeft coverage (`weftgit_pathspec_integration_test.go`, `syncweft_integration_test.go`, `index_integration_test.go`, `weftgit_exclude_test.go`, `weftgit_unborn_warp_test.go`), which regression-guards the `commitWeftLocked` extraction: the refactor must leave every one of them green. The overview's module-wide `go build ./...` additionally confirms the `snapshotTags ...string` variadic keeps all eight cross-package `CommitWeft` callers compiling. The new snapshot-trailer-on-a-real-weft-commit behavior threaded here is exercised end-to-end in batch 3 (where `Fabric.Commit` passes non-empty `snapshotTags`); the pure trailer format itself is fully covered by card 2's Tier-1 tests, so no new integration test is added in this batch.
