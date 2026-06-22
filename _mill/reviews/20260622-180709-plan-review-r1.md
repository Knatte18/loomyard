Confirmed: `store_test.go` has no `Save` calls and no Load-from-disk tests at all (it uses `NewStore("")` exclusively, exercising the in-memory path). Card 3 adds the first disk-backed Load tests. This is fine — the conditional language ("If an existing test...", "Update any test call of `Store.Save`") is defensive and accurate since none exist.

MILL_REVIEW_BEGIN
# Review: Adopt internal/state in board and muxpoc — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-22
```

## Findings

### [NIT] Card 3 references Store.Save calls that do not exist in tests
**Location:** Batch 2, Card 3
**Issue:** store_test.go uses `NewStore("")` exclusively and never calls `Save` or any disk-backed `Load`; the "Update any test call of `Store.Save`" requirement matches nothing.
**Fix:** Harmless (conditional phrasing); optionally note that Card 3 introduces the first disk-backed Load test, not an edit of an existing one.

### [NIT] muxpoc lock-file path silently migrates with no test asserting the new name
**Location:** Batch 3, Card 4
**Issue:** The lock moves `.lyx/muxpoc-state.lock` → `.lyx/muxpoc-state.json.lock`; no card adds an assertion pinning the new lock location (state package has TestLockFileLocation, muxpoc has none).
**Fix:** Optional — add a lock-path assertion in state_test.go; verified safe since `lockRelPath` has no external referrers.

## Verdict

APPROVE
Plan is accurate, complete, and faithfully grounded in the source; only two minor nits.
MILL_REVIEW_END
