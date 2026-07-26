MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite

```yaml
verdict: APPROVE
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-07-26
```

## Findings

### [NOTE] Handoff parse: "fail-loud" vs "never error" reads as tension
**Section:** Decisions § handoff-format-and-ledger / handoff-failure-fallback
**Issue:** "Go parses the frontmatter fail-loud at read" juxtaposed with "unparseable handoff logs a logger.Warn, never an error, never STUCK" can read as contradictory to a plan writer.
**Fix:** State explicitly the two-layer split (as in `ParseJudgeVerdict`): the parser returns an error on malformed input; the loop swallows that error to a Warn-and-fallback, never propagating STUCK.

## Verdict

APPROVE
Scope, decisions, failure modes, and testing are complete and source-grounded; one clarifying NOTE only.
MILL_REVIEW_END
