MILL_REVIEW_BEGIN
# Review: Build builder - the batch-implementation loop

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-11
```

## Findings

### [GAP] Resume has no plan-identity guard
**Section:** Decisions › Crash/resume; Builder run state
**Issue:** Resume cold-starts purely on "report present → batch done" from durable, weft-synced `_lyx/builder/state.json` + `_lyx/builder/reports`, but state.json's field list carries no plan fingerprint — a superseded/replaced plan reusing batch numbers 00,01… would have its stale prior reports misread as completed progress, and a run never re-initializes.
**Fix:** Decide whether state.json records a plan identity (hash/GUID) that forces fresh init on mismatch, or explicitly pin "one plan per builder dir; resume assumes the same plan, cleanup is manual/out-of-scope" so the plan writer doesn't guess the resume-vs-fresh boundary.

### [NOTE] No run-level lock against a duplicate orchestrator
**Section:** Decisions › Crash/resume
**Issue:** Re-running `lyx builder run` after a suspected crash spawns a fresh orchestrator; if the prior orchestrator session is in fact still live, two orchestrators drive the same single state.json (duplicate implementer spawn is partly guarded by strand-live → poll, but concurrent advance/commit is not).
**Fix:** Note whether `run` takes a builder-dir lock or relies on operator discipline; state the chosen stance.

## Verdict

GAPS_FOUND
Design is thorough and source-grounded; one resume-staleness gap needs a decision before plan writing.
MILL_REVIEW_END
