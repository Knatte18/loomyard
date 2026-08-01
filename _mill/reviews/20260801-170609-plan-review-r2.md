MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (Claude Sonnet 5, per system identification)
reviewed_file: plan/
date: 2026-08-01
```

## Findings

### [NIT] Card 8: PartialPullError return value not pinned per stage
**Location:** 02-fabric-pull.md, Card 8
**Issue:** For each intermediate `*PartialPullError{Stage: ...}` return (fetch/resolve/classify/reset/load-index/anchor-walk/reanchor), the card never states whether the accumulated `result` or a zero `PullResult{}` is returned alongside it — only step 2's weft-pull failure explicitly says `PullResult{}`. This matters most for the "reanchor" stage, where `WarpAdvanced`/`NewWarpHEAD`/`AnchorWarpSHA` are already set and the doc says "warp already advanced" should be knowable.
**Fix:** State explicitly that every post-weft-pull `*PartialPullError` returns the accumulated `result`, not a zero value, mirroring `Fabric.Commit`'s "report" convention (commit.go) the card already cites as precedent.

### [NIT] Card 8: contradictory error-handling pseudocode for the index load
**Location:** 02-fabric-pull.md, Card 8, step 9
**Issue:** The snippet `path, _ := f.corrIndexPath()` / `ix, _ := loadCorrIndex(path)` discards both errors via `_`, while the adjoining prose says "(errors → *PartialPullError{Stage: "load-index"})" — an implementer copying the literal snippet would silently swallow a real failure.
**Fix:** Rewrite the snippet to capture and check `err` for both calls, matching the stated behavior.

### [NIT] Card 7: `WarpFetched` field left undocumented
**Location:** 02-fabric-pull.md, Card 7
**Issue:** `PullResult`'s field list gives every field a parenthetical description except `WarpFetched bool`, despite the card requiring "each documented."
**Fix:** Add a one-clause description (e.g. "warp fetch ran and succeeded"), consistent with the other nine fields.

### [NIT] Card 12: fabric.go's own cross-repo-methods sentence stays stale
**Location:** 04-docs-sandbox.md, Card 12
**Issue:** `internal/fabricengine/fabric.go`'s package doc ("...adds a small set of genuinely cross-repo operations (`SyncWeft`, `RevertWithWeft`)...") is not in Card 12's Edits, so it stays stale after this change — still omitting the pre-existing `Commit` and now also omitting `Pull`, even though `doc.go`'s equivalent sentence is correctly updated.
**Fix:** Either add `internal/fabricengine/fabric.go` to Card 12's Edits with a one-line update, or note the omission is accepted pre-existing debt.

## Verdict

APPROVE
Plan is well-grounded in the actual source (gitrepo/fabricengine/fabriccli), internally consistent, DAG/coverage clean; only minor doc/spec-precision NITs remain.
MILL_REVIEW_END
