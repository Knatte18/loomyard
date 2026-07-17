MILL_REVIEW_BEGIN
# Review: loom: Preflight phase (precondition validation) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-17
```

## Findings

### [NIT] Preflight godoc slightly misdescribes checkResolved's reachability
**Location:** `internal/loomengine/preflight.go:25-27`
**Issue:** Preflight's godoc says a caller with a resolved Layout "cannot reach checkResolved directly, since that helper is unexported"; checkResolved's own godoc (line 71) says integration tests do call it directly for isolation. Both are true (in-package vs out-of-package) but juxtaposed they read as contradictory.
**Fix:** Qualify the first sentence with "outside this package" to remove the apparent tension.

### [NIT] Unreachable default branch in weft-sync/junction classification
**Location:** `internal/loomengine/preflight.go:124-133`
**Issue:** The `default: check = CheckWeftSync` branch is unreachable given `warpengine.PairInSync`'s only two non-ok reason strings ("host on …" and "junction …"), both already matched by the two case prefixes.
**Fix:** None required — acceptable as defensive handling of a future new reason string; no action needed, noted for awareness only.

### [NIT] status-schema.md's "Required fields" opening sentence still lists all 9 fields as required
**Location:** `docs/reference/status-schema.md:127-135`
**Issue:** The bullet's first sentence ("Required fields (slug, parent, phase, stage, narration, history, start_sha, pause_requested, next_action) are present") still enumerates all nine fields before the following sentences clarify that only the five mandatory strings are structurally presence-enforced — Card 10's requested clarification was applied but the opening list still reads as if all nine are required.
**Fix:** Optionally trim the opening list to the five mandatory strings, or add "(see below for what 'present' means per field)" — cosmetic only, the clarification that follows already resolves the ambiguity.

## Verdict

APPROVE
Implementation matches the plan precisely across all three batches; no constraint violations, no cross-batch contract gaps.
MILL_REVIEW_END
