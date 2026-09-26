# Batch: friction-reflect-row

```yaml
task: Loom persists done only after post-run friction reflection
batch: friction-reflect-row
number: 1
cards: 4
verify: go test ./internal/loomshed/... ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/loomrecipe/... ./internal/loomcli/... ./internal/landingshed/... && go test -tags integration ./internal/landingshed/...
depends-on: []
```

## Batch Scope

This batch adds the `Friction-Reflect` terminal row on the shed side: the `loomshed` constant and producer, the `shedrecipe.Env.ReflectFriction` closure field and its `FrictionReflect` registry entry, the recipe yaml edit that makes the row the sole terminal after `Finalize`, its interrupt-policy entry, and the loomrecipe tests that pin the done-after-reflection ordering, the accepted pause disposition, and resume.
It also sweeps every row-count and last-row text the new row falsifies (Go comments, yaml header, `lyx loom`'s help text, ly-drive's step-cap arithmetic).
The external interface batch 2 consumes is `shedrecipe.Env.ReflectFriction func() string` and `loomshed.NewFrictionReflect`/`loomshed.NameFrictionReflect`.
No card here touches loomcli's wiring or `loomPostRun`; see the overview's intermediate-state-between-batches Decision.

## Cards

### Card 1: loomshed Friction-Reflect producer and row name

