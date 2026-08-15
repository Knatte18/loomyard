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

`Shed` has no opinion on what a producer's Input or Output *is* — only on how it drives one.
Its own contract is exactly this: **call it**, however it decides to do that internally is invisible to `Shed`;
**get back an outcome** (done / approved / stuck / blocked);
**get back an optional output pointer**, a path `Shed` can check for completeness on resume.
A producer with no output pointer — a **gate producer**, pass/fail only, or a **terminal producer** with no downstream consumer — simply re-runs on resume, since the resume-on-output-files rule degrades gracefully: a cheap idempotent re-check for a gate, and the terminal producer's own recovery obligation, not designed here, if its effect was mid-flight.
That is the entire `ShedProducer` contract — see [Engine adapters](#engine-adapters--a-thin-shared-seam-not-one-per-producer) below.
`Shed` never reads a producer's Input and never inspects the shape of its Output;
it has no concept of a "format-contract file."

**The producer-authoring convention** — a separate concern from `Shed`'s own contract above, governing how instruction files and format-contract docs are written, not how `Shed` runs: a producer's Input and Output, where documented, are pointers into a format-contract file, never a restated copy of its content.
[CONSTRAINTS.md](../../CONSTRAINTS.md)'s Producer Pointer-Rule Invariant is what enforces this, by review, over instruction files and format-contract docs — not over `Shed` itself, which has no Go-level dependency on the rule.
See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for each of `loom`'s concrete producers' Input/Output pointers under this convention, including the thin-Input case (a chain-head producer, whose input is human intent rather than an artifact with a format contract) and the thin-Output case (gate and terminal producers, per above).
Review is never a property attached to the producer it reviews; it is always the next, separate producer in the list.

Producers split into two kinds — **Kind has no bearing on `Shed`'s own mechanism.**
`Shed` calls and resumes every producer identically regardless of Kind;
the axis exists purely to say who owns crash-recovery *inside* a producer's own execution, and the atomicity rule stated above is scoped to the first:

- A **simple, single-agent-spawn producer** is one mechanical action or one LLM session.
  This kind does not typically need its own crash-recovery, since re-running one spawn from scratch is cheap.
  A single-LLM-spawn instance of this kind is a `SingleLLMProducer` — see [Engine adapters](#engine-adapters--a-thin-shared-seam-not-one-per-producer) below.
- A **bespoke, multi-spawn producer** owns its own internal loop — many LLM spawns, or an agent orchestrating sub-agents.
  Bespoke producers are **exempt from the atomicity rule by design, not in violation of it,** and if they would otherwise lose expensive internal progress on a crash, they need their **own** internal crash-recovery — a capability `Shed` does not provide.
  Both current bespoke examples already ship it — `internal/websterengine` re-drives the first unreported batch from its recorded state (see its package documentation's crash/resume section), and `internal/treadleengine`'s round loop keeps its own resumable run-dir state under an OS advisory lock released automatically if the holding process dies.

See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for which of `loom`'s concrete producers is simple versus bespoke, and which engine drives each.

`Shed`'s resume/crash-recovery/pause guarantee operates at **producer granularity only**: after any restart, `Shed` re-calls whichever producer `current_producer` names, never mid-producer — the same mechanism regardless of Kind.
See [The `Shed` loop — exact mechanics](#the-shed-loop--exact-mechanics) below for why this is an unconditional re-call, not a "skip if the output already exists" shortcut.

A producer's worst-case internal shape, not its happy path, decides its typology classification — a producer that is pure Go on the common path but spawns an internal multi-step process (an LLM session, several forks) on an exceptional path is bespoke, because the axis exists to say who owns crash-recovery, and the exceptional path is exactly where that ownership question bites.
See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) and [finalize.md](finalize.md) for `Finalize`'s own worked example of this — bespoke on the typology axis despite a zero-LLM happy path, and adapter-free on the engine axis at the same time, which is exactly the two-axis independence the next section states.

A producer's **definition** — internal to how `Shed` actually runs it, invisible to the contract — additionally names an **engine** (which code drives it) and a **config** (how that engine is parameterized for this specific producer).
Many producers share the same engine: every `*-Review` producer is `engine: perch`, differing only by which rubric/fasit `config` file is handed to it — the same generic, profile-driven mechanism `perch` already implements today ("reused for every phase... only the review profile differs").

### The `Shed` loop — exact mechanics

**`Shed`'s own scaffolding, stated precisely (2026-08-15).** Six steps, nothing else — everything a producer does past its own `Call()` return value is invisible to this loop:

1. Read the status file. Missing → seed at `producers[0]`.
2. Look up the `ProducerDef` at `current_producer`.
3. Check `pause_requested`. Set → exit cleanly; nothing more happens until the next `lyx run`.
4. Call `producer.Call(ctx)` → `(Outcome, OutputPointer, error)`.
5. Append `{producer, outcome, output, at}` to `history`; persist the status file. This append-then-persist is the entire crash-safety mechanism: a crash before it means step 4 simply runs again next time; a crash after it means step 6 already knows where to go.
6. Route on the result:
   - `error` → halt, record it, exit. An engine-level failure, not a producer verdict — never routed anywhere, always a human resolves it.
   - `Stuck` → look up this producer's `OnStuck`. Named target → `current_producer` becomes that target, back to step 2 (bounce back). No target → mark the status `blocked`, exit (escalate to human).
   - `Done` → advance `current_producer` to the next entry, back to step 2. Past the last entry → the run is done.

**The exact `ShedProducer` contract:**

```go
type Outcome int
const (
    Done Outcome = iota
    Stuck
)

type OutputPointer struct {
    Path string // "" = no artifact (gate/terminal producer)
}

type ShedProducer interface {
    Call(ctx context.Context) (Outcome, OutputPointer, error)
}
```

One method, three return values. `Shed` never introspects `Path`'s contents and never validates it against anything — an opaque string it stores for a human to read, per the "no opinion on Output's shape" rule above.

**The exact producer definition — the contract plus what the list needs around it:**

```go
type ProducerDef struct {
    Name     string
    Producer ShedProducer
    OnStuck  string // "" = escalate to human; else bounce back to this Name
}

type Shed struct {
    Producers []ProducerDef
}
```

`OnStuck` is what makes "`Plan-Review`'s stuck routes back to `Plan-Write`" a per-producer config value in the list, not a hardcoded branch in `Shed`'s loop.

**The status file** (`Shed`'s own generic contract — `loom`'s `_lyx/status.json` is one instance of it, not a `loom`-specific shape):

```json
{
  "current_producer": "Plan-Write",
  "pause_requested": false,
  "activity": {"now": "...", "last": "...", "wait": "..."},
  "history": [
    {"producer": "Preflight", "outcome": "done", "output": "", "at": "..."},
    {"producer": "Discussion-Write", "outcome": "done", "output": "_lyx/discussion/decision-record.md", "at": "..."}
  ]
}
```

`history`'s `output` field exists for observability (`lyx loom status`, an audit trail) — `Shed` writes it and never reads it back for control flow, per the point below.

**Step 4 is an unconditional re-call — `Shed` never shortcuts it by checking whether `OutputPointer.Path` already exists on disk.**
That shortcut looks tempting (loom.md's crash-recovery language: "resume on output files, not live processes") but it is unsafe as a generic `Shed`-level check: after an `OnStuck` bounce-back, the *previous* attempt's output file for that producer is still sitting on disk, and `Shed` cannot tell a stale file from a fresh one by existence alone.
So the "is there already a live session, is there already a fresh complete output, should I respawn" three-case discipline is **not** `Shed`'s — it is delegated whole to each engine adapter's own `Call()` implementation (`SingleLLMProducer` wraps `shuttle`+`reed` and does this internally; `perch`/`Webster` already own their own resume, per [Producer contract vs. producer definition](#producer-contract-vs-producer-definition) above).
A mechanical Go-function producer needs no such discipline at all — re-running its check is cheap by construction.
This is the natural conclusion of `Shed` having no opinion on Output's shape: it shouldn't stat a path to make a control-flow decision either.

**What `Shed` does not provide** — each lives in the engine adapter or the product's own CLI wrapper instead:

- Crash-recovery of live-session state (reattach vs. respawn) — inside `SingleLLMProducer`/`perch`/`Webster`'s own `Call()`.
- Session/tmux/reed bootstrap — the product's CLI entry point (`lyx loom run`) does this *before* invoking `Shed`'s loop.
- Status-strand rendering (`lyx loom status --watch`) — `reed` hosts it, reading the file `Shed` writes; `Shed` never renders anything.
- Round loops, N-caps, batch decomposition — `perch`/`burler`/`batcher`'s own internals, opaque behind one `Call()`.
- Anything about Input, or Output's shape — the producer-authoring convention above, not `Shed`.

### Engine adapters — a thin, shared seam, not one per producer

`ShedProducer` (defined above) is the minimal common interface `Shed` uses to drive any producer uniformly, without needing to know what happened inside.
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

`loom.md` is a mature, ~320-line, detailed design (crash recovery, pause semantics, session bootstrap, module decomposition) — this doc's own core model section, including [The `Shed` loop — exact mechanics](#the-shed-loop--exact-mechanics), is now the authoritative description of `Shed`'s generic mechanism (the loop, the status file, the producer contract, engine adapters), and `loom.md`'s own [phase-machine section](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it.
What this doc does *not* redo is `loom.md`'s remaining `loom`-specific detail — session bootstrap (tmux, the status strand, the run-launcher), auto-mode's human-gate framing, and module decomposition — those stay in `loom.md`, described in `loom`-specific terms, layered on top of the generic loop mechanics this doc now pins.

## Related

- [loom.md](loom.md) — `loom`'s concrete producer list built on `Shed`, plus the remaining engine detail (crash recovery, pause, session bootstrap) this doc doesn't restate.
- [finalize.md](finalize.md) — `Finalize`'s own contract in detail; here it is one producer definition among others, not special-cased.
- [raddle.md](raddle.md) — the merge-time regeneration decision and merge-lock scope `Finalize`'s own contract must honor, now that Raddle folds into it rather than keeping a separate slot.
- `internal/treadleengine` package documentation — the sibling generic engine (inner round-loop, not outer phase-FSM), and the precedent for `Shed`'s own engine-adapter seam (`RoundRunner`).
- `internal/batcher` package documentation — the other existing precedent for the engine-adapter pattern (`Batcher` interface).
- [hardener.md](hardener.md) — `Hardener` (`Shed` + Hardener's own producer list), Someday.
