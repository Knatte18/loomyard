# Batch: run-routing-and-budget

```yaml
task: 'shedengine: per-producer bounce budget + explicit OnDone routing'
batch: 'run-routing-and-budget'
number: 2
cards: 7
verify: go test ./internal/shedengine/...
depends-on: [1]
```

## Batch Scope

This batch is the behavior change itself: `Run`'s `Done` arm stops routing by list position and routes by `OnDone`, and its `Stuck` arm stops decrementing a run-wide in-memory counter and starts deriving each producer's own episode `Stuck` count from the persisted history.
It is one batch because the two changes share the same `switch` and the same test files — every scenario in the package that relied on sequential advance has to be re-wired in the same commit that removes sequential advance, or the package does not compile a green suite at all.
The external interface batch 3 consumes is the routing contract: after this batch, a producer list with no `OnDone` anywhere finishes at its first `Done`, which is exactly why `loomshed`'s 12 rows must be migrated next.

Batch-local decision: `internal/loomshed` is knowingly left red by this batch.
Its 12-row list has no `OnDone` values yet, so a real loom run would finish at `Preflight`.
That is why this batch's `verify:` is scoped to `internal/shedengine` alone and batch 3 immediately follows; the repo-wide gate is `pipeline.done_gate`, which runs after every batch has landed.

Batch-local decision on the two existing budget tests: both are re-wired to a **single self-bouncing producer** rather than having their expected totals renumbered.
`TestRun_BounceBudgetExhaustion` and `TestRun_MaxBouncesZeroResolvesToDefault` each wire two producers in an A↔B cycle today and assert the *sum* of their call counts.
Under a per-producer budget that sum becomes `2×budget + 1`, which is the aggregate the design deliberately accepts — not the boundary these two tests exist to guard — and it is exactly the number that silently drifts when a third producer joins the cycle.
A single producer with `OnStuck` naming itself keeps each assertion pinning the boundary directly.

## Cards

### Card 5: Route `Done` through `OnDone` and delete the positional machinery

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/status.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedengine/run.go`, rewrite the `case outcome == Done` arm of `Run`'s routing switch so it branches on `def.OnDone` alone and never on list position.
  An empty `def.OnDone` is the terminal path: persist `def.Name` as the next `current_producer` with `StateDone`, and return a `Result` carrying `RunDone` and `def.Name` as `HaltedProducer`, exactly as the current physically-last branch already does.
  Keep the existing comment explaining why `current_producer` keeps the just-finished producer's own name rather than the empty string — `activity.now` and `Result.HaltedProducer` are both defined in terms of it — and extend it to say the terminal is now chosen by an empty `OnDone`, not by list position.
  A non-empty `def.OnDone` persists it as the next `current_producer` with `StateRunning` and continues the loop; no lookup is needed, because `validate` has already rejected an `OnDone` naming no producer in the list.
  Delete the `indexAfter` function at the bottom of the file outright, along with its doc comment, and delete the `def.Name == s.Producers[len(s.Producers)-1].Name` check it served.
  Narrow `findProducer` to return `(ProducerDef, bool)`: its `int` return is already discarded at its only call site inside `Run` and becomes wholly unused once `Done` routes by name.
  Update that call site's assignment to take two values, and update `findProducer`'s own doc comment and its `return ProducerDef{}, 0, false` line accordingly.
  Add a sentence to `Run`'s own doc comment, or to the `Done` arm's comment, stating that the producer list now carries zero routing meaning — it is storage plus `validate`'s iteration order plus cosmetic display order.
  Do not add any import to this file.
- **Commit:** `feat(shedengine): route Done through OnDone and delete positional routing`

### Card 6: Replace the run-wide counter with an episode-scoped per-producer count

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/validate.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedengine/run.go`, delete the `bouncesRemaining` local and its `0 → defaultMaxBounces` initialisation from the top of `Run`, together with the comment above it explaining the per-`Run`-call, in-memory design.
  Add two unexported package-level helpers below `Run`, placed where `indexAfter` used to sit.
  The first, `episodeStuckCount(history []HistoryEntry, name string) int`, walks `history` backward from the end, returns immediately at the first entry whose `Producer` equals `name` and whose `Outcome` is `Done`, and otherwise counts the entries whose `Producer` equals `name` and whose `Outcome` is `Stuck`.
  Entries authored by other producers are skipped and never terminate the scan — a `Done` by some other producer does not end this producer's episode.
  Its doc comment must state that a `done` entry written by the hard-failure arm also terminates the scan, and that this is accepted rather than special-cased: the engine records the verdict a producer actually returned, and `state: "failed"` halts the run, so every continuation past it is a fresh human-initiated act.
  The second, `effectiveMaxBounces(def ProducerDef, shedMax int) int`, returns `def.MaxBounces` when it is greater than zero, else `shedMax` when that is greater than zero, else `defaultMaxBounces`.
  Rewrite the `case outcome == Stuck` arm's inner switch to use them: keep the `def.OnStuck == ""` case exactly as it is, first and unchanged, then replace the `bouncesRemaining <= 0` case with a check that `episodeStuckCount(st.History, def.Name)` is at or above `effectiveMaxBounces(def, s.MaxBounces)`, and drop the `bouncesRemaining--` from the default case so it only persists `def.OnStuck` and continues.
  The count argument must be `st.History`, the slice read at step 1, and never `nextHistory` — a post-append read shifts the boundary by one and would look like an off-by-one bug rather than a semantic change.
  Add a comment at that call site saying so in those terms.
  Keep the existing boundary comment's arithmetic, restated per-producer: a budget of three performs three bounce-backs and blocks on the fourth `Stuck`.
  Both `blocked` reason strings keep their exact current text and are still written identically to the persisted error field and `Result.Reason`.
  Do not add any import to this file.
