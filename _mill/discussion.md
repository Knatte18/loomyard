# Discussion: loom: pin the spawn/handover status schema + discussion-format.md

```yaml
task: 'loom: pin the spawn/handover status schema + discussion-format.md'
slug: loom-contracts
status: discussing
parent: main
```

## Problem

The `loom` orchestrator (roadmap milestone 12, `docs/modules/loom.md`) inverts today's
mill/millhouse model: control flow moves into Go, and each phase becomes a pure function
over a **file contract**. The independence that buys resume, swapping, and testability
depends entirely on those contracts being pinned precisely — "the design effort moves from
writing long skills to pinning the contracts."

Two of those contracts are not yet pinned. Milestone 12's build order puts them **first**
("Contracts first — spec only, no code, review-gated like everything else, never
hand-written outside the pipeline"):

1. **The spawn/handover status schema** — the seed state of loom's own ongoing `_lyx/`
   status file (the file loom rewrites throughout its whole run).
2. **`discussion-format.md`** — the `discussion.md` ↔ Plan contract, the analogue of the
   existing `plan-format.md`.

**Why now:** these two contracts gate the rest of the loom build (Preflight, the Discussion
producer, the Plan producer, the phase-machine skeleton, Finalize all consume one or both).
They must be pinned through the normal review-gated pipeline before any loom code is written.

This task is **spec/documentation only** — no Go code, no validator, no seed-writer
implementation. It produces two contract documents (plus adjacent doc reconciliation).

## Scope

**In:**

- Author `docs/reference/status-schema.md` — pins the `_lyx/status.json` schema (contract 1).
- Author `docs/reference/discussion-format.md` — pins the `_lyx/discussion/` contract (contract 2).
- **Relocate** `docs/modules/plan-format.md` → `docs/reference/plan-format.md` and
  `docs/modules/builder-contract.md` → `docs/reference/builder-contract.md` (they are
  cross-module *contracts*, not module-design docs). Fix **every inbound reference that
  encodes the old location** repo-wide → `docs/reference/…`, in **both** forms:
  - **Full-path** `docs/modules/{plan-format,builder-contract}.md` — in docs, Go godoc
    **comments** (`internal/hubgeometry/hubgeometry.go`, `internal/buildercli/cli.go`,
    `internal/builderengine/{validate,report,doc,template_test}.go`), the
    `internal/builderengine/implementer-template.md` template,
    `tools/sandbox/SANDBOX-BUILDER-SUITE.md`, and `docs/reviews/builder-review-prompt.md`
    (8 refs — the largest single cluster).
  - **Relative** markdown links that resolve to the moving files but never match a full-path
    grep: `modules/…` from docs at the `docs/` root (`docs/overview.md:269,272,375`,
    `docs/roadmap.md:57,60,74,189,195,333` → `reference/…`), `../modules/plan-format.md` from
    `docs/reference/model-spec.md:5` (→ `plan-format.md`, now a sibling), and same-folder
    sibling links from `docs/modules/loom.md` (`(plan-format.md)` at L36/L111,
    `(builder-contract.md)` at L117, and both at L235 → `../reference/…`). Since this task
    already edits loom.md, overview.md, and roadmap.md, their inbound links move in the same
    pass.
  These are comment/string-only edits — see the "No Go code = no *functional* Go" note under
  Constraints. Bare-filename mentions ("`plan-format.md` pins …", encoding no path) are
  location-agnostic and left untouched. (round-1 gap G1; round-2 gap + note)
- Reconcile `docs/modules/loom.md` to match the pinned status schema **and** the
  Setup→Preflight rename (see Decisions: `loom-md-reconciliation`).
- Update `docs/overview.md`'s Documentation-lifecycle section to distinguish *module-design
  docs* (`docs/modules/`, deleted on landing) from *durable contract/reference docs*
  (`docs/reference/`, kept); also rename Setup→Preflight and add Raddle in overview.md's loom
  phase blurb (`overview.md:273`).
- Mark roadmap milestone 12 sub-item 1 ("Contracts first") ✅ Done on completion.

(The round-1 review's "update `docs/modules/README.md`" candidate was **dropped**: that file's
table links neither doc — verified — so there is nothing there to update. round-1 gap G2.)

**Out:**

- **No Go code.** No `internal/state` schema type, no seed-writer command, no validator, no
  `lyx loom status` renderer. Those land with later milestone-12 pieces.
- No `ly-spawn` skill (an optional thin wrapper may come later; not this task).
- No change to `plan-format.md`/`builder-contract.md` *content* beyond the relocation and the
  link/anchor fixups relocation forces.
- No new `CONSTRAINTS.md` invariant (a "Status Schema Invariant" belongs with the code that
  lands later, not with a spec doc).
- Raddle phase specifics, perch/burler internals — already documented elsewhere; only
  referenced, not redesigned.

## Decisions

### status-file-format-json

- **Decision:** The loom status file is `_lyx/status.json` — **JSON via the existing
  `internal/state` primitive** (locked, atomic, typed `WriteJSON[T]`/`ReadJSON[T]`), the same
  mechanism `builder` uses for its `state.json`.
- **Rationale:** It is machine-written / machine-read orchestration state, exactly what
  milestone 3's `internal/state` was built for. Reusing it gives locking + atomic writes for
  free and keeps one state primitive across modules. `lyx loom status --watch` pretty-prints
  for humans, so the on-disk file need not be hand-readable.
- **Overrides the board brief.** The brief specified "plain YAML … rather than
  markdown-with-frontmatter." The YAML-vs-markdown point stands (no markdown-frontmatter);
  the operator explicitly chose **JSON over YAML** for status-class files. This override is
  deliberate and must be stated in `status-schema.md` so the Discussion-review gate reads it
  as a defended decision, not an accident. (The mill-frontmatter comparison the brief drew is
  still honored — the point was "structured, not prose-with-frontmatter.")

### status-single-schema-superset

- **Decision:** One schema (a superset struct), not two. The "seed"/handover state is that
  same schema with only the handoff fields populated; loom fills the rest as it runs.
- **Rationale:** One Go struct, one `internal/state` type, no seed→full conversion step. The
  seed is simply an instance with empty `history`, null `start_sha`, initial `narration`.
  Mirrors how mill's single `status.md` schema is appended to over its lifetime.
- **"Seed" defined:** the t=0 contents of `_lyx/status.json` at the instant a task is spawned
  and handed to loom, before any `lyx loom run` has executed. Not a separate file/schema —
  the initial snapshot of the file loom then keeps rewriting.

### status-seed-writer-is-a-lyx-command

- **Decision:** The seed is written by a **lyx Go command** at spawn time (the mill-spawn
  analogue, but Go — not a skill/agent). loom's Preflight **requires the file to exist** and
  fails loud if missing (consistent with the "no half-finished prior run" precondition
  checks); the file's existence *is* the handoff signal.
- **Rationale:** "Go owns the machine." An interactive agent must never hand-write this
  contract. A thin `ly-spawn` skill wrapper over the lyx command may exist later for
  convenience but is not the writer and is out of scope here.
- **Binding deferred:** which exact subcommand writes it (`warp add` vs a dedicated
  `lyx loom init`/`spawn`) is pinned when that command lands; `status-schema.md` names the
  *role* ("the spawn-time lyx command"), not the subcommand.

### status-field-set

- **Decision:** The `_lyx/status.json` fields (JSON; `schema_version` intentionally omitted —
  see `no-schema-version`):

  ```jsonc
  {
    "slug": "loom-contracts",        // board-task pointer (board owns title/description)
    "parent": "main",                // parent branch
    "phase": "builder",              // preflight|discussion|plan|builder|raddle|finalize|done
    "stage": "gate",                 // "produce" | "gate": producing the artifact vs in its review gate
    "narration": "now: … / last: … / wait: …",  // human line for `lyx loom status --watch`
    "history": [                     // per-phase outcome trail (per-round verdicts live in perch's block files)
      { "phase": "discussion", "outcome": "approved", "ts": "…" },
      { "phase": "plan", "outcome": "stuck", "bounced_to": "discussion", "ts": "…" }
    ],
    "start_sha": null,               // host HEAD stamped when Builder begins (Raddle diff base)
    "pause_requested": false,        // pause flag kept IN-status (loom.md; builder uses a separate flag file)
    "next_action": null              // when loom yields at a human gate: what the human must do next
  }
  ```

- **Rationale / per-field notes:**
  - `slug` / `parent` — the only handoff pointers; board owns durable title/description.
  - `phase` — the phase enum from loom.md's phase machine, plus terminal `done`.
  - `stage` (`produce`|`gate`) — kept: loom needs to know whether a phase is mid-produce or
    mid-gate for resume; the finer round detail stays in perch. This file is loom's single
    total overview of *where it is*.
  - `narration` — one composed human string with `now:`/`last:`/`wait:` segments (loom.md's
    example); loom writes it, the status strand prints it, mux never parses it.
  - `history` — **per-phase outcome trail** (see `verdict-history-granularity`). Each entry's
    `ts` (and any other timestamp field) is **RFC3339 UTC** (e.g. `2026-07-17T10:01:30Z`) —
    pinned so the fail-loud parse has a well-defined "malformed" for the field; state this in
    `status-schema.md`'s field notes / check list.
  - `start_sha` — host `HEAD` stamped when Builder begins, so Raddle can diff `start_sha..HEAD`.
  - `pause_requested` — in-status flag (loom.md keeps loom's pause flag here; note this
    diverges from builder, which uses a separate pause *flag file* — call the divergence out
    in the doc).
  - `next_action` — a dedicated field (not just narration prose) so "is this blocked on a
    human?" is machine-checkable; it is *also* reflected in `narration`'s `wait:` segment.
- **Parse discipline:** the doc specifies strict, fail-loud parsing (the `internal/state`
  read must reject unknown/malformed fields — the same `KnownFields(true)` discipline as
  builder's `ParseOutcome` and the burler verdict-parse). Never guess a status.

### verdict-history-granularity

- **Decision:** `history` is a **per-phase outcome trail** — one entry per phase attempt
  (`{phase, outcome: approved|stuck, bounced_to?, ts}`), including stuck-handler bounce-backs.
  Per-*round* verdicts are **not** duplicated here; they live in perch's block files.
- **Rationale:** Resolves an apparent contradiction in loom.md, which says status carries "the
  verdict history the progress-judge needs" but *also* says "Separation of state: perch owns
  its block's round state in the block's files; loom's status only needs phase + the block's
  outcome." The progress-judge lives *inside* perch and reads perch's block files. So loom's
  status records phase-level outcomes only; the per-round history is perch's. loom.md's prose
  is reconciled to match (see `loom-md-reconciliation`).

### no-schema-version

- **Decision:** No `format:`/`schema_version` field on either the status file or the
  decision-record.
- **Rationale:** At this scale a version stamp is a rarely-exercised guard that goes stale and
  reads like a pseudo-"version." plan-format needs `format:` because `builder` validates plans
  mechanically and had a real v1→v2 bump; neither the status file (loom-written, single
  writer) nor the discussion record has that pressure. Drop it; reintroduce only if a real
  incompatibility ever forces it.

### discussion-on-disk-split

- **Decision:** `_lyx/discussion/` is a **directory with two files**, a hard access boundary:
  - `decision-record.md` — the **distilled** record; the Plan producer's **sole** input.
  - `support-log.md` — the **raw** support log; the Discussion-review gate reads it, the Plan
    producer **never** does.
- **Rationale:** Mirrors Builder's "distilled digest, never raw prose" rule. Two files give a
  hard boundary (Plan cannot accidentally ingest the raw interview transcript / token bloat),
  stronger than two sections in one file. Self-describing filenames over terse ones.

### decision-record-sections

- **Decision:** `decision-record.md` has **no frontmatter** and these sections:
  Goal / Scope / Decisions / Constraints / Auto-mode assumptions / Open risks /
  Acceptance criteria. Plus an **optional, non-binding** "Notes for the plan writer"
  subsection.
- **Rationale:**
  - **No frontmatter:** `format:` dropped (`no-schema-version`); `approved:` dropped because
    approval is recorded in the status file's `history` (`discussion → approved`) — the status
    file is loom's single total-status locus, so a lone `approved` flag in the record would
    duplicate it. loom always drives the Plan producer *after* approval, so the record needs
    no standalone gate (unlike plan-format, whose `approved:` exists because `lyx builder run`
    runs standalone).
  - **Rejected alternatives are NOT in the record** — they move to `support-log.md` (brief
    mandate); this is a key compaction vs millhouse's template.
  - **Decisions** carry Decision + Rationale only (rejected alts → log).
  - **Compaction vs millhouse:** millhouse's `discussion.md` mixed distilled + raw in one
    file with heavy italic prose-coaching. The rendered decision-record must be terse — no
    prose-coaching, no "pjatt."
  - **Technical context / Testing folded:** millhouse had standalone "Technical context"
    (codebase pointers/helpers/gotchas for the plan-writer) and "Testing" (TDD candidates,
    must-cover scenarios) sections. loom's Plan producer explores the codebase **itself**, so
    neither is a completeness requirement. Must-cover test scenarios go under **Acceptance
    criteria**; a genuinely useful helper/gotcha may go in the optional **Notes for the plan
    writer** head-start subsection (explicitly non-exhaustive).

### support-log-sections

- **Decision:** `support-log.md` sections: Interview (turn-by-turn, distilled not verbatim) /
  Rejected alternatives / **Review rounds** (per round: verdict + findings + how resolved) /
  Question ledger (running open/resolved questions + `--auto` picks).
- **Rationale:** The **Review rounds** ledger is the **anti-circling** store: each new
  discussion-review round reads it before raising findings, so successive reviewers don't
  re-raise points earlier rounds already settled. This is the primary purpose of the extra
  metadata (per operator). The record↔log split keeps all of this out of the Plan producer's
  input.
- **Note (not pinned here):** who *writes* the Review-rounds ledger (the Discussion producer
  vs the perch discussion gate) is an implementation detail for later milestone-12 pieces;
  `discussion-format.md` pins the *contract* (the ledger exists, its purpose, its shape), and
  notes the likely writer without binding it.

### doc-rigor-moderate

- **Decision:** Both docs get **moderate** rigor: a **short validation-check list** and a
  **compact worked example** — lighter than plan-format's 18 checks / full example.
- **Rationale:** These pin real contracts a future validator/consumer will honor, so an
  enumerated (but small) check list earns its keep; a worked example disambiguates the shape.
  But they are smaller/simpler contracts than plan-format, so full parity would be
  over-weight.
- **discussion-format check list (illustrative, spec-for-future-validator):** required
  decision-record sections present; the Plan-never-reads-`support-log` boundary; the two files
  exist under `_lyx/discussion/`. **status-schema check list:** required fields present;
  `phase`/`stage`/`outcome` values in their fixed vocabularies; strict fail-loud parse (reject
  unknown/malformed).

### docs-are-contracts-not-modules

- **Decision:** The two new docs live in `docs/reference/` (alongside `model-spec.md`,
  `tmux_scripting.md`), **not** `docs/modules/`. `plan-format.md` and `builder-contract.md`
  relocate there too.
- **Rationale:** These are cross-module *contracts / reference material*, not per-module
  design docs. `docs/modules/*` are deleted when their module lands (doc lifecycle);
  contract docs are **durable** — plan-format.md and builder-contract.md already self-describe
  as "durable — it stays," which is precisely why they don't belong in the delete-on-landing
  folder. Moving them leaves `docs/modules/` holding only true module-design docs
  (`loom.md`, `hardener.md`, README).

### loom-md-reconciliation

- **Decision:** Reconcile the phase/status prose to match the pinned contracts, same commit:
  - `docs/modules/loom.md` "State & contracts" (and related prose): point at the new
    `docs/reference/status-schema.md`; correct verdict-history wording to **per-phase outcome**
    (not per-round — that's perch's); reflect JSON-via-`internal/state`; keep the in-status
    `pause_requested` note.
  - **Setup→Preflight rename** (the roadmap-12.2 rename loom.md/overview.md never caught up
    to): `docs/modules/loom.md` phase-machine diagram (L54), the "Setup validates geometry…"
    prose (L63), and the module-decomposition table row (L239); plus `docs/overview.md:273`'s
    loom blurb. (round-1 gap G3)
  - **Add Raddle** to `overview.md:273`'s loom phase list (currently
    Setup→Discussion→Plan→Builder→Finalize, omitting Raddle).
- **Rationale:** The doc-lifecycle rule (CLAUDE.md) forbids shipping the pinned `preflight`
  enum + status contract while loom.md/overview.md prose still says "Setup" or describes a
  divergent status shape. The pinned `phase` vocabulary
  (`preflight|discussion|plan|builder|raddle|finalize|done`) is the source of truth the prose
  must match.

## Technical context

- **loom.md** (`docs/modules/loom.md`) — the design reference for the phase machine, the
  status-file role ("State & contracts"), the phase enum, pause semantics, and crash-recovery.
  The status schema must match its described behavior (and reconcile the two divergences noted
  above).
- **plan-format.md** (`docs/modules/plan-format.md`, relocating) — the precedent for a pinned
  file contract: frontmatter discipline, worked example, validation-check list, "fail loud
  never misread." discussion-format.md is its analogue.
- **builder-contract.md** (`docs/modules/builder-contract.md`, relocating) — precedent for
  YAML/JSON state+outcome shapes (`state.json`, `outcome.yaml`, the digest contract) and the
  strict `ParseOutcome`/`KnownFields(true)` fail-loud discipline the status doc echoes.
- **`internal/state`** (roadmap milestone 3, `docs/shared-libs/`) — the generic locked typed
  JSON primitive (`WriteJSON[T]`/`ReadJSON[T]`); the status file is a future consumer. No code
  written this task, but the doc names it as the intended mechanism.
- **`internal/hubgeometry`** — Hub Geometry Invariant: `_lyx` and its subpaths resolve only
  through hubgeometry helpers. The doc should describe `_lyx/status.json` and `_lyx/discussion/`
  as weft-overlay paths that will resolve via hubgeometry when code lands (no path
  construction in this spec task).
- **millhouse discussion template** (`plugins/mill/templates/discussion.md`) — the prior art
  being distilled/split/compacted; referenced for the section mapping, not copied.
- **docs/overview.md** — Documentation-lifecycle + weft-overlay model (`_lyx/` durable vs
  `.lyx/` ephemeral); the status file is durable `_lyx/` (weft-synced → cross-machine resume).

## Constraints

- **Documentation Lifecycle (CLAUDE.md / CONSTRAINTS.md):** shipping these contracts requires
  same-commit reconciliation of loom.md and overview.md; a commit that ships the new docs
  while the old prose still describes a divergent contract is incomplete.
- **Hub Geometry Invariant:** any path reference in the docs treats `_lyx`/`_lyx/config` etc.
  as hubgeometry-owned tokens; the spec describes them, code (later) constructs them only via
  hubgeometry.
- **Markdown rules (`mill:markdown`):** the generated docs follow the repo's markdown
  conventions.
- **Pipeline discipline (board brief):** these contracts must go through the normal
  review-gated pipeline (this discussion → mill-plan → mill-go → review), never hand-written
  outside it.
- **Spec-only:** no `go` code, so no `go test`/build gate applies to the deliverable itself;
  the "verify" for the plan's batches is doc-review / link-integrity, not compilation.
- **"No Go code" = no *functional* Go.** Editing doc-path strings inside Go godoc **comments**
  (and the `implementer-template.md` / sandbox markdown) to retarget the relocated docs is in
  scope and is NOT "writing Go code": no types, no logic, no behavior, nothing recompiled in
  substance. This resolves the apparent Scope↔"no code" contradiction (round-1 gap G1).

## Testing

No code, so no unit tests. The plan's per-batch `verify:` is documentation integrity:

- **Link integrity (repo-wide, both link forms)** — after relocating, grep the whole repo for
  **full-path** (`docs/modules/plan-format.md`, `docs/modules/builder-contract.md`) **and
  relative** (`modules/plan-format.md`, `modules/builder-contract.md`,
  `../modules/plan-format.md`, `../modules/builder-contract.md`, and loom.md's same-folder
  `(plan-format.md)` / `(builder-contract.md)`) references; confirm **zero** that still resolve
  to `docs/modules/…`. Covers docs, Go godoc comments, `implementer-template.md`,
  `tools/sandbox/SANDBOX-BUILDER-SUITE.md`, and `docs/reviews/builder-review-prompt.md`. Also
  fix intra-file relative links *inside* the two moved files (their links to sibling module
  docs such as `loom.md` shift `loom.md` → `../modules/loom.md`; `../overview.md` and the two
  files' mutual sibling link are unchanged since both move together). Bare-filename mentions
  are location-agnostic and excluded.
- **Contract self-consistency** — the status-schema field list, the phase/stage/outcome
  vocabularies, and the worked examples agree internally and with loom.md's (reconciled)
  prose.
- **Boundary statement present** — discussion-format.md explicitly states the
  Plan-never-reads-`support-log` boundary and the record↔log split.
- **No orphaned prose** — loom.md and overview.md contain no remaining wording describing the
  superseded shape (per-round-in-status / YAML / docs-in-modules), **no remaining "Setup"
  label** for the Preflight phase, and overview.md's loom blurb includes Raddle.
- **Roadmap** — milestone 12 sub-item 1 marked ✅ Done with links to the two new docs.

## Q&A log

- **Q:** YAML (per brief) or JSON for the status file? **A:** JSON, via `internal/state`
  (locked/atomic/typed), like builder's `state.json` — overrides the brief's "plain YAML";
  recorded as a deliberate, defended decision.
- **Q:** One status schema or two (seed vs full)? **A:** One superset; the seed is that schema
  with only handoff fields populated.
- **Q:** Who writes the seed? **A:** A lyx Go command at spawn (mill-spawn analogue); loom
  Preflight requires the file to exist (fail-loud if missing). Optional thin `ly-spawn` skill
  wrapper later, out of scope.
- **Q:** verdict-history granularity? **A:** Per-phase outcome trail in status; per-round
  verdicts stay in perch's block files; reconcile loom.md's contradictory wording.
- **Q:** Keep `stage` (produce|gate)? **A:** Yes — this file is loom's single total overview
  of where it is; loom needs produce-vs-gate for resume, round detail stays in perch.
- **Q:** `next_action` a field or narration? **A:** Dedicated field (machine-checkable
  "blocked on human"), also reflected in `narration`'s `wait:` segment.
- **Q:** Where do these docs live? **A:** `docs/reference/` — they're durable contracts, not
  delete-on-landing module-design docs. Move `plan-format.md` + `builder-contract.md` there
  too.
- **Q:** discussion split — one file or two? **A:** Two files under `_lyx/discussion/`
  (`decision-record.md` = Plan's only input; `support-log.md` = review only, never Plan).
- **Q:** decision-record frontmatter? **A:** None. `format:` dropped (over-engineering at this
  scale); `approved:` dropped (approval lives in status `history`; the status file is the
  single total-status locus).
- **Q:** Keep millhouse's Technical context / Testing sections? **A:** Fold — Plan explores the
  code itself; must-cover scenarios → Acceptance criteria; useful pointers → optional
  non-binding "Notes for the plan writer" head-start.
- **Q:** Purpose of the support-log's extra review metadata? **A:** Anti-circling — successive
  discussion-review rounds read the Review-rounds ledger so they don't re-raise settled
  points.
- **Q:** Doc rigor vs plan-format? **A:** Moderate — short validation-check list + compact
  worked example; lighter than plan-format's 18 checks.
- **Q (review r1 G1):** Do godoc/template/sandbox full-path refs to the relocating docs get
  fixed, given "no Go code"? **A:** Yes — fix every full-path `docs/modules/…` reference
  repo-wide; comment/string-only edits are not "functional Go." Bare-name mentions untouched.
- **Q (review r1 G2):** Update `docs/modules/README.md`? **A:** No — its table links neither
  doc (verified); item dropped, link-integrity covers the real inbound links.
- **Q (review r1 G3):** Setup vs pinned `preflight`? **A:** Rename Setup→Preflight in loom.md
  + overview.md and add Raddle to overview.md's loom blurb — the rename was already agreed
  (roadmap 12.2); loom.md/overview.md just never caught up.
- **Q (review r2 G):** Relative inbound links (not only full-path) to the relocated docs?
  **A:** Fix both forms — full-path AND relative (`modules/…`, `../modules/…`, loom.md
  same-folder siblings) → `reference/…`; the link-integrity verify greps both forms. The
  builder-review-prompt.md cluster (8 refs) is named explicitly in Scope.
