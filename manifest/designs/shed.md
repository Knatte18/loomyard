# Shed — a shared Go outer phase-FSM for `loom` and `Hardener`

> **Status: the skeleton — the loop, the status file, the `ShedProducer` interface, and the producer-list validation — is shipped as `internal/shedengine`, and the engine adapters (`SingleLLMProducer`, the `Webster` adapter, the `Burler`-round adapter, and the `Bouncer`) ship as `internal/shedadapters`.** See the `internal/shedengine` and `internal/shedadapters` package documentation for the as-built contract; this doc stays the design's own narrative rather than a duplicate of it. This doc survives both modules landing — which `docs/overview.md`'s two-class Documentation Lifecycle would otherwise read as grounds for deletion, the fate of a per-module design draft whose module has now landed — because it is not that class: it is the shared narrative four still-unbuilt modules are written against ([loom.md](loom.md) explicitly assigns it authority over `Shed`'s own generic mechanism while keeping loom's own producer list for itself, and `manifest/designs/hardener.md` and `manifest/designs/raddle.md` each build on that same narrative, as did the now-deleted `manifest/designs/landing.md` before its two producers landed), as does the durable `contracts/specs/loom-status-spec.md`. Several of those references are anchor-bearing links that Markdown Link Integrity enforces, so deleting this file would break live links in docs whose own modules are not built. Retention should be re-evaluated once those modules land. Naming: a loom's shed is the gap formed between warp threads for the shuttle to pass through — apt for the generic engine that opens a slot for whichever producer list a product configures it with. Pairs naturally with the shipped `shuttle` (the thing that passes through it). This doc is the authoritative description of `Shed`'s own generic mechanism (the flat producer list, the loop, the status file, the producer contract, engine adapters); [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it, plus the `loom`-specific detail (session bootstrap, auto-mode, module decomposition) this doc doesn't restate.

## What it is

`Shed` has no predefined slots at all — no Preflight-slot, no Producer-slot, no shared Publish or Finalize.
It is a generic engine that walks one ordered, flat list of **producers**, honoring resume/crash-recovery/pause uniformly across every entry;
atomicity — one mechanical action or LLM session, never an internal multi-step process of its own — binds **simple** producers only, per the carve-out in [Producer contract vs. producer definition](#producer-contract-vs-producer-definition) below.
Everything that used to look "special" — Preflight, Publish, Finalize, review gates — is just a producer like any other in that list.
What makes `loom` "loom" versus `Hardener` is purely which producers are in the list: pure configuration, not architecture.
The list's order is display and enumeration order only, never routing — `Done` and `Stuck` both route by an explicit per-producer field (`OnDone`, `OnStuck`), never by physical position, so a producer's own definition tells the whole story regardless of where it sits in the list.
See [loom.md's own producer-list table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for `loom`'s concrete list — this doc stays about `Shed`'s own generic mechanism, not about enumerating `loom`'s specific producers.

- **`loom`** = `Shed` + `loom`'s own producer list — unchanged behavior/CLI from the outside, `lyx loom run`. See [loom.md's own producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for the concrete list; this doc never repeats it, so a producer added, removed, or reclassified there needs no edit here.
- **`Hardener`** = `Shed` + Hardener's own list (its own `Preflight`, `Tenter` — `Treadle` + a live-substrate round-runner + behavior-review profile, see the `internal/treadleengine` package documentation — and its own `Publish`/`Finalize`) — `lyx hardener run`.
  Someday, deprioritized;
  not part of this doc's Planned scope.

`Publish` and `Finalize` are not `Shed`'s own special code — each is an ordinary producer both `loom` and `Hardener` happen to reference (by *reference* — the same producer definition named in both lists, so a change to either's definition is visible to both without either copying it), not something `Shed` special-cases.
**Raddle folds into `Finalize`'s own contract**, not a separate producer or a separate slot: updating Raddle before the Finalize merge is impractical given merge-conflict risk, so Raddle-regeneration is scoped as part of the merge itself.
`Hardener`'s `Tenter` will need the equivalent fold eventually — not designed here.

### Producer contract vs. producer definition

`Shed` has no opinion on what a producer's Input or Output *is* — only on how it drives one.
Its own contract is exactly this: **call it**, however it decides to do that internally is invisible to `Shed`;
**get back an outcome**, exactly one of two values — `Done` or `Stuck`, nothing else;
**get back an optional output pointer**, a path `Shed` can check for completeness on resume.
A producer with no output pointer — a **gate producer**, pass/fail only, or a **terminal producer** with no downstream consumer — simply re-runs on resume, since the resume-on-output-files-and-live-agent-evidence rule degrades gracefully: a cheap idempotent re-check for a gate, and the terminal producer's own recovery obligation, not designed here, if its effect was mid-flight.
`Shed`'s own contract is unchanged by that rule's live-agent half — it still re-`Call`s `current_producer` unconditionally on every resume, exactly as before.
What changed sits entirely inside the producer: a `SingleLLMProducer`-shaped producer may now answer that call by re-attaching to a still-live run instead of starting fresh, a distinction `Shed` itself neither makes nor needs to know about.
That is the entire `ShedProducer` contract — see [Engine adapters](#engine-adapters--a-thin-shared-seam-not-one-per-producer) below.
`Shed` never reads a producer's Input and never inspects the shape of its Output;
it has no concept of a "format-contract file."

**Two obligations a producer must honour and `Shed` cannot enforce.**
First, `Call` must return exactly `Done` or `Stuck` — `Outcome` is an open `string` type, so any other value returned with a nil error is treated as an engine-level failure, never a third verdict.
Second, `Call` must surface context cancellation as a non-nil `error`, never as `Stuck`.
The second obligation carries real stakes: `Shed` cannot tell a `Stuck` return with a cancelled context from a genuine producer verdict, so a producer that reports cancellation as `Stuck` would silently consume bounce budget, or escalate to blocked, for what was actually an operator stop.
This is written down rather than assumed because each adapter drives a distinct engine with its own error taxonomy, so the obligation cannot be checked once and forgotten — every engine adapter must independently honour it.

**The producer-authoring convention** — a separate concern from `Shed`'s own contract above, governing how instruction files and format-contract docs are written, not how `Shed` runs: a producer's Input and Output, where documented, are pointers into a format-contract file, never a restated copy of its content.
[CONSTRAINTS.md](../../CONSTRAINTS.md)'s Producer Pointer-Rule Invariant is what enforces this, by review, over instruction files and format-contract docs — not over `Shed` itself, which has no Go-level dependency on the rule.
See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for each of `loom`'s concrete producers' Input/Output pointers under this convention, including the thin-Input case (a chain-head producer, whose input is human intent rather than an artifact with a format contract) and the thin-Output case (gate and terminal producers, per above).
Review is never a property attached to the producer it reviews; it is always a separate producer, reached by explicit routing rather than by position in the list.

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
See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) and [internal/landingshed](../../internal/landingshed/doc.go) for `Finalize`'s own worked example of this — bespoke on the typology axis despite a zero-LLM happy path, and adapter-free on the engine axis at the same time, which is exactly the two-axis independence the next section states.

A producer's **definition** — internal to how `Shed` actually runs it, invisible to the contract — additionally names an **engine** (which code drives it) and a **config** (how that engine is parameterized for this specific producer).
Many producers share the same engine: every `*-Review` producer is a `Bouncer`+`Burler`-round pair, differing only by which rubric/fasit `config` file is handed to it — one generic, profile-driven mechanism reused for every phase, only the review profile differing.

### The `Shed` loop — exact mechanics

`Shed`'s own scaffolding is six steps, nothing else — everything a producer does past its own `Call()` return value is invisible to this loop:

1. Read the status file (`state.ReadJSONStrict`), at the top of **every** iteration, not only the first. Missing → **hard error, halt.** `Shed` never seeds one itself — a status file must already exist before `Shed`'s first call, written by whatever spawns the task (see `contracts/specs/loom-status-spec.md`'s "seed / handover" section for `loom`'s own instance of this). Seeding is a one-time, product-specific bootstrapping act, not part of `Shed`'s per-invocation loop; if `Shed` seeded on missing, a product whose own precondition producer checks for a coherent fresh seed (`loom`'s already-shipped `Preflight`, `CheckSeedMissing`) would find that check permanently unreachable, since `Shed`'s own seed would always land first.
   **The read gate's own strictness rule:** a persisted `state` outside its five legal values — the empty string included — is a hard error here, for the same reason a malformed file is: `state` is an enum string an external actor seeds, so a typo or a partial seed must not fall through to undefined behaviour.
   **The already-done short-circuit** sits exactly here, after this read and before step 2's lookup: a status file whose `state` is already `"done"` returns `Result{Outcome: RunDone}` immediately, filled from the file just read (`HaltedProducer` from `current_producer`, `History` from `history`), calling no producer and writing nothing. Its consequence is a decision, not an accident: a `done` file whose `current_producer` is no longer in the list returns cleanly rather than hard-erroring at step 2, because a finished task must not become un-queryable just because someone later edited the producer list. `blocked` and `failed` deliberately do **not** short-circuit — the loop proceeds and re-calls `current_producer`, which is how a human resumes after fixing whatever caused the halt.
2. Look up the `ProducerDef` at `current_producer`. Not found in `Producers` (the list changed since this status file was last written — a producer renamed, removed, or reordered) → **hard error, halt, change nothing on disk.** `Shed` never guesses which entry was meant, and never restarts from `producers[0]` or advances to the nearest match — both are ways of silently fabricating a status a human never confirmed. A human reconciles.
3. Check `pause_requested` **and** `ctx.Err()`. Either set → write `state: "paused"` (`Result.Outcome = RunPaused`, nil `error`), exit cleanly; nothing more happens until the next `lyx run`. Treated identically on purpose: an operator's Ctrl-C or a parent deadline is an operational stop, not a failure, exactly as resumable as an explicit pause request — one clean-stop path, not two. Checked here, at the top of every loop iteration (not only once, and not left to producers to notice on their own) — a `Shed` that only checked `ctx` inside producers would keep launching *new* producer calls after cancellation, for however long the current one takes to notice.
   **The resume write sits at the end of this step, after the pause check passes.** When the file just read carries any state other than `"running"` — a run resumed from `paused`, `blocked`, or `failed` — `Shed` writes `state: "running"` with an empty `error` before it calls anything.
   Without it the file keeps describing the run as halted for the whole of step 4, which for an LLM row is minutes: `lyx loom status` and the status strand both report `paused`/`blocked` while a real session is already spawning, and `activity.wait` keeps asserting the old halt's reason the loop is at that moment retrying past.
   That is precisely the three-way distinction `state` is persisted to carry — "paused, resumable" versus "blocked, needs a human" versus "crashed" — failing in the one window an operator watches hardest, right after typing `lyx loom run` to resume.
   It is the loop's only write that records no producer verdict, and it is conditional: on the ordinary running-to-running path it never fires, so every step after a resume keeps the one-persist-per-iteration shape steps 5–6 describe.
4. Call `producer.Call(ctx)` → `(Outcome, OutputPointer, error)`.
5. Append `{producer, outcome, output, at}` to `history` — on every branch of step 6 below **except** the cancellation branch, where nothing is appended because the producer never reached a verdict — then persist.
   **Steps 5 and 6 are one atomic persist, not two writes.** Routing is computed first, entirely in memory: the `history` entry (if any), the next `current_producer`, and the next `state`/`error`. Then the whole result — the `history` append, the new `current_producer`, the new `state` and `error`, and the recomposed `activity` — lands in a single locked `state.UpdateJSON` read-modify-write. As two separate writes, a crash landing between them would leave `current_producer` still naming the producer that just finished, so the next run re-calls it and appends a duplicate `history` entry — defeating the exact crash-safety property this step exists to provide, which only holds if "after it" and "step 6 decided" are the same instant.
   **The persist merges rather than rewrites**, carrying `pause_requested` and `product` forward from the on-disk copy rather than overwriting them from an in-memory snapshot, and it aborts without writing when the file it re-reads is found missing — so a status file deleted mid-run is never silently re-created from a zero value.
   **If the persist itself fails** (disk full, lock unavailable, `state.WriteJSON` errors): halt and return the error from `Run` immediately, without attempting a `state: "failed"` write — that write would be the exact same operation that just failed, so retrying it to record the failure is the one action already known not to work. The file keeps its last-good contents, so `current_producer` still names the producer whose `Call()` just ran; it is simply re-called next time, exactly like any other crash.
6. Route on the result, back to **step 1**, not step 2 — the status file is re-read at the top of every iteration, which is what lets step 3's pause check see a pause requested *during* the producer call that just returned. Routing back to step 2 was a bug, not a stylistic choice: a pause requested during a twenty-minute producer call was both never observed, since the old routing skipped past step 3's own data source, and silently destroyed by that iteration's blind rewrite.
   - `error`, context healthy → write `state: "failed"` and `error: err.Error()`, halt, exit. An engine-level failure, not a producer verdict — never routed anywhere, always a human resolves it.
   - `error`, **context cancelled** → routes to the pause exit instead of failure: `state: "paused"`, `RunPaused`, nil `error`. The predicate is the context's own state, `ctx.Err() != nil`, never a match against a cancellation sentinel: an `errors.Is` check would be wrong in the opposite direction, since a producer whose own internal derived context times out is a genuine producer failure while the parent context stays healthy. No `history` entry is appended on this path and `current_producer` is left unchanged.
   - `Stuck` → look up this producer's `OnStuck` and check this producer's own episode bounce budget (below) — the budget is per-producer, never shared with any other row. Named target *and* budget remaining → `current_producer` becomes that target, back to step 1 (bounce back). No target, **or** budget exhausted → write `state: "blocked"` (with the corresponding error string, below), exit (escalate to human).
   - `Done` → routes to this producer's own `OnDone`, back to step 1: a non-empty `OnDone` names the next `current_producer`, forward or backward through the list, with no positional fallback of any kind. An **empty** `OnDone` finishes the run from any list position — write `state: "done"`, exit — `current_producer` still keeps the just-finished producer's own name, never the empty string. Otherwise write `state: "running"` before looping.
   - Neither `Done` nor `Stuck`, returned with a **nil** error → a fourth branch: write `state: "failed"` with an error naming both the offending value and the producer, return that as a non-nil `Run` error, and still append a `history` entry recording the literal value received, so a human diagnosing it can see exactly what the adapter returned.

**Both `blocked` causes carry an exact error string, and `Result.Reason` carries the identical text** — `"stuck with no OnStuck target"` when a `Stuck` producer has no bounce target, and `"bounce budget exhausted"` when the budget runs out — one string written to both the persisted `error` field and the in-memory `Result.Reason`, rather than two phrasings that could drift apart.

**Bounce-budget: per-producer and episode-scoped, no run-wide cap.** `OnStuck` permits a cycle (`Plan-Bouncer` → `Plan-Burler` → `Plan-Bouncer` → …), and every hop can be a full LLM session — an unbounded cycle is not a hypothetical, it is the default outcome whenever a bounced-back producer keeps failing the same way.
This is an inversion of an earlier design, stated explicitly rather than left to be inferred from the code: `Shed` used to decrement one run-wide counter on every bounce, regardless of which producer was involved, and the doc's own argument for that shape was that a per-producer budget would let an A↔B cycle run `2×budget` bounces before either individually trips — which, that argument said, does not actually bound the thing being guarded against (total wasted spend before a human is pulled in).
That argument is now overturned, and answered rather than dropped: the aggregate is still bounded, just by a **sum rather than a single number**.
Within one set of episodes the total is at most the sum of the participating producers' effective `MaxBounces`, so the A↔B cycle really does cost `2×budget` bounces — and that is now a deliberate price, not an oversight, because the two producers are tracked, and budgeted, independently.
Across episodes the lifetime total is unbounded in principle, because a `Done` resets one producer's episode — but a reset is only ever *earned*, by that producer succeeding, or granted once after a hard failure a human had to resolve (see the failure-path note under "The exact boundary" below), so what grows without bound across episodes is progress, not wasted spend.
The residual, named rather than hidden: a cycle whose every member alternates `Stuck` and `Done` forever is bounded by nothing here — the same unbounded shape a `Done`-only cycle has, described further down — and has the same stop, pause or cancellation, checked at the top of every iteration.
**The exact boundary, restated per-producer:** a producer's own `MaxBounces` bounces are permitted, and the next `Stuck` that would otherwise route for *that producer* is the one refused — a budget of three performs three bounce-backs and blocks on the fourth `Stuck`, counting only that producer's own entries. The default, used when a `ProducerDef`'s own `MaxBounces` and `Shed.MaxBounces` are both `0`, is **ten**.
The count is **episode-scoped**, read from the producer's own entries already in the persisted `history[]`: it is the number of `Stuck` entries authored by this producer since its own most recent `Done` entry — all of them, when it has never returned `Done` — so the count spans invocations, crashes, and human resumes, not merely one `Run` call.
This is also an inversion, stated in the same breath rather than left implicit: the budget used to be per-`Run`-call and held in memory, deliberately unpersisted, so a crash-restart or a human-resumed `blocked` run started again with the full budget every time — every event that reset it was a new human-initiated invocation, precisely the outcome the old design meant to force.
That reset was overturned because a crash-restart loop under it is unbounded: a producer that keeps crashing and getting relaunched never runs out of budget.
Under episode scoping a resumed `blocked` run re-blocks immediately on the same producer's next `Stuck`, because a blocked producer has by definition not returned `Done` since — only a producer's own success, not a fresh invocation, earns it a reset.
State the failure-path terminator in the same breath as the episode rule, so the scan's "stop at the first `Done`" is never read as "stop at the first success": a producer that returns `(Done, output, err)` alongside a hard failure still has that `done` outcome recorded verbatim in `history[]`, and under episode scoping that entry ends the producer's episode too — the engine records what a producer actually returned rather than second-guessing it, `state: "failed"` halts the run regardless, and every continuation past a failure is a fresh human-initiated act, exactly the escalation the budget exists to force.
Exhausted behaves exactly like "no `OnStuck` target" — `blocked`, not a distinct third case.
**The `Stuck` entry written on the block path itself counts** toward the producer's own episode: the history append happens before the block-or-bounce decision, so after a block a producer's episode holds `budget + 1` `Stuck` entries, and every subsequent resume that re-blocks adds one more.

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
    Name       string
    Producer   ShedProducer
    OnStuck    string // "" = escalate to human; else bounce back to this Name
    OnDone     string // "" = finishes the whole Shed run from any list position; else routes Done to this Name, forward or backward
    Segment    string // "" = standalone; else a grouping label OnStuck's same-Segment rule enforces
    MaxBounces int    // this producer's own episode Stuck budget; 0 = inherit Shed.MaxBounces, never "no bounces allowed"
}

type Shed struct {
    Producers      []ProducerDef
    StatusPath     string // absolute path to the status file; Shed is told it, never derives it
    LockPath       string // absolute path to the run lock (see Run's own locking, below)
    StatusLockPath string // absolute path to internal/state's own lock; must never name the same file as LockPath
    MaxBounces     int    // the default a ProducerDef.MaxBounces of 0 inherits; 0 here in turn falls back to the internal default of ten, never "no bounces allowed"
}
```

`OnStuck` is what makes "`Plan-Validate`'s stuck route bounces back to `Plan-Write`" a per-producer config value in the list, not a hardcoded branch in `Shed`'s loop.
`OnDone` is the same idea applied to the happy path: the sole router for a `Done` verdict, with no positional fallback of any kind.
`Segment` is a plain grouping label with exactly one mechanical effect — `validate()`'s rule that a non-empty `OnStuck` must name a target sharing this producer's own `Segment` — and no other effect anywhere else; it does not scope the bounce budget and does not constrain `OnDone`, because crossing *out* of a segment on approval is the point.
`MaxBounces` inherits at two levels, and `0` never means "no bounces allowed" at either: a `ProducerDef`'s own `MaxBounces` of `0` inherits `Shed.MaxBounces`, which itself falls back to the internal default of ten when it too is `0`.
This replaces an earlier single-level design, worth naming rather than leaving implicit: `MaxBounces` used to be one field on `Shed` alone, a total-bounce budget for the whole run rather than any single producer's own count, and it was never persisted, so a crash-restart or a human-resumed run started again with the full budget every time.
That shape is gone — the budget is now per-producer and episode-scoped, derived from the persisted `history[]` rather than held in memory — and `Shed.MaxBounces` survives only as the inherited default a `ProducerDef` falls back to, not as a run-wide total.
`StatusPath`/`LockPath`/`StatusLockPath` are exactly the caller-supplied, told-not-derived paths from the geometry question above — `Shed` never constructs any of them from a `_lyx` convention of its own; the caller (`loom`, eventually `Hardener`) resolves them from its own geometry and hands them in.
**`LockPath` and `StatusLockPath` must never name the same file.** `internal/state` acquires its own lock with the blocking form, so a `Shed` whose two lock paths coincide would hang on its first persist rather than failing — the caller already has both paths on hand (`loomengine.LoomStatusLock` and its sibling), so this is never a hard ask, only a mistake worth rejecting loud.

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

A caller must branch on `Outcome` before reading `Reason`, which is populated only alongside `RunBlocked`.
**`Result` is meaningless unless the returned `error` is nil.** `RunOutcome`'s zero value is the empty string, not one of the three legal constants above, and every hard-error path — validation failure, a busy lock, a missing or incoherent status file, a persist failure — returns an unpopulated `Result` alongside its error. A caller checks `error` first and never inspects `Outcome` on a non-nil-error return, the same discipline as branching on `Outcome` before reading `Reason`, one level up.

`History` is the **full persisted history** as it stands when `Run` returns — every entry in the status file, not only the entries this invocation appended. A this-run-only slice would make a resumed run's `Result` silently incomparable to a fresh one's; the full scope is what makes `Result` a faithful view of the file `Shed` just wrote.

Mirrors `internal/treadleengine.Engine.Run(p Profile, runDir string) (result Result, err error)` exactly — same `(Result, error)` shape one level up, not a coincidence: `treadleengine`'s own `RoundRunner` adapter seam is the concrete precedent this follows. `Run` walks the whole six-step loop in one call, from wherever the status file's `current_producer` currently sits, until it hits a stopping condition (`pause_requested`/cancellation, `blocked`, `done`, or an `error`) — never a `Step()`-per-call API the caller loops over, since the loop itself is `Shed`'s entire deliverable; pushing it out to every caller would mean `loom` and `Hardener` each reimplementing the same sequencing/pause-check/routing logic, exactly the duplication `Shed` exists to centralize. A non-nil `error` return covers both step 1's and step 2's hard-error cases above (missing or incoherent status file) and a producer's own `Call()` returning `error`; `Result.Outcome` covers the three clean exits, mirroring `state`'s three non-failure values below.

**What `Run` does before step 1, all fail loud, none of it creating or modifying the status file:**

- **Validates `Producers` and `Shed`'s own fields** — before any producer is ever called: an empty `Producers` list; a duplicate `Name`; an `OnStuck` naming a `Name` not present in the list; an `OnStuck` naming a target whose `Segment` differs from its own; an `OnDone` naming a `Name` not present in the list; an `OnDone` naming its own producer; an **empty** `Name` on any `ProducerDef`; a **nil** `Producer` on any `ProducerDef`; a negative `MaxBounces` on `Shed` or on any `ProducerDef`; `LockPath` and `StatusLockPath` naming the same file; and any of the three paths (`StatusPath`, `LockPath`, `StatusLockPath`) being empty. Caught here rather than lazily (only when a producer actually goes `Stuck` and its `OnStuck` typo surfaces) because that is the worst possible timing — a config mistake compounding an unrelated failure, hours into a real run, instead of failing on the very first invocation. `Shed` stays the plain exported-field struct this doc pins throughout — no `New(...)` constructor, which would create a second, unvalidated way to build one (a bare struct literal) alongside the validated one.
  Two of these rules need their own reason. **An empty `Name` is rejected** because the empty string is already load-bearing twice: it is `OnStuck`'s escalate-to-human sentinel, so a producer literally named `""` would make an `OnStuck: ""` ambiguous, and it is the zero value a malformed or partial seed leaves in `current_producer`, which step 2's lookup would then resolve successfully and *run* — turning a corrupt status file into silent execution. **A nil `Producer` is rejected** because it panics at the call step rather than failing loud, and a panic inside a long unattended run is strictly worse than a validation error at second zero.
  **`OnDone` naming itself is rejected while `OnStuck` naming itself stays legal**, and that asymmetry is deliberate: `Done` routing consumes no bounce budget, so a self-referencing `OnDone` is a statically certain infinite loop worth catching before any producer runs, while a self-referencing `OnStuck` is budgeted and therefore bounded.
  **Deliberately not validated: reachability, and multi-producer `Done` cycles.** `validate()` cannot check whether every producer is reachable from some entry point, because it does not know the entry producer — that comes from the seeded status file's `current_producer`, which `Shed` never writes first and never guesses, so any reachability analysis here would be guesswork over an unknown starting point. A `Done` cycle spanning two or more producers (`A`'s `OnDone` is `B`, `B`'s `OnDone` is `A`) is not statically infinite the way a self-referencing `OnDone` is — any member may still exit via its own `OnStuck` — so a rule rejecting it would reject legitimate backward jumps along with genuine mistakes.
  **The silent-terminal risk:** an omitted `OnDone` is indistinguishable, by `validate()` or by `Shed`, from an intended terminal one — both leave the field empty — so a forgotten `OnDone` ends the whole run quietly with `RunDone` rather than failing loud. This is accepted rather than caught by a new rule, because a candidate rule such as "at most one producer may have an empty `OnDone`" would forbid multiple legitimate terminals, smuggling back the implicit list-position structure this design deliberately removed. The real defence lives outside `Shed` itself, in whichever product assembles the list: an exhaustive test pinning the whole routing table, plus asserting the assembled list passes `validate()`, catches a missing or misspelled `OnDone` before a real run ever sees it.
  **A `Done` cycle is unbounded by design**, once `OnDone` permits a backward jump and `Done` itself consumes no budget: a cycle whose every member keeps returning `Done` runs forever, and `validate()` catches only the single-producer self-reference case above, nothing more. This is a real behavior change from the physically-sequential design this task replaced, where `Done` only ever advanced forward and the loop terminated by list length alone. It is accepted for three reasons: a runtime iteration cap would be a new concept, since every other budget mechanism `Shed` has is `Stuck`-based, and any cap needs an arbitrary number either too low for a legitimate long run or too high to bound anything worth bounding; `OnDone` routing is entirely author-configured and statically visible in one producer list, unlike `Stuck` routing whose reachability depends on producer verdicts at runtime, so a `Done` cycle is a config bug a reader can see rather than an emergent runtime condition; and, decisively, the loop is not un-interruptible — step 3 re-reads the status file and checks `pause_requested` and `ctx.Err()` at the top of every iteration, so an operator stops a runaway cleanly with Ctrl-C or by writing `pause_requested: true`.
  The cost while a `Done` cycle spins is not merely wasted iterations: every iteration appends one `history` entry and the persist rewrites the whole slice, so an unattended `Done` cycle writes a quadratically growing number of status-file bytes into an unboundedly growing file — noisy and disk-hungry, which is also what makes it easy to spot rather than a silent hang.
- **What to do when a producer's budget is exhausted, and what each remedy costs.** The supported remedy is fixing the underlying failure so the producer returns `Done`, which resets its own episode by itself — a real remedy under episode scoping, not a restatement of the problem. When that is not possible, the remedy is raising that producer's `MaxBounces` (or `Shed.MaxBounces`) strictly above its current episode `Stuck` count — which, per the block-path note above, is `budget + 1` immediately after the first block and grows by one per re-block, so raising it by exactly one is never enough. This is today a **source edit and rebuild, not an operator action**: `MaxBounces` reaches no CLI flag and no config key anywhere in the repo. Hand-editing the persisted `history[]` is **not** endorsed as a hatch: it contradicts the status spec's one-entry-per-producer-call rule and the append-only property the whole budget derivation depends on.
  A producer that **structurally never returns `Done`** — one whose own contract is "always `Stuck`, hand off to a peer, never finished" — has no `Done` to reset its episode from, so under episode scoping its `MaxBounces` degrades to a **task-lifetime cap**, not a per-round one, and such a row's budget must be sized accordingly by whoever configures it.
- **Creates both lock paths' parent directories** (`os.MkdirAll`) before acquiring either lock. `internal/lock` opens a lock file with create-if-missing but never creates its parent directory, which is why both `internal/loomengine` and `internal/treadleengine` already do this. This is not path *derivation* — the paths are still told, `Shed` only ensures the told path is usable.
- **Acquires `LockPath` non-blocking**, mirroring `internal/treadleengine/run.go`'s own `lock.TryAcquireWriteLock` / `ErrBlockBusy` precedent exactly (`run.go:119–128`): already held → a sentinel error (e.g. `ErrShedBusy`), `Run` returns immediately. `internal/state`'s own per-write lock does not substitute for this — it is held only for the duration of one write, not across a whole `Call()`, so two concurrent `lyx loom run` invocations could otherwise both read the same `current_producer` and both spawn it, double-spending an LLM session. Released via `defer`, OS-reclaimed on crash even if `Release` is never reached, so a killed process never bricks a later resume.
  **Acquiring a lock is not a no-op on disk — the lock file itself is created.** The honest claim, and the one every other phrasing in this doc must agree with, is that `Shed` never creates or modifies the **status file** outside step 5/6's persist; the run lock and `internal/state`'s own lock file are a separate matter entirely.

**The status file's own Go types** — `Status`, the persisted file itself, and `HistoryEntry`, the element type `Result.History` above is typed on:

```go
type State string
const (
    StateRunning State = "running"
    StatePaused  State = "paused"
    StateDone    State = "done"
    StateBlocked State = "blocked"
    StateFailed  State = "failed"
)

type Activity struct {
    Now  string `json:"now"`
    Last string `json:"last"`
    Wait string `json:"wait"`
}

type HistoryEntry struct {
    Producer string  `json:"producer"`
    Outcome  Outcome `json:"outcome"`
    Output   string  `json:"output"`
    At       string  `json:"at"`
}

type Status struct {
    CurrentProducer string         `json:"current_producer"`
    State           State          `json:"state"`
    Error           string         `json:"error"`
    PauseRequested  bool           `json:"pause_requested"`
    Activity        Activity       `json:"activity"`
    History         []HistoryEntry `json:"history"`
    Product         json.RawMessage `json:"product,omitempty"`
}
```

`State`'s three clean-exit values — `StateRunning` excepted — are the **literal same strings** as `RunOutcome`'s three constants above, so mapping between `Result.Outcome` and `state` is identity, never a lookup table; `State` is the superset, adding `running` (a run in progress or interrupted mid-producer) and `failed` (an engine-level error), neither of which `RunOutcome` ever carries, since `Run` returns a non-nil `error` rather than a `Result` in the failure case.

**The status file** (`Shed`'s own generic contract — `loom`'s `.lyx/loom/status.json` is one instance of it, not a `loom`-specific shape):

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
  ],
  "product": {"slug": "loom-contracts", "parent": "main"}
}
```

`history`'s `output` field exists for observability (`lyx loom status`, an audit trail) — `Shed` writes it and never reads it back for control flow, per the point below.

**`history[].at` is pinned as RFC3339 UTC, written from a direct clock call** (`time.Now().UTC()`), with no injectable clock field — that would add a field to the struct shape pinned above for a value tests can assert structurally instead. Tests assert that each `at` parses as RFC3339 with a zero UTC offset and that entries are non-decreasing, never a literal.

**`state`/`error` are how a terminal condition survives a process exit** — the missing piece an earlier version of this doc's status-file example left out. `Result`'s `Outcome`+`Reason` exist only in memory for the duration of one `Run` call; without persisting the equivalent on disk, a restarted `lyx run` (or a human reading `lyx loom status`) cannot tell "paused, resumable" from "blocked, needs a human" from "crashed" — all three look identical, an unattended status file sitting still. `state` is one of `"running" | "paused" | "done" | "blocked" | "failed"`, written at every step-6 exit per the routing above; `error` is human-readable detail, `""` when `state` carries no failure (mirrors `Result.Outcome`+`Result.Reason`'s split, now on disk instead of only in memory).

**`Shed` is not the status file's only writer, and this doc must not claim it is** — that claim licenses exactly the whole-file clobber the merging persist above exists to prevent. Ownership splits three ways:

- **Shed-owned, rewritten on every persist:** `current_producer`, `state`, `error`, `activity`, `history`.
- **Shared, write-to-clear:** `pause_requested`. An outside actor sets it **true**; `Shed` only ever writes it **false**, exactly once, in the same persist that records `state: "paused"`.
- **External-writer-owned, only ever carried through:** `product`.

The seed itself is written by a spawn-time command, not by `Shed`, and `pause_requested` living in-status rather than in a separate flag file is a deliberate divergence from `webster`'s own separate pause flag file, one `contracts/specs/loom-status-spec.md` documents for `loom`'s instance of this shell.

**The external-writer lock contract.** Any actor other than `Shed` that writes this file — a product's pause verb, its spawn-time seeder, anything touching `product` — must go through `internal/state` using the same `StatusLockPath` `Shed` was told, because that lock is advisory and keyed on the caller-supplied path. A writer that ignores it can still lose its write, and can still clobber `Shed`'s. This cannot be enforced from `Shed`'s side, so it is written down here, alongside the two producer-side obligations that already are. The merge-safety claim above is qualified accordingly: safe against a concurrent external writer *that takes the same lock*, never unconditionally.

**`pause_requested` is a request `Shed` consumes, not a latch.** It is cleared in the same persist that records `state: "paused"`, so no window exists in which a stale `true` flag sits on disk; the durable record of "this run is paused" is the `state` field. Without this, the next run would re-read a still-true flag and pause again immediately, forever.

**`activity` is filled mechanically by `Shed` itself, from data it already holds.** `now` is `current_producer`'s name; `last` is `""` when `history` is empty, and otherwise the most recent entry composed as exactly `"<producer> → <outcome>"` — pinned to that exact format rather than "formatted for a human" because a test asserts this field, and an unpinned format cannot be asserted, only approximated; `wait` is scoped to the `"blocked"` and `"failed"` states only (the `error` text, or a short reason), `""` for every other state. No per-product hook — every field here is either already a `Shed`-owned value or trivially derived from one.

**Strictness is the contract of the read gate, not of the persist's internal merge base**, which re-reads leniently. Malformed JSON still fails loud on both paths. The one behaviour leniency permits is an unknown top-level key written by an external actor *after* the read gate passed: it is silently destroyed by the full-struct marshal, not surfaced, and the next strict read then sees a clean file and has nothing to reject. This is acceptable because `product` is the sanctioned channel for what an external writer owns, so a key outside it is a mistake nothing here promises to preserve — the key would not be caught later. A key present *before* the read does hard-error at the gate, exactly as the strict read gate always has.

**`product` is where a product's own fields live, not a compatibility shim.** `loom`'s own `product` carries exactly `slug`, `parent`, and `start_sha` — see `contracts/specs/loom-status-spec.md` for that shape and the additional coherence rules `loom`'s check 4 layers on top of this shell. A future product configuring `Shed` with a different `product` shape needs no reconciliation with `loom`'s: each product's `product` payload is its own, opaque to `Shed` and to every other product.

**Step 4 is an unconditional re-call — `Shed` never shortcuts it by checking whether `OutputPointer.Path` already exists on disk.**
That shortcut looks tempting (loom.md's crash-recovery language: "resume on output files, not live processes") but it is unsafe as a generic `Shed`-level check: after an `OnStuck` bounce-back, the *previous* attempt's output file for that producer is still sitting on disk, and `Shed` cannot tell a stale file from a fresh one by existence alone.
So the "is there already a live session, is there already a fresh complete output, should I respawn" three-case discipline is **not** `Shed`'s — it is delegated whole to each engine adapter's own `Call()` implementation (`SingleLLMProducer` archives any stale output and respawns rather than reattaching to a live session, since `shuttle` exposes no reattach entry point for it to call; `Webster` already owns its own resume, per [Producer contract vs. producer definition](#producer-contract-vs-producer-definition) above).
A mechanical Go-function producer needs no such discipline at all — re-running its check is cheap by construction.
This is the natural conclusion of `Shed` having no opinion on Output's shape: it shouldn't stat a path to make a control-flow decision either.

**What `Shed` does not provide** — each lives in the engine adapter or the product's own CLI wrapper instead:

- Crash-recovery of live-session state — inside `SingleLLMProducer`/`Webster`'s own `Call()`; `SingleLLMProducer` always archives and respawns rather than reattaching, while `Webster` owns its own resume.
- Session/tmux/reed bootstrap — the product's CLI entry point (`lyx loom run`) does this *before* invoking `Shed`'s loop.
- Status-strand rendering (`lyx loom status --watch`) — `reed` hosts it, reading the file `Shed` writes; `Shed` never renders anything.
- Round loops, N-caps, batch decomposition — the `Bouncer`/`burler`/`batcher`'s own internals, opaque behind one `Call()`.
- Anything about Input, or Output's shape — the producer-authoring convention above, not `Shed`.

### Engine adapters — a thin, shared seam, not one per producer

`ShedProducer` (defined above) is the minimal common interface `Shed` uses to drive any producer uniformly, without needing to know what happened inside.
This is not a new pattern: it mirrors two seams that already exist in this codebase —

- `internal/treadleengine`'s `RoundRunner` seam (no consumer today; reserved for a future `Tenter`).
- `internal/batcher`'s `Batcher` interface (multiple batchifier implementations behind one interface, resolved by name via `Select`).

Applied one level up: every producer satisfies a `ShedProducer` interface, and — critically — **`Shed` needs not one adapter per producer, but one per distinct engine type, plus one entry per producer that is itself new logic over an already-adapted engine rather than a translation of a different one**:

- **A mechanical Go-function producer** needs no translation adapter at all — a plain Go function already satisfies `ShedProducer` directly.
- **A `SingleLLMProducer`** is one generic, reusable `ShedProducer` implementation for the "simple, single-agent-spawn, LLM" case: the parameterization lives entirely in the caller's own `shuttleengine.Spec` source, which the adapter evaluates once per call and never templates itself. Two concrete producers configuring this same generic type is not two adapters — it is one adapter, instantiated twice with different `Spec` sources, unified today via the shared `shuttleengine.Spec` → `shuttle.Run` pattern.
- **The Bouncer** is `shuttleengine`-backed like `SingleLLMProducer`, but templating its own prompt from a rubric stencil and a generic template, with judge-specific work before and after the spawn — the first member of the second kind named above, new logic over an already-adapted engine rather than a translation of a different one.
- **A black-box multi-spawn engine** (e.g. `Webster`'s own verb-driven form) needs its own adapter, one per such engine, not one per producer that happens to use it.
- **The `Burler`-round adapter** wraps one `burlerengine` A-review/B-fix round as a single Shed row, and always hands back to its segment's `Bouncer` via `Stuck`, never advancing on its own.

So the adapter count scales with the number of distinct *engines* in play, plus one entry per producer that is new logic over an already-adapted engine rather than a translation of a different one — never with the number of producers outright.
That distinction is what keeps the original rule true for the cases it was written about: `SingleLLMProducer` and `Webster` are each still exactly one adapter per engine, and the Bouncer is the qualification the rule needed once a second kind of member joined the package.
See [loom.md's producer table](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) for how many concrete adapters `loom`'s own list currently needs.

**The round-artifact pair predicate is the durable, mechanical contract between the `Burler`-round adapter and its segment's `Bouncer`.**
The presence of both `round-<N>-review.md` and `round-<N>-fixer-report.md` in the segment's run directory means, and only means, that round `N` completed and produced a usable review.
Both the round producer and the `Bouncer` run that same test — the round producer to decide whether to advance, the `Bouncer` to tell its seed call from its judge call.
This doc stays the design's narrative and points at the `internal/shedadapters` package documentation for the as-built detail, rather than restating the whole producer contract here.

**The segment's round cap is the smaller of the two rows' `MaxBounces` budgets, not either row's alone.**
Neither row's bounce episode ever resets, and the `Bouncer` — the segment's entry point — runs one `Stuck` ahead of the round producer, so raising the cap means raising both rows together.

This split cuts on **engine type** — which code drives the producer, and therefore how many adapters must be built — whereas the simple/bespoke typology in [Producer contract vs. producer definition](#producer-contract-vs-producer-definition) above cuts on **atomicity and crash-recovery ownership**.
The two axes are independent and need not align: a producer can be mechanical-and-simple, mechanical-and-bespoke (pure Go on its happy path but owning an internal multi-step process on an exceptional one), or LLM-and-either.
One `Bouncer` adapter, for instance, can serve several separate bespoke producers at once — the axes describe different questions, so neither predicts the other.

## Checking an assembled producer list

An omitted `OnDone` is indistinguishable from an intended terminal one and ends the run quietly, so a caller assembling a producer list is responsible for asserting its own routing table exhaustively rather than relying on `Shed` to catch a missing entry — the sentence [`internal/shedengine/doc.go`](../../internal/shedengine/doc.go) itself states, word for word, under "Routing: `OnDone` and `OnStuck`, no positional fallback" above.
Until now no caller did.
`internal/shedcheck` closes that gap: a `Check` function that walks an assembled `OnDone`/`OnStuck` graph the same way `Run` would, without ever calling a producer, and reports every structural defect it finds instead of stopping at the first one.

The checker lives in its own package, `internal/shedcheck`, rather than inside `shedengine`, because it is an authoring-time analysis, not part of the engine's runtime contract,
and putting it in the engine would imply `Run` enforces it.
The import direction is `shedcheck` → `shedengine`, the safe one;
the reverse is already forbidden by the Shed Producer-Seam Invariant.

`Check` takes `entry` and `terminals` as explicit arguments and never infers either from the producer list.
`Shed` has no entry field and no terminal field,
and defaulting to `Producers[0]` would re-introduce the positional routing meaning this same doc's routing model disclaims.

`Check` reports exactly eight kinds, in a fixed order:

- `bad-entry` — entry is empty, or names no producer in the list.
- `no-terminals` — terminals is nil or has length zero.
- `bad-terminal` — one per supplied terminal name that is empty or names no producer.
- `dangling-target` — a live row's `OnDone` or `OnStuck` names no producer in the list.
- `unreachable` — a row's own list index is never reached walking done and stuck edges from entry.
- `unexpected-terminal` — a live, reachable row whose raw `OnDone` is empty and whose `Name` was not told as a terminal.
- `done-cycle` — one finding per member of a cycle among live, reachable rows using done edges only.
- `blind-gate` — a live, reachable row's stuck target has no route back to it, over both edge types.

`blind-gate` supersedes `Segment`'s old cross-segment-wiring-detection job, expressed as a real graph property — a gate whose bounce target never routes back to the gate — rather than as a matching label.
This task removes neither the `Segment` field nor `validate()`'s same-`Segment` rule, and neither is going away: the field and the rule are staying, per the corrected reasoning in [shed-recipe.md](shed-recipe.md#whats-in-a-recipe-row).

`done-cycle` generalises the length-1 case [`internal/shedengine/validate.go`](../../internal/shedengine/validate.go) already rejects (`OnDone: <self>`),
and the asymmetry that motivates it is real: a `Done` route consumes no bounce budget,
so a done cycle is a statically certain infinite loop, whereas a stuck bounce is budgeted and therefore bounded.

Nothing in production calls `Check` — its enforcement point is a `go test` invariant over loom's own producer list, not a runtime guard.

One perch mis-wiring `Check` cannot catch, stated as a limit rather than buried, since an over-claimed guarantee is worse than a narrow one: a `Burler` round producer handing back to its `Bouncer` via `OnDone` instead of `OnStuck` is invisible to `Check`, because both wirings make the gate reachable from the round producer, so the routing graph is identical either way — the difference is which verdict the round producer returns, behaviour inside `Call`, not a property of the graph `Check` inspects.

## Testable cheaply — a throwaway producer list proves the skeleton

Building `Shed`'s skeleton doesn't need a real producer list to validate against.
Plug in a short, disposable list — a couple of steps that just succeed immediately, including stub `Publish`/`Finalize` producers — and sequencing, resume, crash-recovery, and pause can all be exercised end-to-end without any of `loom`'s or `Hardener`'s real producers needing to exist yet.
This mirrors `loom.md`'s own stated approach ("testable against fake phases before real producers are wired in... the same fake-tested approach the round loop used against a fake `burler`") — reused here to validate the *extraction*.

## Process — decomposed into several small tasks

`Shed`'s own skeleton (the loop, the status file, the `ShedProducer` interface) is one task on its own — no adapters, no `Publish`/`Finalize`, nothing `loom`-specific.
The engine adapters (`SingleLLMProducer`, `Webster`, the `Burler`-round adapter) are a separate task: each is a small, self-contained wrapper around an already-shipped engine, sharing nothing with `Shed`'s own skeleton beyond the `ShedProducer` interface each implements.
`Publish` and `Finalize` are bundled with neither — they are genuinely new code (see [internal/landingshed](../../internal/landingshed/doc.go)), scoped as their own task, independent of `loom`'s own build order, on the same footing as the not-yet-detailed Plan and Webster phases.
See `manifest/roadmap.md`'s Planned section for the concrete task sequence this decomposes into.

## Why this doc doesn't rewrite loom.md's full detail

`loom.md` is a mature, ~320-line, detailed design (crash recovery, pause semantics, session bootstrap, module decomposition) — this doc's own core model section, including [The `Shed` loop — exact mechanics](#the-shed-loop--exact-mechanics), is now the authoritative description of `Shed`'s generic mechanism (the loop, the status file, the producer contract, engine adapters), and `loom.md`'s own [phase-machine section](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) is the authoritative description of `loom`'s specific producer list built on it.
What this doc does *not* redo is `loom.md`'s remaining `loom`-specific detail — session bootstrap (tmux, the status strand, the run-launcher), auto-mode's human-gate framing, and module decomposition — those stay in `loom.md`, described in `loom`-specific terms, layered on top of the generic loop mechanics this doc now pins.

## Related

- [loom.md](loom.md) — `loom`'s concrete producer list built on `Shed`, plus the remaining `loom`-specific detail (session bootstrap, auto-mode, module decomposition) this doc doesn't restate.
- [`internal/landingshed`](../../internal/landingshed/doc.go) — `Publish`'s and `Finalize`'s own contracts in detail; here each is one producer definition among others, not special-cased.
- [`internal/shedcheck`](../../internal/shedcheck/doc.go) — the authoring-time structural checker over an assembled `OnDone`/`OnStuck` producer graph, detailed in the "Checking an assembled producer list" section above.
- [raddle.md](raddle.md) — the merge-time regeneration decision and merge-lock scope `Finalize`'s own contract must honor, now that Raddle folds into it rather than keeping a separate slot.
- `internal/treadleengine` package documentation — the sibling generic engine (inner round-loop, not outer phase-FSM), and the precedent for `Shed`'s own engine-adapter seam (`RoundRunner`).
- `internal/batcher` package documentation — the other existing precedent for the engine-adapter pattern (`Batcher` interface).
- [hardener.md](hardener.md) — `Hardener` (`Shed` + Hardener's own producer list), Someday.