- **Context:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/stub_test.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/doc.go`
- **Creates:**
  - `internal/loomshed/frictionreflect.go`
  - `internal/loomshed/frictionreflect_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/loomshed/loomshed.go`, add `NameFrictionReflect = "Friction-Reflect"` to the `const` block, directly after `NameFinalize`.
    Reword the file's header comment and the block's doc comment so neither states a row count: "declares loom's fourteen durable row names" becomes "declares loom's durable row names", and "spells the same fourteen names as yaml strings" becomes "spells the same names as yaml strings".
  - In `internal/loomshed/doc.go`, reword "owns loom's own eight producer constructors, its fourteen durable row names" to "owns loom's own producer constructors, its durable row names" (no counts).
  - Create `internal/loomshed/frictionreflect.go` with a file header comment, an unexported type `frictionReflect` holding `name string` and `reflect func() string`, a compile-time assertion `var _ shedengine.ShedProducer = (*frictionReflect)(nil)`, and:
    - `func NewFrictionReflect(name string, reflectFriction func() string) (shedengine.ShedProducer, error)`: returns `nil, fmt.Errorf("loomshed: %s: reflect closure must not be nil", name)` when `reflectFriction` is nil, otherwise the producer.
      Its doc comment states the return type is the seam interface so the registry can call it from outside the package (mirroring `NewStub`), and that the nil check lives here so the registry entry need not duplicate it.
    - `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)`: first `entryErr(ctx, p.name)` (return `"", shedengine.OutputPointer{}, err` on non-nil); then call `p.reflect()` exactly once; log `logger.Info("loomshed: friction reflection row finished", "producer", p.name, "status", status)`; return `shedengine.Done, shedengine.OutputPointer{}, nil`.
      No `cancelErr` check after the closure returns.
    - The type's doc comment carries the rationale, briefly: this is loom's terminal row, placed after `Finalize` so the engine persists `state: done` only once the reflection returns (batten's Run-Shed row watches the child's status file for `done` and tears the worktree down right after); it returns `Done` on every closure result because failing or blocking a run whose landing already happened over an optional bookkeeping agent is strictly worse than filing nothing; it derives no path and imports nothing friction-specific, since whether to reflect at all (Tier 2 off, armed for `step`) is the injected closure's decision (Told-Geometry Invariant); and the missing post-call `cancelErr` is deliberate, because once the reflection has run the row's work is done and `done` must persist.
  - Create `internal/loomshed/frictionreflect_test.go` (package `loomshed`, untagged, no sleeps):
    - a table over closure statuses `"reflected"`, `"skipped"`, `"failed"`: `Call(context.Background())` returns `shedengine.Done`, an empty `OutputPointer`, a nil error, and the closure was called exactly once per `Call`;
    - `NewFrictionReflect("Friction-Reflect", nil)` returns a non-nil error naming the row and a nil producer;
    - with an already-cancelled context, `Call` returns a non-nil error and the closure is never called.
  - Import only packages already on `internal/loomshed/seam_enforcement_test.go`'s allowlist (`internal/shedengine`, `internal/logger`, plus stdlib).
- **Commit:** `feat(loomshed): add the Friction-Reflect producer and row name`

### Card 2: shedrecipe ReflectFriction seam and FrictionReflect entry

- **Context:**
  - `internal/loomshed/frictionreflect.go`
  - `internal/loomshed/stub.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/config.go`
  - `internal/shedrecipe/coverage_guard_test.go`
- **Edits:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedrecipe/entries_simple_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/build_engines_test.go`
  - `docs/overview.md`
  - `contracts/specs/shed-recipe-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `internal/shedrecipe/recipe.go`, add to `Env`, directly after `ApprovePlan`, a field `ReflectFriction func() string` with a doc comment: the injected closure the `FrictionReflect` entry's producer calls once per `Call`, returning the reflection's status string; it arrives as a closure, following `CommitWebster`/`CommitDiscussion`/`ApprovePlan`, because the reflection's dependencies are already resolved on internal/loomcli's receiver, and building them here would pull friction, lock and loom-config imports into this Told-Geometry-bound package.
  - In `internal/shedrecipe/entries_simple.go`, add `frictionReflectEntry(name string, cfg Config, env Env) (shedengine.ShedProducer, error)`: `configRejectUnknown(cfg)` first; then `p, err := loomshed.NewFrictionReflect(name, env.ReflectFriction)`; on error return `nil, fmt.Errorf("shedrecipe: FrictionReflect: %w", err)`; otherwise `p, nil`.
    Its doc comment states that it validates no Env field of its own and inherits `loomshed.NewFrictionReflect`'s nil-closure refusal, the same delegate-to-constructor split `publishEntry` uses with `landingshed.NewPublish`.
    Update the file header comment so it lists `frictionReflectEntry` and states no count ("implements the registry entries that take an empty Config and validate only the Env fields they read: …").
  - In `internal/shedrecipe/registry.go`, add `"FrictionReflect": frictionReflectEntry,` to the `registry` map literal, and reword "The table is complete at sixteen keys." out of the doc comment (keep the sentence about coverage being checked in the cross-consumer coverage guard).
  - In `internal/shedrecipe/registry_test.go`, rename `TestRegistry_ShipsSixteenEntries` to `TestRegistry_ShipsExpectedEntries`, reword its doc comment to drop "sixteen", and insert `"FrictionReflect"` into its sorted `want` list between `"Finalize"` and `"InnerRun"`.
  - In `internal/shedrecipe/fixture_test.go`'s `newTestEnv`, fill `ReflectFriction: func() string { return "skipped" },`.
  - In `internal/shedrecipe/entries_simple_test.go`, add a `simpleEntryCases` row `{registryKey: "FrictionReflect", entry: frictionReflectEntry, buildEnv: newTestEnv, validatedFields: nil, unreadField: "Cwd"}`, and a new `TestFrictionReflectEntry_NilClosureRejected`: with `env.ReflectFriction = nil`, `frictionReflectEntry("Friction-Reflect", Config{}, env)` returns a non-nil error containing `"FrictionReflect"` and a nil producer.
  - In `internal/shedbuild/fixture_test.go`'s `newTestEnv`, fill `ReflectFriction: func() string { return "skipped" },` so `TestBuild_EveryRegisteredEngineBuilds` builds the new engine.
  - In `internal/shedbuild/build_engines_test.go`, reword `engineMinimalConfig`'s doc comment so it states no engine count ("The other fourteen engines take no config" becomes "Every other engine takes no config"; "join that fourteen" and "two of the fourteen" become "join them" and "two of them"), and reword `TestBuild_EveryRegisteredEngineBuilds`'s "a fifteenth registered engine" to "a newly registered engine".
  - Reword three more registry tallies this card falsifies, naming the source instead of a count:
    - `internal/shedrecipe/entries_simple_test.go`'s header "is table-driven over the seven value-only entries" becomes "is table-driven over the value-only entries `simpleEntryCases` lists";
    - `internal/shedbuild/fixture_test.go`'s `newTestEnv` doc "because two of the sixteen engines need it" becomes "because the Publish and Finalize engines need it" (leave its batten-field sentence as is);
    - `docs/overview.md`'s Shed recipe paragraph "it registers sixteen engine names" becomes "its `registry` map literal declares every engine name a recipe row may use";
    - `contracts/specs/shed-recipe-spec.md`'s consumer bullet "already imports `loomshed` for eight of its constructors" becomes "already imports `loomshed` for its loom-specific constructors".
  - `internal/shedrecipe/coverage_guard_test.go`'s `TestCoverageGuard_EveryRegisteredEngineIsReachedOrAllowlisted` fails after this card until card 3 adds the recipe row that reaches `FrictionReflect`; do not add `FrictionReflect` to `coverageGuardAllowedUnreachableEngines`.
- **Commit:** `feat(shedrecipe): add the ReflectFriction seam and FrictionReflect entry`

### Card 3: make Friction-Reflect loom's sole terminal row

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/frictionreflect.go`
  - `internal/loomrecipe/interruptpolicy_meta_test.go`
  - `internal/loomrecipe/recipe_test.go`
  - `internal/shedrecipe/recipe.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/interruptpolicy.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/sharedbootstrap_test.go`
  - `internal/landingshed/deps.go`
  - `plugins/ly/skills/ly-drive/SKILL.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `contracts/recipes/loom-recipe.yaml`:
    - `terminals:` becomes the single entry `Friction-Reflect`.
    - The `Finalize` row's `on_done` becomes `Friction-Reflect`; replace its "on_done is explicitly empty" comment with one line saying it hands off to the reflection row rather than ending the run.
    - Append a new last row: `name: Friction-Reflect`, `engine: FrictionReflect`, `on_done: ""`, no `on_stuck`, no `config`.
      Above it, a comment: this row runs the Tier 2 friction reflection under the run lock and is the recipe's sole terminal, so the engine persists `state: done` only after the reflection returns (batten's Run-Shed row reads that `done` as "finished" and tears the worktree down, deleting the friction notes); it always returns Done, and under `step` loomcli's closure skips the reflection.
      Move the "on_done is explicitly empty: the empty value is load-bearing and is what ends the whole run quietly, per shedengine.ProducerDef.OnDone's own field doc" comment onto this row.
    - Header: "The fourteen row names below" becomes "The row names below".
      At the end of the paragraph that closes with "eight rows in total escalate rather than bounce, exactly as many as before …" (or as a new short paragraph right after it), add one sentence placing Friction-Reflect outside the escalate-to-a-human set: it also carries no on_stuck, but it always returns Done and never Stuck, so the count of eight stays true.
  - In `internal/loomshed/interruptpolicy.go`:
    - add `NameFrictionReflect: InterruptPolicyReinvoke,` to `InterruptPolicies`, after `NameFinalize`;
    - reword the file header and the `InterruptPolicies` doc comment so neither says "fourteen" ("each of loom's durable row names");
    - the doc paragraph that justifies every reinvoke row by the Attach probe ("On every row but Webster, re-calling current_producer …") is narrowed to "every row but Webster and Friction-Reflect", and a new paragraph states Friction-Reflect's own premise: the table answers only for `lyx loom step`, and armed for `step` the row spawns nothing (loomcli's closure returns skipped without reflecting), so a re-invocation cannot double-spawn — reinvoke holds on the contract's own premise rather than through an Attach probe, which `frictionengine.Reflect` does not have.
  - In `internal/loomrecipe/shape_test.go`:
    - add an unexported helper `frictionReflectProducerType() reflect.Type` that calls `loomshed.NewFrictionReflect("", func() string { return "" })`, panics on a non-nil error, and returns `reflect.TypeOf` of the producer;
    - in `wantProducerTable`, the `loomshed.NameFinalize` row's `onDone` becomes `loomshed.NameFrictionReflect`, and a new last row `{loomshed.NameFrictionReflect, "", "", "", 0, frictionReflectProducerType()}` is appended;
    - in `testEnv`, fill `ReflectFriction: func() string { return "skipped" },`;
    - in `TestNew_RoutingGraphIsClean`, the terminals argument to `shedcheck.Check` becomes `[]string{loomshed.NameFrictionReflect}`.
  - In `internal/loomrecipe/coverage_guard_test.go`, add `loomshed.NameFrictionReflect: "FrictionReflect",` to `loomRowEngines`.
  - In `internal/loomrecipe/fixture_test.go`'s `buildSequenceFixture`, fill `ReflectFriction: func() string { return "skipped" },` in the returned `env`.
  - In `internal/loomcli/cli.go`, reword the `loom` parent command's `Long` text: "The machine walks fourteen producer rows: … and finally Publish and Finalize." becomes "The machine walks its producer rows: a two-row preflight, then Discussion, Plan, and Webster, each of the three followed by its own LLM review segment that loops until it approves or escalates, then Publish and Finalize, and last Friction-Reflect, which runs "run"'s Tier 2 friction reflection before the run records done."
    Keep the raw string's existing manual wrapping width, and use the same double-quote style the surrounding text uses for verb names.
  - In `internal/loomcli/sharedbootstrap_test.go`'s `TestBuildLoomShed_OutputShape`, reword the doc comment's "carries all fourteen producer rows" to "carries every producer row", and replace the stale `len(shed.Producers) != 17` assertion with a comparison against `len(loomshed.InterruptPolicies)` (the table `internal/loomrecipe`'s interrupt-policy meta test pins to exactly the assembled rows), adding the `internal/loomshed` import.
  - In `internal/landingshed/deps.go`, the `Deps.CommitStatus` field doc's "Without this seam the last row of a loom run refuses on the run's own bookkeeping" becomes "Without this seam loom's Finalize row refuses on the run's own bookkeeping"; change nothing else in that file.
  - In `plugins/ly/skills/ly-drive/SKILL.md` § The loop: "Loom's list is fourteen rows, … walks thirty-five steps, rounded up to 40." becomes "Loom's list is fifteen rows, … walks thirty-six steps, rounded up to 40."; "A recipe with a graph shaped differently from loom's fourteen rows" becomes "A recipe with a graph shaped differently from loom's own".
    Keep one sentence per line; edit no other part of the skill (its § Self-report stays as is).
- **Commit:** `feat(loom): persist done only after the Friction-Reflect terminal row`

### Card 4: pin the done-after-reflection ordering, pause, and resume

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/resume_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/run.go`
  - `internal/state/state.go`
