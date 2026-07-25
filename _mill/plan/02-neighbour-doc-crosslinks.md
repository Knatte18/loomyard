# Batch: neighbour-doc-crosslinks

```yaml
task: 'plan-format v3: flat card list'
batch: neighbour-doc-crosslinks
number: 2
cards: 3
verify: go build ./...
depends-on: [1]
```

## Batch Scope

This batch makes the three small neighbour-doc updates that keep the docs
consistent now that `docs/reference/plan-format-v3.md` exists (created in batch
1): soften v2's Status header, move the roadmap item Planned → Done, and add
additive v3 cross-links to the durable neighbour docs. It depends on batch 1
because every edit here points at `docs/reference/plan-format-v3.md`, which
batch 1 creates. All edits are **additive** — v2 stays live and valid; no
neighbour doc is legacy-labelled or reconciled (see `## Shared Decisions` →
`coexistence-not-replacement` in the overview).

Docs-only batch: no Go source is touched. `verify: go build ./...` is a sanity
gate. No `Moves:`, so no `## Rename mechanic`.

## Cards

### Card 4: soften the v2 plan-format.md Status header

- **Context:** none
- **Edits:**
  - `docs/reference/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make the **single softening edit** to v2's Status header
  (the `> **Status: Contract — pinned.**` block, currently lines 3–9) and nothing
  else in the file. Today it asserts, absolutely: "**v2 supersedes v1 outright**
  (a version bump, not a dialect): `builder` refuses a `format: 1` plan via the
  `format-unrecognized` check exactly as it refuses any other unrecognized value
  — there is no dual-version support and no production v1 plans exist to migrate."
  Rework the header so that:
  - the **v1→v2** supersede stays true and unchanged in force (v1 is still
    refused; there are no v1 plans to migrate);
  - the absolute "there is no dual-version support" claim is **softened** to
    describe the **transitional coexistence** with v3 — v2 currently coexists
    with plan-format v3 during the v2→v3 transition and retires when the
    **webster: rewrite for flat card list** item lands and `builder` is deleted;
  - a **one-line pointer** to `plan-format-v3.md` (same-directory link) is added.
  Change no other v2 content — the rest of the doc stays a valid, live v2
  contract. Follow `mill:markdown` conventions.
- **Commit:** `4: soften plan-format.md v2 header for v3 coexistence`

### Card 5: move the roadmap plan-format v3 item Planned → Done

- **Context:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, move the item identified by its
  bold name **plan-format v3: flat card list** (find it by that name — line
  numbers drift; it is currently in the `## Planned` section around lines 35–39)
  from `## Planned` to `## Done`. Per the roadmap's own Maintenance rules the
  literal `1.` numbering is fine (ordered lists auto-renumber per section, so no
  number edits are needed anywhere). Requirements:
  - **Remove** the item from `## Planned`. This deletes its current dangling
    `[designs/plan-format-v3.md](designs/plan-format-v3.md)` link (the design doc
    is deleted in batch 1).
  - **Add** it to `## Done` as a one-line entry with a link to the surviving
    reference doc: `[docs/reference/plan-format-v3.md](../docs/reference/plan-format-v3.md)`
    (the up-and-over path from `manifest/`). Linking a Done entry to a surviving
    module/reference doc is explicitly allowed by the roadmap's Maintenance note
    ("Move an item … with a link to its module doc if one exists, when it ships");
    the "Done entries don't link" remark applies only to items whose `designs/`
    doc was *deleted* on landing — v3's reference doc is durable and survives.
  - Placement within `## Done` is not load-bearing (auto-numbering); place it
    with the related plan-format entries (e.g. near the `builder`/`webster` Done
    entries) is fine.
  - **Leave `manifest/roadmap.md` line 53 unchanged** — "converts `discussion.md`
    into a plan-format-v3 card list" is a prose format-name mention in the
    (still-Planned) **loom: Planner producer** item, not a link to the deleted
    doc, and stays accurate.
  - Follow `mill:markdown` conventions.
- **Commit:** `5: move plan-format v3 roadmap item Planned to Done`

### Card 6: add additive v3 cross-links to the neighbour durable docs

- **Context:** none
- **Edits:**
  - `docs/overview.md`
  - `docs/reference/builder-contract.md`
  - `docs/reference/model-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add **additive** v3 cross-links to three durable docs. Every
  edit is a pure addition — no existing v2 wording is changed, relabelled, or
  removed (v2 references are not stale under coexistence).
  - `docs/overview.md`:
    - In the **durable contract/reference docs** list under
      `## Documentation lifecycle` (currently line 104, reading
      `` `status-schema.md`, `discussion-format.md`, `plan-format.md`,
      `builder-contract.md`, `model-spec.md` ``), add `` `plan-format-v3.md` ``
      to the list (bare filename, matching the existing entries' style).
    - Add **one line** noting v3 is the emerging format the (Planned)
      webster-rewrite consumes, linking `[plan-format-v3.md](reference/plan-format-v3.md)`.
      Place it in the **webster** module bullet (currently around lines 303–314,
      the fork-based-sibling bullet), since webster is the consumer-to-be. Leave
      the **builder** bullet (lines 293–302) v2-accurate and untouched.
  - `docs/reference/builder-contract.md`: add **one line** pointing to
    `plan-format-v3.md` as the emerging flat-card format the Planned
    webster-rewrite will consume. Add it to the Related-links list at the bottom
    (currently around line 440, next to the existing
    `- [plan-format.md](plan-format.md) — builder's pinned input contract …`
    entry), as a sibling bullet
    `- [plan-format-v3.md](plan-format-v3.md) — the emerging flat-card format the Planned webster-rewrite will consume.`
    Leave the doc's v2 description otherwise unchanged.
  - `docs/reference/model-spec.md`: in the Status header (currently line 7,
    "Pinned alongside [plan-format v2](plan-format.md) because the plan is
    model-agnostic …"), extend the parenthetical/clause to mention v3 too,
    linking same-directory `[v3](plan-format-v3.md)` (e.g. "Pinned alongside
    [plan-format v2](plan-format.md) — and the emerging
    [v3](plan-format-v3.md) — because the plan is model-agnostic …"). No other
    change.
  - Follow `mill:markdown` conventions.
- **Commit:** `6: add additive v3 cross-links to overview, builder-contract, model-spec`

## Batch Tests

`verify: go build ./...` is a sanity gate only — no Go source is touched.

Verification is by reading plus these grep checks (run from the worktree root
after the cards land):

- **The roadmap's dangling design-doc link is gone:**
  `grep -rn "designs/plan-format-v3.md" manifest/roadmap.md` must return
  **nothing**.
- **Every new cross-link resolves:** the additive links point at
  `docs/reference/plan-format-v3.md` (created in batch 1) — confirm that file
  exists and that each new link uses the correct relative path for its file
  (`reference/plan-format-v3.md` from `docs/overview.md`; `plan-format-v3.md`
  from the `docs/reference/*` siblings; `../docs/reference/plan-format-v3.md`
  from `manifest/roadmap.md`).
- **Repo-wide no-dangling sweep** (covers both batches together): after this
  batch, `grep -rn "](plan-format-v3.md)" docs/ manifest/` returns nothing, and
  every `plan-format-v3.md` reference across `docs/` and `manifest/` resolves to
  the reference doc (or, for `webster-rewrite.md` line 32, a local anchor).
- **v2 stays valid:** `docs/reference/plan-format.md` still describes v2
  truthfully after only its Status-header softening; no neighbour doc
  contradicts it.
