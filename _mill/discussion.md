# Discussion: loom: phase-machine scaffolding

```yaml
task: 'loom: phase-machine scaffolding'
slug: loom-phase-machine-scaffolding
status: discussing
parent: standalone-producers
```

## Problem

`internal/shedengine` shipped as a generic outer phase-FSM: it walks one flat, ordered list of producers, honouring resume, crash-recovery and pause uniformly at producer granularity.
`internal/shedadapters` shipped the three engine adapters (`SingleLLMProducer`, `PerchProducer`, `WebsterProducer`) that let a Shed-built product drive shuttle, perch and Webster as ordinary producers.
What does not exist is `loom` itself.
Per `manifest/designs/loom.md`, `loom` = `Shed` + `loom`'s own ordered producer list, nothing else — and that list has never been written.
The engine has no product on it.

**Why now:** every downstream loom item is blocked on this list existing.
`loom: session bootstrap` (`lyx loom run`) needs a phase machine to launch, and `loom: write and wire in the real LLM producers` needs rows to swap its real producers into.
Building the full 12-row list now — real where the code already exists, stubbed where it does not — is what makes sequencing, resume, crash-recovery and pause real from day one rather than retrofitted once the LLM producers land.

A second, sharper reason surfaced during exploration: nothing in production seeds `_lyx/loom/status.json` today.
`Shed` refuses to create one (`"Shed never seeds one"`), and `loomengine.Preflight`'s check 4 fails when it is absent.
The phase machine cannot run at all until this task supplies a seeder.

## Scope

**In:**

