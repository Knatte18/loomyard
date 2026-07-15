MILL_REVIEW_BEGIN
# Review: lyx mux remove errors when it empties the last session

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-15
```

## Findings

### [NOTE] Swallow depends on has-session exit-1 for server-gone
**Section:** classify-via-hassession-reprobe / doc-correction-in-scope
**Issue:** The swallow triggers only on `hasSession → (false, nil)`, but the bug is a *server* exit, not just a session removal; `hasSession` (overlay.go:90) maps this to `(false, nil)` only because tmux `has-session` exits 1 on "no server running" — the same exit-1 the reproduction showed for `listPanes`, so this is grounded, not predicted.
**Fix:** State this exit-code dependency explicitly in the new doc.go assumptions bullet so the "server-gone still classifies as (false,nil)" fact is not silently re-lost, and confirm the integration test's success-assertion is what pins it on tmux.

## Verdict

APPROVE
Thorough, source-grounded discussion; all decisions carry rationale and rejected alternatives, no blocking gaps.
MILL_REVIEW_END
