# Batch: loomshed-migration

```yaml
task: 'shedengine: per-producer bounce budget + explicit OnDone routing'
batch: 'loomshed-migration'
number: 3
cards: 3
verify: go test ./internal/loomshed/...
depends-on: [2]
```

## Batch Scope

This batch migrates loom's own 12-row producer list onto explicit `OnDone` routing and brings that package's tests back to green after batch 2 deliberately left it red.
It is one batch because the row literal, the doc comment enumerating those rows, and the tests asserting the assembled table are one artifact seen from three angles — split across batches, either half would be red on its own.
The migration is behavior-preserving with respect to **routing** only: every row keeps the same successor it has today.
Loom's runtime bounce behavior does change in this same task, because `deps.MaxBounces` is now a per-producer, episode-scoped, cross-invocation budget with no run-wide cap — a loom run that blocks today may not block, and a resumed run may block sooner.

Batch-local decision: no row gains a non-empty `Segment` and no row gains a non-zero `MaxBounces`.
Every row keeps `Segment: ""` and inherits its budget, so all twelve pass the same-`Segment` `OnStuck` rule as one implicit standalone group.
Named segments arrive with the review-producer tasks, and forcing loom's existing rows into them now would turn a mechanical migration into a design pass over loom's producer table.

## Cards

### Card 12: Give all 12 rows an explicit `OnDone` and update the doc comments

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/run.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/loomshed/loomshed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/loomshed.go`, add an explicit `OnDone` to each of the twelve entries in the `producers` slice literal inside `New`, naming the next row in today's table order: `NamePreflight` to `NameDiscussionWrite` to `NameDiscussionValidate` to `NameDiscussionReview` to `NamePlanWrite` to `NamePlanValidate` to `NamePlanReview` to `NameBatchifier` to `NameWebster` to `NameWebsterReview` to `NamePublish` to `NameFinalize`, with `NameFinalize`'s own `OnDone` left as the empty string.
  Every `OnDone` value is one of the existing `Name*` constants, never a repeated string literal, per this file's own stated rule that the name is the durable on-disk identity.
  Leave every row's existing `OnStuck` exactly as it is, and do not set `Segment` or `MaxBounces` on any row — both stay at their zero values.
  Update `New`'s doc comment: the enumeration currently reads as twelve rows "with their backing and OnStuck target" and must now carry each row's `OnDone` target as well, with `Finalize`'s stated explicitly as empty and explained as what finishes the run.
  Add a sentence to that comment noting that the list's physical order no longer carries routing meaning — it is preserved as the display and enumeration order, and `OnDone` is what actually routes.
  Update the `MaxBounces` field doc on `Deps`, which currently calls it "Shed's own told bounce budget" with `0` meaning "use the internal default": it is now the default an unset per-producer `MaxBounces` inherits, and the budget it seeds is per-producer and episode-scoped rather than run-wide.
  Do not rename `Deps.MaxBounces` and do not add any import to this file.
- **Commit:** `feat(loomshed): route all 12 rows through explicit OnDone`

### Card 13: Assert the `OnDone` chain exhaustively and re-read the existing validation test

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/run.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/loomshed/loomshed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/loomshed_test.go`, extend `wantProducerRow` with an `onDone` field and fill it for all twelve entries of `wantProducerTable`, with the `NameFinalize` row's value the empty string.
  Extend `TestNew_ProducerTable`'s per-row loop to assert `got.OnDone` against `want.onDone` alongside the existing name and `OnStuck` assertions, so the whole routing table is pinned exhaustively rather than sampled.
  The exhaustive assertion is the named mitigation for this task's accepted silent-terminal risk: an omitted `OnDone` is indistinguishable from an intended terminal and would end a real run quietly at that row, so a test that pins every row's successor is what turns that silence into a failure.
  Also assert in the same loop that every row's `Segment` is the empty string and every row's `MaxBounces` is zero, so a future row that quietly acquires either has to be declared here first.
  Do **not** add a new test asserting that the `*shedengine.Shed` returned by `New` passes `shedengine`'s own validation — `TestNew_PassesShedValidation` already exists in this same file and already does exactly that, driving `Run` to exercise the unexported validation indirectly.
  A second test of the same property would either collide on the obvious name or silently displace the existing scenario's coverage.
  Instead, re-read that existing test against the new semantics and update its explanatory comment.
  Its assertion still holds unchanged and must not be weakened: exactly one producer bounces in that scenario, its guarded artifact never appears on disk so it never returns done, and the run still blocks once that producer's own budget is spent.
  What the comment must stop saying is that a single shared budget of three is exhausted between two producers — the budget belongs to the bouncing producer alone, and the producer it bounces to consumes none of it.
  Do not redeclare `fakeAlwaysDoneProducer`, `testDeps`, `testLandingDeps`, or `wantProducerTable`, and do not rename or delete any existing test in this file.
- **Commit:** `test(loomshed): pin the full OnDone chain across all 12 rows`

### Card 14: Re-read the bounce and sequence scenarios against per-producer semantics

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/sequence_test.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/loomshed/resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/resume_test.go`, re-read `TestBounceRouting_BudgetExhaustionBlocks` against the new semantics rather than assuming it passes unchanged.
  Its arithmetic is genuinely unchanged: exactly one producer bounces in that scenario, its decision record stays absent for the whole run so it never returns `Done` and its episode is the whole history, and the count of its own `Stuck` entries at the block is still one more than the budget.
  What must change is the explanation: rewrite the test's doc comment and the inline comment on the count assertion so both describe a per-producer, episode-scoped budget counted from the persisted history, naming the producer whose budget is being consumed rather than implying a run-wide counter.
  State in that comment why the number is one more than the budget — the `Stuck` entry written on the block path itself is appended before the inner switch decides, so it counts too.
  Confirm, without editing them, that the two scenarios in the other files stay correct: the shared fixture's budget of three is now a per-producer inherited default rather than a run-wide total, and the full-run sequence scenario reaches its terminal row through the new `OnDone` chain rather than by position, blocking on the same row for the same reason as before.
  If either turns out to need an edit, that is a plan defect — report it rather than silently widening this card.
- **Commit:** `test(loomshed): re-read the bounce scenario against per-producer episode semantics`

## Batch Tests

`verify: go test ./internal/loomshed/...` runs the whole `loomshed` package suite, which is the correct scope: every file this batch touches is in that package, and the suite is what turns batch 2's deliberately-red state green again.
It covers the three angles this batch changes at once — `loomshed_test.go`'s table assertions over the assembled rows, `resume_test.go`'s bounce and resume scenarios driving a real `Run` against the shared fixture, and `sequence_test.go`'s full-run walk through all eleven reachable rows, which is the end-to-end proof that the `OnDone` chain reproduces today's order rather than merely looking right in a literal.
It also re-runs `seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`; the migration adds only field values to an existing literal, so a trip there means something beyond the planned edit happened.
The untagged tier is what `verify:` runs; the integration-tagged tests in this package are covered by `pipeline.done_gate`, which runs both tiers repo-wide before the task is marked done.
