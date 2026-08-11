# Batch: loom-md-pointers-and-kind-column

```yaml
task: 'shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions'
batch: 'loom-md-pointers-and-kind-column'
number: 2
cards: 6
verify: go test ./internal/lyxcwd
depends-on: [1]
```

## Batch Scope

This batch writes `manifest/designs/loom.md`'s finished state: the naming note is repaired, the producer table gains a `Kind` column and loses its two stale artifact names, the atomicity sentence and the table-introducing sentence gain the single pair of anchors into `shed.md`'s carve-out, `loom.md`'s own copy of the two-part contract and pointer rule is reduced to a pointer, and the open-questions paragraph's dangling task-E hand-offs are retired.
It is one batch because it is one file whose sections cross-reference each other — the `Kind` column, the atomicity pointer and the contract restatement all have to agree about where the authoritative text lives, and splitting them would ship one half unverified against the other.

It depends on batch 1 because every pointer this batch writes resolves into `shed.md#producer-contract-vs-producer-definition`, whose content batch 1 authors, and because the `Kind` column's `simple`/`bespoke` values are read against batch 1 card 2's typology bullets — including `Finalize`'s bespoke classification, which is the one value that is not carry-forward from the roadmap.

Batch-local decision beyond `## Shared Decisions`: **the `shed.md` anchor never appears inside a table cell and is never repeated per row.**
Cells carry the bare word `simple` or `bespoke`.
Twelve identical per-row anchor links would itself read as a pointer-rule violation, which is the failure this batch exists to avoid.
The anchor does appear in four distinct sentences across cards 7 and 9 — two in card 7, two in card 9 — each one replacing a restatement with a pointer, which is the rule being obeyed rather than bent;
see the `shed-md-is-authoritative-loom-md-points` Shared Decision for the enumeration.

## Cards

### Card 6: repair loom.md's naming note

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/designs/loom.md` lines 15-17 carry a "**Naming note (later addition):**" paragraph with two stale claims.
  Line 16 reads "`loom` = `Shed` + loom's own Preflight + the Discussion/Plan/Webster producer" — old slot framing, which contradicts the flat producer table twenty-five lines below it and which `Shed` has no concept of.
  Rewrite it to say `loom` is `Shed` plus `loom`'s own ordered producer list, and point at the producer-table section rather than enumerating a slot triple.
  Line 17 reads "This doc has not been rewritten to extract `Shed` explicitly — it remains the authoritative design for the engine described here."
  The first clause is now false: the extraction has happened, `manifest/designs/shed.md` is the authoritative description of `Shed`'s generic mechanism, and this doc is the authoritative description of `loom`'s specific producer list plus the engine-level detail (crash recovery, pause, session bootstrap) `shed.md` does not restate.
  Rewrite line 17 to state that split, matching the wording `manifest/designs/shed.md` line 3 already uses for the same split, so the two docs agree.
  Keep the existing `[shed.md](shed.md)` link;
  do not add an anchor to it in this card — cards 7 and 8 own this file's two anchored pointers.
- **Commit:** `docs(loom): repair the naming note's slot framing and the false not-yet-extracted claim`

### Card 7: point loom.md's atomicity sentence and table intro at shed.md's carve-out

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two pointers into `manifest/designs/shed.md`, both written as inline links to `shed.md#producer-contract-vs-producer-definition`.
  These are the first two of the file's four anchored pointers;
  card 9 writes the other two, in the contract-restatement sentence and the pointer-rule sentence.
  **(a)** `manifest/designs/loom.md` line 44 currently reads "It is a generic engine that walks one ordered, flat list of **producers**, each an atomic mechanical action or LLM session, honoring resume/crash-recovery/pause uniformly across the whole list."
  This is the mirror of the claim batch 1 card 2 qualified in `shed.md`, and it gets the symmetric treatment: qualify the atomicity clause so it binds **simple** producers, and link to `shed.md`'s carve-out.
  Do **not** restate the carve-out here — the qualification plus the link is the whole edit.
  **(b)** The sentence introducing the producer table (currently line 45, ending "purely which producers are in the list, in what order:") gains the second and last anchor: state that the table's new `Kind` column records the simple/bespoke typology and link to `shed.md`'s carve-out for its definition.
  The anchor appears **once** in that introducing sentence — never repeated per row and never inside a cell.
  Card 8 adds the column itself;
  this card only writes the sentence above the table, so that card 8's cells can carry the bare word.
- **Commit:** `docs(loom): scope atomicity to simple producers and anchor the typology pointer above the table`

