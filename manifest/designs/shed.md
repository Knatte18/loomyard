# Shed — a shared Go outer phase-FSM for `loom` and `Hardener`

> **Status: Design sketch, Planned** (after `Treadle`, before the `perch` rewrite — see `manifest/roadmap.md`). Naming: a loom's shed is the gap formed between warp threads for the shuttle to pass through — apt for the generic engine that opens a slot for whichever producer list a product configures it with. Pairs naturally with the shipped `shuttle` (the thing that passes through it). This doc is the authoritative description of `Shed`'s own generic mechanism (the flat producer list, the contract/definition split, engine adapters); [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it, plus the engine-level detail (crash recovery, pause, session bootstrap) this doc doesn't restate.

## What it is

**Revised model (2026-08-08), superseding an earlier "two swappable slots" description:** `Shed` has no predefined slots at all — no Preflight-slot, no Producer-slot, no shared Finalize.
It is a generic engine that walks one ordered, flat list of **producers**, each an atomic mechanical action or LLM session, honoring resume/crash-recovery/pause uniformly across every entry.
Everything that used to look "special" — Preflight, Finalize, review gates — is just a producer like any other in that list.
What makes `loom` "loom" versus `Hardener` is purely which producers are in the list, in what order: pure configuration, not architecture.
See [loom.md's own producer-list table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for `loom`'s concrete list — this doc stays about `Shed`'s own generic mechanism, not about enumerating `loom`'s specific producers.

- **`loom`** = `Shed` + `loom`'s own producer list (`Preflight`, `Discussion-Write`, `Discussion-Validate`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Validate`, `Plan-Review`, `Batchifier`, `Webster`, `Webster-Review`, `Finalize`) — unchanged behavior/CLI from the outside, `lyx loom run`.
- **`Hardener`** = `Shed` + Hardener's own list (its own `Preflight`, `Tenter` — `Treadle` + a live-substrate round-runner + behavior-review profile, see the `internal/treadleengine` package documentation — and its own `Finalize`) — `lyx hardener run`.
  Someday, deprioritized;
  not part of this doc's Planned scope.

`Finalize` is not `Shed`'s own special code — it is an ordinary producer both `loom` and `Hardener` happen to reference (by *reference* — the same producer definition named in both lists, so a change to `Finalize`'s definition is visible to both without either copying it), not something `Shed` special-cases.
**Raddle folds into `Finalize`'s own contract**, not a separate producer or a separate slot: updating Raddle before the Finalize merge is impractical given merge-conflict risk, so Raddle-regeneration is scoped as part of the merge itself (resolves the open question an earlier draft left open).
`Hardener`'s `Tenter` will need the equivalent fold eventually — not designed here.

### Producer contract vs. producer definition

A producer's **contract** — the only thing any other producer or instruction file may reference — is exactly two parts: **Input** (a pointer to the format-contract file defining consumed artifact(s)' shape, never a restated copy) and **Output** (same pointer discipline).
**The pointer rule**: an instruction file (a producer's own prompt/skill) must never duplicate or paraphrase another producer's format-contract content, only point at it — so editing that one format file alone is sufficient to change what both its producer and its consumers do.
Review is never a property attached to the producer it reviews; it is always the next, separate producer in the list.

A producer's **definition** — internal to how `Shed` actually runs it, invisible to the contract — additionally names an **engine** (which code drives it) and a **config** (how that engine is parameterized for this specific producer).
Many producers share the same engine: every `*-Review` producer is `engine: perch`, differing only by which rubric/fasit `config` file is handed to it — the same generic, profile-driven mechanism `perch` already implements today ("reused for every phase... only the review profile differs").

### Engine adapters — a thin, shared seam, not one per producer

`Shed` needs a minimal common interface to drive any producer uniformly — call it, get back an outcome (done / approved / stuck / blocked) plus an output-artifact pointer, without needing to know what happened inside.
This is not a new pattern: it mirrors two seams that already exist in this codebase —

- `internal/treadleengine`'s `RoundRunner` seam (`internal/perchengine`'s burler adapter is its reference consumer).
- `internal/batcher`'s `Batcher` interface (multiple batchifier implementations behind one interface, resolved by name via `Select`).

Applied one level up, `Shed` needs a small `ProducerRunner`-shaped seam, and — critically — **not one adapter per producer, one per distinct engine type**:

- **Mechanical Go-function producers** (`Preflight`, `Discussion-Validate`, `Plan-Sweep`, `Plan-Validate`, `Batchifier`, `Finalize`) need no translation adapter at all — a plain Go function can already satisfy the seam directly.
- **Single LLM-spawn producers** (`Discussion-Write`, `Plan-Write`) are already unified today via the shared `shuttleengine.Spec` → `shuttle.Run` pattern (`DiscussionSpec`/`PlanSpec` in `internal/loomengine`) — effectively free.
- **`perch`** needs one adapter, reused by every `*-Review` producer.
- **`Webster`** needs one adapter (its own verb-driven black-box form differs from `perch`'s).

So building this is **two new adapters**, not eleven, and not one-per-engine-type-times-producer-count.

## Testable cheaply — a throwaway producer list proves the skeleton

Building `Shed`'s skeleton doesn't need a real producer list to validate against.
Plug in a short, disposable list — a couple of steps that just succeed immediately, including a stub `Finalize` — and sequencing, resume, crash-recovery, and pause can all be exercised end-to-end without any of `loom`'s or `Hardener`'s real producers needing to exist yet.
This mirrors `loom.md`'s own stated approach ("testable against fake phases before real producers are wired in... the same fake-tested approach `perch` used against a fake `burler`") — reused here to validate the *extraction*, the same way `perch`'s existing behavior validates `Treadle`'s extraction.

## Process — one task, not many

`Shed`'s skeleton, its two adapters (`perch`, `Webster`), and the `Finalize` producer are **one Planned task** — `Finalize` is a producer definition like any other now, not a second, separately-scoped piece of shared code, so there is no reason to split it out.
Same reasoning as the combined `Treadle` + `perch`-rewrite task.

## Why this doc doesn't rewrite loom.md's full detail

`loom.md` is a mature, ~320-line, detailed design (crash recovery, pause semantics, session bootstrap, module decomposition) — this doc's own core model section is now the authoritative description of `Shed`'s generic mechanism (producers, contracts, engine adapters), and `loom.md`'s own [phase-machine section](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it.
What this doc does *not* redo is `loom.md`'s remaining detail sections (crash recovery mechanics, pause, session bootstrap, module decomposition) — those stay in `loom.md`, described in `loom`-specific terms, and apply to `Shed`-based products generically without needing restating here.

## Related

- [loom.md](loom.md) — `loom`'s concrete producer list built on `Shed`, plus the remaining engine detail (crash recovery, pause, session bootstrap) this doc doesn't restate.
- [finalize.md](finalize.md) — `Finalize`'s own contract in detail; here it is one producer definition among others, not special-cased.
- [raddle.md](raddle.md) — the merge-time regeneration decision and merge-lock scope `Finalize`'s own contract must honor, now that Raddle folds into it rather than keeping a separate slot.
- `internal/treadleengine` package documentation — the sibling generic engine (inner round-loop, not outer phase-FSM), and the precedent for `Shed`'s own engine-adapter seam (`RoundRunner`).
- `internal/batcher` package documentation — the other existing precedent for the engine-adapter pattern (`Batcher` interface).
- [hardener.md](hardener.md) — `Hardener` (`Shed` + Hardener's own producer list), Someday.
