MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe

```yaml
duration_s: 540.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:consistency] Cancellation test calls neither New nor Run
**Demoted-from:** BLOCKING
**Section:** `row1-substitution` / `test-ownership` / Testing
**Issue:** `TestCancellation_RealProducersReturnErrorNotStuck` (`internal/loomshed/resume_test.go:331-361`) constructs five producers directly (`NewDiscussionValidate`, `NewPlanValidate`, `NewBatchifier`, `NewWebsterProducer`, `NewLoomPreflight`) and calls `Call` on each — it never calls `New` and never calls `Run`, so "eight tests … more than ten call sites" and "every one of these eight `Run`-driving tests substitutes row 1" are both false for it (the real counts are seven Run-driving tests and exactly ten `New` sites: 1 + 2 + 2 + 2 + 1 + 1 + 1).
**Fix:** correct the counts, and state its disposition explicitly — its subject is `loomshed`'s own five constructors, which is the criterion the same Decision uses to keep `batchifier_test.go`/`planvalidate_test.go` in `internal/loomshed`, so say whether it moves anyway (only `buildSequenceFixture`'s paths tie it to the fixture) and why.

### [NIT:consistency] countingProducer must be one instance across both New calls
**Section:** `row1-substitution`
**Issue:** `TestResume_CrashRecoveryRecallsUnconditionally` holds a single `counting := &countingProducer{}` across `New` at `resume_test.go:114` and `:131`, asserting `counting.calls == 2` at `:142`; the Decision's wording "replaces row 1 with `countingProducer{}`" reads as a fresh value at each substitution, which would leave the count at 1.
**Fix:** state that the same producer instance is substituted at every `New` in that test, not a fresh one per substitution.

### [NIT:scope] Comment-sweep token set misses two live stale sites
**Section:** Scope (doc-comment carve-out)
**Issue:** the prescribed grep (`loomshed.New`/`loomshed.Deps`/`coverage_guard_test`) does not match `internal/loomshed/doc.go:1-2` ("Package loomshed owns loom's own ordered producer list and returns a constructed `*shedengine.Shed`"), falsified by deleting `New`, nor `internal/preflightshed/preflight_test.go:33`, which names `internal/loomshed/resume_test.go`'s moved `TestCancellation_…` test.
**Fix:** widen the sweep tokens to include `internal/loomshed/` path mentions and the moved test-file names (`resume_test`, `sequence_test`, `loomshed_test`), and name `internal/loomshed/doc.go` as a known site.

### [NIT:consistency] "Three constructors reach disk" is true but not of loom's rows
**Section:** Technical context — "`Build` is not filesystem-free"
**Issue:** per `internal/shedbuild/doc.go:8-12` the three disk-touching constructors are exactly `Bouncer`, `BurlerRound`, and `SingleLLM` — the three engines the discussion elsewhere records as unused by loom's thirteen rows — so loom's own `Build` touches disk only through `landingshed.NewPublish`/`NewFinalize`'s pair opener.
**Fix:** say so, so the sentence is not read as a tier-1 obstacle for `internal/loomrecipe`'s tests or as a behaviour change from `loomshed.New`.

## Verdict

APPROVE
One named test is mis-described and its disposition unsettled; everything else verified accurate.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
