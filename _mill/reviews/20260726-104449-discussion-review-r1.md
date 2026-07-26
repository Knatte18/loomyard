MILL_REVIEW_BEGIN
# Review: Treadle: shared round-loop engine + perch rewrite

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-07-26
```

## Findings

### [GAP] In-flight resume vs. config migration breaks ProfileHash
**Section:** Decisions › perch-api-and-identity-stability / config-and-modelspec-migration
**Issue:** The claim "in-flight blocks started by the old binary resume with no migration" collides with the fail-loud config change: the new binary rejects an old-format `perch.yaml` before any resume can start, and a block that ran with empty (provider-default) `JudgeEffort` cannot be reproduced once its migrated bare alias (e.g. `haiku`) picks up a seeded `models.yaml` default effort — the resolved `Profile.JudgeEffort` changes, so `ProfileHash` (which marshals the whole Profile incl. JudgeEffort — `state.go:95`) mismatches and `loadOrInitState` refuses the resume (`state.go:195`). `modelspec.parseBracket` also rejects an empty effort value, so there is no way to re-express "provider default" once a default is seeded.
**Fix:** State explicitly that an in-flight block whose resolved model/effort changes under the migration requires a fresh `--run-id` (resume not guaranteed), or spell out the exact equivalence conditions under which resume survives.

### [NOTE] `version`-param fail-loud lives in perch, not modelspec
**Section:** Decisions › config-and-modelspec-migration
**Issue:** "Unsupported bracket params (e.g. `version`) fail loud" reads as a `modelspec.Parse`/`Resolve` guarantee, but `version` is in `modelspec.knownParams` (`modelspec.go:116`), so Parse accepts `sonnet[version=4.5]` and Resolve merges it silently — nothing in modelspec rejects it.
**Fix:** Clarify the rejection is an explicit perch-layer check on `Resolved.Params` (only `effort` allowed) after resolution, since the grammar layer permits `version`.

### [NOTE] Handoff template structure under-specified
**Section:** Testing / Decisions › handoff-format-and-ledger
**Issue:** "Two new templates (handoff maintenance folded into judge calls, targeting)" is ambiguous about whether the handoff is emitted by extending the existing circling/milestone judge templates in the same call or by a distinct handoff template, which determines whether their pinned `template_test.go` statements change and how many spawns per round occur.
**Fix:** State whether handoff-ledger output is appended to the circling/milestone templates (single call) or is a separate template, and which templates get new pinned-statement tests.

## Verdict

GAPS_FOUND
One resume/identity gap in the migration must be resolved before plan writing.
MILL_REVIEW_END
