MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-26
```

## Findings

### [NIT] Card 7 in-sync precheck placement is imprecise
**Location:** Batch 3, Card 7
**Issue:** The card says insert the `ok=true` precheck "immediately after setup + Add, before the `fslink.Remove`", but `TestPairInSync_BrokenJunction` only becomes in-sync after `WireJunctions` + `paths.Resolve` (drift_test.go:116-124); placing it right after `Add` would observe a missing junction and fail.
**Fix:** Reword to "after `WireJunctions` and the `hostLayout` resolution, before `fslink.Remove`".

### [NIT] Card 9 omits the pre-drift Status() call it requires
**Location:** Batch 3, Card 9
**Issue:** The field-population assertions must run on a healthy pair, so a `Status()` call is needed before `git checkout -b drifted` (status_test.go:101); the card says "before the mutation" but, unlike Card 10, never states that a separate `Status()` invocation is added.
**Fix:** State explicitly: call `Status()` for the field assertions before the drift, then `Status()` again after for the drift check.

### [NIT] Card 6 ForkPointMirrorsHost deletion drops the main-case positive assertion
**Location:** Batch 2, Card 6
**Issue:** `TestWeftForkPointMirrorsHost` positively asserts merge-base==main tip when spawning on main; `SubtaskIsolation` only asserts fork==Y tip and !=main tip (weftwiring_test.go:433-541), so the simple main-fork-point equality is not literally reproduced.
**Fix:** Confirm this deletion is intended per the discussion delete-table; the fork-from-parent mechanism is still exercised by SubtaskIsolation, so acceptable, but note the lost positive case.

## Verdict

APPROVE
Plan is constraint-clean, DAG/numbering valid, no coverage dropped; only minor wording nits.
MILL_REVIEW_END