- **Edits:** none
- **Creates:**
  - `internal/loomrecipe/frictionreflect_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `internal/loomrecipe/frictionreflect_test.go` (package `loomrecipe`, untagged, no sleeps, no real agent, no git).
    Every test starts from `buildSequenceFixture(t)`, sets `env.ReflectFriction` to a test closure **before** calling `New(env, paths)` (the entry captures the closure at construction), and after `New` replaces the `loomshed.NameFinalize` row's `Producer` (found by name in `shed.Producers`) with a fake, so the real landing producer never runs.
    Status files are positioned with `resetCurrentProducer` (declared in `resume_test.go`) or `state.UpdateJSON` over `shedengine.Status`, and read back with `state.ReadJSONStrict[shedengine.Status]`.
  - `TestFrictionReflect_DonePersistsOnlyAfterReflection`: status at `loomshed.NameFinalize`, running; Finalize fake returns Done; the reflect closure reads the status file while it runs and records `CurrentProducer` and `State`, then returns `"reflected"`.
    Assert: the closure observed `current_producer: Friction-Reflect` with `state: running`; `Run` returns `shedengine.RunDone`; the persisted status afterwards is `state: done` with `current_producer: Friction-Reflect`; the closure ran exactly once; the persisted history's last two entries are `Finalize` then `Friction-Reflect`.
  - `TestFrictionReflect_PauseDuringFinalizeHaltsAtTheRow`: status at `loomshed.NameFinalize`; the Finalize fake sets `PauseRequested = true` through `state.UpdateJSON` and returns Done.
    First `Run`: `shedengine.RunPaused`, `HaltedProducer == loomshed.NameFrictionReflect`, persisted `state: paused` at `Friction-Reflect`, history's last entry is `Finalize`, reflect closure not called.
    A second `Run` over a freshly built Shed (same substitutions, same counters): `shedengine.RunDone`, reflect closure called exactly once, Finalize fake still called exactly once in total.
  - `TestFrictionReflect_ResumeAtTheRowCallsOnlyTheRow`: status at `loomshed.NameFrictionReflect`, running; `Run` returns `shedengine.RunDone`, reflect closure called once, Finalize fake never called.
  - `TestFrictionReflect_PreChangeDoneAtFinalizeShortCircuits`: status set through `state.UpdateJSON` to `State: shedengine.StateDone`, `CurrentProducer: loomshed.NameFinalize` (the pre-change shape); `Run` returns `shedengine.RunDone` with neither the Finalize fake nor the reflect closure called.
- **Commit:** `test(loomrecipe): pin done-after-reflection ordering, pause and resume`

## Batch Tests

`verify:` runs the untagged suites of every package this batch edits: `internal/loomshed` (card 1's producer test), `internal/shedrecipe` (registry pin, entry table, cross-consumer coverage guard), `internal/shedbuild` (every registered engine builds), `internal/loomrecipe` (shape table, coverage guard, interrupt-policy meta test, routing graph, the new card-4 tests) `internal/loomcli` (the help-text edit and `sharedbootstrap_test.go`), and `internal/landingshed` untagged plus `integration`-tagged (card 3's comment edit in `deps.go`; the tagged suite runs in about a second).
loomcli's `integration`/`smoke`-tagged tests are not in scope here: this batch changes no loomcli behaviour, only help text and one always-skipping untagged test, and the task-wide `pipeline.done_gate` runs the tagged suites before done.
