# Discussion: plan-format v3: flat card list

```yaml
task: 'plan-format v3: flat card list'
slug: plan-format-v3
status: discussing
parent: main
```

## Problem

Today's pinned plan contract, `docs/reference/plan-format.md`, is **plan-format v2**:
a **batch**-based schema where a plan is an ordered sequence of batch files
(`_lyx/plan/NN-<batch-slug>.md` + a `00-overview.md` batch index), each batch being one
implementer session. The roadmap's next schema decision (`manifest/roadmap.md`, Planned item
**plan-format v3: flat card list**) drops the batch entirely as a *plan-schema* concept: the
plan's unit becomes the individual **card**. Batching, if it ever happens, becomes a
webster-internal execution optimization — never something the plan format expresses.

The v3 design already exists in full at `manifest/designs/plan-format-v3.md`. This task turns
that design into the shipped, pinned contract and retires the design doc, per the repo's
[documentation lifecycle](../../docs/overview.md#documentation-lifecycle) (design docs fold into
their durable reference doc and are deleted when the thing lands).

**Why now:** it is the next Planned roadmap item and a hard prerequisite for the item after it,
**webster: rewrite for flat card list** — webster cannot be rewritten to consume v3 until v3 is a
pinned contract. This is a deliberate **breaking change to an already-shipped contract**, not
filling an empty gap.

**Critical framing correction discovered during exploration.** Both the v2 reference doc and the
v3 design doc are written as if `builder` is a *future, not-yet-built* consumer. That is stale.
`builder` and today's `webster` are **both shipped** (roadmap Done list), and
`internal/builderengine.ParsePlan`/`Validate` is *the single real plan parser in the repo*,
implementing all of v2's batch grammar and validation checks; `internal/websterengine` imports it
directly. **This task does not touch that code.** The Go rewrite that actually parses/executes v3
is the separate, later **webster-rewrite** task, at which point `builder` becomes obsolete. So
there is a deliberate transitional window: after this task, `plan-format.md` describes v3 while the
shipped code still consumes v2. That window is handled honestly in the doc set (see Decisions).

## Scope

**In:**

- Rewrite `docs/reference/plan-format.md` from v2 (batch-based) to **v3 (flat card list)**, keeping
  per-card typed file-op fields, the Rename mechanic, and `root:`/`//` path resolution; dropping
  batch as a plan-schema concept. Include a transitional Status note (see Decisions →
  `transitional-status`).
- Delete `manifest/designs/plan-format-v3.md` (folds into the reference doc per the documentation
  lifecycle).
- Reconcile **every** durable reference so nothing points to a stale "v2/old format":
  - `docs/reference/builder-contract.md` and the `docs/overview.md` **builder** section: keep their
    truthful description of the still-live v2 runtime, but **label it legacy/superseded by v3 /
    webster-rewrite**, and reword their `plan-format.md` cross-references so they no longer imply
    `plan-format.md` defines what `builder` parses.
  - `docs/reference/model-spec.md` ("Pinned alongside plan-format **v2**"),
    `docs/reference/discussion-format.md`, `docs/reference/status-schema.md`, and the
    `docs/overview.md` module table: update the "v2" mentions to v3 / neutral so no cross-reference
    is stale.

**Out:**

- **No Go code changes.** `internal/builderengine`, `internal/websterengine`, `internal/buildercli`,
  `internal/webstercli`, their tests and `testdata/` plans are untouched. The v3 parser/validator is
  webster-rewrite's job.
- **No new v3 execution model here.** The v3 fork-per-card runtime, integration-suite-once-at-end,
  recovery, roles/models — all execution policy — is webster-rewrite's "webster-preprocessing" part.
  This task only strips that content from `plan-format.md`; it does **not** relocate it anywhere.
- No changes to `manifest/roadmap.md`'s Planned/Done ordering (this item stays Planned until it
  actually lands; do not move it — the roadmap flip is the finalization step's call, and per
  CLAUDE.md the roadmap records planned goals, and this *is* completing a planned item — see
  Testing/Task-completion note below).
- No touching the `websterv2.md` retired design or the parked `webster-parallel-execution.md`.

## Decisions

### v3-is-v2-minus-the-batch-layer

