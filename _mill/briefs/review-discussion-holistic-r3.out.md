MILL_REVIEW_BEGIN
# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewed_file: _mill/discussion.md
date: 2026-09-23
```

## Findings

### [NIT:scope] RunDir doc comment names the retired probe
**Section:** Scope, "Doc updates" **Issue:** `internal/shuttleengine/run.go`'s `RunDir` doc comment says "batch 4's pane-liveness probe refuses the bootstrap with exactly this path", and the enumerated doc-comment list omits it, so it goes stale when `awaitDriverPane` is deleted. **Fix:** Add `RunDir`'s doc comment to the doc-update list, or make the list a class ("every comment naming the pane-liveness probe") instead of an enumeration.

### [NIT:consistency] Recipe step 6 jq check contradicts step 7
**Section:** Reproduction recipe, steps 6 and 7 **Issue:** Step 6 treats `.projects | has($p)` turning `true` as proof that Claude recorded the acceptance, but step 7 states Claude Code appends an entry for every launch directory anyway, so `has` can turn true without the gate being accepted. **Fix:** Make step 6 check the acceptance field on the entry (for example `.projects[$p].hasTrustDialogAccepted`), or drop the claim and rely on the pane capture plus `started: true`.

## Verdict

APPROVE
Mechanism, result mapping, lock semantics and tripwire pins match the verified source; only two non-blocking doc/recipe nits remain.
MILL_REVIEW_END
