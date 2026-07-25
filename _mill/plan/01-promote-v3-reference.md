# Batch: promote-v3-reference

```yaml
task: 'plan-format v3: flat card list'
batch: promote-v3-reference
number: 1
cards: 3
verify: go build ./...
depends-on: []
```

## Batch Scope

This batch promotes the v3 design into a durable reference doc and relocates the
parts of the design that do not belong in a schema contract, then deletes the
design doc and repoints every link that pointed at it. It is one batch because
the three cards are tightly coupled around a single file's promotion: card 1
authors the new reference doc, card 2 relocates the detailed DAG/scheduling
design *out* of the design doc into `webster-rewrite.md` (and repoints
`webster-rewrite.md`'s own links, including the anchor fragment), and card 3
deletes the now-emptied design doc and repoints the remaining inbound links. The
cards run in declared order (1 → 2 → 3): cards 1 and 2 both **read** the design
doc `manifest/designs/plan-format-v3.md`, and card 3 **deletes** it last, so no
card ever reads a deleted file.

External interface the next batch consumes: the created file
`docs/reference/plan-format-v3.md` — batch 2's additive cross-links point at it.

This is a docs-only batch: no Go source is touched. `verify: go build ./...` is a
sanity gate (see `## Shared Decisions` → `docs-only-verify` in the overview).

There are **no `Moves:` in this batch** (see `## Shared Decisions` →
`create-plus-delete-not-a-move`), so there is deliberately **no `## Rename
mechanic`** section.

## Cards

### Card 1: create docs/reference/plan-format-v3.md (the v3 reference doc)

- **Context:**
  - `manifest/designs/plan-format-v3.md`
  - `docs/reference/plan-format.md`
- **Edits:** none
- **Creates:**
  - `docs/reference/plan-format-v3.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Author a new durable reference doc that pins **plan-format
  v3 (flat card list)**. Source material: the authoritative design at
  `manifest/designs/plan-format-v3.md` (read in full) plus v2's reusable
  card-level prose in `docs/reference/plan-format.md` (v3 is v2-minus-the-batch,
  so most card-level prose is reusable with batch framing removed and card
  numbering changed from `NN.C` to flat `N`). The document MUST contain the
  following, and MUST NOT contain the excluded material:
  - **Title + Status header.** Title heading `# Plan format v3 — flat card
    list`. A `> **Status: Contract — pinned.**` header stating this is a
    **durable reference doc that is kept** (per the documentation lifecycle),
    like v2's own Status header. Do **NOT** carry over the design doc's
    "Status: Design — not built / Supersedes … / deliberate breaking change /
    when this lands the durable parts fold into `plan-format.md` (replacing v2)
    and this file is deleted" framing — that lifecycle language is now false.
    Instead include a **coexistence note**: v3 coexists with v2
    ([plan-format.md](plan-format.md)) during a transition; v2 stays live and
    valid and retires only when the **webster: rewrite for flat card list**
    roadmap item lands and `builder` is deleted (v3 wins); v3 is the format that
    webster-rewrite consumes. Link v2 as same-directory `plan-format.md`.
  - **What a card is.** Adapt the design doc's "What a card is" (compiles/builds
    on its own; independently committable; bundles its own test when it
    introduces new behavior) and keep the "the DAG is a *consequence* of the
    compile-validity requirement" key-insight paragraph.
  - **Batch is gone / the card is the unit.** Adapt the design doc's "What
    changes: batch is gone as a plan-schema concept" — the plan's unit is the
    individual card; any later grouping of cards is a webster-internal
    execution-policy optimization, not a plan-schema concept. Fold in a short
    note that v2's per-batch `## Scope` concept is **removed entirely** — a
    card's own typed file-op fields *are* its declared footprint (this is the
    `scope-dropped` decision).
  - **Plan vs. schedule.** Carry over the design doc's "Plan vs. schedule"
    section verbatim in substance (schema philosophy: the flat card list is a
    DAG of intent, not an execution order; the format must not change if the
    executor's scheduling policy changes).
  - **On-disk layout.** `_lyx/plan/` (unchanged path) with:
    `00-overview.md` — frontmatter carrying **scalar-only** keys `format: 3`,
    `approved: true`, and an optional plan-level `root:`; body carrying a short
    task-framing paragraph, an ordered **Card Index** whose entries read
    `N — <card-slug> — <one-line intent>`, and the optional plan-level body
    sections `## Shared Decisions`, `## Rename mechanic`, and `## verify:`.
    `NN-<card-slug>.md` — one file per card (`NN` = zero-padded card order);
    the file *is* the card. State that Card Index ↔ card files are cross-checked
    mechanically.
  - **Card fields and order.** A card file's content, in this order: a title
    heading `# Card N — <name>`; then `**What:**` (the instruction — v3 keeps
    lyx's established `What:` name, playing v2's role); then the five typed
    file-op fields `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`,
    `**Moves:**`, **all required**, each carrying the literal `none` sentinel on
    its label line when empty; then `**Depends-on:**` (**new required field**,
    placed after `Moves:`); then optional `**Commit:**` and optional
    `**verify:**`. Reproduce v2's exact per-field grammar (single-backtick-wrapped
    paths, one per indented sub-bullet, no commentary, no line-range suffix, no
    inline comma lists; the `Moves:` sub-bullet is the `` `src` -> `dst` `` ASCII
    -arrow pair). Preserve v2's `Context:` semantics prose (advisory, not a
    strict allowlist; `Edits:` files are implicitly read and never repeated in
    `Context:`) and the per-card mutual-exclusivity rule (`card-field-overlap`).
  - **Depends-on.** Document the new required `**Depends-on:**` field: value is a
    list of card ids (plain card numbers `N`) or `none`; it references only other
    cards in the **same plan**. Explain why it is safe in v0 (it is never a claim
    about external code, so no hallucination risk) and that it powers the
    `depends-on-order` mechanical check (flags a card whose `Depends-on:` names a
    *later* card or itself, or an id referencing no existing card). Draw this from
    the design doc's "Why `depends-on` is safe to include now" reasoning.
  - **Card path resolution: `root:` and `//`.** Adapt v2's section, changing one
    thing: `root:` is now an **optional plan-level** field (in `00-overview.md`
    frontmatter), not per-batch. Keep the `//` worktree-root escape, the
    degenerate `root: "."` handling, and the parse-time normalization rule
    (every card path normalized once to a plain worktree-relative forward-slash
    path). Where v2 names the `scope-malformed` check for card-path
    well-formedness, rename it to **`card-path-malformed`** (Scope is gone).
  - **Moves and the Rename mechanic.** Adapt v2's "Moves and the Rename mechanic"
    section, changing one thing: the `## Rename mechanic` section is now
    **plan-level** — one section in `00-overview.md`, **required when any card in
    the plan has a non-empty `Moves:`** (the `move-mechanic-missing` check, now
    plan-level). Reproduce the canonical `git mv`-then-surgical-edits text
    verbatim. Keep the rename-plus-extraction rule (one `Moves:` pair + a
    separate `Creates:`).
  - **Numbering and commit subject.** Cards are numbered flat **`N` (1..N)**
    across the whole plan; the per-card file prefix `NN` (zero-padded) must equal
    the heading `N`. The **default commit subject is `N: <name>`** — the card
    heading's `<name>`; there is no separate `<short what>` seed. An explicit
    `**Commit:**` overrides the default but must start with the card's own `N: `
    prefix (the `commit-subject-mismatch` check). Explain that commit-per-card is
    the resume trail keyed to `git log`.
  - **verify model.** `verify:` is **optional per-card** plus an **optional
    plan-level integration verify** that lives as a **`## verify:` body section
    in `00-overview.md`** (the `00-overview.md` frontmatter stays scalar-only:
    `format`/`approved`/`root`). There is **no mandatory per-batch/per-card
    verify gate** (batch is gone); the build+unit-test gate is implicit in the
    card definition and is run by the consumer (webster) after each card; the
    plan-level `## verify:` is the single integration suite run once at the end.
  - **Deferred / forward-compat.** A short section stating the symbol fields
    (`creates-symbols`/`edits-symbols`/`reads-symbols`) are **deliberately
    omitted in v0** (waiting on codeintel), with a compact summary and pointers
    to `../../manifest/designs/codeintel-redesign.md` and
    `../../manifest/designs/webster-rewrite.md`. This section MUST **name the
    derived `changes-files` union** — the derived union of the typed file-op
    fields (`Edits:` ∪ `Creates:` ∪ `Deletes:` ∪ both `Moves:` endpoints) —
    explicitly as the artifact webster's future contract-verification compares
    actual changed files against, pointing to
    `../../manifest/designs/webster-rewrite.md` for the verification semantics.
    It MUST also note that the detailed continuous-DAG-update / symbol-matching /
    SCC-merging **scheduling** design now lives in `webster-rewrite.md`'s
    scheduling section (pointer only — the detail is relocated there by card 2,
    not reproduced here). A one-line pointer to the parked parallel-execution
    idea (`../../manifest/designs/webster-parallel-execution.md`) is acceptable
    but optional.
  - **Validation checks (spec for the future validator).** Enumerate the machine
    checks this format is designed to support (they land with webster, not this
    doc — same "spec for the future validator" posture v2 uses), adapting v2's
    list to the flat-card model. **Kept/adapted:** `format-unrecognized` /
    `plan-unapproved`; `index-file-mismatch` (Card Index ↔ card files: numbering,
    slugs, no gaps, no orphaned file — this **absorbs** v2's
    `card-count-mismatch`); `card-path-malformed` (every card path normalized,
    relative, clean, no `..`; `root:`/`//` resolution is part of "normalized" —
    this is v2's `scope-malformed` renamed); `move-format`; `move-redundant`;
    `move-source-missing`; `move-target-collision`; `move-mechanic-missing` (now
    plan-level); `card-missing-field` (now including `Depends-on:`);
    `card-field-overlap`; `card-numbering` (flat `N` runs 1..M, no gaps/dups;
    file prefix `NN` matches heading `N`); `path-missing`;
    `commit-subject-mismatch` (a present `Commit:` must start with `N: `).
    **New:** `depends-on-order`. **Dropped:** `verify-missing` (no mandatory
    verify gate), `chain-end-dangling` (chains gone), `batch-oversized` (oversized
    gone), `card-outside-scope` (Scope gone), `card-count-mismatch` (folded into
    `index-file-mismatch`).
  - **Worked example.** Rewrite v2's worked example (the `--json` flag on
    `lyx board list`) for v3: per-card files under `_lyx/plan/`, a
    `00-overview.md` carrying a Card Index + a `## Shared Decisions` entry + a
    plan-level `root:` + a `## Rename mechanic` section + a `## verify:` section,
    flat `N` card numbering, at least one card with a non-`none` `Depends-on:`
    field, and a card with a non-empty `Moves:` pair. The example MUST be
    **byte-consistent** across Card Index ↔ per-card filenames ↔ card
    headings/numbering (a reader can cross-check them). Do not port v2's
    batch-report YAML — see the exclusions below.
  - **Related.** Carry over the design doc's `## Related` section (its three
    entries: `webster-rewrite.md`, `fabric.md`, `codeintel-redesign.md`) but
    **repoint each same-directory link to `../../manifest/designs/<name>.md`**
    (the up-and-over path from `docs/reference/` to `manifest/designs/`), so the
    links resolve from the new location instead of dangling.
  - **Explicitly EXCLUDE (do not port from v2, per strip-execution-policy /
    output-report-punted):** no batch-sizing "Principle #0"; no `oversized: true`
    / large-window-role content; no deferred-verify chains (`verify: deferred` /
    `chain-end:`); no red-tests / bounded-self-fix / recovery section; no review
    cadence; no roles/models section; no batch-report (output-half) schema. The
    v3 doc pins **only the input plan schema**; the output/report contract is the
    consumer doc's territory.
  - Follow `mill:markdown` conventions throughout.
