MILL_REVIEW_BEGIN
# Review: config degrades to embedded template

```yaml
duration_s: 308.7
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-17
```

## Findings

### [NIT:consistency] Shared-body shape contradicts two later decisions
**Demoted-from:** BLOCKING
**Section:** `### shared-body-refactor` vs `no-missingkeys-on-the-fallback-path` + `fallback-error-messages-name-the-template`
**Issue:** `shared-body-refactor` states the flag "is consulted at exactly the two refusal branches" and that "every other step — `MissingKeys`, `envsource.Build`, `yamlengine.Resolve` — is shared verbatim", but the other two decisions require the fallback path to *skip* `MissingKeys` and to wrap `Build`/`Resolve` failures with template-named prose instead of `config file %s` — i.e. divergence at all three of those steps, and its rationale ("cannot diverge on … key validation, or error wording") is false as written.
**Fix:** Restate the body's shape concretely — each refusal branch returns through one fallback tail that skips `MissingKeys` and wraps errors as `%s config template: %w` — and narrow the anti-drift rationale to what genuinely stays shared (the env-marker resolution semantics), so a plan writer cannot implement the "verbatim" reading.

### [NIT:scope] Doc-surface table claims completeness it does not have
**Section:** Technical context → "Doc surface" (declared "the single authority")
**Issue:** Two further stale claims sit inside the sections being rewritten and are unlisted: `docs/shared-libs/configengine.md:136` calls `Load` a "five-step flow" while the same file's Resolution model lists six steps, and `:87` still names `board.LoadConfig`/`worktree.LoadConfig`/`weft.LoadConfig` as the typed wrappers — the same staleness class the task fixes in `config.go`'s header.
**Fix:** Add both rows to the table, or drop the "complete list"/"single authority" framing.

### [NIT:scope] Markdown Link Integrity absent from Constraints
**Section:** `## Constraints` (From `CONSTRAINTS.md`)
**Issue:** `docs/shared-libs/configengine.md` is a scanned source for `TestEnforcement_MarkdownLinks`, and the doc pass adds a new section plus a `CONSTRAINTS.md`-referencing correction at :102, yet the invariant is not acknowledged; the file today contains no inline links, so any added link is newly exposed to the guard.
**Fix:** Name Markdown Link Integrity in Constraints and state that any link added to that file (including an `#anchor` into `CONSTRAINTS.md`) must resolve.

### [NIT:consistency] Membership-rule attribution overstates the design doc
**Section:** `### membership-rule-is-a-standalone-entry-point`, Rationale
**Issue:** "this is the rule the design doc actually applied" — `producers-standalone.md:271` argues Webster's placement on exactly the rejected governs-what grounds ("operator-tunable producer config … not hub state, the same distinction that keeps `burlerengine` off the strict list"); only :272 adds the standalone-entry reason. The design applied both.
**Fix:** Reword to "the design gave both rationales; the standalone-entry one is the only one that survives `loomengine`", so the surviving contradiction at :271 is visible rather than papered over.

## Verdict

APPROVE
One internal contradiction over where the fallback flag is consulted; everything else verified sound.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
