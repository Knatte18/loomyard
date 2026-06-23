MILL_REVIEW_BEGIN
# Review: Extract internal/proc (cross-OS windowless + detached spawn) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-23
```

## Findings

### [NIT] proc_other.Detach drops process-group isolation flag
**Location:** Batch 1 / Card 1
**Issue:** The non-Windows `Detach` sets only `Setsid: true`, which is behavior-preserving versus the deleted board/weft/muxpoc spawn_other files (all used `Setsid: true`), so this is correct — but the Windows `Detach` adds `createNewProcessGroup` while non-Windows relies solely on Setsid for group isolation; worth a one-line code comment noting Setsid alone provides the new-session equivalent.
**Fix:** Add a comment in `proc_other.go` Detach explaining Setsid is the non-Windows equivalent of the Windows new-process-group + windowless combination.

## Verdict

APPROVE
Plan is accurate, behavior-preserving, well-sequenced, and fully source-grounded.
MILL_REVIEW_END
