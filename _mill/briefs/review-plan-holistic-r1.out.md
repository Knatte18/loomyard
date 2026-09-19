MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [NIT:design] Card 15's CLI error string names an unsourced "remote name"
**Location:** batch 4, card 15 **Issue:** The required `fmt.Errorf("weft branch %q was deleted locally, but its copy on %q was not: %s", ...)` needs "the remote name," but `originRemoteName` (card 4, `destroy.go`) is unexported and card 15's Context (`remove.go`, `envelope.go`) exposes no constant fabriccli can read it from. **Fix:** State explicitly that fabriccli hardcodes the literal `"origin"` here, matching the rest of the codebase's hardcoded-origin posture (card 4's own rationale), so the implementer isn't left inventing the source.

## Verdict

APPROVE
Every batch, decision, and cross-file claim verified against source; only one trivial CLI-string sourcing gap found.
MILL_REVIEW_END
