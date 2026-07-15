MILL_REVIEW_BEGIN
# Review: Reconsider whether lyx mux needs anchor:top at all — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-15
```

## Findings

### [NIT] Stale "top/stack" wording in smoke_test.go's file-header comment
**Location:** `internal/muxcli/smoke_test.go:9`
**Issue:** The shared smoke-harness file's header still describes the suite as covering "layout survival under mixed top/stack adds" — a description of the pre-redesign scenario. Card 7 renamed the actual test to `TestSmokeStackedAddsKeepEverySessionPane` and re-expressed it as below-parent-only (confirmed in `smoke_lifecycle_test.go:90-98`), but `smoke_test.go` was Context-only in card 7 (never in any batch's Edits/`All Files Touched`), so this one phrase was never swept.
**Fix:** Reword to "layout survival under stacked below-parent adds" (or similar) to match the renamed test; same category as the round-1-fixed `layout.go` stale doc, a plan gap rather than an implementer miss.

## Verdict

APPROVE
End-to-end plan alignment across all three batches confirmed; zero stray `AnchorTop`/`TopBandRows`/`top-band` references in internal/, docs/, tools/.
MILL_REVIEW_END
