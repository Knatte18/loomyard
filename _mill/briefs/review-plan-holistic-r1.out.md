MILL_REVIEW_BEGIN
# Review: lyx loom step + external supervisor skill — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5), per this session's own system info
reviewed_file: plan/
date: 2026-09-12
```

## Findings

### [BLOCKING:scope] Batch 2 card 5 cites `testEnv(t)` in the wrong file, and its real home is missing from Context
**Location:** `02-loomshed-interrupt-policy.md`, Card 5.
**Issue:** Requirements say to build the env/paths pair "with the existing `testEnv(t)` helper from `fixture_test.go`," but `testEnv` is declared in `internal/loomrecipe/shape_test.go` (verified: `grep func testEnv` returns only `shape_test.go:66`), which is a different helper from `fixture_test.go`'s `buildSequenceFixture`. `shape_test.go` is absent from Card 5's `Context:` (`interruptpolicy.go`, `loomshed.go`, `coverage_guard_test.go`, `loomrecipe.go`, `fixture_test.go`), so the file that actually declares the function the Requirements tell the implementer to reuse is not readable per the Context contract.
**Fix:** Correct the citation to `internal/loomrecipe/shape_test.go` and add that path to Card 5's `Context:` list (or drop `fixture_test.go` if it isn't otherwise needed).

## Verdict

REQUEST_CHANGES
One BLOCKING context-completeness/citation defect in batch 2 card 5; the rest of the plan checks out against source.
MILL_REVIEW_END
