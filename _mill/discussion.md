# Discussion: plan-format v3: flat card list

```yaml
task: 'plan-format v3: flat card list'
slug: plan-format-v3
status: discussing
parent: main
```

## Problem

Today's pinned plan contract, `docs/reference/plan-format.md`, is **plan-format v2**: a
**batch**-based schema (a plan is an ordered sequence of batch files
`_lyx/plan/NN-<batch-slug>.md` + a `00-overview.md` batch index; each batch is one implementer
session). `internal/builderengine.ParsePlan`/`Validate` is *the single real plan parser in the
repo*, implementing v2; `internal/websterengine` imports it directly. **Both `builder` and today's
`webster` are shipped** (roadmap Done list) and consume v2.

The roadmap's next schema item, **plan-format v3: flat card list**, drops the **batch** as a
*plan-schema* concept: the plan's unit becomes the individual **card**. The full v3 design already
exists at `manifest/designs/plan-format-v3.md`. This task **lands v3 as a pinned contract**.

**Landing model — coexistence, not replacement (decided in discussion).** The v3 design doc, and
the roadmap wording, frame v3 as a *breaking replacement* of v2. That was reconsidered: instead,
**v2 and v3 coexist as two pinned reference docs during a transition**. This task promotes the v3
design into a **new** durable reference doc `docs/reference/plan-format-v3.md` and leaves
`docs/reference/plan-format.md` (v2) live and valid. v2 is retired later — when the separate
**webster: rewrite for flat card list** roadmap item lands and `builder` is deleted (`builder`
becomes obsolete per that item; v3 wins). See Decisions → `coexistence-transitional`.

**Why now:** it is the next Planned roadmap item and a hard prerequisite for **webster: rewrite for
flat card list** — webster cannot be rewritten to consume v3 until v3 is a pinned contract.

**Framing note for the plan-writer:** this task changes **only Markdown docs**. No Go. The code
that parses/executes v3 (and eventually retires the v2 parser) is the separate webster-rewrite task.

## Scope

**In:**

- **Create `docs/reference/plan-format-v3.md`** — a new durable reference doc pinning **v3 (flat
  card list)**, promoted from `manifest/designs/plan-format-v3.md`. Content per the Decisions below
  (flat cards; per-card typed file-op fields; `Depends-on:`; per-card + plan-level `verify:`;
  `root:`/`//`; plan-level Rename mechanic; flat `N` numbering; dropped Scope; deferred symbol
  fields; validation-check spec; a rewritten worked example). Include a short **coexistence note** in
  its Status header (coexists with v2; v2 retires when webster-rewrite lands and builder is deleted;
  v3 is the format webster-rewrite consumes).
- **Delete `manifest/designs/plan-format-v3.md`** — folds into the new reference doc per the
  [documentation lifecycle](../../docs/overview.md#documentation-lifecycle) (design doc → durable
  reference doc; the target is the new `plan-format-v3.md`, not v2's file).
- **`docs/reference/plan-format.md` (v2): one softening edit only** — soften its absolute
  "there is no dual-version support" claim (currently in its Status header) to reflect the
  **transitional coexistence** with v3 (until builder retires), and add a one-line pointer to
  `plan-format-v3.md`. Otherwise **untouched** — v2 stays a valid, live contract.
- **Repoint the deleted design doc's inbound links to the new reference doc.** Deleting
  `manifest/designs/plan-format-v3.md` dangles every `[...](plan-format-v3.md)` /
  `[...](designs/plan-format-v3.md)` link. Repoint all of them to `docs/reference/plan-format-v3.md`:
  - `manifest/roadmap.md:39` (the **plan-format v3** Planned item) — handled by the Planned→Done
    move, see `roadmap-planned-to-done`.
  - `manifest/designs/*.md` that link to it: `loom.md`, `loom-planner.md`, `codeintel-redesign.md`,
    `webster-parallel-execution.md` (grep `plan-format-v3.md` to get the exact set at edit time) —
    plain path repoints to `docs/reference/plan-format-v3.md`.
  - **`manifest/designs/webster-rewrite.md` gets special handling** (see
    `symbol-fields-deferred-compact`): the detailed continuous-DAG-update/SCC scheduling design is
    **relocated into** this doc's "Scheduling: no DAG, no SCC merging in v0" section (it would
    otherwise be lost by the trim), and `webster-rewrite.md:32`'s link is repointed to that
    **now-local section anchor**, not to the trimmed `plan-format-v3.md` stub — otherwise the anchor
    fragment `#continuous-dag-update-…` dangles even though the filename resolves.