### Card 8: add the Kind column and repair row 9's stale artifact names

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/finalize.md`
  - `manifest/roadmap.md`
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `manifest/designs/loom.md`'s twelve-row producer table (currently lines 47-60).
  **(a) Add one new `Kind` column** whose only values are `simple` and `bespoke`.
  Column order becomes `# | Producer | Kind | Type | Input | Output`.
  The existing **Type** column is **left alone** — it holds engine-type values (`mechanical`, `LLM`, `LLM/perch`, `black box …`) and stays on that axis.
  Do not merge the two axes into one cell;
  that is precisely the conflation the cross-reference in `manifest/designs/shed.md`'s engine-adapter section exists to prevent, and it would make the engine axis unreadable at a glance.
  Update the header separator row to six columns, matching the new six-column header above the table's twelve producer rows.
  **(b) The values.**
  Rows 4, 8, 10, 11 and 12 — `Discussion-Review`, `Plan-Review`, `Webster`, `Webster-Review`, `Finalize` — are `bespoke`.
  The other seven — `Preflight`, `Discussion-Write`, `Discussion-Validate`, `Plan-Sweep`, `Plan-Write`, `Plan-Validate`, `Batchifier` — are `simple`.
  Row 12 `Finalize` is the one classification that is not carry-forward from `manifest/roadmap.md`;
  batch 1 card 2 argues it in `shed.md` and this cell records it.
  **(c) Row 12's Type cell** currently reads "mechanical (**mostly**)".
  That hedge was this classification, unnamed;
  now that the `Kind` cell says `bespoke`, drop the hedge and let the Type cell read `mechanical` on the engine axis, which is correct and consistent with `manifest/designs/shed.md`'s adapter list keeping `Finalize` among the adapter-free mechanical Go-function producers.
  Leave the rest of the row's Input/Output cells unchanged.
  **(d) Row 10's Output cell** currently describes `internal/websterengine`'s per-batch loop as staying "opaque to `loom`'s flat list".
  Keep the black-box framing and the existing intra-doc link, but adjust the wording so the internal loop reads as the carve-out's licensed case rather than as an unresolved conflict with atomicity — it is a bespoke, multi-spawn producer, exempt by design.
  Do not add an anchor link into `shed.md` from inside the cell.
  **(e) Row 9's two stale artifact names.**
  The `Batchifier` row's Input cell currently reads "`plan.md` (approved) + `webster.yaml`'s `batcher:` key".
  Neither name is live. `plan.md` does not exist — the artifact is the `_lyx/plan/` directory, exactly the fix applied to rows 2-7 already.
  The `batcher:` key moved out of `webster.yaml`;
  the live key is `batcher.yaml`'s `active:`, as recorded in `CONSTRAINTS.md`'s `## Batcher Registry+Config Invariant` and in `docs/overview.md`'s batcher module entry.
  Rewrite the cell to name `_lyx/plan/` (approved) and `batcher.yaml`'s `active:` key.
  **(f) Row 2's Input cell** currently reads "— (starting point)".
  Leave the text as-is;
  it is the thin-Input carve-out's only instance and the carve-out now covers it from `shed.md`.
  Do not add a per-cell link.
  Keep every table cell on a single line, per the repo's Markdown convention.
- **Commit:** `docs(loom): add the Kind column and repair the Batchifier row's stale artifact names`

### Card 9: reduce loom.md's contract restatement and pointer-rule copy to pointers

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/designs/loom.md` lines 70-72 carry this file's own copy of the two-part contract and the pointer rule.
  Both are edited, for different reasons, and each gets **its own** inline link to `shed.md#producer-contract-vs-producer-definition` — two links, one per sentence, not one merged sentence and not a shared link.
  They are this file's third and fourth anchored pointers;
  card 7 wrote the first two.
  **(a) Line 70** states "A producer's contract is exactly two parts — **Input** (a *pointer* to the format-contract file defining consumed artifact(s)' shape, never a restated copy of its content) and **Output** (same pointer discipline)."
  The thin-Input carve-out contradicts it outright: `Discussion-Write` has no Input and therefore no pointer.
  Qualify the sentence to admit the thin-Input and thin-Output cases, pointing at `manifest/designs/shed.md`'s contract section rather than restating either carve-out.
  **(b) Line 71** restates the pointer rule in full, including its worked example and the `mill-start` analogy.
  Reduce it to a pointer at `shed.md`'s statement of the rule.
  **Note precisely which rule forces this, so it is not over-applied:** the new `## Producer Pointer-Rule Invariant` in `CONSTRAINTS.md` binds **instruction files** (agent prompts and skills), not design docs, so `loom.md` restating the rule was never itself a violation of it.
  What forces the reduction is the authority split — `shed.md` owns the generic mechanism — plus the plain drift risk of two full copies.
  Do **not** cite the invariant as the reason in the doc text.
  **(c) Line 72** ("Review is never a property attached to the producer it reviews…") is correct and stays;
  keep its existing `[the gate](#the-gate)` link intact.
  Both links this card writes resolve to the same `shed.md` section and anchor as card 7's two, and that is correct rather than redundant: each of the four sits in a different sentence making a different claim, and each replaces a restatement rather than decorating one.
  What the pointer rule forbids is a repeated per-row or per-cell link, and none of the four is that.
