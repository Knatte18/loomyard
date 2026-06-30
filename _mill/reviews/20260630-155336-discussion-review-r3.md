MILL_REVIEW_BEGIN
# Review: Rename internal/paths to internal/hubgeometry

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\rename-paths-to-hubgeometry\_mill\discussion.md
date: 2026-06-30
```

## Findings

### [GAP] `package paths_test` external test files unaccounted for
**Section:** Technical context (line ~197) / Scope "In" (line ~50) / Testing no-dangling check (line ~253)
**Issue:** Four test files (`paths_test.go`, `paths_unit_test.go`, `geometry_test.go`, `weft_test.go`) declare `package paths_test`, not `package paths`; the claim "every *_test.go in the package declare `package paths` today and must become `package hubgeometry`" is factually wrong (they must become `package hubgeometry_test`), and the verification grep `grep -rn "package paths\b"` will NOT match `package paths_test` because `_` is a word char so `\b` fails between `paths` and `_test`.
**Fix:** State the external-test variant explicitly (`package paths_test` → `package hubgeometry_test`) and change the dangling-check pattern to `package paths(_test)?\b` (or drop `\b`) so a stale external-test clause is caught.

## Verdict

GAPS_FOUND
One factual error plus a holed verification grep around `package paths_test` external test files.
MILL_REVIEW_END
