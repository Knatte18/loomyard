# Discussion: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
task: 'shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions'
slug: shed-producer-typology-sweep
status: discussing
parent: main
```

## Problem

The Planned `Shed` roadmap item states that a producer is always **atomic** — one mechanical action or one LLM session, never an internal multi-step process of its own.
`Webster` and the three `perch`-gated review producers plainly violate that as written: each owns an internal loop (Webster's per-batch fork loop, perch's round loop).
The scoping task deliberately left this open as **Question 1**, gated as a named precondition on `manifest/roadmap.md`'s Planned `Shed` item ("must be decided before `Shed` is built, not during").

A design conversation on 2026-08-11 resolved it, and the resolution is already recorded on the roadmap item (`manifest/roadmap.md:57–61`, commit `6b396aa1`).
**Why now:** the resolution lives only on the roadmap. `shed.md` and `loom.md` — the authoritative design docs `Shed`'s build task will actually be written from — still carry the unqualified atomicity claim and still read as an unresolved tension.
The precondition is not discharged until it lands in the docs that describe the mechanism.

The same conversation also established that the superseded `shed-model-contradiction-sweep` task (task E of the six-task `shed-followups.md` chain) was removed from the wiki without running.
`shed-followups.md:536–540` names E as the **final owner** of `loom.md`, `manifest/roadmap.md`, and `docs/overview.md`.
Nobody else is coming: a `list_tasks_full` sweep of the wiki confirms no task holds E's residue.
So this task is E's successor, not merely the atomicity carve-out.

## Scope

**In:**

- `manifest/designs/shed.md` — the producer-typology carve-out (the authoritative, full statement), the thin-Input carve-out, the two-axes cross-reference in the engine-adapter section, plus E's own `shed.md` residue (`:7`, `:18`, `:19`, `:63`).
- `manifest/designs/loom.md` — a new `Kind` column on the producer table plus a pointer to `shed.md`'s carve-out from the atomicity sentence, and E's `loom.md` residue (`:15–17`, `:57` row 9, `:76–83`).
- `CONSTRAINTS.md` — a new, short `## Producer Pointer-Rule Invariant`.
- `docs/overview.md` — the `shed`/`glance` name-disambiguation note at `:300`, and the stale phase chain at `:283`.
- `manifest/designs/hardener.md` — `:17`'s "producer-slot".
- `manifest/roadmap.md` — `:110`'s "deferred phase slot between Webster and Finalize", `:54`'s six-task breakdown line (which still names task E as a pending final owner), and the phase-enum deferral record E was to write; verify-only on `:57–61`.
- `manifest/designs/shed-followups.md` — one supersession block at the head of section E.

**Out:**

- **Rewriting `Discussion-Write`/`Plan-Write` as instances of the shared "LLM-Producer" type.**
  Deleting `internal/loomengine/discussion.go`, `plan.go`, `discussion-template.md`, `plan-template.md` and their tests, and rebuilding on the shared type, is real follow-up work this decision enables — but the shared type is a `Shed`-level construct that does not exist yet.
  It cannot be scoped before `Shed` is built.
  File it as its own task once `Shed` lands.
  This task names the shared type only as a **candidate**, never as a decided design.
- **Any Go source change.**
  This task is docs-only. No package is created, renamed, or deleted.
- **Editing the `phase` enum.** `internal/loomengine/coherence.go:14–22`'s `validPhases` map and `docs/reference/status-schema.md`'s twin stay untouched, per `shed-followups.md:529–532` — the flat producer list replaces the enum rather than editing it, and that realignment lands with the `Shed` build task.
  **Writing the deferral *record* is in scope, though** — see `carry-forward-e-phase-enum-record` below. `shed-followups.md:529–532` gives E two obligations, and only the first is a non-goal here.
- **A machine-checked guard for the pointer rule.** `shed-followups.md:499` explicitly scopes it as review-enforced.
- **`manifest/designs/finalize.md`, `raddle.md`, `self-report.md`.** Task D (`ab3d67b1`) owns these and has landed; this task does not reopen them.
- **`docs/reference/discussion-format.md` and `plan-format.md`.** Task C (`2186ff53`) owns these and has landed.

## Decisions

### inherit-task-e-residue

