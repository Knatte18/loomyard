MILL_REVIEW_BEGIN
# Review: plan-format v3: flat card list

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [NOTE] webster-rewrite.md plain links not enumerated
**Section:** Scope → "Repoint the deleted design doc's inbound links"
**Issue:** The enumeration lists loom/loom-planner/codeintel-redesign/webster-parallel-execution for plain repoints and pulls webster-rewrite.md out for line-32 anchor handling only, but webster-rewrite.md also has plain links at lines 5, 72, 165, 190 that dangle after the design doc is deleted.
**Fix:** State that webster-rewrite.md's other plain `plan-format-v3.md` links also repoint to `../../docs/reference/plan-format-v3.md` (the "Repoint all of them" header + Testing grep cover it, but the per-file bullet reads as anchor-only).

### [NOTE] Q&A log commit-subject wording contradicts decision
**Section:** Q&A log (line ~460) vs Decision `numbering-and-commit-subject`
**Issue:** The Q&A entry says `commit N: <short what>`, but the resolved decision (and Q&A 473-475) sets the default to `N: <name>` and explicitly rejects a separate `<short what>` seed as the round-2-flagged double-spec.
**Fix:** Update the Q&A line to `N: <name>` so the authoritative doc is internally consistent.

## Verdict

APPROVE
Prior-round gaps resolved; only two non-blocking doc-consistency notes remain.
MILL_REVIEW_END
