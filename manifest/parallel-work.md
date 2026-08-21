# Parallel work map

Which `roadmap.md` Planned items can spawn in parallel right now.
Point-in-time — recompute when tasks land or the roadmap changes.
No status banner; delete or regenerate freely.

Constraint is semantic, not file overlap: does task B need something task A creates.
Several items below touch `contracts/recipes/loom-recipe.yaml` — expected, resolved at merge time, not a blocker.

## Spawned

- **shedengine: per-producer bounce budget + explicit `OnDone` routing** (`shedengine-segments-bounce-budget`)
- **preflight: split into two Shed rows** (`preflight-loom-agnostic`)
- **scout: extract into its own standalone repo** (`scout-extract-standalone-repo`) — independent of everything else on this map; nothing outside `internal/scoutengine` imports it except `internal/scoutcli`.

Nothing — `landing: Publish + Finalize producers`, `loom: session bootstrap`, the `fabric-merge-crucible-hardening` crucible campaign (wiki-tracked only, no `roadmap.md` entry), and now `shedengine: per-producer bounce budget + explicit OnDone routing` have all landed. Priority now shifts to the remainder of the "Shed flattening" group below.

## Can start now, no caveats

- **preflight: split into two Shed rows** — no `depends_on` beyond the Done `loom: phase-machine scaffolding`.

`loom: Discussion-Write producer`/`loom: Plan-Write producer` are deliberately NOT listed here despite having no `depends_on` either: `roadmap.md` groups all five producer tasks (these two plus the three review-producer tasks below) as "the only items in this initiative that touch LLM-prompt content, and stay deliberately last relative to everything above" — a standing policy call, not a code dependency, and this map does not override it. They belong with the rest of that group, see below.

## Can start now, dependency met

`shedengine: per-producer bounce budget + explicit OnDone routing` has landed, so the precondition these two tasks were cleanest waiting on is now satisfied: both read directly on its `OnDone` field (Bouncer's entry-point/exit wiring, Burler's always-`Stuck`-never-`Done` hand-off), not just its `Segment`/`MaxBounces` fields.

- **shedadapters: Burler-round producer**
- **Bouncer: the generic review-gate producer**

## Blocked by policy — LLM-prompt-content group

`roadmap.md` holds all five back until the flattening group lands, since they're the only items touching LLM-prompt content.

- **loom: Discussion-Write producer** / **loom: Plan-Write producer** — no code dependency, policy only.
- **loom: Discussion-Review producer** / **loom: Plan-Review producer** / **loom: Webster-Review producer** — policy + hard `depends_on` on the flattening trio.

## Blocked on the three review-producer tasks above

- **Retire `internal/treadleengine`** (Someday) — deliberately sequenced last; treadle's own round-loop consumer is already gone (see the roadmap's Done retirement entry), and treadle's own final call needs the `Shed`-segment design proven on `loom: Discussion-Review producer`/`Plan-Review producer`/`Webster-Review producer`, and on `Tenter`, first.
