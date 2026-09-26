# Discussion: Loom persists done only after post-run friction reflection

```yaml
task: Loom persists done only after post-run friction reflection
slug: loom-done-after-friction
status: discussing
parent_branch: main
```

## Problem

loom's run writes `state: done` to its status file before the Tier 2 friction reflection runs.
The terminal `Finalize` row returns `Done`, `shedengine.stepLocked` persists `StateDone` (`internal/shedengine/run.go`, the `outcome == Done` / empty-`OnDone` arm), `shed.Run` returns and releases the run lock, and only then does `shedverbs`' run body call loom's `PostRun` hook (`loomPostRun`, `internal/loomcli/arm.go`), which calls `reflectFriction` (`internal/loomcli/run.go`) — a real agent bounded by `friction_timeout_min` (thirty minutes in the shipped template).

batten's Run-Shed row (`internal/battenshed/innerrun.go`, `innerRunProducer.Call`) watches the child's status file and returns `Done` on the first poll that reads `StateDone`.
The next row, Worktree-Teardown, ends the child's reed session and removes the worktree, deleting `.lyx/loom/friction/` while the reflection agent is still working.
With friction on (the shipped default), every batten-driven task loses its Tier 2 report (crucible-batten-followup finding R2-F5).

A step-driven run (`lyx loom step`, the `llm` driver) never reflects, and that is deliberate, not a gap: the reflection agent files public GitHub issues itself (`contracts/stencils/friction/friction-template-reflection.md`, Step 3), while `plugins/ly/skills/ly-drive/SKILL.md` § Self-report keeps filing behind an operator ("Nothing files automatically while this loop is driving `loom`") and has the driver list `.lyx/loom/friction/` at every stop.
This task preserves that split.

The chosen fix, fixed by the task brief: loom persists `done` only after its post-run bookkeeping, so batten keeps its contract of watching the status file, not the driver.

## Scope

**In:**

- A new loom recipe row, `Friction-Reflect`, placed after `Finalize` and made the recipe's sole terminal: `Finalize.on_done: Friction-Reflect`, `Friction-Reflect.on_done: ""`, `terminals: [Friction-Reflect]` in `contracts/recipes/loom-recipe.yaml`.
- A new `NameFrictionReflect = "Friction-Reflect"` constant in `internal/loomshed/loomshed.go`, a producer type for it (in `internal/loomshed`), a `"FrictionReflect"` registry entry in `internal/shedrecipe`, and an interrupt-policy entry in `internal/loomshed/interruptpolicy.go`.
- A new `shedrecipe.Env` closure field carrying the reflection call, filled by loomcli's `wire` (`internal/loomcli/wiring.go`, the `c.env = shedrecipe.Env{...}` literal).
- `loomPostRun` stops reflecting on `RunDone` (the row already did it) and keeps reflecting on `RunBlocked`; the run envelope's `friction` key keeps reporting the status.
- Doc-comment and doc updates wherever the text says reflection runs after `shed.Run` returns on the done path (see Technical context), and wherever text pins loom's row count or names `Finalize` as the last row, including `plugins/ly/skills/ly-drive/SKILL.md` § The loop (see "Stale row-count and last-row text").

**Out:**

- Any change to `internal/shedengine` (no new hook, no new state).
- Any change to batten (`internal/battenshed`, `internal/battencli`): its watch contract stays exactly "child `StateDone` means finished".
- Step-driven runs' reflection behaviour: still none (see "Reflection only when armed for run"), so `plugins/ly/skills/ly-drive/SKILL.md`'s § Self-report stays true and is not edited.
  Its row-count text is a separate matter and is in scope (see "Stale row-count and last-row text").
- batten itself: no change.
  Run-Shed's watch budget (`max_bounces: 1440` × `poll_interval_s: 30`, twelve hours, in `contracts/recipes/batten-recipe.yaml`) now also spans the child's reflection, up to `friction_timeout_min` (thirty minutes in the shipped template) more before `done`; that is well inside the budget, so no batten change is needed.
