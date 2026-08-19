MILL_REVIEW_BEGIN
# Review: loom: session bootstrap

```yaml
duration_s: 279.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (claude-opus-5), best-effort self-assessment
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [NIT:consistency] ensureWeftLockDirAt already exists
**Demoted-from:** BLOCKING
**Section:** `weft-commit-mechanism`; Technical context; Q&A r3
**Issue:** The discussion states in three places that this task adds a "new package-level `ensureWeftLockDirAt(weftPath)`, extracted from the existing `Fabric.ensureWeftLockDir` body (the method becomes a thin delegation)" — but `internal/fabricengine/weftgit.go:53` already declares `ensureWeftLockDirAt`, `:47-49` is already the thin delegation, and `coalesce.go:101` already calls it.
**Fix:** Delete the extraction claim; `CommitWeftPaths` simply calls the existing `ensureWeftLockDirAt(weftPath)` and no refactor of `Fabric.ensureWeftLockDir` is in scope.

### [BLOCKING:design] origin.json's lock path is undecided
**Section:** `origin-record-ownership-seam`; "The fabric change, concretely"
**Issue:** `ReadOrigin`/`WriteOrigin` are pinned to `state.ReadJSON`/`WriteJSON` (that is what keeps the raw-write tokens out of `fabricengine`), and both take a mandatory `lockPath` the discussion never names; the package precedent `path+".lock"` (`mergestate.go:112`, `corrindex.go:36`) is safe only because those records sit in the weft **gitdir**, whereas a `.lock` beside a tracked `_lyx/fabric/origin.json` contradicts Durable-vs-Ephemeral's "`_lyx` holds tracked content only / never-tracked files live under `.lyx` at the mirrored subpath".
**Fix:** State the lock path explicitly and how its parent is created — `state.WriteJSON` MkdirAlls only the *record's* directory, not the lock's (the gap `loomshed/seed.go:31` works around), and a raw `os.MkdirAll(` in a new `fabricengine` file trips `TestNoUncontainedWrite_FabricengineProductionSource`'s per-file allowlist.

### [NIT:scope] removeLaunchers' hardcoded script-name list
**Section:** `launcher-reuses-existing-set`
**Issue:** "The same `writeLaunchers`/`removeLaunchers` pair" understates the removal edit: `launchers.go:253` iterates a literal `{"ide"+ext, "fabric-checkout"+ext}` and then removes the launcher directory **non-recursively**, so a third `run<ext>` left off that list makes `Remove`/`rollbackAdd` fail on a non-empty directory rather than merely leaking a file.
**Fix:** Name the `removeLaunchers` script-name list as an explicit edit point beside `writeLaunchers`.

### [NIT:scope] live-state matrix Add cells have no disposition
**Section:** Testing → Integration (`internal/fabricengine`)
**Issue:** `Add` is a live-state matrix verb at both anchors (`livestate_verbs_test.go:480`), and every cell runs `AssertRecordMatchesDiff` (`livestate_matrix_test.go:242`) in both directions; the new write, its commit, and any second path the warp junction exposes for the same file must be covered by the single `KindFileWritten` entry, which the testing section never mentions.
**Fix:** Add one line giving the matrix/oracle its disposition (expected to pass unchanged, or an enumerated adjustment).

## Verdict

REQUEST_CHANGES
Two blockers: a superseded refactor claim and an unspecified, invariant-constrained lock path.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
