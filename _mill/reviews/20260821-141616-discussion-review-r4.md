MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe

```yaml
duration_s: 215.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:scope] "Both moved Run tests" undercounts eight
**Demoted-from:** BLOCKING
**Section:** `row1-substitution`, Testing **Issue:** `internal/loomshed/resume_test.go` holds **seven** Run-driving tests (`TestResume_DoesNotRestartAtRowOne`, `TestResume_CrashRecoveryRecallsUnconditionally`, `TestResume_PauseStopsAtBoundaryAndClearsFlag`, `TestBounceRouting_StuckContinuesAtDeclaredTarget`/`_EmptyTargetBlocksInstead`/`_BudgetExhaustionBlocks`, `TestCancellation_RealProducersReturnErrorNotStuck`), not one, plus `sequence_test.go`'s — and each builds `New(deps)` **twice**, so the substitution is eight tests / ten-plus call sites, not "both". **Fix:** restate the decision as "every moved test that calls `Run` substitutes row 1 after each `New`", and enumerate the seven resume tests in Testing rather than the single "Resume" bullet.

### [BLOCKING:design] Row-1 substitution has two distinct fakes, not one
**Section:** `row1-substitution` **Issue:** `TestResume_CrashRecoveryRecallsUnconditionally` (`resume_test.go:111-112`) overrides `deps.Preflight` with `countingProducer{}` — its whole subject is the row-1 **call count** — so a rule fixed at `shed.Producers[0].Producer = fakeAlwaysDoneProducer{}` erases what that test measures. **Fix:** state that the fixture substitutes a default always-done fake and that an individual test may substitute its own (counting) fake instead, at the same seam.

### [NIT:scope] Moved fixture depends on helpers in staying files
**Demoted-from:** BLOCKING
**Section:** `test-ownership`, Testing **Issue:** `buildSequenceFixture` calls `writeDiscussionFixture`+`validDecisionRecord` (`discussionvalidate_test.go`), `seedPlanValidateFixture` (`planvalidate_test.go`), `writeBatcherConfig` (`batchifier_test.go`) and `(&fakeWebsterRun{}).run` (`webster_test.go`) — four files the discussion explicitly keeps in `internal/loomshed` — so `internal/loomrecipe` will not compile from "`fixture_test.go` moves with them" alone, and moving those helpers would break the per-producer tests that stay. **Fix:** give an explicit disposition (duplicate the four helpers into `internal/loomrecipe`, or name a shared testdata/exported-helper route) in the `test-ownership` Decision.

### [NIT:consistency] Roadmap has a third falsified present-tense claim
**Demoted-from:** BLOCKING
**Section:** `docs` **Issue:** the Decision names exactly two roadmap corrections (lines 160, 168), but line 159 also asserts in the present tense that "`internal/shedrecipe/coverage_guard_test.go` pins the registry against `loomshed.New`'s current, real row list, both directions" — false once `New` is deleted and the guard's loom half moves. **Fix:** name line 159 as a third correction, or replace the fixed line list with a grep-based sweep instruction as the comment carve-out already does.

### [NIT:scope] `docs/overview.md` edit list misses the loom module line
**Section:** `docs` **Issue:** line 307 lists loom as "`internal/loomcli` + `internal/loomengine` + `internal/loomshed`", which the new package makes incomplete; the Decision names only the module tree and lines 322-324. **Fix:** add line 307 to the overview edit list.

### [NIT:consistency] "Five doc files" vs six
**Section:** Constraints → Documentation Lifecycle **Issue:** the bullet says "the five doc files in the `docs` Decision", while Scope and the `docs` Decision both name six. **Fix:** say six.

## Verdict

REQUEST_CHANGES
Test-move scope is under-enumerated; moved fixture cannot compile as described.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
