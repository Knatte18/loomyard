# Batch: batten-producers

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: batten-producers
number: 5
cards: 5
verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/shedrecipe/... ./internal/shedengine/...
depends-on: [3]
```

## Batch Scope

This batch rebuilds batten's producer layer: `Run-Shed` becomes re-entrant with an inverted read-before-spawn order and a hard-error escalation, a new `Seed-Child` producer and registry entry land between `Worktree-Create` and `Run-Shed`, the recipe file gains the fourth row plus the `on_stuck` self-route with its explicit `max_bounces`/`poll_interval_s` pair, and `shedengine.persist`'s doc comment is qualified for the caller-side skip batch 6 introduces.
It is one batch because the recipe file, the row-name constants, the registry entries and the two producers are a single contract: a row added to the yaml without its registry entry fails to build, and a registry entry without its row is unreachable.

The external interface batch 6 consumes: `battenshed.SeedChildDeps` and `battenshed.NewSeedChild`, the reshaped `battenshed.InnerRunDeps`, and `shedrecipe.Env.SeedChild`.

Batch-local decisions beyond the overview's: the two producers stay pure over injected closures, so every verdict in both disposition tables is drivable from a Tier 1 test with a fake clock and a no-op sleep — `deps.Now`/`deps.Sleep` nil-resolution stays in the constructor, which is what tests substitute.
Neither producer imports `internal/shedrun`, per the overview's `seed-encoding-stays-behind-a-seam-in-battenshed` Shared Decision.

## Cards

### Card 16: make Run-Shed re-entrant

- **Context:**
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
  - `internal/battenshed/stuck.go`
  - `internal/battenshed/ctx.go`
- **Edits:**
  - `internal/battenshed/innerrun.go`
  - `internal/battenshed/innerrun_test.go`
  - `internal/battenshed/deps.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `innerRunProducer.Call` so it reads the child's status **before** it does anything else, inverting today's spawn-then-poll order, and performs exactly one status check per `Call` rather than a bounded poll loop.
  Replace the `pollAttempts` field and constructor parameter with nothing — the budget now lives on the recipe row as `max_bounces` — and keep `pollInterval` as the single sleep the still-running arm performs.
  The full disposition table, evaluated top to bottom: child status absent (`found == false`) **and** `deps.Spawn` has not yet run this entry, spawn then re-read once; `deps.Spawn` returning an error, **hard error** rather than today's `Stuck`; child status still absent after a successful spawn, **hard error** naming the spawn that returned success without producing a status; child `done`, `Done`; child still `running`, `deps.Sleep(pollInterval)` then `Stuck`; child `blocked`, `paused` or `failed`, **hard error** whose message carries the child's `State`, `Error` and `CurrentProducer`; any unrecognised state, hard error as today.
  The read-before-spawn ordering is the re-entry-safety mechanism: the child's own status file is the durable record of whether the spawn already happened, so no separate marker is needed and a resumed run never double-spawns.
  The hard errors are the load-bearing half.
  `ProducerDef.OnStuck` is a static per-producer value and `Call` returns only `(Outcome, OutputPointer, error)`, so once `on_stuck` is non-empty **every** `Stuck` from this row routes back to it — there is no such thing as "Stuck with no self-route" for one case and a self-route for another.
  A blocked child returning `Stuck` would bounce with nothing to sleep on, since the sleep lives in the still-running arm alone, and would burn the whole budget in a tight loop before reaching `StateBlocked`.
  Rewrite `Call`'s doc comment's whole verdict table to match, and keep the existing `cancelErr` context checks and the two `Live-Substrate Spawn Observability` log lines around `deps.Spawn`.
  In `deps.go`, leave `InnerRunDeps`' five fields as they are but amend `Spawn`'s and `ReadStatus`' doc comments where they describe the old ordering.
  In `innerrun_test.go` cover every arm above.
  Two assertions carry more weight than the rest and must be present explicitly: that `Stuck` is returned for the still-running case **and no other**, which is what makes the static self-route safe; and that a second `Call` against a child whose status now exists does **not** call `deps.Spawn` again.
  Also cover exactly one `deps.Sleep` call on the still-running arm, and context cancellation surfacing as an error rather than as `Stuck`.
- **Commit:** `feat(battenshed): make Run-Shed re-entrant with read-before-spawn and hard-error escalation`

### Card 17: the Seed-Child producer

- **Context:**
  - `internal/battenshed/innerrun.go`
  - `internal/battenshed/stuck.go`
  - `internal/battenshed/ctx.go`
  - `internal/battenshed/create.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/battenshed/deps.go`
