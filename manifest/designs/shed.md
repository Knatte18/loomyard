# Shed — a shared Go outer phase-FSM for `loom` and `Hardener`

> **Status: Design sketch, Planned** (after `Treadle`, before the `perch` rewrite — see `manifest/roadmap.md`). Naming: a loom's shed is the gap formed between warp threads for the shuttle to pass through — apt for the generic engine that opens a slot for whichever producer list a product configures it with. Pairs naturally with the shipped `shuttle` (the thing that passes through it). This doc is the authoritative description of `Shed`'s own generic mechanism (the flat producer list, the loop, the status file, the producer contract, engine adapters); [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it, plus the `loom`-specific detail (session bootstrap, auto-mode, module decomposition) this doc doesn't restate.

## What it is

`Shed` has no predefined slots at all — no Preflight-slot, no Producer-slot, no shared Finalize.
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
**Raddle folds into `Finalize`'s own contract**, not a separate producer or a separate slot: updating Raddle before the Finalize merge is impractical given merge-conflict risk, so Raddle-regeneration is scoped as part of the merge itself.
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

`Shed`'s own scaffolding is six steps, nothing else — everything a producer does past its own `Call()` return value is invisible to this loop:

1. Read the status file. Missing → **hard error, halt.** `Shed` never seeds one itself — a status file must already exist before `Shed`'s first call, written by whatever spawns the task (see `status-schema.md`'s "seed / handover" section for `loom`'s own instance of this). Seeding is a one-time, product-specific bootstrapping act, not part of `Shed`'s per-invocation loop; if `Shed` seeded on missing, a product whose own precondition producer checks for a coherent fresh seed (`loom`'s already-shipped `Preflight`, `CheckSeedMissing`) would find that check permanently unreachable, since `Shed`'s own seed would always land first.
2. Look up the `ProducerDef` at `current_producer`. Not found in `Producers` (the list changed since this status file was last written — a producer renamed, removed, or reordered) → **hard error, halt, change nothing on disk.** `Shed` never guesses which entry was meant, and never restarts from `producers[0]` or advances to the nearest match — both are ways of silently fabricating a status a human never confirmed. A human reconciles.
3. Check `pause_requested` **and** `ctx.Err()`. Either set → write `state: "paused"` (`Result.Outcome = RunPaused`, nil `error`), exit cleanly; nothing more happens until the next `lyx run`. Treated identically on purpose: an operator's Ctrl-C or a parent deadline is an operational stop, not a failure, exactly as resumable as an explicit pause request — one clean-stop path, not two. Checked here, at the top of every loop iteration (not only once, and not left to producers to notice on their own) — a `Shed` that only checked `ctx` inside producers would keep launching *new* producer calls after cancellation, for however long the current one takes to notice.
4. Call `producer.Call(ctx)` → `(Outcome, OutputPointer, error)`.
5. Append `{producer, outcome, output, at}` to `history`; persist the status file. This append-then-persist is the entire crash-safety mechanism: a crash before it means step 4 simply runs again next time; a crash after it means step 6 already knows where to go. **If the persist itself fails** (disk full, lock unavailable, `state.WriteJSON` errors): halt and return the error from `Run` immediately, without attempting a `state: "failed"` write — that write would be the exact same operation that just failed, so retrying it to record the failure is the one action already known not to work. The file keeps its last-good contents, so `current_producer` still names the producer whose `Call()` just ran; it is simply re-called next time, exactly like any other crash.
6. Route on the result:
   - `error` → write `state: "failed"` and `error: err.Error()`, halt, exit. An engine-level failure, not a producer verdict — never routed anywhere, always a human resolves it.
   - `Stuck` → look up this producer's `OnStuck` and check the bounce budget (below). Named target *and* budget remaining → `current_producer` becomes that target, back to step 2 (bounce back). No target, **or** budget exhausted → write `state: "blocked"` (and, if exhausted, `error: "bounce budget exhausted"`), exit (escalate to human).
   - `Done` → advance `current_producer` to the next entry, back to step 2. Past the last entry → write `state: "done"`, exit. Otherwise write `state: "running"` before looping.

**Bounce-budget: a single total cap across the whole run, not per-producer.** `OnStuck` permits a cycle (`Plan-Review` → `Plan-Write` → `Plan-Review` → …), and every hop can be a full LLM session — an unbounded cycle is not a hypothetical, it is the default outcome whenever a bounced-back producer keeps failing the same way. `internal/treadleengine` already carries exactly this discipline for the identical risk shape (a hard round cap, "generalized machinery moved here verbatim from perch's shipped round loop") — `Shed` mirrors it rather than reinventing it. `Shed` decrements one counter on every bounce, regardless of which producers are involved: a per-producer budget would let an A↔B cycle run `2×budget` bounces before either individually trips, which does not actually bound the thing being guarded against (total wasted spend before a human is pulled in). A sane default, overridable per `Shed` instance; exhausted behaves exactly like "no `OnStuck` target" — `blocked`, not a distinct third case.

**The exact `ShedProducer` contract:**

```go
type Outcome string
const (
    Done  Outcome = "done"
    Stuck Outcome = "stuck"
)

type OutputPointer struct {
    Path string // "" = no artifact (gate/terminal producer)
}

type ShedProducer interface {
    Call(ctx context.Context) (Outcome, OutputPointer, error)
}
```

One method, three return values. `Shed` never introspects `Path`'s contents and never validates it against anything — an opaque string it stores for a human to read, per the "no opinion on Output's shape" rule above.
**String-typed, not `int`+`iota`** — matching `internal/treadleengine.Outcome` (`type Outcome string; const (OutcomeApproved Outcome = "APPROVED", ...)`) exactly, and for the same reason: `history`'s persisted `outcome` field below is the literal string `"done"`, so a string-typed `Outcome` makes the in-memory value and the on-disk value one vocabulary, never a hand-maintained int→string mapping between the two.

**The exact producer definition — the contract plus what the list needs around it:**

```go
type ProducerDef struct {
    Name     string
    Producer ShedProducer
    OnStuck  string // "" = escalate to human; else bounce back to this Name
}

type Shed struct {
    Producers  []ProducerDef
    StatusPath string // absolute path to the status file; Shed is told it, never derives it
    LockPath   string // absolute path to the run lock (see Run's own locking, below)
    MaxBounces int    // total Stuck-routed bounces across the whole run; 0 = an internal sane default, never "no bounces allowed"
}
```

`OnStuck` is what makes "`Plan-Review`'s stuck routes back to `Plan-Write`" a per-producer config value in the list, not a hardcoded branch in `Shed`'s loop.
`MaxBounces` is the total-bounce budget from above — one field on `Shed` itself, not per-`ProducerDef`, since the risk it guards is total wasted spend across the run, not any single producer's own bounce count.
`StatusPath`/`LockPath` are exactly the caller-supplied, told-not-derived paths from the geometry question above — `Shed` never constructs either from a `_lyx` convention of its own; the caller (`loom`, eventually `Hardener`) resolves them from its own geometry and hands them in.

**The entrypoint:**

```go
type RunOutcome string
const (
    RunDone    RunOutcome = "done"
    RunBlocked RunOutcome = "blocked"
    RunPaused  RunOutcome = "paused"
)

type Result struct {
    Outcome        RunOutcome
    HaltedProducer string // the producer current_producer named when Run returned
    Reason         string // set only alongside RunBlocked -- why (no OnStuck target, or budget exhausted)
    History        []HistoryEntry
}

func (s *Shed) Run(ctx context.Context) (Result, error)
```

`RunOutcome` is deliberately its own type, not a reuse of `ShedProducer`'s `Outcome` above — the two describe different things (one producer's call vs. the whole run's exit) and `Done` already names a value on the first; a shared type would force a naming collision or an overloaded meaning for no benefit. String-typed for the same reason `Outcome` is: `RunDone`/`RunBlocked`/`RunPaused`'s values are exactly `state`'s persisted values below, one vocabulary, not two kept in sync by hand.

Mirrors `internal/treadleengine.Engine.Run(p Profile, runDir string) (result Result, err error)` exactly — same `(Result, error)` shape one level up, not a coincidence: `perch`'s own adapter into `treadle` is the concrete precedent this follows. `Run` walks the whole six-step loop in one call, from wherever the status file's `current_producer` currently sits, until it hits a stopping condition (`pause_requested`/cancellation, `blocked`, `done`, or an `error`) — never a `Step()`-per-call API the caller loops over, since the loop itself is `Shed`'s entire deliverable; pushing it out to every caller would mean `loom` and `Hardener` each reimplementing the same sequencing/pause-check/routing logic, exactly the duplication `Shed` exists to centralize. A non-nil `error` return covers both step 1's and step 2's hard-error cases above (missing or incoherent status file) and a producer's own `Call()` returning `error`; `Result.Outcome` covers the three clean exits, mirroring `state`'s three non-failure values below.

**Two things `Run` does before step 1, both fail loud, neither touches the status file:**

- **Validates `Producers`** — an empty list, a duplicate `Name`, or an `OnStuck` naming a `Name` not present in the list are all a returned `error`, before any producer is ever called. Caught here rather than lazily (only when a producer actually goes `Stuck` and its `OnStuck` typo surfaces) because that is the worst possible timing — a config mistake compounding an unrelated failure, hours into a real run, instead of failing on the very first invocation. `Shed` stays the plain exported-field struct this doc pins throughout — no `New(...)` constructor, which would create a second, unvalidated way to build one (a bare struct literal) alongside the validated one.
- **Acquires `LockPath` non-blocking**, mirroring `internal/treadleengine/run.go`'s own `lock.TryAcquireWriteLock` / `ErrBlockBusy` precedent exactly (`run.go:119–128`): already held → a sentinel error (e.g. `ErrShedBusy`), `Run` returns immediately, nothing on disk touched. `internal/state`'s own per-write lock does not substitute for this — it is held only for the duration of one write, not across a whole `Call()`, so two concurrent `lyx loom run` invocations could otherwise both read the same `current_producer` and both spawn it, double-spending an LLM session. Released via `defer`, OS-reclaimed on crash even if `Release` is never reached, so a killed process never bricks a later resume.

**The status file** (`Shed`'s own generic contract — `loom`'s `_lyx/loom/status.json` is one instance of it, not a `loom`-specific shape):

```json
{
  "current_producer": "Plan-Write",
  "state": "running",
  "error": "",
  "pause_requested": false,
  "activity": {"now": "...", "last": "...", "wait": "..."},
  "history": [
    {"producer": "Preflight", "outcome": "done", "output": "", "at": "..."},
    {"producer": "Discussion-Write", "outcome": "done", "output": "_lyx/discussion/decision-record.md", "at": "..."}
  ]
}
```

`history`'s `output` field exists for observability (`lyx loom status`, an audit trail) — `Shed` writes it and never reads it back for control flow, per the point below.

**`state`/`error` are how a terminal condition survives a process exit** — the missing piece an earlier version of this doc's status-file example left out. `Result`'s `Outcome`+`Reason` exist only in memory for the duration of one `Run` call; without persisting the equivalent on disk, a restarted `lyx run` (or a human reading `lyx loom status`) cannot tell "paused, resumable" from "blocked, needs a human" from "crashed" — all three look identical, an unattended status file sitting still. `state` is one of `"running" | "paused" | "done" | "blocked" | "failed"`, written at every step-6 exit per the routing above; `error` is human-readable detail, `""` when `state` carries no failure (mirrors `Result.Outcome`+`Result.Reason`'s split, now on disk instead of only in memory).

**`activity` is filled mechanically by `Shed` itself, from data it already holds** — `Shed` is the file's only writer, so if `Shed` does not fill this, nothing else can. `now` is `current_producer`'s name; `last` is the most recent `history` entry's `producer`+`outcome`, formatted for a human; `wait` is set only when `state` is `"blocked"` or `"failed"` (the `error` text, or a short reason), else `""`. No per-product hook — every field here is either already a `Shed`-owned value or trivially derived from one.

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

## Process — decomposed into several small tasks

`Shed`'s own skeleton (the loop, the status file, the `ShedProducer` interface) is one task on its own — no adapters, no `Finalize`, nothing `loom`-specific.
The three engine adapters (`SingleLLMProducer`, `perch`, `Webster`) are a separate task: each is a small, self-contained wrapper around an already-shipped engine, sharing nothing with `Shed`'s own skeleton beyond the `ShedProducer` interface each implements.
`Finalize` is bundled with neither — it is genuinely new code (see [finalize.md](finalize.md)), scoped as its own task once `loom`'s producer list reaches it, on the same footing as the not-yet-detailed Plan and Webster phases.
See `manifest/roadmap.md`'s Planned section for the concrete task sequence this decomposes into.

## Why this doc doesn't rewrite loom.md's full detail

`loom.md` is a mature, ~320-line, detailed design (crash recovery, pause semantics, session bootstrap, module decomposition) — this doc's own core model section, including [The `Shed` loop — exact mechanics](#the-shed-loop--exact-mechanics), is now the authoritative description of `Shed`'s generic mechanism (the loop, the status file, the producer contract, engine adapters), and `loom.md`'s own [phase-machine section](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it.
What this doc does *not* redo is `loom.md`'s remaining `loom`-specific detail — session bootstrap (tmux, the status strand, the run-launcher), auto-mode's human-gate framing, and module decomposition — those stay in `loom.md`, described in `loom`-specific terms, layered on top of the generic loop mechanics this doc now pins.

## Related

- [loom.md](loom.md) — `loom`'s concrete producer list built on `Shed`, plus the remaining `loom`-specific detail (session bootstrap, auto-mode, module decomposition) this doc doesn't restate.
- [finalize.md](finalize.md) — `Finalize`'s own contract in detail; here it is one producer definition among others, not special-cased.
- [raddle.md](raddle.md) — the merge-time regeneration decision and merge-lock scope `Finalize`'s own contract must honor, now that Raddle folds into it rather than keeping a separate slot.
- `internal/treadleengine` package documentation — the sibling generic engine (inner round-loop, not outer phase-FSM), and the precedent for `Shed`'s own engine-adapter seam (`RoundRunner`).
- `internal/batcher` package documentation — the other existing precedent for the engine-adapter pattern (`Batcher` interface).
- [hardener.md](hardener.md) — `Hardener` (`Shed` + Hardener's own producer list), Someday.
