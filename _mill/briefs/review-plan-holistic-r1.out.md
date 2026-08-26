MILL_REVIEW_BEGIN
# Review: fabric: clone doesn't commit written module configs — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-26
```

## Findings

### [BLOCKING:scope] Card 4 Context omits internal/configsync/configsync.go
**Location:** Batch 3 (`03-clone-commits-module-configs.md`) / Card 4, test 4 ("One commit, not one per module").
**Issue:** Test 4's Requirements text calls `configsync.ReconcileAll` directly and reads its `Result.Applied` field ("a second `configsync.ReconcileAll` over the same weft base reports `Applied` false for every module"), but `internal/configsync/configsync.go` is listed in Card 3's Context, not Card 4's — Card 4's Context stops at `configreg/configreg.go` and `configengine/config.go`.
**Fix:** Add `internal/configsync/configsync.go` to Card 4's `Context:` list, matching Card 3's inclusion.

## Verdict

REQUEST_CHANGES
Card 4 calls `configsync.ReconcileAll`/`Result.Applied` without that file in its own Context list.
MILL_REVIEW_END
