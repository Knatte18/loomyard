# Module: Shed recipe — declarative producer lists (DRAFT — concept not yet settled)

> **⚠️ DRAFT. This is an early concept sketch, not a settled design.** Captures a 2026-08-21 discussion's conclusions; expect fields and mechanisms to change before this is implemented. Do not implement from this doc yet.
>
> **Status: Planned, sequenced right after `Retire perch` and before `loom: real LLM producers`.** Not required for `loom`'s existing hardcoded producer list to keep working, but the five remaining `loom` LLM-producer tasks are sequenced to build on this instead of on the Go literal — see `manifest/roadmap.md`'s "Shed recipe" Planned group.

## The idea

`internal/loomshed.New()` builds its 13-row `[]shedengine.ProducerDef` as a Go literal (`loomshed.go:137-151`) — hand-written, hard-coded row order and wiring. This doc explores replacing that literal with a **declarative recipe**: a data file (YAML, format TBD) naming, per row, `{Name, Engine, Config, OnDone, OnStuck, MaxBounces}`, loaded and assembled into the same `[]shedengine.ProducerDef` `shedengine.Shed` already consumes — no change to `shedengine` itself.

Motivation: several rows are already pure `Engine + Config` in spirit — `SingleLLMProducer` differs across `Discussion-Write`/`Plan-Write` only in which prompt stencil and interactivity setting it's given, exactly the shape a declarative recipe expresses cleanly. The question the discussion worked through was whether the *other* rows (the loom-specific ones — `Loom-Preflight`, `DiscussionValidate`, `PlanValidate`) resist this shape, and the answer was no: they don't need to be reusable to fit the pattern, they just need a name in the same registry as everything else — a bespoke, single-consumer `Engine` is exactly as valid a registry entry as a widely-shared one.

## What's in a recipe row

- **`Name`** — the row's identifier, same as today (`shedengine.ProducerDef.Name`).
- **`Engine`** — a name looked up in a registry mapping engine-name strings to `ShedProducer`-constructing functions. **Restricted to names already implementing `shedengine.ShedProducer`** — deliberately not "any Go module": every row today, shared adapter or loom-specific type alike, already satisfies the interface (the Shed Producer-Seam Invariant requires it to sit in a `ProducerDef` at all), so this restriction costs nothing and keeps the recipe-builder simple (registry lookup + fixed-signature constructor call, no reflection, no handling of arbitrary shapes).
- **`Config`** — the row's static, portable configuration (e.g. which rubric stencil, which prompt template). **Never contains absolute paths.** This is what lets one `Engine` (e.g. `SingleLLMProducer`) serve many rows — the row-to-row difference lives entirely in `Config`.
- **`OnDone` / `OnStuck`** — same routing semantics as today's `ProducerDef` fields, unchanged.
- **`MaxBounces`** — same as today's field: a static, per-row integer, set once at recipe-authoring time. **Not runtime-adjustable by any producer, including a Bouncer judging its own segment** — the whole point of the cap is a safety backstop against runaway review-fix cycles; letting a producer raise its own ceiling would undermine that.

**Not in a recipe row: `Segment`.** Leaving every row's `Segment` unset is already a no-op in `shedengine.validate()` today (the check is `segmentByName[p.OnStuck] != p.Segment`, which is always false when every producer's Segment is `""`) — no `shedengine` change needed to drop it. Its old job (catching illegitimate cross-segment `OnStuck` wiring) is superseded by the validity-checker below, which is needed regardless since `OnDone` has never had segment-style enforcement even where `Segment` is used.

## What's never in a recipe

**Geometry** (absolute paths — `AnchorPath`, `WorktreeRoot`, `StencilsDir`, `PlanDir`, etc.) is resolved once, centrally, by whichever caller invokes the recipe-builder — `lyx loom run` for hub mode, a standalone CLI entry point for told mode (mirroring the existing `hubgeom`/`standalonegeom` dual-constructor split) — and merged with each row's `Config` when its `ProducerDef` is constructed. The recipe itself never names a geometry mode and never contains a path; it stays portable across every worktree of the product it describes. This follows directly from the existing Told-Geometry Invariant — nothing new, just extending the same discipline to the recipe layer.

## Pieces to build

Four separable pieces, none blocking `loom`'s remaining work, each independently scoped — see the Someday roadmap items:

1. **Engine registry** — name → constructor mapping for every existing `ShedProducer` type, shared and loom-specific alike.
2. **Recipe loader/builder** — reads the recipe file, resolves `Engine` names via (1), merges `Config` with caller-supplied geometry, assembles `[]shedengine.ProducerDef`.
3. **Shed-setup validity checker** — a standalone tool inspecting an assembled `OnDone`/`OnStuck` graph for blind gates (unreachable rows, unintended cross-wiring) — needed regardless of the recipe work, since neither `Segment` (if used) nor `OnDone` enforce this today.
4. **Convert `internal/loomshed`'s own list** to an actual recipe file using (1)-(3), as the proof the mechanism works — the first real consumer.

## Escalation and the future watchdog

No design impact here worth a dedicated mechanism: `OnStuck: ""` already halts the run and records a reason (`RunBlocked` + `Status.Reason`) — that signal is generic, and a future "watchdog" process (an LLM running outside the `lyx` loop, reacting to `lyx loom`'s own halted/blocked output) would just be another external reader of the same signal a human already reads today. No new field, no contract change.

## Related

- [shed.md](shed.md) — `Shed`'s own generic mechanism (the loop, the producer contract, engine adapters) this recipe layer sits on top of, unchanged.
- [loom.md](loom.md) — `loom`'s concrete producer list, which this would eventually replace the hardcoded form of.
- `CONSTRAINTS.md`'s Told-Geometry Invariant and Shed Producer-Seam Invariant — both directly shape this design's constraints.
