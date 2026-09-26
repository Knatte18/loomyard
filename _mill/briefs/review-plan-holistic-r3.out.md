MILL_REVIEW_BEGIN
# Review: Loom persists done only after post-run friction reflection — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-09-26
```

Verified against source across both batches: `internal/loomshed` (loomshed.go, doc.go, stub.go, ctx.go, seam_enforcement_test.go, interruptpolicy.go), `internal/shedrecipe` (recipe.go, entries_simple.go, registry.go, registry_test.go, fixture_test.go, entries_simple_test.go, env.go, config.go, coverage_guard_test.go), `internal/shedbuild` (fixture_test.go, build_engines_test.go), `internal/loomrecipe` (shape_test.go, coverage_guard_test.go, interruptpolicy_meta_test.go, recipe_test.go, fixture_test.go, resume_test.go, loomrecipe.go), `internal/shedengine` (producer.go, shed.go, run.go, status.go, validate.go), `internal/loomcli` (cli.go, arm.go, run.go, wiring.go, bootstrap.go, start.go, friction_test.go, wiring_test.go, bootstrap_test.go, sharedbootstrap_test.go), `internal/loomengine/config.go`, `internal/lock/lock.go`, `internal/frictionengine/deps.go`, `contracts/recipes/loom-recipe.yaml`, `docs/overview.md`, `contracts/specs/shed-recipe-spec.md`, `plugins/ly/skills/ly-drive/SKILL.md`.

Traced the row-ordering mechanics directly against `shedengine`'s `stepLocked`/`Run`/`persist` (run.go) and `validate()` (validate.go): card 4's four new tests (`TestFrictionReflect_DonePersistsOnlyAfterReflection`, `PauseDuringFinalizeHaltsAtTheRow`, `ResumeAtTheRowCallsOnlyTheRow`, `PreChangeDoneAtFinalizeShortCircuits`) match the engine's actual Done/pause/resume/already-done-short-circuit semantics exactly, including the two-iteration persist sequence that lets the reflect closure observe `current_producer: Friction-Reflect`/`state: running` before it runs.

Traced card 6's `TestReflectFrictionRow_WaitsOnAHeldReflectionLock` against `gofrs/flock` semantics in `internal/lock/lock.go`: `AcquireWriteLock`'s blocking form genuinely contends against a same-process `TryAcquireWriteLock` holder on the same path (separate `flock.Flock`/fd per call), so the described block-then-release sequence is real, not a false positive.

Confirmed the row-count sweep (Shared Decision `no-perishable-row-counts`) is complete: grepped the whole tree for `fourteen|sixteen|seventeen|fifteen` and `last row|final row`; every hit outside the plan's own listed files is unrelated (landingshed's fifteen `Deps` fields, planparser's seventeen format-4 checks, fabricengine's fifteen structural states/files, `seedinput.go`'s "final row" of an unrelated decision table). No missed site.

Confirmed the Driver Choice Single-Site Invariant holds: `armedVerb` gates on the verb the operator/driver typed for *this* invocation, never on `shedrun.Seed.Driver`; card 6 explicitly requires the existing AST tripwire (`bootstrap_test.go`) to stay green with no new carve-out, and nothing in cards 5–6 reads the seed's `Driver` field.

Confirmed `All Files Touched` is the exact union of every card's `Edits`/`Creates` across both batches, `Batch Index` DAG is acyclic and file-complete, global step numbering (1–6) has no gaps, and no card carries a non-empty `Moves:` (so no Rename mechanic section is required).

No BLOCKING or NIT findings.

## Verdict

APPROVE
Mechanically verified against source; row-count sweep, lock/engine concurrency claims, and invariant compliance all check out.
MILL_REVIEW_END