- **Commit:** `1: create docs/reference/plan-format-v3.md v3 reference doc`

### Card 2: relocate the DAG/scheduling design into webster-rewrite.md and repoint its links

- **Context:**
  - `manifest/designs/plan-format-v3.md`
- **Edits:**
  - `manifest/designs/webster-rewrite.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Relocate the detailed scheduling/DAG design **out of**
  `manifest/designs/plan-format-v3.md` **into** `manifest/designs/webster-rewrite.md`
  (it is scheduling/execution, not schema, so it must not live in the pinned
  contract), and repoint every `plan-format-v3.md` link in `webster-rewrite.md`.
  Do **NOT** delete `manifest/designs/plan-format-v3.md` in this card (card 3
  does that) — read it as source and copy the relevant sections out.
  - **Content to relocate** (copy from `manifest/designs/plan-format-v3.md`,
    adapting section headings to fit): the sections currently titled
    "Mechanical DAG derivation" (both `Mechanism 1 — plan-internal symbol
    matching` and `Mechanism 2 — codeintel as a verification layer`), "Symbol
    fields and the planner/webster codeintel-availability mismatch (resolved)",
    and "Continuous DAG update as cards land (designed, deferred with the symbol
    fields)". Fold this content into `webster-rewrite.md`'s existing
    "Scheduling: no DAG, no SCC merging in v0" section as one or more
    subsections. Keep `webster-rewrite.md`'s existing `HasSymbolFields()` Go code
    block; the relocated detail is the *why* behind that reserved branch.
  - **Anchor target.** Give the relocated continuous-DAG-update content a stable
    subsection heading whose GitHub slug ends up being
    `#continuous-dag-update-as-cards-land-deferred-with-the-symbol-fields` (i.e.
    a heading `### Continuous DAG update as cards land (deferred with the symbol
    fields)`), so the link fixed below resolves to it.
  - **Repoint the five `plan-format-v3.md` links in `webster-rewrite.md`** (grep
    the whole file for `plan-format-v3.md` — do not fix only some). As of writing
    they are:
    - line 5 `[plan-format v3](plan-format-v3.md)` → `[plan-format v3](../../docs/reference/plan-format-v3.md)`.
    - line 32 — currently
      `[plan-format-v3.md](plan-format-v3.md#continuous-dag-update-as-cards-land-designed-deferred-with-the-symbol-fields)`.
      Because the mechanism now lives **in this same file**, rewrite this
      sentence so the link is a **local same-doc anchor** pointing at the
      relocated subsection (target
      `#continuous-dag-update-as-cards-land-deferred-with-the-symbol-fields`),
      NOT a link to `plan-format-v3.md`. The reworded sentence should read as
      "the whole DAG/cycle-detection/SCC-merging mechanism is designed
      [below](#continuous-dag-update-as-cards-land-deferred-with-the-symbol-fields)
      but depends on `codeintel`/symbol fields and is out of scope until those
      land" (or equivalent). Do **not** leave a dead `#continuous-dag-update-…`
      anchor pointing at the trimmed reference doc.
    - line 72 `[plan-format-v3.md](plan-format-v3.md)` → `[plan-format-v3.md](../../docs/reference/plan-format-v3.md)`.
    - line 165 `[plan-format-v3.md](plan-format-v3.md)` → `[plan-format-v3.md](../../docs/reference/plan-format-v3.md)`.
    - line 190 (in `## Related`) `[plan-format-v3.md](plan-format-v3.md)` → `[plan-format-v3.md](../../docs/reference/plan-format-v3.md)`.
  - Follow `mill:markdown` conventions.
- **Commit:** `2: relocate DAG scheduling design into webster-rewrite.md`

### Card 3: delete the design doc and repoint the remaining inbound links

- **Context:** none
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/designs/loom-planner.md`
  - `manifest/designs/codeintel-redesign.md`
  - `manifest/designs/webster-parallel-execution.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/plan-format-v3.md`
- **Moves:** none
- **Requirements:** Delete the promoted design doc and repoint every remaining
  inbound Markdown link so nothing dangles. This card runs last in the batch, so
  cards 1 and 2 have already extracted everything they need from the design doc.
  - **Delete** `manifest/designs/plan-format-v3.md` with `git rm` (so the
    deletion is staged as a rename-free removal in git).
  - **Repoint each inbound link to where the content it names actually lives.**
    Most links name the v3 *schema* and repoint to the reference doc; a few in
    `codeintel-redesign.md` name the detailed *mechanism* that card 2 relocated
    into `webster-rewrite.md`, and those must point there instead — otherwise
    they resolve to a doc that (by card 1) deliberately excludes that detail.
    Grep each file for `plan-format-v3.md` to confirm the exact set at edit time.
    As of writing:
    - `manifest/designs/loom.md`: line 37 `[plan-format v3](plan-format-v3.md)`
      (names the schema) → `[plan-format v3](../../docs/reference/plan-format-v3.md)`.
    - `manifest/designs/loom-planner.md`: lines 10 and 28 (both name the schema)
      → `../../docs/reference/plan-format-v3.md`.
    - `manifest/designs/webster-parallel-execution.md`: lines 12, 69, 82 (all
      name the schema / `depends-on`) → `../../docs/reference/plan-format-v3.md`.
      Per the task's Out-of-scope, touch this file **only** to repoint these
      links — make no other edits.
    - `manifest/designs/codeintel-redesign.md` — **split by what the link names**:
      - Lines 16 (`'s symbol fields`) and 165 (Related — "the symbol fields this
        module makes trustworthy") name the symbol-field *concept*, which the
        reference doc's Deferred/forward-compat section names → repoint to
        `../../docs/reference/plan-format-v3.md`.
      - Lines 25 ("mechanical DAG-derivation is webster's own logic"), 139
        ("plan-internal name matching for not-yet-existing symbols" = Mechanism
        1), and 150 ("verifies symbol names against the real codebase" =
        Mechanism 2) name the detailed mechanism card 2 relocated into
        `webster-rewrite.md` → repoint these to the **same-directory**
        `webster-rewrite.md` (NOT the reference doc, which excludes this detail).
  - **Fix the bare-prose mention at `manifest/designs/codeintel-redesign.md`
    line 18** ("see plan-format-v3.md's resolution of this exact machine-mismatch
    problem"): the availability-mismatch *resolution* it names is relocated by
    card 2 into `webster-rewrite.md`, so pointing at the reference doc's basename
    would be false. Repoint it to `webster-rewrite.md` — either convert it to a
    proper same-directory link (`see [webster-rewrite.md](webster-rewrite.md)'s
    resolution of this exact machine-mismatch problem`) or reword to name
    `webster-rewrite.md`.
  - **Leave the one remaining bare-prose format-name mention unchanged:**
    `manifest/designs/loom-planner.md` line 24 ("the plan-format-v3 card list" —
    a format-*name* mention with no `.md` and no link) stays as-is; it names the
    format, not the file, so it is neither a link nor a claim about doc contents.
  - Follow `mill:markdown` conventions.
- **Commit:** `3: delete design doc, repoint remaining inbound links`

## Batch Tests

`verify: go build ./...` is a sanity gate only — this batch touches no Go source,
so a green build confirms nothing under `internal/`, `cmd/`, or `testdata/` was
disturbed (the v2 parser/validator and its plan fixtures must stay byte-identical).

The substantive verification is link/consistency review, done by reading plus
these grep checks (run from the worktree root after the batch's cards land):

- **No dangling same-directory link remains:** `grep -rn "](plan-format-v3.md)" docs/ manifest/`
  must return **nothing** (every inbound link is now up-and-over to
  `docs/reference/plan-format-v3.md`, or — for `webster-rewrite.md` line 32 and
  the mechanism-naming `codeintel-redesign.md` links 25/139/150 + prose 18 — a
  point at the local/same-dir `webster-rewrite.md` where card 2 relocated that
  content).
- **No `designs/plan-format-v3.md` path reference remains:** `grep -rn "designs/plan-format-v3.md" docs/ manifest/`
  must return **nothing** (the roadmap's item — repointed in batch 2 — is the
  only place that used that spelling; within this batch confirm none was
  introduced).
- **The design doc is gone and the reference doc exists:**
  `test ! -e manifest/designs/plan-format-v3.md && test -e docs/reference/plan-format-v3.md`.
- **The relocated anchor resolves:** confirm `manifest/designs/webster-rewrite.md`
  contains a heading whose slug is
  `continuous-dag-update-as-cards-land-deferred-with-the-symbol-fields` and that
  line 32's link now points at that local anchor, not at `plan-format-v3.md`.
- **New doc's outbound `## Related` links resolve** from `docs/reference/`:
  each `../../manifest/designs/<name>.md` target
  (`webster-rewrite.md`, `fabric.md`, `codeintel-redesign.md`) exists on disk.
- **Worked example internal consistency:** read
  `docs/reference/plan-format-v3.md`'s worked example and confirm Card Index
  entries, per-card filenames, and card headings/numbering all agree.
