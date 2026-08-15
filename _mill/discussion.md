# Discussion: Shed: outer phase-FSM skeleton

```yaml
task: 'Shed: outer phase-FSM skeleton'
slug: shed
status: discussing
parent: main
```

## Problem

`loom` and the eventual `Hardener` both need the same generic outer phase-FSM: a Go engine that walks one flat, ordered list of **producers**, handling resume, crash-recovery, and pause uniformly at producer granularity, with no predefined slots — no Preflight-slot, no Producer-slot, no shared Finalize.
Everything that used to look special is just an entry in that list, so what makes `loom` "loom" versus `Hardener` is purely which producers are in the list and in what order: configuration, not architecture.
That engine is `Shed`.
This task builds its **skeleton alone**.

Why now: `Shed` is item 1 of `manifest/roadmap.md`'s Planned section and nothing blocks it — it is the first task in the sequence.
Three separately-scoped later tasks depend on it: the three engine adapters (`SingleLLMProducer`, the `perch` adapter, the `Webster` adapter), `loom`'s Discussion-phase producers, and eventually `Finalize`.
None of them can start until the `ShedProducer` interface and the loop that drives it exist.

The authoritative design is `manifest/designs/shed.md`, committed to `main` and revised four times during this discussion to close gaps this session found.
**Read it in full before planning** — especially "The `Shed` loop — exact mechanics", which now gives the six-step loop, the exact Go type shapes, and the status-file JSON shape verbatim.
This file records the decisions that discussion produced;
`shed.md` is where the design itself lives.

## Scope

**In:**

- A new package `internal/shedengine` holding the whole skeleton.
- The `ShedProducer` interface, `OutputPointer`, `Outcome`, `ProducerDef`, and the `Shed` struct — the exact shapes pinned in `shed.md`.
- The `Run(ctx) (Result, error)` entrypoint and the six-step loop it walks: read status → look up producer → check pause/cancellation → `Call` → append-and-persist → route on outcome.
- The status-file Go type and its locked, atomic, strict JSON round-trip through `internal/state`, including the opaque `product` passthrough field.
- Pre-loop `Run` work: producer-list validation and non-blocking run-lock acquisition.
- The total bounce budget (`MaxBounces`) guarding an unbounded `OnStuck` cycle.
- Tier 1 tests driving every path with hand-written fake producers.
- A new **Shed Producer-Seam Invariant** in `CONSTRAINTS.md` plus its enforcement test.
- Docs in the same commit: package doc, `manifest/designs/shed.md` status banner, `docs/overview.md` module table and tree, `manifest/roadmap.md` Planned → Done for the **Shed** item.

**Out:**

- **All three engine adapters** — `SingleLLMProducer`, the `perch` adapter, the `Webster` adapter. Their own separately-numbered Planned roadmap item.
- **`Finalize`** — genuinely new code with its own design doc (`manifest/designs/finalize.md`), scoped when `loom`'s producer list reaches it.
- **Anything `loom`-specific**: no real producer list, no `loom` producers, no rewiring of `internal/loomengine` onto `Shed`.
- **`internal/loomengine`'s existing `Status`, `checkCoherence`, `Preflight`, and `docs/reference/status-schema.md`** — left untouched (see the `loom-status-schema-untouched` decision).
- **No cobra module.** No `internal/shedcli`, no `lyx shed` verb, no `cmd/lyx/main.go` registration — so the CLI/Cobra Invariant and the Sandbox Suite Coverage Invariant are not engaged by this task.
- **No seeding.** `Shed` never creates a status file; a product's spawn-time command writes it.
- **No `_lyx` path construction.** `Shed` derives no geometry at all.

## Decisions

### package-location

- Decision: the skeleton lives in `internal/shedengine`.
- Rationale: matches the CLI/Cobra Invariant's `<module>engine` kernel naming that every sibling uses (`treadleengine`, `loomengine`, `perchengine`), and leaves the `shedcli` name free if a verb is ever justified.
- Rejected: `internal/shed` — Shed is a pure library today, but the bare name breaks the repo-wide naming pattern and the invariant's own litmus (returns `(T, error)`, no cobra, no `io.Writer` ⇒ engine).

