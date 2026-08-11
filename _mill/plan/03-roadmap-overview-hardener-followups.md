# Batch: roadmap-overview-hardener-followups

```yaml
task: 'shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions'
batch: 'roadmap-overview-hardener-followups'
number: 3
cards: 7
verify: go test ./internal/lyxcwd
depends-on: [2]
```

## Batch Scope

This batch closes out the four remaining files and then proves the whole task landed. `manifest/roadmap.md` gets its self-contradicting generic-contract sentence scoped, its six-task breakdown line retired of a pending-owner claim about a task that never ran, its Someday `raddle` item's false "deferred phase slot" framing replaced by the fold task D decided, and the phase-enum deferral record written. `docs/overview.md` gets its stale `loom` phase chain replaced by a pointer and its first `shed`/`glance` occurrence disambiguated. `manifest/designs/hardener.md` loses its "producer-slot" wording. `manifest/designs/shed-followups.md` gets one supersession block at the head of section E.
Card 18 then runs the acceptance grep set over the whole tree.

It is one batch because these are seven small, independent edits across four files plus the terminal verification, all of which read against the finished state of batches 1 and 2 — the supersession block records what those batches resolved, and the grep set can only run once every retired phrasing has been rewritten.
It depends on batch 2 because card 15 points `docs/overview.md` at `manifest/designs/loom.md`'s producer table and card 18's greps assert over `loom.md`'s finished text.

Batch-local decision beyond `## Shared Decisions`: **card 18's greps must never be satisfiable only by editing text this task declares out of scope.**
If a grep trips on out-of-scope text, narrow the grep — do not widen the scope.

## Cards

### Card 12: scope roadmap.md's generic-contract sentence

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/roadmap.md` line 51 sits inside the Planned `Shed` item and contradicts, twice over, the resolution recorded six lines below it at lines 57-61.
  It reads "A producer's contract is two parts only — **Input** (artifact(s) consumed, pointer to the format-contract file defining their shape, never a copy) and **Output** (artifact produced, same pointer discipline) — a producer is always atomic (one mechanical action or one LLM session, never an internal multi-step process of its own)."
  The atomicity clause is the unqualified claim, and the Input definition forecloses the thin-Input case.
  This is the identical wording scoped at `manifest/designs/shed.md` line 8 and `manifest/designs/loom.md` line 44, so leaving it alone would leave the one doc that carries both halves six lines apart self-contradicting.
  Qualify the atomicity clause so it binds **simple** producers, and admit the thin-Input and thin-Output cases in the Input and Output definitions.
  **The two halves point at different targets, and conflating them would write a false cross-reference.**
  The atomicity qualification **points forward to lines 57-61**, which state the simple/bespoke typology — that text stays verbatim and is the source of the wording used everywhere else.
  The thin-Input and thin-Output admission **points at `manifest/designs/shed.md`'s producer-contract section instead**, because lines 57-61 say nothing about either carve-out;
  that section is their authoritative home per the `shed-md-is-authoritative-loom-md-points` Shared Decision.
  Pointing the thin-case admission at 57-61 would send a reader to text that does not discuss what is being admitted — the exact class of doc contradiction this task exists to remove.
  Restate neither target: a prose reference to `shed.md`'s contract section is sufficient here, and `manifest/roadmap.md` need not carry an anchored link.
  Lines 57-61 are **verify-only**: read them to confirm they still state the resolution accurately, then leave them byte-identical.
  Do not mark the `Shed` roadmap item done — `Shed` is unbuilt;
  only its precondition is discharged.
- **Commit:** `docs(roadmap): scope the generic contract sentence to simple producers and thin contracts`

### Card 13: retire roadmap.md's pending task-E claim and the raddle slot framing

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/finalize.md`
  - `manifest/designs/raddle.md`
  - `manifest/designs/shed.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two edits in `manifest/roadmap.md`, neither of which touches lines 57-61.
  **(a) Line 54** is the six-task breakdown of the Planned `Shed` item.
  Its final clause still reads "`shed-model-contradiction-sweep` (E — final owner of `shed.md`/`loom.md`/this roadmap item, sweeps the remaining contradictions and adds the `CONSTRAINTS.md` pointer-rule invariant)" — a present-tense pending claim about a task that was removed from the wiki without ever running.
  Rewrite that clause to record that E was superseded by `shed-producer-typology-sweep`, which discharged E's scope: it swept the remaining contradictions, added the `CONSTRAINTS.md` pointer-rule invariant, and took over E's three ownership positions.
  Keep the rest of the line's task chain intact — A, B, C, D and F all landed and their descriptions are correct.
  **(b) Line 110** is the Someday `raddle` item's third clause, reading "deferred phase slot between Webster and Finalize."
  That is false: task D decided it — Raddle-regeneration folds into `Finalize`'s own contract, not a separate producer and not a slot.
  Rewrite it to state the fold and drop the slot framing entirely, consistent with `manifest/designs/finalize.md` and with `manifest/designs/shed.md`'s own statement of the same fold.
  This is carry-forward of a landed decision, not a new one — do not re-open what fills the slot, because there is no slot.
- **Commit:** `docs(roadmap): record task E's supersession and the raddle fold into Finalize`

