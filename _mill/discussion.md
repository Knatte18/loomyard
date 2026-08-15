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
- The status-file Go types — `Status` (the file itself) and `HistoryEntry` (one `history` element, also the element type of `Result.History`) — with their JSON tags, plus their locked, atomic round-trip through `internal/state`: a strict read gate and a read-modify-write persist, including the opaque `product` passthrough field.
- Pre-loop `Run` work: producer-list validation, the two-distinct-lock-paths check, and non-blocking run-lock acquisition.
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

### unrecognised-outcome-is-an-engine-error

- Decision: an `Outcome` that is neither `Done` nor `Stuck` — `Outcome("")`, `Outcome("approved")`, a typo, anything — returned with a **nil** error is treated as an engine-level failure: `state: "failed"`, `error` naming the offending value and producer, `Run` returns a non-nil error. The `history` entry is still appended, recording the **literal value received**, so a human diagnosing it can see exactly what the adapter returned.
- Rationale: `Outcome` is a `string` type, which is open — `outcome-string-typed` bought JSON-vocabulary unity at the cost of an unbounded value space, and the routing in step 6 covers only three cases (`error`, `Stuck`, `Done`). Without this, an adapter returning a fourth value falls through to undefined behaviour: silently treated as one of the two known outcomes, or dropped. Hard-failing is the only disposition consistent with "never guess a status" — the same principle that makes an unknown `current_producer` a hard error rather than a best-guess advance.
- **Producer-contract obligation:** a `ShedProducer` must return exactly `Done` or `Stuck`, alongside the cancellation obligation in `ctx-cancellation-as-pause`. Both belong in `shed.md`'s contract section.
- **`shed.md` reconciliation:** `shed.md:27` still describes the contract as "done / approved / stuck / blocked" — **four** values — while its own Go block (`shed.md:80-84`) declares two. That prose predates the two-value contract and must be corrected, not preserved.
- Rejected: silently coercing an unknown value to `Stuck` (guesses a verdict, and a `Stuck` guess consumes bounce budget for what is actually a broken adapter); coercing to `Done` (advances past a producer that may not have done its work); ignoring the case (undefined behaviour in a design that pins every other edge).

### two-outcome-types-not-one

- Decision: `Outcome` and `RunOutcome` stay two distinct types.
- Rationale: they describe different things — one producer's call versus the whole run's exit — and `Done` already names a value on the first, so a shared type forces either a naming collision or an overloaded meaning.
- Rejected: one shared type for both.

### state-type-and-values

- Decision: the persisted `state` field is its own string type in `shedengine` with five values — `"running"`, `"paused"`, `"done"`, `"blocked"`, `"failed"`. Its three clean-exit values are the **literal same strings** as `RunOutcome`'s, so mapping between `Result.Outcome` and `state` is identity, never a lookup table.
- Rationale: `state` is a superset — it must also express `running` (a run in progress or interrupted mid-producer) and `failed` (an engine-level error), neither of which is a `RunOutcome`, since `Run` returns a non-nil `error` rather than a `Result` in the failure case.
- Rejected: reusing `RunOutcome` as the persisted type and adding `running`/`failed` members to it — that would let `Result.Outcome` carry values `Run` can never actually return.

### told-never-derived-paths

- Decision: `Shed` is **told** all three of its paths via struct fields — `StatusPath` (the durable status file), `LockPath` (the run lock), and `StatusLockPath` (the lock `internal/state` itself takes). It never derives any of them, never calls `lyxcwd`, and never joins an `_lyx`-relative constant of its own.
- Rationale: this is precisely `internal/treadleengine`'s shipped contract (`Engine.Run(p Profile, runDir string)`, `Profile.GateDir`) and the reason its own seam invariant excludes `internal/lyxcwd`. It is also what keeps the Cwd Resolution Invariant intact — a module's durable subdirectory is that module's own concern, and `Shed` is generic across products that will not share one. It makes every test hermetic against a `t.TempDir()`.
- Rejected: `Shed` resolving `_lyx/loom/status.json` itself — bakes one product's geometry into a generic engine, and forces every test to stand up a real anchored worktree.

### loom-status-schema-untouched

- Decision: `internal/loomengine`'s `Status`, `checkCoherence`, `Preflight`, and `docs/reference/status-schema.md` are **not modified** by this task. `shedengine` defines its own status type in its own package. The divergence is recorded in `shedengine`'s package doc.
- Rationale: the two shapes genuinely differ — loom's is `phase`/`stage`/`history[{phase,outcome,bounced_to,ts}]`, Shed's is `current_producer`/`state`/`activity`/`history[{producer,outcome,output,at}]`. Reconciling them means rewiring `loom` onto `Shed`, which is squarely the later "loom: Discussion-phase producers" roadmap item, and rewriting `status-schema.md` now would break the shipped, tested `Preflight`.
- Rejected: adding a "superseded by Shed" banner to `status-schema.md` (edits a pinned contract for a rewiring that has not happened); rewriting both onto Shed's shape now (out of scope and breaks `Preflight`).

### run-entrypoint-result

- Decision: `func (s *Shed) Run(ctx context.Context) (Result, error)`, walking the whole loop in one call until a stopping condition. `Result` carries `Outcome RunOutcome`, `HaltedProducer string`, `Reason string` (set only alongside `RunBlocked`), and `History []HistoryEntry`.
- Rationale: mirrors `internal/treadleengine/result.go:50-55` role-for-role — a terminal string `Outcome`, a reason field populated only on the non-happy terminal, and the per-entry history slice — and `treadleengine.Engine.Run(p Profile, runDir string) (result Result, err error)` for the `(Result, error)` shape. `perch`'s adapter onto treadle is the concrete precedent, not an analogy. Carry treadle's doc-comment discipline too: state that a caller must branch on `Outcome` before reading `Reason`.
- **`Result` is meaningless unless `error` is nil, and the doc comment must say so.** `RunOutcome`'s zero value is `""`, which is not one of the three legal constants, and every hard-error path — validation failure, busy lock, missing or incoherent status file, persist failure — returns an unpopulated `Result` alongside its error. A caller must check `error` first and never inspect `Outcome` on a non-nil-error return. This is the same discipline as branching on `Outcome` before reading `Reason`, one level up.
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

### field-ownership-split

