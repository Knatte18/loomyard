# Shed — a shared Go outer phase-FSM for `loom` and `Hardener`

> **Status: Design sketch, Planned** (after `Treadle`, before the `perch` rewrite — see `manifest/roadmap.md`). Naming: a loom's shed is the gap formed between warp threads for the shuttle to pass through — apt for the generic engine that opens a slot for whichever producer list a product configures it with. Pairs naturally with the shipped `shuttle` (the thing that passes through it). This doc is the authoritative description of `Shed`'s own generic mechanism (the flat producer list, the contract/definition split, engine adapters); [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it, plus the engine-level detail (crash recovery, pause, session bootstrap) this doc doesn't restate.

## What it is

**Revised model (2026-08-08), superseding an earlier "two swappable slots" description:** `Shed` has no predefined slots at all — no Preflight-slot, no Producer-slot, no shared Finalize.
It is a generic engine that walks one ordered, flat list of **producers**, honoring resume/crash-recovery/pause uniformly across every entry;
atomicity — one mechanical action or LLM session, never an internal multi-step process of its own — binds **simple** producers only, per the carve-out in [Producer contract vs. producer definition](#producer-contract-vs-producer-definition) below.
Everything that used to look "special" — Preflight, Finalize, review gates — is just a producer like any other in that list.
What makes `loom` "loom" versus `Hardener` is purely which producers are in the list, in what order: pure configuration, not architecture.
See [loom.md's own producer-list table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for `loom`'s concrete list — this doc stays about `Shed`'s own generic mechanism, not about enumerating `loom`'s specific producers.

- **`loom`** = `Shed` + `loom`'s own producer list — unchanged behavior/CLI from the outside, `lyx loom run`. See [loom.md's own producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for the concrete list; this doc never repeats it, so a producer added, removed, or reclassified there needs no edit here.
- **`Hardener`** = `Shed` + Hardener's own list (its own `Preflight`, `Tenter` — `Treadle` + a live-substrate round-runner + behavior-review profile, see the `internal/treadleengine` package documentation — and its own `Finalize`) — `lyx hardener run`.
  Someday, deprioritized;
  not part of this doc's Planned scope.

`Finalize` is not `Shed`'s own special code — it is an ordinary producer both `loom` and `Hardener` happen to reference (by *reference* — the same producer definition named in both lists, so a change to `Finalize`'s definition is visible to both without either copying it), not something `Shed` special-cases.
**Raddle folds into `Finalize`'s own contract**, not a separate producer or a separate slot: updating Raddle before the Finalize merge is impractical given merge-conflict risk, so Raddle-regeneration is scoped as part of the merge itself (resolves the open question an earlier draft left open).
`Hardener`'s `Tenter` will need the equivalent fold eventually — not designed here.

### Producer contract vs. producer definition

A producer's **contract** — the only thing any other producer or instruction file may reference — is exactly two parts: **Input** (a pointer to the format-contract file defining consumed artifact(s)' shape, never a restated copy) and **Output** (same pointer discipline).
**Thin-Input carve-out:** the Input contract permits **no Input at all** for a chain-head producer, because its input is human intent expressed in an interactive session rather than an artifact with a format contract.
A producer with no Input has nothing to re-read on resume, so a crashed chain-head producer re-runs from its own partial output plus fresh human input — correct, since the human is present at that boundary by definition.
This explicitly **rejects** the alternative framing that the task record is the Input, making the pointer target a task record rather than a format-contract file: that is a mill-ism which does not transfer, since `lyx` has no wiki and no task record.
Admitting a second kind of pointer target would weaken the pointer rule for a target that does not exist in `lyx`.
**Thin-Output carve-out, stated as two cases, never one:** first, a **gate producer** genuinely emits nothing at all — the Output contract permits a bare pass/fail gate signal with no artifact, and the resume-on-output-files rule degrades gracefully, because a producer with no artifact simply re-runs on resume, which is correct since a gate is a cheap idempotent re-check.
Second, a **terminal producer** (the last in the list) is a different case and must not be folded into the first: it may plainly have effects, and what it has no instance of is a **contract-level output artifact** — nothing downstream consumes its output through a format pointer, because nothing runs after it.
Its thin Output is therefore "no *pointer target*", not "no effect", and its resume story is not the graceful degradation above, since a partially-completed terminal effect is not a cheap idempotent re-run;
that recovery is the terminal producer's own obligation and is explicitly not designed here.
**The pointer rule**: an instruction file (a producer's own prompt/skill) must never duplicate or paraphrase another producer's format-contract content, only point at it — so editing that one format file alone is sufficient to change what both its producer and its consumers do.
Review is never a property attached to the producer it reviews; it is always the next, separate producer in the list.

See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for which of `loom`'s concrete producers falls into each carve-out.

Producers split into two kinds, and the atomicity rule stated above is scoped to the first:

- A **simple, single-agent-spawn producer** is one mechanical action or one LLM session.
  This kind does not typically need its own crash-recovery, since re-running one spawn from scratch is cheap.
  A single-LLM-spawn instance of this kind is a `SingleLLMProducer` — see [Engine adapters](#engine-adapters--a-thin-shared-seam-not-one-per-producer) below.
- A **bespoke, multi-spawn producer** owns its own internal loop — many LLM spawns, or an agent orchestrating sub-agents.
  Bespoke producers are **exempt from the atomicity rule by design, not in violation of it.**

See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for which of `loom`'s concrete producers is simple versus bespoke, and which engine drives each.

`Shed`'s own contract stays exactly two parts, Input and Output pointers.
Its resume/crash-recovery/pause guarantee operates at **producer granularity only**, re-driving a crashed producer from its last recorded pointer and never mid-producer.
A bespoke multi-spawn producer that would lose expensive internal progress on a crash needs its **own** internal crash-recovery, a capability `Shed` does not provide;
both current bespoke examples already ship it — `internal/websterengine` re-drives the first unreported batch from its recorded state (see its package documentation's crash/resume section), and `internal/treadleengine`'s round loop keeps its own resumable run-dir state under an OS advisory lock released automatically if the holding process dies.

A producer's worst-case internal shape, not its happy path, decides its typology classification — a producer that is pure Go on the common path but spawns an internal multi-step process (an LLM session, several forks) on an exceptional path is bespoke, because the axis exists to say who owns crash-recovery, and the exceptional path is exactly where that ownership question bites.
See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) and [finalize.md](finalize.md) for `Finalize`'s own worked example of this — bespoke on the typology axis despite a zero-LLM happy path, and adapter-free on the engine axis at the same time, which is exactly the two-axis independence the next section states.

A producer's **definition** — internal to how `Shed` actually runs it, invisible to the contract — additionally names an **engine** (which code drives it) and a **config** (how that engine is parameterized for this specific producer).
Many producers share the same engine: every `*-Review` producer is `engine: perch`, differing only by which rubric/fasit `config` file is handed to it — the same generic, profile-driven mechanism `perch` already implements today ("reused for every phase... only the review profile differs").

### Engine adapters — a thin, shared seam, not one per producer

`Shed` needs a minimal common interface to drive any producer uniformly — call it, get back an outcome (done / approved / stuck / blocked) plus an output-artifact pointer, without needing to know what happened inside.
This is not a new pattern: it mirrors two seams that already exist in this codebase —

- `internal/treadleengine`'s `RoundRunner` seam (`internal/perchengine`'s burler adapter is its reference consumer).
- `internal/batcher`'s `Batcher` interface (multiple batchifier implementations behind one interface, resolved by name via `Select`).

Applied one level up: every producer satisfies a `ShedProducer` interface, and — critically — **`Shed` needs not one adapter per producer, but one per distinct engine type**:

- **A mechanical Go-function producer** needs no translation adapter at all — a plain Go function already satisfies `ShedProducer` directly.
- **A `SingleLLMProducer`** is one generic, reusable `ShedProducer` implementation for the "simple, single-agent-spawn, LLM" case: parameterized by an Input-format pointer, an Output-format pointer, and one instruction file (which may itself point to further files a producer reads mid-session). Two concrete producers configuring this same generic type is not two adapters — it is one adapter, instantiated twice with different pointers, unified today via the shared `shuttleengine.Spec` → `shuttle.Run` pattern.
- **`perch`** needs one adapter, reusable by every review-gate producer regardless of which artifact it reviews.
- **A black-box multi-spawn engine** (e.g. `Webster`'s own verb-driven form) needs its own adapter, one per such engine, not one per producer that happens to use it.

So the adapter count scales with the number of distinct *engines* in play, never with the number of producers — see [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for how many concrete adapters `loom`'s own list currently needs.

This split cuts on **engine type** — which code drives the producer, and therefore how many adapters must be built — whereas the simple/bespoke typology in [Producer contract vs. producer definition](#producer-contract-vs-producer-definition) above cuts on **atomicity and crash-recovery ownership**.
The two axes are independent and need not align: a producer can be mechanical-and-simple, mechanical-and-bespoke (pure Go on its happy path but owning an internal multi-step process on an exceptional one), or LLM-and-either.
One `perch` adapter, for instance, can serve several separate bespoke producers at once — the axes describe different questions, so neither predicts the other.

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
