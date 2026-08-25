# Batch: bouncer-commit-seam

```yaml
task: 'loom: Plan-Review producer'
batch: 'bouncer-commit-seam'
number: 2
cards: 5
verify: go test ./internal/shedadapters/... ./internal/shedrecipe/...
depends-on: []
```

## Batch Scope

This batch delivers the one new generic mechanism the task needs: an optional commit closure on `shedadapters.BouncerConfig`, called on the approved branch of `settle`, plus the `commit_seam` recipe key that selects which of the two closures `shedrecipe.Env` already carries fills it.
It is one batch because the two halves are a single seam: the `shedadapters` field with no `shedrecipe` key is unreachable from a recipe, and the `shedrecipe` key with no field does not compile.
Both packages are small, adjacent, and already imported one way (`shedrecipe` imports `shedadapters`).

It is deliberately scoped **below** loom: nothing here names `Plan-Bouncer`, `Plan-Burler`, or the plan artifact directory.
Batch 3 is what wires the mechanism into loom's own recipe.

The external interface batch 3 consumes is the recipe key `commit_seam` with its two accepted values, `plan` and `discussion`.

Batch-local decision: `NewBouncer` gains **no** validation for the new field.
A nil `Commit` is the absent value and legally means "commit nothing" — that is exactly what keeps the shipped `Discussion-Bouncer` row byte-identical in behaviour.
The presence check lives one layer up, in `bouncerEntry`, where "no seam configured" and "seam configured but missing" are distinguishable.

## Cards

### Card 4: Add the optional Commit closure to the Bouncer and call it on approval

- **Context:**
  - `internal/shedengine/producer.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/shedadapters/bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a field `Commit func() error` to `BouncerConfig`, placed after `Shuttle` and before `Now`.
  Its doc comment states: it is the injected closure the loop owner commits the reviewed artifacts through, called on the approved branch of `settle` before `Done` is returned;
  nil is the absent value and means "commit nothing", which is what keeps a segment with no seam configured behaving exactly as before.

  In `settle`, change the `case verdictApproved:` branch from a bare `return shedengine.Done, ptr, nil` to: call `b.cfg.Commit()` when it is non-nil, and on a non-nil error return the zero `shedengine.Outcome`, the zero `shedengine.OutputPointer`, and that error wrapped with the producer's own name and `bouncerEngineLabel`, in the same `"shedadapters: %s (%s): ...: %w"` shape `Call`'s resolve-round error already uses.
  Only when `Commit` is absent or returns nil does the branch return `shedengine.Done` with `ptr`.

  The commit must **not** be routed through `degrade`, and this is the single most important behavioural detail in the card.
  `degrade` returns `shedengine.Stuck`, and its own doc states none of its callers ever return `shedengine.Done`;
  sending a commit failure through it would take a plan the judge approved and hand it to the segment's round producer for a fixer round with no BLOCKING findings to fix, without the `ensureFocus(round + 1)` call the real `verdictBlocking` branch makes — and it would not converge, because on re-entry `judged(n)` is still true, so the same verdict re-approves and re-attempts the commit every bounce until the budget is spent.

  The commit must also **not** be gated on `cancelErr`.
  `settle`'s existing contract is that a genuinely parsed verdict is the one exception `cancelErr` never applies to;
  that rule says a parsed verdict is never retracted because the context was cancelled, not that the branch performs no side effects.
  Leaving approved work uncommitted because an operator pressed Ctrl-C is precisely the dirt this seam exists to prevent, and the injected closure is idempotent in production, so a redundant attempt costs nothing.

  Update `settle`'s own doc comment to describe the new approved-branch behaviour and to state, in one sentence each, why the failure is an error rather than a `degrade`, and why the blocking branch deliberately commits nothing (an unapproved artifact must not be committed, and a blocked run has already escalated to a human who is the right party to judge the partial fixes).
  A later reader must not be able to mistake the missing blocking-branch commit for a gap.

  Add nothing to `NewBouncer`: the new field is optional, and a nil value is legal.
  Change no other function in this file.
- **Commit:** `feat(shedadapters): add an optional Commit seam to Bouncer's approved branch`

