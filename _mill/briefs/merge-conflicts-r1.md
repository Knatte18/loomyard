# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/loom-phase-machine-scaffolding rm <file>`.

### From discussion.md

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
- The doc set the schema change falsifies — `docs/overview.md`, `manifest/designs/shed.md`, `loom.md`'s State-&-contracts bullet and row-10 Input, and `internal/shedengine/doc.go`'s divergence paragraph (see `shedengine-doc-carve-out`).
  Enumerated by grep, not by memory; the full list and the method are under Constraints → Documentation Lifecycle.
- Adding `internal/loomshed` to the Told-Geometry Invariant's machine-enforced list in `CONSTRAINTS.md`, alongside the import guard that earns it the listing.
- A new `loomengine.LoomRunLock(l)` accessor for `Shed`'s run lock (`.lyx/loom/run.lock`), which has no declarer today, plus its entries in the constructor-anchoring and no-transients test sets.

**Out:**

- Any cobra module or CLI verb. `lyx loom run` belongs to `loom: session bootstrap`; this task is engine-only, exactly as `loomengine.Preflight` is today.
  No `internal/loomcli`, no registration in `cmd/lyx`.
  The Sandbox Suite Coverage invariant therefore does not engage — it keys on registered cobra modules.
- Any change to `internal/shedengine`'s **code**. Its skeleton is shipped and its Shed Producer-Seam Invariant forbids it knowing anything about loom.
  The one carve-out is its `doc.go` divergence paragraph — see the `shedengine-doc-carve-out` decision.
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

- Decision: `loomshed`'s constructor takes an explicit dependencies struct.
  **What is injected is exactly the two rows that would otherwise spawn a real substrate; everything else `loomshed` builds itself from told values.**

```go
type Deps struct {
    // Shed's own told paths (see the Durable-vs-Ephemeral note below).
    StatusPath, LockPath, StatusLockPath string
    MaxBounces                           int

    // Told paths the mechanical rows need. AnchorPath and WorktreeRoot are
    // deliberately separate fields: planparser.PlanDir takes the anchor path,
    // planparser.Validate takes the worktree root, and they are not the same value.
    AnchorPath         string // planparser.PlanDir, and batcher.Active — see below
    WorktreeRoot       string // planparser.Validate
    DecisionRecordPath string // Discussion-Validate
    SupportLogPath     string // Discussion-Validate

    // Row 1, injected pre-constructed: it is the only row that spawns git.
    Preflight shedengine.ShedProducer

    // Row 10, injected as parts rather than as a ShedProducer, because the lazy
    // Batcher wrapper is loomshed-owned (see batchifier-is-a-gate).
    // WebsterDeps.Batcher is left nil; the wrapper fills it per Call.
    WebsterRun  shedadapters.WebsterRunner
    WebsterDeps websterengine.RunDeps
}
```

- **Rows `loomshed` constructs itself:** `Discussion-Validate`, `Plan-Validate` and `Batchifier` (new code, pure functions over told paths), the `Webster` wrapper, and all seven stubs.
  **Rows injected:** `Preflight` as a `ShedProducer`, `Webster` as `WebsterRun`+`WebsterDeps`.
  No other row is injectable, because no other row touches anything a Tier-1 test cannot run for real.
- **There is no separate `BaseDir` field.** `batcher.Active(baseDir)` reaches `configengine.FindBaseDir`, which stats `<baseDir>/_lyx` and returns `baseDir` unchanged, while `planparser.PlanDir(anchorPath)` joins `<anchorPath>/_lyx/plan` — the two are the same directory by construction.
  Two fields would only invite silent divergence, so `AnchorPath` feeds both.
  `WorktreeRoot` stays separate because `planparser.Validate` genuinely takes a different value.
- The two discussion file paths are told rather than derived because `loomengine.DiscussionDecisionRecord`/`DiscussionSupportLog` take a `*lyxcwd.Location`, and `loomshed` must not import `internal/lyxcwd` directly.
  The caller resolves them and passes the results as strings.
  `Preflight`'s own `cwd` argument never appears in `Deps` at all — the caller closes over it when constructing the injected `Preflight` producer.
- Rationale: it is the only option that keeps the thing under test the *real* producer list.
  It matches `shedadapters`' own told/injected style, and mirrors the fake-`burler` precedent `perch` used to validate its own loop.
  It is also what makes the verify requirement ("the full 12-row sequence runs against the stubs, including resume, crash-recovery, and pause") reachable at Tier 1: `Preflight` and `Webster` are the only rows that would otherwise spawn git or LLM sessions, and both are injected.
- Rejected: build-tag or test-only overrides (worse seam, same result); a separate production list and test list (the tested list stops being the real one, defeating "sequencing is real from the start").

### onstuck-routing

- Decision: every gate and validator bounces back to the producer whose artifact it guards — **and a gate whose guarded artifact is produced by no row in the list escalates instead (`OnStuck: ""`)**; every other row escalates too.
  The second clause is what makes the rule agree with the table without cross-checking: `Preflight` gates git and filesystem state, and `Batchifier` gates `batcher.yaml`, neither of which any producer in the list writes, so there is nothing to bounce to and a human is the only thing that can fix either.
- Rationale: `loom.md` already specifies the shape — "`Plan-Review`'s stuck routes back to `Plan-Write`".
  The rest follows the same rule mechanically.
  Building the real table now is cheap and is what "sequencing is real from the start" means; deferring it would leave a table that has to be revisited the moment any gate becomes real.
