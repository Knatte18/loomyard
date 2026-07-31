MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Deletion footprint still misses a surviving gogit_test harness
**Section:** Technical context → Deletion footprint, `gogit_test.go` bullet
**Issue:** `runLinkedWorktreeParityChecks` (gogit_test.go:717, driven by `TestLinkedWorktree_Parity`:847, run twice — direct and via junction) calls `repo.remoteName()` (758), `repo.SnapshotSHA(gogitParitySnapshotKey)` (772) and `repo.isStrictDescendant(...)` (829); none of those three subtests, nor the helpers they need (`oracleRemoteName` 376, `oracleIsStrictDescendant` 400, `oracleSnapshotSHA` 597, `gogitParitySnapshotKey` 630, the `update-ref` seeding at 684, the doc at 710-716), appear in the list, so the package will not compile after `snapshot.go` is deleted.
**Fix:** Add those three subtests plus their helpers/fixture seeding to the `gogit_test.go` footprint, and record the same explicit coverage judgement made for the repack case — the linked-worktree/junction parity harness loses three of its seven read-side cases.

### [GAP] Reader has no stale-warp-SHA decision, unlike the corr index
**Section:** Decisions → `reader-api-snapshot-warp-sha`; Scope → "No staleness helper"
**Issue:** `RebuildIndex`/`WeftSHAForWarpSHA` have an explicit stale-SHA decision (index.go:268-271, 296-303: record anyway, validate with `f.Warp.SHAExists` at use), but the discussion never says what `SnapshotWarpSHA` does when the newest tagged commit's `Warp-SHA` names a warp commit a history rewrite removed — and the prescribed composition `f.Warp.ChangedFilesSince(f.SnapshotWarpSHA(tag))` then returns a hard error, contradicting `ChangedFilesSince`'s own doc (gitrepo.go:430-434, "callers are expected to check SHAExists first and treat a missing SHA as staleness").
**Fix:** Decide and record: return the dangling SHA raw, skip to the next-newest tagged commit, or return `("", nil)` — and state the matching consumer idiom in `fabricengine/doc.go` and `raddle.md`.

### [GAP] `CommitEmpty`'s index pre-check is unspecified on an unborn HEAD
**Section:** Decisions → `commit-empty-as-a-new-gitrepo-primitive` / `unborn-weft-lands-an-empty-root-commit`
**Issue:** The pre-check is specified as "verifies the index is clean against HEAD (`git diff --cached --quiet`)", but the root-commit contract requires it to run where there is no HEAD; neither the unborn behaviour nor the exit-code mapping (0 → proceed, 1 → `ErrIndexNotEmpty`, other → error) is stated, and the consequence bullet's own demand ("must be specified, not left as pin-whatever-git-happens-to-do") is met only for the commit half.
**Fix:** State the pre-check's unborn-HEAD semantics explicitly (compare against the empty tree, or skip the check when `CurrentSHA` reports `ErrNoCommits`) plus the exit-code mapping, and pin it with the existing unborn-HEAD test case.

### [GAP] `ErrIndexNotEmpty` propagation through the weft path is unstated
**Section:** Decisions → `snapshot-tags-always-force-a-weft-commit`; Testing → `CommitEmpty`
**Issue:** The rule fires at `weftgit.go:383` (`!committed`) after `StageAndCommit` has already run `git add`; if the weft index carries anything staged outside this call — an aborted prior run, not something the combined write lock excludes — `CommitEmpty` refuses and a path that is a documented silent no-op today becomes an error, surfaced as a `*PartialCommitError` from `Fabric.Commit`. The discussion asserts "under the combined write lock this should not arise" but never records the fabricengine-layer outcome or accepts it.
**Fix:** State what `commitWeftLocked`/`Commit` return on `ErrIndexNotEmpty` (propagate as the unlanded-weft `*PartialCommitError`), record it as accepted, and note it in `commitWeftLocked`'s godoc rewrite.

### [NOTE] Two stale snapshot comments outside the enumerated list
**Section:** Technical context → Deletion footprint (docs)
**Issue:** `gogit.go:82` ("CurrentBranch, remoteName, and both snapshot ref reads") is not in the list that names 89, 108-109, 184; `parity_test.go:591` describes a surviving test in terms of `SetSnapshotSHA`.
**Fix:** Add both to the doc/comment sweep, since the footprint is presented as grep-verified and exhaustive.

## Verdict

GAPS_FOUND
Compile-breaking test omission, plus three unspecified failure-mode behaviours.
MILL_REVIEW_END
