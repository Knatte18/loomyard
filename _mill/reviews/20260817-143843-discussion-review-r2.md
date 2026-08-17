MILL_REVIEW_BEGIN
# Review: config degrades to embedded template

```yaml
duration_s: 221.4
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [NIT:consistency] Q&A log still answers Debug for the log level
**Demoted-from:** BLOCKING
**Section:** Q&A log (line 252) vs `### info-level-observability` (lines 88-96) and Q&A line 259
**Issue:** Line 252 answers "`logger.Debug` inside `configengine` … `logger.Info` as too loud, since standalone is meant to be the normal case", which is the round-1-falsified premise the decision section and line 259 both reverse — the artefact now carries two opposite answers to the same question with no supersession marker on the stale one (unlike lines 256/259, which do mark theirs).
**Fix:** Rewrite line 252 to answer `logger.Info` (or mark it superseded and point at line 259), so a plan writer cannot implement Debug from the Q&A log.

### [NIT:design] Import-widening analysis covers cycles only
**Section:** `### info-level-observability` rationale (line 95) / Technical context (line 128)
**Issue:** The no-cycle claim checks out (`internal/logger` imports only `lyxcwd`, `lyxdirs`, `proc`; none reaches `configengine`), but adding `internal/logger` also puts `logger`+`proc` into the transitive closure of two allowlist-capped importers of `configengine` — `internal/modelspec` (Modelspec Leaf Invariant) and `internal/gitkit` (gitkit Leaf Invariant) — which the discussion does not mention.
**Fix:** State that both enforcement tests are direct-import allowlists (`internal/modelspec/leaf_enforcement_test.go`, `internal/gitkit/leaf_enforcement_test.go` both walk this package's own files only), so neither invariant is tripped, rather than leaving the reader to derive it.

### [NIT:scope] Doc-surface enumeration misses configengine.md's key-properties bullet
**Section:** Technical context, "Doc surface" (lines 185-186) and Q&A line 263
**Issue:** The named stale ranges are 21-22, 30-41, 127-131, 138-149, but `docs/shared-libs/configengine.md:47` ("**Errors are strict**: missing template keys, absent files … cause hard errors") sits in the un-named 43-49 block and becomes equally half-false; the two lists also disagree on the second range (127-131 vs 124-131).
**Fix:** Add line 47 (the "Key properties" bullet) to the doc-update list and reconcile the two range citations.

## Verdict

APPROVE
One stale Q&A answer contradicts the chosen log level; two minor gaps otherwise.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