- A new package `internal/loomshed` that owns `loom`'s 12-row producer list and returns a constructed `*shedengine.Shed`.
- A `Seed(...)` function in `internal/loomshed` that writes the initial `_lyx/loom/status.json`.
- `Discussion-Validate` built for real (new code, small).
- `Plan-Validate` built for real (thin wrap over `internal/planparser.Validate`).
- `Batchifier` built for real as a fail-fast gate over `batcher.Active` (see the `batchifier-is-a-gate` decision).
- `Preflight` and `Webster` wired in as-is via thin producer wrappers, no changes to their own engines.
- Seven stub producers: `Discussion-Write`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Review`, `Webster-Review`, `Finalize`.
- Migration of `_lyx/loom/status.json` onto `shedengine.Status`, including the rewrite of `loomengine.Status`, `loomengine`'s coherence check, and `contracts/specs/loom-status-spec.md`.
- The `loom.md` row-9 Output-column correction (see the `batchifier-is-a-gate` decision).

**Out:**

- Any cobra module or CLI verb. `lyx loom run` belongs to `loom: session bootstrap`; this task is engine-only, exactly as `loomengine.Preflight` is today.
  No `internal/loomcli`, no registration in `cmd/lyx`.
  The Sandbox Suite Coverage invariant therefore does not engage — it keys on registered cobra modules.
- Any change to `internal/shedengine`. Its skeleton is shipped and its Shed Producer-Seam Invariant forbids it knowing anything about loom.
- Any change to `internal/websterengine`, `internal/batcher`, `internal/perchengine`, `internal/burlerengine`, or `internal/shuttleengine`.
- Any change to `internal/shedadapters`. All three adapters exist and are used as-is; this task instantiates them, it does not extend them.
- A real `Plan-Sweep`. Moved out of this task by `c12b330a` — its only consumer, `Plan-Write`, is a stub here, so a real `Plan-Sweep` would have nothing to feed.
  It is built in `loom: write and wire in the real LLM producers`.
- A real `Finalize`. It is its own Planned roadmap item, blocked on `fabric: merge-conflict primitive`, and swaps in by reference once that lands.
- Any LLM prompt, rubric, or perch profile content. That is the whole of `loom: write and wire in the real LLM producers`.
- Narration/activity authoring. `shedengine.composeActivity` already fills `activity{now,last,wait}` mechanically from data Shed holds; no producer supplies it.

## Decisions

### shed-schema-wins

- Decision: `_lyx/loom/status.json` becomes a `shedengine.Status`.
  `loom`'s own `slug`, `parent` and `start_sha` move into the existing `product json.RawMessage` passthrough field; `narration` is dropped in favour of Shed's mechanically-composed `activity{now,last,wait}`.
- Rationale: `loomengine.Preflight`'s check 4 and `shedengine` currently read the *same file* with two mutually incompatible strict schemas — `slug`/`parent`/`phase`/`stage`/`narration` versus `current_producer`/`state`/`activity`/`history`.
  `shedengine/doc.go` names this divergence explicitly and parks it as "loom's own later rewiring work"; this task is that work.
  One file, one writer, one schema follows directly from "loom = Shed + its own producer list, nothing else".
- Rejected: two separate files (`_lyx/loom/shed.json` alongside the existing one) — avoids the collision instead of resolving it, and leaves two sources of truth about one run.
  Making loom's schema win is not available at all: it would require editing `shedengine`, which the Shed Producer-Seam Invariant puts off-limits.

### loomshed-is-its-own-package

- Decision: the producer list lives in a new package `internal/loomshed`, which takes told absolute paths and returns a constructed `*shedengine.Shed`.
- Rationale: `internal/loomengine` already imports `internal/lyxcwd` (its `DiscussionDir`/`LoomStatusFile` accessors take `*lyxcwd.Location`), and the Told-Geometry Invariant's producers-standalone wave is actively pulling packages the other way.
  A fresh package with no direct `lyxcwd` import keeps the list on the right side of that invariant from birth, and mirrors the `hubgeom`/`standalonegeom` adapter direction — adapters depend on engines, never the reverse.
- Rejected: putting the list in `internal/loomengine` (inherits the `lyxcwd` dependency); putting it in `cmd/lyx` (bypasses the module seam the CLI/Cobra Invariant wants).

### explicit-deps-struct

- Decision: `loomshed`'s constructor takes an explicit dependencies struct in which each real producer arrives as an already-constructed `shedengine.ShedProducer`.
  Production wiring builds the real ones; tests substitute fakes.
- Rationale: it is the only option that keeps the thing under test the *real* producer list.
  It matches `shedadapters`' own told/injected style, and mirrors the fake-`burler` precedent `perch` used to validate its own loop.
  It is also what makes the verify requirement ("the full 12-row sequence runs against the stubs, including resume, crash-recovery, and pause") reachable at Tier 1: `Preflight` and `Webster` are the only rows that would otherwise spawn git or LLM sessions, and both are injected.
- Rejected: build-tag or test-only overrides (worse seam, same result); a separate production list and test list (the tested list stops being the real one, defeating "sequencing is real from the start").

### onstuck-routing

- Decision: every gate and validator bounces back to the producer whose artifact it guards; every other row escalates to a human (`OnStuck: ""`).
- Rationale: `loom.md` already specifies the shape — "`Plan-Review`'s stuck routes back to `Plan-Write`".
  The rest follows the same rule mechanically.
  Building the real table now is cheap and is what "sequencing is real from the start" means; deferring it would leave a table that has to be revisited the moment any gate becomes real.
- Rejected: routing `Webster-Review` back to `Plan-Write` instead of `Webster` — a rejected diff can indeed mean a bad plan, but it is the costlier bounce and re-runs an approved phase; revisit if evidence shows the cheaper bounce loops.
  Escalating everything to a human in this task — true that no stub can return `Stuck`, but the table is durable config, not throwaway scaffolding.

### loomshed-owns-seed

- Decision: `internal/loomshed` exports a `Seed(...)` that writes the initial status file via `internal/state`: `current_producer: "Preflight"`, the initial state, empty `history`, and loom's payload (`slug`, `parent`, `start_sha`) in `product`.
- Rationale: nothing writes this file today, `Shed` refuses to create one, and `Preflight` fails without it — so the task cannot satisfy its own verify requirement without a real seeder.
  `loom: session bootstrap` calls it later; this task both needs it and is the right owner.
- Rejected: deferring to session bootstrap and having tests write their own seed inline — the verify requirement would then rest on test code with no production equivalent.
  Having `lyx fabric add` drop the seed — fabric is generic infrastructure and knows nothing about loom; that is the wrong direction of dependency.

### rewrite-loom-status-in-place

- Decision: `loomengine.Status` is rewritten as a thin product struct (`slug`, `parent`, `start_sha`) decoded from Shed's `product` field.
  `checkCoherence` is rewritten to validate the Shed shell plus that product payload.
  `contracts/specs/loom-status-spec.md` is rewritten in the same commit.
- Rationale: the Told-Geometry Invariant pins `loomengine.Preflight` as the documented tier-3 entry point — "tiers 1+2 plus the orchestrator's own status seed".
  Deleting `loomengine.Status`/`checkCoherence` outright would break that without anyone asking to amend `CONSTRAINTS.md`.
- Rejected: deleting both and moving check 4 into `loomshed` (see above).
  Thinning check 4 to "the file exists and decodes as `shedengine.Status`" — coherence means the content matches reality, not merely that it parses; reducing it to a decode check discards the point of check 4.
  Note that the old `phase` vocabulary (`preflight|discussion|plan|webster|raddle|finalize|done`) disappears entirely — `current_producer` is the identity, and the status strand prints `activity.now`.
  A shorter UI label, if ever needed, is derived in the presentation layer, never stored redundantly in the status file.

### preflight-signature-unchanged

- Decision: `loomengine.Preflight(cwd string) (Report, error)` keeps its signature.
  A small wrapper in `loomshed` adapts it to `ShedProducer`, and `loomshed` receives that wrapper already constructed per `explicit-deps-struct`.
- Rationale: the roadmap says "wire in `Preflight`, `Batchifier`, and `Webster` as-is — all three already shipped, no new code in any of them".
  Told-Geometry is satisfied at the `loomshed` boundary, and `Preflight` stays the documented tier-3 entry point.
  The only change inside `loomengine` is the one `rewrite-loom-status-in-place` forces.
- Rejected: refactoring `Preflight` onto told paths — churn in a package the roadmap explicitly wanted untouched.

### producer-names-verbatim

- Decision: `ProducerDef.Name` uses the design table's strings verbatim: `Preflight`, `Discussion-Write`, `Discussion-Validate`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Validate`, `Plan-Review`, `Batchifier`, `Webster`, `Webster-Review`, `Finalize`.
- Rationale: the name is the durable on-disk identity in `current_producer`; renaming it later breaks resume for any in-flight task.
  `manifest/designs/loom.md`'s table is the contract, and the status file is read by humans.
