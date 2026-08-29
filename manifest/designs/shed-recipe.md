# Module: Shed recipe — declarative producer lists (all four pieces shipped)

> **All four pieces — the engine registry, the recipe loader/builder, the validity checker, and loom's own conversion to a recipe file — are built and shipped, as `internal/shedrecipe`, `internal/shedbuild`, `internal/shedcheck`, and `internal/loomrecipe` respectively.**
>
> **Status: all four pieces of this group are Done.** See each package's own documentation for as-built detail — `manifest/roadmap.md`'s Done section is cleared regularly, not a durable record.
> This doc survives its pieces landing, which the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle) would otherwise read as grounds for deletion, for the same reason [shed.md](shed.md) does and audited alongside it on 2026-08-29: it is shared narrative the still-unbuilt `loom` is written against.
> [loom.md](loom.md) cites it for the recipe's own row/routing model and [shed.md](shed.md) links it by anchor (`#whats-in-a-recipe-row`) for the `Segment`-field reasoning, both links Markdown Link Integrity enforces, and `internal/shedrecipe/paths.go` cites it for the reject-absolute-paths rationale.
> Retention should be re-evaluated once `loom` lands.

## The idea

`internal/loomrecipe.New()` builds loom's seventeen-row `[]shedengine.ProducerDef` by parsing and building the **declarative recipe** at `contracts/recipes/loom-recipe.yaml` — a data file naming, per row, `{Name, Engine, Config, OnDone, OnStuck, Segment, MaxBounces}`, loaded and assembled into the same `[]shedengine.ProducerDef` `shedengine.Shed` already consumes, with no change to `shedengine` itself.
This replaces the earlier Go literal `internal/loomshed.New()` used to build directly.

Motivation: several rows are already pure `Engine + Config` in spirit, exactly the shape a declarative recipe expresses cleanly — but not `Discussion-Write`, which turns out not to fit that mold.
Its Spec also carries per-run values a static recipe `Config` cannot hold — the task slug and the mode-rules block — plus a model and timeout resolved from the `discussion` role's own config rather than from recipe strings.
That is why it ships as its own `DiscussionWrite` registry entry, over an injected `shedadapters.SpecSource` closure, rather than as a `SingleLLM` row.
`Plan-Write` is the second such row: its Spec is resolved from the `plan` role's own model-spec and timeout rather than from recipe strings, and building it needs a `*lyxcwd.Location` to build the plan paths, so it ships as its own `PlanWrite` entry for the same reason.
The question the discussion worked through was whether the *other* rows (the loom-specific ones — `Loom-Preflight`, `DiscussionValidate`, `PlanValidate`) resist this shape, and the answer was no: they don't need to be reusable to fit the pattern, they just need a name in the same registry as everything else — a bespoke, single-consumer `Engine` is exactly as valid a registry entry as a widely-shared one.

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

A `Config` key may **select** among the seams the told `Env` already carries, by name, without carrying one — this does not break the rule above, it extends it.
A `Bouncer` row's `commit_seam` key takes one of exactly two literal values, `plan` and `discussion`, resolving to `Env.CommitPlan` and `Env.CommitDiscussion` respectively.
Two rules make it safe: an absent key is a legitimate "no seam configured" and leaves the closure nil, while a **present** key naming a closure the `Env` does not carry is a construction error rather than a silent nil — a nil closure would silently mean "commit nothing," the exact condition the key exists to eliminate.
This is the same shape `rubric_stencil` already has, naming a stencil rather than carrying one, so `commit_seam` extends the existing `Env`-versus-`Config` rule rather than forking it.

## Pieces to build

Four separable pieces, each independently scoped, all now shipped:

1. **Engine registry — ✅ built, `internal/shedrecipe`.** Name → constructor mapping for every existing `ShedProducer` type, shared and loom-specific alike.
2. **Recipe loader/builder — ✅ built, `internal/shedbuild`.** Reads the recipe file, resolves `Engine` names via (1), merges `Config` with caller-supplied geometry, assembles `[]shedengine.ProducerDef`.
   See "The recipe loader/builder, shipped" below for the shape it landed in.
3. **Shed-setup validity checker — ✅ built, `internal/shedcheck`.**
   See [shed.md's "Checking an assembled producer list" section](shed.md#checking-an-assembled-producer-list) for the design.
4. **Convert loom's own list to an actual recipe file — ✅ built, `contracts/recipes/loom-recipe.yaml` + `internal/loomrecipe`.**
   The proof the mechanism works, and the first real consumer of (1)-(3).
   See "Decisions this piece settled" below.

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

**Building is not filesystem-free.** Three registry constructors reach disk of their own accord at construction time, producing four distinct effects, and `Build` is a pass-through for those effects rather than a suppressor or a wrapper of them — see `internal/shedbuild/doc.go` for the single site enumerating which constructor produces which effect.

**No on-disk location.** This package defines no on-disk location for recipe files — no directory constant, no filename convention, no embedded default.
Piece 4 settled that decision for loom's own recipe;
see "Decisions this piece settled" below.

## Decisions this piece settled

Three decisions this doc originally deferred, settled by piece 4:

- **On-disk location.** loom's recipe ships as an embedded default at `contracts/recipes/loom-recipe.yaml`, read through `shedbuild.Parse` on the embedded bytes (`contracts/recipes/recipes.go`'s `LoomRecipe`) — never `shedbuild.Load`.
  There is no seeding, no operator override, and no runtime on-disk path.
- **The consumer.** `internal/loomrecipe` is the recipe's sole consumer, sitting above `internal/loomshed` rather than inside it: `internal/shedrecipe`'s registry already imports `loomshed` for eight of its constructors, so a `loomshed` → `shedbuild` → `shedrecipe` → `loomshed` production import cycle would not compile if the consumer lived inside `loomshed` instead.
- **Test ownership.** The assembled-graph tests — the coverage guard driving loom's real row list against the registry, the sequencing/cancellation/resume tests that build the real seventeen-row list — live in `internal/loomrecipe`, not `internal/loomshed`.

**Accepted consequence.** `shedbuild.Load` now has no production caller — loom's only caller reaches its recipe through the embedded bytes, never a told path.
This is deliberate: `Load` stays exported and covered because it is the entry a future non-embedded consumer needs, exactly the shape a second recipe-backed product would use.

## Escalation and the future watchdog

No design impact here worth a dedicated mechanism: `OnStuck: ""` already halts the run and records a reason (`RunBlocked` + `Status.Reason`) — that signal is generic, and a future "watchdog" process (an LLM running outside the `lyx` loop, reacting to `lyx loom`'s own halted/blocked output) would just be another external reader of the same signal a human already reads today. No new field, no contract change.

## Related

- [shed.md](shed.md) — `Shed`'s own generic mechanism (the loop, the producer contract, engine adapters) this recipe layer sits on top of, unchanged.
- [loom.md](loom.md) — `loom`'s concrete producer list, now recipe-backed by this module.
- `CONSTRAINTS.md`'s Told-Geometry Invariant and Shed Producer-Seam Invariant — both directly shape this design's constraints.