### no-shedcli

- Decision: no `internal/shedcli` and no `lyx shed` command in this task.
- Rationale: `Shed` has no user-facing verb of its own — the product CLI (`lyx loom run`) constructs a `Shed` with `loom`'s producer list and calls `Run`. A bare `lyx shed` would be a command with no producer list to walk, and registering it would pull in a `Short`, help-tree tests, and a Sandbox Suite Coverage entry for a command that cannot do anything.
- Rejected: adding a thin `shedcli` now for symmetry with other modules.

### outcome-string-typed

- Decision: both `Outcome` (one producer's call) and `RunOutcome` (the whole run's exit) are `string`-typed, with values matching the persisted JSON exactly — `Done = "done"`, `Stuck = "stuck"`, `RunDone = "done"`, `RunBlocked = "blocked"`, `RunPaused = "paused"`.
- Rationale: `internal/treadleengine/result.go:9-16` is `type Outcome string` with string constants for exactly this reason, and the status file's `history[].outcome` and `state` fields are already literal strings in `shed.md`'s own JSON examples. String-typing makes the in-memory value and the on-disk value one vocabulary instead of a hand-maintained int→string mapping.
- Rejected: `int` + `iota` as an earlier draft of `shed.md` pinned — it marshals to `0`/`1` and needs a hand-written `MarshalJSON` to produce the shape the design's own example shows. `shed.md` was corrected upstream (commit `32229ad9`) before this task starts.

### two-outcome-types-not-one

- Decision: `Outcome` and `RunOutcome` stay two distinct types.
- Rationale: they describe different things — one producer's call versus the whole run's exit — and `Done` already names a value on the first, so a shared type forces either a naming collision or an overloaded meaning.
- Rejected: one shared type for both.

### state-type-and-values

- Decision: the persisted `state` field is its own string type in `shedengine` with five values — `"running"`, `"paused"`, `"done"`, `"blocked"`, `"failed"`. Its three clean-exit values are the **literal same strings** as `RunOutcome`'s, so mapping between `Result.Outcome` and `state` is identity, never a lookup table.
- Rationale: `state` is a superset — it must also express `running` (a run in progress or interrupted mid-producer) and `failed` (an engine-level error), neither of which is a `RunOutcome`, since `Run` returns a non-nil `error` rather than a `Result` in the failure case.
- Rejected: reusing `RunOutcome` as the persisted type and adding `running`/`failed` members to it — that would let `Result.Outcome` carry values `Run` can never actually return.

### told-never-derived-paths

- Decision: `Shed` is **told** its status-file path and its lock path via the `StatusPath`/`LockPath` fields on the struct. It never derives either, never calls `lyxcwd`, and never joins an `_lyx`-relative constant of its own.
- Rationale: this is precisely `internal/treadleengine`'s shipped contract (`Engine.Run(p Profile, runDir string)`, `Profile.GateDir`) and the reason its own seam invariant excludes `internal/lyxcwd`. It is also what keeps the Cwd Resolution Invariant intact — a module's durable subdirectory is that module's own concern, and `Shed` is generic across products that will not share one. It makes every test hermetic against a `t.TempDir()`.
- Rejected: `Shed` resolving `_lyx/loom/status.json` itself — bakes one product's geometry into a generic engine, and forces every test to stand up a real anchored worktree.

### loom-status-schema-untouched

- Decision: `internal/loomengine`'s `Status`, `checkCoherence`, `Preflight`, and `docs/reference/status-schema.md` are **not modified** by this task. `shedengine` defines its own status type in its own package. The divergence is recorded in `shedengine`'s package doc.
- Rationale: the two shapes genuinely differ — loom's is `phase`/`stage`/`history[{phase,outcome,bounced_to,ts}]`, Shed's is `current_producer`/`state`/`activity`/`history[{producer,outcome,output,at}]`. Reconciling them means rewiring `loom` onto `Shed`, which is squarely the later "loom: Discussion-phase producers" roadmap item, and rewriting `status-schema.md` now would break the shipped, tested `Preflight`.
- Rejected: adding a "superseded by Shed" banner to `status-schema.md` (edits a pinned contract for a rewiring that has not happened); rewriting both onto Shed's shape now (out of scope and breaks `Preflight`).

