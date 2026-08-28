MILL_REVIEW_BEGIN
# Review: Audit internal/logger coverage across spawn/hard-error paths

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:consistency] websterengine already has the proposed test shape
**Section:** Testing → `internal/websterengine`; Q&A r5 **Issue:** The claim "every existing test file in this package that could serve as a model is `//go:build integration` *and* external `package websterengine_test`" is false — `internal/websterengine/gitwrap_test.go` is `//go:build integration` + `package websterengine`, and `testmain_test.go` is internal, so the exact tagged-internal combination the decision reaches for already exists in-package. **Fix:** Name `gitwrap_test.go` as the model so the plan writer copies it rather than inventing the shape, and drop the "both obvious choices are wrong" framing.

### [NIT:consistency] `singlellm-parity` field set contradicts itself in one bullet
**Section:** Decisions → `singlellm-parity` **Issue:** The bullet says the new lines carry "the same fields the `OutcomeAsking` branch already emits", then enumerates `producer, engine, sessionID, strandGUID, runDir, outcome` — but `singlellm.go:171` also emits `lastAssistantMessage`, so the prose and the list disagree on whether that key is included. **Fix:** State explicitly whether `lastAssistantMessage` is carried on the `Died`/`Timeout`/`default` lines; the enumerated list should be the authority.

### [NIT:consistency] `withCapturedOutput` is unreachable regardless of export
**Section:** Testing → `internal/shedadapters` **Issue:** "check whether it is exported or needs a local equivalent" leaves a settled fact open — `withCapturedOutput` lives in `internal/logger/logger_test.go` under `package logger`, so no other package can reach it whether or not it were exported; a local equivalent is unconditionally required. **Fix:** State that `shedadapters` (and `mergeresolve`) each need their own `logger.SetOutput` capture helper, dropping the conditional.

## Verdict

APPROVE
Both selectors verified against the tree; every table row and blocked-import claim checks out.
MILL_REVIEW_END