- Decision: v3 keeps v2's **per-card grammar essentially verbatim** and removes only the **batch
  wrapper**. The card becomes the top-level unit.
- Rationale: the user wants the typed per-card categorization retained (see `keep-typed-fields`);
  the batch layer existed only to bound implementer context per session, which fork-per-card makes
  moot.
- Rejected: collapsing every card's footprint into a single flat `changes-files` list (the design
  doc's literal v0 field list) — the user explicitly wants the typed split kept.

### keep-typed-fields

- Decision: each card carries v2's five typed file-op fields **`Context:` / `Edits:` / `Creates:` /
  `Deletes:` / `Moves:`** with the exact same grammar as v2 (label line; one or more indented
  single-backtick-wrapped path sub-bullets; the literal `none` sentinel on the label line when
  empty; `Moves:` sub-bullets use the `` `src` -> `dst` `` ASCII-arrow pair). The Rename mechanic
  and `root:`/`//` path resolution are kept too.
- Rationale: preserves mechanically-declared rename intent (the repo's `git mv` history-preservation
  convention), the Edits-vs-Creates distinction, and everything the v2 validation checks police.
- Rejected: design doc's single flat `changes-files` field; `Moves:`-only compromise.

### changes-files-is-derived-not-a-field

- Decision: there is **no separate `changes-files` card field**. The design brief's `changes-files`
  is realized as the **union of the typed file-op fields** (`Edits:` ∪ `Creates:` ∪ `Deletes:` ∪
  both `Moves:` endpoints), derived, never authored separately.
- Rationale: a separate flat list duplicating the typed fields is derivative bloat that can drift
  from them (the same reasoning v2 used to reject mill's "All Files Touched" overview section).
- Rejected: a maintained flat `changes-files` field alongside the typed fields.

### card-fields-and-order

- Decision: a card is a per-card file `NN-<card-slug>.md` whose content is:
  - Title heading `# Card N — <name>` (`<name>` = design doc's short human-readable card name; used
    in the commit message when no explicit `Commit:` is given).
  - `**What:**` — the card's instruction ("what to build and why"; this is the design doc's
    `description`). lyx keeps its established `What:` name (as v2 already documents).
  - `**Context:**`, `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**` — the five typed
    fields, in this order, all required (`none` sentinel when empty).
  - `**Depends-on:**` — **new required field**, after `Moves:`. Value is a list of card ids this
    card depends on (the ids are plain card numbers `N`), or `none`. See `depends-on-in-v0`.
  - Optional `**Commit:**` (pins the exact commit subject; must start with the card's own `N: `
    prefix) and optional `**verify:**` (per-card cheap check).
- Rationale: mirrors v2's card field set and ordering so the format reads familiarly and the v2
  grammar/validation carry over; adds only `Depends-on:` and swaps `NN.C` numbering for flat `N`.
- Rejected: reordering fields; making `Depends-on:` optional (an omitted field is
  indistinguishable from a forgotten one — same `none`-sentinel reasoning v2 gives for the typed
  fields).

### on-disk-layout

- Decision: `_lyx/plan/` directory (unchanged path) with:
  - `00-overview.md` — frontmatter (`format: 3`, `approved: true`), a short task-framing paragraph,
    an ordered **Card Index** (`N — <card-slug> — <one-line intent>`), and the optional plan-level
    sections: `## Shared Decisions`, `## Rename mechanic`, a plan-level `root:` (in frontmatter),
    and a plan-level integration `verify:`.
  - `NN-<card-slug>.md` — **one file per card** (`NN` = zero-padded card order); the file *is* the
    card.
- Rationale: a single all-cards file becomes unwieldy on large (40+ card) plans; per-card files keep
  each card independently diffable/reviewable and let the planner write many small files. This is v2's
  structure with the **card** as the file unit instead of the batch.
- Rejected: one single `_lyx/plan.md` holding all cards (user judged it too long/overkill); keeping
  batch-slug files.

### numbering-and-commit-subject

- Decision: cards are numbered flat **`N` (1..N)** across the whole plan. The card heading id is `N`;
  the per-card file prefix `NN` (zero-padded) must equal it; the commit subject is `N: <short what>`.