- Decision: `Shed` is **not** the status file's only writer, and the design must not claim it is. Ownership splits explicitly:
  - **Shed-owned, rewritten on every persist:** `current_producer`, `state`, `error`, `activity`, `history`.
  - **Shared, write-to-clear:** `pause_requested` — an outside actor (a product's pause verb) sets it **true**; `Shed` never sets it true and only ever writes it **false**, exactly once, in the same persist that records `state: "paused"` (see `pause-is-a-consumed-request`).
  - **External-writer-owned, only ever read and carried through by Shed:** `product`, the opaque product payload.
- Rationale: `status-schema.md:69` pins `pause_requested` as kept **in-status**, deliberately diverging from webster's separate flag file, which means an outside actor writes the same file `Shed` writes. The seed itself is likewise written by a spawn-time command, not by `Shed`. Asserting sole ownership would license exactly the whole-file clobber the persist decision below exists to prevent.
- Rejected: moving pause to a separate flag file so `Shed` genuinely is the only writer — clean in isolation, but it reverses a decision `status-schema.md` pins deliberately and puts this task in the business of redesigning `loom`'s pause contract, which is out of scope.

### reread-and-merge-persist

- Decision: three changes to the loop, together:
  1. Step 6 routes back to **step 1**, not step 2 — the status file is re-read (via `state.ReadJSONStrict`) at the top of every iteration, so step 3's `pause_requested` check sees a pause requested *during* the producer call that just returned.
  2. Step 5 persists via `state.UpdateJSON` (locked read-modify-write), whose mutate function overwrites only the Shed-owned fields above and carries `pause_requested` and `product` forward from the on-disk copy.
  3. **Routing is computed before persisting, and steps 5 and 6 share exactly one `UpdateJSON` call per iteration** — the `history` append, the new `current_producer`, and the new `state`/`error` all land in a single mutate.
  4. **The mutate function returns an error when `UpdateJSON` reports `found == false`**, so the persist aborts without writing.
- Rationale: as originally specified, the loop read the file once and rewrote it whole from an in-memory copy, so a pause requested during a 20-minute producer call was both **never observed** (step 6 routed to step 2, past the step-3 check's data source) and **silently destroyed** (step 5's blind rewrite), along with any external `product` update. `state.UpdateJSON` exists for exactly this hazard — it holds one exclusive lock across read, mutate, and write, so a concurrent writer's value can never be clobbered by a payload composed from a base that writer already superseded (`internal/state/state.go:99-133`).
- **Rationale for the `found == false` guard — it is what keeps `no-seeding-hard-error-on-missing` true at the persist as well as at the read.** `state.UpdateJSON` treats a missing file as a non-error: it calls `mutate(zero, false)` and then writes whatever comes back (`internal/state/state.go:110-133`). So if the status file is deleted, or its directory replaced, between step 1's read and step 5's persist, `Shed` would silently **create** a status file — directly contradicting "Shed never creates a status file", and seeding one from a zero value at that. Returning an error from the mutate on `found == false` makes the whole persist abort untouched, and routes into the `persist-failure-halt` path, which already halts without a compensating write.
- Rationale for the single persist: written as two separate writes — step 5 appending `history`, step 6 then writing `state` and advancing `current_producer` — a crash landing between them leaves `current_producer` still naming the producer that just finished, so the next `Run` re-calls it and appends a **duplicate** `history` entry. That defeats the very crash-safety property step 5 exists to provide ("a crash after it means step 6 already knows where to go"), which only holds if "after it" and "step 6 decided" are the same instant. Computing the route first and committing it atomically makes the persist the single commit point of the whole iteration.
- Consequence for `shed.md`: the loop's step-6 text ("back to step 2"), the "Shed is the file's only writer" sentence under `activity`, and the step-5/step-6 split as two separate persists must all be corrected in this task's commit — see `docs-and-roadmap` for the full inventory.
- Rejected: re-reading only `pause_requested` before each step-3 check (sees the pause, still clobbers external `product` writes on every persist); leaving the loop as-is and accepting lost pauses; two persists per iteration with a duplicate `history` entry on crash accepted as harmless (it is not — `history` is the audit trail a human and a future progress judge both read, and a phantom re-run entry misrepresents what happened).

### pause-is-a-consumed-request

- Decision: `pause_requested` is a **request `Shed` consumes**, not a latch. When step 3 observes it set, the same persist that writes `state: "paused"` also writes `pause_requested: false`. `Shed` never sets the flag true — only an outside actor does.
- Rationale: without this, the flag is never cleared by anyone, so the *next* `Run` re-reads a still-true flag at step 3 and pauses again immediately, forever — the run is permanently unresumable. `internal/treadleengine/run.go:132` has a `clearPauseFlag` call for precisely this reason: a resumed run must not re-pause on the flag it is resuming from. Consuming the flag at the moment it is honoured is better than clearing it on the resuming run, because it leaves no window in which a stale true flag exists on disk at all; the durable record of "this run is paused" is `state: "paused"`, which is what a human or `lyx loom status` reads.
- Consequence: pause is the one field crossing the ownership boundary, so `field-ownership-split` names it separately rather than lumping it with `product`.
- Rejected: the product's resume verb clearing it (splits one invariant across two modules, and a product that forgets leaves an unresumable task); leaving it set and having `Shed` compare against `state` to infer whether a pause is fresh (inference where a written value would do).

### terminal-state-on-completion-and-rerun

- Decision: two values the design left undefined, pinned together:
  - On `RunDone`, `current_producer` keeps the **last producer's name** — never `""`. Correspondingly `activity.now` is that name and `Result.HaltedProducer` is that name.
  - A `Run` against a status file already carrying `state: "done"` returns `Result{Outcome: RunDone}` **immediately**, calling no producer and writing nothing. `state: "blocked"` and `state: "failed"` do **not** short-circuit: `Run` proceeds normally and re-calls `current_producer`, which is exactly how a human resumes after fixing whatever caused the halt, and the state field is overwritten as the loop advances.
  - **The short-circuit's `Result` is filled from the file it just read, and it writes nothing.** `HaltedProducer` takes `current_producer`'s value from that file and `History` takes the file's `history`, exactly as a non-short-circuited return would. Otherwise the two rules disagree — `HaltedProducer` is defined as "the producer `current_producer` named when `Run` returned" and `History` as "the full persisted history as it stands when `Run` returns", neither of which is satisfied by a bare `Result{Outcome: RunDone}`. Filling both makes a re-run's `Result` **identical** to the original completing run's, which is what idempotence should mean at the API surface and not merely on disk.
  - **Exact position of the short-circuit: after step 1's read, before step 2's lookup.** The consequence, stated so it is a decision rather than an accident: a `done` file whose `current_producer` is no longer in the list returns `RunDone` cleanly and does **not** hard-error. A finished task must not become un-queryable because someone later edited the producer list — the run is over, and step 2's "never guess a status" protection exists to stop a *live* run resuming into the wrong producer, a risk that does not exist once nothing more will be called.
  - Rationale: `activity.now` is defined as "`current_producer`'s name" and `Result.HaltedProducer` as "the producer `current_producer` named when `Run` returned", so both were undefined at the happy-path terminal the tests assert on. Keeping the last name beats `""` because it leaves both fields meaningful — a reader of a finished status file sees which producer ended the run. (It also keeps the value valid for step 2 should the short-circuit ever be removed, but that is a secondary benefit, not the reason: with the short-circuit positioned before the lookup, a `done` file never reaches step 2 at all.) The `done` short-circuit is what makes `Run` idempotent — without it, a second `lyx loom run` on a finished task re-calls the final producer, which for a `Finalize`-shaped producer means re-running a merge.
- Rejected: `current_producer: ""` on completion; a sentinel like `"<done>"` (a value that is not a producer name, defeating the step-2 lookup's own error message); short-circuiting on `blocked`/`failed` too (would make a halted task unresumable without hand-editing the status file).

### strictness-is-scoped-to-the-read-gate

- Decision: strict decoding is the contract of the **top-of-iteration read** (`state.ReadJSONStrict`), not of the persist's internal merge base. `state.UpdateJSON` re-reads through `readJSONUnlocked` — plain `json.Unmarshal`, no `DisallowUnknownFields` (`internal/state/state.go:122`, `:80-97`) — and that is accepted rather than worked around.
- Rationale: this would otherwise be a silent contradiction between `strict-decode` and `reread-and-merge-persist`, so it is stated outright. Malformed JSON still fails loud on the merge path (`json.Unmarshal` errors, so `UpdateJSON` returns an error and the persist-failure path takes over). The one behaviour leniency permits that strictness would not is an **unknown top-level key written by an external actor after step 1's read**, and its fate must be stated honestly: it is **silently destroyed**, not surfaced. The persist marshals Shed's full struct (`internal/state/state.go:47-54`, `:127-132`), so any key absent from that struct is simply not written back, and the next iteration's strict read then sees a clean file and has nothing to reject.
- **Why that is acceptable:** `product` is the sanctioned channel for everything an external writer legitimately owns, and `pause_requested` is the one shared field. A top-level key outside those two is not a supported extension point — it is a mistake, and one nothing in this design promises to preserve. What is *not* acceptable is claiming the key would be caught later; it would not. A key present *before* step 1 is a different case and does hard-error at the gate, exactly as `strict-decode` says.
- What must not happen is `Shed` reaching for `ReadJSON` at the gate to make the two paths match; the gate stays strict.
- Rejected: adding a strict variant of `UpdateJSON` to `internal/state` (out of scope, and a change to a primitive four modules share); dropping the strict gate to match the merge (loses the fail-loud parse discipline `status-schema.md` pins).

### activity-mechanical-fill

- Decision: `Shed` fills `activity` itself, mechanically, from data it already holds:
  - `now` — `current_producer`'s name.
  - `last` — the most recent `history` entry, composed as exactly `"<producer> → <outcome>"` (e.g. `"Plan-Write → done"`), and `""` when `history` is empty. The format is pinned rather than left to judgment because a TDD-candidate test asserts this field; an unpinned "formatted for a human" cannot be asserted, only approximated.
  - `wait` — set only when `state` is `"blocked"` or `"failed"` (the `error` text, or a short reason), `""` otherwise.
- Rationale: `activity` is a Shed-owned field per `field-ownership-split`, composed entirely from values `Shed` already holds — no external actor has the data to fill it, and no product needs it filled differently. Depends on the `state` field existing, so the two decisions fit together.
- Rejected: a caller-supplied `func(...) Activity` hook (indirection with one trivial implementation and zero product variance today); omitting `activity` entirely (drops a field the design's own status-file shape pins).

### total-bounce-budget

- Decision: one **total** bounce budget per `Run` **call**, held in memory — `MaxBounces int` on `Shed`, decremented on every `Stuck`-routed bounce regardless of which producers are involved.
- **Exact boundary semantics:** `MaxBounces` bounces are **permitted**; the `(MaxBounces + 1)`-th `Stuck` that would otherwise route to an `OnStuck` target is the one refused, writing `state: "blocked"` with the exhaustion reason. So with `MaxBounces: 3`, three bounce-backs occur and the fourth `Stuck` blocks. This is pinned rather than left to implementation judgment because it is the classic off-by-one seam, and because the test asserting "the bounce count" needs an unambiguous target — every other exact boundary in this document is pinned, so leaving this one loose would be the sole exception. It is deliberately **not** persisted: the status file carries no bounces-used field, so a crash-restart or a human-resumed `blocked` run starts again with the full budget.
- Scope rationale: "run" means one `Run` invocation, not the task's whole lifetime. That is the right scope because the budget guards *unattended* runaway spend inside a single invocation, and every event that resets it — a crash-restart, or a human resuming a `blocked` task — is a new human-initiated invocation where a person has already been pulled in, which is precisely the outcome the budget exists to force. Persisting it would also mean a legitimate resume inherits an exhausted budget and blocks immediately, and it would add a field to a status shape that is otherwise fully pinned. The `history` trail still records every bounce across every invocation, so nothing is hidden from a human reading the file. Exhausted behaves exactly like "no `OnStuck` target": `state: "blocked"`, not a distinct third case. `MaxBounces: 0` means "use the internal default"; the default is **10**.
- Rationale: `OnStuck` permits a cycle and every hop can be a full LLM session, so an unbounded cycle is the default outcome whenever a bounced-back producer keeps failing the same way. `internal/treadleengine` already carries the identical discipline for the identical risk shape (a hard round cap). A **per-producer** budget would let an A↔B cycle run 2×budget bounces before either individually trips, which does not bound the thing actually being guarded — total wasted spend before a human is pulled in. The default of 10 matches the magnitude of `perchengine`'s own shipped hard cap (`defaultRoundCaps = []int{5, 8, 10}`, `internal/perchengine/profile.go:43`), where each round is likewise a real LLM spend.
- Rejected: no cap at all as `shed.md` originally had it; a per-`ProducerDef` budget.

### persist-failure-halt

- Decision: if the step-5 persist itself fails (disk full, lock unavailable, `state.WriteJSON` errors), `Run` halts and returns the error immediately, **without** attempting a `state: "failed"` write.
- Rationale: that write is the exact same operation that just failed, so retrying it to record the failure is the one action already known not to work. The file keeps its last-good contents, so `current_producer` still names the producer whose `Call()` just ran and it is simply re-called next time — precisely step 5's already-stated crash semantics, needing no new mechanism.
- Rejected: a best-effort `state: "failed"` write first; retry-with-backoff (the run lock is what prevents contention in the first place).

### ctx-cancellation-as-pause

- Decision: `Run` checks `ctx.Err()` at the top of every loop iteration, alongside `pause_requested`, and treats the two identically: write `state: "paused"`, return `Result{Outcome: RunPaused}` with a **nil** error.
- **Step 6's error branch is also cancellation-aware, which the top-of-iteration check alone does not cover.** The common Ctrl-C case is cancellation arriving *during* a producer call, and a well-behaved producer handed a cancelled `ctx` returns an error from `Call` — which the error branch would otherwise write as `state: "failed"` with a non-nil `Run` error. That is precisely the "misrepresents an intentional stop as something broken" outcome this decision rejects, and it would break the "one clean-stop path, not two" claim in the most common case of all. So: when `Call` returns a non-nil error **and `ctx.Err() != nil`**, the iteration routes to the pause exit instead of the failure exit — `state: "paused"`, `RunPaused`, nil error.
- **The predicate is `ctx.Err() != nil`, not `errors.Is(err, context.Canceled)`.** The context's own state is ground truth about whether an operator stopped the run, and it stays correct even if a producer wraps or discards the sentinel. The `errors.Is` form would be actively wrong in the opposite direction: a producer whose *own* internal derived context times out returns `context.DeadlineExceeded` while the parent `ctx` is perfectly healthy, and that is a genuine producer failure, not an operator stop.
- **Producer-contract obligation, stated so a future adapter cannot get it wrong:** a `ShedProducer` **must** surface context cancellation as a non-nil `error` from `Call`, never as `(Stuck, ..., nil)`. `Shed` cannot detect the difference — a `Stuck` return with a cancelled `ctx` is indistinguishable to it from a genuine producer verdict — so a producer that reports cancellation as `Stuck` would silently consume bounce budget, or escalate to `blocked`, for what was actually an operator Ctrl-C: exactly the misrepresentation this decision exists to prevent for the `error` case. Returning an error on cancellation is idiomatic Go and the natural thing any adapter would do, so this is cheap to honour — but three of the four planned adapters (`perch`, `Webster`, a bespoke multi-spawn engine) own their own error taxonomies and are not designed yet, so it is written down here rather than assumed. This belongs in `shed.md`'s `ShedProducer` contract section, not only in this file.
- **No `history` entry is appended on the cancellation path**, and `current_producer` is left unchanged. The producer never reached a verdict, so there is nothing to record; leaving `current_producer` put means the next `Run` simply re-calls it, which is the same semantics as a crash before step 5. The accepted trade is that a producer returning a genuine, unrelated error in the same instant an operator hits Ctrl-C is reported as a pause — harmless, because the producer is re-called on resume and the real error surfaces again then.
- Rationale: an operator's Ctrl-C or a parent deadline is an operational stop, not a failure, and is exactly as resumable as an explicit pause request — one clean-stop path, not two. Matches `loom.md`'s own "Graceful pause" framing. Checking at the top of each iteration (rather than leaving it to producers) is what stops a cancelled run from launching *new* producer calls for however long the current one takes to notice.
- Rejected: treating cancellation as `failed` with a non-nil error (misrepresents an intentional stop as something broken); not checking `ctx` in the loop at all.

### run-lock

- Decision: `Run` acquires `LockPath` **non-blocking** before step 1, via `lock.TryAcquireWriteLock`. Already held ⇒ return a sentinel error (`ErrShedBusy`) immediately, without reading or writing the status file. Released via `defer`.
- **`Shed` creates each lock path's parent directory** (`os.MkdirAll`) before acquiring, for both `LockPath` and `StatusLockPath`. `internal/lock` opens the lock file with `O_CREATE` but never creates parents (`internal/lock/lock.go:22-23`), which is why `internal/loomengine/preflight.go:151` and `internal/treadleengine/run.go:102-119` both `MkdirAll` before locking. Without it, a first run against a not-yet-existing `.lyx/loom/` fails with a raw lock-acquire error instead of running. This is not path *derivation* — the paths are still told, `Shed` only ensures the told path is usable — so `told-never-derived-paths` is unaffected.
- **Wording correction:** acquiring a lock is not a no-op on disk — the lock file itself is created. The honest claim is that `Shed` never creates or modifies the **status file** outside step 5's persist. Any "touches nothing on disk" phrasing in the design doc must be corrected to that.
- Rationale: mirrors `internal/treadleengine/run.go:119-128` exactly (`lock.TryAcquireWriteLock`, `ErrBlockBusy`). `internal/state`'s own per-write lock does not substitute: it is held only for the duration of one write, never across a whole `Call()`, so two concurrent `lyx loom run` invocations could otherwise both read the same `current_producer` and both spawn it, double-spending an LLM session. An OS advisory lock is reclaimed on process death, so a killed run never bricks a later resume.
- Rejected: relying on `internal/state`'s write lock alone; pushing the lock out to the product CLI (every product then reimplements it, and `Shed` owns the loop the lock protects).

### two-lock-paths-never-the-same-file

- Decision: the run lock and the `internal/state` lock are **two distinct, separately-supplied paths** — `LockPath` and `StatusLockPath`. `Shed` passes `StatusLockPath` (never `LockPath`) to every `state.ReadJSONStrict`/`state.UpdateJSON` call. `Run`'s pre-loop validation rejects a `Shed` whose `LockPath` and `StatusLockPath` name the same file, and both fields' doc comments state the rule outright.
- Rationale: `state.WriteJSON`/`UpdateJSON` acquire the **blocking** `lock.AcquireWriteLock` (`internal/state/state.go:34`, `:116`), and `UpdateJSON`'s own doc comment (`state.go:108-109`) says nesting on the same path "hangs rather than failing". With one shared path, `Run` would deadlock on its first persist — a hang, not an error, which is the worst possible failure shape. `internal/treadleengine` already keeps `run.lock` and `state.json.lock` deliberately distinct (see `internal/treadleengine/state.go`'s header comment). The caller already has both paths on hand: `loomengine.LoomStatusLock` returns the `.lyx`-side state lock today, and the run lock is its sibling.
- Rejected: deriving the state lock internally as `StatusPath + ".lock"` — `Shed` would be constructing a path, breaking told-never-derived, and it would place the lock beside the durable status file under `_lyx` rather than at its mirrored `.lyx` subpath, violating the Durable-vs-Ephemeral State Invariant. Also rejected: one lock serving both by reaching into `internal/state`'s unlocked internals, which are unexported and out of scope to change.

### validate-at-run-top

- Decision: `Run` validates before step 1 and before acquiring the lock. Every one of these is a returned error, before any producer is called:
  - an empty `Producers` list;
  - a duplicate `Name`;
  - an `OnStuck` naming a `Name` absent from the list;
  - **an empty `Name`** on any `ProducerDef`;
  - **a nil `Producer`** on any `ProducerDef`;
  - a negative `MaxBounces` (`0` means "use the default", per `total-bounce-budget`);
  - `LockPath` equal to `StatusLockPath` (per `two-lock-paths-never-the-same-file`), or any of the three paths being empty.
- **Why an empty `Name` must be rejected specifically:** `""` is already load-bearing twice over. It is `OnStuck`'s "escalate to human" sentinel, so a producer literally named `""` would make an `OnStuck: ""` ambiguous between "escalate" and "bounce to that producer". It is also the zero value a malformed or partial seed leaves in `current_producer`, which step 2 would then look up successfully and run — turning a corrupt status file into a silently-executed producer instead of the hard error `unknown-current-producer-hard-error` promises.
- **Why a nil `Producer` must be rejected:** it panics at step 4 rather than failing loud, and a panic inside a long unattended run is strictly worse than a validation error at second zero.
- Rationale: an `OnStuck` typo then fails on the very first invocation rather than only when that producer first goes `Stuck`, hours into a real run, compounding an unrelated failure. Keeps `Shed` the plain exported-field struct `shed.md` pins throughout.
- Rejected: a `New(...) (*Shed, error)` constructor (creates a second, unvalidated door via a bare struct literal alongside the validated one); lazy validation only on the `Stuck` path (worst possible timing).

### product-field-passthrough

- Decision: the status type carries one opaque `product` field (`json.RawMessage`) that `Shed` round-trips **verbatim** and never inspects, validates, or interprets.
- Rationale: `Shed` rewrites the file's Shed-owned fields on every persist, so a product needs somewhere durable to keep state `Shed` knows nothing about. An opaque passthrough is the same discipline already established for `OutputPointer.Path` ("`Shed` never introspects it"), and it is the only option compatible with the strict read gate, since the product's keys sit inside one known field rather than as stray top-level keys `DisallowUnknownFields` would reject.
- **Carries no compatibility claim for `loom`'s shipped schema.** A Shed-written status file would still fail `loomengine.checkCoherence`: `status-schema.md` mandates `phase`, `stage`, and `narration` as **top-level** fields and pins a `{phase, outcome, bounced_to, ts}` history shape, none of which a `product` sub-object satisfies. `product` is a generic mechanism for whatever a product needs to keep, not a bridge to `loom`'s current schema — reconciling those two shapes is the later `loom` rewiring task, exactly as `loom-status-schema-untouched` states. This wording must not drift into the package-doc divergence note as an implied compatibility guarantee.
- Rejected: lenient decode into `map[string]any` with a merge-back (loses the fail-loud parse discipline and lets `Shed` silently propagate a corrupt file); a separate product-owned file beside Shed's (splits "loom's single source of truth for orchestration state" across two files, which `status-schema.md` explicitly commits against).

### strict-decode

- Decision: the status file is read via `state.ReadJSONStrict` — `DisallowUnknownFields`, so an unknown or malformed field is a hard error.
- Rationale: not a new rule; `status-schema.md`'s "Parse discipline" section already pins exactly this, and `Shed` inherits it. Works cleanly with `product-field-passthrough`.
- Rejected: `state.ReadJSON` (lenient) — silently ignores what it cannot parse.

### timestamps-and-result-history-scope

- Decision: two one-line pins the design left unstated.
  - `history[].at` is **RFC3339 UTC**, written from a direct `time.Now().UTC()` call — no injectable clock field, since that would add a field to the struct shape `shed.md` pins, for a value tests can assert structurally instead. Tests assert that each `at` parses as RFC3339 with a zero UTC offset and that entries are non-decreasing, never an exact literal.
  - `Result.History` is the **full persisted history** as it stands when `Run` returns — every entry in the status file, not only the entries this invocation appended.
- Rationale: RFC3339 UTC matches the timestamp rule `status-schema.md` already pins for every timestamp field, and `loomengine.isRFC3339UTC` (`internal/loomengine/coherence.go:103-110`) is the existing in-repo validator for exactly that shape. Full-history scope makes `Result` a faithful view of the file `Shed` just wrote, which is what the crash-recovery test needs to assert against — a this-run-only slice would make a resumed run's `Result` silently incomparable to a fresh one's.
- Rejected: an injectable clock (changes a doc-pinned struct for test convenience alone); this-run-only `History` scope.

### tier1-fake-producer-tests

- Decision: Tier 1, untagged, in-package `shedengine` tests with hand-written fake producers (a `funcProducer` adapter over a closure), against a real status file in `t.TempDir()` through `internal/state`.
- Rationale: zero spawns and zero git, so the Test Tier Purity Invariant holds trivially, while crash-recovery is genuinely exercised — the real risk is whether the `internal/state` round-trip is correct, and a mocked persistence layer cannot exercise that.
- Rejected: mocking persistence behind an interface (the crash-recovery test would then prove nothing about the actual round-trip); an integration-tagged test in a real hub fixture (with no adapters in scope there is nothing real to drive — it would test `hubforge`, not `Shed`).

### shed-producer-seam-invariant

- Decision: add a **Shed Producer-Seam Invariant** to `CONSTRAINTS.md`, modeled on the Treadle Runner-Seam Invariant, with a matching `internal/shedengine/seam_enforcement_test.go` allowlist test in the same commit. `internal/shedengine` production imports are capped at **stdlib, `internal/state`, and `internal/lock`** — never `internal/loomengine`, never any `*engine` adapter package, and never `internal/lyxcwd`, since `Shed` is told its paths and derives none.
- **`internal/logger` is deliberately excluded.** No decision in this file gives `shedengine` anything to log — it starts no OS process, so the Live-Substrate Spawn Observability invariant does not engage, and the product CLI owns operator-facing output. Excluding it also buys a genuinely stronger property than treadle's: `internal/logger` imports `internal/lyxcwd` (`internal/logger/sink.go:21`), whereas `internal/lock` imports no internal package at all and `internal/state` imports only `internal/fsx` and `internal/lock`. So for `shedengine` alone, "never `internal/lyxcwd`" holds **transitively**, not merely on direct imports — a claim treadle explicitly cannot make.
- Rationale: the told-never-derived property is the entire reason `Shed` is generic, and it is exactly the property that erodes silently without a machine check. Every import-boundary invariant in this codebase is machine-enforced; the one review-only exception (the Producer Pointer-Rule Invariant) is a content rule, not an import boundary, so it sets no precedent here. Follow Treadle's wording for the **policing** half — direct imports only, not the transitive closure, since that is what the test actually checks — while stating the stronger transitive fact above as an observation about today's allowlist, not as something the test enforces. Do not copy treadle's "excluding it buys no isolation" caveat verbatim: for this allowlist it would be false.
- Rejected: keeping `internal/logger` on the allowlist for future convenience (nothing needs it, and it would forfeit the transitive property for zero present benefit).
- Rejected: a review-obligation-only invariant; no invariant at all.

### docs-and-roadmap

- Decision: four doc updates land in the same commit as the code — (1) `manifest/designs/shed.md` is **reconciled against every decision in this file**; (2) `docs/overview.md` gains a module-table entry and a tree line for `internal/shedengine`; (3) `CONSTRAINTS.md` gains the invariant above; (4) `manifest/roadmap.md` moves the **Shed: shared outer phase-FSM, no predefined slots** item from Planned to Done.
- **The `shed.md` reconciliation is a whole-document pass, not a fixed checklist.** This discussion changed enough of the pinned design that an enumerated list would read as exhaustive and let the rest silently rot. The known edits, non-exhaustively:
  - Status banner: "Design sketch, Planned" → shipped skeleton, adapters still Planned.
  - Step-6 routing target: "back to step 2" → back to step 1 (re-read each iteration).
  - Steps 5 and 6 are one atomic `UpdateJSON` persist, not two writes.
  - The "Shed is the file's only writer" sentence → the explicit field-ownership split.
  - `pause_requested` is consumed (cleared) in the same persist that writes `state: "paused"`.
  - The `Shed` struct gains `StatusLockPath` beside `StatusPath`/`LockPath`, with the never-the-same-file rule.
  - `Run` `MkdirAll`s both lock parents; the "nothing on disk touched" wording (`shed.md:146`) is corrected, since acquiring a lock creates the lock file.
  - The status-file JSON example (`shed.md:150-162`) gains the `product` field, which is currently absent from it.
  - `current_producer`'s value on completion (the last producer's name) and the already-`done` short-circuit.
  - `MaxBounces`'s concrete default (10) and its per-`Run`-call, in-memory scope.
  - `history[].at` as RFC3339 UTC, and `Result.History` as the full persisted history.
  - Step 6's error branch is cancellation-aware (`ctx.Err() != nil` routes to the pause exit, not `failed`), and the `ShedProducer` contract section gains **two** obligations: surface cancellation as an `error` (never as `Stuck`), and return only `Done` or `Stuck`.
  - **`shed.md:27`'s "done / approved / stuck / blocked" prose is wrong** — four values against a two-value Go contract (`shed.md:80-84`). It predates the current contract and must be corrected, plus step 6 gains the unrecognised-`Outcome` disposition.
  - The bounce budget's exact boundary: `MaxBounces` bounces permitted, the `(MaxBounces + 1)`-th `Stuck` blocks.
  - The persist aborts rather than creating a status file that vanished mid-run.
  - `activity.last`'s exact composed format, and `Result`'s meaninglessness on a non-nil error.
  - **Two Go types `shed.md` references but never declares:** `Status` — the persisted status file, currently pinned only as a JSON example — and `HistoryEntry`, which `Result.History` is typed on with no declaration anywhere. Both need a real Go block with JSON tags, alongside the `ShedProducer`/`ProducerDef`/`Shed`/`Result` blocks that already have one. `Status`'s fields: `current_producer`, `state`, `error`, `pause_requested`, `activity`, `history`, `product`. `HistoryEntry`'s: `producer`, `outcome`, `output`, `at`. The `activity` sub-object needs a named type too (`now`, `last`, `wait`).
  - The plan must re-read `shed.md` against this file's Decisions section as a whole and fix anything else that has drifted, rather than treating the above as complete.
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
Three entry points matter, and which one is used where is a decision above, not an implementation detail:

- `state.ReadJSONStrict[T](path, lockPath)` — the **top-of-iteration read gate**. Returns `(zero, false, nil)` on a missing file, so "missing" is distinguishable from an error, which `no-seeding-hard-error-on-missing` needs; `state.ErrRead`/`state.ErrDecode` sentinel the two failure classes. It does **not** create parent directories, unlike `ReadJSON` — correct here, since `Shed` never creates the file.
- `state.UpdateJSON[T](path, lockPath, mutate)` — the **step-5 persist**. Holds one exclusive lock across read, mutate, and write (`state.go:99-133`), which is what makes the merge safe against a concurrent external writer.
- `state.WriteJSON` — **not used by the loop.** `UpdateJSON` must never be composed from `ReadJSON`+`WriteJSON`: both acquire `lockPath` internally, so nesting hangs rather than failing (`state.go:108-109`). That same hazard is why `LockPath` and `StatusLockPath` must be different files.

Both locking calls inside `internal/state` are the **blocking** `lock.AcquireWriteLock`/`AcquireReadLock`, never the `TryAcquire` form — only Shed's own run lock uses `TryAcquireWriteLock`.

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

- **Shed Producer-Seam Invariant** — `internal/shedengine` production imports capped at stdlib, `internal/state`, and `internal/lock`; never `loomengine`, never an adapter package, never `lyxcwd`, never `logger`. Policed on direct imports only, though with this allowlist the `lyxcwd` exclusion happens to hold transitively too. Enforced by `internal/shedengine/seam_enforcement_test.go`.

Also binding:

- **Semantic line breaks** in every `.md` file touched — one sentence per line, plus a break at internal independent-clause boundaries. No fixed-column hard wrap, no trailing double-space.
- **Worktree isolation** — all work stays in `/home/knatte/Code/loomyard/wts/shed`; never push to `main` from here.

## Testing

All tests are **Tier 1, untagged, in-package** `shedengine`, using a `funcProducer` fake (a closure adapted onto `ShedProducer`) and a real status file under `t.TempDir()` read and written through `internal/state`.
No mocked persistence — the `internal/state` round-trip is the thing most likely to break, so it must be exercised for real.

**TDD candidates** — the pure, table-friendly units, written test-first:

- Producer-list validation: empty list, duplicate `Name`, `OnStuck` naming an absent producer, **empty `Name`**, **nil `Producer`**, negative `MaxBounces`, `LockPath == StatusLockPath`, and each of the three paths empty. One table, one case per rule, each asserting a distinct error. The nil-`Producer` case must assert an error rather than a recovered panic.
- The `activity` fill rule: given a `current_producer`, a `history`, and a `state`, assert the three composed fields — including `wait` being `""` for every non-`blocked`/`failed` state.
- Status-file JSON round-trip: marshal → unmarshal → deep-equal, and a `product` payload surviving a full read-modify-write. **Semantic equality, not byte identity** — persistence goes through `json.MarshalIndent` (`internal/state/state.go:48`), which re-indents an embedded `json.RawMessage`, so a payload written with different whitespace survives semantically but not byte-for-byte. Compare via a `json.Marshal`-normalised form or an `any` deep-equal, never a raw byte compare.

**Loop scenarios that must be covered** (each drives `Run` against a purpose-built fake list):

- Happy path — every producer returns `Done`; `state: "done"`, `Result.Outcome == RunDone`, `history` in order, one entry per call.
- `Stuck` with an `OnStuck` target — `current_producer` moves to the named target and that producer runs again; the bounce is recorded in `history`.
- `Stuck` with no target — `state: "blocked"`, `Result.Outcome == RunBlocked`, `Reason` set, `HaltedProducer` naming the right producer.
- Bounce-budget exhaustion — an always-`Stuck` cycle performs exactly `MaxBounces` bounce-backs and then blocks on the `(MaxBounces + 1)`-th `Stuck`, with the exhaustion reason rather than looping. Assert the bounce **count** against that exact boundary (a `MaxBounces: 3` list bounces three times, blocks on the fourth `Stuck`), since the off-by-one is the whole point of the assertion.
- `MaxBounces: 0` resolves to the internal default rather than "no bounces allowed".
- Producer returns `error` — `state: "failed"`, `error` populated, `Run` returns a non-nil error, and **no** further producer is called.
- Producer returns an **unrecognised `Outcome`** with a nil error (e.g. `Outcome("approved")`, `Outcome("")`) — `state: "failed"`, a non-nil error naming the offending value and producer, no further producer called, and the `history` entry recording the literal value received.
- `pause_requested: true` mid-list — exits before calling the next producer, `state: "paused"`, nil error, `current_producer` unchanged so the next `Run` resumes there.
- Cancelled `ctx` between producers — same observable outcome as pause, and specifically: no producer is called after cancellation.
- Cancelled `ctx` **during** a producer call — a fake producer that returns `ctx.Err()` after the test cancels mid-`Call`. Assert `state: "paused"`, `RunPaused`, a **nil** error, no new `history` entry, and `current_producer` unchanged so the next `Run` re-calls it. This is the common Ctrl-C shape; without this scenario the top-of-iteration check passes while every real cancellation still lands in `failed`.
- Producer error with a **healthy** `ctx` — still `state: "failed"` and a non-nil error, confirming the cancellation-aware branch keys on `ctx.Err()` and does not swallow genuine failures.
- Stray external key is destroyed — a fake producer writes an **unrecognised top-level key** into the status file from inside its own `Call` (after step 1's read has passed). Assert the key is **absent** after the next persist. This pins the corrected behaviour from `strictness-is-scoped-to-the-read-gate`: the key is silently dropped by the full-struct marshal, *not* caught by a later strict read. It is the negative counterpart to the external mid-producer write scenario above, which only covers the two known shared fields surviving — and it is worth a test precisely because it is a corrected misconception rather than an obvious fact, so nothing but a test will stop a future reader re-deriving the wrong answer.
- Status file deleted mid-run — a fake producer deletes the status file from inside its own `Call`; assert `Run` returns an error and **no** status file is re-created, proving the persist's `found == false` guard holds the no-seeding rule at the write as well as the read.
- Crash recovery — run a list partway, then construct a **fresh** `Shed` against the same status file and `Run` again; assert it resumes at the persisted `current_producer` and does not re-run completed producers.
- Unconditional re-call — a producer whose `OutputPointer.Path` names an existing file is still called again on resume, never skipped. This is an explicit design guarantee and deserves its own test.
- Missing status file — hard error, and **the status file is not created** (the lock files and their parent directories *are*, per `run-lock`; an assertion of "nothing created on disk" would be false).
- `current_producer` naming an absent producer — hard error, and the status file is byte-identical afterwards.
- Malformed status file, and one carrying an unknown top-level key — both hard errors via the strict decode.
- Run lock already held — `Run` returns the busy sentinel immediately and the status file is untouched. Acquire the lock directly in the test via `internal/lock`; no second process needed.
- Persist failure — assert exactly two things, and no more: `Run` returns a non-nil error, and **no status file is re-created** at the target path. The tempting third assertion, "no compensating `state: \"failed\"` write was attempted", is **unobservable under this injection** and must not be written: the injection destroys the write target, so a hypothetical compensating write would fail identically to the persist itself, and the test cannot distinguish "Shed never tried" from "Shed tried and also failed". That `persist-failure-halt` forbids the compensating write is a design decision enforced by review, not by this test. **Fault injection must fail the write only, and reaching that is harder than it looks:**
  - A parent directory that merely does not exist will not fail — `state.UpdateJSON` calls `os.MkdirAll(filepath.Dir(path))` before locking (`internal/state/state.go:111-114`).
  - An unwritable parent directory is a no-op under root and on Windows.
  - A `StatusPath` routed through an existing regular file (`x/status.json` where `x` is a file) fails too **early**: step 1's `state.ReadJSONStrict` calls `os.ReadFile` first, and `ENOTDIR` is not `os.IsNotExist`, so it returns `ErrRead` (`state.go:147-153`) and `Run` hard-errors before any producer runs — testing the read gate, not the persist.
  - **Workable method:** inject from *inside* a fake producer's `Call`, after step 1's read has already succeeded — the producer replaces `StatusPath`'s parent directory with a regular file, so the persist's own `MkdirAll` fails with `ENOTDIR` on every platform regardless of privilege.
  - **Drop the byte-identical last-good assertion** as unstageable: that injection necessarily destroys the file it would be asserted against. The "file keeps its last-good contents" property is covered instead by the crash-recovery scenario, which exercises the same resume path without needing a forced write failure.
- `product` passthrough — a status file carrying an arbitrary product payload survives a full `Run` with semantic equality (see the round-trip note above).
- External mid-producer write — while a fake producer is executing, the test writes `pause_requested: true` (and mutates `product`) directly into the status file from inside that producer's own `Call`. Assert both survive the persist and that the pause is honoured at the very next iteration. This is the regression test for the whole-file-clobber hazard, and it is the reason the loop re-reads and merges rather than rewriting from an in-memory copy.
- Resume after pause — the run that honours a pause writes `pause_requested: false` alongside `state: "paused"`, and a **second** `Run` against that same file proceeds instead of pausing again. Without this scenario the permanently-unresumable bug is invisible: every single-`Run` pause test passes while the task can never restart.
- Completion terminal values — on `RunDone`, assert `current_producer` holds the last producer's name (not `""`), `activity.now` matches it, and `Result.HaltedProducer` matches it.
- Re-run idempotence — a second `Run` against a `state: "done"` file returns `RunDone` immediately, calls **zero** producers (assert the call counter), and leaves the file unchanged.
- Re-run `Result` equality — the `Result` from the short-circuited second `Run` equals the first, completing run's `Result` (same `Outcome`, `HaltedProducer`, and `History`), proving the short-circuit fills both from the file rather than returning a bare `RunDone`.
- Re-run idempotence with a changed list — a `state: "done"` file whose `current_producer` is no longer in `Producers` still returns `RunDone` cleanly, **not** the step-2 hard error, confirming the short-circuit sits before the lookup.
- Resume after halt — a `Run` against a `state: "blocked"` or `state: "failed"` file does **not** short-circuit: it re-calls `current_producer` and overwrites the state as it advances.
- Lock-parent creation — a `Shed` whose `LockPath`/`StatusLockPath` parents do not yet exist runs successfully, rather than failing with a raw lock-acquire error.
- Two-lock validation — a `Shed` whose `LockPath` and `StatusLockPath` name the same file is rejected by `Run`'s pre-loop validation with an error, and specifically **does not hang**. Assert the error; a test that deadlocks here would hang the suite rather than fail it.

**The seam-enforcement test** follows `internal/treadleengine/seam_enforcement_test.go`: walk the package's non-test `.go` files, collect direct imports, and fail on anything outside the allowlist.

## Q&A log

- **Q:** Package name — `internal/shedengine` or `internal/shed`? **A:** `shedengine`, matching the repo-wide `<module>engine` kernel naming; no `shedcli` in this task, since a bare `lyx shed` would be a command with no producer list to walk.
- **Q:** `Outcome` as `int`+`iota` (as `shed.md` then pinned) or string-typed? **A:** String-typed. Checking `internal/treadleengine.Outcome` directly showed it is `type Outcome string` for exactly this reason, and `shed.md`'s own JSON example already wrote `"outcome": "done"` — the design had implicitly committed to a hand-maintained int→string mapping without noticing. `shed.md` was corrected upstream for both `Outcome` and `RunOutcome`.
- **Q:** Does `Shed` derive its status-file path or is it told? **A:** Told, exactly as treadle is told `runDir` and `Profile.GateDir`.
- **Q:** Does this task reconcile Shed's status shape with `loom`'s pinned `status-schema.md`? **A:** No — out of scope; that is `loom`'s rewiring task. Recorded as a divergence in the package doc instead.
- **Q:** What happens when the status file is missing? **A:** Hard error — reversing `shed.md`'s original "seed at `producers[0]`", which would have made `loom`'s shipped `CheckSeedMissing` permanently unreachable dead code.
- **Q:** What if `current_producer` names a producer no longer in the list? **A:** Hard error, nothing written. Both alternatives guess at what already happened, the failure mode this design rejects everywhere else.
- **Q:** How does a terminal condition survive a process exit? **A:** New `state` and `error` fields on the status file, mirroring `Result`'s `Outcome`+`Reason` split — otherwise "paused", "blocked", and "crashed" are indistinguishable on disk.
- **Q:** Who fills `activity`? **A:** `Shed`, mechanically — every field is composed from data `Shed` already owns (`current_producer`, `history`, `state`), and no external actor has that data. No hook; there is zero product variance today.
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
- **Q:** Can one `LockPath` serve both the run lock and `internal/state`'s own lock? **A:** No — `state.WriteJSON`/`UpdateJSON` use the *blocking* acquire, so a shared path deadlocks on the first persist rather than erroring. Two separately-supplied paths, with a same-path check in `Run`'s validation so the mistake fails loud instead of hanging.
- **Q:** Is `Shed` really the status file's only writer? **A:** No, and the design must not claim it. `pause_requested` is set in-status by an outside actor by deliberate design (`status-schema.md:69`), so the loop re-reads the file each iteration and persists via `state.UpdateJSON`, carrying `pause_requested` and `product` forward instead of clobbering them. A pause requested during a long producer call was otherwise both unobserved and destroyed.
- **Q:** Doesn't `UpdateJSON` break the strict-decode decision? **A:** Its internal re-read is lenient (`state.go:122`), so strictness is scoped explicitly to the read gate. Malformed JSON still fails loud; an unknown top-level key added by an external writer mid-iteration is **silently destroyed** by the full-struct marshal — accepted because `product` is the sanctioned external channel, and a key outside it is a mistake nothing promises to preserve. (An earlier version of this rationale claimed the key would be caught by the next strict read. That was wrong — the merge strips it, so the next read sees a clean file.)
- **Q:** Does the `product` field make a Shed-written file satisfy `loom`'s schema? **A:** No, and the rationale must not imply it — `phase`/`stage`/`narration` are top-level in `status-schema.md` and would still fail `checkCoherence`. `product` is a generic mechanism; reconciling the two shapes is `loom`'s own later task.
- **Q:** How is a persist failure actually provoked in a test? **A:** Not by a missing parent directory — `state` calls `os.MkdirAll` first — and not by an unwritable directory, which is a no-op under root and on Windows. Point `StatusPath` through an existing regular file so `MkdirAll` fails with `ENOTDIR` on every platform.
- **Q:** What writes `history[].at`, and what does `Result.History` contain? **A:** RFC3339 UTC from a direct `time.Now().UTC()` (no injectable clock — tests assert the format and ordering, not literals), and the full persisted history, not just this run's entries.
- **Q:** Who clears `pause_requested`? **A:** `Shed`, in the same persist that writes `state: "paused"` — the flag is a request it consumes. Nobody clearing it meant the next `Run` would re-pause on the flag it was resuming from, forever; `treadleengine`'s `clearPauseFlag` exists for exactly this reason.
- **Q:** What does `current_producer` hold once the run completes? **A:** The last producer's name, never `""` — an empty value would make the next run's step-2 lookup fail with a misleading "producer not in list" error. A `Run` against an already-`done` file returns `RunDone` immediately without calling anything, which is what makes re-running idempotent; `blocked`/`failed` deliberately do not short-circuit, so a human can resume after fixing the cause.
- **Q:** Who creates the lock files' parent directories? **A:** `Shed`, via `MkdirAll` before acquiring — `internal/lock` opens with `O_CREATE` but never creates parents, which is why both `loomengine` and `treadleengine` already do this. Also corrected the false claim that lock acquisition touches nothing on disk; it creates the lock file.
- **Q:** How do you actually provoke a persist failure in a test? **A:** From inside a fake producer's `Call`, after the read gate has passed — replacing the status file's parent directory with a regular file. Every simpler method either fails to fail (`MkdirAll` runs first; unwritable dirs are no-ops under root and on Windows) or fails too early at step 1's read. The byte-identical last-good assertion was dropped as unstageable, since that injection destroys the file it would check.
- **Q:** Does a `product` payload survive byte-for-byte? **A:** No — `json.MarshalIndent` re-indents an embedded `json.RawMessage`. The assertion is semantic equality, not byte identity.
- **Q:** Are steps 5 and 6 one persist or two? **A:** One. Routing is computed first, then the `history` append, the new `current_producer`, and the new `state` land in a single `UpdateJSON` mutate. Two writes would let a crash between them re-call the finished producer and append a duplicate `history` entry — defeating the exact crash-safety property step 5 exists to provide.
- **Q:** Does the bounce budget survive a crash or a resume? **A:** No — it is per-`Run`-call and in-memory, deliberately unpersisted. Every event that resets it is a new human-initiated invocation, which is the outcome the budget exists to force; persisting it would make a legitimate resume inherit an exhausted budget.
- **Q:** Can the persist itself create a status file, given Shed swore never to seed one? **A:** It could — `UpdateJSON` treats a missing file as non-error and writes the mutate's result — so the mutate returns an error on `found == false`, aborting the write. Otherwise a file deleted mid-run would be silently re-seeded from a zero value.
- **Q:** What stops a producer reporting cancellation as `Stuck` instead of an error? **A:** Nothing mechanical — `Shed` cannot tell the difference — so it is a stated `ShedProducer` contract obligation. Otherwise an operator's Ctrl-C would silently burn bounce budget or escalate to `blocked`.
- **Q:** What happens if a producer returns an `Outcome` that is neither `Done` nor `Stuck`? **A:** Hard failure — `state: "failed"`, the literal value recorded in `history` for diagnosis. `Outcome` is a `string` type and therefore open, so the routing had an undefined fourth case; coercing to either known value would guess a verdict. Also surfaced that `shed.md:27` still describes a four-value contract against its own two-value Go block.
- **Q:** Why must a `ProducerDef.Name` be non-empty? **A:** `""` is already `OnStuck`'s escalate-to-human sentinel, and it is the zero value a partial seed leaves in `current_producer` — which step 2 would look up and *run*, converting a corrupt status file into silent execution. A nil `Producer` is rejected for a plainer reason: it panics at step 4.
- **Q:** Does `MaxBounces: 3` allow three bounces or two? **A:** Three; the fourth `Stuck` blocks. Pinned because it is the classic off-by-one and the test asserts an exact count.
- **Q:** What happens when Ctrl-C lands *during* a producer call? **A:** The producer returns an error, which would have been written as `state: "failed"` — the exact misrepresentation the pause decision rejects. Step 6's error branch now checks `ctx.Err() != nil` and routes to the pause exit instead, appending no history entry and leaving `current_producer` put. The predicate is `ctx.Err()`, not `errors.Is(err, context.Canceled)`, because a producer's own internal deadline firing is a genuine failure, not an operator stop.
- **Q:** Should `internal/logger` be on the seam allowlist? **A:** No — nothing in `shedengine` logs, and excluding it makes "never `lyxcwd`" hold *transitively* (logger imports lyxcwd; `lock` and `state` import nothing that does), a stronger property than treadle's own invariant can claim.