- Rejected: lowercase-kebab (`discussion-write`) — more consistent with lyx's CLI vocabulary, but the status file is not a CLI surface and the design table is the authority.

### batchifier-is-a-gate

- Decision: the `Batchifier` row calls `batcher.Active(baseDir)` and returns `Stuck` when `batcher.yaml` is broken or names an unknown batchifier.
  It reports an empty `OutputPointer`.
  The resolved `batcher.Batcher` is injected into the Webster producer's `websterengine.RunDeps` at construction time, per `explicit-deps-struct` — never handed across through Shed.
  `manifest/designs/loom.md`'s row-9 Output column is corrected in the same commit, from "batch grouping handed to `Webster`" to a gate description.
- Rationale: `websterengine.RunDeps` already carries a `Batcher batcher.Batcher` field and Webster resolves its own batches internally (`beginbatch.go`, `recordbatch.go`, `awaitbatch.go` all take `[]batcher.Batch`).
  There is no channel for a row-9-to-row-10 handover in the first place: `Call` returns only `(Outcome, OutputPointer, error)` and `Batchifier` writes no artifact.
  The design table described a mechanism that cannot exist, so the doc is wrong, not merely unimplemented.
  As a gate the row earns a real job the table missed: catching a broken batch config *before* Webster spawns LLM sessions rather than partway through them — the same gate shape `Discussion-Validate` and `Plan-Validate` already have.
- Rejected: dropping the row — contradicts the roadmap text, which lists `Batchifier` as a row to wire in as-is; a scope change nobody requested, and it would force a synchronous design-table and `loom.md` edit for something that was not the question.
  Materialising the grouping to disk — duplicates state Webster already holds in its own `state.json` and requires a `websterengine` change, against the same "as-is, no new code" rule that settled `preflight-signature-unchanged`.

## Technical context

**The producer list to build.** `Real` = built or wired in this task; `Stub` = returns `Done` without doing work.

| # | `ProducerDef.Name` | Status here | `OnStuck` | Backing |
|---|---|---|---|---|
| 1 | `Preflight` | Real (wire as-is) | `""` | `loomengine.Preflight(cwd)` behind a wrapper |
| 2 | `Discussion-Write` | Stub | `""` | later: `shedadapters.NewSingleLLMProducer` + `loomengine.DiscussionSpec` |
| 3 | `Discussion-Validate` | Real (build) | `Discussion-Write` | new code in `loomshed` |
| 4 | `Discussion-Review` | Stub | `Discussion-Write` | later: `shedadapters.NewPerchProducer` |
| 5 | `Plan-Sweep` | Stub | `""` | later: `scoutengine` inventory |
| 6 | `Plan-Write` | Stub | `""` | later: `shedadapters.NewSingleLLMProducer` + `loomengine.PlanSpec` |
| 7 | `Plan-Validate` | Real (build) | `Plan-Write` | `planparser.ParsePlan` + `planparser.Validate` |
| 8 | `Plan-Review` | Stub | `Plan-Write` | later: `shedadapters.NewPerchProducer` |
| 9 | `Batchifier` | Real (gate) | `""` | `batcher.Active(baseDir)` |
| 10 | `Webster` | Real (wire as-is) | `""` | `shedadapters.NewWebsterProducer` |
| 11 | `Webster-Review` | Stub | `Webster` | later: `shedadapters.NewPerchProducer` |
| 12 | `Finalize` | Stub | `""` | later: the shared, by-reference `Finalize` producer |