- **Creates:**
  - `internal/battenshed/seamchild.go`
  - `internal/battenshed/seamchild_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `SeedChildDeps` in `deps.go` alongside the existing three seam types, carrying five injected closures and no paths of its own: `ReadBoardType func(ctx context.Context) (string, error)` returning the Board task's `type` with the empty string meaning `loom`; `ChildDriver func() (string, error)` returning the driver read from prime's own seed params; `WriteSeed func(ctx context.Context, recipe, driver string) error`, which resolves the child's seed path and encodes it, and whose error means an unknown recipe name or a write failure; `CommitSeed func(ctx context.Context) error`; and `PushSeed func(ctx context.Context) error`.
  Create `NewSeedChild(name, slug string, deps SeedChildDeps, scratchDir string) shedengine.ShedProducer` in `seamchild.go`, modelled on `NewWorktreeCreate`'s shape.
  Its `Call` reads the Board `type` **fresh at `Call` time**, never from a value captured at wiring time, so a `type` corrected after prime was seeded is still honoured; an empty value resolves to `loom`.
  It then reads the driver, writes, commits and pushes, in that order.
  Verdicts: `Done` on a successful write-and-commit; `Stuck` on an unknown recipe name, an unreadable Board, or a failed commit, each naming which of the three failed in its stuck reason; a hard error on a path-resolution failure, matching `innerRunProducer`'s error-vs-verdict split; and — the arm most easily collapsed into the one beside it — a failed **push** warns via `internal/logger` and still returns `Done`, because an offline machine must not halt a run and the next push on that pair catches the branch up.
  In `seamchild_test.go` cover the happy path asserting the child's seed takes its `recipe` from the Board `type` and its `driver` from the injected `ChildDriver` — the two-source split is the thing most likely to be collapsed into one source; a Board `type` changed between prime's seeding and this `Call` being honoured; an empty `type` defaulting to `loom`; `Stuck` on each of unknown recipe, unreadable Board and failed commit; the failed push warning and still returning `Done`; and a hard error on a path-resolution failure.
- **Commit:** `feat(battenshed): add the Seed-Child producer`

### Card 18: the SeedChild registry entry and the retired poll_attempts key

- **Context:**
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/battenshed/seamchild.go`
  - `internal/battenshed/innerrun.go`
- **Edits:**
  - `internal/shedrecipe/entries_batten.go`
  - `internal/shedrecipe/entries_batten_test.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/registry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/shedrecipe/recipe.go`, add `SeedChild battenshed.SeedChildDeps` to `Env` as a whole-struct passthrough, following the `InnerRun` and `Teardown` fields' own precedent and for the same stated reason: it has behaviour of its own that per-seam fakes must substitute individually.
  In `entries_batten.go`, add `seedChildEntry`, validating `configRejectUnknown(cfg)` with no allowed keys, `requireNonEmpty("SeedChild", "Slug", env.Slug)`, `requireAbsRoot("SeedChild", "ScratchDir", env.ScratchDir)`, and `requireSeam` for each of the five `env.SeedChild` closures, then returning `battenshed.NewSeedChild(name, env.Slug, env.SeedChild, env.ScratchDir)`.
  Register it in `internal/shedrecipe/registry.go`'s single map literal under the key `"SeedChild"`, placed beside the three existing batten entries; add no `init()` self-registration and no runtime `Register`, per the Shed Recipe Registry Invariant.
  Update that file's doc comment, which reads "The table is complete at seventeen keys" — this entry makes it eighteen, and no test pins the literal count, so it drifts silently otherwise.
  In the same file's `innerRunEntry`, retire the `poll_attempts` config key completely: delete the `configInt` read, its negative-value guard, its `defaultInnerRunPollAttempts` constant, and its entry in the `configRejectUnknown` allowlist, which becomes `configRejectUnknown(cfg, "poll_interval_s")`.
  Change `defaultInnerRunPollIntervalS` from `5` to `30` so an omitted key and the explicit key the recipe now carries agree, and amend the constant block's doc comment, which today derives twelve hours from the attempt count at the old interval.
  Drop the `pollAttempts` argument from the `battenshed.NewInnerRun` call to match card 16's signature.
  In `entries_batten_test.go` add a constructor test for `seedChildEntry` covering `configRejectUnknown` and each `requireNonEmpty`/`requireAbsRoot`/`requireSeam` field, matching the existing entry tests' shape, and assert `innerRunEntry` now **rejects** `poll_attempts` as an unknown key rather than reading it.
- **Commit:** `feat(shedrecipe): register the SeedChild entry and retire the poll_attempts config key`

### Card 19: the four-row batten recipe with the self-route

