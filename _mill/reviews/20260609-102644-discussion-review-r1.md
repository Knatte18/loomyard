Confirmed: proposals depend on `t.Body`, and seedWiki sets `Brief` (not `Body`), so no proposal files. The writer upserts `task-0` with only slug+title, so no body. 3 AtomicWrites per write holds throughout. All claims in the discussion are accurate.

The discussion is thorough. Let me assess against the criteria. One subtle point: the writer upserts `task-0` setting title, and the test asserts `len(tasks) != 100` stays true. The discussion claims UpsertTask of an existing slug doesn't change count — verified the writer comment says "never adds/removes". The reader reads `task-50`. All consistent.

Let me now produce the review. All major claims are source-grounded and accurate. The decisions all have rationale + rejected alternatives. Testing strategy is named. I'll check for genuine gaps only.

One thing worth noting: the validation step says tune `writes` within [40,75] but the constraint says 50 gives ~1s. The `task.Slug != "task-50"` assertion mentioned in Testing — the test reads task-50 and asserts found. That's fine. No real gaps. Let me consider the `-race` note: validation step 3 says run with `-race` once "not committed into the test" — this is clear.

MILL_REVIEW_BEGIN
# Review: boardtest-concurrency-speed

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\boardtest-concurrency-speed\_mill\discussion.md
date: 2026-06-09
```

## Findings

### [NOTE] Race-window claim is plausible but unverified
**Section:** Decisions § reduce-writes-constant
**Issue:** The "hundreds of thousands of reader iterations over a ~1s window" figure and the "~1–1.5s isolated" projection are extrapolations from the 8.2s/300-write baseline, not measured at writes=50; the actual isolated time depends on the same XDR/contention variance the discussion blames for the multi-minute spread.
**Fix:** None required — Testing step 1 already gates this empirically with a [40,75] tuning band; just acknowledged as a projection, not a guarantee.

### [NOTE] Lower bound of overlap not asserted in-test
**Section:** Constraints (determinism)
**Issue:** Correctness depends on the writer still running while readers loop, but nothing in the test fails fast if the writer finishes before readers start (e.g. on a fast/quiet machine writes=50 could complete in well under the reader spin-up); the test would still pass green while exercising near-zero overlap.
**Fix:** Optional — note that overlap is validated once via `-race` (Testing step 3) rather than enforced structurally; if stronger guarantee is wanted, a plan could mention asserting the writer is still live at first reader read, but this is out of the stated minimal-change scope.

## Verdict

APPROVE
All decisions grounded and source-verified; only non-blocking projection/overlap notes.
MILL_REVIEW_END