**`shedengine` API the list plugs into.**
`ProducerDef{Name string, Producer ShedProducer, OnStuck string}`; `ShedProducer` is a single method `Call(ctx) (Outcome, OutputPointer, error)`.
`Shed{Producers, StatusPath, LockPath, StatusLockPath, MaxBounces}` is a plain exported-field struct — there is no constructor, and `Run` validates every field before touching anything.
`Shed.validate()` rejects an empty `Producers`, an empty `Name`, a nil `Producer`, a duplicate `Name`, an `OnStuck` naming no producer in the list, and `LockPath == StatusLockPath`.
Forward references in `OnStuck` are legal — the whole name set is collected before `OnStuck` is checked.

**Two obligations `Shed` cannot enforce**, binding every producer written here:
`Call` returns exactly `Done` or `Stuck` and nothing else, and `Call` surfaces context cancellation as a non-nil `error`, never as `Stuck`.
A `Stuck` return under a cancelled context is indistinguishable to `Shed` from a genuine verdict, so it would silently consume bounce budget for what was an operator stop.

**`shedengine.Status`** is `{current_producer, state, error, pause_requested, activity{now,last,wait}, history[]{producer,outcome,output,at}, product json.RawMessage}`.
`Shed.persist` writes it through `state.UpdateJSON` under `StatusLockPath` and refuses to create the file if it has vanished.
`composeActivity` fills `Activity` mechanically: `Now` is `current_producer` verbatim, `Last` is `"<producer> → <outcome>"` from the newest history entry, `Wait` is the error text only under `blocked`/`failed`.

**Paths.** `loomengine.LoomStatusFile(l)` and `loomengine.LoomStatusLock(l)` already declare the status file and its lock; both take a `*lyxcwd.Location`, so `loomshed` receives their results as told strings rather than importing `lyxcwd` itself.
The lock must be a different file from `Shed`'s run lock — `internal/state` acquires its own lock with the blocking form, so one shared path hangs on the first persist.
Per the Durable-vs-Ephemeral State Invariant the status file is durable under `_lyx` and both locks are never-tracked under `.lyx` at the mirrored subpath.

**`Discussion-Validate`'s exact checks**, exhaustively — the design states it "has no judgment, and nothing beyond these two checks is 'its' to look for":

1. Both `decision-record.md` and `support-log.md` exist under `_lyx/discussion/`.
2. `decision-record.md` contains all seven required H2 sections: `## Goal`, `## Scope`, `## Decisions`, `## Constraints`, `## Auto-mode assumptions`, `## Open risks`, `## Acceptance criteria`.

`## Notes for the plan writer` is optional by contract and its absence is never a violation.
Section *order* and "no other sections" are pinned in the stencil (`contracts/stencils/loom/loom-template-discussion.md`) but are deliberately **not** validator checks — do not add them.
`loomengine.DiscussionDecisionRecord`/`DiscussionSupportLog` already declare the two paths.

**`Plan-Validate`** is a thin wrap: `planparser.ParsePlan(planDir)` then `planparser.Validate(plan, worktreeRoot) []ValidationError`; a non-empty slice maps to `Stuck`.
`planparser.PlanDir(anchorPath)` declares the directory.
The Planparser Sole-Parser Invariant means no plan parsing may be written here — the producer calls `planparser` and maps the result, nothing more.

**The `Plan-never-reads-support-log` boundary** is asserted once at build/test time over `Plan-Write`'s producer *definition* (its declared input set never names `support-log.md`), never per run.
The design says this assertion "lands with `Shed`" — with `Plan-Write` stubbed here, decide during planning whether the assertion is meaningful yet or waits for the real producer.

