MILL_REVIEW_BEGIN
# Review: webster: stop re-rendering already-inherited context into fork prompts

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] Q&A log contradicts card-pointer decision
**Section:** Q&A log (lines 135, 139) vs `card-pointer-relative-via-hubgeometry` / round-4 Q (line 141)
**Issue:** Lines 135 ("render.go composes the Cwd-relative pointer via hubgeometry") and 139 ("verbatim (filepath.Join under WorktreeRoot, filepath.Rel against Cwd)") still prescribe the exact composition the Decision and round-4 Q explicitly forbid ("does NOT filepath.Rel/filepath.Join-compose … rendered verbatim").
**Fix:** Reconcile the stale round-1/round-2 Q&A entries to the bare-token-verbatim resolution so a plan writer cannot lift the reversed composition instruction.

### [NOTE] Shared-body banner comment leaks under byte composition
**Section:** `shared-implementer-body` Decision
**Issue:** `stencil.stripLeadingComment` strips only a `<!-- -->` banner at the very top of the composed text; every existing webster template asset carries such a banner, so composing `prefix + shared-body` would leave the body asset's own banner mid-template, rendered literally into the prompt.
**Fix:** State that only the prefix asset may carry a leading banner (body asset/constant has none), so nothing leaks after byte composition.

## Verdict

GAPS_FOUND
One stale Q&A entry still prescribes the composition the card-pointer decision reverses.
MILL_REVIEW_END
