# Batch: producer-table-and-rename-sweep

```yaml
task: 'format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate'
batch: 'producer-table-and-rename-sweep'
number: 2
cards: 5
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: [1]
```

## Batch Scope

This batch lands the producer-table half of the task and the repo-wide rename sweep.
It inserts `Discussion-Validate` as `loom.md`'s new row 3, renumbers the table, rewrites rows 2–8's Input/Output cells so they name the artifacts that exist, repairs the open question at `loom.md:75` that this task's own edit falsifies, renames `Plan-Review-Gate` → `Plan-Validate` at all eight sites across four markdown files, sweeps the superseded `Discussion-Review-Gate` out of `roadmap.md:47`, and records the override note in `shed-followups.md`'s section C.
It is one batch because the rename is a single atomic token sweep whose acceptance gate is one repo-wide zero-hit grep — splitting it across batches would leave the repo describing one producer under two names between commits.

It depends on batch 1 because `loom.md`'s new `Discussion-Validate` row points into `discussion-format.md`'s validation-checks section, which batch 1 writes.

Batch-local decisions beyond the overview's shared set, each recorded so no card reopens it:

- **`loom.md:56`'s `Batchifier` Input cell keeps its stale `plan.md` reference.**
  It is old row 8, outside the spec's "rows 2–7 only" grant (`shed-followups.md:329`), and `shed-followups.md:447` already assigns `:56` to task E to be rewritten against whatever task F lands.
  Only its number cell changes.
- **`loom.md:78`'s `## The gate` section is not rewritten.**
  It uses "gate" in sense A (perch), was already ambiguous before this task ran, and is task E's territory — E must re-read `loom.md` end to end anyway (`shed-followups.md:486`).
- **The rename crosses task-ownership boundaries deliberately.**
  `shed.md`, `roadmap.md`, and `shed-followups.md` are touched even though task E owns the first two and the third is the spec file.
  Only the literal token is replaced;
  no surrounding prose is rewritten.
  This is the precedent task A set and recorded at `shed-followups.md:449–453`.

## Cards

### Card 8: `loom.md` producer table — insert `Discussion-Validate`, renumber, name real artifacts

- **Context:**
  - `docs/reference/discussion-format.md`
  - `docs/reference/plan-format.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/shed-followups.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Edit the producer table at `:47–59` only.
  Insert `Discussion-Validate` as the new **row 3**, immediately after `Discussion-Write` and immediately before `Discussion-Review`, mirroring the `Plan-Write` → `Plan-Validate` → `Plan-Review` ordering already in the table.
  The list is ordered and the order is its semantics, so a pre-check appended as a last row would be meaningless.
  Renumber the whole table: old rows 3–11 become 4–12.
  Rows 1 (`Preflight`) and 9–12 (`Batchifier`, `Webster`, `Webster-Review`, `Finalize`) get their **number cell renumbered and nothing else** — no other cell content is touched, so task E still writes their finished state.
  In particular row 9's `Batchifier` Input cell keeps its stale `plan.md` reference.
  Rows 2–8's `Input` and `Output` cells are rewritten to the following pinned content — the exact prose of each cell is the implementer's to word, the content is not:

  | # | Producer | Type | Input | Output |
  |---|---|---|---|---|
  | 2 | `Discussion-Write` | LLM | — (starting point) | `_lyx/discussion/` (`decision-record.md` + `support-log.md`), shape: `discussion-format.md` |
  | 3 | `Discussion-Validate` | mechanical | `_lyx/discussion/` → `discussion-format.md`'s validation checks | pass/fail |
  | 4 | `Discussion-Review` | LLM/`perch` | `_lyx/discussion/` (both files) → `discussion-format.md` | verdict (APPROVED/stuck) + review file |
  | 5 | `Plan-Sweep` | mechanical | `_lyx/discussion/decision-record.md` (approved) | scout inventory (internal artifact, not gated) |
  | 6 | `Plan-Write` | LLM | `_lyx/discussion/decision-record.md` (**never** `support-log.md`) + `Plan-Sweep`'s inventory | `_lyx/plan/`, shape: `plan-format.md` |
  | 7 | `Plan-Validate` | mechanical | `_lyx/plan/` → `plan-format.md`'s existing hard-fail checks (e.g. `depends-on-order`) | pass/fail |
  | 8 | `Plan-Review` | LLM/`perch` | `_lyx/plan/` → `plan-format.md` | verdict + review file |

  Row 7 is the renamed old row 6 (`Plan-Review-Gate` → `Plan-Validate`);
  its `Type` and `Output` cells are unchanged and its `Input` cell changes only `plan.md` → `_lyx/plan/`.
  Every Input/Output cell is a **pointer** to the format-contract file defining the consumed artifact's shape, never a restated copy of that file's content — that is `shed.md:24–25`'s definition of a producer's contract and the reason this task exists.
  Table cells stay on one line each, per `CLAUDE.md`'s markdown rule.
- **Commit:** `docs(loom): insert Discussion-Validate, renumber the producer table, and point rows 2-8 at the artifacts that exist`

### Card 9: `loom.md:75` — repair the open question this task's own edit falsifies

- **Context:**
  - `docs/reference/discussion-format.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `:75` today opens with two open questions in one sentence.
  The **first** reads that `Discussion` has no mechanical pre-gate "the way `Plan-Review-Gate` mirrors `plan-format.md`'s `depends-on-order` check — asymmetric, possibly by nature (no structural check exists for `Discussion` the way order-validation exists for a card list) rather than an oversight, but worth deciding rather than assuming".
  Card 8's insertion decides it, so rewrite that clause to record the resolution: the asymmetry was **not** by nature, `Discussion-Validate` closes it, and the checks it runs are `discussion-format.md`'s validation checks.
  The line must no longer present this as unresolved, and it must no longer contain the token `Plan-Review-Gate`.
  Leave the **second** open question on that line — whether `Preflight`/`Finalize`'s unusually thin Output (pass/fail only, no real artifact) needs its own carve-out in the Output contract's definition — **untouched**;
  `shed-followups.md:482` assigns it to task E.
  Append a short hand-off note recording that this task **widens** that second question's subject: `Plan-Validate` and the newly-inserted `Discussion-Validate` now have the identical thin-Output property, so E resolves the question over four producers, not two.
  Also record in the commit message that `:75`'s first clause was repaired by this task, so task E does not go looking for it, and that `loom.md:78`'s `## The gate` section still uses "gate" in the perch sense and is handed to E unchanged.
