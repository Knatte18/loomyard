# Shed — a shared Go outer phase-FSM for `loom` and `Hardener`

> **Status: Design sketch, Planned** (after `Treadle`, before the `perch` rewrite — see
> `manifest/roadmap.md`). Naming: a loom's shed is the gap formed between warp threads for the
> shuttle to pass through — apt for the generic slot this engine opens for a swappable producer.
> Pairs naturally with the already-shipped `shuttle` (the thing that passes through it). This doc
> does **not** re-derive [loom.md](loom.md)'s already-detailed phase machine — that content stays
> the authoritative design for now; extracting the generic engine out from under it is real,
> separate refactor work, not done in this pass.

## What it is

`Shed` generalizes the outer phase-sequencing engine loom.md already specifies in full —
sequencing, resume, crash-recovery, pause, the status-file contract, session bootstrap — into a
shared skeleton with two swappable slots, not one:

- **Preflight-slot** — validates preconditions before anything runs. Producer-specific: `loom`'s
  Preflight checks loom's own preconditions (already built, `internal/loomengine.Preflight` — see
  [loom.md](loom.md#the-phase-machine)); `Hardener` needs its own, different Preflight (sandbox
  provisioning, live-suite readiness — see [hardener.md](hardener.md)). Not shared code.
- **Producer-slot** — the phase(s) that actually do the work. For `loom`, this is
  Discussion → Plan → Webster (each gated by a `perch` review — see [loom.md](loom.md#the-gate));
  for `Hardener`, this is `Tenter` (see [treadle.md](treadle.md)).

What **is** literally shared code (not swappable, identical either way): the sequencing skeleton
itself (resume-on-files, crash recovery, graceful pause — all already specified in
[loom.md](loom.md#state--contracts)), the Raddle-regeneration trigger and merge-lock scope (see
[raddle.md](raddle.md#when-it-runs-deferred-to-merge-time-not-mid-task) — open question there on
whether Raddle collapses into Finalize/Merge or keeps a separate slot), and Finalize/Merge (see
[loom-finalize.md](loom-finalize.md)) — including the warp-side real-git-conflict path and the
weft-side document-driven (non-git) conflict path for `_raddle`/`_pattern` content.

Two named products come from configuring the same engine differently:

- **`loom`** = `Shed` + loom's own Preflight + Discussion/Plan/Webster producer — unchanged
  behavior/CLI from the outside, `lyx loom run`.
- **`Hardener`** = `Shed` + Hardener's own Preflight + `Tenter` producer (`Treadle` + a
  live-substrate round-runner + behavior-review profile — see [treadle.md](treadle.md)) —
  `lyx hardener run`. Someday, deprioritized; not part of this doc's Planned scope.

## Why this doc doesn't rewrite loom.md

`loom.md` is a mature, ~320-line, already-detailed design (phase machine, crash recovery, pause
semantics, session bootstrap, module decomposition) written before this generalization was
conceived. Retrofitting it into "Shed does X, `loom` configures X" throughout is real work with
real risk of quietly losing detail — the same discipline [treadle.md](treadle.md#process--do-not-fold-this-into-hardeners-task)
already applies to perch's own extraction. Not folded into this pass. For now, `loom.md` remains
the authoritative reference for what the engine actually does; this doc only records that the
engine is being named `Shed` and is intended to be reused by `Hardener`.

## Related

- [loom.md](loom.md) — the authoritative, already-detailed design this doc generalizes a name over,
  not yet rewritten to extract `Shed` explicitly.
- [loom-finalize.md](loom-finalize.md) — the Finalize/Merge phase `Shed` shares as literal code.
- [raddle.md](raddle.md) — the merge-time regeneration decision and merge-lock scope `Shed`'s
  Finalize/Merge step must honor.
- [treadle.md](treadle.md) — the sibling generic engine (inner round-loop, not outer phase-FSM).
- [hardener.md](hardener.md) — `Hardener` (`Shed` + `Tenter`), Someday.