- **Commit:** `feat(shedengine): derive the bounce budget per-producer from history episodes`

### Card 7: Add a linear-chain builder to the shared test support

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/testsupport_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one helper to `internal/shedengine/testsupport_test.go` so the mechanical `OnDone` re-wiring across three test files stays uniform rather than hand-written per scenario.
  Name it `linearChain` and give it the signature `linearChain(names []string, producers []ShedProducer) []ProducerDef`.
  It returns one `ProducerDef` per name, in order, with `OnDone` set to the following name and the last entry's `OnDone` left empty, so the built list reproduces today's sequential advance exactly.
  It fails loud rather than guessing when the two slices differ in length — take a `*testing.T` as its first parameter, call `t.Helper()`, and `t.Fatalf` on a mismatch, matching the style every other helper in this file uses.
  Its doc comment must say that the empty `OnDone` on the last entry is what finishes the run, and that the helper exists so a scenario that only needs "these producers, in this order" does not restate the chain by hand.
  Do not redeclare `funcProducer`, `fixedOutcomeProducer`, `newTestShed`, `seedStatus`, `readStatus`, `commonSeed`, `assertRFC3339UTC`, or `assertHistoryNonDecreasing`.
  Do not add any import beyond what the file already has.
- **Commit:** `test(shedengine): add a linearChain builder to the shared test support`

### Card 8: Re-wire the existing routing tests onto explicit `OnDone` chains

- **Context:**
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/run_routing_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-wire every scenario in `internal/shedengine/run_routing_test.go` that relies on sequential advance so it declares its routing explicitly, keeping each test's meaning identical.
  `TestRun_HappyPath`, `TestRun_CompletionTerminalValues`, `TestRun_ProducerError`, and `TestRun_UnrecognisedOutcome` each build a multi-row list that today advances by position: give each row an explicit `OnDone` naming the next, with the final row's `OnDone` empty, using the `linearChain` helper where the scenario needs nothing but the chain.
  `TestRun_UnconditionalRecall` and `TestRun_StuckWithNoTarget` are single-row lists whose producer must finish the run, so they are already correct under an empty `OnDone` — leave their wiring alone.
  `TestRun_StuckWithOnStuckTarget` wires A and B where B bounces to A: A needs `OnDone` naming B so control still reaches B, and B's `OnDone` stays empty so its second call finishes the run.
  Re-wire `TestRun_BounceBudgetExhaustion` to a single producer named A with `OnStuck` naming itself, keeping `shed.MaxBounces = 3`, and assert `a.calls` equals `shed.MaxBounces + 1` directly instead of summing two producers' counts.
  Re-wire `TestRun_MaxBouncesZeroResolvesToDefault` the same way, asserting `a.calls` equals `defaultMaxBounces + 1`.
  Rewrite both tests' explanatory comments: the boundary they pin is now one producer's own episode budget, not a run-wide total, and the comment must say that the two-producer aggregate of `2×budget + 1` is a deliberate design consequence rather than the property under test here.
  Update the file's own top-of-file comment, which describes the bounce budget as card 12's run-wide mechanism.
  Do not change any assertion's meaning while re-wiring — a scenario whose expected outcome shifts is a plan defect, not a test to adjust.
- **Commit:** `test(shedengine): re-wire routing tests onto explicit OnDone chains`

### Card 9: New tests for `OnDone` routing

- **Context:**
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/status.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/run_routing_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three named tests to `internal/shedengine/run_routing_test.go` covering the routing freedom `OnDone` introduces.
  `TestRun_EmptyOnDoneFinishesFromNonLastPosition`: a three-row list whose *first* row has an empty `OnDone` finishes the whole run on that row's `Done` — assert `RunDone`, a persisted `StateDone`, `current_producer` and `Result.HaltedProducer` both keeping that first row's own name, and both later producers' call counts at zero.
  `TestRun_OnDoneSkipsForward`: a three-row list whose first row's `OnDone` names the *third* row skips the middle row entirely — assert the middle producer's call count is zero and the persisted history holds exactly the two rows that ran, in order.
  `TestRun_OnDoneRoutesBackward`: a list where a later row's `OnDone` names an *earlier* row and the run continues from there — use a counter-driven `funcProducer` so the backward target changes its own `OnDone`-reached behavior on its second call and the run terminates, and assert the history records the backward re-entry.
  Every new test uses `newTestShed`, `commonSeed`, `seedStatus`, and `readStatus` from the shared support file rather than building its own fixture, and asserts timestamps structurally via `assertRFC3339UTC` rather than by literal.