- **Decision:** this task is the outright successor to the removed `shed-model-contradiction-sweep` (task E), inheriting its full remaining scope and all three of its ownership positions (`loom.md`'s final owner, `roadmap.md`'s last owner, `docs/overview.md`'s last owner) — not just the four items named in this task's own wiki brief.
- **Rationale:** `shed-followups.md:536–540` names E as final owner of three files; E was removed from the wiki without running (verified: `list_tasks_full` shows no task holding it, and `git log` shows A/B/C/D/F landed as `0149776a`/`80238b3f`/`2186ff53`/`ab3d67b1`/`e179ad0c` with no E commit).
  Leaving the residue orphaned means `shed.md` permanently carries dangling back-references and a "by value" claim that `finalize.md` and `shed-followups.md:381` both contradict.
  The marginal cost is small: this task already re-reads `shed.md` and `loom.md` end to end to land the carve-out, the residue is eight sites, and the whole thing stays docs-only and one commit.
- **Rejected:** brief-only scope with a new `shed-doc-residue` follow-up task — recreates the exact orphaning this decision fixes, and a fourth doc-sweep task on the same three files invites another shared-file collision.
  Brief-plus-contradicting-residue-only — an arbitrary line through a set of fixes that all cost the same.

### shed-md-is-authoritative-loom-md-points

- **Decision:** the full producer-typology text lives in `shed.md`'s producer-contract section (`## Producer contract vs. producer definition`, currently `:22–29`).
  `loom.md` gets short pointers only — it cites `shed.md`'s carve-out by anchor rather than restating it.
- **Concrete shape in `loom.md`'s producer table.** The existing **Type** column is left alone: it holds engine-type values (`mechanical`, `LLM`, `LLM/perch`, `black box …`) and stays on that axis.
  Add **one new `Kind` column** whose only values are `simple` and `bespoke`.
  Column order becomes `# | Producer | Kind | Type | Input | Output`.
  Rows 4, 8, 10, 11 **and 12** (`Discussion-Review`, `Plan-Review`, `Webster`, `Webster-Review`, `Finalize`) are `bespoke`; the other seven are `simple`. See `finalize-is-bespoke` below for row 12, the one classification that is not carry-forward.
  **The anchor pointer into `shed.md` appears exactly once**, in the sentence introducing the table — not repeated per row, and not inside any cell.
  Cells carry the bare word only.
- **Rationale for a separate column rather than augmenting `Type`:** merging both axes into one cell (e.g. `LLM/perch — bespoke`) is precisely the conflation `two-axes-cross-reference` exists to prevent, and it would make the engine axis unreadable at a glance.
  A single anchor above the table also keeps the doc from carrying twelve identical links, which would itself read as a pointer-rule violation.
- **Rationale:** this is the authority split the two docs already declare. `shed.md:3` calls itself "the authoritative description of `Shed`'s own generic mechanism"; `loom.md:43` defers to `shed.md` for the mechanism and owns only `loom`'s concrete list.
  The typology is a property of `Shed`'s generic contract, not of `loom`'s particular producers.
  It is also the pointer rule applied to the docs themselves — the same discipline this task is adding to `CONSTRAINTS.md`.
- **Rejected:** restating in both — two copies that drift, and a direct violation of the rule being codified in the same commit.
  Full text in `loom.md` with a pointer from `shed.md` — inverts the established authority split.

### producer-typology-carve-out

- **Decision:** producers split into two kinds, and the atomicity rule is scoped to the first:
  - a **simple, single-agent-spawn producer** — one mechanical action or one LLM session. `Discussion-Write` and `Plan-Write` are the current LLM examples; the mechanical ones are the five gate/step producers — `Preflight`, `Discussion-Validate`, `Plan-Sweep`, `Plan-Validate` and `Batchifier`.
    **`Finalize` is deliberately not in this list** — see `finalize-is-bespoke` below. This bullet lands close to verbatim in `shed.md`'s contract section, so the omission has to be right here rather than corrected downstream.
    The LLM examples are **candidates** for a shared `Shed`-level "LLM-Producer" type (input file(s) + optional input-format pointer, internal instruction files, output file(s) + optional output-format pointer, plus a log).
    This kind does **not** typically need its own crash-recovery — re-running one spawn from scratch is cheap.
  - a **bespoke, multi-spawn producer** — owns its own internal loop (many LLM spawns, or an agent orchestrating sub-agents). `Webster` (per-batch fork loop) and the three `perch`-gated review producers (`Discussion-Review`, `Plan-Review`, `Webster-Review` — perch's own round loop, now `internal/treadleengine`, spawning a fresh burler round per iteration plus ephemeral judge/triage calls) are the current examples.
    These are **exempt from the atomicity rule by design, not in violation of it.**
- `Shed`'s own contract stays exactly two parts, Input and Output pointers.
  Its resume/crash-recovery/pause guarantee operates at **producer granularity only** — it re-drives a crashed producer from its last recorded pointer, never mid-producer.
- A bespoke multi-spawn producer that would lose expensive internal progress on a crash needs its **own** internal crash-recovery, a capability `Shed` does not provide.
  Both current examples already ship it: `internal/websterengine` re-drives the first unreported batch from `state.json` (see its `doc.go`'s "crash/resume" section), and `perchengine`'s round loop (now `internal/treadleengine`) keeps its own resumable run-dir state under an OS advisory lock, released automatically if the holding process dies.
- **Rationale:** decided 2026-08-11 and already recorded verbatim on `manifest/roadmap.md:57–61`.
  This task **carries it into the design docs; it does not re-derive or re-open it.**
  The roadmap wording is the source — prefer it over any paraphrase.
- **Rejected:** decomposing `Webster` into flat producers the way `Plan` was decomposed — this was the live alternative in the original Question 1 and was rejected on 2026-08-11.
  Do not re-litigate it.

### two-axes-cross-reference

- **Decision:** `shed.md`'s existing engine-adapter section (`:31–46`) keeps its four-way split (mechanical Go-function / single-LLM-spawn / `perch` / `Webster`) unchanged.
  Add one sentence noting it cuts on **engine type** — which code drives the producer, and therefore how many adapters must be built — whereas the simple/bespoke typology cuts on **atomicity and crash-recovery ownership**.
  The two axes align on `Webster` and `perch` but not elsewhere: `Discussion-Validate` is mechanical *and* simple, while one `perch` adapter serves three separate bespoke producers.
- **`Finalize` is the sharpest non-alignment case, and the cross-reference sentence must name it.** It is **`bespoke` on the typology axis** (per `finalize-is-bespoke`) and **still adapter-free on the engine axis** — `shed.md:41` keeps listing it among the mechanical Go-function producers that need no translation adapter, and that stays correct.
  Why both hold at once: `Shed` drives `Finalize` by calling a plain Go function that already satisfies the `ProducerRunner` seam. The conflict-path LLM spawn and raddle's leaf forks happen *inside* that function, through the existing `shuttle` layer, and are invisible to `Shed` — which is exactly what "bespoke" means (the producer owns its own internals, including recovery) rather than something that changes who drives it.
  Naming this explicitly is the whole value of the cross-reference: `Finalize` is the one row where a reader would otherwise conclude the two axes contradict each other, and the plan writer should not have to derive the answer.
- **Consequence for `shed.md:39–46`:** the "two new adapters, not eleven" count is **unchanged** — `Finalize`'s reclassification adds no third adapter.
- **Rationale:** the adapter section exists to make one argument — "two new adapters, not eleven" (`:46`).
  Restructuring it around the typology would lose that argument.
  But leaving two overlapping partitions in one doc with no acknowledgement invites a reader to conflate them.
- **Rejected:** merging into one taxonomy — destroys the section's point.
  Saying nothing — leaves the overlap for every future reader to work out unaided.

### finalize-is-bespoke

- **Decision:** `Finalize` is classified **bespoke**, not simple.
- **This is the one `Kind` value that is not carry-forward.** `roadmap.md:58` names `Discussion-Write`/`Plan-Write` as simple and `Webster`/the three reviews as bespoke, and classifies `Finalize` **neither way**.
  Adding the `Kind` column forces the call, so this task makes it and argues it rather than letting it fall out of a default.
- **Rationale — `Finalize` owns an internal multi-spawn process, post-task-D:**
  - `finalize.md:36–37` (landed by task D, `ab3d67b1`) puts raddle-regeneration *inside* the merge's critical section: **parallel leaf forks** plus a serial `Overview.md` step, then a `SyncWeft` commit.
  - `finalize.md:37` requires the merge lock to span that whole critical section as one atomic unit — "never released and re-acquired partway through".
    That is an explicit internal-atomicity obligation, which is exactly what the carve-out says `Shed` does **not** provide at sub-producer granularity.
  - `finalize.md:9` spawns a fresh, higher-capability LLM in a clean session on merge conflict.
  - `loom.md:60` already hedges the row as "mechanical (**mostly**)" — the hedge is this classification, unnamed.
- **The happy path is genuinely pure Go with zero LLM spawns** (`finalize.md:8`), so `Finalize` looks simple most of the time.
  Classify on the worst case regardless: the axis exists to say who owns crash-recovery, and a producer whose recovery obligation appears only on the unhappy path still owns it.
- **Record, do not design, the gap this surfaces.** `Webster` and the `perch`-gated reviews both already ship their own internal crash-recovery (`websterengine`'s `state.json` re-drive; `treadleengine`'s locked run-dir). `Finalize` does **not**.
  A crash inside its locked critical section is therefore unrecovered today.
  State this in `shed.md` as an observation for the `Shed` build task and note that `finalize.md:39` already records "an alternative giving Raddle its own `Shed` producer, with merge-in and locking lifted into `Shed` itself" as a live candidate for a future task.
  **Do not design that recovery here** — it is `Finalize`'s own contract, task D's territory, and outside this task's docs-only remit.
- **Rejected:** classifying `simple` because the happy path is one mechanical action — makes the `Kind` column describe the common case rather than the contract, and would assert `Shed` covers a recovery obligation it does not.
  Leaving row 12 blank or `simple/bespoke` — a table column that declines to answer on the one row where the answer is non-obvious is worse than no column.

### thin-input-carve-out

- **Decision:** resolve `shed-followups.md`'s **Question 2** with a symmetric thin-**Input** carve-out, recorded in `shed.md`'s producer-contract section immediately beside the thin-Output carve-out it mirrors.
  The Input contract permits **no Input at all** for a chain-head producer, because its input is human intent expressed in an interactive session, not an artifact with a format contract. `Discussion-Write` is the current and only example.
- **Explicitly reject** the framing carried in this task's own wiki body — that the task record is the Input, making the pointer target "the wiki task record rather than a format-contract file, a different kind of pointer than every other row".
  That is a mill-ism that does not transfer: `loom` has no wiki and no task record. `loom.md:114–119` already describes Discussion input as an inherently interactive human boundary `lyx run` yields at.
  Admitting a second kind of pointer target would weaken the pointer rule for a target that does not exist in `lyx`.
- **State the resume consequence honestly**, mirroring the thin-Output carve-out's own reasoning: a producer with no Input has nothing to re-read on resume, so a crashed `Discussion-Write` re-runs from its own partial output plus fresh human input — which is correct, since the human is present at that boundary by definition.
- **Rationale:** the thin-Output carve-out is already decided (`shed-followups.md:487`); the Input side is the symmetric case and closing both together is what makes the two-part contract coherent.
- **Rejected:** declaring a second pointer-target kind — introduces a contract concept with zero instances in `lyx`.
  Leaving Question 2 open — it is a contract-wording decision with an obvious answer, and re-gating it would block `Shed` on nothing.

### resolve-thin-output-over-four-producers

- **Decision:** also discharge E's **Part five** thin-Output obligation, in the same `shed.md` section — but state it as **two cases, not one**, because the four producers do not share a single story:
  - **The three gate producers** — `Preflight`, `Discussion-Validate`, `Plan-Validate` — genuinely emit nothing at all. The Output contract permits a bare pass/fail gate signal with no artifact, and the resume-on-output-files rule degrades gracefully: a producer with no artifact simply re-runs on resume, which is correct for all three because each is a cheap, idempotent re-check.
  - **`Finalize` is a different case and must not be folded into that sentence.** `loom.md:60`'s Output cell reads "merge-back, PR", and `finalize.md` describes a real `SyncWeft` commit plus an optional PR — so it plainly *does* have effects.
    What it has no instance of is a **contract-level output artifact**: nothing downstream consumes its output through a format pointer, because it is the terminal producer in the list.
    Its thin Output is therefore "no *pointer target*", not "no effect".
    And its resume story is **not** the graceful degradation above — a partially-completed merge is not a cheap idempotent re-run. That recovery is `Finalize`'s own obligation, per `finalize-is-bespoke`, and is explicitly not designed here.
- **Rationale for splitting:** the original single sentence claimed all four "genuinely have no output artifact", which is false for `Finalize` in the plainest reading and would have written a wrong claim into `shed.md`'s contract section.
- **Rationale:** `loom.md:78–82` currently states this question is open and explicitly hands it to task E over **four** producers (widened from two by task C's insertion of `Discussion-Validate`).
  Landing the Input carve-out while leaving its Output twin flagged open would be incoherent — they are one contract section.
- **Rejected:** leaving thin-Output open — `loom.md:79–82`'s hand-off note names task E, which no longer exists, so it would dangle exactly like the other stale-owner claims this task retires.

### roadmap-terminology-verbatim

- **Decision:** use `manifest/roadmap.md:58`'s exact terms — "simple, single-agent-spawn producer" and "bespoke, multi-spawn producer" — and name the shared type as a **candidate** "LLM-Producer", never as decided.
- **Rationale:** the roadmap is the recorded source of the resolution; introducing a synonym in the design docs makes the two disagree in vocabulary while agreeing in substance.
- **Rejected:** "atomic producer" / "composite producer" — "atomic" is the exact word the carve-out is about, so it re-creates the confusion.
  "leaf producer" / "loop-owning producer" — names the real discriminator more precisely but diverges from the recorded decision.

### pointer-rule-invariant-placement

- **Decision:** add `## Producer Pointer-Rule Invariant` to `CONSTRAINTS.md`, placed **immediately after `## Batcher Registry+Config Invariant`** (currently ending `:353`) and before `## GitHub Auth Invariant`.
  Shape: one invariant statement, one clarifying bullet naming its subject (instruction files and format-contract docs, not Go source), and one `- **Enforced by** review obligation.` line.
- **Rationale:** `shed-followups.md:497–500` requires it short and in the shape of the existing seam entries, explicitly citing Treadle Runner-Seam / Scout Engine-Seam / Shuttle Provider-Seam / Batcher Registry+Config as precedent.
  Batcher Registry+Config is the one other `Shed`/producer-model invariant in the file and is likewise review-only, so the topical adjacency is real.
- **Rejected:** appending at the end before `## Documentation Lifecycle` — newest-last is not a convention this file follows, and it loses the adjacency.
  Folding into `## Documentation Lifecycle` — buries a producer-contract rule under a docs-retention rule.

### overview-disambiguation-at-first-hit

- **Decision:** put the `shed`/`glance` disambiguating note at `docs/overview.md:300` only — the first occurrence in reading order.
  One sentence: the `shed` named there is an abandoned earlier `reed` model/view draft, unrelated to `Shed` the outer phase-FSM (link `manifest/designs/shed.md`).
- **Rationale:** `shed-followups.md:525` states the failure mode precisely — "a reader hitting `:289` first will mis-resolve it". `:329` is 29 lines later in the same section chain and inherits the disambiguation.
  Note the task body's `:289`/`:318` line numbers are **stale**; the live sites are `:300–301` and `:329–331`.
- **Rejected:** noting both sites — redundant for a linear reader; the second occurrence is already inside a bullet that explains the fold-back.
  Renaming the historical draft references — rewrites the record of what those drafts were called.

### mechanical-residue-resolutions

- **Decision:** two residue sites whose "open" framing is already answered elsewhere get rewritten to match the landed model, with no new decision made:
  - `manifest/roadmap.md:110` — the Someday `raddle` item's "deferred phase slot between Webster and Finalize" is false.
    Task D (`ab3d67b1`) decided it: Raddle-regeneration folds into `Finalize`'s own contract, not a separate producer and not a slot.
    Rewrite to state the fold and drop the slot framing.
  - `docs/overview.md:283` — the `loom` module entry prints the stale chain "Preflight → Discussion → Plan → Webster → Raddle → Finalize", which names a `Raddle` phase that no longer exists and predates the flat producer list.
    Rewrite to drop `Raddle` and **point at `loom.md`'s producer table** rather than inlining a chain.
- **Rationale:** E's brief described `roadmap.md:110` as "deciding what fills the deferred slot"; task D already decided it, so this is carry-forward, not design.
  Inlining a producer list at `overview.md:283` is what produced the current drift, so the fix must not recreate the failure mode.
- **Rejected:** inlining the full twelve-producer list at `:283` — self-contained but drifts again on the next producer-list change.
  Fixing `:283` only — leaves `roadmap.md:110` contradicting `finalize.md`.

### carry-forward-e-phase-enum-record

- **Decision:** add the phase-enum deferral record to `manifest/roadmap.md`, on the Planned `Shed` item beside the atomicity resolution at `:57–61`.
  Two or three sentences: `internal/loomengine/coherence.go`'s `validPhases` map and `docs/reference/status-schema.md`'s twin (`preflight | discussion | plan | webster | raddle | finalize | done`) are deliberately left as-is; realigning them lands with the `Shed` build task, because the flat producer list **replaces** the enum rather than editing it, and rewriting it now would invent an interim phase set `Shed` would immediately discard.
  Note that task A already renamed `builder` → `webster` in both, so the enum is not stale in the way it was at scoping time.
- **Rationale:** `shed-followups.md:529–532` gives task E **two** obligations, not one — leave the enum alone, *and* "record this deferral explicitly alongside its roadmap edits, so a later reader finds a decision rather than an oversight".
  A grep confirms `manifest/roadmap.md` carries no such record today (no match for `phase enum`, `validPhases`, or `coherence.go`).
  Inheriting only the leave-it-alone half would deliver exactly the oversight-looking outcome the second half exists to prevent — and this task is the last owner of `roadmap.md`, so there is no later chance to write it.
- **Rejected:** dropping the record — the deferral would then be visible only inside `shed-followups.md`, a file describing a task chain that has finished, rather than on the live roadmap item the `Shed` build task will be written from.
  Putting it in `shed.md` instead — `shed-followups.md:532` says "alongside its roadmap edits", and the roadmap item is where `Shed`'s preconditions are already gathered.

### shed-followups-supersession-block

- **Decision:** add one supersession block at the **head of section E** in `manifest/designs/shed-followups.md`, leaving E's body intact as the historical record.
  It must record: (a) task E was removed from the wiki without running and is superseded by `shed-producer-typology-sweep`; (b) E's **Question 1** was **resolved** on 2026-08-11 (producer typology + atomicity carve-out) rather than surfaced, so E's "not resolved by this task" framing at `:504` no longer holds for it; (c) E's **Question 2** is resolved here per `thin-input-carve-out` above; (d) E's **Question 3** is discharged here per `overview-disambiguation-at-first-hit`; (e) this task discharged all three of E's ownership positions.
- **Rationale:** `shed-followups.md:3–5` declares itself the durable, versioned source of truth for the chain, and tasks A, B and C each amended it with `**Override recorded …**` blocks rather than editing their bodies.
  Leaving section E describing pending work under a slug that no longer exists makes the next reader re-derive everything this discussion just established.
- **Rejected:** renaming section E's heading — breaks the file's convention that bodies are the scoping-time record, amended only by appended blocks.
  Leaving the file untouched — permanently misrepresents the chain's state.

## Technical context

### The five sibling tasks all landed; E did not

| Task | Slug | Commit |
|---|---|---|
| A | `builder-retire` | `0149776a` |
| B | `plan-format-drop-v3-suffix` | `80238b3f` |
| C | `format-docs-name-producers` | `2186ff53` |
| D | `raddle-finalize-fold-and-link-repair` | `ab3d67b1` |
| F | `batcher-standalone-split` | `e179ad0c` |
| E | `shed-model-contradiction-sweep` | **never ran — removed from wiki, superseded by this task** |

The precondition-resolution commit is `6b396aa1` ("roadmap: resolve Shed's Webster-atomicity precondition").

### Exact edit sites

Line numbers are a **starting inventory, not a bound** — `shed.md` and `loom.md` must both be re-read end to end, exactly as `shed-followups.md:491` instructed task E.
Every number below was verified against the tree at branch point `c3af3c9c`.

**`manifest/designs/shed.md`**

- `:8` — "each an atomic mechanical action or LLM session" is the unqualified atomicity claim the carve-out scopes.
- `:22–29` — `## Producer contract vs. producer definition`. This is where the typology, the thin-Input carve-out, and the thin-Output carve-out all land.
- `:24` — the two-part Input/Output contract statement the carve-outs attach to.
- `:25` — the pointer rule in prose; the new `CONSTRAINTS.md` invariant is its short formal twin. Keep both, and make sure they do not disagree.
- `:7` — "superseding 'two swappable slots' **below**" dangles; the referenced text was deleted in commit `256b8262`.
- `:19` — "(resolves the open question the pre-revision text **below** left open)" dangles the same way.
- `:18` — "reference (by *value* — the same producer definition named in both lists)". Becomes **by reference**, per `shed-followups.md:378–383`'s `finalize-shared-by-reference` decision. Note the markdown italics make this **invisible to a plain `by value` grep**.
- `:31–46` — the engine-adapter section that gains the two-axes sentence.
- `:59–63` — `## Why this doc doesn't rewrite loom.md's full detail`. **Disposition: the section stays; only `:63` is retired.**
  `shed-followups.md:492` warns that the section's premise changes once C and E run, and it does shift — but only in the "who finishes the job" sense, not in the section's actual claim.
  Its argument is that `loom.md` keeps its own detail sections (crash recovery, pause, session bootstrap, module decomposition) rather than having them duplicated into `shed.md`, and that division is unchanged by anything in this task.
  Do not rewrite or delete the section.
- `:63` — the one line inside it that must go: it claims wiki task `shed-producer-model-scoping` "is the dedicated pass that reconciles any remaining detail mismatch". That task completed on 2026-08-09 and this task is the last owner, so the claim is stale in the present tense.

**`manifest/designs/loom.md`**

- `:15–17` — the naming note still reads "`loom` = `Shed` + loom's own Preflight + the Discussion/Plan/Webster producer" (old slot framing, contradicting the table 25 lines below), and its "This doc has not been rewritten to extract `Shed` explicitly" claim is now false.
- `:29` — **verify-only**, inherited from `shed-followups.md:446–449`: task B rewrote this line in full rather than leaving it self-contradicting, so E's obligation here was reduced to confirming the rewrite. It now names the live plan format with no v2 link and no "target format is changing" framing. Confirm and move on; no edit expected.
- `:44` — the mirror of `shed.md:8`'s atomicity claim; gets the pointer to `shed.md`'s carve-out.
- `:47–60` — the producer table, which gains a new `Kind` column per `shed-md-is-authoritative-loom-md-points`. The existing `Type` column is not touched.
- `:57` — row 9, `Batchifier`. **Two stale artifact names**, both left behind because task C's scope was rows 2–7 only and task F did not edit `loom.md` at all (`shed-followups.md:618`): the Input cell reads "`plan.md` (approved) + `webster.yaml`'s `batcher:` key".
  `plan.md` does not exist — the artifact is the `_lyx/plan/` directory, exactly the fix task C applied to rows 2–7.
  The `batcher:` key moved out of `webster.yaml` in task F (`e179ad0c`); the live key is `batcher.yaml`'s `active:` (see `CONSTRAINTS.md:352` and `docs/overview.md:282`).
  `shed-followups.md:452` assigns this row to E ("rewritten to match whatever task F landed"), so it is this task's.
  Note the row-number drift: `shed-followups.md` calls it "row 8" at `:452` and `:618`, which was correct at scoping time; task C's insertion of `Discussion-Validate` shifted it to **row 9**. Same row, renumbered.
- `:50` — `Discussion-Write`'s Input cell, "— (starting point)", is Question 2's subject; it stays as-is textually but now cites the thin-Input carve-out.
- `:58` — the `Webster` row, which must cite the carve-out explicitly instead of reading as an unresolved conflict with atomicity.
- `:70–72` — `loom.md`'s own copy of the two-part contract and the pointer rule.
- `:76–83` — the open-questions paragraph, cited as the whole paragraph for context. `:76–77` is the **already-resolved** first question (`Discussion-Validate` closed it, per task C) and needs no edit; the residue this task actually owns is `:78–83`. `:78` states thin-Output is open; `:79–82` is task C's hand-off note widening it to four producers and naming **task E**, which no longer exists; `:83` carries the stale `shed-producer-model-scoping` claim.
- `:82` specifically — "The `## The gate` section below still uses 'gate' in the perch sense (sense A) and is unchanged by this task — it remains task E's territory."
  **Disposition: the `## The gate` section (`:85–90`) is verify-only; only this dangling hand-off sentence is deleted.**
  Task C already resolved the overload it names, by landing the mechanical pre-checks as `Discussion-Validate`/`Plan-Validate` rather than `*-Review-Gate` precisely so "gate" could mean perch alone (`shed-followups.md:327–330`).
  The gate section therefore already uses the word in the only surviving sense, and needs no edit — the sentence is stale because the ambiguity it warns about is gone, not because the section is wrong.

**`CONSTRAINTS.md`** — `## Batcher Registry+Config Invariant` runs `:348–353`; the new invariant goes immediately after it.

**`docs/overview.md`** — `:283` (stale `loom` phase chain), `:300–301` (first `shed`/`glance` mention, gets the note), `:329–331` (second mention, left alone).

**`manifest/designs/hardener.md`** — `:17`'s "just with `Tenter` in the producer-slot instead of Discussion/Plan/Webster". `Shed` has no slots; reword to `Hardener`'s own producer list.

**`manifest/roadmap.md`** — `:110` (the raddle slot line) and `:54` (the six-task breakdown, which still names `shed-model-contradiction-sweep` as "E — final owner of `shed.md`/`loom.md`/this roadmap item, sweeps the remaining contradictions and adds the `CONSTRAINTS.md` pointer-rule invariant" — a present-tense pending claim about a task that never ran).

- `:51` — **in scope, and the subtlest site in the file.** It states the generic contract inside the *same* Planned `Shed` bullet whose `:57–61` states the carve-out, and it contradicts it twice over: "a producer is always atomic (one mechanical action or one LLM session, never an internal multi-step process of its own)" is the unqualified claim, and "**Input** (artifact(s) consumed, …)" forecloses the thin-Input case.
  This is the identical wording this task scopes at `shed.md:8` and `loom.md:44`, so treating it differently in the one doc that carries both halves six lines apart would leave the roadmap item self-contradicting.
  **Disposition: scope it.** Qualify `:51`'s atomicity clause to bind simple producers, and admit the thin-Input/thin-Output cases in the Input/Output definitions — in both cases pointing forward to `:57–61` rather than restating it, since that text stays verbatim.
Also add the phase-enum deferral record here, per `carry-forward-e-phase-enum-record` below. `:57–61` already carries the atomicity resolution and is **verify-only**; do not rewrite it, and prefer its wording when phrasing the design-doc text.

**`manifest/designs/shed-followups.md`** — section E starts at `:409` and runs up to (not including) `## F — batcher-standalone-split`'s heading at `:552`.
The supersession block goes at section E's head, immediately under the `## E — shed-model-contradiction-sweep` heading.

### Gotchas

- **`shed-followups.md` is a permanently grep-exempt file.**
  Per its own `**Override recorded 2026-08-09 (task B, as landed)**` note at `:227–231`, it deliberately preserves pre-rename paths and stale citations as a historical record.
  Any acceptance grep this task runs must exclude it, exactly as task B's did.
  Do not "fix" its stale references.
- **`by *value*` is italicised**, so a `by value` grep misses `shed.md:18`. Grep for `by \*value\*` or read the line.
- **The task body's `docs/overview.md:289`/`:318` are stale.** The live sites are `:300` and `:329`.
- **Task C landed the producer as `Discussion-Validate`, not `Discussion-Review-Gate`** (`shed-followups.md:327–330`), and renamed `Plan-Review-Gate` → `Plan-Validate` at the same time, to free the word "gate" for perch alone.
  `shed-followups.md` still says `Discussion-Review-Gate` in section C's body — that is the historical record, not an error to correct.
  Use `Discussion-Validate` in all new text.
- **`shed.md:13` and `:41` already list `Discussion-Validate`** — task E's Part-one obligation there is already satisfied. Verify, do not re-add.
- **`perchengine`'s round loop now lives in `internal/treadleengine`.** Both names appear across the docs; the carve-out text should name `internal/treadleengine` as the current home, matching `roadmap.md:60`.
- **Markdown Link Integrity is machine-checked** (`internal/lyxcwd/docslink_test.go`, `TestEnforcement_MarkdownLinks`) over every `.md` under `manifest/` and `docs/`, including `#anchor` resolution.
  This task adds cross-doc anchor links from `loom.md` into `shed.md`, so **any heading this task renames breaks an existing link**, and any new anchor must match the generated slug exactly.
  `CONSTRAINTS.md` is a **link target** for that test but not a scan source.

## Constraints

From `CONSTRAINTS.md`:

- **Markdown Link Integrity** — the binding invariant for this task. Every inline link this task introduces or touches must resolve, file part and anchor.
  The anchor for `shed.md`'s producer-contract section is currently `#producer-contract-vs-producer-definition`; changing that heading would break inbound links.
- **Documentation Lifecycle** — `shed.md` and `loom.md` are durable design docs (`loom.md:3` states it explicitly), kept until their modules land, then folded into `overview.md` and package headers. This task does not trigger any fold.
- **Producer Pointer-Rule Invariant** — added by this task, and this task must itself obey it: `loom.md` points at `shed.md`'s carve-out rather than restating it.
- **Fabric Vocabulary Invariant** — its `internal/**/*.md` walk does not reach `manifest/` or `docs/`, but the prose-doc split is a standing review obligation. No file here discusses fabric's own mechanism, so nothing should say warp/weft.

From `CLAUDE.md`:

- **Semantic line breaks, no fixed-column hard-wrap.** One sentence per line; break inside a long sentence only at an internal independent-clause boundary. This binds every `.md` file touched, including lines this task merely edits within an existing paragraph.
- **Task completion — docs land in the same commit.** Satisfied trivially: this task is only docs.
- **`manifest/roadmap.md` moves only on completing or adding a planned item.** This task edits `:110` (a Someday item carrying a claim task D falsified) and verifies `:57–61`. It does **not** mark the `Shed` item done — `Shed` is unbuilt; only its precondition is discharged.
- **Worktree isolation** — all work stays in `wts/shed-producer-typology-sweep`. No push to `main`.

## Testing

**This task is docs-only. There are no TDD candidates and no new test files.**
The meaningful failure mode is *incompleteness* — a residue site missed — which is checked by grep, not by an assertion.

Acceptance, in order:

1. **`go test ./internal/lyxcwd -run TestEnforcement_MarkdownLinks`** — the Markdown Link Integrity invariant. This is the one machine check that directly covers this task's output, since it adds cross-doc anchor links.
2. **A targeted grep set** proving zero surviving instances of the retired phrasings, run over `manifest/`, `docs/`, **and `README.md`**.
   `README.md` is in the grep roots even though this task edits nothing in it: `:97` inlines its own producer chain ("Preflight → Discussion → Plan → Webster → Finalize"), which is the same drift failure mode as `docs/overview.md:283`, and `CONSTRAINTS.md:263` records that `README.md`'s own outgoing links are checked by nobody.
   **Checked at branch point `c3af3c9c`: `README.md` is clean** — no `Raddle` phase, no `producer-slot`, no retired phrasing. Including it makes that a verifiable claim rather than an untested assumption, and catches a regression if a later edit reintroduces one.
   **Two standing exclusions apply to every grep below**, and each exists because the excluded text is correct where it stands — so a grep that flagged it would only be satisfiable by editing something this task's Scope forbids editing:
   - `manifest/designs/shed-followups.md` — permanently exempt by its own recorded convention (see Gotchas).
   - `docs/reference/status-schema.md` — carries the `phase` enum, which Scope declares out.
   The greps:
   - `producer-slot` — expect 0, **excluding `manifest/roadmap.md:48`**, where "no built-in concept of Preflight, a producer-slot, or Finalize at all" is a *correct negation* on a line this task does not own. Grep for the affirmative use (a producer sitting *in* the producer-slot), not the bare token.
   - `by \*value\*` — expect 0.
   - `shed-producer-model-scoping` used in a *present/future-tense owner* sense (`shed.md:63`, `loom.md:83`) — expect 0. A past-tense historical mention is fine.
   - `webster.yaml`'s `batcher:` key — expect 0 across `manifest/` and `docs/`; `loom.md:57` is the only live site, and `CONSTRAINTS.md:352` / `docs/overview.md:282` are the correct-target reference.
   - `plan.md` as a producer-table artifact name in `loom.md` — expect 0; the artifact is `_lyx/plan/`.
   - `Raddle` named as a **phase or slot between Webster and Finalize** (`roadmap.md:110`, `overview.md:283`) — expect 0. This is a phrase grep, not a bare-token grep: `docs/reference/status-schema.md`'s `raddle` enum entries are excluded per the standing exclusion above, and ordinary references to the raddle *module* are untouched.
   - `task E` / `shed-model-contradiction-sweep` referenced as a *pending future owner* (`loom.md:80`, `roadmap.md:54`) — expect 0.
   - The two dangling `below` back-references in `shed.md:7` and `:19` — verified by reading, since `below` is too common to grep usefully.

   The general rule these exclusions encode: **an acceptance grep must never be satisfiable only by editing text this task declares out of scope.**
   If a new grep trips on out-of-scope text, narrow the grep — do not widen the scope.
3. **`go test ./...`** as a regression backstop — `CONSTRAINTS.md` and the `docs/` tree are walked by several enforcement tests (`TestEnforcement_FabricVocabulary`, `TestEnforcement_GeometryLiterals`), so a docs edit can trip a Go test.

A positive check worth running as well: `manifest/roadmap.md:57–61`'s resolution text and the new `shed.md` carve-out text must agree in substance and terminology.
They are the same decision stated twice by necessity (roadmap item vs. design doc), which is the one place this task tolerates duplication.

## Q&A log

- **Q:** Does this task inherit task E's full remaining residue, or only the four items in its own brief? **A:** Full residue — this task is E's outright successor, inheriting all three ownership positions. E was removed without running and no other wiki task holds it.
- **Q:** Where does the typology text live — `shed.md`, `loom.md`, or both? **A:** Full text in `shed.md`'s producer-contract section; `loom.md` points at it — from its atomicity sentence at `:44`, and from a single anchor in the sentence introducing the producer table, which gains a new `Kind` column (see the auto-pick entry below). Applying the pointer rule to the docs themselves.
- **Q:** How is Question 2 (`Discussion-Write` has no Input) resolved? **A:** Symmetric thin-Input carve-out — the Input contract permits no Input for a chain-head producer, because its input is human intent, not an artifact. Explicitly reject the "wiki task record is the pointer target" framing as a mill-ism that does not transfer to `lyx`.
- **Q:** What terminology names the two kinds? **A:** `roadmap.md:58`'s exact terms — "simple, single-agent-spawn producer" and "bespoke, multi-spawn producer" — with "LLM-Producer" named only as a candidate.
- **Q:** How does the typology reconcile with `shed.md`'s existing four-way engine-adapter split? **A:** Two distinct axes, cross-referenced with one sentence. The adapter split cuts on engine type to argue "two adapters, not eleven"; the typology cuts on atomicity and crash-recovery ownership. They align on `Webster`/`perch`, not on the mechanical producers.
- **Q:** What name and placement for the `CONSTRAINTS.md` invariant? **A:** `## Producer Pointer-Rule Invariant`, immediately after `## Batcher Registry+Config Invariant` — the other producer-model, review-only entry.
- **Q:** Does the `shed`/`glance` note go at one site or both? **A:** `:300` only, the first hit in reading order. (The task body's `:289`/`:318` line numbers are stale.)
- **Q:** Does `shed-followups.md` get a supersession record? **A:** Yes — one block at the head of section E, recording E's removal, this task's succession, and the resolution of all three of E's surfaced questions. Section E's body stays intact as the historical record.
- **Q:** `roadmap.md:110`'s "deferred phase slot" and `overview.md:283`'s stale chain — open decisions or mechanical fixes? **A:** Mechanical. Task D already decided the Raddle fold; both sites just carry it forward. `:283` points at `loom.md`'s table rather than inlining a chain, so it cannot drift again.
- **Q:** `loom.md`'s producer table Type column already holds engine-type values — does the typology replace them, augment them per row, or get its own column? **A:** [auto-pick] New `Kind` column holding `simple`/`bespoke`; `Type` left alone on the engine axis; column order `# | Producer | Kind | Type | Input | Output`; the `shed.md` anchor stated once above the table, never per row. **Why:** merging both axes into one cell is the exact conflation `two-axes-cross-reference` exists to prevent, and twelve identical anchor links would itself read as a pointer-rule violation.
- **Q:** Is `Finalize` `simple` or `bespoke`? `roadmap.md:58` classifies it neither way, and adding the `Kind` column forces the call. **A:** [auto-pick] `bespoke`. **Why:** task D put raddle's parallel leaf forks plus a serial `Overview.md` step inside `Finalize`'s merge-lock critical section, which `finalize.md:38` requires be atomic end to end, and `finalize.md:9` spawns a fresh LLM on merge conflict — an internal multi-spawn process with a crash-recovery obligation `Shed` does not cover. The pure-Go happy path does not change the classification, since the axis is about who owns recovery. Unlike `Webster` and `perch`, `Finalize` does not ship that recovery today; that gap is recorded as an observation for the `Shed` build task, not designed here.
- **Q:** If `Finalize` is `bespoke`, does it now need an engine adapter, contradicting `shed.md:41`'s "mechanical Go-function producers need no translation adapter at all"? **A:** [auto-pick] No — it stays adapter-free on the engine axis while being `bespoke` on the typology axis, and the cross-reference sentence names it as the sharpest non-alignment case. **Why:** `Shed` drives it by calling a plain Go function that already satisfies the `ProducerRunner` seam; the conflict-path LLM spawn and raddle's leaf forks happen inside that function through the existing `shuttle` layer and are invisible to `Shed`. "Bespoke" means the producer owns its internals and its recovery, not that something different drives it. The "two new adapters, not eleven" count is unchanged.
- **Q:** What is the acceptance criterion? **A:** `TestEnforcement_MarkdownLinks` + a targeted grep set for the retired phrasings (excluding the permanently-exempt `shed-followups.md`) + `go test ./...` as a backstop. No new tests — `shed-followups.md:499` scopes the pointer rule as review-enforced.
