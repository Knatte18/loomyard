# Parallel work map

Which `roadmap.md` Planned items can spawn in parallel right now.
Point-in-time — recompute when tasks land or the roadmap changes.
No status banner; delete or regenerate freely.

Constraint is semantic, not file overlap: does task B need something task A creates.
Several items below touch `internal/loomshed/loomshed.go` — expected, resolved at merge time, not a blocker.

## Spawned

- **shedengine: per-producer bounce budget + explicit `OnDone` routing** (`shedengine-segments-bounce-budget`)
- **preflight: split into two Shed rows** (`preflight-loom-agnostic`)
- **scout: extract into its own standalone repo** (`scout-extract-standalone-repo`) — independent of everything else on this map; nothing outside `internal/scoutengine` imports it except `internal/scoutcli`.

## Waiting on shedengine above

Not a hard `depends_on`, but both read on its new `OnDone` field — start early only if the wait is what's inconvenient.

- **shedadapters: Burler-round producer**
- **Bouncer: the generic review-gate producer**

## Blocked by policy — LLM-prompt-content group

`roadmap.md` holds all five back until the flattening group lands, since they're the only items touching LLM-prompt content.

- **loom: Discussion-Write producer** / **loom: Plan-Write producer** — no code dependency, policy only.
- **loom: Discussion-Review producer** / **loom: Plan-Review producer** / **loom: Webster-Review producer** — policy + hard `depends_on` on the flattening trio.

## Blocked on the three review-producer tasks above

- **Bouncer → Perch: rename, retire `internal/perchengine`/`internal/treadleengine`** (Someday) — needs the design proven on all three first.