### Card 14: write the phase-enum deferral record

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed-followups.md`
  - `internal/loomengine/coherence.go`
  - `docs/reference/status-schema.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the phase-enum deferral record to `manifest/roadmap.md`'s Planned `Shed` item, placed **after the atomicity resolution at lines 57-60 and before the `designs/shed-followups.md` pointer at line 61** — that is, between line 60 and line 61.
  Line 61 is itself the "See … for the full surfaced-open-questions record" pointer, so it sits inside the commonly-cited 57-61 range;
  the record goes above it, not after it.
  Two or three sentences, no more.
  It must state that `internal/loomengine/coherence.go`'s `validPhases` map and `docs/reference/status-schema.md`'s matching phase enum are deliberately left as-is;
  that realigning them lands with the `Shed` build task, because the flat producer list **replaces** the enum rather than editing it, and rewriting it now would invent an interim phase set `Shed` would immediately discard;
  and that an earlier task in the chain already renamed `builder` to `webster` in both, so the enum is not stale in the way it was at scoping time.
  Read `internal/loomengine/coherence.go` and `docs/reference/status-schema.md` only to confirm the current enum values before writing;
  **change neither file** — the enum is explicitly out of this task's scope.
  This record is the second of the two obligations `manifest/designs/shed-followups.md` assigns for the deferral (leave the enum alone, *and* record the deferral alongside the roadmap edits), and this task is the roadmap's last owner, so there is no later chance to write it.
  Confirm before writing that no such record exists yet — `manifest/roadmap.md` currently has no match for "phase enum", "validPhases", or "coherence.go".
- **Commit:** `docs(roadmap): record the deferred phase-enum realignment on the Shed item`

### Card 15: repair docs/overview.md's stale chain and disambiguate shed/glance

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/shed.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two edits in `docs/overview.md`.
  **(a) Line 283** is the `loom` module entry and prints the chain "drives Preflight → Discussion → Plan → Webster → Raddle → Finalize, each gated by a perch review".
  It names a `Raddle` phase that no longer exists and predates the flat producer list.
  Rewrite it to drop `Raddle` and **point at `manifest/designs/loom.md`'s producer table** rather than inlining a chain at all — inlining a producer list here is exactly what produced the current drift, so the fix must not recreate the failure mode.
  The entry already carries a `See [manifest/designs/loom.md](../manifest/designs/loom.md).` line at its end;
  the new pointer should be an inline link to that file's producer-table section anchor so a reader lands on the table itself.
  Leave the rest of the entry — the built-Discussion-producer and built-Planner-producer notes, the config-module sentence, the design status — unchanged.
  **(b) Line 300** carries the first `shed`/`glance` mention in reading order: "(Earlier drafts split reed into separate `shed`/`glance` modules; both folded back into reed …)".
  Add one disambiguating sentence there: the `shed` named in that parenthetical is an abandoned earlier `reed` model/view draft, unrelated to `Shed` the outer phase-FSM, and link `manifest/designs/shed.md`.
  Put the note at line 300 **only** — the second occurrence around lines 329-331 is twenty-nine lines later in the same section chain, inherits the disambiguation, and sits inside a bullet that already explains the fold-back.
  Do not touch it, and do not rename the historical draft references anywhere;
  that would rewrite the record of what those drafts were called.
- **Commit:** `docs(overview): point the loom entry at loom.md's table and disambiguate shed/glance`

### Card 16: drop hardener.md's producer-slot framing

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
- **Edits:**
  - `manifest/designs/hardener.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/designs/hardener.md` line 17 ends "the same lifecycle `loom` uses, just with `Tenter` in the producer-slot instead of Discussion/Plan/Webster."
  `Shed` has no slots of any kind — that is the whole point of the flat producer list.
  Reword the clause so it describes `Hardener`'s own producer list containing `Tenter` where `loom`'s list contains its Discussion, Plan and Webster producers, with no slot framing.
  This is a one-clause rewrite: leave the rest of the bullet, its `[shed.md](shed.md)` link, and the surrounding `Tenter`/`Hardener` definitions untouched.
- **Commit:** `docs(hardener): drop the producer-slot framing in favour of Hardener's own producer list`

