I've verified all the discussion's load-bearing claims against source: cwd-local gating in both `boardengine`/`warpengine` config.go, the `not initialized here; run "lyx init"` message, the `upsert [json-payload]` arg and `invalid json: %v` error wrapping, the embed/render in suite.go, and the `"Sandbox test-scheme"` heading the test pins. All accurate. Now the findings.

MILL_REVIEW_BEGIN
# Review: Refine SANDBOX-SUITE.md from the 2026-06-30 sandbox run

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-30
```

## Findings

### [GAP] S1 named in Problem but no remedy in Scope
**Section:** Problem (#40.3) / Decisions → board-durability-note
**Issue:** Problem and the board-durability rationale both say "S1/S3 wording assumes a fresh board," but the decision adds the durability/cleanup note only to S3; S1 ("Hub orientation") gets no edit and its current text shows no fresh-board assumption.
**Fix:** State explicitly whether S1 needs a change or is intentionally left untouched (covered by the S3 note), so the plan writer doesn't have to guess scope.

### [NOTE] "five refinements" but seven items listed
**Section:** Scope → In
**Issue:** The lead-in says "All five refinements land here:" then enumerates seven numbered items (1–7).
**Fix:** Correct the count to "seven" (or renumber) to keep the scope list self-consistent.

### [NOTE] cwd-relpath suite-note placement unspecified
**Section:** Scope item 7 / Decisions → external-readme-clarified-out-of-band
**Issue:** The decision says add "a short suite note in test-scheme.md" clarifying cwd-relpath mirroring, but names no target section (unlike the other notes, which pin Pre-conditions/S3); the phrase itself does not currently appear in test-scheme.md.
**Fix:** Name the section the note lands in (e.g. Notes or Pre-conditions) so placement isn't left to the plan writer.

## Verdict

GAPS_FOUND
One unresolved scope question (S1) plus two minor consistency notes; otherwise well-grounded and verified.
MILL_REVIEW_END