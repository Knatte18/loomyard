# Batch: shedadapters-probe-before-archive

```yaml
task: 'loom: interactive Discussion-Write'
batch: 'shedadapters-probe-before-archive'
number: 3
cards: 3
verify: go test ./internal/shedadapters/ ./internal/shedrecipe/ ./internal/shedbuild/ ./internal/loomrecipe/
depends-on: [2]
```

## Batch Scope

This batch wires batch 2's `Attach` into the producer that needs it: `shedadapters.SingleLLMProducer`.
It widens the shared `shedadapters.Shuttle` seam by the one `Attach` method, updates every implementor of that seam, and re-orders `Call` to probe-then-archive-then-run.

It is one batch because widening an interface is not separable from updating its implementors: the moment `Attach` joins `Shuttle`, the four test fakes across four packages stop compiling, so the widening card and the fake updates must land together.
The verify scope spans four packages for exactly that reason — `internal/shedadapters` owns the seam, and `internal/shedrecipe`, `internal/shedbuild`, and `internal/loomrecipe` each hold a fake implementing it.

The blast radius is stated plainly, per `attach-is-unconditional-not-interactive-only`: the probe is unconditional, so this changes `Plan-Write` and every future generic `SingleLLM` row as well as `Discussion-Write`.
The behaviour change is confined to the case where a live matching run exists at `Call` time, which today happens only on a resume after a crash — on the ordinary first call the probe finds nothing and the existing archive-then-run path is taken unchanged.

The `Bouncer`/`Burler` review rows are explicitly out of scope and keep the duplicate-agent path: they drive the same `Shuttle` seam via `shedrecipe.Env.Shuttle` but do not go through `SingleLLMProducer`, so widening that producer does not reach them.
`internal/mergeresolve`'s own separate `Shuttle` seam is likewise untouched.
Neither omission is a claim that they are safe — both are scope calls, recorded in the discussion.

Batch-local decision: the outcome switch is **shared, not duplicated**.
An attached `Result` is mapped through the same `switch result.Outcome` block a spawned `Result` is, so `OutcomeDone` → `shedengine.Done` with the first output file as the pointer, `OutcomeAsking` → `shedengine.Stuck`, and `OutcomeDied`/`OutcomeTimeout` → an error, with no second copy of that mapping anywhere.

## Cards

### Card 7: widen the `Shuttle` seam with `Attach` and update every implementor

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/run.go`
  - `internal/shedadapters/doc.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_bouncer.go`
- **Edits:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/singlellm_test.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/shedadapters/singlellm.go`, add a second method to the `Shuttle` interface: `Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error)`.
  Its doc comment on the interface must state what the bool means (`false` with a zero `Result` and a nil error means "nothing to attach to, start one") and must record the widen-rather-than-optional-interface decision from `attach-lives-in-shuttleengine`: `Shuttle` is not `SingleLLMProducer`'s private seam — `shedrecipe.Env.Shuttle` feeds the `Bouncer` row and the `PlanWrite`, `DiscussionWrite`, and generic `SingleLLM` rows alike — so `Attach` is a method only `SingleLLMProducer` calls, and it is added to the shared seam anyway because the sole production implementor is `*shuttleengine.Runner`, which gains it for free, and every other implementor is a test fake in this repo.
  It must also record what was rejected: keeping `Shuttle` at one method and type-asserting an optional `Attacher` inside `Call`, which would make the attach behaviour silently absent for any implementor that forgets it — a compile error in a test fake is a better failure than a producer that quietly stops probing.
  The existing `var _ Shuttle = (*shuttleengine.Runner)(nil)` compile-time proof stays and now also proves `Attach` is satisfied.

  Update all four test fakes so the package set still compiles:
  `internal/shedadapters/singlellm_test.go`'s `fakeShuttle` gains a scriptable `Attach` — fields for the returned `shuttleengine.Result`, the `found` bool, and an error, plus an `attachCalled` flag and a recorded spec, so a later card can assert the probe ran and with what;
  `internal/shedrecipe/fixture_test.go`'s `fakeShuttle`, `internal/shedbuild/fixture_test.go`'s `fakeShuttle`, and `internal/loomrecipe/fixture_test.go`'s `fakeLoomShuttle` each gain a minimal `Attach` returning a zero `Result`, `false`, and a nil error, matching each fake's existing doc comment about whether it is ever actually called.
  `fakeLoomShuttle`'s always-not-found `Attach` is the regression guard that every existing `loomrecipe` sequence and resume test still drives the unchanged archive-then-spawn path; batch 5 makes it scriptable.
  Each new method carries a one-line doc comment in the same style as the fake's existing `Run` comment.
  Do not change `internal/burlerengine`, `internal/treadleengine`, or `internal/mergeresolve` — each declares its own separate one-method `Shuttle` interface that this widening does not touch.
- **Commit:** `feat(shedadapters): widen the Shuttle seam with Attach`

