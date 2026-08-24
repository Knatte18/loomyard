MILL_REVIEW_BEGIN
# Review: webster: DAG-derived card sequencing — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (per system context; exact model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-24
```

## Findings

### [BLOCKING:scope] Card 6 Context omits render.go
**Location:** Batch 2, Card 6 (recover-batch predecessor digest). **Issue:** Requirements states "`RenderRecoveryPrompt`'s own empty-digest sentinel handling is unchanged" — `RenderRecoveryPrompt` is defined in `internal/websterengine/render.go`, but Card 6's `Context:` list carries only `_mill/discussion.md`, `batcher.go`, `beginbatch.go`, `sequence.go`, `state.go`; `render.go` is in neither Context nor Edits. **Fix:** Add `internal/websterengine/render.go` to Card 6's Context.

### [BLOCKING:scope] Card 11 Context omits integration_test.go
**Location:** Batch 2, Card 11 (Run-level sequencing tests). **Issue:** Requirements instructs building the new fixture-mutation helper "modelled on however `integration_test.go`'s own `appendIntegrationVerify` mutates a seeded plan" — but `internal/websterengine/integration_test.go` is not in Card 11's Context (only `internal/websterengine/integration.go`, a different file, is listed), so the implementer has no cold-start access to the function it is told to model its helper on. **Fix:** Add `internal/websterengine/integration_test.go` to Card 11's Context.

### [NIT:consistency] Card 2 overstates the external-test-package convention
**Location:** Batch 1, Card 2. **Issue:** Requirements claims package `websterengine_test` "match[es] the external-test-package convention the rest of the package's `_test.go` files already use," but roughly half of `internal/websterengine`'s existing `_test.go` files (e.g. `classify_test.go` and `testmain_test.go`, both in this card's own Context, plus `digest_test.go`, `fingerprint_test.go`, `report_test.go`) use internal `package websterengine` instead. **Fix:** Reword to say the choice matches the specific sibling files this task actually touches (`template_test.go`, `beginbatch_test.go`, `recoverbatch_test.go`, `runlevel_test.go`), not "the rest of the package."

### [NIT:design] Cycle visibility drops discussion.md's "Run logs" half
**Location:** Batch 2, Card 4. **Issue:** `_mill/discussion.md`'s `cycle-visibility` decision states cycles surface two ways — "`Run` logs one line per detected cycle" and the run result — but Card 4 wires only the result/warnings half (`RunResult.Cycles`, prepended `Warnings`); `internal/websterengine` has no `internal/logger` import today and this plan adds none. **Fix:** Either add a `logger.Info`/`Warn` call per detected cycle in Card 4, or have the plan note that the log-line half is deliberately dropped in favor of the `warnings`/`cycles` envelope keys added in Card 7.

## Verdict

REQUEST_CHANGES
Two Context-completeness gaps (Cards 6, 11) are mechanical fixes; the rest of the plan is well-grounded against source.
MILL_REVIEW_END
