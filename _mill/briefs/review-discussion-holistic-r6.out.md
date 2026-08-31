MILL_REVIEW_BEGIN
# Review: Surface merge-in-progress in fabric status

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:consistency] Merge-record path paraphrase adds a stray `.git`
**Section:** Technical context → "The two merge predicates" **Issue:** The discussion writes the record location as `<weft gitdir>/.git/fabric-merge.json` for the prime pair, while `mergestate.go:215-217`'s own comment says `<weft gitdir>/fabric-merge.json` (the gitdir already *is* `.git` for prime). **Fix:** Quote the engine's wording so a plan writer copying the path does not double-count `.git`.

## Verdict

APPROVE
Scope, decisions, doc inventory, constraints and test tiering all verified against source; no blockers.
MILL_REVIEW_END
