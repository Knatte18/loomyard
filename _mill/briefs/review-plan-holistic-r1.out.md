MILL_REVIEW_BEGIN
# Review: CLI help & error ergonomics from sandbox run — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-28
```

## Findings

### [NIT] Card 12 references output.Err without output.go in Context
**Location:** Batch 3 / Card 12
**Issue:** `Requirements` invokes `output.Err` (defined in `internal/output/output.go`), which is not in Card 12's `Context`/`Edits`; strictly a context-completeness gap.
**Fix:** No real cold-start risk since both Edits files (`clone.go`, `warp.go`) already import and use `output.Err`; optionally add `output.go` to Context for symmetry with cards 2/3/17.

### [NIT] Card 19 references configreg.Names() without configreg.go in Context
**Location:** Batch 5 / Card 19
**Issue:** The harmonized unknown-module message uses `configreg.Names()`, but Card 19's `Context` lists only `output.go`.
**Fix:** Symbol is already used verbatim in the Edits file `configcli.go:43`, so no exploration is needed; add `configreg.go` to Context to satisfy the rule cleanly.

### [NIT] Card 17 should explicitly preserve Args: MaximumNArgs(1)
**Location:** Batch 5 / Card 17
**Issue:** Switching `config` to a `configCmd` variable names `ValidArgs` (card 18) but neither card states the existing `Args: cobra.MaximumNArgs(1)` must be retained.
**Fix:** Add a one-line requirement to preserve `Args: cobra.MaximumNArgs(1)` so `config a b c` still rejects.

### [NIT] Stale comment in cmd/lyx/main_test.go after batch 5
**Location:** Batch 5 (fallout) / Card 19
**Issue:** `TestRunDispatchesToConfig` comment ("config output is human-readable text (not JSON)") becomes false once config errors are JSON-harmonized; the test still passes (exit-code only) but the comment rots, and main_test.go is edited only in batch 1.
**Fix:** Optionally update that comment when touching main_test.go in card 4, or note it in card 19.

## Verdict

APPROVE
Plan is sound, constraint-compliant, correctly sequenced, and accurately grounded in the cobra/source behavior.
MILL_REVIEW_END