### run-entrypoint-result

- Decision: `func (s *Shed) Run(ctx context.Context) (Result, error)`, walking the whole loop in one call until a stopping condition. `Result` carries `Outcome RunOutcome`, `HaltedProducer string`, `Reason string` (set only alongside `RunBlocked`), and `History []HistoryEntry`.
- Rationale: mirrors `internal/treadleengine/result.go:50-55` role-for-role — a terminal string `Outcome`, a reason field populated only on the non-happy terminal, and the per-entry history slice — and `treadleengine.Engine.Run(p Profile, runDir string) (result Result, err error)` for the `(Result, error)` shape. `perch`'s adapter onto treadle is the concrete precedent, not an analogy. Carry treadle's doc-comment discipline too: state that a caller must branch on `Outcome` before reading `Reason`.
- Rejected: `Run` returning only `error` (forces every caller to re-parse state Shed already holds); a `Step()`-per-call API (moves the loop — this task's entire deliverable — out into every caller, so `loom` and `Hardener` each reimplement the sequencing, pause check, and routing that `Shed` exists to centralize).

### no-seeding-hard-error-on-missing

- Decision: a missing status file is a **hard error** from `Run`. `Shed` never seeds one.
- Rationale: `loom`'s already-shipped `Preflight` has a real, tested `CheckSeedMissing` check (`internal/loomengine/preflight.go:135`) that treats a missing status file as a failure, and `status-schema.md` pins the seed as written once by a spawn-time Go command, never by the run loop. If `Shed` auto-seeded, its own seed would always land first and `CheckSeedMissing` would become permanently unreachable dead code. Seeding is a one-time, product-specific bootstrapping act, not part of a per-invocation loop.
- Rejected: seeding at `producers[0]` on missing (what an earlier `shed.md` draft said; corrected upstream in commit `b1b38c5e`).

### unknown-current-producer-hard-error

- Decision: if `current_producer` names a producer not present in `Producers` — the list changed since the file was last written — `Run` returns a hard error, halts, and changes **nothing** on disk.
- Rationale: the standing "never guess a status" discipline (`status-schema.md`: "An unparseable or malformed status file is a hard error; loom never guesses a status"). A human reconciles.
- Rejected: restarting from `producers[0]` (silently re-runs every completed producer, including expensive LLM sessions); advancing to the nearest following match (fabricates a status nobody confirmed).

### state-and-error-fields

- Decision: the status file carries `state` and `error` fields, written at every step-6 exit — `done` on completion, `blocked` (with `error: "bounce budget exhausted"` when that is the cause) on escalation, `failed` with `error: err.Error()` on an engine-level error, `paused` on pause or cancellation, `running` otherwise before looping.
- Rationale: `Result`'s `Outcome`+`Reason` exist only in memory for one `Run` call. Without the on-disk equivalent, a restarted run — or a human reading a future `lyx loom status` — cannot tell "paused, resumable" from "blocked, needs a human" from "crashed": all three look identical, an unattended status file sitting still.
- Rejected: `state` alone with the error text folded into `activity.wait` (buries a hard engine failure in a prose field nothing parses); neither field, with the terminal condition living only in `Result` (does not survive a process exit).

### activity-mechanical-fill

- Decision: `Shed` fills `activity` itself, mechanically, from data it already holds — `now` is `current_producer`'s name, `last` is the most recent `history` entry's producer + outcome formatted for a human, `wait` is set only when `state` is `"blocked"` or `"failed"` (the `error` text or a short reason) and `""` otherwise.
- Rationale: `Shed` is the file's only writer, so if `Shed` does not fill this, nothing can. Every field is either already a `Shed`-owned value or trivially derived from one. Depends on the `state` field existing, so the two decisions fit together.
- Rejected: a caller-supplied `func(...) Activity` hook (indirection with one trivial implementation and zero product variance today); omitting `activity` entirely (drops a field the design's own status-file shape pins).

### total-bounce-budget

- Decision: one **total** bounce budget across the whole run — `MaxBounces int` on `Shed`, decremented on every `Stuck`-routed bounce regardless of which producers are involved. Exhausted behaves exactly like "no `OnStuck` target": `state: "blocked"`, not a distinct third case. `MaxBounces: 0` means "use the internal default"; the default is **10**.
- Rationale: `OnStuck` permits a cycle and every hop can be a full LLM session, so an unbounded cycle is the default outcome whenever a bounced-back producer keeps failing the same way. `internal/treadleengine` already carries the identical discipline for the identical risk shape (a hard round cap). A **per-producer** budget would let an A↔B cycle run 2×budget bounces before either individually trips, which does not bound the thing actually being guarded — total wasted spend before a human is pulled in. The default of 10 matches the magnitude of `perchengine`'s own shipped hard cap (`defaultRoundCaps = []int{5, 8, 10}`, `internal/perchengine/profile.go:43`), where each round is likewise a real LLM spend.
- Rejected: no cap at all as `shed.md` originally had it; a per-`ProducerDef` budget.

### persist-failure-halt

- Decision: if the step-5 persist itself fails (disk full, lock unavailable, `state.WriteJSON` errors), `Run` halts and returns the error immediately, **without** attempting a `state: "failed"` write.
- Rationale: that write is the exact same operation that just failed, so retrying it to record the failure is the one action already known not to work. The file keeps its last-good contents, so `current_producer` still names the producer whose `Call()` just ran and it is simply re-called next time — precisely step 5's already-stated crash semantics, needing no new mechanism.
- Rejected: a best-effort `state: "failed"` write first; retry-with-backoff (the run lock is what prevents contention in the first place).

### ctx-cancellation-as-pause

- Decision: `Run` checks `ctx.Err()` at the top of every loop iteration, alongside `pause_requested`, and treats the two identically: write `state: "paused"`, return `Result{Outcome: RunPaused}` with a **nil** error.
- Rationale: an operator's Ctrl-C or a parent deadline is an operational stop, not a failure, and is exactly as resumable as an explicit pause request — one clean-stop path, not two. Matches `loom.md`'s own "Graceful pause" framing. Checking at the top of each iteration (rather than leaving it to producers) is what stops a cancelled run from launching *new* producer calls for however long the current one takes to notice.
- Rejected: treating cancellation as `failed` with a non-nil error (misrepresents an intentional stop as something broken); not checking `ctx` in the loop at all.

### run-lock

- Decision: `Run` acquires `LockPath` **non-blocking** before step 1, via `lock.TryAcquireWriteLock`. Already held ⇒ return a sentinel error (`ErrShedBusy`) immediately, touching nothing on disk. Released via `defer`.
- Rationale: mirrors `internal/treadleengine/run.go:119-128` exactly (`lock.TryAcquireWriteLock`, `ErrBlockBusy`). `internal/state`'s own per-write lock does not substitute: it is held only for the duration of one write, never across a whole `Call()`, so two concurrent `lyx loom run` invocations could otherwise both read the same `current_producer` and both spawn it, double-spending an LLM session. An OS advisory lock is reclaimed on process death, so a killed run never bricks a later resume.
- Rejected: relying on `internal/state`'s write lock alone; pushing the lock out to the product CLI (every product then reimplements it, and `Shed` owns the loop the lock protects).

### validate-at-run-top

- Decision: `Run` validates `Producers` before step 1 and before acquiring the lock — an empty list, a duplicate `Name`, or an `OnStuck` naming a `Name` absent from the list are each a returned error, before any producer is called. A negative `MaxBounces` is likewise a validation error (`0` means "use the default", per `total-bounce-budget`).
- Rationale: an `OnStuck` typo then fails on the very first invocation rather than only when that producer first goes `Stuck`, hours into a real run, compounding an unrelated failure. Keeps `Shed` the plain exported-field struct `shed.md` pins throughout.
- Rejected: a `New(...) (*Shed, error)` constructor (creates a second, unvalidated door via a bare struct literal alongside the validated one); lazy validation only on the `Stuck` path (worst possible timing).

### product-field-passthrough

- Decision: the status type carries one opaque `product` field (`json.RawMessage`) that `Shed` round-trips **verbatim** and never inspects, validates, or interprets.
- Rationale: `Shed` is the file's only writer and rewrites it whole, but `status-schema.md` requires `loom`'s file to carry `slug`, `parent`, `start_sha`, and `next_action` — fields `Shed` knows nothing about and would otherwise destroy on its next write. An opaque passthrough is the same discipline already established for `OutputPointer.Path` ("`Shed` never introspects it"), and it is the only option compatible with the strict decode below, since the product's keys sit inside one known field rather than as stray top-level keys `DisallowUnknownFields` would reject.
- Rejected: lenient decode into `map[string]any` with a merge-back (loses the fail-loud parse discipline and lets `Shed` silently propagate a corrupt file); a separate product-owned file beside Shed's (splits "loom's single source of truth for orchestration state" across two files, which `status-schema.md` explicitly commits against).

### strict-decode

- Decision: the status file is read via `state.ReadJSONStrict` — `DisallowUnknownFields`, so an unknown or malformed field is a hard error.
- Rationale: not a new rule; `status-schema.md`'s "Parse discipline" section already pins exactly this, and `Shed` inherits it. Works cleanly with `product-field-passthrough`.
- Rejected: `state.ReadJSON` (lenient) — silently ignores what it cannot parse.

### tier1-fake-producer-tests

- Decision: Tier 1, untagged, in-package `shedengine` tests with hand-written fake producers (a `funcProducer` adapter over a closure), against a real status file in `t.TempDir()` through `internal/state`.
- Rationale: zero spawns and zero git, so the Test Tier Purity Invariant holds trivially, while crash-recovery is genuinely exercised — the real risk is whether the `internal/state` round-trip is correct, and a mocked persistence layer cannot exercise that.
- Rejected: mocking persistence behind an interface (the crash-recovery test would then prove nothing about the actual round-trip); an integration-tagged test in a real hub fixture (with no adapters in scope there is nothing real to drive — it would test `hubforge`, not `Shed`).

### shed-producer-seam-invariant

- Decision: add a **Shed Producer-Seam Invariant** to `CONSTRAINTS.md`, modeled on the Treadle Runner-Seam Invariant, with a matching `internal/shedengine/seam_enforcement_test.go` allowlist test in the same commit. `internal/shedengine` production imports are capped at stdlib, `internal/state`, `internal/lock`, and `internal/logger` — never `internal/loomengine`, never any `*engine` adapter package, and never `internal/lyxcwd`, since `Shed` is told its paths and derives none.
- Rationale: the told-never-derived property is the entire reason `Shed` is generic, and it is exactly the property that erodes silently without a machine check. Every import-boundary invariant in this codebase is machine-enforced; the one review-only exception (the Producer Pointer-Rule Invariant) is a content rule, not an import boundary, so it sets no precedent here. Follow Treadle's own wording: policed on **direct** imports only, not the transitive closure, and state plainly what the exclusion buys — that `Shed` is *told* its geometry, never derives it.
- Rejected: a review-obligation-only invariant; no invariant at all.

### docs-and-roadmap

- Decision: four doc updates land in the same commit as the code — (1) `manifest/designs/shed.md`'s status banner flips from "Design sketch, Planned" to reflect the shipped skeleton, with the adapters still Planned; (2) `docs/overview.md` gains a module-table entry and a tree line for `internal/shedengine`; (3) `CONSTRAINTS.md` gains the invariant above; (4) `manifest/roadmap.md` moves the **Shed: shared outer phase-FSM, no predefined slots** item from Planned to Done.
- Rationale: CLAUDE.md's task-completion rule requires the module doc, `docs/overview.md`, and `CONSTRAINTS.md` in the same commit for a change adding a module and cross-cutting infrastructure. The roadmap move is correct and not premature: the adapters are a **separately numbered** Planned item, split that way deliberately this session precisely so the skeleton could land without them.
- Rejected: holding the roadmap move until the adapters land (re-merges two items the roadmap deliberately split); deferring `overview.md` and the roadmap to the adapters task (breaks the same-commit docs rule).

## Technical context

**The authoritative design.**
`manifest/designs/shed.md` (on `main`, revised through commit `32229ad9`) gives the six-step loop, the exact Go type shapes, and the status-file JSON shape verbatim.
Read it in full first — this file records decisions, not the design.

**Closest precedent — `internal/treadleengine`.**
The sibling generic engine, one level down (inner round loop rather than outer phase FSM), and the model for nearly every structural choice here:

- `internal/treadleengine/result.go:9-16` — `type Outcome string` with string constants; `:50-55` — the `Result` shape `Shed`'s mirrors.
- `internal/treadleengine/runner.go:61-63` — the `RoundRunner` seam, the one-method-interface pattern `ShedProducer` repeats a level up.
- `internal/treadleengine/run.go:119-128` — the non-blocking run-lock acquisition (`lock.TryAcquireWriteLock`, `ErrBlockBusy`) `Run`'s own lock copies.
- `internal/treadleengine/state.go` — the told-never-derived split between a durable `runDir` and a never-tracked `scratchDir`, and the `internal/state` usage pattern.
- `CONSTRAINTS.md`'s Treadle Runner-Seam Invariant plus `internal/treadleengine/seam_enforcement_test.go` — the shape the new invariant and its test copy.

**Persistence — `internal/state`.**
`state.ReadJSONStrict[T](path, lockPath)` returns `(zero, false, nil)` on a missing file (so "missing" is distinguishable from an error, which `no-seeding-hard-error-on-missing` needs), and `state.ErrRead`/`state.ErrDecode` sentinel the two failure classes.
`state.WriteJSON[T](path, lockPath, v)` is locked and atomic via `internal/fsx`.
Note `ReadJSONStrict` does **not** create parent directories, unlike `ReadJSON` — correct here, since `Shed` never creates the file.
Do not compose `UpdateJSON` from `ReadJSON`+`WriteJSON`; that package's doc comment explains why.

**The `loom` side, for context only — not to be modified.**
`internal/loomengine/status.go` (`Status`, `HistoryEntry`), `coherence.go` (`checkCoherence`), `preflight.go:135` (`CheckSeedMissing`), and `config.go`'s `LoomStatusFile`/`LoomStatusLock` — the latter now product-scoped to `_lyx/loom/status.json` and `.lyx/loom/status.json.lock` so `Hardener` can coexist.
That rename landed on `main` this session (commit `dc97e980`) and is already merged here.

**Naming caution.**
`internal/state` is the persistence package and `state` is also the status file's own field name.
Pick local identifiers that keep the two apart.

**Not in play.**
No cobra, no `lyxcwd`, no `fabricengine`, no git, no `shuttle`/`reed`.
`Shed` starts no OS processes, so the Live-Substrate Spawn Observability invariant is not engaged either.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `shedengine` resolves no cwd and constructs no `_lyx` path; it is told `StatusPath` and `LockPath`. Never name a variable `root` for a value that is a `cwd`.
- **Lyxdirs Single-Declarer Invariant** — `shedengine` must not name the `_lyx`/`.lyx` literals at all, which the told-never-derived contract already guarantees.
- **Durable-vs-Ephemeral State Invariant** — the status file is durable (`_lyx`-side) and the lock is a never-tracked transient (`.lyx`-side). `Shed` does not choose either location; the caller passes paths already obeying the rule, and `shedengine`'s doc should say so explicitly.
- **CLI / Cobra Invariant** — not engaged: no cobra package in this task. The `<module>engine` naming half is what dictates `internal/shedengine`.
- **Test Tier Purity Invariant** — the tests must stay untagged and spawn nothing: no `gitexec`, no `exec.Command`, no `gitkit.Copy*`, no `hubforge.NewHub`, and no compile-time-constant `time.Sleep` of 1s or more.
- **Hermetic Git Test Environment Invariant** — not engaged, since no test in this package spawns git; do not add a `TestMain` that implies otherwise.
- **Sandbox Suite Coverage** — not engaged: coverage is checked per registered cobra module, and this task registers none.
- **Markdown Link Integrity** — every link added to `manifest/designs/shed.md`, `docs/overview.md`, and `manifest/roadmap.md` must resolve, including `#anchor` fragments on `.md` targets.
- **Documentation Lifecycle** plus CLAUDE.md's task-completion rule — module doc, `docs/overview.md`, and `CONSTRAINTS.md` in the same commit.

New, added by this task:

- **Shed Producer-Seam Invariant** — `internal/shedengine` production imports capped at stdlib, `internal/state`, `internal/lock`, `internal/logger`; never `loomengine`, never an adapter package, never `lyxcwd`. Policed on direct imports only. Enforced by `internal/shedengine/seam_enforcement_test.go`.

Also binding:

- **Semantic line breaks** in every `.md` file touched — one sentence per line, plus a break at internal independent-clause boundaries. No fixed-column hard wrap, no trailing double-space.
- **Worktree isolation** — all work stays in `/home/knatte/Code/loomyard/wts/shed`; never push to `main` from here.

## Testing

All tests are **Tier 1, untagged, in-package** `shedengine`, using a `funcProducer` fake (a closure adapted onto `ShedProducer`) and a real status file under `t.TempDir()` read and written through `internal/state`.
No mocked persistence — the `internal/state` round-trip is the thing most likely to break, so it must be exercised for real.

**TDD candidates** — the pure, table-friendly units, written test-first:

- Producer-list validation: empty list, duplicate `Name`, `OnStuck` naming an absent producer, negative `MaxBounces`. One table, one case per rule, each asserting a distinct error.
- The `activity` fill rule: given a `current_producer`, a `history`, and a `state`, assert the three composed fields — including `wait` being `""` for every non-`blocked`/`failed` state.
- Status-file JSON round-trip: marshal → unmarshal → deep-equal, and a `product` payload surviving a full read-modify-write byte-identically.

**Loop scenarios that must be covered** (each drives `Run` against a purpose-built fake list):

- Happy path — every producer returns `Done`; `state: "done"`, `Result.Outcome == RunDone`, `history` in order, one entry per call.
- `Stuck` with an `OnStuck` target — `current_producer` moves to the named target and that producer runs again; the bounce is recorded in `history`.
- `Stuck` with no target — `state: "blocked"`, `Result.Outcome == RunBlocked`, `Reason` set, `HaltedProducer` naming the right producer.
- Bounce-budget exhaustion — an always-`Stuck` cycle terminates at exactly `MaxBounces` bounces, ending `blocked` with the exhaustion reason rather than looping. Assert the bounce **count**, since that is the guarantee.
- `MaxBounces: 0` resolves to the internal default rather than "no bounces allowed".
- Producer returns `error` — `state: "failed"`, `error` populated, `Run` returns a non-nil error, and **no** further producer is called.
- `pause_requested: true` mid-list — exits before calling the next producer, `state: "paused"`, nil error, `current_producer` unchanged so the next `Run` resumes there.
- Cancelled `ctx` mid-list — same observable outcome as pause, and specifically: no producer is called after cancellation.
- Crash recovery — run a list partway, then construct a **fresh** `Shed` against the same status file and `Run` again; assert it resumes at the persisted `current_producer` and does not re-run completed producers.
- Unconditional re-call — a producer whose `OutputPointer.Path` names an existing file is still called again on resume, never skipped. This is an explicit design guarantee and deserves its own test.
- Missing status file — hard error, and nothing is created on disk.
- `current_producer` naming an absent producer — hard error, and the file is byte-identical afterwards.
- Malformed status file, and one carrying an unknown top-level key — both hard errors via the strict decode.
- Run lock already held — `Run` returns the busy sentinel immediately and the status file is untouched. Acquire the lock directly in the test via `internal/lock`; no second process needed.
- Persist failure — force `state.WriteJSON` to fail (an unwritable directory, or a `StatusPath` whose parent does not exist) and assert `Run` returns the error, the file keeps its last-good contents, and no `state: "failed"` write was attempted.
- `product` passthrough — a status file carrying an arbitrary product payload survives a full `Run` unchanged.

**The seam-enforcement test** follows `internal/treadleengine/seam_enforcement_test.go`: walk the package's non-test `.go` files, collect direct imports, and fail on anything outside the allowlist.

## Q&A log

- **Q:** Package name — `internal/shedengine` or `internal/shed`? **A:** `shedengine`, matching the repo-wide `<module>engine` kernel naming; no `shedcli` in this task, since a bare `lyx shed` would be a command with no producer list to walk.
- **Q:** `Outcome` as `int`+`iota` (as `shed.md` then pinned) or string-typed? **A:** String-typed. Checking `internal/treadleengine.Outcome` directly showed it is `type Outcome string` for exactly this reason, and `shed.md`'s own JSON example already wrote `"outcome": "done"` — the design had implicitly committed to a hand-maintained int→string mapping without noticing. `shed.md` was corrected upstream for both `Outcome` and `RunOutcome`.
- **Q:** Does `Shed` derive its status-file path or is it told? **A:** Told, exactly as treadle is told `runDir` and `Profile.GateDir`.
- **Q:** Does this task reconcile Shed's status shape with `loom`'s pinned `status-schema.md`? **A:** No — out of scope; that is `loom`'s rewiring task. Recorded as a divergence in the package doc instead.
- **Q:** What happens when the status file is missing? **A:** Hard error — reversing `shed.md`'s original "seed at `producers[0]`", which would have made `loom`'s shipped `CheckSeedMissing` permanently unreachable dead code.
- **Q:** What if `current_producer` names a producer no longer in the list? **A:** Hard error, nothing written. Both alternatives guess at what already happened, the failure mode this design rejects everywhere else.
- **Q:** How does a terminal condition survive a process exit? **A:** New `state` and `error` fields on the status file, mirroring `Result`'s `Outcome`+`Reason` split — otherwise "paused", "blocked", and "crashed" are indistinguishable on disk.
- **Q:** Who fills `activity`? **A:** `Shed`, mechanically — it is the file's only writer, so nothing else can. No hook; there is zero product variance today.
- **Q:** Bounce budget — total or per-producer? **A:** Total. A per-producer cap lets an A↔B cycle run 2×cap bounces before either individually trips, which fails to bound the actual risk (runaway LLM spend).
- **Q:** What if the step-5 persist itself fails? **A:** Halt and return the error, with no `state: "failed"` write — that write is the same operation that just failed. The existing crash semantics already cover the recovery; it just needed saying explicitly.
- **Q:** Is `ctx` cancellation a failure or a pause? **A:** A pause — an operational stop, not something broken. Same clean-stop path as `pause_requested`, checked at the top of every iteration so no new producer is launched after cancellation.
- **Q:** Does `internal/state`'s per-write lock prevent two concurrent runs? **A:** No — it is held only for one write, not across a whole `Call()`, so both runs could read the same `current_producer` and both spawn it. Hence a run-held lock, mirroring treadle's `run.lock`/`ErrBlockBusy`.
- **Q:** Validate the producer list in a `New()` constructor or at the top of `Run`? **A:** At the top of `Run` — a constructor would leave a bare struct literal as a second, unvalidated door.
- **Q:** How do `loom`'s `slug`/`parent`/`start_sha`/`next_action` survive Shed rewriting the file? **A:** An opaque `product` (`json.RawMessage`) field Shed round-trips verbatim — the same "never introspects it" discipline as `OutputPointer.Path`, and the only option compatible with the strict decode.
- **Q:** Strict or lenient decode? **A:** Strict — `status-schema.md`'s Parse discipline already pins `ReadJSONStrict` and `DisallowUnknownFields`; Shed inherits it rather than inventing a rule.
- **Q:** Mock the persistence layer in tests? **A:** No — that defeats the point of the crash-recovery test, whose real risk is the `internal/state` round-trip itself.
- **Q:** New `CONSTRAINTS.md` invariant, and machine-enforced or review-only? **A:** Machine-enforced, modeled on the Treadle Runner-Seam Invariant. Every import-boundary invariant here gets a test; the one review-only exception is a content rule, not an import boundary.
- **Q:** Does the roadmap item move to Done in this commit? **A:** Yes — the skeleton and the adapters were split into two separately-numbered Planned items this session precisely so one could land without the other.
