MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [NIT:consistency] plan-format-v3.md:5 link dangles, excluded anyway
**Demoted-from:** BLOCKING
**Section:** Scope → Out (`plan-format-v3.md:5` deferred to task C) vs `loom-md-links-fixed-prose-deferred`
**Issue:** `docs/reference/plan-format-v3.md:5` contains `[plan-format.md v2](plan-format.md)`, and this task deletes `docs/reference/plan-format.md` — the exact "link to a file this task deletes" case the loom.md decision says must be repaired regardless of owner, yet :5 is placed wholly out of scope.
**Fix:** Apply the loom.md rule symmetrically: state that this task repairs the dangling `plan-format.md` link at `plan-format-v3.md:5` while leaving C's "Coexistence, not replacement" prose rewrite alone.

### [NIT:scope] Acceptance grep does not cover the module-word sweep
**Demoted-from:** BLOCKING
**Section:** Testing → Acceptance commands, bullet 4 / decision `sweep-everything`
**Issue:** The decision claims "every `builderengine` / `buildercli` / builder-module reference", but the completion grep only covers `builderengine`, `buildercli`, and phase/gate tokens — module-word sites (`lyx builder`, `builder.yaml`, "builder suite", `builder-suite`) have no criterion, and verified un-inventoried sites exist: `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:28,:49,:123,:193,:195`, `tools/sandbox/suite.go:2`, `tools/sandbox/main.go:6,:12`.
**Fix:** Either add a third grep pattern for the module word with its own explicit exclusion list (noting `strings.Builder`/"content builder"/"fixture builders" are ordinary-English false positives that must be excluded by pattern, not by judgment), or narrow the `sweep-everything` decision to what the grep actually checks.

### [NIT:scope] S9 line range truncates the scenario body
**Section:** Markdown sites → `tools/sandbox/SANDBOX-CORE-SUITE.md:224–232`
**Issue:** S9 actually spans `:224–:286` — the plan fixture and `lyx builder status`/`validate` steps at `:234–:283` sit outside the quoted range, so deleting `:224–232` orphans the body.
**Fix:** Restate the range as the whole S9 block through its `**Verdict:**` line / the following `---`.

### [NIT:design] summary.md contract omits the integration-failure append
**Section:** `webster-contract-is-consumer-facing-only` (summary.md carry-over)
**Issue:** `internal/websterengine/summary.go` also exposes `AppendIntegrationFailure`, which extends an already-written `summary.md` with an integration-suite failure section — a consumer (Finalize's PR text) reads that content, but the carry-over list names only title + prose + `outcome: done` gating.
**Fix:** Say explicitly whether the appended integration-failure section is part of the consumer-facing contract or webster-internal.

## Verdict

REQUEST_CHANGES
Two blocking gaps: an asymmetric dangling-link exclusion and an under-scoped completeness grep.
MILL_REVIEW_END