### Card 17: add the supersession block at the head of shed-followups section E

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/designs/shed-followups.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one supersession block to `manifest/designs/shed-followups.md`, placed **immediately under the `## E — shed-model-contradiction-sweep` heading** (currently line 409) and before the task-body heading that follows it.
  Section E's body — everything from there up to but not including the `## F — batcher-standalone-split` heading — **stays intact as the historical record**;
  do not edit it, do not rename the section heading, and do not correct any of its stale paths, line numbers, or `Discussion-Review-Gate` naming.
  The file's own convention is that bodies are the scoping-time record, amended only by appended blocks — tasks A, B and C each did exactly this.
  Follow that convention's formatting: an `**Override recorded …**`-style bolded lead line naming the date and this task, then the record.
  The block must record five things.
  (a) Task E was removed from the wiki without ever running, and is superseded by `shed-producer-typology-sweep`.
  (b) E's **Question 1** — `Webster` versus producer atomicity — was **resolved** on 2026-08-11 (the producer typology plus the atomicity carve-out) rather than merely surfaced, so section E's "not resolved by this task" framing no longer holds for it.
  (c) E's **Question 2** — `Discussion-Write` has no Input — is resolved by the successor task as a symmetric thin-Input carve-out, recorded in `manifest/designs/shed.md`'s producer-contract section, with the "the task record is the Input" framing explicitly rejected as a mill-ism that does not transfer to `lyx`.
  (d) E's **Question 3** — the overloaded `shed` name — is discharged by a disambiguating note at the first occurrence in `docs/overview.md`.
  (e) The successor task discharged all three of E's ownership positions (`manifest/designs/loom.md`'s final owner, `manifest/roadmap.md`'s last owner, `docs/overview.md`'s last owner), and also wrote the deferred-phase-enum record E was assigned.
  Keep the block short — it is a record, not a restatement of the decisions.
- **Commit:** `docs(shed-followups): record task E's supersession and the resolution of its three questions`

### Card 18: acceptance grep sweep

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
  - `docs/overview.md`
  - `manifest/designs/hardener.md`
  - `README.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff verification card.
  Run the acceptance grep set over `manifest/`, `docs/` and `README.md`, and report the results.
  **Two standing exclusions apply to every grep**, because the excluded text is correct where it stands and a grep that flagged it could only be satisfied by editing something this task's scope forbids: `manifest/designs/shed-followups.md` (permanently exempt by its own recorded convention) and `docs/reference/status-schema.md` (carries the out-of-scope phase enum).
  The greps, each expecting zero surviving hits:
  (1) `producer-slot` used **affirmatively** — a producer sitting *in* the producer-slot.
  Exclude `manifest/roadmap.md`'s "no built-in concept of Preflight, a producer-slot, or Finalize at all", which is a *correct negation* on a line this task does not own;
  grep for the affirmative use, not the bare token.
  (2) `by \*value\*` — the italicised form, which a plain `by value` grep misses.
  (3) `shed-producer-model-scoping` used in a **present or future-tense owner** sense.
  A past-tense historical mention is fine.
  (4) `webster.yaml`'s `batcher:` key across `manifest/` and `docs/`. `CONSTRAINTS.md` and `docs/overview.md` carry the correct target (`batcher.yaml`'s `active:`) and are the reference, not a hit.
  (5) `plan.md` used as a producer-table artifact name in `manifest/designs/loom.md` — the artifact is `_lyx/plan/`.
  (6) `Raddle` named as a **phase or slot between Webster and Finalize**.
  This is a phrase grep, not a bare-token grep: ordinary references to the raddle *module* are untouched and are not hits.
  (7) `task E` or `shed-model-contradiction-sweep` referenced as a **pending future owner**.
  (8) The two dangling "below" back-references formerly at `manifest/designs/shed.md` lines 7 and 19 — verified by **reading** those lines, since "below" is too common to grep usefully.
  Also confirm by reading that `README.md` remains clean: no `Raddle` phase in its own inlined producer chain, no `producer-slot`, no retired phrasing.
  `README.md` is in the grep roots even though this task edits nothing in it, because it inlines its own producer chain and its outgoing links are checked by nobody — including it makes cleanliness a verified claim rather than an assumption.
  Finally, read `manifest/roadmap.md` lines 57-61 against the new carve-out text in `manifest/designs/shed.md` and confirm the two agree in substance and terminology;
  they are the same decision stated twice by necessity, and that is the one place this task tolerates duplication.
  Make no edit in this card.
  If any grep produces a hit that is genuinely in scope, stop and report it as a finding rather than fixing it here — the fix belongs in the card that owns the file.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/lyxcwd` runs `TestEnforcement_MarkdownLinks` (`internal/lyxcwd/docslink_test.go`) plus `TestEnforcement_GeometryLiterals` and `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) and the package's own tests.
Card 15 adds two new inline links (`docs/overview.md` into `manifest/designs/loom.md`'s producer-table anchor, and into `manifest/designs/shed.md`), so anchor resolution is the direct machine check on this batch's output.
The scope is `internal/lyxcwd` rather than the whole tree because that package holds every enforcement test a Markdown edit can trip.

Card 18 is the batch's — and the task's — substantive acceptance gate, and it is deliberately a grep sweep rather than an assertion: the meaningful failure mode for this task is **incompleteness**, a residue site missed, which no unit test can detect.
No new test file is written and none is expected;
the pointer-rule invariant is scoped as review-enforced.
The repo-wide regression backstop is `go test ./...`, which runs at the configured done gate — `CONSTRAINTS.md` and the `docs/` tree are walked by several enforcement tests, so a docs edit can trip a Go test outside this batch's verify scope.
