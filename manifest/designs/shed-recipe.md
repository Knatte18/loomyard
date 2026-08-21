# Module: Shed recipe — declarative producer lists (pieces 1-3 shipped; piece 4 planned)

> **Pieces 1-3 — the engine registry, the recipe loader/builder, and the validity checker — are built and shipped as `internal/shedrecipe`, `internal/shedbuild`, and `internal/shedcheck`.**
> Piece 4 — converting `loom`'s own producer list to an actual recipe file — remains an early concept sketch, not a settled design: expect fields and mechanisms to change before it is implemented, and do not implement piece 4 from this doc as written.
>
> **Status: the group's remaining work is the conversion item, sequenced right after `Retire perch` and before `loom: real LLM producers`.** Not required for `loom`'s existing hardcoded producer list to keep working, but the five remaining `loom` LLM-producer tasks are sequenced to build on this instead of on the Go literal — see `manifest/roadmap.md`'s "Shed recipe" Planned group.

## The idea

`internal/loomshed.New()` builds its 13-row `[]shedengine.ProducerDef` as a Go literal (`loomshed.go:137-151`) — hand-written, hard-coded row order and wiring. This doc explores replacing that literal with a **declarative recipe**: a data file (YAML, format TBD) naming, per row, `{Name, Engine, Config, OnDone, OnStuck, MaxBounces}`, loaded and assembled into the same `[]shedengine.ProducerDef` `shedengine.Shed` already consumes — no change to `shedengine` itself.

Motivation: several rows are already pure `Engine + Config` in spirit — `SingleLLMProducer` differs across `Discussion-Write`/`Plan-Write` only in which prompt stencil and interactivity setting it's given, exactly the shape a declarative recipe expresses cleanly. The question the discussion worked through was whether the *other* rows (the loom-specific ones — `Loom-Preflight`, `DiscussionValidate`, `PlanValidate`) resist this shape, and the answer was no: they don't need to be reusable to fit the pattern, they just need a name in the same registry as everything else — a bespoke, single-consumer `Engine` is exactly as valid a registry entry as a widely-shared one.

## What's in a recipe row

- **`Name`** — the row's identifier, same as today (`shedengine.ProducerDef.Name`).
- **`Engine`** — a name looked up in a registry mapping engine-name strings to `ShedProducer`-constructing functions. **Restricted to names already implementing `shedengine.ShedProducer`** — deliberately not "any Go module": every row today, shared adapter or loom-specific type alike, already satisfies the interface (the Shed Producer-Seam Invariant requires it to sit in a `ProducerDef` at all), so this restriction costs nothing and keeps the recipe-builder simple (registry lookup + fixed-signature constructor call, no reflection, no handling of arbitrary shapes).
- **`Config`** — the row's static, portable configuration (e.g. which rubric stencil, which prompt template). **Never contains absolute paths.** This is what lets one `Engine` (e.g. `SingleLLMProducer`) serve many rows — the row-to-row difference lives entirely in `Config`.
- **`OnDone` / `OnStuck`** — same routing semantics as today's `ProducerDef` fields, unchanged.
- **`MaxBounces`** — same as today's field: a static, per-row integer, set once at recipe-authoring time. **Not runtime-adjustable by any producer, including a Bouncer judging its own segment** — the whole point of the cap is a safety backstop against runaway review-fix cycles; letting a producer raise its own ceiling would undermine that.

**A recipe row does carry `Segment`.** This reverses the plan this section originally argued for, and it is worth recording why. The old argument was that leaving every row's `Segment` unset is already a no-op in `shedengine.validate()` (the check is `segmentByName[p.OnStuck] != p.Segment`, which is always false when every producer's Segment is `""`), so a recipe could just omit the field. That premise holds only while every row leaves `Segment` unset — and three already-planned roadmap items (the three review-producer tasks in `loom: real LLM producers`) each specify a producer pair sharing one segment name, so the premise breaks the moment any of them lands. Worse, `shedengine.validate()`'s own rule enforces that a non-empty `OnStuck` names a producer sharing the bouncing row's `Segment`, so a producer list mixing recipe rows left at the empty default with hand-wired rows at a real segment name would fail validation at run time rather than at authoring time. So the shipped `Row` carries an optional `Segment`, mapped straight onto `shedengine.ProducerDef.Segment`. What survives from the old argument is its other half: `Segment`'s old job — catching illegitimate cross-segment `OnStuck` wiring — is superseded by the validity checker below, which is needed regardless since `OnDone` has never had segment-style enforcement even where `Segment` is used. Only the prediction that the field itself departs was wrong.

## What's never in a recipe

**Geometry** (absolute paths — `AnchorPath`, `WorktreeRoot`, `StencilsDir`, `PlanDir`, etc.) is resolved once, centrally, by whichever caller invokes the recipe-builder — `lyx loom run` for hub mode, a standalone CLI entry point for told mode (mirroring the existing `hubgeom`/`standalonegeom` dual-constructor split) — and merged with each row's `Config` when its `ProducerDef` is constructed. The recipe itself never names a geometry mode and never contains a path; it stays portable across every worktree of the product it describes. This follows directly from the existing Told-Geometry Invariant — nothing new, just extending the same discipline to the recipe layer.

