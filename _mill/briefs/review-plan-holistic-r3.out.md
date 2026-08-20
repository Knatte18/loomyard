MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [BLOCKING:scope] Card 11 states a shedengine/producer.go fact outside its Context
**Location:** batch 4 / card 11 (`internal/shedadapters/doc.go`)
**Issue:** The `# Shared cancellation rule` addition must state that "`internal/shedengine/producer.go` binds every implementation to surface cancellation as a non-nil error and never as `Stuck`" — but card 11's `Context:` lists only `shedadapters/{burler,focus,perch,singlellm,ctx,archive}.go`, never `internal/shedengine/producer.go`. Neither card 6's `BurlerProducer` godoc requirement nor card 8's `Call` doc-comment requirement (which pins only the archive-rule keying, the two carve-outs, and the no-bridge reason) mandates this exact claim land verbatim in `burler.go`, so the implementer cannot verify it from any file in this card's Context alone.
**Fix:** Add `internal/shedengine/producer.go` to card 11's `Context:` list.

### [NIT:consistency] New validate() error path bypasses the existing double-prefix regression guard
**Location:** batch 1 / card 2 (`internal/burlerengine/profile_test.go`)
**Issue:** `TestProfile_Validate`'s shared table asserts every error carries exactly one `"burler: "` prefix — a guard written after a real bug (N1: a wrapped `ResolveFan` error double-prefixing). Card 2 places the new `ClusterExclude`-without-`ClusterFan` error case in a separate `TestProfileValidate_ClusterExclude` function that only asserts the error text names `ClusterExclude`, not the exactly-one-prefix invariant.
**Fix:** Either add the new case to the existing table, or add the same exactly-one-`"burler: "`-prefix assertion to the new test function.

## Verdict

REQUEST_CHANGES
Card 11 cites a fact from a file absent from its Context; one minor test-coverage gap in card 2.
MILL_REVIEW_END