- Rationale: batch is gone, so the `NN.C` (batch.card) composite id collapses to a plain `N`; still
  keyed to the git-log resume trail exactly as v2's scheme was.
- Rejected: keeping `NN.C`; using the card `name` as the commit id.

### verify-model

- Decision: `verify:` is **optional per-card** plus an **optional plan-level integration `verify:`**
  (in `00-overview.md`). There is **no mandatory per-batch verify gate** (batch is gone). The
  build+unit-test gate is implicit in the card definition ("compiles on its own" + "bundles its own
  test"), which the consumer (webster) runs after each card; the plan-level `verify:` expresses the
  single integration suite run once at the end.
- Rationale: matches the design's card definition and webster-rewrite's "unit tests after every
  card, integration suite once at end" model; avoids forcing a declared command on every card.
- Rejected: mandatory per-card `verify:`; porting v2's mandatory per-batch verify.

### scope-dropped

- Decision: the v2 per-batch `## Scope` concept is **removed entirely**. A card's own typed file-op
  fields *are* its declared footprint.
- Rationale: with fork-per-card, each card already declares what it touches; webster-rewrite treats
  a `changes-files` mismatch as informational, never blocking, so a separate ownership fence adds no
  enforcement value. Removes the `card-outside-scope` check and scope-drift.
- Rejected: a plan-level Scope list every card must fall under (less meaningful when each card
  already declares its own footprint).

### root-is-plan-level

- Decision: `root:` becomes an **optional plan-level** field (in `00-overview.md` frontmatter); the
  `//` worktree-root escape and the parse-time normalization rules carry over from v2 unchanged.
- Rationale: simplest port now that the batch (which owned `root:` in v2) is gone; harmless when
  unset.
- Rejected: per-card `root:` (little benefit when a card touches few files); dropping `root:` (user
  wants it kept).

### rename-mechanic-plan-level

- Decision: one **plan-level `## Rename mechanic`** section (in `00-overview.md`), **required when
  any card in the plan has a non-empty `Moves:`**. Its canonical `git mv`-then-surgical-edits text
  is reproduced verbatim (adjusted only for the paths involved), exactly as v2 pins it.
- Rationale: the mechanic is plan-wide boilerplate; repeating it per Moves-card is needless
  duplication now that there is no batch to scope it to.
- Rejected: per-card Rename mechanic sections.

### output-report-punted

- Decision: `plan-format.md` pins **only the plan (input schema)**. The output/report contract
  (v2's on-disk `NN-<batch-slug>.yaml` batch-report; v3's fork-return "OK, SHA X" / deviation note)
  is **not** part of this doc — it is execution territory that belongs in the consumer doc
  (`builder-contract.md` / webster-rewrite's fold).
- Rationale: cleaner input-vs-output separation; the report shape is a consumer concern that
  webster-rewrite defines.
- Rejected: porting v2's batch-report into a per-card report schema inside `plan-format.md`.

### strip-execution-policy

- Decision: strip **all execution-policy content** from `plan-format.md`: batch-sizing "Principle
  #0", `oversized: true`, deferred-verify chains, red-tests/recovery, review cadence, and the
  roles/models discussion. Keep the **"plan vs schedule"** principle and **`depends-on`** (the
  DAG-of-intent), which are schema philosophy, not execution.
- Rationale: oversized/chains die with the batch; recovery/review-cadence/roles are consumer
  (webster) decisions, not plan-schema. All of this lands in webster-rewrite's "webster-preprocessing"
  part later; this task does not relocate it.
- Rejected: keeping a trimmed execution overview (risks overlap with builder-contract/webster).

### depends-on-in-v0

- Decision: `Depends-on:` ships in v0 (unlike the symbol fields). It references only other cards
  within the same plan (card ids `N`). A new mechanical check, **`depends-on-order`**, flags a card
  whose `Depends-on:` names a *later* card (or itself), and a `Depends-on:` id that references no
  existing card.
- Rationale: `depends-on` carries no hallucination risk (never a claim about external code) and is a
  cheap, LLM-free, pre-review order-validation gate; it is human-readable escalation context and
  forward-compatible input for the future codeintel-derived DAG. (Straight from the design doc's
  reasoning.)
- Rejected: deferring `depends-on` with the symbol fields.

### symbol-fields-deferred-compact

- Decision: keep only a **short "Deferred / forward-compat"** section in `plan-format.md`: symbol
  fields (`creates-symbols`/`edits-symbols`/`reads-symbols`) are **deliberately omitted in v0**
  (waiting on codeintel), with a compact one-paragraph summary and pointers to
  `codeintel-redesign.md` and `webster-rewrite.md`. Trim the detailed Mechanism-1/2 /
  continuous-DAG-update / SCC-merging design (it already lives in those two docs).
- Rationale: the pinned contract should not be weighed down by non-v0 dead-code design; the detail
  is preserved elsewhere and survives the design-doc deletion.
- Rejected: porting the full deferred design into `plan-format.md`.

### validation-checks-enumerated

- Decision: the v3 doc enumerates the machine checks it is designed to support (they land with
  webster-rewrite, not this doc — same "spec for the future validator" posture v2 uses), adapting v2's
  list to the flat-card model:
  - **Kept/adapted:** `format-unrecognized`/`plan-unapproved`; `index-file-mismatch` (Card Index ↔
    card files: numbering, slugs, no gaps, no orphaned file — this **absorbs** v2's
    `card-count-mismatch`); `card-path-malformed` (every card path normalized, relative, clean, no
    `..`; `root:`/`//` resolution is part of "normalized" — this is v2's `scope-malformed` renamed
    now that Scope is gone); `move-format`; `move-redundant`; `move-source-missing`;
    `move-target-collision`; `move-mechanic-missing` (now plan-level); `card-missing-field`
    (including `Depends-on:`); `card-field-overlap`; `card-numbering` (flat `N` runs 1..M, no
    gaps/dups; file prefix `NN` matches heading `N`); `path-missing`; `commit-subject-mismatch`
    (present `Commit:` must start with `N: `).
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
  `root:` + a `## Rename mechanic` section, flat `N` card numbering, a `Depends-on:` field, a
  `Moves:` card, and a pinned `Commit:`. It must stay byte-consistent across Card Index ↔ filenames.
- Rationale: the pinned contract carries a complete, self-consistent example, as v2 does.
- Rejected: dropping the worked example.

### transitional-status

- Decision: `plan-format.md`'s Status header states: it pins **v3** (the target schema); v3
  supersedes v2; the **shipped code (`builder` + today's `webster`) still consumes v2** until the
  webster-rewrite task lands; and it points a reader to `builder-contract.md` for the legacy v2
  runtime. It is consumer-neutral about who executes v3 (webster is the intended consumer; builder
  is obsolete). Keep the documentation-lifecycle note that this replaces v2 and that the design doc
  is deleted.
- Rationale: honest in both directions — the doc does not pretend the code is already on v3, and no
  cross-reference implies `plan-format.md` still describes v2.
- Rejected: silently flipping to v3 with no transitional note; deferring the reference flip to
  webster-rewrite (contradicts the roadmap wording that this item replaces v2).

### reconcile-all-references-no-stale-v2

- Decision: update **every** durable doc that references the old format so none is stale, per the
  legacy-labelling approach (user: "Oppdater ALLE referanser. Jeg vil ikke ha stale referanser til
  gammelt format"):
  - `docs/reference/builder-contract.md`: it truthfully documents the **v2**-consuming `builder`
    runtime (batch-by-batch, `NN-<batch-slug>` reports, `builderengine.ParsePlan`). **Keep that
    description truthful** but mark it explicitly **legacy / superseded by v3 (webster-rewrite)**,
    and reword its `plan-format.md` cross-references so they no longer imply `plan-format.md` defines
    the format `builder` parses (it now pins v3; builder parses v2).
  - `docs/overview.md` **builder** section (currently "pinned plan-format **v2** plan, batch by
    batch", ✅ Implemented, links to `plan-format.md`): same legacy/superseded label; reword the
    plan-format cross-reference.
  - `docs/reference/model-spec.md` (line ~5, "Pinned alongside plan-format **v2**"): → v3.
  - `docs/reference/discussion-format.md` (references plan-format's `approved:`/`format:` fields and
    "`lyx builder run`"): v3 still has `format:`/`approved:`, so the substantive point holds — just
    ensure no wording is stale (e.g. the `builder run` aside can stay factual about the legacy
    runtime or go neutral).
  - `docs/reference/status-schema.md` (references `plan-format.md` needing a `format` field): v3
    still has `format:` — reword any "v2"-specific phrasing to neutral.
  - `docs/overview.md` module table row for builder / any `plan-format v2` string: → neutral/v3.
- Rationale: the only way to remove every misleading v2 reference **without lying about the code**
  (which stays v2) is to label the code-documenting docs as legacy and neutralize the passing
  cross-references.
- Rejected: (a) leaving neighbours untouched (knowingly self-contradictory doc set); (b) converting
  `builder-contract.md`/overview to *describe* v3 (would falsely claim the code parses v3);
  (c) deprecating/removing `builder-contract.md` (it is a *kept* durable doc; larger than "update
  references" and not wanted).

## Technical context

Everything in scope is **Markdown docs** under `docs/` and `manifest/`. No Go.

Files to change:

- `docs/reference/plan-format.md` — full rewrite v2 → v3. Source of the v3 design:
  `manifest/designs/plan-format-v3.md` (read it in full; it is the authoritative design). Keep v2's
  card-grammar prose (`docs/reference/plan-format.md` sections "Card", "Card path resolution",
  "Moves and the Rename mechanic", the `none`-sentinel rules, the worked example) as the base to
  adapt from — v3 is v2-minus-batch, so most card-level prose is reusable with the batch framing
  removed and numbering changed `NN.C` → `N`.
- `manifest/designs/plan-format-v3.md` — **delete** (git rm).
- `docs/reference/builder-contract.md` — legacy/superseded labelling + cross-ref rewording (see
  `reconcile-all-references-no-stale-v2`). It references `plan-format.md` at lines ~6–7, ~15, ~35,
  ~72, ~405, ~440 (verify exact lines at edit time).
- `docs/overview.md` — builder section (~lines 293–302) legacy label + cross-ref; module-doc
  lifecycle list (~line 104, keeps `plan-format.md` — no change needed there beyond it staying
  listed); any `plan-format v2` string.
- `docs/reference/model-spec.md` (~line 5), `docs/reference/discussion-format.md` (~lines 6, 33,
  41), `docs/reference/status-schema.md` (~lines 7, 106) — v2 → v3/neutral wording.

Cross-doc references that must stay valid (grep `plan-format` across `docs/` and `manifest/` before
finishing to confirm nothing dangles): several `manifest/designs/*.md` already link to
`plan-format-v3.md` (e.g. `loom.md`, `loom-planner.md`, `codeintel-redesign.md`,
`webster-rewrite.md`, `webster-parallel-execution.md`). **When `plan-format-v3.md` is deleted, those
links break.** Repoint them to `docs/reference/plan-format.md` (the durable home) as part of this
task — a deleted design doc must not leave dangling `[...](plan-format-v3.md)` links behind.

The reframe (builder/webster are shipped, not future) is grounded in: `internal/builderengine/doc.go`
(package header: "pinned plan-format v2 plan… batch by batch"), `internal/websterengine/doc.go`
("A/B contract-compatible with builder… parsed by the same builderengine.ParsePlan"), and
`manifest/roadmap.md` Done entries for `builder` and `webster`.

## Constraints

From `CONSTRAINTS.md` and `CLAUDE.md`, relevant here:

- **Documentation Lifecycle** (`CONSTRAINTS.md` → `## Documentation Lifecycle`, detail in
  `docs/overview.md#documentation-lifecycle`): design docs (`manifest/designs/*.md`) are deleted when
  the thing lands; durable reference docs (`docs/reference/*.md`, incl. `plan-format.md`) are kept.
  This task *is* that lifecycle event for `plan-format-v3.md` → `plan-format.md`.
- **Task completion** (`CLAUDE.md`): a change touching a named module's design must update docs in
  the same commit. Here the deliverable *is* docs. `manifest/roadmap.md` is for planned
  modules/milestones — completing this planned item means moving **plan-format v3** from Planned to
  Done **is legitimate**, but is a finalization concern; the plan should treat the roadmap flip as an
  explicit final step, not scatter roadmap edits through the doc rewrite.
- **No cross-cutting code invariants apply** (Hub Geometry, lyxtest Leaf, CLI/Cobra) — no Go changes.
- **Markdown style**: follow `mill:markdown` conventions for the generated/edited `.md` files.

## Testing

This is a documentation-only task — there is no runtime behaviour to exercise and no new code to
unit-test. Verification is:

- `go build ./...` and the existing Go test suite must **still pass unchanged** — a sanity check that
  no code was accidentally touched (the v2 parser/validator and its `testdata/` plans stay exactly as
  they are).
- **Internal consistency of the rewritten `plan-format.md`**: the worked example must be
  byte-consistent across its Card Index ↔ per-card filenames ↔ card headings/numbering (this is the
  same property v2's example holds; check by reading, since no validator runs it).
- **No dangling links**: grep `plan-format` (and specifically `plan-format-v3.md`) across `docs/` and
  `manifest/` after the edits; every reference must resolve — `plan-format-v3.md` links must be
  repointed to `docs/reference/plan-format.md`, and no doc may still describe `plan-format.md` as v2
  in a way that contradicts the new content.
- **No stale-v2 residue**: confirm the reconciliation left `builder-contract.md`/`overview.md`
  honestly labelled (legacy v2 runtime, v3 the pinned schema) and every passing "v2" mention updated.

No TDD candidates (no code). If any CLI help text or help-tree test referenced plan-format wording,
it would need updating — none does (no CLI surface changes).

## Q&A log

- **Q:** Does this task rewrite the Go code (`builderengine`/`websterengine`) to consume v3?
  **A:** No. Only the schema/doc lands here; the code rewrite is the separate later **webster-rewrite**
  roadmap item. Builder is obsolete and will be replaced by webster; today's webster is also rewritten
  then.
- **Q:** How to handle the transitional window where `plan-format.md` = v3 but the shipped code still
  parses v2? **A:** Doc-leads-code with an explicit transitional Status note, and reconcile the
  neighbouring durable docs so nothing is stale (chosen over leaving them untouched or fully
  converting them to v3).
- **Q:** How much reconciliation of neighbour docs? **A:** Update **all** references — no stale v2
  pointers. `builder-contract.md`/overview keep a truthful-but-**legacy-labelled** v2 description;
  `model-spec.md`/`discussion-format.md`/`status-schema.md`/overview-table get "v2"→v3/neutral.
- **Q:** Does v3 collapse the per-card footprint to a single `changes-files` list (design doc's
  literal v0 field list)? **A:** No — keep the typed split `Context:`/`Edits:`/`Creates:`/`Deletes:`/
  `Moves:`, plus Rename mechanic and `root:`. `changes-files` is the derived union, not a field.
- **Q:** On-disk layout — one file or per-card files? **A:** Per-card files `NN-<card-slug>.md` under
  `_lyx/plan/` plus a `00-overview.md` index (a single all-cards file was judged too long/overkill).
- **Q:** `verify:` model? **A:** Optional per-card + optional plan-level integration `verify:`; no
  mandatory per-batch gate (implicit build+unit-test gate per card).
- **Q:** Scope? **A:** Dropped entirely — the card's typed fields are its footprint.
- **Q:** `root:` placement? **A:** Plan-level (in `00-overview.md`), `//` escape retained.
- **Q:** Rename mechanic placement? **A:** One plan-level `## Rename mechanic` section, required when
  any card has a non-empty `Moves:`.
- **Q:** Card numbering / commit subject? **A:** Flat `N` (1..N); commit `N: <short what>`; file
  prefix `NN` matches `N`.
- **Q:** Output/report schema in `plan-format.md`? **A:** Punted to the consumer doc; `plan-format.md`
  pins only the input plan schema.
- **Q:** Execution-policy sections (roles/models, review cadence, oversized, chains)? **A:** Stripped
  here — that info lands in webster-rewrite's "webster-preprocessing" part; not relocated by this task.
- **Q:** The deferred symbol-field/DAG design? **A:** Keep a short "Deferred / forward-compat" section
  (symbol fields omitted in v0, pending codeintel) with pointers to `codeintel-redesign.md` and
  `webster-rewrite.md`; trim the detailed mechanism design.
