MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Absent plan-level `## verify:` unhandled
**Section:** Decision *integration-suite-fork-with-bisect* / Scope "Integration suite"
**Issue:** `plan-format-v3.md` §"verify model" makes the plan-level `## verify:` **optional**, but the decision unconditionally spawns "one dedicated fork [that] runs the plan-level `## verify:` once"; behavior when `00-overview.md` has no `## verify:` section is undefined (skip the integration fork? error?).
**Fix:** State that a plan with no `## verify:` section skips the integration fork/bisect stage entirely and proceeds straight to the summary/finish path.

### [NOTE] Per-card `verify:` failure semantics unstated
**Section:** Decision *per-card-verify-executed* / *fork-return-contract*
**Issue:** The fork runs a card's `verify:` after committing, but the decision never says a failing `verify:` yields `FAILED` (the build+unit gate's failure disposition is stated; `verify:`'s is only implied).
**Fix:** Note that a non-zero card `verify:` fails the card exactly like the build+unit gate, returning `FAILED` with the reached SHA.

## Verdict

GAPS_FOUND
One empty-state gap: optional plan-level `## verify:` absence has no defined behavior.
MILL_REVIEW_END