- Rejected: routing `Webster-Review` back to `Plan-Write` instead of `Webster` — a rejected diff can indeed mean a bad plan, but it is the costlier bounce and re-runs an approved phase; revisit if evidence shows the cheaper bounce loops.
  Escalating everything to a human in this task — true that no stub can return `Stuck`, but the table is durable config, not throwaway scaffolding.

### loomshed-owns-seed

- Decision: `internal/loomshed` exports `Seed(statusPath, statusLockPath, slug, parent string) error`, which writes the initial status file via `internal/state`: `current_producer: "Preflight"`, `state: shedengine.StateRunning`, empty `history`, `pause_requested: false`, and loom's payload (`slug`, `parent`, `start_sha: null`) in `product`.
  `Seed` **refuses when the file already exists**, returning an error rather than overwriting.
- It takes bare told paths rather than a `Deps`, and takes `slug`/`parent` from its caller: seeding happens *before* any `Shed` exists, so a `Deps` would couple the seam to a struct whose producer fields are irrelevant to it.
  `loom: session bootstrap` is the production caller and supplies all four from the worktree it is launching in.
- The `state` value is pinned rather than left to planning because `State` is a five-member enum whose empty string the read gate hard-rejects (`status.go`'s `valid()`), so an unpinned seed is a hard error at Shed's first read.
  `StateRunning` is the only member that means "a run may proceed from here"; `paused`, `done`, `blocked` and `failed` all describe a run that has already happened.
- Refuse-over-overwrite is a production-safety choice, not a style one: overwriting silently destroys an in-flight run's `history`, and the whole resume contract rests on that history.
  A deliberate re-seed is then an explicit operator act (delete the file first), never an accident.
- **Write mechanics, since the obvious implementation is wrong twice over.**
  `Seed` makes the refuse-if-exists decision **under the held lock**, via `state.UpdateJSON`'s `found` argument — returning an error when `found` is true — rather than stat-then-`state.WriteJSON`, which is a TOCTOU window between the check and the write.
  `Seed` also **creates the lock file's parent directory** before acquiring: `state.WriteJSON` MkdirAlls the *status file's* parent but not the *lock file's*, the exact gap `internal/loomengine/preflight.go` already documents and works around at its own lock acquisition.
  Both are cheap and both are silent failures if missed.
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
- **The fresh-start check must tolerate a `Preflight`-only history.**
  The rewritten check rejects a history entry naming any producer *after* `Preflight`, and tolerates entries naming `Preflight` itself.
  Without this the row deadlocks permanently: `run.go:187-200` appends a history entry *before* persisting `StateBlocked`, including on the `OnStuck: ""` path, so a `Stuck` at row 1 leaves `len(History) == 1`; the human-resumed re-run then re-calls `Preflight`, whose old `len(s.History) != 0` test (`coherence.go:92`) fails `CheckHalfFinished` forever.
  Narrowing the test this way preserves exactly what check 4 protects against — a run that got *past* `Preflight` and left work half-finished — while letting a blocked `Preflight` be resumed.
  `start_sha` and `pause_requested` keep their existing fresh-start treatment unchanged.
- **The non-`history` Shed fields need the same treatment, or the deadlock returns through another field.**
  A `Preflight` that returned `Stuck` under `OnStuck: ""` persists `state: "blocked"` *and* `error: "stuck with no OnStuck target"` in the same write, and the resumed run re-enters check 4 against exactly that file.
  So the rewritten check pins:
  `state` — every valid member tolerated **except** `done`, which is rejected as a finished run.
  (`done` is in practice unreachable here, since `run.go`'s already-done short-circuit returns before any producer is called; rejecting it is belt-and-braces, not a live path.)
  `error` — any value tolerated, including non-empty: it is the previous halt's reason, which is precisely what a human resumes *after* reading, never a coherence violation in itself.
  `activity` — never validated at all. `Shed` composes it mechanically on every persist, so it is derived output; validating it would assert `Shed`'s own arithmetic against itself.
  `current_producer` — must name `Preflight`, since that is the only way check 4 is reached.
- **The read itself changes type, and `loomengine` gains an import.**
  Check 4 reads through `state.ReadJSONStrict[Status]`, which rejects unknown fields, so once `loomengine.Status` becomes the thin `{slug, parent, start_sha}` product struct the existing read would hard-fail on every Shed field in the file.
  The rewritten check therefore reads `state.ReadJSONStrict[shedengine.Status]` first and decodes the `product` payload into `loomengine.Status` second — meaning `internal/loomengine` gains a production import of `internal/shedengine`.
  That direction is fine: the Shed Producer-Seam Invariant constrains what `shedengine` may import, never who may import it.
- **Field-by-field disposition** of everything `loom-status-spec.md` pins today, so nothing is silently dropped:
  `phase` — gone (`current_producer` is the identity; the strand prints `activity.now`).
  `narration` — gone, replaced by Shed's mechanically-composed `activity{now,last,wait}`.
  `stage` (`produce|gate`) — gone; it is statically derivable from which row `current_producer` names, so storing it duplicates the producer list.
  `next_action` — gone from the durable file; `state` plus `activity.wait` (which carries the error text under `blocked`/`failed`) is the operator-facing equivalent, and the old field also participated in the fresh-start check, which the bullet above now expresses through history alone.
  `history[].bounced_to` — **lost**, accepted deliberately: `shedengine.HistoryEntry` is `{producer, outcome, output, at}` with no bounce-target field, and adding one would edit `shedengine`, which Scope excludes.
  Bounce provenance stays reconstructible from the sequence — a `stuck` entry is followed by an entry for that row's `OnStuck` target — so nothing is unrecoverable, only less direct.
  `slug`, `parent`, `start_sha` — moved into `product` per `shed-schema-wins`.
- **The two per-entry history rules `loom-status-spec.md` pins are kept, translated to the new field names**, not dropped: every `history[].outcome` must be one of `done | stuck` (Shed's `Outcome` vocabulary, replacing the old `approved | stuck`), and `history[].at` must be RFC3339 UTC (replacing `ts`).
  The `bounced_to`-only-when-`stuck` rule goes with the field itself.
  These are kept, unlike `activity`, because the reasoning differs: `activity` is recomposed by `Shed` on every persist, so validating it asserts `Shed` against itself, whereas `history` accumulates across runs and the file has sanctioned external writers — and `Shed`'s own read gate validates only `state`, never per-entry outcome membership or timestamp shape.
- Rejected: deleting both and moving check 4 into `loomshed` (see above).
  Thinning check 4 to "the file exists and decodes as `shedengine.Status`" — coherence means the content matches reality, not merely that it parses; reducing it to a decode check discards the point of check 4.
  Moving half-finished detection off `history` onto `current_producer`+`state` — more robust against future history-shape changes, but a larger rewrite of what check 4 means than the deadlock actually requires.
  A shorter UI label replacing `phase`, if ever needed, is derived in the presentation layer, never stored redundantly in the status file.

### preflight-signature-unchanged

- Decision: `loomengine.Preflight(cwd string) (Report, error)` keeps its signature.
  **`internal/loomshed` owns and exports the wrapper constructor** — `NewPreflightProducer(cwd string) shedengine.ShedProducer` — and therefore imports `internal/loomengine`.
  `Deps.Preflight` stays typed as a bare `shedengine.ShedProducer` so a Tier-1 test injects a fake; production wiring passes `loomshed.NewPreflightProducer(cwd)`.
  The integration-tagged test for the real wrapper lands in `internal/loomshed`, which consequently needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
- Importing `loomengine` does not compromise `loomshed`'s Told-Geometry position: the invariant's membership predicate is about a **direct** production import of `internal/lyxcwd`, and transitive is explicitly fine.
  The import guard polices exactly that, so `loomengine` — which does import `lyxcwd` — is a legal dependency.
  Leaving the wrapper to the not-yet-built caller was the alternative, and it would leave this task with a `Preflight` row nothing can construct until `loom: session bootstrap` lands.
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

- Decision: the `Batchifier` row calls `batcher.Active(baseDir)` and returns `Stuck` when the call errors.
  It reports an empty `OutputPointer`.
  **Nothing is injected or handed across.** Neither row holds a pre-resolved `batcher.Batcher`: the Webster producer resolves `batcher.Active(baseDir)` itself, lazily, inside its own `Call`, from the same told `baseDir`.
  Row 9 is a strictly-earlier invocation of that identical resolution whose only purpose is to fail fast.
  `manifest/designs/loom.md`'s row-9 Output column is corrected in the same commit, from "batch grouping handed to `Webster`" to a gate description.
- **Why lazily, not injected at construction.** `shedadapters.NewWebsterProducer(name, run, deps)` takes `websterengine.RunDeps` **by value**, so injecting a resolved `Batcher` would require `batcher.Active` to have already succeeded before `Shed.Run` ever starts — which makes row 9's stated value ("catch a broken config before Webster spawns") unreachable, since the process would have failed earlier.
  It would also survive a crash badly: after a restart with `current_producer: "Webster"`, row 9 never re-runs in the new process, so an injected value would have to be re-resolved anyway.
  Lazy resolution in a thin `loomshed` wrapper — one whose `Call` resolves the `Batcher`, constructs `NewWebsterProducer(...)`, and delegates — keeps `shedadapters` and `websterengine` untouched and makes both rows resume-identical.
- **The mid-run-edit consequence, stated explicitly:** if `batcher.yaml` changes between row 9 and row 10, row 10 uses the newer config.
  That is correct behaviour under lazy resolution, not staleness — there is no cached value to go stale.
  Row 9's guarantee is therefore precisely "the config was resolvable at row 9", never "the config Webster will use is the one row 9 saw".
- **Error mapping, at both call sites.** Every error from `batcher.Active` maps to `Stuck` — in row 9's gate **and** in row 10's wrapper, identically.
  The wrapper's disposition has to be stated separately because the two outcomes differ materially in `run.go`: a `Stuck` at row 10 (`OnStuck: ""`) persists `StateBlocked` and returns `RunBlocked`, which a human resumes after fixing the config, whereas returning the error instead persists `StateFailed` and aborts `Run` with a hard error.
  A broken `batcher.yaml` is operator-fixable, so `blocked` is the correct resting state at both rows; making them differ would mean the same fault ends the run one way before Webster and another way at Webster.
  The conflation below is accepted in writing.
  `Active` returns a bare `error` for unknown-name, malformed YAML, and I/O failure alike, with no sentinel to discriminate on.
  The conflation is cheap here because `Active` already falls back to the embedded `ConfigTemplate()` when `_lyx/` or `batcher.yaml` is absent — so a remaining error is a genuinely broken config far more often than an infra fault, and `blocked` is the right resting state for that.
  A future sentinel in `internal/batcher` could split the two; adding one is out of scope.
- Rationale: `websterengine.RunDeps` already carries a `Batcher batcher.Batcher` field and Webster resolves its own batches internally (`beginbatch.go`, `recordbatch.go`, `awaitbatch.go` all take `[]batcher.Batch`).
  There is no channel for a row-9-to-row-10 handover in the first place: `Call` returns only `(Outcome, OutputPointer, error)` and `Batchifier` writes no artifact.
  The design table described a mechanism that cannot exist, so the doc is wrong, not merely unimplemented.
  As a gate the row earns a real job the table missed: catching a broken batch config *before* Webster spawns LLM sessions rather than partway through them — the same gate shape `Discussion-Validate` and `Plan-Validate` already have.
- Rejected: dropping the row — contradicts the roadmap text, which lists `Batchifier` as a row to wire in as-is; a scope change nobody requested, and it would force a synchronous design-table and `loom.md` edit for something that was not the question.
  Materialising the grouping to disk — duplicates state Webster already holds in its own `state.json` and requires a `websterengine` change, against the same "as-is, no new code" rule that settled `preflight-signature-unchanged`.

### shedengine-doc-carve-out

- Decision: `internal/shedengine/doc.go`'s "# Divergence from loom's status schema" paragraph is rewritten in the same commit, as the single carve-out from Scope's "no changes to `internal/shedengine`".
- Rationale: that paragraph asserts two things this task makes false — that reconciling the schemas "is loom's own later rewiring work", and that "a Shed-written file would still fail loom's coherence check".
  Leaving them would leave the package's own doc actively lying about its only consumer.
  A doc-comment edit adds no import, so the Shed Producer-Seam Invariant — which is an *import* allowlist, machine-enforced by `seam_enforcement_test.go` — is untouched by it.
  The exclusion in Scope is about the package's code and its dependency surface, and it stays absolute there.
- Rejected: leaving the paragraph stale — cheapest, but it makes the seam's own documentation the least trustworthy description of the seam.

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
`internal/shedadapters` solves this with `entryErr`/`cancelErr` in its `ctx.go`, but both are **unexported** and Scope forbids changing that package — so every real producer written in `loomshed` re-implements the same entry/exit check locally.
That duplication is deliberate, recorded here so a plan writer proposes a small local helper in `loomshed` rather than exporting `shedadapters`' own.

**`shedengine.Status`** is `{current_producer, state, error, pause_requested, activity{now,last,wait}, history[]{producer,outcome,output,at}, product json.RawMessage}`.
`Shed.persist` writes it through `state.UpdateJSON` under `StatusLockPath` and refuses to create the file if it has vanished.
`composeActivity` fills `Activity` mechanically: `Now` is `current_producer` verbatim, `Last` is `"<producer> → <outcome>"` from the newest history entry, `Wait` is the error text only under `blocked`/`failed`.

**Paths.** `loomengine.LoomStatusFile(l)` (`_lyx/loom/status.json`) and `loomengine.LoomStatusLock(l)` (`.lyx/loom/status.json.lock`) already declare the status file and its status lock; both take a `*lyxcwd.Location`, so `loomshed` receives their results as told strings rather than importing `lyxcwd` itself.

**`Shed`'s *run* lock has no declarer today, and this task adds one.**
`Deps.LockPath` is a third path, distinct from the status lock — `internal/state` acquires its own lock with the blocking form, so one shared file hangs on the first persist, which is why `Shed.validate()` rejects `LockPath == StatusLockPath` outright.
No accessor for it exists anywhere in the repo.
This task adds `loomengine.LoomRunLock(l)` returning `<AnchorPath>/.lyx/loom/run.lock`, beside the two existing accessors and scoped under the same `loom/` subdirectory for the same product-collision reason.
It is not deferred to `loom: session bootstrap`: the Durable-vs-Ephemeral State Invariant requires a module scratch accessor beside its durable one, and `cmd/lyx/constructoranchoring_test.go` and `notransients_test.go` pin every such accessor — a told path with no declarer would satisfy Tier-1 temp dirs while leaving production wiring with nowhere to get the value from.
Both locks are never-tracked under `.lyx` at the mirrored subpath; the status file is durable under `_lyx`.

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
The design says this assertion "lands with `Shed`".
**It does not land in this task.** The assertion is over `Plan-Write`'s *declared input set*, and a stub declares no input set at all — there is nothing to assert against, so writing the test now would either assert a vacuous truth or invent a declaration the real producer has not yet made.
It lands with the real `Plan-Write` in `loom: write and wire in the real LLM producers`.
`manifest/designs/loom.md`'s line "This assertion lands with `Shed`" is false as of this task and is corrected in the same commit — it is added to the doc set **manually**, since the status-file-keyed grep enumeration method cannot reach it.

**Preflight's ordering, and the retry deadlock it creates.** `Shed` reads and validates the status file *before* calling row 1, and `Preflight`'s check 4 then re-reads the same file to assert a coherent *fresh* seed.
The double read is not itself circular — `Preflight` only ever runs when `current_producer` names it, which is a fresh run or a human-resumed halt at row 1, and `OnStuck: ""` everywhere means nothing bounces back to it.
The deadlock is in the *fresh-start* half of the check, not the ordering: `run.go:187-200` appends a history entry before persisting `StateBlocked` even on the `OnStuck: ""` path, so a `Stuck` at row 1 leaves `len(History) == 1`, and the old `len(s.History) != 0` test at `coherence.go:92` then fails `CheckHalfFinished` on every subsequent resume.
`rewrite-loom-status-in-place` resolves it by narrowing the test to reject only history entries naming a producer after `Preflight`.
This is the one place where `Preflight`'s doc-comment warning ("invoking it on an already-advanced task is a caller error") and `Shed`'s unconditional re-call semantics genuinely collide, so it needs a test of its own — see Testing.

## Constraints

From `CONSTRAINTS.md`, in force for this task:

- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `internal/state`, `internal/lock`.
  Nothing in this task may add an import to that package; producers adapt onto `ShedProducer` in their own packages.
  Machine-enforced by `internal/shedengine/seam_enforcement_test.go`.
- **Told-Geometry Invariant** — `internal/loomshed` takes told absolute paths and must have no direct production import of `internal/lyxcwd`.
  An orchestrator requires tier 3 (`loomengine.Preflight`) and threads the extracted plain values down its producer list; a producer requires none of the three tiers.
  **`internal/loomshed` gets a `leaf_enforcement_test.go`-style import guard policing its production import set to exclude `internal/lyxcwd`, and is added to the invariant's machine-enforced list in `CONSTRAINTS.md` in the same commit.**
  This is not optional polish: it converts a review obligation into a machine check for the one package this task creates, at the cost of a single table-driven test, and `internal/shedengine`/`internal/treadleengine` are the in-repo precedent for exactly this guard on exactly this property.
- **Lyxdirs Single-Declarer Invariant** — no production file outside `internal/lyxdirs` may name `_lyx` or `.lyx` in path-construction context; use `lyxdirs.LyxDirName`/`DotLyxDirName`, or take the path as told.
  Enforced by `internal/lyxcwd/enforcement_test.go`.
- **Durable-vs-Ephemeral State Invariant** — status file durable under `_lyx`, both locks never-tracked under `.lyx` at the mirrored subpath.
- **Planparser Sole-Parser Invariant** — `internal/planparser` is the sole parser of `_lyx/plan/` and the sole declarer of its path.
- **Batcher Registry+Config Invariant** — batching is selected by `internal/batcher`'s name-keyed registry plus `batcher.yaml`'s `active:` key; no batch grouping is decided anywhere else.
- **Producer Pointer-Rule Invariant** — binds instruction files and format-contract docs, not Go source.
  It engages here only through the doc edits (`loom.md` row 9, `loom-status-spec.md`), which must point rather than restate.
- **Test Tier Purity Invariant** — an untagged test file must not call `gitexec.Run`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub`, by raw substring match including in comments and string literals.
- **Hermetic Git Test Environment Invariant** — any test package that spawns git needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
- **Documentation Lifecycle** — the same-commit doc set, enumerated rather than guessed.

  **Enumeration method**, so the list is reproducible and not incidental: `grep -rn 'loom/status.json\|loom-status-spec\|current phase\|current review stage' --include='*.md' --include='*.go' .`, then keep every hit that asserts something about the file's *shape* and drop every hit that is only a pointer to it.

  Affected, all updated in the same commit:

  - `contracts/specs/loom-status-spec.md` — the whole schema.
  - `manifest/designs/loom.md` — the State-&-contracts bullet ("current phase, current review stage"), row 9's Output column, row 10's **Input** column ("batch grouping"), falsified by the same reasoning that corrects row 9, and — added manually, since the grep above cannot reach it — the `Plan-never-reads-support-log` line "This assertion lands with `Shed`", which this task's decision falsifies.
    Also in the same bullet group: the "*It also carries a human-readable current-activity narration*" bullet, reworded off the retired `narration` term onto `activity` — its own `now:`/`last:`/`wait:` example survives verbatim as Shed's field, so only the word changes, not the described behaviour.
  - `manifest/designs/shed.md` — the `product`-carries-no-compatibility-claim paragraph ("`loom-status-spec.md` mandates `phase`, `stage`, and `narration` as top-level fields… reconciling the two is loom's own later rewiring task") and the surrounding seed/external-writer text that depends on it.
  - `docs/overview.md` — the `_lyx/` bullet's "current phase, review round, verdict history" wording, plus the internal-package tree, which gains `internal/loomshed`.
  - `internal/shedengine/doc.go` — the divergence paragraph, per `shedengine-doc-carve-out`.
  - `CONSTRAINTS.md` — `internal/loomshed` joins the Told-Geometry machine-enforced list.

  Checked and **not** affected, recorded so the next reader does not re-derive it: `contracts/specs/webster-spec.md:50` and `manifest/designs/self-report.md:15` are pointers to the status file that assert nothing about its shape, and `docs/overview.md:98` only lists `loom-status-spec.md` among the kept contract docs, which stays true.

  `manifest/roadmap.md` moves only on completing the item.

Not engaged: the **CLI/Cobra Invariant** and **Sandbox Suite Coverage**, since this task registers no cobra module.

## Testing

**Tier 1, untagged — `internal/loomshed`.**
The whole point of `explicit-deps-struct` is that the 12-row list is exercisable offline.

- The list's shape: 12 rows, names verbatim per `producer-names-verbatim`, in table order, with the `OnStuck` map from `onstuck-routing`.
  Assert against a table so a reordering or rename is a test failure, since both break resume.
- `Shed.validate()` accepts the constructed list — no empty/duplicate name, no nil producer, every `OnStuck` naming a real row, distinct lock paths.
  This is the cheapest guard against a typo'd `OnStuck` and should be asserted explicitly rather than relied on implicitly.
- **The full 12-row sequence run to `RunDone`** — the verify requirement's core. Assert the terminal outcome and the `history` order.
  **This needs a shared Tier-1 fixture builder, not more injection points.** Only `Preflight` and `Webster` are injectable, so rows 3, 7 and 9 are real code reading real on-disk state and must be made to genuinely pass:
  `Discussion-Validate` needs `_lyx/discussion/` with both files and all seven H2 sections;
  `Plan-Validate` needs a `_lyx/plan/` fixture satisfying every one of `planparser.Validate`'s checks, including the ones that stat paths against `WorktreeRoot` (`checkPathMissing`, `checkMoveSourceMissing`), so the fixture creates those files too;
  `Batchifier` needs either an `_lyx/` with a valid `batcher.yaml` or none at all, since `batcher.Active` falls back to the embedded `ConfigTemplate()`.
  One builder produces the whole temp anchor and is reused by every sequence test below.
  Adding injection points for rows 3/7/9 instead would contradict `explicit-deps-struct`'s two-rows-only rule and would stop the sequence test from exercising the three producers this task actually builds — which is most of its value.
- **Resume:** run to a mid-list producer, construct a fresh `Shed` over the same status file, assert it re-calls `current_producer` and completes.
- **Crash-recovery:** the unconditional re-call — a producer whose output already exists is still called again; assert `Shed` does not skip it.
- **Pause:** set `pause_requested`, assert the run stops at the next producer boundary, that the flag is consumed in the same persist, and that a subsequent run resumes rather than re-pausing.
- **Bounce routing:** a faked gate returning `Stuck` routes to its `OnStuck` target; a row with `OnStuck: ""` escalates to `RunBlocked`.
  Assert the bounce budget is consumed and that exhausting it blocks.
- **Cancellation:** a producer returning `Stuck` under a cancelled context is the obligation `Shed` cannot enforce — assert each *real* producer written here returns an error instead.

**TDD candidates** — pure, table-shaped, no I/O beyond a temp dir:

- `Discussion-Validate`'s two checks. Table-drive: both files present and all seven sections; each file missing in turn; each of the seven sections missing in turn; `## Notes for the plan writer` present and absent (both pass); sections present but out of order (must pass — deliberately not a check); an extra unexpected H2 (must pass).
- `Seed`'s output: assert the exact `shedengine.Status` written — `current_producer: "Preflight"`, `state: "running"`, empty history, `pause_requested: false`, and the `product` payload round-tripping through `json.RawMessage`.
  Assert `Seed` returns an error and leaves the file byte-identical when one already exists, and that it succeeds when the lock file's parent directory does not yet exist.
- The rewritten `loomengine` coherence check against the new schema: table-driven over each mandatory field empty, plus the fresh-start invariants (set `start_sha`, `pause_requested`), the tolerated/rejected `state` members, and the two retained per-entry history rules (`outcome ∈ {done, stuck}`, `at` RFC3339 UTC).
- **The Preflight retry deadlock, as a named regression test.** A history containing only `Preflight` entries passes the fresh-start check; a history containing any entry naming a later producer fails it.
  This is the one finding round 1 caught that would have shipped as a permanent runtime deadlock, so it gets an explicit test rather than incidental coverage.
- The `Batchifier` gate and the `Webster` wrapper both resolve `batcher.Active` independently at their own `Call` time — assert neither holds a value resolved at construction, e.g. by mutating `batcher.yaml` between the two calls and observing that the second sees the new config.
  Assert the wrapper maps a `batcher.Active` error to `Stuck`, not to a returned error — the two produce `blocked` and `failed` respectively, and only the first is resumable.
- `Plan-Validate`'s mapping: a `planparser.Validate` result with zero errors maps to `Done`, non-zero to `Stuck`, and a parse failure maps to a non-nil error rather than `Stuck`.
- `Batchifier`'s gate: a valid `batcher.yaml` maps to `Done` with an empty `OutputPointer`; an unknown batchifier name and a malformed file each map to `Stuck`.
  An absent `_lyx/` or absent `batcher.yaml` maps to `Done`, since `batcher.Active` resolves the embedded `ConfigTemplate()` rather than erroring — assert this, so the fallback is not mistaken for a gate failure.
- A guard test over `internal/loomshed`'s production import set, excluding `internal/lyxcwd`, modelled on `internal/shedengine/seam_enforcement_test.go`.

**Integration-tagged, in `internal/loomshed`.**
One test covering `loomshed.NewPreflightProducer`'s real wrapper against a `hubforge` fixture hub — the only row that needs real git.
The package therefore needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
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
- **Q:** What does the `Batchifier` row concretely do, given `websterengine.RunDeps` already owns a `Batcher` and `Call` has no channel to hand a grouping to row 10? **A:** A fail-fast gate over `batcher.Active`. The design table's row-9 Output wording described a mechanism that cannot exist and is corrected in the same commit. Dropping the row contradicts the roadmap; materialising the grouping to disk duplicates Webster's own state and would need a `websterengine` change.
- **Q:** (Review r1) How does the rewritten coherence check reconcile the fresh-start invariant with the history entry `Shed` appends before blocking at row 1? **A:** Tolerate history entries naming only `Preflight`, reject any naming a later producer. Preserves what check 4 protects against — a run that got past `Preflight` — while making a blocked `Preflight` resumable. Rejected: moving detection onto `current_producer`+`state` (a larger rewrite of check 4's meaning than the deadlock requires), and dropping the fresh-start half entirely (the same thinning rejected in Q7).
- **Q:** (Review r1) If the resolved `Batcher` is injected into Webster at construction, what is left for the gate to catch? **A:** Nothing — the finding was correct and the decision was self-defeating. Neither row now holds a pre-resolved `Batcher`; both call `batcher.Active` lazily at their own `Call` time, which also makes them resume-identical after a crash at row 10. The mid-run-edit consequence is stated explicitly rather than treated as staleness.
- **Q:** (Review r1) What happens to `stage`, `next_action`, and `history[].bounced_to`? **A:** `stage` and `next_action` are dropped as derivable from the producer list and from `state`+`activity.wait` respectively; `bounced_to` is lost deliberately, since `shedengine.HistoryEntry` has no such field and adding one would edit `shedengine` — provenance stays reconstructible from the history sequence.
- **Q:** (Review r1) What `state` does `Seed` write, and what does it do when the file exists? **A:** `running` — the only enum member meaning "a run may proceed", and the empty string is a hard error at Shed's read gate. It refuses on an existing file: overwriting would destroy an in-flight run's history, which the whole resume contract rests on.
- **Q:** (Review r1) Scope excludes `internal/shedengine` but the task falsifies its `doc.go` divergence paragraph. **A:** Carve out the doc comment. A doc edit adds no import, so the Shed Producer-Seam Invariant — an import allowlist — is untouched; the code exclusion stays absolute.
- **Q:** (Review r1) `batcher.Active` returns an undifferentiated error. **A:** All of them map to `Stuck`, conflation accepted in writing. `Active` already falls back to the embedded template for an absent config, so a remaining error is a broken config far more often than an infra fault.
- **Q:** (Review r1) The doc set omits `docs/overview.md`. **A:** Added — both the `_lyx/` bullet's status wording and the internal-package tree.
- **Q:** (Review r2) Which of the 12 rows are injected, and what told values does the constructor take? **A:** Only the two rows that touch a real substrate are injected — `Preflight` as a `ShedProducer`, `Webster` as `WebsterRun`+`WebsterDeps` (as parts, since the lazy `Batcher` wrapper is `loomshed`-owned). Everything else `loomshed` builds from told paths, with `AnchorPath` and `WorktreeRoot` as separate fields because `planparser.PlanDir` and `planparser.Validate` take different values. The two discussion file paths are told because `loomengine`'s accessors take a `*lyxcwd.Location` that `loomshed` may not import.
- **Q:** (Review r2) The history narrowing fixes the deadlock — what about `state` and `error`, which the same blocked write also sets? **A:** Pinned explicitly: every valid `state` except `done` is tolerated, any `error` is tolerated (it is the halt reason a human resumes after reading), `activity` is never validated because `Shed` composes it, and `current_producer` must name `Preflight`. Without this the deadlock returns through a different field.
- **Q:** (Review r2) The doc set omits docs the change falsifies. **A:** Extended to `manifest/designs/shed.md`, `loom.md`'s State-&-contracts bullet and row-10 Input, with a stated grep-based enumeration method and an explicit not-affected list so the set is reproducible.
- **Q:** (Review r3) Row 9's `batcher.Active` error maps to `Stuck` — what about row 10's wrapper, which resolves the same call lazily? **A:** Identically to `Stuck`. Stated separately because the alternative (returning the error) persists `failed` and aborts `Run`, while `Stuck` persists `blocked`, which a human resumes after fixing the config — the same fault must not end the run one way before Webster and another way at Webster.
- **Q:** (Review r3) Who declares `Shed`'s run-lock path? **A:** Nobody today — this task adds `loomengine.LoomRunLock(l)` at `.lyx/loom/run.lock`, plus its constructor-anchoring and no-transients test entries. Deferring it would leave production wiring with no source for a path the Durable-vs-Ephemeral Invariant requires an accessor for.
- **Q:** (Review r3) `BaseDir` and `AnchorPath` are the same directory. **A:** Correct — `configengine.FindBaseDir` stats `<baseDir>/_lyx` and returns it unchanged, exactly what `planparser.PlanDir` anchors on. `BaseDir` dropped; `AnchorPath` feeds both. `WorktreeRoot` stays separate because `planparser.Validate` takes a genuinely different value.
- **Q:** (Review r3) How does `Seed` actually write? **A:** Under the held lock via `state.UpdateJSON`'s `found` guard, not stat-then-`WriteJSON` (a TOCTOU), and it creates the lock file's parent first — `state.WriteJSON` MkdirAlls the status file's parent but not the lock's, the same gap `preflight.go` already works around.
- **Q:** (Review r3) `shedadapters`' cancellation helpers are unexported and the package is out of scope. **A:** So each `loomshed` producer re-implements the entry/exit check locally. Recorded so a plan writer proposes a small local helper rather than exporting `shedadapters`'.
- **Q:** (Review r3) What type does check 4 read after the migration? **A:** `state.ReadJSONStrict[shedengine.Status]`, then the `product` payload into the thin `loomengine.Status` — `loomengine` gains a `shedengine` import, which the seam invariant permits since it constrains `shedengine`'s imports, not its importers.
- **Q:** (Review r4) Which package owns the `Preflight` wrapper, and where does its integration test live? **A:** `internal/loomshed` exports `NewPreflightProducer(cwd)` and imports `internal/loomengine`; the guard only forbids a *direct* `lyxcwd` import, and transitive is explicitly fine. `Deps.Preflight` stays a bare `ShedProducer` so Tier 1 injects a fake. The integration test lands in `internal/loomshed`, which then needs a `TestMain` with `gitkit.HermeticGitEnv()`. Leaving the wrapper to the caller would leave this task with a row nothing can construct until session bootstrap lands.
- **Q:** (Review r4) What happens to the spec's two per-entry history rules? **A:** Kept, translated: `outcome ∈ {done, stuck}` and `at` RFC3339 UTC. Unlike `activity`, history accumulates across runs and has sanctioned external writers, and Shed's read gate validates only `state` — so the rules still do work.
- **Q:** (Review r4) `loom.md` still says the support-log assertion "lands with `Shed`". **A:** False as of this task; added to the doc set manually, since the status-file-keyed grep cannot reach it.
- **Q:** (Review r5) "Every row faked to `Done`" is impossible when only two rows are injectable. **A:** Correct — rows 3, 7 and 9 are real and read real state. Resolved with a shared Tier-1 fixture builder (both discussion files with all seven H2s, a plan fixture satisfying every `planparser.Validate` check including the path-stat ones, and no `batcher.yaml` so `Active` uses its embedded template), not with more injection points, which would contradict `explicit-deps-struct` and stop the sequence test exercising the three producers this task builds.
- **Q:** (Review r5) The `onstuck-routing` rule as stated does not fit rows 1 and 9. **A:** Missing clause added — a gate whose guarded artifact is produced by no row escalates. `Preflight` gates git state and `Batchifier` gates `batcher.yaml`; no producer writes either, so there is nothing to bounce to.
- **Q:** (Review r5) What does `Seed` take? **A:** `Seed(statusPath, statusLockPath, slug, parent string) error` — bare told paths, not `Deps`, since seeding happens before any `Shed` exists.
- **Q:** (Review r5) `loom.md`'s narration bullet is unaddressed. **A:** Reworded onto `activity`; its `now`/`last`/`wait` example survives verbatim, so only the retired term changes.
- **Q:** (Review r1) Two items were deferred to planning. **A:** Both resolved here. The `Plan-never-reads-support-log` assertion does not land in this task at all (a stub declares no input set to assert against); the `loomshed` import guard is built, and `internal/loomshed` joins the Told-Geometry machine-enforced list in the same commit.


### From _mill/plan/00-overview.md


```yaml
task: 'loom: phase-machine scaffolding'
slug: loom-phase-machine-scaffolding
approved: true
started: '20260819-093203'
parent: standalone-producers
root: ""
verify: null
```

### From _mill/plan/01-status-schema-migration.md


```yaml
task: 'loom: phase-machine scaffolding'
batch: status-schema-migration
number: 1
cards: 2
verify: go test ./internal/loomengine/... ./internal/lyxcwd/... ./cmd/lyx/... && go test -tags integration ./internal/loomengine/...
depends-on: []
```



- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/loomstatus_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomengine/status.go`
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/coherence_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `contracts/specs/loom-status-spec.md`
  - `internal/shedengine/doc.go`
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-loomshed-producers.md


```yaml
task: 'loom: phase-machine scaffolding'
batch: loomshed-producers
number: 2
cards: 8
verify: go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [1]
```



- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:**
  - `internal/loomshed/doc.go`
  - `internal/loomshed/ctx.go`
  - `internal/loomshed/ctx_test.go`
  - `internal/loomshed/seam_enforcement_test.go`
- **Deletes:** none
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/stub_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/discussionvalidate_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomshed/planvalidate.go`
  - `internal/loomshed/planvalidate_test.go`
- **Deletes:** none
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:**
  - `internal/loomshed/batchifier.go`
  - `internal/loomshed/webster.go`
  - `internal/loomshed/batchifier_test.go`
  - `internal/loomshed/webster_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomshed/preflight.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/loomshed_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomshed/seed.go`
  - `internal/loomshed/seed_test.go`
- **Deletes:** none

### From _mill/plan/03-sequence-and-integration.md


```yaml
task: 'loom: phase-machine scaffolding'
batch: sequence-and-integration
number: 3
cards: 4
verify: go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/... && go test -tags integration ./internal/loomshed/...
depends-on: [2]
```



- **Edits:** none
- **Creates:**
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/sequence_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomshed/resume_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomshed/preflight_integration_test.go`
  - `internal/loomshed/testmain_integration_test.go`
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `manifest/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
   When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure.
   Do NOT pick one side wholesale just because the region overlaps syntactically;
   picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values).
   Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content.
   This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file;
   it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path.
   Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk;
     keep only the other side's unrelated edit.
     Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file.
     The resolution keeps the item only under `## Done`;
     it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`.
     Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item.
     The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/loom-phase-machine-scaffolding add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/loom-phase-machine-scaffolding rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/loom-phase-machine-scaffolding rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
   Do NOT silently keep the modification.
8. Before reporting `{"status":"success"}` (with or without `discarded`), re-read each file listed in Conflicting files in full and explicitly verify no contradictory losing-side claims survive the resolution — e.g. a stale value from one side of the conflict left alongside the correct value from the other side, or a claim that only made sense before the other side's edit was applied.
   If you find a contradiction you missed, fix it before reporting.
   If you find a contradiction you cannot confidently resolve, report `{"status":"stuck","stuck_type":"logic","reason":"self-verification found an unresolved contradiction in <file>: <description>"}` instead of `{"status":"success"}`.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost.
If anything was discarded, you MUST list it;
an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity.
The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/loom-phase-machine-scaffolding` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/loom-phase-machine-scaffolding`.
