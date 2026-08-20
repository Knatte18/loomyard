MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (self-assessed; exact point version not independently verifiable from inside the session)
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [NIT:consistency] Card 22's rationale for dropping the internal/clihelp banned-import entry is wrong
**Location:** batch 3, card 22 **Issue:** Card 22 claims the surviving `/internal/`+suffix-`cli` rule in `seam_enforcement_test.go` "already covers" `internal/clihelp`, but `strings.HasSuffix(importPath, "cli")` is false for `"...internal/clihelp"` (it ends in "help", not "cli") — the two checks are independent, and the real reason dropping the entry is safe is that no `clihelp`-named package exists in quarry at all. **Fix:** Reword the justification to state the entry is dropped because quarry has no such package, not because the suffix rule subsumes it.

## Verdict

APPROVE
Every batch, decision, and numeric claim I spot-checked against source (call-site counts, line citations, DAG, card numbering, All Files Touched) verified accurate.
MILL_REVIEW_END
