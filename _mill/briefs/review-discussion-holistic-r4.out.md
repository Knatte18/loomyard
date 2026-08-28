MILL_REVIEW_BEGIN
# Review: Producer-agnostic final-summary artifact + wire Finalize

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [BLOCKING:scope] Broken-fixture inventory misses landingshed's own tests
**Section:** Testing → "Existing fixtures the two changes break" **Issue:** The list is declared exhaustive ("carried by the plan, not discovered at `go test` time") but omits `internal/landingshed/finalize_integration_test.go:167-193`, which builds a `landingshed.Deps` with no summary field, calls `landingshed.NewFinalize(deps)` and then a real `fz.Call(ctx)` — so it breaks on *both* new gates (constructor empty-path rejection and the top-of-`Call` parse) and needs a real artifact on disk, not just a non-empty string; `internal/landingshed/commitstatus_test.go:52,74,99` likewise builds `&Finalize{...}` and calls `Call`, beyond the nine literals counted in `finalize_test.go` (those three are incidentally covered only if the shared `newFinalizeDeps` helper at `finalize_test.go:53` is what gets updated, which the discussion never says). **Fix:** Name `finalize_integration_test.go` explicitly with its own remediation (write a real artifact, set `FinalSummaryPath`) and state that the fixture fix lands in the shared `newFinalizeDeps` helper so every in-package `&Finalize{...}` site — `finalize_test.go` and `commitstatus_test.go` both — is covered.

## Verdict

REQUEST_CHANGES
Fixture-break inventory incomplete; an integration test running real `Call` is unaccounted for.
MILL_REVIEW_END
