MILL_REVIEW_BEGIN
# Review: config degrades to embedded template

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, opus-4/5 generation
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [NIT:consistency] Doc-surface table counted as five and seven
**Section:** Technical context → "Doc surface" (+ Q&A "Which docs must land") **Issue:** The lead-in says the file "carries five distinct claims", the table has seven rows, the closing sentence says "That is seven rows", and the Q&A restates "five stale claims". **Fix:** Say seven in all three places, or drop the numeral and refer to the table.

### [NIT:consistency] Same invariant contradiction left in an edited file
**Section:** Doc surface (pre-existing-falsehood table, lines 99-102 row) vs. "Any staleness outside this file is out of scope" **Issue:** `internal/configengine/config_test.go:383-385` carries the same claim the discussion calls "not optional" to fix ("now that configengine is the single declarer of the `_lyx` token"), contradicting the Lyxdirs Single-Declarer Invariant, in a file this task already edits. **Fix:** State a disposition for that comment explicitly (fix in-pass or defer with a reason) rather than covering it by the blanket out-of-file clause.

## Verdict

APPROVE
Scope, decisions, failure modes and source claims verified; only two cosmetic consistency nits.
MILL_REVIEW_END
