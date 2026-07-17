# Batch: discussion-format-doc

```yaml
task: 'loom: pin the spawn/handover status schema + discussion-format.md'
batch: discussion-format-doc
number: 2
cards: 1
verify: test -f docs/reference/discussion-format.md
depends-on: []
```

## Batch Scope

Author the second new contract doc: `docs/reference/discussion-format.md`, which pins the
`_lyx/discussion/` directory contract — the `discussion.md` ↔ Plan analogue of `plan-format.md`.
Self-contained; no other batch depends on its contents. The design is fully specified in
`_mill/discussion.md` (Decisions `discussion-on-disk-split`, `decision-record-sections`,
`support-log-sections`, `doc-rigor-moderate`, `no-schema-version`).

## Cards

### Card 2: Write docs/reference/discussion-format.md

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/plan-format.md`
- **Edits:** none
- **Creates:**
  - `docs/reference/discussion-format.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `docs/reference/discussion-format.md` as a durable, pinned contract
  doc — the analogue of `plan-format.md`. Include, in order:
  1. A top blockquote marking it **Status: Contract — pinned** and durable (kept, not deleted on
     landing).
  2. **What it is + who consumes it:** the `discussion.md` ↔ Plan contract. `_lyx/discussion/`
     is a **directory with two files** and a hard access boundary:
     - `decision-record.md` — the distilled record, the Plan producer's **sole** input.
     - `support-log.md` — the raw support log, read by the **Discussion-review gate**, **never**
       by the Plan producer.
     State the rationale (mirrors Builder's "distilled digest, never raw prose"; two files give a
     hard boundary against the Plan producer ingesting the raw interview transcript / token
     bloat). Paths are weft-overlay (`_lyx/`) resolving via `internal/hubgeometry` when code
     lands.
  3. **`decision-record.md` shape:** **no frontmatter** (state why: `format:` dropped —
     `no-schema-version`; `approved:` dropped — approval lives in `_lyx/status.json`'s `history`,
     the single total-status locus, and loom always drives Plan after approval, unlike
     plan-format whose `approved:` exists for standalone `lyx builder run`). Sections, in order:
     **Goal / Scope / Decisions / Constraints / Auto-mode assumptions / Open risks / Acceptance
     criteria**, plus an **optional, non-binding "Notes for the plan writer"** subsection.
     Pin the compaction rules: Decisions carry Decision + Rationale only; **rejected alternatives
     do NOT appear here** — they live in `support-log.md`; must-cover test scenarios go under
     Acceptance criteria; no italic prose-coaching. Explain the optional Notes subsection is a
     non-exhaustive head-start because the Plan producer explores the codebase itself.
  4. **`support-log.md` shape:** sections **Interview** (turn-by-turn, distilled not verbatim) /
     **Rejected alternatives** / **Review rounds** (per round: verdict + findings + how resolved)
     / **Question ledger** (running open/resolved questions + `--auto` picks). Pin the primary
     purpose of the **Review rounds** ledger: it is the **anti-circling** store each new
     discussion-review round reads before raising findings, so successive reviewers do not
     re-raise points earlier rounds already settled. Note (without binding it) that who *writes*
     the ledger — the Discussion producer vs the perch discussion gate — is a later
     implementation detail; this doc pins the contract (the ledger exists, its purpose, its
     shape).
  5. **A short validation-check list** (spec for a future validator): required decision-record
     sections present; the **Plan-never-reads-`support-log`** boundary; both files exist under
     `_lyx/discussion/`.
  6. **A compact worked example:** a minimal `decision-record.md` and a minimal `support-log.md`
     for a fictional task, showing the section skeleton of each (lighter than plan-format's full
     example).
  Terse, structured, house style of `plan-format.md`; no filler.
- **Commit:** `docs(discussion-format): pin _lyx/discussion/ record+log contract`

## Batch Tests

`verify: test -f docs/reference/discussion-format.md` — pure-docs batch, no runnable surface;
the only mechanical assertion is file existence. Content correctness is a review-gate concern.
