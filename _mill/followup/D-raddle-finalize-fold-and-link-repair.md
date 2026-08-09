```yaml
slug: raddle-finalize-fold-and-link-repair
title: "finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md"
depends_on: ["builder-retire"]
brief: |
  Fold Raddle-regeneration into finalize.md's own producer contract as a first-class part of the merge rather than a Related-section mention, and repair the dead fabric.md links, dead #the-phase-machine anchors, and non-existent Weft Git Invariant citation left across finalize.md, raddle.md and self-report.md — the one follow-up task that stays genuinely parallel to the rest of the set.
```

# finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md

## Why

The landed model folds Raddle-regeneration into `Finalize`'s own contract, rather than keeping it as a step of its own.
`finalize.md` and `raddle.md` still describe a machine that no longer exists.
The same files also carry three separate dead references, which is what makes this a repair task rather than a prose pass: a link to a design doc that does not exist, a renamed anchor that no longer resolves in three places, and a citation to a `CONSTRAINTS.md` invariant that was never named that.

## What needs to happen

1. Fold Raddle into `finalize.md`'s own contract as a first-class part of the merge, not a Related-section mention.
2. Remove `raddle.md`'s superseded "reserved phase slot between Builder and Finalize" text at lines 3 and 85, and close its explicitly-open question at line 54 — the fold is decided.
3. Fix `finalize.md:3`'s verbatim two-slot text ("not a swappable per-instance slot the way Preflight and the producer are").
4. Fix `finalize.md:11` and `:52`, which link `fabric.md` — a file that does not exist in `manifest/designs/`.
5. Fix the dead `loom.md#the-phase-machine` anchor, renamed to `#the-phase-machine--a-flat-producer-list-no-predefined-slots`, in `raddle.md:3`, `raddle.md:54`, and `self-report.md:30`.
6. Fix `finalize.md:26`, which cites a "Weft Git Invariant" in `CONSTRAINTS.md` that does not exist — the real entry is the Fabric Git Invariant (warp + weft) at `CONSTRAINTS.md:173`.

**This task re-reads `finalize.md` end to end**, rather than working the fixed line list above — the line numbers are a starting inventory, not a bound.
Known additional residue the discussion already found:

- `:45–46` still calls Finalize "`Shed`'s literally-shared code ... both share this exact code", which is the retired shared-code framing.
- `:48` asserts "`Shed` hasn't been extracted from it yet (see that doc's own naming note)", which is false once task E fixes `loom.md:15–17`.
- `:9` references Builder's escalation behavior, which task A retires.

## Finalize is shared by reference

**`finalize-shared-by-reference`.**
`Finalize` is shared **by reference** — both `loom`'s and `Hardener`'s lists name the same producer definition: one definition, named twice, never copied.
This is the framing the fold is written against.
`shed.md:18`'s "by value" wording is task E's to fix, not this task's, so the two tasks do not both edit `shed.md`.

## Scope

This task deliberately does not touch `manifest/roadmap.md`.
`roadmap.md:68`'s "deferred phase slot between Builder and Finalize" is real residue, but `roadmap.md` is edited by task A and task E too, so scoping it to this task would recreate exactly the shared-file collision that forced task E to be serialized.
It moves to task E, `roadmap.md`'s last owner.

This task owns `finalize.md`, `raddle.md`, and `self-report.md`, and no other task in the set touches any of them — that is what makes this task genuinely parallel rather than parallel-by-assertion.

**Deferred, on record so it does not read as an oversight.**
Per the discussion's surfaced open questions, item 4: `Hardener` and `Tenter`'s equivalent Raddle-into-Finalize fold is deferred by the landed design at `shed.md:20` and `loom.md:67`, and stays deferred.
This task does not design it.

## Sequencing

`depends_on: builder-retire` — `finalize.md:36` and `:50`'s link targets move in task A.

This task branches off task A in parallel with the B → {C, F} → E chain; it does not block, and is not blocked by, any of C, E, or F.

## Acceptance

Docs-only.
The one mechanical check worth running is that every relative markdown link and anchor introduced or touched resolves — a link-check pass over `manifest/` and `docs/` is the acceptance criterion.
It is exactly what would have caught the dead `fabric.md` links, the dead phase-machine anchors, and the non-existent Weft Git Invariant citation before they shipped.