### Card 5: Split the test Bouncer config out of its constructor helper

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncer_config_test.go`
- **Edits:**
  - `internal/shedadapters/bouncer_seed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extract the `BouncerConfig` literal inside `newTestBouncer` into a new package-level helper `testBouncerConfig(t *testing.T) BouncerConfig` that takes no shuttle, builds the same fresh run dir and the same shipped-stencils fixture, and returns the config with every field except `Shuttle` filled exactly as it is filled today.
  Rewrite `newTestBouncer` to call it, set `cfg.Shuttle = shuttle`, call `NewBouncer(cfg)`, and return the same `(*Bouncer, BouncerConfig)` pair it returns today.

  This is a pure refactor: no existing test's observable fixture may change, and the returned config's field values must stay identical, including the fixed clock and the `round-%d-report.md` report name.
  Give `testBouncerConfig` a doc comment saying it exists so a test needing a non-default config field — the commit seam is the first — can build one without duplicating the fixture.

  Do not change any `Test*` function in this file.
- **Commit:** `refactor(shedadapters): extract testBouncerConfig from newTestBouncer`

### Card 6: Pin the Commit seam's five behaviours

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncer_seed_test.go`
  - `internal/shedadapters/bouncer_replay_test.go`
  - `internal/shedadapters/bouncer_judge_test.go`
  - `internal/shedadapters/singlellm_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/bouncer_commit_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write a new test file whose header comment states it covers `BouncerConfig.Commit` alone — the seam that lets a segment whose round producer runs no git of its own still have its approved artifacts committed by the loop owner.

  Build each case on the replay path, following `TestBouncer_Replay_Approved` as the precedent: `testBouncerConfig(t)`, set `cfg.Shuttle` to a `&fakeShuttle{}` and `cfg.Commit` to the case's closure, `NewBouncer(cfg)`, then `layoutBouncerRun(t, cfg, ...)` with one `bouncerJudgeFixture` carrying round 1's report, verdict, and ledger, then one `Call`.
  The replay path is the right vehicle because it reaches `settle` with no shuttle spawn at all, so nothing but the seam is under test.

  Five cases, each its own `Test*` function or subtest:

  1. **Approved calls Commit exactly once, before Done.** An `APPROVED` verdict fixture, a closure incrementing a counter and returning nil.
     Assert the counter is 1, the outcome is `shedengine.Done`, and the pointer is the round's ledger path.
  2. **Blocking never calls Commit.** A `BLOCKING` verdict fixture, the same counting closure.
     Assert the counter is 0 and the outcome is `shedengine.Stuck` — an unapproved artifact must not be committed.
  3. **A nil Commit is not an error and commits nothing.** Leave `cfg.Commit` nil over an `APPROVED` fixture.
     Assert `Call` returns `shedengine.Done`, the ledger pointer, and a nil error.
     Say in the test's own doc comment that this case is what pins the shipped `Discussion-Bouncer` row's behaviour as unchanged.
  4. **A failing Commit makes settle return that error.** A closure returning a sentinel error built with `errors.New`.
     Assert the returned outcome is the zero `shedengine.Outcome` and the pointer is the zero `shedengine.OutputPointer`, that the returned error is non-nil and satisfies `errors.Is` against the sentinel, and — explicitly — that the outcome is **not** `shedengine.Stuck`, so a regression that reroutes the failure through `degrade` fails here rather than being caught by a reader.
     State the consequence of that regression in the test's doc comment: an approved artifact bounced into a findings-free fixer round, re-approving and re-committing every bounce until the budget is spent.
  5. **A cancelled context still commits, and the commit's result still governs.** An `APPROVED` fixture and a context already cancelled via `context.WithCancel` plus an immediate `cancel()`.
     Assert the counting closure ran, and assert the return still follows the commit's own result rather than a cancellation error.

  Do not add a `Commit`-related assertion to any existing test file — this file owns the seam's coverage.
- **Commit:** `test(shedadapters): pin Bouncer's Commit seam across approval, blocking, nil, error, and cancellation`

### Card 7: Add the commit_seam recipe key to the Bouncer entry