- The `RunBlocked` reflection path's behaviour: unchanged, still in `PostRun`, still lock-free after `shed.Run` returns.
  batten treats a blocked child as a hard error and never tears it down, so that path has no race.
- `frictionengine.Reflect` itself, the friction note directive, and the reflection stencil.
- Making friction a shed-level location shared by every recipe (an open question in `shed-llm-driver.md`, not this task).
- `manifest/roadmap.md`: this is a bugfix, and the roadmap moves only on planned items.

## Decisions

### Mechanism: a terminal recipe row, not an engine hook

- Decision: reflection becomes the `Friction-Reflect` producer row, the new terminal after `Finalize`.
  The engine persists `Finalize`'s `Done` as `current_producer: Friction-Reflect, state: running`, runs the row under the still-held run lock, and persists `state: done` only when the row returns.
- Rationale: it uses the shed machine as it stands, so the "done means finished" ordering falls out of the existing one-persist-per-iteration loop with no engine change.
  A crash mid-reflection resumes at the row alone, never re-running `Finalize`'s landing.
  The run lock stays held for the reflection's whole life, so a second `lyx loom start` sees a busy driver instead of the lock-free window `reflectFriction`'s doc comment describes.
  A step-driven run also reaches the row; what the row does there is decided under "Reflection only when armed for run".
- Rejected: a `shedengine` pre-terminal hook called before persisting `StateDone`.
  It widens a generic engine for one recipe's need, and a crash during the hook leaves `current_producer: Finalize, state: running`, so resume re-runs the landing.
- Rejected: a product-payload "finalizing" flag batten also reads.
  It changes batten's contract, which the brief rules out.

### A pause or cancel landing at Friction-Reflect is accepted

- Decision: accept that a `lyx loom pause` requested while `Finalize` runs (or a context cancellation — Ctrl-C, a parent deadline — arriving then) now halts the run at `current_producer: Friction-Reflect, state: paused` on an already-landed task, instead of `Finalize`'s Done persisting `done` as today.
  This holds even with Tier 2 off, where the row itself is a no-op: the pause check in `stepLocked` (step 3, `internal/shedengine/run.go`) runs before every producer call.
  Resuming the child's run (`lyx loom run`, or `lyx loom start` as the bootstrap verb) calls only the row and then persists `done`; a batten run watching that child hard-errors on `StatePaused` in `innerRunProducer.Call` and names exactly this remedy already (`haltedChildRemedy`: resume the task worktree's own run first, then the batten run).
- Rationale: a pause or cancel is an explicit operator stop, and the engine honours it at every producer boundary; `Friction-Reflect` is one more boundary, not a new failure class.
  The resulting state is the same shape as a pause at any earlier row, with the same documented two-step resume, and it loses nothing: `Finalize`'s Done is already in history and the friction notes are still on disk.
  The alternative costs more than it saves: ruling the window out needs either an engine-level "pause-exempt" producer flag (this task keeps `internal/shedengine` untouched, per the Shed Producer-Seam Invariant) or making `Finalize` itself terminal again, which reintroduces the bug.
- Rejected: an engine flag making a row pause-exempt — widens the generic engine for one recipe's corner case.
- Rejected: loomcli's `pause` verb refusing while `Finalize` runs — pause is recorded as a flag and consumed at the next boundary, so the verb cannot tell which boundary will consume it, and a refusal would also block the legitimate case of pausing before `Finalize` starts.

### Reflection only when armed for run

- Decision: the row reflects only when loomcli's spec was armed for the `run` verb.
  Armed for `step` (`lyx loom step`, `lyx shed step --recipe loom`), the closure returns `frictionengine.StatusSkipped` without reflecting, and the row returns `Done`, leaving the notes in `.lyx/loom/friction/` for ly-drive's operator-gated flow.
  loomcli records the arming verb on the receiver in `armAt` (it already switches on `verb` there and in `specFor`), and the `Env` closure reads that field.
