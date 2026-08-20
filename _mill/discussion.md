# Discussion: Bouncer: the generic review-gate producer

```yaml
task: 'Bouncer: the generic review-gate producer'
slug: shedadapters-generic-bouncer-producer
status: discussing
parent: main
```

## Problem

`perch` today is one opaque `Shed` row backed by `internal/perchengine`, which delegates its round loop to `internal/treadleengine`.
That gives every review gate in `loom` a second, nested loop with its own round-caps ladder, its own progress judge, and its own pause and locking machinery — all of it duplicating what `Shed` now does natively.
The previous roadmap item (`shedengine: per-producer bounce budget + explicit OnDone routing`, shipped as commit `fa71d2a9`) moved the loop into `Shed` itself: a `ProducerDef` now carries its own episode-scoped `MaxBounces` and an explicit `OnDone`, so a review gate can be expressed as two ordinary rows that bounce off each other rather than one row hiding a loop.

This task builds the **judge half** of that pair: the `Bouncer`.
It is the piece that decides whether an artifact has passed, and — unlike the Burler-round producer it gates, which is `burlerengine`-specific — it is genuinely domain-agnostic.
It is parametrized purely by a rubric stencil name and a report/ledger file-path convention, never by which round producer sits opposite it.
Why now: the three `loom` review-producer tasks (`Discussion-Review`, `Plan-Review`, `Webster-Review`) all instantiate this one producer, and none of them can start until it exists.
It is also one of the two pieces the Someday `Tenter` review-loop is expected to reuse verbatim, so it must not acquire a `loom`-shaped or `burler`-shaped dependency.

## Scope

**In:**

- A new `Bouncer` producer at `internal/shedadapters/bouncer.go`, satisfying `shedengine.ShedProducer`, with one exported constructor.
- Its three-mode `Call`: the **seed call** (no report exists), the **judge call** (the highest contiguous round's report exists and has no verdict yet), and the **re-bounce** (that round's verdict already exists) — told apart by file existence alone.
- Fail-loud parsers, in-package, for the three file contracts this producer owns: the bouncer verdict, the finding-identity ledger, and the structured next-round focus file.
- A writer for the structured focus file, used both by the seed call's mechanical fallback and by any path that must synthesise an empty-exclusions focus file.
- Two new generic stencil templates, `contracts/stencils/bouncer/bouncer-template-seed.md` and `contracts/stencils/bouncer/bouncer-template-judge.md`, registered in `contracts/stencils/stencils.go`.
- An exported round-resolution helper in `internal/shedadapters` so the later Burler-round producer resolves the same round number from the same on-disk convention.
- Package-doc updates in `internal/shedadapters/doc.go` (the outcome-mapping table, the shared cancellation rule, and the pointer rule all gain a fourth entry) and a note in `manifest/designs/shed.md`'s engine-adapter section.
- Tests: table tests over a fake `Shuttle` for every `Call` path and every fail-safe degradation, parser tests for every malformed-file rule, and a registry-completeness case for the two new stencils.

**Out:**

- The `shedadapters: Burler-round producer` item — a separate roadmap item and a separate task, even though it is the Bouncer's counterpart row.
  Nothing in this task may import `internal/burlerengine`.
- Any `loom` wiring: no `Discussion-Bouncer`, no `Plan-Bouncer`, no `Webster-Bouncer` instance, no producer list, no change to `internal/loomengine`.
- Any per-segment review rubric.
  This task ships the *generic* templates with a `rubric` marker; the concrete rubrics are written by the three `loom` review-producer tasks.
- Retiring or deprecating `internal/perchengine`, `internal/treadleengine`, `shedadapters.PerchProducer`, or the `lyx perch run|pause` CLI.
  That call belongs to the Someday `Bouncer → Perch` item and stays deferred.
- `manifest/roadmap.md` movement beyond marking this item shipped when it lands.
- Any milestone/round-caps ladder, progress judge, asking-triage, pluggable command gate, or run-dir locking.
  Those are `treadleengine`'s machinery and are exactly what the flattening deletes.

## Decisions

### Package placement — `internal/shedadapters`, not a new engine package

- Decision: the Bouncer lands as `internal/shedadapters/bouncer.go`, alongside `singlellm.go`, `perch.go`, and `webster.go`.
  No new `internal/bounceengine`.
- Rationale: mechanically it *is* an adapter over `shuttleengine` — one spawn per `Call`, exactly like `SingleLLMProducer` — with judge-specific work before and after the spawn.
  It reuses this package's existing shared helpers (`entryErr`, `cancelErr`, `archiveStaleOutputs`) directly.
  Splitting it into its own engine would create a package whose entire public surface is one `Call` plus three parsers, and would then need a second, empty adapter file in `shedadapters` anyway.
  The task slug itself (`shedadapters-generic-bouncer-producer`) already pins this placement.
- Rejected: a new `internal/bounceengine` with a thin `shedadapters` wrapper.
  Follows the engine/adapter split precedent, but buys a second package and a second seam for no additional isolation — the Bouncer has no loop of its own to protect.

### Round resolution — told a run dir plus a report-name convention

- Decision: the constructor is told an absolute `RunDir` and a report-name function `func(round int) string`.
  `Call` resolves the current round by scanning upward from round 1 for the highest round whose report file exists, then asking whether the *next* round's report exists yet.
  Concretely: if no report exists at all, this is the seed call for round 1; otherwise the highest existing report is the round to judge.
  An exported helper in `internal/shedadapters` implements this scan so the Burler-round producer can share it verbatim.
- Rationale: "told, never derived" is this package's stated rule — every constructor already receives resolved absolute paths and constructs nothing from `lyxcwd`, `os.Getwd`, or git.
  Resolving from disk each `Call` rather than holding a round counter in memory is also what makes a process restart resolve the same round, the same property `PerchProducer`'s run-id scheme buys.
  Passing the naming convention as a function keeps the Bouncer ignorant of `burler`'s `round-<N>-review.md` spelling while still letting both halves of a segment agree.
- Rejected: (a) a bare `ReportPathFor func(int) string` with no run dir — leaves the ledger, verdict, and focus files with no anchor to be written under, and no directory for stale-output archiving.
  (b) Hardcoding `round-<N>-review.md` — couples a deliberately generic producer to one round producer's file naming, which is precisely what the roadmap says this producer must not do.

### File contracts — own fail-loud parsers, no `treadleengine` import

- Decision: the Bouncer defines and parses its own three file contracts in-package, each YAML frontmatter over unconstrained prose, each fail-loud with a `bouncer: `-prefixed error.
  It does not import `internal/treadleengine`.