### Card 8: `SingleLLMProducer.Call` probes before archiving

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedadapters/archive.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/doc.go`
  - `internal/shuttleengine/attach.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shedengine/producer.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/shedadapters/singlellm.go`
  - `internal/shedadapters/singlellm_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Re-order `SingleLLMProducer.Call` to the sequence `probe-before-archive` prescribes: entry-check the context via the existing `entryErr`; build the spec from `p.specs()`; reject any relative `OutputFiles` entry exactly as today; **probe `p.shuttle.Attach(spec)`**; if it reports found, map its `Result` through the existing outcome switch and return; otherwise `archiveStaleOutputs`, then `p.shuttle.Run(spec)`, then the same outcome switch.

  Extract the outcome switch into an unexported method or function on `SingleLLMProducer` so both the attach path and the spawn path reach one copy — the mapping must not be duplicated.
  It keeps every existing behaviour: `OutcomeDone` returns `shedengine.Done` with `spec.OutputFiles[0]` as the pointer and its empty-`OutputFiles` panic guard, and survives cancellation as the one genuine-success exception; `OutcomeAsking` returns `shedengine.Stuck` after a `cancelErr` check and its existing `logger.Warn`; `OutcomeDied`/`OutcomeTimeout` and an unrecognized outcome return an error after a `cancelErr` check.

  A probe error propagates as an error wrapped in the package's existing `"shedadapters: %s (%s): ..."` shape, and neither archives nor spawns.
  Run a `cancelErr` check around the probe on that error path, the same way the `Run` error path already does.

  Comment the ordering at the call site: archiving is a rename of the very files a live agent may be about to write, and `Wait` polls for bare existence at the spec's paths, so archiving before the probe would make an attached run unable to ever classify `Done` — in exactly the case the probe exists to protect.
  Comment also that the probe is unconditional, gated on neither `spec.Interactive` nor `spec.AwaitOperator`, because respawning over a still-live agent is a correctness bug in autonomous mode too, and gating on interactive would knowingly ship the duplicate-agent path for `Plan-Write`.
  Update `Call`'s own doc comment and `singlellm.go`'s file-header comment to describe the new order.

  Extend `singlellm_test.go`, driving the fake `Shuttle` from card 7 and a temp dir holding pre-existing output files:
  a probe returning `found == false` archives the files to timestamped siblings and calls `Run` (today's path, unchanged);
  a probe returning `found == true` does **not** call `Run` and does **not** archive — proven by asserting the original output-file paths still exist with their original content, not merely that no archive sibling appeared;
  an attached `OutcomeDone` maps to `shedengine.Done` with the first output file as the pointer, an attached `OutcomeDied` and an attached `OutcomeTimeout` each map to a returned error, and an attached `OutcomeAsking` maps to `shedengine.Stuck` — proving the outcome switch is shared rather than duplicated;
  a probe error propagates as an error and neither archives nor spawns;
  context cancellation is still honoured at entry (no probe attempted) and around the probe.
- **Commit:** `fix(shedadapters): probe Attach before archiving so a resume never respawns over a live agent`

### Card 9: `shedadapters` package documentation for the probe

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedadapters/singlellm.go`
  - `internal/shuttleengine/attach.go`
- **Edits:**
  - `internal/shedadapters/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the `# Limitations` section's opening paragraph, which currently reads that `SingleLLMProducer` never reattaches to a live shuttle session and respawns because `shuttleengine` exposes no reattach entry point.
  That statement is now false.
  In its place, state in the `# Outcome mapping` section (or a short new paragraph beside it) that `SingleLLMProducer.Call` probes `shuttleengine`'s `Attach` seam before archiving anything, on every call and regardless of mode; that a found run's `Result` is mapped through the identical outcome switch a spawned run's is; and that a not-found probe falls through to the unchanged archive-then-run path.
  State the ordering rationale — archiving renames the files a live agent is about to write — and the blast radius: the probe applies to `PlanWrite` and the generic `SingleLLM` row too, not only `DiscussionWrite`.
  State what remains a limitation: the `Bouncer` and `BurlerProducer` rows drive the same `Shuttle` seam but do not go through `SingleLLMProducer`, so they keep the respawn-over-a-live-agent path, deliberately, as a scope call rather than a safety claim.
  Leave every other `# Limitations` paragraph — the mid-run cancellation bridge ones and the `Bouncer` ledger carry-forward one — unchanged.
- **Commit:** `docs(shedadapters): replace the never-reattaches limitation with the Attach probe`

## Batch Tests

`verify: go test ./internal/shedadapters/ ./internal/shedrecipe/ ./internal/shedbuild/ ./internal/loomrecipe/` is scoped to the four packages this batch touches, not the repo.
`internal/shedadapters` is where the behaviour change and its new tests live;
the other three are compiled and run because each holds a `shedadapters.Shuttle` test fake that card 7 widens, and a stale fake surfaces as a compile failure in its own package rather than in `shedadapters`.

`internal/loomrecipe` is in scope for a second reason: its `fakeLoomShuttle` serves rows 3 and 6 and all three segments' `Bouncer` rows through one `Env.Shuttle` field, so its whole sequence and resume suite exercises the re-ordered `Call` end to end against a fake whose `Attach` always reports not-found — the regression guard that the ordinary path is unchanged.
Card 7 leaves that fake's `Attach` at the minimal always-not-found shape; batch 5 makes it scriptable for the bounce/attach regression pair.

The overview's module-wide `verify: go vet ./...` catches any other package broken by the seam widening at this boundary.
