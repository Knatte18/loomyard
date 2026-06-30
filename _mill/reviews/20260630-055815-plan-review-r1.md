I have everything I need. I verified the key load-bearing claims against source: the `not initialized here; run "lyx init"` string in `boardengine/config.go` (line 63), the board upsert help example in `boardcli/cli.go` (line 117), and critically the PowerShell error-envelope example in Card 2 — `output.Err` marshals a `map[string]any`, so Go emits keys alphabetically (`error` before `ok`), which exactly matches the card's `{"error":...,"ok":false}` example.

MILL_REVIEW_BEGIN
# Review: Refine SANDBOX-SUITE.md from the 2026-06-30 sandbox run — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-30
```

## Findings

### [NIT] Card 2 implies a pre-documented default shell
**Location:** Batch 1 / Card 2
**Issue:** Requirements says "on the suite's documented default shell (Windows PowerShell)", but neither `test-scheme.md` nor `docs/sandbox-hub.md` currently documents any default shell, so the phrasing presupposes documentation that does not exist.
**Fix:** Reword the note to *establish* PowerShell as the assumed session shell rather than reference it as already documented.

### [NIT] Batch-scope refinement count is off-by-one
**Location:** Batch 1 / Batch Scope
**Issue:** Scope says "all seven meta/doc refinements" but the six cards deliver eight distinct edits (Card 1 bundles Operating-model paragraph + S4 reword + S6 verdict note).
**Fix:** State "seven" reflects grouped items, or correct the count, so the scope line matches the card breakdown.

### [NIT] Card 4 Context lists cli.go but names no identifier from it
**Location:** Batch 1 / Card 4
**Issue:** The durability/cleanup note is empirical session prose (the `T1 "Test task from S3"` persistence is an observed sandbox-board fact, not present in any provided file); `internal/boardcli/cli.go` in Context names no referenced identifier.
**Fix:** Either cite the specific cli.go board command the note leans on, or drop the Context entry as non-load-bearing.

## Verdict

APPROVE — well-grounded single-file doc plan; verified error strings and JSON envelope ordering match source.
MILL_REVIEW_END