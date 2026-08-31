MILL_REVIEW_BEGIN
# Review: Reconsider the collapsed strand strip default size

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:scope] loom.md symbolic reference not dispositioned
**Section:** Scope / Out, Technical context ("Symbolic doc references") **Issue:** `manifest/designs/loom.md:440` also says "collapsing to `collapsed_strip_rows`" symbolically, but only the two sandbox suites are enumerated as symbolic-and-no-edit. **Fix:** Add `manifest/designs/loom.md` to the same symbolic-reference list so the no-edit disposition is explicit rather than inferred.

### [NIT:consistency] Divider rows double-counted in one clamp sentence
**Section:** Decisions / `strip-default-six` **Issue:** "the stack box is 28 rows, so four strips plus their four dividers consume all 24 usable rows" — the 24 is already net of the four dividers, so the strips alone consume 24; the phrasing reads as 28. **Fix:** Reword to "four dividers leave 24 usable and four 6-row strips consume all 24"; the derived conclusion (active pane 0, clamp fires) is correct and needs no change.

## Verdict

APPROVE
Every load-path, render, test and reconcile claim verified against source; both findings are wording-level.
MILL_REVIEW_END