- **`manifest/roadmap.md`: move the plan-format v3 item Planned → Done**, with a link to the new
  `docs/reference/plan-format-v3.md` (see `roadmap-planned-to-done`).
- **Additive cross-links to v3 in the neighbour durable docs** (NOT reconciliation — v2 stays valid,
  nothing is stale or legacy):
  - `docs/overview.md`: add `plan-format-v3.md` to the durable-reference-docs list (~line 104), and a
    one-line mention (builder section / module map) that v3 is the emerging format webster-rewrite
    consumes. Builder section stays v2-accurate.
  - `docs/reference/builder-contract.md`: a one-line pointer to `plan-format-v3.md` as the emerging
    format for webster-rewrite. Its v2 description stays truthful and unlabelled (v2 is not legacy —
    it coexists).
  - `docs/reference/model-spec.md` (~line 5, "Pinned alongside plan-format v2"): extend to mention
    v3 too.

**Out:**

- **No Go code changes.** `internal/builderengine`, `internal/websterengine`, `internal/buildercli`,
  `internal/webstercli`, their tests and `testdata/` plans are untouched. The v3 parser/validator —
  and any decision about whether the code supports both formats or retires v2 — is webster-rewrite's.
- **No rewrite of `plan-format.md` v2**, beyond the single softening note above. No legacy-labelling
  or "reconciliation" of `builder-contract.md`/`overview.md` — v2 references are not stale, so there
  is nothing to reconcile (this supersedes the earlier replace-and-reconcile plan).
- **No permanent dual-version architecture.** Coexistence is transitional; v2 retires with builder in
  webster-rewrite. This task does not commit the code to supporting two formats forever.
- **No new v3 execution model here.** Fork-per-card runtime, integration-once-at-end, recovery,
  roles/models — all execution policy — is webster-rewrite's "webster-preprocessing" part; not
  written or relocated here.
- No touching the retired `websterv2.md` or the parked `webster-parallel-execution.md` (beyond
  repointing its `plan-format-v3.md` link).
- No changes to `discussion-format.md` / `status-schema.md` — they reference `plan-format.md` (v2),
  which stays valid; no edit needed.

## Decisions

### coexistence-transitional

- Decision: v2 and v3 **coexist as two pinned reference docs** during a transition. v2
  (`plan-format.md`) stays live and valid; v3 (`plan-format-v3.md`) is added new. v2 is retired when
  the **webster: rewrite for flat card list** item lands and `builder` is deleted — v3 wins; the
  coexistence is not permanent.
- Rationale: avoids the dishonest "transitional window" of the replace-in-place approach
  (`plan-format.md` claiming v3 while the shipped code still parses v2); no neighbour doc becomes
  stale or needs legacy-labelling; the roadmap dangling-link problem dissolves.