**Live seams**, alongside geometry, are the other thing never in a recipe: `shedadapters.Shuttle`, `shedadapters.BurlerRunner`, `shedadapters.WebsterRunner`, `websterengine.RunDeps`, `landingshed.Deps`'s closures and its `modelspec.Registry`, and the injected clock.
None of these can be written in a file at all — a recipe row names an `Engine` and carries `Config`, and a seam is neither.
They travel in the same told bundle as geometry, `shedrecipe.Env`, filled once by whichever caller invokes the registry — this extends the existing discipline rather than inventing a second one.
The rule that makes the `Env`-versus-`Config` split decidable: `Env` holds roots and run-wide values only, never a value that differs between two rows, and anything per-row is a relative path or scalar in `Config`, resolved against one of those roots by the entry that reads it.

## Pieces to build

Four separable pieces, none blocking `loom`'s remaining work, each independently scoped — see the Someday roadmap items:

1. **Engine registry — ✅ built, `internal/shedrecipe`.** Name → constructor mapping for every existing `ShedProducer` type, shared and loom-specific alike.
2. **Recipe loader/builder — ✅ built, `internal/shedbuild`.** Reads the recipe file, resolves `Engine` names via (1), merges `Config` with caller-supplied geometry, assembles `[]shedengine.ProducerDef`.
   See "The recipe loader/builder, shipped" below for the shape it landed in.
3. **Shed-setup validity checker** — built and independent of the recipe work: `internal/shedcheck` ships this piece already, ahead of the other three.
   See [shed.md's "Checking an assembled producer list" section](shed.md#checking-an-assembled-producer-list) for the design.
4. **Convert `internal/loomshed`'s own list** to an actual recipe file using (1)-(3), as the proof the mechanism works — the first real consumer.

## The recipe loader/builder, shipped

`internal/shedbuild` is the recipe file format's loader and builder, shipped as a single package.
Its production import set is one-way: it imports `internal/shedrecipe`, `internal/shedengine`, and `internal/shedcheck`, and none of those three, nor `internal/loomshed` or `internal/loomcli`, imports it back.

**Document shape.** A recipe document decodes straight into the package's own `Recipe` type, with no intermediate struct.
It requires a `version` (currently only `1` is accepted), a told `entry` producer name, a told `terminals` list, and a `producers` list.
Each row carries `name`, `engine`, `config`, `on_done`, `on_stuck`, `segment`, and `max_bounces` — the same field set `shedengine.ProducerDef` itself carries, `segment` included per the reversal recorded above.

**Four exported functions.** `Parse` decodes a byte slice into a `Recipe` and runs every shape check the value must pass before `Build` may consume it.
`Load` reads the recipe file at a told absolute path and delegates to `Parse` — it is the only function in the package whose own code touches the filesystem.
`Build` resolves each row's `Engine` name against the `internal/shedrecipe` registry, calls the returned constructor with a caller-supplied `shedrecipe.Env`, and assembles the `[]shedengine.ProducerDef` list `shedengine.Shed` already consumes unchanged.
`Check` is a thin, non-production forward of an assembled producer list plus a `Recipe`'s own `Entry` and `Terminals` into `shedcheck.Check`, for a caller's own authoring-time test suite.

**Strictness.** Unknown keys and duplicate keys are errors at both document level and row level, and every message the decoder produces keeps its own yaml line number.
Every error this package raises itself, after a successful decode, names the offending row's zero-based list index and its `name`.

**Validation split.** This package owns file shape and engine-name resolution alone.
It runs no reachability, cycle, blind-gate, dangling-target, or segment analysis of its own, because `shedengine`'s own validation and `internal/shedcheck` already own routing, cycles, and reachability, and a third copy is what drifts.

**Building is not filesystem-free.** Four registry constructors reach disk of their own accord at construction time, and `Build` is a pass-through for those effects rather than a suppressor or a wrapper of them — see `internal/shedbuild/doc.go` for the single site enumerating which constructor produces which effect.

**No on-disk location.** This package defines no on-disk location for recipe files — no directory constant, no filename convention, no embedded default. That remains piece 4's decision.

## Escalation and the future watchdog

No design impact here worth a dedicated mechanism: `OnStuck: ""` already halts the run and records a reason (`RunBlocked` + `Status.Reason`) — that signal is generic, and a future "watchdog" process (an LLM running outside the `lyx` loop, reacting to `lyx loom`'s own halted/blocked output) would just be another external reader of the same signal a human already reads today. No new field, no contract change.

## Related

- [shed.md](shed.md) — `Shed`'s own generic mechanism (the loop, the producer contract, engine adapters) this recipe layer sits on top of, unchanged.
- [loom.md](loom.md) — `loom`'s concrete producer list, which this would eventually replace the hardcoded form of.
- `CONSTRAINTS.md`'s Told-Geometry Invariant and Shed Producer-Seam Invariant — both directly shape this design's constraints.
