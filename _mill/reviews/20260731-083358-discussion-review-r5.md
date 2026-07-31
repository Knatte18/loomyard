MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] RebuildIndex IS order-sensitive; claim is wrong

**Section:** `### reader-orders-by-topology-not-commit-date` ("Effect on the correspondence rebuild")
**Issue:** "Its result is insensitive to the input ordering" is false against `index.go:283-314`: the dedup is *last-recorded-wins over the reversed scan*, so for a warp SHA recorded by more than one weft commit the winning `WeftSHA` is whichever the scan listed first; and `sort.SliceStable` preserves input order among equal `WarpSeq`, which includes the `seq = 0` dangling sentinel (line 305) and equal-first-parent-depth side-branch commits.
**Fix:** Replace the insensitivity claim with the two order-dependent cases and state which outcome is intended for each, or scope the claim to "linear history / distinct warp SHAs".

### [GAP] No test can witness the topo-vs-date order change

**Section:** `## Testing` (reader cases; "`RebuildIndex` still produces the same correspondence index ... after `--topo-order` is added")
**Issue:** `TestRebuildIndex_EqualsIncrementallyBuiltIndex` (`syncweft_integration_test.go:117`) is three linear `SyncWeft` rounds — exactly the shape where date order and topological order coincide — so it cannot turn the no-op claim "from reasoning into a checked fact", and every listed reader case ("newest wins", tag isolation) is linear too.
**Fix:** Name a test that builds a weft history where the two orders differ (a merged side branch with a back-dated commit) and assert both the reader's pick and rebuild equivalence on it.

### [GAP] Empty commits overwrite an existing correspondence entry

**Section:** `### snapshot-tags-always-force-a-weft-commit` / `## Scope` ("No changes to the correspondence index")
**Issue:** A tags-only or repeated tagged call leaves warp HEAD unchanged, so `RecordCorrespondence(warpSHA, emptySHA)` upserts over the entry the content commit wrote (`index.go:107-112`); `WeftSHAForWarpSHA(X)` and `RevertWithWeft(X)` then resolve to the content-free empty commit. The code comment at `index.go:283-287` calls that duplicate shape "rare but legal" — this slice makes it routine, and no listed test covers it.
**Fix:** State which weft commit should win for a repeated warp SHA and whether the revert target changing to the empty commit is accepted; add a case pinning it.

### [NOTE] PartialCommitError text is wrong for a tags-only call

**Section:** `### snapshot-tags-always-force-a-weft-commit` (failure propagation)
**Issue:** With zero warp files, the `err != nil && !weftCommitted` arm yields `Error()` = "warp commit  landed, weft commit failed: …" (`commit.go:54`) with an empty `WarpSHA`, and the type's godoc (`commit.go:30-41`) opens "reports a Fabric.Commit call that landed a warp commit" — both false on the newly first-class tags-only shape.
**Fix:** Add `PartialCommitError`'s godoc/message to the same-commit staleness sweep, or state that the pre-existing wording is accepted unchanged.

### [NOTE] CommitEmpty's never-sweep guard is check-then-commit

**Section:** `### commit-empty-as-a-new-gitrepo-primitive`
**Issue:** `StageAndCommit` enforces never-sweeping *structurally* (`git commit … -- <files>`, `gitrepo.go:216`); the pre-check + unscoped `git commit --allow-empty` is two spawns, so an index write landing between them is still swept — a weaker guarantee than "the same norm".
**Fix:** Record the residual window in the decision and in `CommitEmpty`'s godoc so the contract does not over-claim.

### [NOTE] One comment site missing from the "exhaustive" footprint

**Section:** `## Technical context` (deletion footprint, "Two further comment sites")
**Issue:** `gogit.go:88`'s object-lookup bullet names `isStrictDescendant` alongside the `SetSnapshotSHA` clause the footprint does call out; only the latter is listed, and `hasUnpushed` in the same sentence must survive the edit.
**Fix:** Add `gogit.go:88`'s `isStrictDescendant` mention to the sweep list.

### [NOTE] Three Q&A entries contradict the corrected decisions

**Section:** `## Q&A log`
**Issue:** Line 276 still says the unborn-warp rule "needs no new code … lives inside the existing `if !unborn` arm" (corrected at line 105), line 279 still gives the two-step `ChangedFilesSince(SnapshotWarpSHA(tag))` composition (corrected at line 79), and line 281 still says `parseSnapshotTags` is promoted (dropped at line 284).
**Fix:** Mark those three as superseded in place, so a plan writer skimming the log does not size or code from them.

## Verdict

GAPS_FOUND
Ordering claim, its test, and the correspondence-overwrite consequence need resolving.
MILL_REVIEW_END
