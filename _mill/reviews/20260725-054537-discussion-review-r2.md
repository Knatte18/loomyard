MILL_REVIEW_BEGIN
# Review: plan-format v3: flat card list

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Repointed anchor link dangles after section trim
**Section:** Scope (link repoint) + `symbol-fields-deferred-compact` / `strip-execution-policy`
**Issue:** `webster-rewrite.md:32` links `plan-format-v3.md#continuous-dag-update-as-cards-land-designed-deferred-with-the-symbol-fields`, but the doc deliberately trims the "Continuous DAG update as cards land" / Mechanism-1/2 / SCC sections into a compact deferred stub — so the repoint fixes the path while leaving a dead fragment, and the "grep the filename resolves" test would pass anyway.
**Fix:** State how to handle that fragment — drop it, retarget to the compact deferred section's actual anchor, or point it at `webster-rewrite.md`/`codeintel-redesign.md` where the detailed design now lives.

### [NOTE] Default commit-subject text is doubly specified
**Section:** `card-fields-and-order` vs `numbering-and-commit-subject`
**Issue:** `card-fields-and-order` says the heading `<name>` is "used in the commit message when no explicit `Commit:` is given," while `numbering-and-commit-subject` says the subject is `N: <short what>`; v2's own worked example has heading title ≠ commit text, so which string seeds the default is ambiguous.
**Fix:** Pick one rule for the default commit body (card `<name>` from the heading, or a `<short what>` from `What:`) and state it once so the worked example is unambiguous.

## Verdict

GAPS_FOUND
One dangling-anchor repoint gap; a commit-text ambiguity noted but non-blocking.
MILL_REVIEW_END