**Preflight's ordering.** `Shed` reads and validates the status file *before* calling row 1, and `Preflight`'s check 4 then re-reads the same file to assert a coherent *fresh* seed (empty history, unset `start_sha`).
That is consistent, not circular: `Preflight` only ever runs when `current_producer` names it, which is only on a fresh run or a human-resumed halt at row 1.
`Preflight`'s own doc comment warns that invoking it on an already-advanced task is a caller error — `OnStuck: ""` on every row means nothing ever bounces back to it.

## Constraints

From `CONSTRAINTS.md`, in force for this task:

- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `internal/state`, `internal/lock`.
  Nothing in this task may add an import to that package; producers adapt onto `ShedProducer` in their own packages.
  Machine-enforced by `internal/shedengine/seam_enforcement_test.go`.
- **Told-Geometry Invariant** — `internal/loomshed` takes told absolute paths and must have no direct production import of `internal/lyxcwd`.
  An orchestrator requires tier 3 (`loomengine.Preflight`) and threads the extracted plain values down its producer list; a producer requires none of the three tiers.
  Consider adding a `leaf_enforcement_test.go`-style import guard to `loomshed` so this is machine-enforced rather than a review obligation.
- **Lyxdirs Single-Declarer Invariant** — no production file outside `internal/lyxdirs` may name `_lyx` or `.lyx` in path-construction context; use `lyxdirs.LyxDirName`/`DotLyxDirName`, or take the path as told.
  Enforced by `internal/lyxcwd/enforcement_test.go`.
- **Durable-vs-Ephemeral State Invariant** — status file durable under `_lyx`, both locks never-tracked under `.lyx` at the mirrored subpath.
- **Planparser Sole-Parser Invariant** — `internal/planparser` is the sole parser of `_lyx/plan/` and the sole declarer of its path.
- **Batcher Registry+Config Invariant** — batching is selected by `internal/batcher`'s name-keyed registry plus `batcher.yaml`'s `active:` key; no batch grouping is decided anywhere else.
- **Producer Pointer-Rule Invariant** — binds instruction files and format-contract docs, not Go source.
  It engages here only through the doc edits (`loom.md` row 9, `loom-status-spec.md`), which must point rather than restate.
- **Test Tier Purity Invariant** — an untagged test file must not call `gitexec.Run`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub`, by raw substring match including in comments and string literals.
- **Hermetic Git Test Environment Invariant** — any test package that spawns git needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
- **Documentation Lifecycle** — `manifest/designs/loom.md` and `contracts/specs/loom-status-spec.md` are updated in the same commit as the code that changes them.
  `manifest/roadmap.md` moves only on completing the item.

Not engaged: the **CLI/Cobra Invariant** and **Sandbox Suite Coverage**, since this task registers no cobra module.

## Testing

**Tier 1, untagged — `internal/loomshed`.**
The whole point of `explicit-deps-struct` is that the 12-row list is exercisable offline.

- The list's shape: 12 rows, names verbatim per `producer-names-verbatim`, in table order, with the `OnStuck` map from `onstuck-routing`.
  Assert against a table so a reordering or rename is a test failure, since both break resume.
- `Shed.validate()` accepts the constructed list — no empty/duplicate name, no nil producer, every `OnStuck` naming a real row, distinct lock paths.
  This is the cheapest guard against a typo'd `OnStuck` and should be asserted explicitly rather than relied on implicitly.
- **The full 12-row sequence with every row faked to `Done`** — the verify requirement's core. Assert the terminal `RunDone`, and the `history` order.
- **Resume:** run to a mid-list producer, construct a fresh `Shed` over the same status file, assert it re-calls `current_producer` and completes.
- **Crash-recovery:** the unconditional re-call — a producer whose output already exists is still called again; assert `Shed` does not skip it.
- **Pause:** set `pause_requested`, assert the run stops at the next producer boundary, that the flag is consumed in the same persist, and that a subsequent run resumes rather than re-pausing.
- **Bounce routing:** a faked gate returning `Stuck` routes to its `OnStuck` target; a row with `OnStuck: ""` escalates to `RunBlocked`.
  Assert the bounce budget is consumed and that exhausting it blocks.
- **Cancellation:** a producer returning `Stuck` under a cancelled context is the obligation `Shed` cannot enforce — assert each *real* producer written here returns an error instead.

**TDD candidates** — pure, table-shaped, no I/O beyond a temp dir:

- `Discussion-Validate`'s two checks. Table-drive: both files present and all seven sections; each file missing in turn; each of the seven sections missing in turn; `## Notes for the plan writer` present and absent (both pass); sections present but out of order (must pass — deliberately not a check); an extra unexpected H2 (must pass).
- `Seed`'s output: assert the exact `shedengine.Status` written, including `current_producer: "Preflight"`, empty history, and the `product` payload round-tripping through `json.RawMessage`.
  Assert `Seed` is safe against an existing file in whichever way planning decides (refuse vs. overwrite) — pick one and test it.
