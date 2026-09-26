# Batch: loomcli-row-wiring

```yaml
task: Loom persists done only after post-run friction reflection
batch: loomcli-row-wiring
number: 2
cards: 2
verify: go test ./internal/loomcli/... ./internal/loomengine/... ./internal/loomrecipe/... ./internal/shedcli/... && go test -tags integration -skip TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver ./internal/loomcli/... && go test -tags smoke ./internal/loomcli/...
depends-on: [1]
```

## Batch Scope

This batch connects batch 1's `Friction-Reflect` row to loomcli's real reflection: the receiver records its arming verb, `wire` fills `shedrecipe.Env.ReflectFriction` with a row-path wrapper that reflects only when Tier 2 is on and the receiver was armed for `run`, the row path waits on the reflection lock instead of skipping, and `loomPostRun` stops reflecting on `RunDone` while keeping its blocked-path reflection and the envelope's `friction` key.
Every comment and doc that says reflection runs after the run lock is released on the done path is narrowed to the blocked path in the same commit.
Batch-local decision: the arming verb is stored on the receiver as `armedVerb` and read by the closure at call time, never the seed's recorded driver.

## Cards

### Card 5: route done-path reflection through the row

- **Context:**
  - `internal/lock/lock.go`
  - `internal/frictionengine/deps.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/loomshed/frictionreflect.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/run.go`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/run.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/friction_test.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/start.go`
  - `internal/loomengine/config.go`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/loomcli/cli.go`'s `loomCLI` struct, add two fields with doc comments:
    - `armedVerb string` — the verb `armAt` was called with; read by `reflectFrictionRow` at call time so the `Friction-Reflect` row reflects only under `run`, a fact of this invocation rather than the recorded seed driver, which the Driver Choice Single-Site Invariant bars any code path from gating behaviour on.
    - `rowFrictionStatus string` — the status the `Friction-Reflect` row's closure recorded in this process; empty when the row did not run here; `loomPostRun` reports it on `RunDone`.
  - In `internal/loomcli/arm.go`'s `armAt`, set `c.armedVerb = verb` as its first statement, and mention it in `armAt`'s doc comment.
  - In `internal/loomcli/run.go`:
    - `shouldReflectFriction` returns true only for a non-empty `frictionDir` and `shedengine.RunBlocked`; its doc comment says the done path reflects inside the terminal `Friction-Reflect` row instead.
    - `reflectFriction` gains a parameter: `func (c *loomCLI) reflectFriction(wait bool) string`.
      With `wait` true it takes `loomengine.LoomFrictionLock(c.location)` with the blocking `lock.AcquireWriteLock` (on error: `logger.Warn` and return `frictionengine.StatusFailed`); with `wait` false it keeps today's `lock.TryAcquireWriteLock` probe and skip.
      The `os.MkdirAll` of the lock's directory, the deferred release, the `frictionengine.Reflect` call and every failure-to-`StatusFailed` mapping are unchanged.
    - Rewrite `reflectFriction`'s doc comment: the log-don't-fail rule stays; the lock paragraph now says the lock guards against two reflections over one friction directory, which can overlap because the blocked-path call (from `loomPostRun`) runs after `shed.Run` has returned and released the run lock, so an operator can resume the task and reach a new driver's `Friction-Reflect` row while that earlier reflection still runs.
      The blocked path skips on a held lock (the other reflection already covers these notes).
      The row path waits, because its purpose is to hold `done` back until every reflection over these notes has finished; the wait is bounded by the holder's `friction_timeout_min`, an advisory lock is released on process death, and the holder never waits on anything the row holds, so it cannot deadlock.
      The wait is not cancellable, matching `frictionengine.Reflect` itself.
    - Add `func (c *loomCLI) reflectFrictionRow() string`: status is `frictionengine.StatusSkipped` unless `c.frictionDir != ""` and `c.armedVerb == "run"`, in which case it is `c.reflectFriction(true)`; it assigns the status to `c.rowFrictionStatus` and returns it.
      Its doc comment: it is the `shedrecipe.Env.ReflectFriction` closure `wire` installs; under `step` (`lyx loom step`, `lyx shed step --recipe loom`) it never reflects, so the notes stay in `.lyx/loom/friction/` for ly-drive's operator-gated filing, because the reflection agent files public issues itself.
    - Update the file header comment to name all three functions.
  - In `internal/loomcli/wiring.go`'s `c.env = shedrecipe.Env{...}` literal, add `ReflectFriction: c.reflectFrictionRow,` with a comment: a method value over the receiver, so `frictionDir` and `armedVerb` are read when the row runs, not when `wire` runs.
  - In `internal/loomcli/arm.go`'s `loomPostRun`, the friction status becomes: `c.rowFrictionStatus` when `result.Outcome == shedengine.RunDone` and it is non-empty; otherwise `c.reflectFriction(false)` when `shouldReflectFriction(c.frictionDir, result.Outcome)`; otherwise `frictionengine.StatusSkipped`.
    Rewrite its doc comment to match: `RunDone` never reflects here (the `Friction-Reflect` row did it under the run lock, before `done` persisted), `RunBlocked` still reflects here, and the `friction` key is still returned unconditionally on the success path.
    A `RunDone` whose row did not run in this process (the engine's done short-circuit on an already-done status file) reports `skipped`.
  - In `internal/loomcli/friction_test.go`, change every existing `c.reflectFriction()` call to `c.reflectFriction(false)`, change `TestShouldReflectFriction`'s `Done_DirSet` row's `want` to `false`, and update that test's doc comment and the file header comment to the new rule (only `RunBlocked` with a non-empty directory reflects in `loomPostRun`).
    Reword `TestReflectFriction_SkipsWhenAnotherDriverHoldsTheReflectionLock`'s doc comment to describe the blocked path's skip.
  - Narrow to the blocked path every comment that says reflection runs after the run lock is released:
    - `internal/loomcli/bootstrap.go`: `awaitRunLock`'s paragraph on `halted` ("`lyx loom run` then spends up to friction_timeout_min in the Tier 2 reflection step with the lock free") and `dispositionForHandshake`'s `awaitRunLockHalted` paragraph now say this happens after a blocked halt; the `awaitRunLockHalted` arm itself stays unchanged.
    - `internal/loomcli/start.go`: the `halted` closure's comment ("now that the run lock is released before the friction reflection runs") names the blocked halt's reflection.
    - `internal/loomengine/config.go`: rewrite `LoomFrictionLock`'s doc comment so it no longer claims the reflection always fires after `shedengine.Run` returns: on a finished run the reflection runs inside the terminal `Friction-Reflect` row with the run lock held; on a blocked halt it runs after `Run` returns with the run lock free, which is why a second reflection can overlap it and why this separate lock exists; the sibling-of-the-directory rationale stays.
    - `docs/overview.md`: the `internal/frictionengine/` line in the module tree becomes "the aggregation-and-reflection step loom's terminal Friction-Reflect row runs, and loom's run verb runs after a blocked halt" (keep it one line, aligned like its neighbours).
- **Commit:** `fix(loom): reflect friction in the terminal row, not after done persists`

### Card 6: test the row closure's gating, lock wait, and PostRun reporting

- **Context:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/arm.go`
  - `internal/loomcli/cli.go`
  - `internal/loomshed/frictionreflect.go`
  - `internal/loomshed/seed.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `internal/frictionengine/deps.go`
  - `internal/loomcli/bootstrap_test.go`
- **Edits:**
  - `internal/loomcli/friction_test.go`
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - All new tests are untagged Tier 1: no real agent, no git, no `time.Sleep` of a second or more.
    Receivers are hand-built `&loomCLI{...}` with `location: &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}` and `runDeps: websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: "stencils"}}`, as the existing tests in `friction_test.go` build them.
    A relative `frictionDir` (`"relative-friction-dir"`) makes `frictionengine.Reflect` fail Deps validation, so `frictionengine.StatusFailed` proves a reflection was attempted and `frictionengine.StatusSkipped` proves it was not.
  - In `internal/loomcli/friction_test.go`, add:
    - `TestReflectFrictionRow_SkipsWhenTierTwoOff`: `frictionDir: ""`, `armedVerb: "run"` → `StatusSkipped`, and `c.rowFrictionStatus` equals it.
    - `TestReflectFrictionRow_SkipsWhenArmedForStep`: `frictionDir` an absolute temp directory holding one note file, `armedVerb: "step"` → `StatusSkipped`, and the note file still exists afterwards.
    - `TestReflectFrictionRow_ReflectsWhenArmedForRun`: relative `frictionDir`, `armedVerb: "run"` → `StatusFailed`, and `c.rowFrictionStatus` equals it.
    - `TestReflectFrictionRow_WaitsOnAHeldReflectionLock`: receiver with relative `frictionDir`, `armedVerb: "run"`.
      Seed a status file with `loomshed.Seed` in a temp dir, then move it to `current_producer: Friction-Reflect`, `state: running` through `state.UpdateJSON[shedengine.Status]`.
      Build `&shedengine.Shed{Producers: []shedengine.ProducerDef{{Name: loomshed.NameFrictionReflect, Producer: p}}, StatusPath: …, LockPath: …, StatusLockPath: …}` where `p` comes from `loomshed.NewFrictionReflect(loomshed.NameFrictionReflect, c.reflectFrictionRow)`, and `LockPath` and `StatusLockPath` are distinct files.
      Take `loomengine.LoomFrictionLock(c.location)` with `lock.TryAcquireWriteLock` in the test (creating its directory first), start `shed.Run(context.Background())` in a goroutine that sends its result on a channel, and assert over a bounded `select` with a sub-second `time.After` that no result arrives and that the persisted status still reads `state: running` at `Friction-Reflect`.
      Release the lock, then wait (bounded `select`, a timeout of a few seconds as the failure path) for the result: `shedengine.RunDone`, persisted `state: done`, and `c.rowFrictionStatus == frictionengine.StatusFailed`.
    - `TestLoomPostRun_DoneReportsTheRowStatusWithoutReflecting`: relative `frictionDir` and zero `cfg` (self-report disabled); with `rowFrictionStatus: "reflected"`, `c.loomPostRun(context.Background(), shedengine.Result{Outcome: shedengine.RunDone}, nil)["friction"]` is `"reflected"`; with `rowFrictionStatus` empty it is `frictionengine.StatusSkipped` (not `StatusFailed`, which a reflection attempt would have produced).
    - `TestLoomPostRun_BlockedStillReflects`: relative `frictionDir`, `shedengine.RunBlocked` → `"friction"` is `frictionengine.StatusFailed`; with `frictionDir: ""` it is `frictionengine.StatusSkipped`.
  - In `internal/loomcli/wiring_test.go`, add, following the existing `TestWire_WebsterRunIsFilled` shape (`hubLocation(t, "warp", ".")`):
    - `TestWire_ReflectFrictionIsFilled`: after `c.wire(loc, loc.AnchorPath())`, `c.env.ReflectFriction` is non-nil.
    - `TestArmAt_RecordsTheArmingVerb`: `c := &loomCLI{}`; `c.armAt(loc, "start", nil)` returns a nil error and leaves `c.armedVerb == "start"` (`start` is not a generic shed verb, so `resolveRunID` does not demand a seed).
  - `internal/loomcli/bootstrap_test.go`'s driver-field tripwire must stay green with no new carve-out; no test here reads a seed's `Driver` field.
- **Commit:** `test(loomcli): cover the Friction-Reflect closure, its lock wait and PostRun reporting`

## Batch Tests

`verify:` runs the untagged suites of `internal/loomcli` (the card-5 behaviour, card-6 tests, the driver-field tripwire in `bootstrap_test.go`), `internal/loomengine` (the `config.go` doc edit compiles), `internal/loomrecipe` (the recipe still builds against the Env shape), and `internal/shedcli` (arms loom through `loomcli.ArmAt` for `lyx shed run|step --recipe loom`).
It then runs loomcli's `integration`-tagged suite, skipping only `TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver`, which fails on the unmodified tree for a reason unrelated to this task (see the overview's pre-existing-integration-failure-skipped Decision), and loomcli's `smoke`-tagged suite, which skips itself when tmux is absent.
The smoke suite's drivers block at Discussion-Write and never reach `Friction-Reflect`; it runs here because this batch changes the package's run-path behaviour, not because it covers the new row.
