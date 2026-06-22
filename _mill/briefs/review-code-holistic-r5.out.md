MILL_REVIEW_BEGIN
# Review: Prune and consolidate the test suite (board first) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [NIT] ide name-map omits TestRunCLIUnknownSubcommand
**Location:** `docs/benchmarks/test-suite-timing.md:180-191`
**Issue:** The ide equivalence guardrail section lists 11 names under "ide (9 dropped)" but omits `TestRunCLIUnknownSubcommand`, which is in the ide baseline (`_mill/plan/baseline/ide.txt:13`) and was correctly folded into `TestRunCLIErrors/TestRunCLIUnknownSubcommand` in the code — the doc record is incomplete for that name.
**Fix:** Add `- TestRunCLIUnknownSubcommand → TestRunCLIErrors/TestRunCLIUnknownSubcommand` to the ide section of the guardrail name-map.

### [NIT] board name-map header count vs. list entries mismatch
**Location:** `docs/benchmarks/test-suite-timing.md:123`
**Issue:** The header "board (23 dropped via folding)" lists 34 bullet entries, but 5 of them (`TestRenderSingleTask`, `TestRenderStatusVariants`, `TestRenderToDisk`, `TestRerender`, `TestUpsertTask`) still exist as top-level test functions in the code, so they should not appear in a "dropped via folding" list; the 23 net-reduction count is correct but the list overstates which names were removed.
**Fix:** Move those 5 entries out of the dropped-via-folding section (or add a note clarifying they are still top-level funcs that became table-driven wrappers, not removed names).

### [NIT] board "After: 38" count is off by one
**Location:** `docs/benchmarks/test-suite-timing.md:95`
**Issue:** Counting top-level `TestXxx` functions actually present in the board package source files after the prune yields 37 (render 7 + cli 2 + board 6 + config 2 + init 2 + store 13 + layer 3 + task 2), not 38.
**Fix:** Correct the "After" cell for `internal/board` to 37 and update the Reduction and Total rows accordingly.

## Verdict

APPROVE
All code changes are correctly implemented; three doc-only inaccuracies in the equivalence guardrail section are all non-blocking nits.
MILL_REVIEW_END