- The rewritten `loomengine` coherence check against the new schema: table-driven over each mandatory field empty, plus the fresh-start invariants (non-empty history, set `start_sha`).
- `Plan-Validate`'s mapping: a `planparser.Validate` result with zero errors maps to `Done`, non-zero to `Stuck`, and a parse failure maps to a non-nil error rather than `Stuck`.
- `Batchifier`'s gate: a valid `batcher.yaml` maps to `Done` with an empty `OutputPointer`; an unknown batchifier name and a malformed file each map to `Stuck`.

**Integration-tagged.**
One test covering the real `loomengine.Preflight` wrapper against a `hubforge` fixture hub — the only row that needs real git.
It needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
Keep it to the wrapper's outcome mapping (`Report{OK:true}` → `Done`, `Report{OK:false}` → `Stuck`, error → error); `Preflight`'s own four checks are already covered by `internal/loomengine`'s existing tests and must not be re-tested here.

**Not tested here:** Webster's own loop, perch's round loop, `batcher`'s implementations, `planparser`'s checks.
All are shipped and independently covered; this task tests only its own wiring and mapping.

## Q&A log

- **Q:** How do the two incompatible status schemas over `_lyx/loom/status.json` reconcile? **A:** Shed's schema wins; loom's fields move into `product`. Two files would avoid the collision rather than resolve it and leave two sources of truth about one run; making loom's schema win is blocked by the Shed Producer-Seam Invariant.
- **Q:** Where does the 12-row producer list live? **A:** A new `internal/loomshed` — the right side of Told-Geometry, no `lyxcwd` dependency, same adapter direction as `hubgeom`/`standalonegeom`.
- **Q:** Is `Plan-Sweep` built in this task? **A:** No — resolved upstream by `c12b330a`, which moved its real build to `loom: write and wire in the real LLM producers` because its only consumer, `Plan-Write`, is stubbed here.
- **Q:** How are the real engines injected so the sequence is testable? **A:** An explicit deps struct of already-constructed `ShedProducer`s. No build tags, and the tested list stays the real list.
- **Q:** What is the `OnStuck` map? **A:** Gates and validators bounce to the producer whose artifact they guard; everything else escalates to a human. `loom.md` already specifies `Plan-Review` → `Plan-Write`; the rest follows the same rule. Build the real table now rather than deferring it, since it is durable config.
- **Q:** Who seeds `_lyx/loom/status.json`, given nothing does today? **A:** `loomshed.Seed`. The task's own resume verify needs a production seeder; putting it in `fabric` would give generic infrastructure knowledge of loom.
- **Q:** What happens to `loomengine.Status`, its coherence check, and `loom-status-spec.md`? **A:** Rewritten in place. Deleting them would break Told-Geometry's designation of `loomengine.Preflight` as the tier-3 entry point without amending `CONSTRAINTS.md`; thinning check 4 to a decode check discards what coherence means.
- **Q:** Does `Preflight` keep its `(cwd string)` signature? **A:** Yes — a wrapper adapts it. The roadmap's "as-is, no new code" is explicit, and Told-Geometry is satisfied at the `loomshed` boundary.
- **Q:** Verbatim design-table producer names, or lowercase-kebab? **A:** Verbatim. The name is the durable on-disk identity in `current_producer`, a later rename breaks resume, and the design table is the contract.
- **Q:** Does the old 7-value `phase` vocabulary survive? **A:** No. `current_producer` is the identity and the strand prints `activity.now`; a shorter UI label is derived in the presentation layer, never stored redundantly.
- **Q:** What does the `Batchifier` row concretely do, given `websterengine.RunDeps` already owns a `Batcher` and `Call` has no channel to hand a grouping to row 10? **A:** A fail-fast gate over `batcher.Active`, with the resolved `Batcher` injected into Webster's `RunDeps` at construction. The design table's row-9 Output wording described a mechanism that cannot exist and is corrected in the same commit. Dropping the row contradicts the roadmap; materialising the grouping to disk duplicates Webster's own state and would need a `websterengine` change.