- Rejected: (a) **replace v2 in place** (the design doc's original intent) — creates a doc that
  pins a contract no shipped code honors until webster-rewrite lands, and forces reconciling every
  neighbour doc. (b) **Permanent coexistence** — contradicts the roadmap ("builder becomes
  obsolete") and the repo's "a version bump is not a dialect" stance, and buys nothing: there are
  **no production plans** to migrate (the Planner isn't built; loom isn't running end-to-end), and
  A/B-testing builder-vs-webster is already explicitly declined in `webster-rewrite.md`.
- Note the invariant this softens: `plan-format.md` currently states "v2 supersedes v1 outright — a
  version bump, not a dialect ... there is no dual-version support." This task **softens** that to
  transitional coexistence with v3 (see `v2-doc-softening-note`), a deliberate, recorded reversal
  for the v2→v3 transition only.

### new-reference-doc-not-replacement

- Decision: v3 lands as a **new file** `docs/reference/plan-format-v3.md`; `plan-format.md` keeps its
  name and its v2 content. Consolidation/renaming (e.g. eventually making `plan-format.md` mean v3)
  is deferred to webster-rewrite, when v2 retires.
- Rationale: non-breaking — every existing `plan-format.md` link stays valid; no churn on v2's
  inbound references.
- Rejected: renaming v2 to `plan-format-v2.md` (breaks existing links for no present benefit).

### v2-doc-softening-note

- Decision: the **only** edit to `plan-format.md` is to soften its Status-header "no dual-version
  support" absolute into a transitional-coexistence statement and add a one-line pointer to
  `plan-format-v3.md`. No other v2 content changes.
- Rationale: keeps v2 truthful under coexistence without rewriting it.
- Rejected: leaving the "no dual-version support" claim as-is (it would now be false); rewriting v2
  more broadly (unnecessary).

### roadmap-planned-to-done

- Decision: move the `manifest/roadmap.md` **plan-format v3: flat card list** item from **Planned to
  Done**, with a link to the new `docs/reference/plan-format-v3.md`. This both records completion
  (per `CLAUDE.md` task-completion) and resolves the `plan-format-v3.md` dangling link (repointed to
  the surviving reference doc — Done entries *may* link to a module doc when it exists, per the
  roadmap's Maintenance note; v3's reference doc is not deleted, so the Done entry links to it).
- Rationale: this task delivers the item (the v3 doc lands); the code that consumes it is the
  separate still-Planned webster-rewrite item, so plan-format v3 being Done while webster-rewrite
  stays Planned is exactly the intended decomposition. The Done-move is this task's own
  doc-completion (committed on the branch, real on merge); it is distinct from any wiki `[done]`
  flip that mill-merge performs.
- Rejected: keeping the item under Planned with a repointed link (the item is delivered — it belongs
  in Done); dropping the link entirely (a surviving reference doc should be linked).

### v3-is-v2-minus-the-batch-layer

- Decision: v3 keeps v2's **per-card grammar essentially verbatim** and removes only the **batch
  wrapper**. The card becomes the top-level unit.
- Rationale: the user wants the typed per-card categorization retained; the batch layer existed only
  to bound implementer context per session, which fork-per-card makes moot.
- Rejected: collapsing every card's footprint into a single flat `changes-files` list (the design
  doc's literal v0 field list).

### keep-typed-fields

- Decision: each card carries v2's five typed file-op fields **`Context:` / `Edits:` / `Creates:` /
  `Deletes:` / `Moves:`** with the exact same grammar as v2 (label line; indented
  single-backtick-wrapped path sub-bullets; the literal `none` sentinel on the label line when
  empty; `Moves:` sub-bullets use the `` `src` -> `dst` `` ASCII-arrow pair). The Rename mechanic
  and `root:`/`//` resolution are kept too.
- Rationale: preserves mechanically-declared rename intent (the repo's `git mv` history-preservation
  convention), the Edits-vs-Creates distinction, and everything the v2 validation checks police.
- Rejected: the design doc's single flat `changes-files` field; a `Moves:`-only compromise.

### changes-files-is-derived-and-named

- Decision: there is **no separate `changes-files` card field** — it is the **derived union** of the
  typed file-op fields (`Edits:` ∪ `Creates:` ∪ `Deletes:` ∪ both `Moves:` endpoints). The v3 doc
  **names** this derived union explicitly (in the Deferred/forward-compat section) as the artifact
  webster's future contract-verification compares actual changed files against, pointing to
  `webster-rewrite.md` for the verification semantics. *(Resolves review NOTE.)*
- Rationale: a separate authored flat list would duplicate and drift from the typed fields; but the
  term `changes-files` appears in the design and in webster-rewrite, so the doc defines it once as
  derived rather than leaving it undefined.
- Rejected: a maintained flat `changes-files` field; omitting any mention of the derivation.

### card-fields-and-order

- Decision: a card is a per-card file `NN-<card-slug>.md` whose content is:
  - Title heading `# Card N — <name>` (`<name>` = design doc's short human-readable card name; used
    in the commit message when no explicit `Commit:` is given).
  - `**What:**` — the card's instruction ("what to build and why"; the design doc's `description`).
    lyx keeps its established `What:` name.
  - `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**` — the five typed
    fields, in this order, all required (`none` sentinel when empty).
  - `**Depends-on:**` — **new required field**, after `Moves:`. Value is a list of card ids (plain
    card numbers `N`), or `none`. See `depends-on-in-v0`.
  - Optional `**Commit:**` (must start with the card's own `N: ` prefix) and optional `**verify:**`.
- Rationale: mirrors v2's card field set/order so the grammar and validation carry over; adds only
  `Depends-on:` and swaps `NN.C` numbering for flat `N`.
- Rejected: reordering fields; making `Depends-on:` optional.

### on-disk-layout

- Decision: `_lyx/plan/` directory (unchanged path) with:
  - `00-overview.md` — frontmatter (`format: 3`, `approved: true`, optional plan-level `root:`), a
    short task-framing paragraph, an ordered **Card Index** (`N — <card-slug> — <one-line intent>`),
    and the optional plan-level body sections: `## Shared Decisions`, `## Rename mechanic`, and a
    `## verify:` section (the plan-level integration verify — see `verify-model`).
  - `NN-<card-slug>.md` — **one file per card** (`NN` = zero-padded card order); the file *is* the
    card.
- Rationale: a single all-cards file is unwieldy on large (40+ card) plans; per-card files keep each
  card independently diffable/reviewable. This is v2's structure with the **card** as the file unit
  instead of the batch. Card Index ↔ card files are cross-checked mechanically.
- Rejected: one single `_lyx/plan.md` holding all cards; keeping batch-slug files.

### numbering-and-commit-subject

- Decision: cards are numbered flat **`N` (1..N)** across the whole plan. Heading id is `N`; the
  per-card file prefix `NN` (zero-padded) must equal it. **The default commit subject is
  `N: <name>`** — the card's heading `<name>` (honoring the design doc's "name used in the commit
  message"); there is no separate `<short what>` string. An explicit `**Commit:**` field overrides
  the default but must start with the card's own `N: ` prefix (`commit-subject-mismatch` check).
- Rationale: batch is gone, so the `NN.C` composite id collapses to `N`; still keyed to the git-log
  resume trail. Seeding the default from the single card `<name>` removes the v2 ambiguity where
  heading title and commit text could diverge — there is now exactly one source for the default.
- Rejected: keeping `NN.C`; a distinct `<short what>` seed separate from `<name>` (the doubly-specified
  default the round-2 review flagged); using the card `name` as the numeric card id.

### verify-model

- Decision: `verify:` is **optional per-card** plus an **optional plan-level integration verify**.
  The plan-level integration verify lives as a **`## verify:` body section in `00-overview.md`**
  (mirroring v2, where the actual command lived in a `## verify:` body section; frontmatter stays
  scalar-only: `format`/`approved`/`root`). There is **no mandatory per-batch verify gate** (batch is
  gone). The build+unit-test gate is implicit in the card definition ("compiles on its own" +
  "bundles its own test"), which the consumer (webster) runs after each card; the plan-level
  `## verify:` expresses the single integration suite run once at the end. *(Placement resolves
  review NOTE.)*
- Rationale: matches the card definition and webster-rewrite's "unit tests after every card,
  integration suite once at end"; avoids forcing a declared command on every card.
- Rejected: mandatory per-card `verify:`; porting v2's mandatory per-batch verify; a frontmatter
  `verify:` key (commands belong in a body section, per v2).

### scope-dropped

- Decision: the v2 per-batch `## Scope` concept is **removed entirely**. A card's own typed file-op
  fields *are* its declared footprint.
- Rationale: with fork-per-card, each card already declares what it touches; webster-rewrite treats a
  `changes-files` mismatch as informational, never blocking, so a separate ownership fence adds no
  enforcement value. Removes the `card-outside-scope` check and scope-drift.
- Rejected: a plan-level Scope list every card must fall under.

### root-is-plan-level

- Decision: `root:` becomes an **optional plan-level** field (in `00-overview.md` frontmatter); the
  `//` worktree-root escape and the parse-time normalization rules carry over from v2 unchanged.
- Rationale: simplest port now that the batch (which owned `root:` in v2) is gone; harmless when
  unset.
- Rejected: per-card `root:`; dropping `root:`.

### rename-mechanic-plan-level

- Decision: one **plan-level `## Rename mechanic`** section (in `00-overview.md`), **required when
  any card in the plan has a non-empty `Moves:`**. Its canonical `git mv`-then-surgical-edits text is
  reproduced verbatim (adjusted only for the paths involved), exactly as v2 pins it.
- Rationale: the mechanic is plan-wide boilerplate; repeating it per Moves-card is needless
  duplication now that there is no batch to scope it to.
- Rejected: per-card Rename mechanic sections.

### output-report-punted

- Decision: `plan-format-v3.md` pins **only the plan (input schema)**. The output/report contract
  (v2's `NN-<batch-slug>.yaml` batch-report; v3's fork-return "OK, SHA X" / deviation note) is **not**
  in this doc — it is execution territory for the consumer doc (`builder-contract.md` /
  webster-rewrite's fold).
- Rationale: cleaner input-vs-output separation; the report shape is webster-rewrite's to define.
- Rejected: porting v2's batch-report into a per-card report schema inside `plan-format-v3.md`.

### strip-execution-policy

- Decision: the v3 doc carries **no execution-policy content**: no batch-sizing "Principle #0", no
  `oversized: true`, no deferred-verify chains, no red-tests/recovery, no review cadence, no
  roles/models discussion. It keeps the **"plan vs schedule"** principle and **`depends-on`** (the
  DAG-of-intent), which are schema philosophy.
- Rationale: oversized/chains die with the batch; recovery/review-cadence/roles are consumer
  (webster) decisions. All of it lands in webster-rewrite's "webster-preprocessing" part; this task
  does not write or relocate it.
- Rejected: keeping a trimmed execution overview (risks overlap with builder-contract/webster).

### depends-on-in-v0

- Decision: `Depends-on:` ships in v0 (unlike the symbol fields). It references only other cards in
  the same plan (card ids `N`). A mechanical check, **`depends-on-order`**, flags a card whose
  `Depends-on:` names a *later* card (or itself), and a `Depends-on:` id referencing no existing card.
- Rationale: `depends-on` carries no hallucination risk (never a claim about external code) and is a
  cheap, LLM-free, pre-review order-validation gate; human-readable escalation context; and
  forward-compatible input for the future codeintel-derived DAG.
- Rejected: deferring `depends-on` with the symbol fields.

### symbol-fields-deferred-compact

- Decision: keep only a **short "Deferred / forward-compat"** section in `plan-format-v3.md`: symbol
  fields (`creates-symbols`/`edits-symbols`/`reads-symbols`) are **deliberately omitted in v0**
  (waiting on codeintel), with a compact summary and pointers to `codeintel-redesign.md` and
  `webster-rewrite.md`. This section is also where the derived `changes-files` union is named (see
  `changes-files-is-derived-and-named`).
- **Relocate, do not drop, the detailed design.** The detailed Mechanism-1/2 / continuous-DAG-update
  (Kahn-style greedy topological selection) / SCC-merging (Tarjan) design currently lives **only** in
  `manifest/designs/plan-format-v3.md` — trimming it to a stub would *lose* it, since
  `webster-rewrite.md` today only *points* at it (`webster-rewrite.md:32`), it does not contain it.
  That machinery is **scheduling/execution** (how webster incrementally updates the DAG and merges
  cyclic card groups as cards land), which by `strip-execution-policy` does not belong in the schema
  doc. So **move the detailed design into `webster-rewrite.md`** — fold it into that doc's existing
  "Scheduling: no DAG, no SCC merging in v0" section — and **repoint `webster-rewrite.md:32`'s link to
  that now-local section anchor** (not to the trimmed `plan-format-v3.md` stub, which would leave a
  dead anchor fragment). *(Resolves round-2 gap.)*
- Rationale: the pinned contract carries no non-v0 dead-code scheduling design; the design is
  preserved in the doc that will implement it; and no inbound anchor dangles.
- Rejected: porting the full deferred design into `plan-format-v3.md` (violates `strip-execution-policy`;
  long "deferred" section); dropping the detailed design entirely (loses the incremental-DAG/SCC
  reasoning); repointing only the file path and leaving the `#continuous-dag-update-…` anchor dead.

### validation-checks-enumerated

- Decision: the v3 doc enumerates the machine checks it is designed to support (they land with
  webster-rewrite, not this doc — same "spec for the future validator" posture v2 uses), adapting
  v2's list to the flat-card model:
  - **Kept/adapted:** `format-unrecognized`/`plan-unapproved`; `index-file-mismatch` (Card Index ↔
    card files: numbering, slugs, no gaps, no orphaned file — **absorbs** v2's `card-count-mismatch`);
    `card-path-malformed` (every card path normalized, relative, clean, no `..`; `root:`/`//`
    resolution is part of "normalized" — v2's `scope-malformed` renamed now that Scope is gone);
    `move-format`; `move-redundant`; `move-source-missing`; `move-target-collision`;
    `move-mechanic-missing` (now plan-level); `card-missing-field` (including `Depends-on:`);
    `card-field-overlap`; `card-numbering` (flat `N` runs 1..M, no gaps/dups; file prefix `NN`
    matches heading `N`); `path-missing`; `commit-subject-mismatch` (present `Commit:` must start
    with `N: `).
  - **New:** `depends-on-order` (see `depends-on-in-v0`).
  - **Dropped:** `verify-missing` (no mandatory verify gate), `chain-end-dangling` (chains gone),
    `batch-oversized` (oversized gone), `card-outside-scope` (Scope gone), `card-count-mismatch`
    (folded into `index-file-mismatch`).
- Rationale: keeps the mechanical-check discipline v2 established while shedding batch/scope/chain
  concepts that no longer exist.
- Rejected: deferring the whole check list to webster-rewrite with no spec in the doc.

### worked-example-rewritten

- Decision: rewrite v2's worked example (the `--json` flag on `lyx board list`) for v3: per-card
  files under `_lyx/plan/`, a `00-overview.md` with Card Index + a `## Shared Decisions` entry +
  `root:` + a `## Rename mechanic` section + a `## verify:` section, flat `N` card numbering, a
  `Depends-on:` field, and a `Moves:` card. Must stay byte-consistent across Card Index ↔ filenames.
- Rationale: the pinned contract carries a complete, self-consistent example, as v2 does.
- Rejected: dropping the worked example.

## Technical context

Everything in scope is **Markdown docs** under `docs/` and `manifest/`. No Go.

Files to change:

- **Create** `docs/reference/plan-format-v3.md` — the v3 reference doc. Source: read
  `manifest/designs/plan-format-v3.md` in full (authoritative design) and adapt v2's card-level prose
  from `docs/reference/plan-format.md` (sections "Card", "Card path resolution", "Moves and the
  Rename mechanic", the `none`-sentinel rules, the worked example) — v3 is v2-minus-batch, so most
  card-level prose is reusable with batch framing removed and numbering `NN.C` → `N`.
- **Delete** `manifest/designs/plan-format-v3.md` (git rm).
- **Edit** `docs/reference/plan-format.md` — the single softening note only (`v2-doc-softening-note`).
- **Edit** `manifest/roadmap.md` — Planned→Done move of the plan-format v3 item, linking to
  `docs/reference/plan-format-v3.md` (`roadmap-planned-to-done`). This is the roadmap:39 link
  repoint.
- **Edit** the `manifest/designs/*.md` files that link to `plan-format-v3.md` — repoint each to
  `docs/reference/plan-format-v3.md`. Known set (verify by grep at edit time): `loom.md`,
  `loom-planner.md`, `codeintel-redesign.md`, `webster-parallel-execution.md`. Note these currently
  use a same-directory link `plan-format-v3.md`; the new target is up-and-over
  (`../../docs/reference/plan-format-v3.md` from `manifest/designs/`).
- **Edit** `manifest/designs/webster-rewrite.md` — (a) **relocate** the detailed
  continuous-DAG-update/Mechanism-1/2/SCC-merging design out of the promoted `plan-format-v3.md` into
  this doc's "Scheduling: no DAG, no SCC merging in v0" section; (b) repoint its line-32 link's
  **anchor** to that now-local section (not the trimmed stub). See `symbol-fields-deferred-compact`.
- **Edit** `docs/overview.md` — add `plan-format-v3.md` to the durable-reference-docs list
  (~line 104) + a one-line "v3 is the emerging format webster-rewrite consumes" mention; builder
  section stays v2-accurate.
- **Edit** `docs/reference/builder-contract.md` — one-line pointer to `plan-format-v3.md`;
  v2 description otherwise unchanged.
- **Edit** `docs/reference/model-spec.md` (~line 5) — mention v3 alongside v2.

Verification anchor (the reframe that builder/webster are shipped, not future): `internal/builderengine/doc.go`
("pinned plan-format v2 plan… batch by batch"), `internal/websterengine/doc.go` ("parsed by the same
builderengine.ParsePlan"), `manifest/roadmap.md` Done entries for `builder` and `webster`.

## Constraints

From `CONSTRAINTS.md` and `CLAUDE.md`, relevant here:

- **Documentation Lifecycle** (`CONSTRAINTS.md` → `## Documentation Lifecycle`; detail in
  `docs/overview.md#documentation-lifecycle`): design docs (`manifest/designs/*.md`) are deleted when
  the thing lands; durable reference docs (`docs/reference/*.md`) are kept. This task performs that
  event for `plan-format-v3.md` (design) → `docs/reference/plan-format-v3.md` (durable). `docs/overview.md`'s
  durable-reference-docs list must gain the new file.
- **Task completion** (`CLAUDE.md`): a change touching a named module's design updates docs in the
  same commit — here the deliverable *is* docs. Completing a planned item means moving it Planned→Done
  in `manifest/roadmap.md` (`roadmap-planned-to-done`).
- **No cross-cutting code invariants apply** (Hub Geometry, lyxtest Leaf, CLI/Cobra) — no Go changes.
- **Markdown style**: follow `mill:markdown` conventions for every edited/created `.md`.

## Testing

Documentation-only task — no runtime behaviour to exercise, no new code to unit-test. Verification:

- `go build ./...` and the existing Go test suite **still pass unchanged** — a sanity check that no
  code was touched (v2 parser/validator and its `testdata/` plans stay exactly as they are).
- **Internal consistency of the new `plan-format-v3.md`**: the worked example must be byte-consistent
  across Card Index ↔ per-card filenames ↔ card headings/numbering (checked by reading; no validator
  runs it).
- **No dangling links, including anchor fragments** (the core mechanical check for this task): after
  deleting `manifest/designs/plan-format-v3.md`, grep `plan-format-v3.md` across the repo — every
  inbound link must now resolve to `docs/reference/plan-format-v3.md` (roadmap item + the
  `manifest/designs/*.md` set). Crucially, verify **anchor fragments** too, not just filenames: any
  `plan-format-v3.md#…` link into a section that was trimmed (e.g. `webster-rewrite.md:32`'s
  `#continuous-dag-update-…`) must be retargeted to a section that still exists — a filename-only
  grep passes while the anchor is dead. Also grep `plan-format` broadly to confirm nothing else
  dangles.
- **v2 stays valid**: `plan-format.md` (v2) still describes v2 truthfully after its one softening
  note; no neighbour doc contradicts it. `builder-contract.md`/`overview.md` remain v2-accurate with
  only additive v3 cross-links.

No TDD candidates (no code). No CLI surface changes, so no help-tree pins to update.

## Q&A log

- **Q:** Does this task rewrite the Go code (`builderengine`/`websterengine`) to consume v3?
  **A:** No. Only the schema/doc lands here; the code rewrite (and any decision to retire the v2
  parser) is the separate later **webster-rewrite** roadmap item.
- **Q:** Should v2 be replaced by v3, or should the two coexist? **A:** **Coexist** — v3 lands as a
  new pinned reference doc `docs/reference/plan-format-v3.md`; `plan-format.md` (v2) stays live and
  valid. The transition is **gradual and transitional**: v2 retires when webster-rewrite lands and
  builder is deleted (v3 wins). Chosen over replace-in-place (avoids the "doc claims v3 while code is
  v2" dishonesty and all neighbour-doc reconciliation) and over permanent coexistence (contradicts
  "builder becomes obsolete"; no installed base to migrate).
- **Q:** Naming/layout for the two docs? **A:** Keep `plan-format.md` = v2 (untouched but one
  softening note), add `plan-format-v3.md` = v3. Non-breaking; consolidation deferred to
  webster-rewrite.
- **Q:** How is `manifest/roadmap.md:39`'s link to the deleted design doc resolved? **A:** By moving
  the plan-format v3 item Planned→Done with a link to the new `docs/reference/plan-format-v3.md`
  (records completion + repoints the link).
- **Q:** Does v3 collapse the per-card footprint to a single `changes-files` list? **A:** No — keep
  the typed split `Context:`/`Edits:`/`Creates:`/`Deletes:`/`Moves:`, plus Rename mechanic and
  `root:`. `changes-files` is the derived union, **named** in the doc's Deferred/forward-compat
  section, not an authored field.
- **Q:** On-disk layout — one file or per-card files? **A:** Per-card files `NN-<card-slug>.md` under
  `_lyx/plan/` plus a `00-overview.md` index (a single all-cards file was judged too long/overkill).
- **Q:** `verify:` model and the plan-level integration verify's placement? **A:** Optional per-card
  `verify:` + an optional plan-level integration verify as a **`## verify:` body section in
  `00-overview.md`** (frontmatter stays scalar-only); no mandatory per-batch gate.
- **Q:** Scope? **A:** Dropped entirely — the card's typed fields are its footprint.
- **Q:** `root:` placement? **A:** Plan-level (in `00-overview.md` frontmatter), `//` escape retained.
- **Q:** Rename mechanic placement? **A:** One plan-level `## Rename mechanic` section, required when
  any card has a non-empty `Moves:`.
- **Q:** Card numbering / commit subject? **A:** Flat `N` (1..N); commit `N: <short what>`; file
  prefix `NN` matches `N`.
- **Q:** Output/report schema in the doc? **A:** Punted to the consumer doc; `plan-format-v3.md` pins
  only the input plan schema.
- **Q:** Execution-policy sections (roles/models, review cadence, oversized, chains)? **A:** Not in
  the v3 doc — that info lands in webster-rewrite's "webster-preprocessing" part; not relocated here.
- **Q:** The deferred symbol-field/DAG design? **A:** A short "Deferred / forward-compat" section
  (symbol fields omitted in v0, pending codeintel) with pointers to `codeintel-redesign.md` and
  `webster-rewrite.md`.
- **Q:** Where does the detailed continuous-DAG-update/SCC-merging design go when it's trimmed from
  the promoted doc (it lives only there, and `webster-rewrite.md:32` links into it)? **A:** Relocate
  it into `webster-rewrite.md`'s scheduling section (it is scheduling/execution, not schema) and
  repoint that link's anchor to the now-local section — do not just repoint the file path, which
  would leave a dead `#continuous-dag-update-…` anchor.
- **Q:** Which string seeds the default commit subject (heading `<name>` vs a `<short what>`)?
  **A:** The card `<name>` — default subject is `N: <name>`; there is no separate `<short what>`. An
  explicit `Commit:` overrides but must start with `N: `.