- Rationale: this keeps today's split exactly — `run` reflects and files, `step` leaves filing to the operator ly-drive keeps in the loop — so no autonomous public-issue filing is introduced, and ly-drive's Self-report text (including "`step` never spawns the reflection pass that `run` runs") stays true.
  `shed-llm-driver` (#28) is already reworking ly-drive's friction handling; changing the filing gate belongs there, not in a bugfix.
  Gating on the verb, not the seed, matters: the **Driver Choice Single-Site Invariant** forbids any code path from gating behaviour on the *recorded* seed driver, so reading `Seed.Driver` here would violate it, whereas the verb is the invocation's own fact.
- Rejected: gating on the recorded seed driver (`go` vs `llm`) — barred by the invariant above.
- Rejected: reflecting under `step` too and rewriting ly-drive's Self-report section — reverses a deliberate operator gate as a side effect of a bugfix.

### Stale row-count and last-row text

- Decision: every text that pins loom's row count ("fourteen") or names `Finalize` as the recipe's last/terminal row is updated in the same commit, found by method rather than from a hand list: grep the repo (Go comments, cobra help text, yaml comments, skills under `plugins/`, `docs/`, `manifest/designs/`, `contracts/`) for `fourteen`, `14 rows`/`14 producer`, and for `Finalize` in terminal/last-row/ends-the-run phrasing, and fix every hit that describes loom's recipe.
  Known hits at discussion time, as a floor rather than the list: the recipe yaml header, `internal/loomshed/loomshed.go`, `internal/loomshed/doc.go`, `internal/loomshed/interruptpolicy.go` (twice), `lyx loom`'s user-visible help in `internal/loomcli/cli.go` ("walks fourteen producer rows … and finally Publish and Finalize"), and `plugins/ly/skills/ly-drive/SKILL.md` § The loop.
  Where a count is not load-bearing, reword it away rather than bump it, per the no-perishable-tally rule; where it is (ly-drive's step-cap arithmetic), re-derive it: one more row makes the three-rounds-per-segment walk thirty-six steps, still under the forty-step cap, so the cap is unchanged and only the arithmetic text moves.
- Rationale: the reviewer found the hand list incomplete twice over; a grep-driven sweep is the only enumeration that survives.
  The ly-drive edit is limited to the row-count/cap text this change falsifies, since `shed-llm-driver` (#28) is reworking that skill.
- Rejected: a hand-maintained list only — already proved incomplete.

### Blocked outcome keeps today's PostRun reflection

- Decision: `loomPostRun` reflects only on `RunBlocked` (still gated by `shouldReflectFriction`'s Tier-2-enabled check, i.e. a non-empty `frictionDir` path string — it does not check whether the directory holds notes; `frictionengine.Reflect` itself returns `skipped` on an empty directory).
  On `RunDone` it no longer calls `reflectFriction`.
- Rationale: a row cannot fire on a blocked halt, and blocked reflection is unaffected by the bug, since batten never tears down a blocked child.
  A blocked task may never be resumed, so dropping its reflection would lose notes.
- Rejected: dropping blocked reflection entirely.
  That turns a fix into a regression for halted tasks.

### Row always returns Done

- Decision: the `Friction-Reflect` producer returns `shedengine.Done` with a nil error on every path: reflection filed, skipped, failed, Tier 2 off, or reflection lock held by another driver.
  Failures stay logged at Warn inside `reflectFriction`, as today.
- Rationale: this is `reflectFriction`'s existing rule — failing an already-merged run because an optional bookkeeping agent could not run is strictly worse than filing nothing.
  A `Stuck` or error here would block or fail a run whose landing already happened.
- Rejected: `Stuck` on reflection failure.
  It would leave a landed task reading `blocked` and stall batten's watch forever on a halted child.

### How the row reaches loomcli's reflection

- Decision: add one closure field to `shedrecipe.Env`, e.g. `ReflectFriction func() string`, returning the `frictionengine` status string.
  loomcli's `wire` fills it with a small wrapper around `c.reflectFriction` that also records the returned status on the receiver.
  The wrapper returns `frictionengine.StatusSkipped` without calling `reflectFriction` when `c.frictionDir` is empty (Tier 2 off, mirroring `shouldReflectFriction`'s empty-directory gate) or when the receiver was armed for a verb other than `run` (see "Reflection only when armed for run").
  The producer's own constructor in `internal/loomshed` refuses a nil closure with an error; the `"FrictionReflect"` registry entry validates no `Env` field itself and returns that constructor's error wrapped with the row name, following `publishEntry`'s delegate-to-constructor split in `entries_simple.go` (which leaves nil-closure checks to `landingshed.NewPublish`).
- Rationale: keeps `internal/shedrecipe` and `internal/loomshed` free of any `frictionengine`, `lock` or `loomengine` import for this purpose — the reflection's deps (shuttle runner, stencils dir, registry, config, lock path) are already resolved on loomcli's receiver, and `Env` already carries closures such as `CommitWebster`, `CommitDiscussion` and `ApprovePlan` for the same reason.
  `reflectFriction` itself is reused unchanged, including its non-blocking `LoomFrictionLock`, which still matters because a blocked-path `PostRun` reflection runs lock-free and can overlap a later driver's row.
- Rejected: constructing `frictionengine.Deps` inside the registry entry.
  That duplicates loomcli's wiring and pulls feature packages into a Told-Geometry-bound package.

### Run envelope's friction key

- Decision: the run envelope keeps its unconditional `friction` key.
  `loomPostRun` reports the status the row recorded this process on `RunDone`; the fresh `reflectFriction` result on a reflecting `RunBlocked`; `frictionengine.StatusSkipped` otherwise, including a `RunDone` whose row did not run in this process (a re-run of an already-done status file hits the engine's done short-circuit).
- Rationale: keeps the envelope's shape and meaning for every existing consumer, and the key is still never silently dropped (per `loomPostRun`'s own doc comment).
- Rejected: dropping the key on `RunDone`.
  It is a visible envelope change for no gain.
  The step envelope is closed (see the Shed Verb-Set Invariant) and gains nothing.

### Row naming and interrupt policy

- Decision: row name `Friction-Reflect`, engine name `FrictionReflect`, interrupt policy `InterruptPolicyReinvoke`.
- Rationale: follows the recipe's hyphenated row / camel-cased engine convention.
  Re-invoking is not a guaranteed no-op: the reflection agent files issues before it writes its report, and `frictionengine.Reflect` archives the directory only after that, so an interrupt between a `lyx selfreport create` call and the archive leaves the notes in place and a re-invoked row can file duplicate issues.
  That window exists identically on today's `PostRun` path, so this task does not widen it.
  `reinvoke` is still preferred because the common interrupt (before any filing, or after the archive) either finishes the job or no-ops on an empty directory, and a rare duplicate issue is cheaper than stopping a landed run for a human.
- Rejected: `handback`.
  It halts every interrupted, already-landed run for a human, whose only possible action is to re-run the same reflection.

### In-flight runs

- Decision: no migration.
  A status file already `done` at `Finalize` keeps working: the engine's done short-circuit fires before the producer lookup, and `Finalize` stays in the list anyway.
  A run still mid-flight when this lands routes `Finalize → Friction-Reflect` on its next `Finalize` Done, from the recipe as rebuilt at that process's start.
- Rationale: row names are durable resume identities, and this change only adds one; no existing name is renamed or removed.

## Technical context

- Engine loop: `internal/shedengine/run.go`. `stepLocked` persists once per iteration; a `Done` with non-empty `OnDone` persists `StateRunning` with `current_producer` set to the `OnDone` target, which is what holds `done` back until the new row returns.
  `Run` holds the run lock across the whole loop.
- Generic run verb: `internal/shedverbs/run.go` calls `PostRun` after `shed.Run`, before the envelope; hook contracts in `internal/shedverbs/spec.go`.
- loom hooks: `internal/loomcli/arm.go` (`loomPostRun`, `specFor`); reflection call and gate: `internal/loomcli/run.go` (`shouldReflectFriction`, `reflectFriction`); receiver fields: `internal/loomcli/cli.go` (`frictionDir`); `Env` filled in `internal/loomcli/wiring.go`.
- Recipe: `contracts/recipes/loom-recipe.yaml` (embedded by `contracts/recipes/recipes.go`).
  Its header comment counts rows ("fourteen row names") and enumerates the escalate-to-a-human set; the `Finalize` row carries a comment calling its empty `on_done` load-bearing — that comment moves to the new terminal row.
- Row names: `internal/loomshed/loomshed.go` (row-count text is swept per "Stale row-count and last-row text"); interrupt policies: `internal/loomshed/interruptpolicy.go`, with a meta test in `internal/loomrecipe/interruptpolicy_meta_test.go`.
- Recipe assembly and guards: `internal/loomrecipe/loomrecipe.go`, `coverage_guard_test.go` (pins yaml names to `loomshed` constants), `shape_test.go`, `sequence_test.go`, `recipe_test.go`, `resume_test.go`.
- Registry: `internal/shedrecipe/registry.go` (one map literal), `entries_simple.go` (closure-style entries to model on), `Env` in `internal/shedrecipe/recipe.go`.
- Graph checks: `internal/shedbuild` / `internal/shedcheck` validate `entry`/`terminals` against the built producer list, so `terminals` must name `Friction-Reflect`, not `Finalize`.
- batten watch: `internal/battenshed/innerrun.go` — `StateDone` → `Done`, `StateRunning` → poll again, blocked/paused/failed → hard error.
  Untouched, but it is the consumer whose behaviour proves the fix.
- Comments that describe reflection as running after the run lock is released, to reword so they refer to the blocked path only: `reflectFriction`'s doc (`internal/loomcli/run.go`), `awaitRunLock` / `dispositionForHandshake` (`internal/loomcli/bootstrap.go`), the `halted` closure comment in `internal/loomcli/start.go`, `LoomFrictionLock`'s doc in `internal/loomengine/config.go` ("the reflection fires after that return, so for the whole of the reflection agent's life the run lock reads as free"), and `internal/frictionengine/doc.go`.
  The `awaitRunLockHalted` arm itself stays: a fast blocked halt still reflects lock-free after `shed.Run` returns.
- `docs/overview.md` describes `internal/frictionengine` as "the aggregation-and-reflection step internal/loomcli's run verb calls once per run"; reword to cover the terminal row plus the blocked path.
## Constraints

- **Shed Producer-Seam Invariant**: `internal/shedengine` is not modified at all.
- **Shed Recipe Registry Invariant**: the new constructor is one more entry in the existing map literal, reached through `Lookup`/`Names`; no `init()` registration; no `lyxcwd` import.
- **Told-Geometry Invariant**: `shedrecipe` and `loomshed` are bound packages — the new producer and entry derive no path; the friction paths stay resolved inside loomcli behind the closure.
- **Shed Verb-Set Invariant**: `shedverbs`' run/step bodies are not reimplemented or changed; the step envelope's closed shape is untouched.
- **Friction Leaf Invariant**: `internal/friction` gains no import.
- **Driver Choice Single-Site Invariant**: the row's run-versus-step gate reads the arming verb, never the seed's recorded `Driver` field; `internal/loomcli/bootstrap_test.go`'s driver-field-reader tripwire must stay green without a new carve-out.
- **Batten Bookend Invariant**: batten's teardown sequencing is not touched.
- **Test Tier Purity Invariant**: tests use injected fakes for the reflection closure, never a real agent or a long sleep.
- **Documentation Lifecycle** / CLAUDE.md docs rule: doc and comment updates land in the same commit as the behaviour change.
  No new cross-cutting invariant is introduced, so `CONSTRAINTS.md` is not edited.
- Markdown edits follow semantic line breaks.

## Testing

- **TDD candidate — ordering (the bug itself).** Drive loom's recipe (or a `shedengine.Shed` built from it with fakes for every row) to the end with a fake `ReflectFriction` closure that reads the status file while it runs.
  Assert it observes `state: running` with `current_producer: Friction-Reflect`, and that `state: done` appears only after the closure returns.
  Place it where the loomrecipe tests already build the recipe against fakes (`internal/loomrecipe`'s fixture), so no real agent runs.
- **Producer unit test (`internal/loomshed`)**: the row returns `Done`, nil error, for every status the closure returns (filed/skipped/failed); it calls the closure exactly once per `Call`.
- **Registry test (`internal/shedrecipe`)**: the `FrictionReflect` constructor refuses a nil closure and builds with one.
- **Recipe shape/coverage**: update `coverage_guard_test.go`, `shape_test.go`, `sequence_test.go` and any row-count or terminal assertions for the fifteen-row list with `Friction-Reflect` as the sole terminal and `Finalize.on_done: Friction-Reflect`; the interrupt-policy meta test must see the new entry.
- **Pause at the row (pins the accepted disposition)**: with `pause_requested` set while `Finalize` runs (a fake `Finalize` that sets the flag through the status file), `Run` halts `paused` at `current_producer: Friction-Reflect` with `Finalize`'s Done in history and the reflection closure not called; a following `Run` calls only the row and persists `done`.
- **Resume**: a status file `done` at `Finalize` (pre-change shape) still short-circuits cleanly; a status file `running` at `Friction-Reflect` resumes by calling only the reflection row.
- **loomcli (`internal/loomcli/friction_test.go` or neighbour)**: `loomPostRun` on `RunDone` does not call reflection and reports the row-recorded status (or `skipped` when none); on `RunBlocked` with Tier 2 enabled (non-empty `frictionDir`) it still reflects; the wiring wrapper returns `skipped` without reflecting when `frictionDir` is empty, and also when the receiver was armed for `step` (Tier 2 enabled, with notes present that are left in place), while armed for `run` it reflects.
- Full `go test ./...` (cgo on) must pass.

## Q&A log

- **Q:** Which mechanism holds `done` back until reflection finishes? **A:** [auto-pick] A new terminal `Friction-Reflect` recipe row after `Finalize`. **Why:** no engine change, crash-resumable at the row without re-running the landing, and the run lock stays held.
- **Q:** What happens to reflection on a blocked halt? **A:** [auto-pick] Keep today's `PostRun` reflection for `RunBlocked`. **Why:** a row cannot fire on blocked, batten never tears down a blocked child, and dropping it would lose notes for tasks never resumed.
- **Q:** What does the row return when reflection fails? **A:** [auto-pick] Always `Done`, failures logged. **Why:** matches `reflectFriction`'s existing rule that a bookkeeping failure must never block or fail a landed run.
- **Q:** What does the run envelope's `friction` key report? **A:** [auto-pick] Kept, reporting the row-recorded status on done, the fresh result on blocked, `skipped` otherwise. **Why:** preserves the envelope's shape for existing consumers.
- **Q:** Should the row reflect under step-driven runs (`llm` driver)? **A:** [auto-pick] No — reflect only when armed for `run`, gated on the arming verb, not the recorded seed driver. **Why:** reflection files public issues, ly-drive deliberately keeps filing behind an operator, and the Driver Choice Single-Site Invariant bars gating on the recorded driver (raised by the round-1 orchestrator review).
- **Q:** A pause or cancel during `Finalize` now halts `paused` at `Friction-Reflect` on a landed task, which batten treats as a halted child — accept or rule out? **A:** [auto-pick] Accept and document; pin with a test. **Why:** it is an explicit operator stop at an ordinary producer boundary with batten's existing remedy text, and ruling it out needs an engine change or reintroduces the bug (raised by round-2 review).
- **Q:** How are stale "fourteen rows" / "Finalize is last" texts found? **A:** [auto-pick] A grep-driven sweep across code comments, help text, yaml, skills and docs, with the known hits listed as a floor; ly-drive's step-cap arithmetic is re-derived (36 steps, cap 40 unchanged). **Why:** the hand list proved incomplete (raised by round-3 review).
- **Q:** How does the row reach loomcli's already-resolved reflection deps? **A:** [auto-pick] One closure field on `shedrecipe.Env`, filled by loomcli's `wire`. **Why:** matches the existing closure fields and keeps feature imports out of Told-Geometry-bound packages.