- **Commit:** `test(shedengine): cover forward, backward, and terminal OnDone routing`

### Card 10: New tests for the per-producer episode budget

- **Context:**
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/run_routing_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add named tests to `internal/shedengine/run_routing_test.go` for each property the per-producer episode budget must have.
  Independence: two producers each bouncing under their own budget, where the first exhausting its budget does not reduce the second's remaining count — the property the whole task exists for, and the one a run-wide counter cannot express.
  Inheritance, two-level: a `ProducerDef` whose `MaxBounces` is zero inherits `Shed.MaxBounces`, and a `Shed.MaxBounces` of zero inherits `defaultMaxBounces`; assert both levels, including that a non-zero `ProducerDef.MaxBounces` overrides a different non-zero `Shed.MaxBounces`.
  History derivation across invocations: seed a status file whose history already holds N `Stuck` entries for the producer about to run, then run, and assert the budget accounts for those pre-existing entries — this is the direct test of the persisted-count decision and the one that fails under a per-`Run`-call count.
  Episode reset: a producer that bounces, later returns `Done`, and is then re-entered and bounces again starts from zero on re-entry, so `Stuck` entries preceding its own last `Done` do not count.
  Give this one its own named test rather than a table row: it is the loom `Discussion-Validate` shape that decided episode scoping over all-time counting.
  No spurious reset: a `Done` entry authored by a *different* producer does not end this producer's episode.
  Never-passing gate: a producer with no `Done` entry anywhere in history accumulates all-time, which is the anti-crash-loop property episode scoping must not weaken.
  Failure-path terminator: a seeded history containing a `done` entry for a producer that had returned an error alongside that verdict still ends that producer's episode, pinning the accepted behavior so a future reader does not rewrite the scan to ignore it.
  Attribution: `Stuck` entries authored by a different producer do not consume this producer's budget, even when that other producer's `OnStuck` targets it.
  Block-path arithmetic: after a budget-exhausted block the producer's episode `Stuck` count is `budget + 1`, a resumed run whose budget was raised by exactly one blocks again immediately, and a resumed run whose budget was raised above the current count proceeds — this pins the escape-hatch arithmetic and is the test an operator's bug report would otherwise write.
  Unchanged behavior to re-assert: a `Stuck` with an empty `OnStuck` still blocks with the no-target reason ahead of any budget check, which `TestRun_StuckWithNoTarget` already covers — extend that test only if the ordering is not already pinned by it.
  Seed pre-existing history through `seedStatus` with a `Status` built from `commonSeed` and its `History` field filled, never by hand-writing JSON.
- **Commit:** `test(shedengine): cover episode scoping, inheritance, attribution, and block-path arithmetic`

### Card 11: Re-wire the pause and persist suites, and sweep for missed literals

- **Context:**
  - `internal/shedengine/testsupport_test.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run_routing_test.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedengine/run_pause_test.go`
  - `internal/shedengine/run_persist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give every multi-row producer list in `internal/shedengine/run_pause_test.go` and `internal/shedengine/run_persist_test.go` an explicit `OnDone` chain reproducing today's sequential advance, using `linearChain` where the scenario needs nothing but the chain.
  Single-row lists are already correct under an empty `OnDone` and must be left alone.
  No scenario in either file changes meaning: these are mechanical re-wirings of lists that previously advanced by position.
  Then run the completeness sweep the migration depends on: grep the whole repository for every occurrence of a `ProducerDef` slice literal and for every assignment to a `Producers` field, and confirm each one either carries an explicit `OnDone` chain or is a deliberate single-row terminal.
  A green package suite is necessary but not sufficient evidence here — a scenario asserting only a done outcome passes unchanged on a run that silently ended early at a row with a forgotten `OnDone`, so the grep is the authority, not the test run.
  The sweep covers the whole repository, but the only production list outside this package is `internal/loomshed`'s own, which batch 3 migrates; record any other hit in the commit message rather than fixing it here.
- **Commit:** `test(shedengine): re-wire the pause and persist suites onto explicit OnDone chains`

## Batch Tests

`verify: go test ./internal/shedengine/...` runs the whole `shedengine` package suite.
That is the right scope: every file this batch edits is in that package, and the suite spans exactly the four test files the routing change touches — `run_routing_test.go`, `run_pause_test.go`, `run_persist_test.go`, and the shared `testsupport_test.go` helpers all three consume — plus `seam_enforcement_test.go`, which re-checks that neither the episode scan nor the new helpers pulled in an import outside the Shed Producer-Seam allowlist.
`validate_test.go` from batch 1 also re-runs here, which is deliberate: card 5 relies on `validate` having already rejected an `OnDone` naming no producer, so a regression there would surface as a routing bug rather than a validation one.
The suite is deliberately **not** widened to the whole repository at this batch boundary: `internal/loomshed` is knowingly red until batch 3 migrates its 12 rows, and the repo-wide check is `pipeline.done_gate`.