- Rationale: this resolves the roadmap's explicitly-deferred "reuse option to resolve during this task".
  `treadleengine.ParseJudgeVerdict` is *not* reusable — its second parameter's type `judgeFraming` is unexported, so no external caller can construct an argument for it, a fact `perchengine`'s own doc already records.
  `ParseHandoff` *is* callable, but its schema is treadle-loop-specific (`covers_rounds` exists to bound a judge's read-set across a nested round loop that no longer exists here), and its `Handoff` type would leak that vocabulary into the Bouncer's public shape.
  Beyond the schema mismatch: `treadleengine` is the package this producer is built to make retirable, so importing it would create a dependency pointing the wrong way down the retirement path.
  The parsers are small and the error posture is well-established — copy the *posture* from `burlerengine.ParseReview` and `treadleengine.ParseHandoff`, not the code.
- Rejected: importing `ParseHandoff` for the ledger and writing a fresh verdict parser.
  Saves roughly sixty lines at the cost of a dependency on a package slated for retirement, and a public type carrying a field this design has no use for.

### The three file contracts

- Decision:
  - **Verdict file** — frontmatter carries `verdict` (exactly `APPROVED` or `BLOCKING`, case-sensitive) and a non-empty `rationale`; prose after is unconstrained and human-facing.
    A verdict outside the two spellings, a missing or empty rationale, or malformed frontmatter is a parse error.
  - **Ledger file** — frontmatter carries `round` (positive int) and `ledger`, a list of entries each with a non-empty `key`, a non-empty `rounds` list of positive ints, and a `status` of exactly `open` or `resolved`.
    An empty `ledger` list is legal (a first ledger carries no prior findings).
    Prose after the frontmatter is the distilled cross-round narrative.
    Every entry from the previous ledger is carried forward, open or resolved, never dropped — the prompt states this, and the Go side does not enforce it.
    **Known soft spot, deliberately accepted in this task:** because carry-forward is prompt-enforced only, a misbehaving judge LLM can silently drop a ledger entry with nothing at the Go layer catching it, which would lose a recurring finding from the cross-round record.
    Go-side enforcement is a real feature rather than a one-line addition — the parser would have to diff the new ledger's key set against the *previous* ledger's and decide what a missing key means (reject, or re-open with a warning) — so it is scoped out here and left as a candidate follow-up.
    The plan should treat this as a named soft spot, not a gap to close: the `judge` template must state the carry-forward rule prominently, and a parser test should assert the *absence* of enforcement so the gap is explicit in the test record rather than merely unmentioned.
  - **Focus file** — frontmatter carries `round` (positive int), `exclude_lenses` (list of strings, possibly empty), and `focus` (list of strings, possibly empty).
    Prose after is optional rationale.
    Unknown extra keys are tolerated in all three files (no `KnownFields`), matching `reviewHeader` and `judgeHeader`.
- Rationale: three files, three jobs — a machine-read verdict, a cross-round identity record, and a mechanically-parseable next-round directive.
  The roadmap is explicit that the focus file must be structured rather than prose, unlike treadle's `PreRoundTargeting`, precisely so the round producer's *Go code* decides whether to drop a lens, not an LLM's discretion.
  Keeping `exclude_lenses` and `focus` as always-present, possibly-empty lists is what lets the seed call write the same shape as a judge call, so the round producer has exactly one format to read.
  Tolerating unknown keys matches the yaml-strictness-split decision already recorded in this codebase: agent-written metadata in a header is harmless noise, unlike the CLI's strict profile decode.
- Rejected: pure YAML with no prose section for the ledger and focus files — loses the human-readable rationale that makes an operator able to eyeball what the judge was thinking, which is the same reason treadle's handoff and judge files carry prose.

### Artifact naming and paths

- Decision: the Bouncer writes its three files flat in `RunDir`, named `round-<N>-bouncer-verdict.md`, `round-<N>-bouncer-ledger.md`, and `round-<N>-focus.md`, where `N` is the round the file pertains to.
  The seed call's spawn declares exactly one output file, `round-1-focus.md`.
  A judge call for round `N` declares exactly three, **unconditionally**: `round-<N>-bouncer-verdict.md`, `round-<N>-bouncer-ledger.md`, and `round-<N+1>-focus.md`.
  The judge writes all three on every call, including when its verdict is `APPROVED` — in that case `round-<N+1>-focus.md` is simply never read, because the segment exits.
- **The spawn's `OutputFiles` list is unconditional, and this is load-bearing.**
  `shuttleengine` classifies a run `done` only when *every* `Spec.OutputFiles` entry exists — `allOutputFilesExist` at `internal/shuttleengine/wait.go:279`, consulted at `wait.go:222`; a run whose files are not all present is classified `OutcomeAsking` instead.
  So a three-entry list whose third entry the judge writes "only when `BLOCKING`" would make every `APPROVED` run classify non-`done`, fall into this design's own fail-safe degradation, and return `Stuck` — making `Done` unreachable and the segment unable to ever approve anything.
  Writing an unused focus file on approval is a few hundred wasted bytes; a conditional output file is a producer that cannot terminate.
  The judge template states this explicitly: always write all three, and on `APPROVED` write the focus file with empty `exclude_lenses` and empty `focus`.
- Rejected for the focus file's authorship: having the *Bouncer* render `round-<N+1>-focus.md` itself after parsing a `BLOCKING` verdict, keeping the spawn at two output files.
  It satisfies the file contract equally well, but the focus file's whole purpose is to carry the judge's own assessment of which lenses found nothing — content only the LLM has — so the Bouncer would have to smuggle it out through the ledger's frontmatter and re-render it, which is one more format and one more failure point for no gain.
- Rationale: flat-in-run-dir with a `round-<token>-<kind>.md` shape is the convention `treadleengine/roundfiles.go` already established and every existing artifact follows, so an operator reading a run dir sees one naming scheme, not two.
  The `bouncer-` infix keeps the verdict and ledger distinguishable from a round producer's own files in the same directory.
  Numbering the focus file by the round it *targets* (rather than the round that wrote it) means the round producer reads `round-<its own N>-focus.md` with no off-by-one reasoning.
- Rejected: a `bouncer/` subdirectory under the run dir — an extra directory level for three files, and it splits one segment's artifacts across two places for a human reading them.

### Three modes, told apart by file existence only

- Decision: `Call` resolves its mode from two file-existence questions and nothing else — no state is threaded through `Call(ctx)`.
  Let `N = ResolveRound(RunDir, ReportName)` (see "An exported round-resolution helper" below), and let **`judged(N)`** be the predicate defined immediately after this list.
  - `N == 0` and `round-1-focus.md` **absent** → **seed call**: spawn, write `round-1-focus.md`, return `Stuck` with an empty pointer and no verdict.
  - `N == 0` and `round-1-focus.md` **present** → **re-bounce**: the segment was already seeded and the round producer handed control back without producing round 1's report.
    Spawn nothing, log a `logger.Warn`, return `Stuck` with an empty pointer (no ledger exists yet).
  - `N >= 1` and **not** `judged(N)` → **judge call**: read the report plus the previous round's ledger (when one exists), spawn the judge, parse its verdict, and return `Done` on `APPROVED` or `Stuck` on `BLOCKING`.
  - `N >= 1` and `judged(N)` → **re-bounce**: round `N` is settled, so the round producer handed control back without producing a new report.
    Spawn nothing, log a `logger.Warn` naming the producer and round `N`, and return `Stuck` with `round-<N>-bouncer-ledger.md` as the pointer — which `judged(N)` guarantees exists.

  **`judged(N)`** is true only when all three hold: `round-<N>-bouncer-verdict.md` exists, `round-<N>-bouncer-ledger.md` exists, **and** the verdict file parses cleanly.
  Anything less means a judge call for round `N` started and did not finish — most plausibly a spawn that wrote the verdict but not the ledger, which `shuttleengine` classifies non-`done` (see the unconditional-`OutputFiles` note under "Artifact naming and paths"), degrading to `Stuck` and leaving a verdict file on disk with no ledger beside it.
  A partially-written round is **not** judged, so the next `Call` re-judges it, archiving the debris on the way in via the ordinary stale-output step.
  Re-judging costs one more session; treating the debris as a settled verdict would silently accept a round no judge ever finished assessing, and — because `Shed` never stats a pointer — would then report a ledger path pointing at nothing.
  Requiring the *parse* as well as the file, rather than existence alone, is what makes a truncated or malformed verdict re-judged rather than permanently mistaken for a decision.
- Rationale: `ShedProducer.Call(ctx)` has no parameter to carry mode in, by design — the seam is deliberately minimal.
  File existence is the same discriminator already needed to fix the `Discussion-Validate`/`Plan-Validate` findings-discarded-on-`Stuck` gap, so this is an established pattern rather than a new one.
  It also survives a crash-restart for free: the disk state *is* the mode.

  The re-bounce mode is not hypothetical, and consulting the report file alone would be a live bug.
  The roadmap pins the round producer to return `Stuck` with `OnStuck` at the Bouncer *unconditionally* — it "always returns `Stuck` ..., never `Done`", including on its own degraded paths.
  So a round producer that bounces back without writing a report leaves the highest report at `N`, whose verdict already exists;
  a report-file-only discriminator would then re-spawn a full judge call on an already-judged round, archive the prior verdict and ledger, pay for the session a second time, and repeat until the bounce budget is exhausted — burning the whole budget on LLM calls that re-decide a settled round.
  Consulting verdict presence turns that into a cheap, logged, zero-cost bounce that still ticks the budget, so the segment terminates on the same boundary it always would have, just without paying for it.
- Rejected: (a) a mode field on the producer struct mutated across calls — an in-memory flag that a process restart silently resets, which is the exact failure the persisted-bounce-budget decision in `shedengine` overturned.
  (b) Discriminating on report *mtime* versus verdict mtime — works, but makes correctness depend on filesystem timestamp granularity and on nothing ever rewriting a file in place, where plain existence needs neither.
  (c) Treating a re-bounce as a hard error — an infrastructure hiccup in the round producer would kill an unattended run, and `Stuck` already routes it back to the producer that can retry.

### The Bouncer is the segment's entry point, and never falls through

- Decision: the Bouncer's `ProducerDef` sits where the segment's slot is in the producer list and inherits control from the previous stage's `Done`.
  Its `OnStuck` names the round producer, for both the seed call and a rejection.
  Its `OnDone` is set explicitly to whatever follows the segment and never left empty.
  The round producer's physical list position carries no routing meaning.
- Rationale: this is what makes the Bouncer run *before* the first round exists, which is what the seed call needs.
  `ProducerDef.OnDone`'s empty value is load-bearing and silent — it ends the whole `Shed` run quietly — so leaving it unset on an approval would end the run rather than advance the pipeline.
  This decision is a documentation and constructor-doc obligation in this task, not code: the wiring lives in the three `loom` tasks.
  `shedengine.validate()` already requires a non-empty `OnStuck` to name a target sharing the same `Segment`, so the pair must share a `Segment` label.
- Rejected: nothing — the roadmap pins this shape.

### The constructor surface — a config struct, validated, returning an error

- Decision: the constructor is `NewBouncer(cfg BouncerConfig) (*Bouncer, error)`, and `BouncerConfig` carries exactly these fields:

  | Field | Meaning | Validation |
  |---|---|---|
  | `Name` | log-field and error-text identity only — never compared, parsed, or used for control flow | non-empty |
  | `RunDir` | absolute run dir every artifact path is joined onto | non-empty, absolute |
  | `ReportName` | `func(round int) string` naming the round producer's report artifact | non-nil |
  | `StencilsDir` | absolute, already-resolved stencils directory | non-empty, absolute |
  | `RubricStencil` | stencil *name* of this instance's rubric | non-empty |
  | `Model` | model the judge spawn runs as | may be empty (defers to the provider default) |
  | `Effort` | reasoning-effort override for the judge spawn | may be empty (defers to the provider default) |
  | `Shuttle` | the seam the two spawns run through | non-nil |
  | `Now` | injected clock, for the archive filename's same-second collision suffix only | nil defaults to `time.Now` |

  `Model` and `Effort` are **told, never resolved here**: the Bouncer receives an already-resolved `(model, effort)` pair and threads it into `shuttleengine.Spec.Model`/`.Effort` verbatim.
  It never parses a model-spec string, never loads `models.yaml`, and never consults a config file.
  Resolution happens once, at the caller's own config-load time — which for the three `loom` instances means `loom`'s per-phase config, not any engine-general config file.
- Rationale: nine inputs with real invariants is past the point where a positional constructor reads well, and two of them (`RunDir`, `StencilsDir`) must be absolute — a rule that has to be enforced *somewhere*, and the only alternative to the constructor is discovering it mid-`Call` inside an unattended run.
  Returning an `error` is a deliberate deviation from this package's three existing constructors, which return a bare pointer;
  the deviation is worth calling out in the package doc, and it is justified because those three either validate nothing (`SingleLLMProducer`, whose `Spec` is validated downstream by `shuttleengine`) or take already-constructed engines that validated themselves.
  Keeping `Model`/`Effort` told rather than resolved follows the rule `perchengine`'s own doc states for the whole stack — knowledge of phases and lifecycle collects in the product layer, and the engines beneath it stay phase-agnostic.
  `Name` being log-and-error-text-only is the package's existing convention for exactly the reason it documents: `Call(ctx)` carries no identity, and two instances of one adapter type in one producer list is the expected shape — which here is not merely expected but certain, since `loom` instantiates three.
- Rejected: (a) a positional `NewBouncer(name, runDir, reportName, stencilsDir, rubric, model, effort, shuttle, now)` — nine positional arguments, four of them `string`, is a call site where a transposition compiles silently.
  (b) Validating lazily at first `Call` — turns a wiring typo into a mid-run failure, and `Shed` would record it as an engine-level error rather than surfacing it at construction.

### Failure posture — fail-safe toward another round, never toward approval

- Decision: any judge-call infrastructure failure — stencil read, `stencil.Fill`, shuttle `Run` error, a non-`done` shuttle outcome, an unreadable or unparseable verdict file, an unparseable ledger file — logs a `logger.Warn` naming the producer, the round, and the cause, and returns `Stuck` with the round producer as the bounce target.
  It is never a hard error, and never `Done`.
  A malformed *previous* ledger degrades further but identically: log a `Warn` and run the judge with no prior ledger, exactly as treadle's `latestValidHandoff` falls back.
- Rationale: treadle's own recorded reasoning applies unchanged — a false `STUCK` costs a few extra bounded rounds, a false pass ships an unreviewed artifact.
  What makes fail-safe *safe* here is that the damage is now bounded by `ProducerDef.MaxBounces` (episode-scoped, counted from persisted history, so a crash-restart loop cannot refresh it) rather than by treadle's hard cap.
  Degrading toward `Done` would be the one genuinely unsafe direction and is forbidden outright.
- Rejected: (a) hard error on the first failure — one flaky spawn kills a long unattended run, and `Shed`'s whole point is surviving that.
  (b) Fail-safe once, hard error on the second consecutive failure — requires persisting a consecutive-failure counter the producer has nowhere to put, to cap something the bounce budget already caps.

### Seed call — spawns, with a mechanical fallback

- Decision: the seed call spawns the judge model against `bouncer-template-seed.md` to write `round-1-focus.md`.
  If that spawn fails at any point, the Bouncer writes `round-1-focus.md` itself with `round: 1`, empty `exclude_lenses`, and empty `focus`, logs a `Warn`, and still returns `Stuck`.
- Rationale: the roadmap requires the rubric to cover the seed call's focus-setting pass, not only post-round judgment — which only means anything if an LLM actually reads the rubric and the artifact at seed time.
  This is also what makes `Crucible`'s current behavior (an orchestrator picks focus before spawning a reviewer round) the working precedent for `Tenter`'s eventual reuse rather than a loose analogy.
  The mechanical fallback exists because a missing focus file would otherwise break the round producer's read path on round 1, and losing the focus guidance is a far cheaper failure than blocking the first round entirely.
- Rejected: a purely mechanical seed that never spawns — contradicts the roadmap's explicit rubric requirement, and gives the first round no targeting at all.

### No round-cap ladder, no progress judge

- Decision: the Bouncer carries no round caps, no milestone rungs, no circling check, and no continuation gate.
  Termination is `ProducerDef.MaxBounces` alone.
- Rationale: the previous roadmap item moved exactly this machinery into `Shed`'s per-producer bounce budget.
  Re-implementing a ladder inside the Bouncer would restore the nested loop the flattening exists to delete, and would leave two independent termination authorities disagreeing about when a segment is done.
- Rejected: porting treadle's `round_caps` ladder — re-imports the complexity this initiative is removing.

**Budget semantics, and the seed call's off-by-one.**
`shedengine.episodeStuckCount` (`run.go:275`) counts every `Stuck` a producer authored since that producer's own most recent `Done`, walking the persisted history backward and skipping entries authored by other producers.
The Bouncer returns `Done` only on approval, and approval exits the segment — so within one segment episode the Bouncer never authors a `Done`, and its episode never resets.
The seed call's unconditional `Stuck` therefore permanently consumes one unit of the budget.

The resulting rule, which is what the three `loom` wiring tasks and the constructor doc comment must be told: **a Bouncer configured with `MaxBounces: N` gets `N` judged rounds, and the `N`th one blocks the run if it comes back `BLOCKING`.**
Deriving it: the budget check at `run.go:197` counts `st.History` — the slice read *before* this call's `Stuck` is appended, deliberately and with a comment saying so — and blocks when that count already equals the budget.
So `Stuck` number `N+1` is the one that blocks, and the Bouncer's `Stuck` sequence is [seed, reject round 1, reject round 2, ...].
With `N = 1`: the seed bounces, the round producer runs round 1, and the Bouncer's judge call for round 1 *runs* — returning `Done` if it approves, or blocking the run with its `Stuck` if it does not.
One judged round, not zero.
Generally, `N` rounds are judged and the `N`th rejection is what exhausts the budget.

This is documented rather than compensated for in code, deliberately: silently adding one inside the Bouncer would make `MaxBounces` mean something different for this producer than for every other one in the list, which is worse than an offset an operator can read about.
Note also that a re-bounce consumes budget on exactly the same terms, which is what makes that mode safely terminating rather than a free loop.

### Stale outputs — archive, using the existing shared helper

- Decision: before each spawn, `Call` archives any already-existing files among the spawn's `OutputFiles`, via this package's existing `archiveStaleOutputs` helper and the same injected clock convention `SingleLLMProducer` uses (a nil `now` defaults to `time.Now`; the clock resolves only the archive filename's same-second collision suffix).
- Rationale: `shuttleengine.Spec.validate` rejects a pre-existing `OutputFiles` entry outright, because a stale file would satisfy the file contract on the first turn end and silently classify an unfinished run as done.
  So a crash between "wrote the verdict" and "recorded the outcome" would hard-fail the resume without this.
  The helper already exists in this package and is already tested; reusing it also keeps the archive naming identical across all four adapters.
- Rejected: deleting stale outputs — destroys the partial artifact an operator would want when diagnosing why the resume happened.

### Cancellation and the output pointer

- Decision: `Call` checks `entryErr(ctx, ...)` at entry and returns immediately without starting anything.
  On exit, `cancelErr` replaces every result *except* a genuinely parsed verdict, which is returned as its mapped `Done`/`Stuck` with its pointer regardless of cancellation.
  The `OutputPointer.Path` is the bouncer ledger path **only when a ledger was actually written and parsed** — that is, on `Done`, and on the `BLOCKING` `Stuck` of a completed judge call.
  Every other path reports an empty pointer: the seed call, every degraded judge path (which returns `Stuck` without a ledger having been written), and the re-bounce mode's pointer to a prior round's ledger is the one exception — that ledger provably exists, since its round was judged, so the re-bounce reports it.
  Stated as a rule: the pointer names a file the Bouncer has verified exists, or it is empty; it is never a path that merely *would* have been written.
  No mid-run cancellation bridge is installed.
- Rationale: the entry/exit rule is the package's stated shared cancellation rule and the reasoning transfers directly — converting a finished verdict into the context error would make `Shed` record no history entry, so the next `Call` would archive a valid artifact and pay for the same LLM session twice.
  The pointer choice deliberately *differs* from `PerchProducer`, which reports an empty pointer on the grounds that a gate producer's verdict is always re-derived rather than read back; the Bouncer's ledger is a real cross-round artifact a human reads, and hiding it on a `BLOCKING` `Stuck` would hide it exactly when an operator most needs it.
  The exists-or-empty rule matters because `Shed` never stats a pointer — `OutputPointer`'s own doc is explicit that `Shed` "never introspects Path's contents, never validates it, and never stats it to make a control-flow decision".
  So a pointer naming an unwritten file is not caught anywhere; it is simply persisted into `history[].output` and read later by an operator or a status renderer as though the artifact were there.
  This delta must be stated explicitly in the package doc so a later reader does not read it as drift.
  No mid-run bridge because `shuttleengine` exposes no such seam — the same limitation `SingleLLMProducer` and `WebsterProducer` already document.
- Rejected: empty pointer on `Stuck`, ledger path on `Done` — matches `PerchProducer` cosmetically while losing the artifact in the case that matters.

### Stencils — two generic templates, rubric injected by name

- Decision: two new files under `contracts/stencils/bouncer/`, `bouncer-template-seed.md` and `bouncer-template-judge.md`, each registered in `contracts/stencils/stencils.go` with its own `//go:embed` var and registry entry.
  Each carries a `rubric` marker.
  The constructor is told a rubric *stencil name*; `Call` reads it via `stencilstore.Read` from the told `StencilsDir` and fills it into the `rubric` marker.
- Rationale: this is what "parametrized purely by a rubric stencil path" means concretely — the generic prompt is shipped once, and each segment's rubric is a separate stencil the `loom` tasks will write.
  Reading the rubric through `stencilstore.Read` at call time, from a told absolute directory, is required by the Stencil Ownership Invariant; taking rubric *bytes* in the constructor would bypass edit detection and hash stamping entirely.
  Two templates rather than one because a single prompt trying to be both a focus-setter and a judge reads badly, and because a rubric must be able to speak to the two passes differently.
  Per the Producer Pointer-Rule Invariant, both templates *point at* the three file-format contracts rather than restating them, so editing the format in one place changes what the producer and its consumers both do.
- Rejected: one template with a mode marker — saves a file, costs prompt clarity in the two places clarity matters most.

### An exported round-resolution helper — signature and semantics pinned here

- Decision: the round-resolution scan is an exported function in `internal/shedadapters`, not an unexported method on the Bouncer, and its exact contract is pinned in this discussion rather than left to the plan or to either consumer:

  ```go
  // ResolveRound returns the highest round N for which reportName(N) names an existing file
  // inside runDir, scanning upward from 1 and stopping at the first absent round.
  // It returns 0 when reportName(1) does not exist. It returns an error only when runDir
  // itself is unreadable -- never for an absent report, which is an ordinary answer.
  func ResolveRound(runDir string, reportName func(round int) string) (int, error)
  ```

  Semantics, binding on both halves of a segment:
  - **The scan is contiguous.** It stops at the first absent round and never looks past the gap.
    With reports for rounds 1 and 3 present and round 2 absent, `ResolveRound` returns `1`, and round 3's report is treated as though it did not exist.
  - **`0` means "no round has been run yet"** — the Bouncer's seed case. It is a legal, non-error answer.
  - **The Bouncer's "round to judge" is the return value itself.**
  - **The round producer's "round to write" is the return value plus one.**
    The two consumers differ by exactly one on identical disk state, and that difference is expressed at the call sites rather than by two functions.
  - **An unreadable `runDir` is an error**, not a silent `0` — a mistyped or not-yet-created run dir must never look like a fresh segment about to be seeded.
- Rationale: the Burler-round producer must resolve *the same* round number from *the same* on-disk convention, or the two halves of a segment disagree about which round they are in — a bug that would surface as a silently skipped or double-judged round.
  Exporting it now, in the task that defines the convention, is what prevents the later task from duplicating and drifting.
  Pinning the return *shape* matters as much as exporting the function: "round to judge" and "round to write" are both plausible readings of a bare `ResolveRound`, and two consumers that each pick a different one would drift while sharing the same code.
  Contiguity is the right gap rule because contiguity is what the segment produces by construction — round `N+1`'s report is only ever written after round `N` was judged — so a gap means something wrote out of band, and the contiguous prefix is the only part of the sequence the segment itself authored.
  Stopping at the gap also fails in the safe direction: the next judge call re-judges the highest *contiguous* round rather than skipping over an unjudged one.
- Rejected: (a) keeping it unexported and letting the Burler item write its own — guarantees drift between two functions that must agree exactly.
  (b) Returning the highest existing report regardless of gaps — silently skips judging every round inside the gap, which is the exact failure mode the helper exists to prevent.
  (c) Returning `(round, isSeed bool)` — encodes one consumer's question into a signature both must share; `0` already carries it without privileging the Bouncer's reading.

## Technical context

**The seam being implemented.**
`internal/shedengine/producer.go` defines `ShedProducer` as a single method, `Call(ctx context.Context) (Outcome, OutputPointer, error)`.
Two obligations bind every implementation and cannot be enforced mechanically: return exactly `Done` or `Stuck`, and surface context cancellation as a non-nil error, never as `Stuck`.
`OutputPointer.Path` is never introspected, validated, or stat'd by `Shed` — step 4 of the loop is an unconditional re-call.

**Routing and budget** live in `shedengine.ProducerDef`, not in the producer: `OnStuck`, `OnDone`, `Segment`, and `MaxBounces`.
`MaxBounces` of 0 inherits `Shed.MaxBounces`, which at 0 falls back to an internal default of ten.
The budget is per-producer and episode-scoped, counted from persisted `history[]` rather than an in-memory counter — deliberately, so a crash-restart loop cannot hand itself a fresh budget.
`validate()` enforces that a non-empty `OnStuck` names a target sharing the producer's `Segment`.

**Existing adapters to mirror**, all in `internal/shedadapters`:

- `singlellm.go` — the closest structural sibling.
  Read it for the exact shape: `entryErr` at entry, build the spec, guard against relative `OutputFiles`, `archiveStaleOutputs`, run the seam, then a `switch` over the engine's outcome where every non-success branch calls `cancelErr` first.
- `perch.go` — the current review-gate adapter this pair supersedes; read it for the resolve-identity-from-disk-each-`Call` pattern.
- `ctx.go` — `entryErr(ctx, name, engineLabel)` and `cancelErr(ctx, name, engineLabel)`, the shared cancellation helpers.
- `archive.go` — `archiveStaleOutputs(paths []string, now func() time.Time)`.
- `doc.go` — the package doc carries an outcome-mapping table, a "told, never derived" section, a shared-cancellation-rule section, and a limitations section.
  All four need a Bouncer entry, and the pointer-rule delta versus `PerchProducer` must be called out explicitly.

**The judge spawn pattern** is `internal/treadleengine/judge.go`.
Read `runJudgeCall` in particular — it is the reference for the whole sequence: `stencilstore.Read` for the template, `stencil.Fill` with a `map[string]string` of marker values, a `shuttleengine.Spec` carrying `Prompt`, `OutputFiles`, `Model`, `Effort`, `Role`, and `Round`, then `sh.Run`, then read and parse the output file, with every step degrading to a `Warn` plus a fallback.
Note `previousHandoffMarker` there: `stencil.Fill` has no conditionals and requires every marker to resolve to *some* value, so a "none yet" case needs its own literal (`"(none)"`) rather than an empty string.
The Bouncer's previous-ledger marker needs the same treatment.
**Do not declare a `Shuttle` interface.**
`internal/shedadapters/singlellm.go:22-27` already declares `type Shuttle interface { Run(shuttleengine.Spec) (shuttleengine.Result, error) }` and its `var _ Shuttle = (*shuttleengine.Runner)(nil)` compile-time proof **in this same package**, so a second declaration of either is a compile error.
The Bouncer reuses both as they stand and adds neither.
(`treadleengine/judge.go` declares its own only because it is a different package.)
The Bouncer's own struct holds a `Shuttle` field exactly as `SingleLLMProducer` does, and the same package-level fake serves both adapters' tests.

**Parser posture** is `internal/burlerengine/verdict.go` (`ParseReview`) and `internal/treadleengine/judgeverdict.go`/`handoff.go`.
Both use the same `splitFrontmatter` shape: the file must open with a `---` line and have a closing `---` line, the header between them must be non-empty and valid YAML, and everything after is unconstrained prose.
`treadleengine/handoff.go`'s `frontmatterProse` shows the prose-extraction scan, CRLF-normalised and trimmed.
Both packages deliberately omit `KnownFields` so unknown agent-written keys are tolerated.
Note the two-layer posture both packages describe: the parser is fail-loud, and the fail-*safe* swallowing lives one layer up in the caller.
The Bouncer has both layers in one package — keep them in separate functions so the distinction stays visible.

**Artifact naming** is `internal/treadleengine/roundfiles.go`.
It shows the flat-in-run-dir convention and the `roundToken` attempt-suffix scheme (`3`, `3b`, `3c`).
The Bouncer has no attempt concept — retries are `Shed` bounces, not in-producer retries — so it uses the bare round number, but the file layout and the "single place that turns a round into concrete paths" discipline are worth copying.

**Stencil registration** is `contracts/stencils/stencils.go`, the one place a stencil's on-disk path and its Go identifier are both named.
Each new stencil needs a `//go:embed` directive, an exported `[]byte` var with a doc comment, and a registry entry.
`contracts/stencils/registry_test.go` enforces registry completeness.
`//go:embed` reaches only at or below its own directory, which is why all the vars live in that one file.

**Gotchas discovered while exploring:**

- `treadleengine.ParseJudgeVerdict` is *not* callable from outside its package: its `framing judgeFraming` parameter uses an unexported type.
  `perchengine`'s doc records this explicitly.
  Do not plan around reusing it.
- `shuttleengine.Spec.OutputFiles` entries *must not already exist* when the run starts — `validate` rejects a pre-existing entry.
  This is why archiving is mandatory rather than nice-to-have.
- `Spec.validate` resolves relative `OutputFiles` against a worktree root the adapter must not read, which is why `SingleLLMProducer` rejects a relative entry outright rather than resolving it.
  The Bouncer builds its own absolute paths from `RunDir`, so this is naturally satisfied, but the constructor should still reject a relative `RunDir`.
- `internal/loomengine` currently has no producer list and no `Discussion-Review` stub in Go — the `loom` wiring genuinely does not exist yet, so there is nothing in that package for this task to touch.
- `internal/shedadapters` has no seam-enforcement test of its own (the invariant is on `internal/shedengine`), so there is no import allowlist to extend here.
  It already imports `logger`, `shuttleengine`, `perchengine`, and `websterengine`; adding `stencil` and `stencilstore` breaks nothing.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Producer-Seam Invariant** — `internal/shedengine` production code imports only stdlib, `internal/state`, and `internal/lock`.
  This task adds no import to `shedengine` and must not.
  Producers adapt onto the seam in their own packages, which is exactly what this task does.
  Enforced by `internal/shedengine/seam_enforcement_test.go`.
- **Treadle Runner-Seam Invariant** — relevant here only in the negative direction: it restricts `treadleengine`'s own imports, not who imports its exports.
  The decision not to import `treadleengine` is a design choice made on other grounds, not an invariant requirement.
- **Stencil Ownership Invariant** — every producer prompt is read at call time from a told, absolute stencils directory, never from embedded bytes.
  `//go:embed` in `contracts/stencils` carries seed defaults only and is never a live read path.
  `internal/stencilstore` is the sole owner of seeding, hash-stamping, edit detection, reading, and validation, and takes a fully resolved absolute base directory from its caller.
  A file whose body hash does not match its stamp is never overwritten.
  The seed/refresh pass runs once per process at `cmd/lyx`'s root pre-run, never lazily inside `stencilstore.Read`.
  → The Bouncer is *told* `StencilsDir`; it never derives one, and it never reads `contracts/stencils` embedded vars at runtime.
- **Told-Geometry Invariant** and **Cwd Resolution Invariant** — `internal/lyxcwd` alone owns cwd resolution.
  The Bouncer calls no `lyxcwd`, no `os.Getwd`, no `filepath.Abs`, and never writes the literals `_lyx` or `.lyx`.
  Every path it constructs is joined onto the told `RunDir`.
- **Producer Pointer-Rule Invariant** — an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it.
  → The two new stencil templates point at the verdict/ledger/focus format contracts rather than restating them, so one edit changes both producer and consumers.
  Review obligation, not machine-checked.
- **Review Round Invariant** — binds the burler/hardener round, not this producer.
  Noted so the plan does not mistakenly apply A-before-B round discipline to the Bouncer itself; the Bouncer is the judge, not a round.
- **Config Strictness Invariant** — the three file contracts here are agent-written artifacts, not config files, so they follow the yaml-strictness-split's *lenient* side (no `KnownFields`), matching `reviewHeader` and `judgeHeader`.
  Do not apply strict decoding.
- **Live-Substrate Spawn Observability** — disposition, not a check to defer: the Bouncer starts no OS process itself.
  The spawn and teardown `logger.Info` lines belong to `internal/shuttleengine/run.go`, which `CONSTRAINTS.md` already lists as an instrumented call site, so the Bouncer adds no spawn/teardown logging — the same disposition all three sibling adapters reached.
  What *does* bind the Bouncer is the invariant's last bullet: a live-substrate module's operational messages all belong on `internal/logger`, never on stdlib `log`, because a retry, an unreadable probe, or a skipped cleanup is exactly what an operator goes looking for afterwards and `log.Printf` reaches neither the durable trace file nor a trace correlation id.
  Every fail-safe degradation, the re-bounce, and the seed-call fallback therefore log via `logger.Warn` with the producer name, the round, and the cause — which is what this design already specifies throughout.
- **Documentation Lifecycle** and the repo's `CLAUDE.md` task-completion rule — a task introducing cross-cutting infrastructure updates its docs in the same commit.
  → `internal/shedadapters/doc.go` and `manifest/designs/shed.md` land with the code.
  `manifest/roadmap.md` moves because this is a planned item completing.
  No new cross-cutting invariant is expected, so `CONSTRAINTS.md` is likely untouched — but if the plan discovers one, it is recorded there in the same commit.
- **Markdown Link Integrity** — any new markdown link in `manifest/` or `docs/` is checked by `internal/lyxcwd/docslink_test.go`.
- **Markdown style** (`CLAUDE.md`) — semantic line breaks, one sentence per line, never a fixed-column hard wrap.
  Applies to the two new stencil templates and every doc touched.

Discovered during discussion:

- The Bouncer must never import `internal/burlerengine`, `internal/loomengine`, or `internal/treadleengine`.
  The first two would destroy the domain-agnosticism the roadmap requires for `Tenter` reuse; the third points at a package this initiative exists to make retirable.
- `Done` must never be reachable from a degraded path.
  Every fail-safe fallback resolves to `Stuck`.

## Testing

TDD candidates — write these before the implementation, in this order:

1. **The three parsers.**
   Pure functions over `[]byte`, no filesystem, no spawn.
   Table-driven, one case per rule, each asserting a specific error rather than merely non-nil.
   Cover, for each file type: missing opening `---`; missing closing `---`; empty frontmatter; invalid YAML; prose correctly extracted and CRLF-normalised; unknown extra keys tolerated.
   Verdict-specific: each legal spelling accepted; a wrong-case spelling rejected; an unknown verdict rejected; empty or whitespace-only rationale rejected.
   Ledger-specific: empty ledger list legal; empty `key` rejected; empty `rounds` rejected; non-positive round in `rounds` rejected; a status outside `open`/`resolved` rejected; non-positive `round` rejected.
   Also assert the *absence* of carry-forward enforcement — a ledger that drops a key present in the previous ledger parses cleanly — so the known soft spot recorded under "The three file contracts" is visible in the test record rather than merely unmentioned.
   Focus-specific: empty `exclude_lenses` and empty `focus` both legal; non-positive `round` rejected; a non-list where a list is required rejected.
2. **The focus-file writer, round-tripped through the focus parser.**
   Property-shaped: what the writer emits, the parser accepts and yields back unchanged.
   Include the empty-exclusions seed shape explicitly, since that is the seed call's fallback output and the round producer's round-1 input.
3. **The exported `ResolveRound` helper.**
   Filesystem-only, no spawn.
   Cover: empty run dir → `0`, nil error; only round 1's report present → `1`; rounds 1–3 present → `3`; a gap in the sequence (1 and 3 present, 2 absent) → `1`, asserting explicitly that round 3 is *not* returned, since the contiguity rule is what both halves of a segment depend on agreeing about; a run dir that does not exist → an error, not a silent `0`.
   Also assert both derived readings against one disk state in the same test — "round to judge" is the return value and "round to write" is the return value plus one — so the two consumers' contract is exercised, not just the scan.
4. **`Call`, against a fake `Shuttle`.**
   The fake records the `Spec` it received and returns a scripted `Result`, with a hook to write the output files the real agent would have written.
   Assert on the recorded `Spec` — that `OutputFiles` are absolute, that the filled prompt contains the rubric text, and that `Role`/`Round` are set — not only on the returned outcome.

Scenarios `Call` must cover:

- **Seed call, happy path** — empty run dir; returns `Stuck`, empty pointer, nil error; `round-1-focus.md` exists and parses; exactly one spawn against the *seed* template; no verdict or ledger file written.
- **Seed call, spawn fails** — every failure mode in turn (stencil read, fill, `Run` error, non-`done` outcome, agent wrote nothing, agent wrote an unparseable focus file); each returns `Stuck` with nil error, and `round-1-focus.md` exists with `round: 1` and both lists empty.
- **Judge call, `APPROVED`** — returns `Done` with the ledger path as the pointer; all three files written, `round-<N+1>-focus.md` included and carrying empty `exclude_lenses`/`focus`.
- **Judge call, `BLOCKING`** — returns `Stuck` with the ledger path as the pointer; verdict, ledger, and `round-<N+1>-focus.md` all written.
- **Judge call, every degradation path** — returns `Stuck`, nil error, a `Warn` logged.
  Enumerate them individually rather than as one case: stencil read failure, rubric stencil read failure, fill failure, `Run` error, each non-`done` shuttle outcome, unreadable verdict file, unparseable verdict file, unparseable ledger file.
  Assert explicitly in each that the outcome is not `Done` — this is the one property that must never regress.
- **Re-bounce, post-seed** — no report exists but `round-1-focus.md` does: returns `Stuck` with an **empty** pointer, nil error, a `Warn` logged, and the fake `Shuttle` never called; assert `round-1-focus.md` is left byte-identical, not archived and not re-seeded.
- **Re-bounce, post-judgment** — `judged(N)` holds: returns `Stuck` with `round-<N>-bouncer-ledger.md` as the pointer, nil error, and a `Warn` logged; assert the fake `Shuttle` was **never called**, since spending an LLM session here is the whole failure this mode exists to prevent.
  Also assert the existing verdict and ledger files are left untouched — not archived, not rewritten.
- **`judged(N)` is not satisfied by debris** — three cases, each of which must produce a *judge call* (fake `Shuttle` invoked) rather than a re-bounce: verdict present, ledger absent; ledger present, verdict absent; both present but the verdict unparseable.
  Assert in each that the debris is archived on the way in, so the spawn's `OutputFiles` are clear and `Spec.validate` does not reject them.
- **Unconditional `OutputFiles`** — assert the recorded judge `Spec.OutputFiles` has exactly three entries on *both* an `APPROVED` and a `BLOCKING` round, and that `round-<N+1>-focus.md` exists after an `APPROVED` round too.
  This is the guard against the conditional-output-file regression that would make `Done` unreachable.
- **Pointer discipline** — one case per outcome asserting the exists-or-empty rule: `Done` and `BLOCKING`-`Stuck` name a ledger that exists on disk; the seed call and every degraded judge path report an empty pointer; the re-bounce names a ledger that exists.
- **Previous ledger handling** — a valid prior ledger's path reaches the prompt; a *malformed* prior ledger degrades to the no-prior-ledger marker with a `Warn`, and the judge still runs; the no-prior-ledger case fills the "(none)" literal rather than an empty string.
- **Stale outputs** — a pre-existing verdict/ledger/focus file at a target path is archived before the spawn, and the spawn then succeeds rather than tripping `Spec.validate`.
- **Cancellation** — an already-cancelled context at entry returns an error with nothing started (assert the fake `Shuttle` was never called); a context cancelled during the run returns an error on every path *except* a genuinely parsed verdict, which returns its mapped outcome and pointer regardless.
- **Constructor validation** — one case per rule in the `BouncerConfig` table: empty `Name`; empty and relative `RunDir`; nil `ReportName`; empty and relative `StencilsDir`; empty `RubricStencil`; nil `Shuttle`.
  Each returns an error naming the offending field.
  Empty `Model` and empty `Effort` are *accepted* (they defer to the provider default) — assert that explicitly, so a later tightening does not silently break the default-model path.
  A nil `Now` defaults to `time.Now`.

Also required: a case in `contracts/stencils/registry_test.go` covering the two new stencils, so registry completeness holds.

Not in scope for tests: any real spawn or smoke test.
The other three adapters have none, and adding one here would cross the Test Tier Purity Invariant's boundary for no coverage the fake does not already give.

## Q&A log

- **Q:** Where does the Bouncer live — `internal/shedadapters`, or a new engine package with a thin adapter? **A:** [auto-pick] `internal/shedadapters/bouncer.go`. **Why:** it is mechanically one shuttle spawn per `Call`, reuses this package's existing shared helpers, and the task slug already pins the placement; a separate engine buys a second seam with nothing to protect behind it.
- **Q:** How does the Bouncer find the current round's report artifact? **A:** [auto-pick] told an absolute `RunDir` plus a `func(round int) string` report-name convention, scanning disk each `Call`. **Why:** "told, never derived" is the package rule, resolving from disk survives restart, and passing the naming convention as a function keeps the Bouncer ignorant of burler's file spelling.
- **Q:** Reuse `treadleengine`'s exported parsers, or write fresh ones? **A:** [auto-pick] fresh, in-package, no `treadleengine` import. **Why:** `ParseJudgeVerdict` is not callable externally at all (unexported `framing` type), `ParseHandoff`'s `covers_rounds` vocabulary is specific to a nested round loop this design deletes, and importing the package this initiative aims to retire points the dependency the wrong way.
- **Q:** What shape is the structured focus file? **A:** [auto-pick] YAML frontmatter (`round`, `exclude_lenses`, `focus`) over optional prose, with both lists always present and possibly empty. **Why:** the roadmap requires it be mechanically parseable rather than prose, and always-present lists let the seed call emit the identical shape so the round producer has one format to read.
- **Q:** How is the judge LLM invoked? **A:** [auto-pick] a package-local `Shuttle` seam over `shuttleengine.Spec`, prompt via `stencilstore.Read` + `stencil.Fill`. **Why:** identical to `treadleengine/judge.go` and `singlellm.go`, and it keeps every test fake-driven.
- **Q:** What happens on judge-call infrastructure failure? **A:** [auto-pick] fail-safe toward another round — `Warn` plus `Stuck`, never a hard error, never `Done`. **Why:** a false stuck costs bounded extra rounds while a false pass ships an unreviewed artifact, and `ProducerDef.MaxBounces` now bounds the cost that treadle's hard cap used to bound.
- **Q:** Does the Bouncer carry its own round-cap ladder? **A:** [auto-pick] no — `MaxBounces` is the whole budget. **Why:** the previous roadmap item moved exactly this into `Shed`; re-adding it would restore the nested loop and create two disagreeing termination authorities.
- **Q:** Does the seed call spawn an LLM, or write the empty focus file mechanically? **A:** [auto-pick] spawn, with a mechanical empty-exclusions fallback on failure. **Why:** the rubric is required to cover seed-time focus-setting, which is meaningless unless an agent reads it; the fallback exists so a flaky focus call never blocks round 1.
- **Q:** Archive or delete stale output files before a spawn? **A:** [auto-pick] archive, via the existing `archiveStaleOutputs` helper. **Why:** `Spec.validate` rejects pre-existing `OutputFiles` outright, so a crash-resume would hard-fail without this, and the partial artifact is what an operator needs to diagnose the crash.
- **Q:** What does `OutputPointer` carry, and is a cancellation bridge installed? **A:** [auto-pick] the ledger path on both `Done` and `Stuck` from a judge call, empty from a seed call; entry/exit cancellation only, no mid-run bridge. **Why:** the ledger is a real cross-round artifact, unlike perch's re-derived verdict, so hiding it on `Stuck` would hide it exactly when it matters — a deliberate delta from `PerchProducer` that the package doc must state so it is not read as drift.
- **Q:** Does this task wire an instance into `loom`? **A:** [auto-pick] no — producer, parsers, stencils, tests, docs only. **Why:** the three `loom` review-producer items are separate tasks and are blocked on the Burler-round producer regardless.
- **Q:** One stencil template or two? **A:** [auto-pick] two — `bouncer-template-seed.md` and `bouncer-template-judge.md`, each with a `rubric` marker. **Why:** one prompt trying to be both a focus-setter and a judge reads badly in the two places clarity matters most, and a rubric must be able to address the two passes differently.
- **Q:** How is the per-instance rubric supplied? **A:** [auto-pick] as a stencil *name*, read via `stencilstore.Read` from the told `StencilsDir` at call time. **Why:** the Stencil Ownership Invariant forbids embedded bytes as a live read path; taking rubric bytes in the constructor would bypass hash stamping and edit detection.
- **Q:** Is a real-spawn smoke test needed? **A:** [auto-pick] no — fake-`Shuttle` table tests only. **Why:** the other three adapters have none, and a real spawn adds no coverage the fake does not already give while crossing a test-tier boundary.
- **Q:** Is the round-resolution scan exported? **A:** [auto-pick] yes, exported from `internal/shedadapters`. **Why:** the Burler-round producer must resolve the same round from the same convention or the two halves of a segment silently disagree; exporting it in the task that defines the convention is what prevents the later task from duplicating and drifting.
- **Q:** [review r1] Does the seed call's unconditional `Stuck` consume a bounce, and if so, is that compensated for? **A:** [auto-pick] yes it consumes one, and it is documented rather than compensated: `MaxBounces: N` yields `N-1` judged rounds. **Why:** `episodeStuckCount` never resets within a segment episode because the Bouncer's only `Done` exits the segment; silently adding one inside the Bouncer would make `MaxBounces` mean something different for this producer than for every other one in the list, which is worse than a documented off-by-one.
- **Q:** [review r1] What exactly does the exported round-resolution helper return, and what happens on a gap in the report sequence? **A:** [auto-pick] pinned in this discussion — `ResolveRound(runDir, reportName) (int, error)`, contiguous scan from round 1, `0` for "nothing run yet", error only on an unreadable run dir; a gap stops the scan, so reports 1 and 3 with 2 absent returns `1`. **Why:** "round to judge" and "round to write" are both plausible readings of a bare helper and differ by one on identical disk state, so leaving the return shape unpinned would let the two consumers drift while sharing the same code; contiguity is what the segment produces by construction, and stopping at the gap fails safe by re-judging rather than skipping.
- **Q:** [review r2] What is the judge spawn's `OutputFiles` list, given the focus file was described as conditional? **A:** [auto-pick] exactly three entries, unconditionally, with the judge writing all three on every call including `APPROVED`. **Why:** `shuttleengine` classifies a run `done` only when every `OutputFiles` entry exists (`wait.go:222,279`), so a conditionally-written third entry would make every approval classify non-`done`, degrade to `Stuck`, and render `Done` unreachable — the segment could never approve anything.
- **Q:** [review r2] Does `MaxBounces: N` yield `N` or `N-1` judged rounds? **A:** [auto-pick] `N` — the `N`th judged round blocks the run if it comes back `BLOCKING`. **Why:** the budget check at `run.go:197` counts history read *before* the current `Stuck` is appended (deliberately, per its own comment), so `Stuck` number `N+1` blocks; with the seed as `Stuck` #1 that leaves exactly `N` rounds judged. The earlier `N-1` claim was derived from a post-append reading and was wrong.
- **Q:** [review r2] What stops the seed call from re-seeding when the round producer bounces back at round 1 without writing a report? **A:** [auto-pick] the seed discriminator also consults `round-1-focus.md`; present means a re-bounce with an empty pointer, not a second seed spawn. **Why:** the roadmap's unconditional-`Stuck` rule makes this reachable, and without the guard the seed mode reproduces exactly the paid-re-spawn loop the re-bounce mode was introduced to prevent.
- **Q:** [review r2] Is a round with a verdict file but no ledger "already judged"? **A:** [auto-pick] no — `judged(N)` requires verdict present, ledger present, *and* the verdict parsing; anything less is debris from an unfinished judge call and gets re-judged. **Why:** a spawn that wrote the verdict but not the ledger classifies non-`done` and degrades, so verdict-presence alone would accept a round no judge finished assessing and then report a pointer to a ledger that does not exist — which `Shed` never stats and therefore never catches.
- **Q:** [review r2] What is the constructor's full parameter set, and where do `Model`/`Effort` come from? **A:** [auto-pick] a validated `BouncerConfig` struct through `NewBouncer(cfg) (*Bouncer, error)`, with `Model`/`Effort` told as an already-resolved pair by the caller. **Why:** nine inputs including two that must be absolute paths is past the point where positional arguments are safe, and validating at first `Call` would turn a wiring typo into a mid-run engine-level failure; keeping model resolution in the product layer follows the rule that engines beneath `loom` stay phase-agnostic.
- **Q:** [review r1] What happens when the round producer bounces back without writing a new report, leaving the highest report already judged? **A:** [auto-pick] a third mode — re-bounce: spawn nothing, log a `Warn`, return `Stuck` with the existing ledger as pointer. **Why:** the roadmap pins the round producer to return `Stuck` unconditionally including on its own degraded paths, so this is reachable in normal operation; a report-file-only discriminator would re-judge a settled round, archive its verdict, and burn the entire bounce budget on paid LLM calls that decide nothing.