- **Context:**
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedengine/run.go`
  - `internal/shedrecipe/registry.go`
- **Edits:**
  - `contracts/recipes/batten-recipe.yaml`
  - `internal/battenrecipe/names.go`
  - `internal/battenrecipe/recipe_test.go`
  - `internal/battenrecipe/coverage_guard_test.go`
  - `internal/battenrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `NameSeedChild = "Seed-Child"` to `internal/battenrecipe/names.go`'s constant block, documented like its three siblings as a durable on-disk identity resume depends on.
  In `contracts/recipes/batten-recipe.yaml`, insert a `Seed-Child` row between `Worktree-Create` and `Run-Shed`, with `engine: SeedChild` and `on_done: Run-Shed`, and repoint `Worktree-Create`'s `on_done` from `Run-Shed` to `Seed-Child`.
  Give the `Run-Shed` row `on_stuck: Run-Shed`, a row-level `max_bounces: 1440`, and a `config:` block carrying `poll_interval_s: 30`.
  These land on two different surfaces and must not be conflated: `max_bounces` is a `shedbuild.Row` field, while `poll_interval_s` is a config key the registry entry reads.
  The two values encode a **12-hour** watch window — 1440 bounces at 30 seconds — which is the same wall clock today's Go constants express, not the same attempt count; reusing the count would silently stretch the window to 72 hours.
  Rewrite the header comment's load-bearing paragraph, which is the single easiest thing in this task to get wrong.
  It currently claims `Loom-Run`'s **empty** `on_stuck` is what makes the destructive `Worktree-Teardown` row unreachable from a failure path.
  That routing is gone.
  The property is now carried by `Run-Shed`'s own hard-error return, which `shedengine` turns into `StateFailed` at `Run-Shed` with the run halted, so `Worktree-Teardown` is never routed to — say exactly that, and do not leave any wording implying the old mechanism still applies.
  In `recipe_test.go`, extend the coverage guard to the four-row shape, keep the explicit `"Run-Shed"` value assertion, and add a test pinning `Run-Shed`'s `on_stuck` self-route together with its `max_bounces: 1440` and `poll_interval_s: 30` pair, asserted against the 12-hour wall clock they encode rather than as bare numbers.
  A dropped `max_bounces` would silently inherit the engine's default budget of ten bounces and turn every long child run into a spurious `StateBlocked` — the highest-consequence, lowest-visibility failure in this task, which is why it gets its own assertion.
  Update `coverage_guard_test.go` for the new row and engine.
- **Commit:** `feat(battenrecipe): add the Seed-Child row and Run-Shed's self-route with its 12-hour budget`

### Card 20: qualify shedengine.persist's CommitStatus contract

- **Context:**
  - `internal/shedengine/shed.go`
  - `internal/battenrecipe/names.go`
- **Edits:**
  - `internal/shedengine/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Qualify `persist`'s doc comment in `internal/shedengine/run.go`, which today states the `CommitStatus` closure "is called on every persist invocation, never conditionally on `nextCurrentProducer` having changed: state, history, and error can all change without it".
  That sentence rules out exactly the conditional batch 6's batten seam introduces one layer out, and leaving it would make the next reader trust a contract the code no longer keeps.
  The qualification to write: the engine still calls the seam unconditionally, and a **caller's** seam may skip; batten's does, keyed on the `(producer, state)` pair alone and therefore deliberately blind to `history` and `error` changes.
  State why that blindness is safe: the only transition it ever skips is `Run-Shed`'s self-bounce, where `error` is empty by construction and the accumulated `history` is committed by the next real transition.
  Change no code in this card — `persist`'s behaviour is unchanged, and the Shed Producer-Seam Invariant bars adding any import to this package.
- **Commit:** `docs(shedengine): qualify persist's CommitStatus contract for a caller-side no-op skip`

## Batch Tests

`verify: go build ./... && go test ./internal/battenshed/... ./internal/battenrecipe/... ./internal/shedrecipe/... ./internal/shedengine/...` covers the four package trees this batch changes, plus a whole-module build that catches any consumer broken by `NewInnerRun`'s dropped `pollAttempts` parameter and `Env`'s new field.

`internal/battenshed`'s two producer test files carry the batch's real weight.
`innerrun_test.go` must assert the full disposition table, and in particular that `Stuck` is returned for the still-running case and no other — that single assertion is what proves the static self-route cannot loop a blocked child — and that a re-entered `Call` does not respawn.
`seamchild_test.go` must keep the two-source split visible (recipe from the Board's `type`, driver from prime's seed params) and must keep the failed-commit and failed-push arms distinct, since collapsing them is the likeliest regression.
Both files fake `deps.Now` and `deps.Sleep` throughout, so the Test Tier Purity Invariant's ban on a `time.Sleep` of a second or more in an untagged file is satisfied by construction.

`internal/battenrecipe/recipe_test.go` is where the recipe's own numbers are pinned; its new `max_bounces`/`poll_interval_s` assertion is the guard against the silent-default failure card 19 describes.
`internal/shedrecipe`'s registry and entry tests prove the new `"SeedChild"` key is reachable through `Lookup`/`Names` and that `poll_attempts` is now rejected rather than silently ignored.
`internal/shedengine/...` runs despite card 20 changing only a comment: the package's own `run_commitstatus_test.go` pins the unconditional-call behaviour the qualified comment still describes, and running it proves the comment and the code have not drifted apart in the other direction.