- **Commit:** `docs(loom): reduce the contract restatement and pointer-rule copy to shed.md pointers`

### Card 10: retire the open-questions paragraph's resolved and dangling text

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `manifest/designs/loom.md` lines 76-83 are the open-questions paragraph.
  Lines 76-77 record the **first** question as already resolved by `Discussion-Validate` and need no edit — leave them alone.
  The residue this card owns is lines 78-83.
  **(a) Line 78** states the second question — whether `Preflight`/`Finalize`'s thin Output needs its own carve-out — "stays open, untouched by this task".
  It is now resolved.
  Rewrite it to record the resolution and point at `manifest/designs/shed.md`'s contract section for the two-case statement, without restating either case here.
  **(b) Lines 79-81** are task C's hand-off note widening the second question to four producers and naming **task E** as its resolver.
  Task E was removed from the wiki without running and no longer exists, so the hand-off dangles.
  Delete the hand-off note.
  Its factual content — that `Discussion-Validate` and `Plan-Validate` share the thin-Output property — is already carried by the resolution text (a) points at, so nothing is lost.
  **(c) Line 82** reads "The [`## The gate`](#the-gate) section below still uses 'gate' in the perch sense (sense A) and is unchanged by this task — it remains task E's territory."
  Delete this sentence.
  The ambiguity it warns about is gone: task C landed the mechanical pre-checks as `Discussion-Validate`/`Plan-Validate` rather than `*-Review-Gate` precisely so "gate" could mean `perch` alone, so the section already uses the word in the only surviving sense.
  The sentence is stale because the ambiguity resolved, not because the section is wrong — **do not edit the `## The gate` section itself** (currently lines 85-90);
  card 11 verifies it needs no change.
  **(d) Line 83** reads "Wiki task `shed-producer-model-scoping` is the dedicated survey pass that reconciles this table against `discussion-format.md`/`plan-format*.md` and `raddle.md`/`finalize.md`, and produces the actual buildable follow-up tasks — this table is settled on the model, not yet on every file-level detail."
  That task completed on 2026-08-09 and this task is `loom.md`'s final owner, so the present-tense pending-owner claim is stale.
  Delete the sentence.
  After these edits the paragraph must not name any pending future owner and must not leave a heading dangling — check that whatever remains reads as a finished paragraph rather than a fragment.
- **Commit:** `docs(loom): resolve the thin-Output question and retire the dangling task-E hand-offs`

### Card 11: verify loom.md's two verify-only sites need no edit

- **Context:**
  - `manifest/designs/loom.md`
  - `_mill/discussion.md`
  - `docs/reference/plan-format.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff verification card.
  Confirm two sites in `manifest/designs/loom.md` are already correct and record the confirmation in the batch's own output, making no edit.
  **(a) Line 29** was rewritten in full by an earlier task in the chain rather than left self-contradicting: it must name the live plan format with no link to a retired v2 document and no "the target format is changing" framing.
  Read it and confirm.
  If it does still carry a v2 link or the changing-format framing, that is a genuine finding — report it as a blocker rather than silently fixing it, since the whole point of the verify-only classification is that the repair already landed.
  **(b) The `## The gate` section** (currently lines 85-90) uses "gate" in the `perch` sense only, which is the sole surviving sense after `Discussion-Validate`/`Plan-Validate` landed under those names.
  Read the section and confirm it needs no edit.
  Card 10 already deleted the dangling sentence that pointed at it;
  the section body itself is untouched.
  Make no edit in this card.
  If either check fails, stop and report rather than editing — a failure here means an assumption this whole batch rests on is wrong.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/lyxcwd` runs `TestEnforcement_MarkdownLinks` (`internal/lyxcwd/docslink_test.go`) plus `TestEnforcement_GeometryLiterals` and `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) and the package's own tests.
This is the batch that most needs it: cards 7 and 9 introduce new cross-doc anchor links from `manifest/designs/loom.md` into `manifest/designs/shed.md`, and `TestEnforcement_MarkdownLinks` resolves both the file part and the `#anchor` of every such link.
Cards 6, 9 and 10 also delete or rewrite prose containing existing intra-doc anchors (`#the-gate`, the phase-machine and Webster section anchors), so a careless rewrite that drops or mistypes one is caught here rather than in review.
The scope is `internal/lyxcwd` rather than the whole tree because that package holds every enforcement test a Markdown edit can trip;
the repo-wide `go test ./...` regression backstop runs at the done gate.

No new test file is written — the batch is pure docs.
Card 11 is the batch's own zero-diff verification gate over the two sites an earlier task in the chain already repaired, and the acceptance grep set in batch 3 card 18 is what proves the retired phrasings (`plan.md` as a producer-table artifact name, `webster.yaml`'s `batcher:` key, `task E` as a pending future owner, the present-tense `shed-producer-model-scoping` claim) have zero surviving instances.