- **Commit:** `docs(loom): record Discussion-Validate as the resolution of line 75's first open question and widen the thin-Output hand-off`

### Card 10: `shed.md` — rename and insert `Discussion-Validate` into both producer lists

- **Context:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two lines, each getting both a rename and an insertion, per `shed-followups.md:423–424`.
  (a) `:13`'s `loom` producer enumeration currently lists `Preflight`, `Discussion-Write`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Review-Gate`, `Plan-Review`, `Batchifier`, `Webster`, `Webster-Review`, `Finalize`.
  Rename `Plan-Review-Gate` → `Plan-Validate` and insert `Discussion-Validate` between `Discussion-Write` and `Discussion-Review`, so the enumeration matches `loom.md`'s table order after card 8.
  (b) `:41`'s mechanical-Go-function-producer list currently reads `Preflight`, `Plan-Sweep`, `Plan-Review-Gate`, `Batchifier`, `Finalize`.
  Rename `Plan-Review-Gate` → `Plan-Validate` and insert `Discussion-Validate`, placing it so the list stays in `loom.md`'s table order.
  Replace only the literal tokens and add only the new names — no surrounding prose is rewritten;
  task E owns this file's producer-contract section.
- **Commit:** `docs(shed): rename Plan-Review-Gate to Plan-Validate and add Discussion-Validate to both producer lists`

### Card 11: `manifest/roadmap.md` — rename sweep and the `Discussion-Review-Gate` disposition

- **Context:**
  - `manifest/designs/loom.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three token replacements, one per line, no surrounding prose rewritten.
  `:45` and `:46` each carry one `Plan-Review-Gate` → replace with `Plan-Validate`.
  `:47` carries one `Discussion-Review-Gate`, inside task C's own entry in the six-task breakdown ("add the `Discussion-Review-Gate` producer") → replace with `Discussion-Validate`.
  The roadmap describes the live, forward-looking plan and names this task's deliverable, so leaving it naming a producer that never lands under that name is the same defect the `Plan-Review-Gate` sweep exists to fix.
  This is **not** a roadmap move under `CLAUDE.md`'s rule — no item changes state, none is added, none is removed — so do not touch any item's status, and do not touch the deferred-slot line reading "deferred phase slot between Webster and Finalize" (currently `:102`), which is task E's remaining obligation.
  After the replacement, read each of the three sentences and confirm it still reads correctly;
  a rename that lands inside prose describing the *old* concept is a silent regression the zero-hit grep will not catch.
- **Commit:** `docs(roadmap): rename Plan-Review-Gate to Plan-Validate and name task C's producer Discussion-Validate`

