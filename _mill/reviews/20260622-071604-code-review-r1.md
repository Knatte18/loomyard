MILL_REVIEW_BEGIN
# Review: Optimise and slim the rest of the test suite — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [BLOCKING] Docs names `CopyBoardRepo` as a real fixture function it isn't
**Location:** `docs/benchmarks/test-suite-timing.md:221`
**Issue:** The parallel-safety note lists `CopyBoardRepo` among the lyxtest fixture helpers (`CopyBoardRepo`, `CopyHostHub`, `CopyWeft`, `CopyPaired`), but `CopyBoardRepo` was never added to `internal/lyxtest/lyxtest.go` — the implementer used `CopyWeft` directly for all sync tests (the valid fallback path), so no `CopyBoardRepo` exists in the codebase. Referencing a non-existent function in the durable benchmark doc is a factual error and will mislead future readers.
**Fix:** Replace `CopyBoardRepo` with `CopyWeft` in that list, or drop it; add a parenthetical noting that the `CopyBoardRepo` fallback was evaluated and not needed.

### [NIT] "Reducing wall-clock" section gives stale floor numbers
**Location:** `docs/benchmarks/test-suite-timing.md:235-242`
**Issue:** The pre-existing "Reducing wall-clock" subsection still describes `internal/board` (~24 s) and `internal/ide` (~12 s) as the current Tier-1 floor and suggests applying the shared-fixture/build-tag split to board as "the next highest-leverage move" — but this task has just completed that work, rendering the advice obsolete and potentially confusing.
**Fix:** Append a one-line update note such as "As of `optimize-remaining-test-suites` both packages are now offline (board ~0.7 s, ide ~0.6 s Tier 1); the floor is now build/link overhead across packages." — or update the body of item 3 in place.

## Verdict

REQUEST_CHANGES
One blocking factual error in the docs (non-existent `CopyBoardRepo` named as a real helper); one stale paragraph.
MILL_REVIEW_END
