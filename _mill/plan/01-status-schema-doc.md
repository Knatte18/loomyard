# Batch: status-schema-doc

```yaml
task: 'loom: pin the spawn/handover status schema + discussion-format.md'
batch: status-schema-doc
number: 1
cards: 1
verify: test -f docs/reference/status-schema.md
depends-on: []
```

## Batch Scope

Author the first of the two new contract docs: `docs/reference/status-schema.md`, which pins
the `_lyx/status.json` spawn/handover status schema. Self-contained — no other batch depends on
its contents. The design is fully specified in `_mill/discussion.md` (Decisions
`status-file-format-json`, `status-single-schema-superset`, `status-seed-writer-is-a-lyx-command`,
`status-field-set`, `verdict-history-granularity`, `no-schema-version`, `doc-rigor-moderate`);
this batch renders that design as a pinned doc in the house style of `builder-contract.md`.

## Cards

### Card 1: Write docs/reference/status-schema.md

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/loom.md`
  - `docs/modules/builder-contract.md`
- **Edits:** none
- **Creates:**
  - `docs/reference/status-schema.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `docs/reference/status-schema.md` as a durable, pinned contract doc.
  Include, in order:
  1. A top blockquote marking it **Status: Contract — pinned** and stating it is a durable
     reference doc (kept, not deleted on landing), the loom analogue of
     `builder-contract.md`/`plan-format.md`.
  2. **What it is:** the `_lyx/status.json` file is loom's single source of truth for
     orchestration state — the file loom rewrites every `lyx loom run`, and whose t=0 "seed"
     is written at spawn time. State that it is durable weft-overlay state (`_lyx/`,
     git-synced → cross-machine resume) whose path resolves via `internal/hubgeometry` when
     code lands (no path construction is specified by this spec doc).
  3. **Format decision (defended):** the file is **JSON via `internal/state`** (`WriteJSON[T]`/
     `ReadJSON[T]`, locked + atomic + typed), the same primitive `builder` uses for
     `state.json`. Explicitly record that this **overrides the board brief's "plain YAML"**:
     the brief's real point was "structured, not markdown-with-frontmatter," and JSON was chosen
     over YAML because this is machine-written/machine-read state and `lyx loom status --watch`
     pretty-prints it for humans, so the on-disk file need not be hand-readable. Phrase it as a
     deliberate decision, not an accident (per discussion `status-file-format-json`).
  4. **The seed / handover:** define "seed" as the t=0 contents of the same schema, written at
     spawn time by a **lyx Go command** (the mill-spawn analogue; an optional thin `ly-spawn`
     skill may wrap it later, but the Go command is the writer — never an agent). loom's
     Preflight **requires the file to exist** and fails loud if missing (the file's existence is
     the handoff signal). One schema (a superset), not two: the seed is that schema with only
     the handoff fields (`slug`, `parent`, `phase: discussion`, an initial `narration`)
     populated and the rest at their zero/null values. Name the *role* ("the spawn-time lyx
     command"); do not bind the exact subcommand — that is pinned when the command lands.
  5. **The schema:** reproduce the pinned JSON shape from discussion `status-field-set`
     (`slug`, `parent`, `phase`, `stage`, `narration`, `history[]`, `start_sha`,
     `pause_requested`, `next_action`) and a per-field notes list. Pin the vocabularies:
     `phase` ∈ `preflight|discussion|plan|builder|raddle|finalize|done`; `stage` ∈
     `produce|gate`; each `history[]` entry is `{phase, outcome: approved|stuck, bounced_to?,
     ts}`. `history` is a **per-phase outcome trail** (per-round verdicts live in perch's block
     files, not here). Every timestamp field (`history[].ts`) is **RFC3339 UTC** (e.g.
     `2026-07-17T10:01:30Z`). `pause_requested` is kept **in-status** (note this diverges from
     builder, which uses a separate pause flag file). `next_action` is a dedicated
     machine-checkable "blocked on a human" field, also reflected in `narration`'s `wait:`
     segment. Note that **no `schema_version`/`format` field** is carried, and why (discussion
     `no-schema-version`).
  6. **Parse discipline:** strict, fail-loud parsing — the `internal/state` read rejects
     unknown or malformed fields (the `KnownFields(true)` discipline of builder's `ParseOutcome`
     and the burler verdict-parse). Never guess a status.
  7. **A short validation-check list** (spec for a future validator): required fields present;
     `phase`/`stage`/`outcome` values within their fixed vocabularies; timestamps RFC3339 UTC;
     strict fail-loud parse rejecting unknown/malformed fields.
  8. **A compact worked example:** a realistic seed instance and a realistic mid-run instance of
     the same file (mirroring discussion `status-single-schema-superset`).
  Keep it terse and structured (house style of `builder-contract.md`); no prose-coaching filler.
- **Commit:** `docs(status-schema): pin _lyx/status.json spawn/handover schema`

## Batch Tests

`verify: test -f docs/reference/status-schema.md` — a pure-docs batch with no runnable code
surface; the only mechanical assertion is that the new file exists at its pinned path. Content
correctness is a review-gate concern (the plan/code review reads it against `_mill/discussion.md`),
not a runnable test.
