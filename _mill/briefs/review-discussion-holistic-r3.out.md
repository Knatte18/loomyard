MILL_REVIEW_BEGIN
# Review: Add a local-only file category to weft

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude, Opus-class (harness reports claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-27
```

## Findings

### [BLOCKING:design] Weft-side merge guards and pre-merge sync undecided
**Section:** Scope "In" / `### weft-is-never-merged` / Technical context "Where the weft merge happens"
**Issue:** The decision covers `MergeStart` and `concludeMergeSides`, but `Merge`/`MergeIn` also run weft-side *guards and sync* that are not merge participation: `pairDirtyReason` (`mergeguards.go:142-147`), `syncedToUpstreamReason`→`sideNotSyncedToUpstream(f.weft, ...)` (`:229-234`), the weft `HeadDetached` check (`:182-187`), `resolveMergeSources`' weft arm (`:84-97`, which can refuse with `mergeReasonNotFabricManaged`/`mergeReasonSourceNotFound`), and `syncSideBeforeMerge(rec, f.weft, ...)` (`merge.go:447`). Their disposition is never stated, and it is now load-bearing: a push rejection is warn-and-continue by `### commit-and-push-every-transition`, which leaves the local weft permanently diverged from its upstream, so a retained `syncedToUpstreamReason` refuses every subsequent landing merge with `mergeReasonNotSynced` — the failure this task exists to remove, relocated. `TestMerge_FetchedDivergedWeftRefuses` / `TestMerge_UnfetchedDivergedWeftRefuses` (`merge_target_integration_test.go:793,817`) pin the current behaviour either way.
**Fix:** State per weft-side guard and for `syncSideBeforeMerge` whether it stays, is dropped, or is narrowed to warp, and say what a diverged weft means once weft is not a merge participant.

### [BLOCKING:consistency] Raddle premise contradicts raddle.md and CLAUDE.md
**Section:** `### raddle-gate-removed` / Scope "Out — Raddle"
**Issue:** The rationale rests on "`_lyx/raddle/` would be weft content, hence per-branch-local … nothing to fold back", but `manifest/designs/raddle.md:12-18,47` designs raddle as tracked `_lyx/raddle/` docs regenerated **at merge time against the parent's current HEAD** and committed to describe the landed diff — content whose entire purpose is to reach the parent; the project `CLAUDE.md` likewise directs durable notes to `_lyx/raddle/` as "versioned and merged into `main`". Under this design neither is achievable, and `raddle.md` is named but given no disposition.
**Fix:** Either state that raddle's merge-time fold-back is superseded and that `raddle.md`/`CLAUDE.md` are corrected in this commit, or justify the gate removal on grounds that do not depend on the false premise.

### [BLOCKING:scope] fabricengine's own module doc absent from the doc list
**Section:** Scope "In" (docs) / `## Constraints` (Documentation lifecycle)
**Issue:** The doc list is `CONSTRAINTS.md`, `loom.md`, `shed.md` only, yet `internal/fabricengine/doc.go` is the module's authoritative narrative and states two-sided merge semantics throughout (e.g. `:861`, `:876`, `:1025-1045` on both-sides abort/outcome), and `cleanup.go:6-9,72-77,177-181` documents the `raddleFoldedBack` flag matrix being deleted. A plan writer following the list leaves both asserting removed behaviour.
**Fix:** Name `internal/fabricengine/doc.go` (and the `Cleanup`/`Protected` doc comments, plus `manifest/designs/fabric-unified-view.md` if it makes the same claim) as same-commit doc updates.

### [BLOCKING:design] MergeStateActive probe error has no disposition
**Section:** `### skip-while-mid-merge`
**Issue:** `MergeStateActive(l) (bool, error)` returns an error (its model `foreignMergeStatePresent`, `mergestate.go:257-276`, errors on any `MergeHeadPresent`/`ConflictedFiles` failure), but only the `true` branch is decided; the discussion never says whether a probe error hard-errors from `persist`, warns and skips, or warns and commits anyway — three defensible readings given commit hard-errors and push warns.
**Fix:** State the probe-error disposition explicitly alongside the true/false branches.

### [NIT:scope] No named test for the two new fabricengine functions
**Section:** `## Testing`
**Issue:** `PushAnchored` and `MergeStateActive` are new exported `fabricengine` functions, but the fabricengine test list covers only merge and `Cleanup`; they are exercised indirectly via the `loomcli` closure tests alone.
**Fix:** Name a direct test for each (`MergeStateActive` true for foreign merge state on either side; `PushAnchored` gating on `SkipGit`/`SkipPush` and surfacing `ErrPushRejected`).

## Verdict

REQUEST_CHANGES
Weft-side guard disposition, the raddle premise, doc scope, and probe-error handling need deciding.
MILL_REVIEW_END