### Card 12: `shed-followups.md` — rename sweep and the override note

- **Context:**
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/shed-followups.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two changes, and one thing deliberately left alone.
  (a) Replace the `Plan-Review-Gate` token at `:304` and `:306` with `Plan-Validate`.
  `:304` carries **both** tokens in one sentence ("scope a `Discussion-Review-Gate` mechanical producer, mirroring `Plan-Review-Gate`") — only the `Plan-Review-Gate` half is swept;
  the `Discussion-Review-Gate` half stays.
  `:306` carries only `Plan-Review-Gate` and is a plain rename site.
  (b) All nine `Discussion-Review-Gate` occurrences — `:265`, `:281`, `:283`, `:301`, `:304`, `:325`, `:329`, `:342`, `:424` — keep their original wording.
  This file is a task-body archive, not live documentation: it records what was *specified*, and rewriting those nine would falsify the record and make the spec's own reasoning at `:301–312` read incoherently.
  (c) Add exactly one new note under section C's `#### The `Discussion-Review-Gate` producer` subsection at `:301`, opening with the bolded lead-in `**Override recorded 2026-08-11 (task C, as landed).**` — the same convention already used at `:289`, `:296`, `:441`, `:449`, `:462`, and `:470`.
  The note states that the producer landed as `Discussion-Validate` rather than `Discussion-Review-Gate`, and that `Plan-Review-Gate` was renamed to `Plan-Validate` at the same time, with the reason: `loom.md` overloads "gate" across perch's black-box review gate and the mechanical pre-check, so `-Validate` frees "gate" to mean perch alone.
- **Commit:** `docs(shed-followups): sweep Plan-Review-Gate to Plan-Validate and record task C's naming override`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` runs `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks`, enforcing `CONSTRAINTS.md`'s Markdown Link Integrity invariant over every `.md` file under `manifest/` and `docs/`.
All four files this batch edits are inside that scan scope.
The inbound anchor `loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots`, referenced from `shed.md:3`, `:11`, and `:61`, must still resolve — card 8 edits the table under that heading but not the heading text itself, so the anchor survives;
this test is what confirms it rather than an assumption.
Do not add an allowlist entry to work around a break this batch creates — fix the link.

The batch is otherwise docs-only with no runnable surface of its own.
Three additional criteria are verified by grep rather than by a test, and the batch is not done until all three pass:

1. **Zero surviving `Plan-Review-Gate`.**
   `grep -rn "Plan-Review-Gate" --include=*.md --include=*.go --include=*.yaml . | grep -v '^./_mill/'` returns nothing.
   `_mill/` is excluded because the discussion and review artifacts quote both tokens as subject matter;
   they are task-state records, not documentation.
   This is the rename's acceptance gate, and it is the authoritative criterion — not the occurrence count.
2. **`Discussion-Review-Gate` survives only where it should.**
   The same grep for that token returns hits in `manifest/designs/shed-followups.md` **only** — zero in `manifest/roadmap.md`, `manifest/designs/loom.md`, `manifest/designs/shed.md`, and both `docs/reference/` files.
   `shed-followups.md`'s section C additionally contains the new `**Override recorded 2026-08-11 (task C, as landed).**` note.
3. **No surviving `discussion.md` or `plan.md` artifact reference in the rows this task owns** — new rows 2–8 of `loom.md`'s producer table.
   Scoped to those rows, not the whole file: row 9's `Batchifier` Input cell keeps its stale `plan.md` by this batch's own scope decision, and both strings appear legitimately elsewhere in the file (e.g. `:27`, `:39`), which are task E's.

Every rename site must also be read back in context after the sweep.
A token replacement that lands inside a sentence describing the *old* concept is a silent regression none of the three greps will catch.

`go build ./...` and `go test ./...` are covered once by the configured `pipeline.done_gate` rather than per batch.
They are a no-op assertion for a docs-only task, but they prove no Go file was touched by an over-broad sweep.
