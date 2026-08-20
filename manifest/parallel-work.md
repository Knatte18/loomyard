# Parallel work map

A snapshot of which `roadmap.md` Planned items can be spawned in parallel right now, and which are genuinely blocked.
Recompute this whenever tasks land or the roadmap changes — it is a point-in-time reading of the dependency graph, not durable content, and carries no status banner of its own for that reason (delete or regenerate freely).

## The actual constraint

File overlap is **not** the constraint — several items below touch `internal/loomshed/loomshed.go`, and that is expected, not a blocker: conflicts there get resolved at merge time (`mill-merge-in`), the same way the `loomshed.Deps.Landing` field / `loom-session-bootstrap` overlap flagged earlier this session already is.
The real constraint is semantic: does task B need a type, function, or constructor that task A *creates* to do its own work meaningfully — not just "do they touch the same file."

## Running now

Nothing — `landing: Publish + Finalize producers`, `loom: session bootstrap`, the `fabric-merge-crucible-hardening` crucible campaign (wiki-tracked only, no `roadmap.md` entry), and now `shedengine: per-producer bounce budget + explicit OnDone routing` have all landed. Priority now shifts to the remainder of the "Perch → Shed flattening" group below.

## Can start now, no caveats

- **preflight: split into two Shed rows** — no `depends_on` beyond the Done `loom: phase-machine scaffolding`.

`loom: Discussion-Write producer`/`loom: Plan-Write producer` are deliberately NOT listed here despite having no `depends_on` either: `roadmap.md` groups all five producer tasks (these two plus the three review-producer tasks below) as "the only items in this initiative that touch LLM-prompt content, and stay deliberately last relative to everything above" — a standing policy call, not a code dependency, and this map does not override it. They belong with the rest of that group, see below.

## Can start now, dependency met

`shedengine: per-producer bounce budget + explicit OnDone routing` has landed, so the precondition these two tasks were cleanest waiting on is now satisfied: both read directly on its `OnDone` field (Bouncer's entry-point/exit wiring, Burler's always-`Stuck`-never-`Done` hand-off), not just its `Segment`/`MaxBounces` fields.

- **shedadapters: Burler-round producer**
- **Bouncer: the generic review-gate producer**

## Deliberately last — the LLM-prompt-content group

Not a dependency ordering, a policy one: `roadmap.md` holds all five of `loom`'s content-producer tasks back until everything else above has landed, because they are the only items in this initiative that touch LLM-prompt content.
Within the group, three additionally carry a hard `depends_on` on the "Perch → Shed flattening" trio above — their *wiring* half cannot be built before those exist, only their rubric-writing half could start early if split out further (not done here, each task is scoped as one unit).

- **loom: Discussion-Write producer** — no code `depends_on`, held back by policy only.
- **loom: Plan-Write producer** — same.
- **loom: Discussion-Review producer** — policy + hard `depends_on` on the flattening trio.
- **loom: Plan-Review producer** — same.
- **loom: Webster-Review producer** — same.

## Blocked on the three review-producer tasks finishing

- **Bouncer → Perch: rename, and retire `internal/perchengine`/`internal/treadleengine`** (Someday) — deliberately sequenced last, to avoid "which Perch do you mean" confusion mid-rewrite; needs the design proven on `loom: Discussion-Review producer`/`Plan-Review producer`/`Webster-Review producer` first, not on the two write tasks in the same group above.

## Not yet a wiki task

Scout-extraction-to-its-own-repo (discussed, not yet created) would be independent of everything above if and when it is spawned — no code in this repo currently imports `internal/scoutengine` outside its own package.