- **Context:**
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_planwrite.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:**
  - `internal/shedrecipe/entries_bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extract an optional `commit_seam` string from `cfg` via `configString(cfg, "commit_seam", false)`, and add `"commit_seam"` to `configRejectUnknown`'s recognised set.
  Adding the key to that set is part of this change, not an afterthought: omitting it makes every recipe using the key fail to build, per the Config Strictness Invariant.

  Resolve the extracted value to a `func() error` before building `shedadapters.BouncerConfig`, with exactly three accepted states:

  - absent (the empty string) — leave the resolved closure nil, and check nothing.
    "No seam configured" is a legitimate configuration and is never an error, which is what keeps every existing `Bouncer` row valid unchanged.
  - `plan` — guard with `requireSeam("Bouncer", "CommitPlan", env.CommitPlan)`, then use `env.CommitPlan`.
  - `discussion` — guard with `requireSeam("Bouncer", "CommitDiscussion", env.CommitDiscussion)`, then use `env.CommitDiscussion`.

  Any other value is a construction error naming the key and the offending value, and listing the two accepted values.

  The `requireSeam` guard on a **present** key is load-bearing rather than defensive: without it a nil `Env` closure would assign a nil `Commit`, which this design defines as "commit nothing", silently reproducing the exact no-seam condition the key exists to eliminate, with no error anywhere.
  `requireSeam` catches a typed nil as well as an untyped one via its `reflect.Func` case, and `planWriteEntry` already guards `CommitPlan` the same way — this is the established handling for this class, not a new pattern.

  Assign the resolved closure to the new `Commit` field of the `shedadapters.BouncerConfig` literal.

  Update `bouncerEntry`'s own doc comment to name the new key alongside the ones it already describes, and update the file header comment if it enumerates the entry's responsibilities.
  Change no other entry in this package, and add no key to the `BurlerRound` entry.
- **Commit:** `feat(shedrecipe): resolve the Bouncer entry's new commit_seam key`

### Card 8: Pin commit_seam's resolution, its guard, and its rejection surface

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/entries_planwrite_test.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:**
  - `internal/shedrecipe/entries_bouncer_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `TestBouncerEntry_CommitSeam` covering six cases as subtests.
  `shedadapters.BouncerConfig`'s own field is unexported from this package's point of view, so resolution is asserted through behaviour, exactly as `TestBouncerEntry_EnvReviewFallback` already asserts the model triple through a recorded spec.

  The behavioural vehicle is the replay path: after building the producer, write `round-1-review.md`, the round-1 verdict file, and the round-1 ledger file directly into the entry's run directory under `env.RunRoot`, so the first `Call` settles an already-judged `APPROVED` round without spawning anything.
  The verdict file's body is `---`-delimited frontmatter carrying `verdict: APPROVED` and a double-quoted single-line `rationale`;
  the ledger file's body is `---`-delimited frontmatter carrying a positive integer `round` and an empty `ledger` list.
  Read `internal/shedadapters/bouncer.go`'s own parser expectations before writing them, and keep the two file names in step with the pinned `round-<n>-review.md` report name the entry already sets.

  The six cases:

  1. `commit_seam: plan` resolves to `env.CommitPlan` — fill that field with a counting closure and assert it ran exactly once after the settling `Call`.
  2. `commit_seam: discussion` resolves to `env.CommitDiscussion` — same shape, and assert the `CommitPlan` counter stayed at zero, so the two are not silently interchangeable.
  3. An absent `commit_seam` leaves the seam nil — both counters stay at zero after the same settling `Call`, and the `Call` still succeeds.
  4. An unrecognised value is a construction error naming the key.
  5. A present `commit_seam: plan` with `env.CommitPlan` nil is a construction error naming `CommitPlan`;
     a present `commit_seam: discussion` with `env.CommitDiscussion` nil is a construction error naming `CommitDiscussion`.
     Cover both.
  6. An absent `commit_seam` with **both** closures nil constructs successfully — the passing case that proves the guard is on the key's presence, not on the `Env` field.

  Separately, assert the key is accepted by `configRejectUnknown` rather than rejected as unknown: a config carrying `commit_seam` must not produce an unrecognised-key error.
  Case 1 already demonstrates this implicitly, but state it in that subtest's own comment so a later reader knows the recognised-set edit is covered.

  Leave every existing test in this file unchanged.
- **Commit:** `test(shedrecipe): pin commit_seam resolution, its presence guard, and its rejection surface`

## Batch Tests

`verify: go test ./internal/shedadapters/... ./internal/shedrecipe/...` runs both packages the batch touches, and nothing else.
The scope is two packages rather than two files because the change spans both and because each package's existing suite is the regression net that matters here: `internal/shedadapters`' seven bouncer test files must all still pass after card 5's fixture refactor, and `internal/shedrecipe`'s `registry_test.go`, `seam_enforcement_test.go`, and every other entry's tests must all still pass after card 7 adds a recognised key.
Neither package's tests spawn a process or resolve a working directory, so both stay fast.

New coverage added by the batch:

- `internal/shedadapters/bouncer_commit_test.go` — the five seam behaviours, of which the failing-`Commit`-is-an-error case is the highest value: a regression there is silent and its consequence is a non-converging bounce loop.
- `internal/shedrecipe/entries_bouncer_test.go`'s `TestBouncerEntry_CommitSeam` — the two-value resolution, the presence guard, and the recognised-set edit.

`TestRegistry_ShipsFourteenEntries` is deliberately untouched: this batch adds a config key, never a registry entry.
