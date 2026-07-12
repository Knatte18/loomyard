# Discussion: Add typed file-ops to lyx's plan-format

```yaml
task: Add typed file-ops to lyx's plan-format
slug: plan-format-file-ops
status: discussing
parent: internal-builder
```

## Problem

plan-format v1 (`docs/modules/plan-format.md`, consumed by `internal/builderengine`)
gives each Card only a free-text `**What:**` plus a flat `**Where:**` file list. There is
no distinction between creating, editing, deleting, or renaming a file. Two observed
consequences: a rename shows up in git as an unrelated create+delete pair (breaking git
history — the exact failure the repo's `git mv` + surgical-edits convention exists to
prevent, but the plan format gives no structured way to declare a rename), and the `poll`
verb's scope-drift check cannot distinguish "this batch should *create* X" from "this
batch should *edit* X". Cards are also numbered batch-locally ("Card 1, Card 2") even
though the commit-subject convention already uses global `NN.C` identifiers, and the
overview has no place for cross-cutting decisions.

Mill's plan format solved all of this over a long evolution (typed card fields,
mechanically validated `Moves:` pairs, a Shared Decisions overview section). This task
ports the parts that fit lyx's simpler DAG-less, single-orchestrator model into
**plan-format v2**: the format doc, the parser (`internal/builderengine/plan.go`), and
the validator (`internal/builderengine/validate.go`).

## Scope

**In:**

- `docs/modules/plan-format.md` rewritten as **plan-format v2** (typed card fields,
  `<batchNR>.<cardNR>` card numbering, optional `Commit:` field, per-batch `root:` path
  shorthand, card counts in the Batch Index, `## Shared Decisions` overview section,
  `## Rename mechanic` requirement, updated worked example, updated Validation checks
  list).
- `internal/builderengine/plan.go` — parser extended: per-card struct, five typed
  file-op fields, `Moves:` pair grammar, `root:`/`//` path resolution, Batch Index card
  counts, `WhereFiles` removed.
- `internal/builderengine/validate.go` — new checks (see Decisions), existing checks
  adapted (`recognizedFormat` = 2; batch-oversized estimate reads typed fields).
- `internal/builderengine/implementer-template.md` — rewritten for typed fields, the
  `NN.C` card headings, per-card `Commit:`, and "read your batch file **and**
  `00-overview.md`" (replacing "never read 00-overview.md").
- Test fixtures (`internal/builderengine/testdata/`) and tests updated to v2.
- `internal/builderengine/orchestrator-template.md` and other in-package references to
  v1 card fields ("Where:", "Card N") updated where they describe the format.
- Docs lifecycle: module doc(s) that describe the plan format or builder's parsing
  updated in the same commit(s).

**Out:**

- **Mill's DAG (`depends-on:`)** — lyx deliberately keeps a plain ordered batch
  sequence; documented design choice, not an oversight.
- **Mill's per-batch review loop** and the plan-reviewer "bulking" rule — lyx is
  holistic-review-only (v1 decision). No `Context:`-driven review bulking lands here.
- **Mill's optional module-wide `verify:` run at every batch boundary** — explicitly
  banned by the task. Per-batch `verify:` stays narrowly package-scoped (e.g.
  `go test ./internal/builderengine/...`, never `go test ./...`). If a module-wide gate
  is ever wanted it must be baseline-aware and boundary-gated — a future task.
- **Mill's "All Files Touched" overview section and its mismatch check** — investigated
  and dropped (see Decisions).
- **poll/digest enrichment using typed ops** (e.g. flagging "declared `Creates:` but the
  file already existed") — natural follow-up task; runtime drift logic is unchanged
  here. The typed fields land in the parsed `Plan`, so the data is ready when that task
  comes.
- No changes to batch-report schema, chain/oversized mechanics, roles/models, or
  `_lyx` path geometry.

## Decisions

### typed-card-fields

- Decision: Each card carries five typed file-op fields, all five **required** on every
  card, with the literal `none` on the label line when a field is empty: `Context:`
  (files read but not changed), `Edits:` (existing files that change), `Creates:` (new
  files), `Deletes:` (files removed), `Moves:` (rename pairs). Field values are
  backtick-wrapped paths, one per indented sub-bullet (mill's grammar): no inline
  commentary, no line-range suffixes, no comma-separated inline lists. Files in
  `Edits:` are implicitly read and are not repeated in `Context:`. The fields are
  mutually exclusive **within a card**: the same path in two fields of one card is a
  contradiction, enforced by the `card-field-overlap` check (see validator-check-set).
  Across cards of the same batch, `Creates:` in one card followed by `Edits:` of the
  same path in a later card is legitimate planning (the `path-missing` suppression
  sets are built on exactly that), and the same path in two cards' `Edits:` is normal.
  `Moves:` endpoints additionally must not collide with `Creates:`/`Deletes:` anywhere
  in the same batch — that is `move-redundant`'s batch-level rule, ported from mill.
- Rationale: mill's `card-missing-field` experience — a forgotten field must be
  mechanically detectable; an absent-means-none convention silently degrades a forgotten
  `Moves:` into create+delete drift, the exact failure this task exists to kill.
- Rejected: optional fields (absent = none); a lighter 3-field subset.

### card-prose-fields

- Decision: The card keeps lyx's `**What:**` as the prose field (playing mill's
  `Requirements:` role — concrete, stable identifiers, detailed enough for a cheap
  model). Field order per card: `What:`, `Context:`, `Edits:`, `Creates:`, `Deletes:`,
  `Moves:`, then optional `Commit:`, then optional `verify:`. `**Where:**` is gone.
- Rationale: `What` is established lyx v1 vocabulary; renaming to `Requirements:` buys
  nothing. Files-after-prose keeps the card readable top-down.
- Rejected: adopting mill's `Requirements:` name.

### context-is-advisory

- Decision: `Context:` is an **advisory read-list** — "files the Planner expects the
  implementer to read" — not mill's allowlist ("read ONLY these; an unlisted needed file
  is a plan defect"). The implementer may read beyond it. `Context:` bytes count toward
  the batch-oversized context estimate.
- Rationale: consistent with lyx's documented "declared ownership, not a cage" scope
  philosophy; a read restriction is not mechanically enforceable anyway. Mill's stricter
  posture served its per-batch review bulking, which lyx deliberately does not have.
- Rejected: mill's allowlist semantics; dropping `Context:` entirely (it feeds the
  context estimate and the implementer's cold start).

### moves-grammar-and-rename-mechanic

- Decision: `Moves:` sub-bullets use mill's exact grammar: `` `old/path` -> `new/path` ``
  (backtick-wrapped paths, ASCII ` -> ` arrow). A path appearing in `Moves:` must not
  also appear in `Creates:`/`Deletes:` of the same batch. A rename-plus-extraction is
  one `Moves:` pair (the relocated file) plus a separate `Creates:` entry (the extracted
  file). Every batch with at least one non-empty `Moves:` field MUST carry a
  `## Rename mechanic` section; plan-format.md pins the canonical text (adapted from
  mill): (1) run `git mv <old> <new>` FIRST, before any other change to the moved file;
  (2) then make ONLY surgical edits (package declaration, imports, identifier
  retargeting); (3) use `Creates:` only for genuinely new files; (4) never write the
  relocated file from scratch and delete the original.
- Rationale: this is the repo's own rename convention (`git mv` + surgical edits) made
  declarable and mechanically checkable; mill's five `move-*` checks depend on exactly
  this grammar.
- Rejected: freer prose grammar (not mechanically checkable).

### card-numbering-batch-dot-card

- Decision: The card's markdown heading IS the global identifier:
  `### Card NN.C — <short title>` where `NN` is the batch's zero-padded number (same as
  the filename prefix) and `C` restarts at 1 within each batch (e.g. `### Card 02.3 —
  emission path`). This matches the existing commit-subject convention `02.3: <short
  what>` 1:1. A new `card-numbering` validation check enforces: heading prefix `NN`
  equals the batch's own number, and `C` runs 1..M sequentially with no gaps or
  duplicates.
- Rationale: a card is then citable unambiguously ("5.3") in review findings and
  discussion, and the heading matches the commit log exactly. Deliberate divergence
  from mill, which numbers cards globally sequentially (1, 2, … 7 across batches) —
  `NN.C` carries the same uniqueness plus the batch context for free, and lyx's commit
  convention already uses it.
- Rejected: mill's global sequential numbering; keeping batch-local "Card 1/2/3"
  headings with `NN.C` as a derived internal identifier only.

### per-card-commit-field

- Decision: Optional `**Commit:**` field per card pins the exact commit subject,
  backtick-wrapped (e.g. `` **Commit:** `02.3: add the --json flag` ``). When absent,
  the implementer derives the subject from the global `NN.C: <short what>` convention
  as today. A new `commit-subject-mismatch` validation check requires a present
  `Commit:` value to start with the card's own `NN.C: ` prefix.
- Rationale: lets the Planner pin wording where it matters without changing the
  fallback; a pinned message that breaks the `NN.C` shape would corrupt the resume
  trail that `git log` provides, so the prefix is validated.
- Rejected: mandatory `Commit:` on every card; free-form pinned messages.

### per-batch-root-path-shorthand

- Decision: Batch frontmatter gains an optional `root: <worktree-relative-dir>`. When
  set, every card file-op path (all five fields, both sides of a `Moves:` pair) without
  a `//` prefix resolves as `<root>/<path>`. A path starting with `//` is ALWAYS
  worktree-root-relative (root set or not — one rule, no special cases): that is how a
  file outside the shared root is written, e.g. `//cmd/lyx/main.go`. The parser
  normalizes everything to worktree-relative paths at parse time, so validator, context
  estimate, and drift logic see only normalized paths. A single-`/` prefix or a `..`
  segment is malformed (fail loud). `## Scope` entries stay worktree-relative and are
  NOT affected by `root:`.
- Rationale: mill plans showed heavy token bloat from long repeated path prefixes —
  the same directory prefix repeated across every card field. `root:` removes the
  repetition; `//` keeps escape unambiguous and mechanically checkable.
- Rejected: basename-shorthand resolved against Scope (implicit lookup rule, fails
  late on collision); keeping full paths everywhere.

### batch-index-card-counts

- Decision: The Batch Index entry format gains a mandatory card count:
  `- NN — <batch-slug> (C cards) — <one-line intent>`. A new `card-count-mismatch`
  validation check compares the index count against the number of `### Card` headings
  actually in the batch file.
- Rationale: user wants the overview to show per-batch card counts at a glance; the
  count is a free cross-check that the index and batch files agree.
- Rejected: `cards: N` in batch frontmatter (mill's placement — but then the count is
  not visible in the overview, which was the point); both places (redundant).

### shared-decisions-section

- Decision: `00-overview.md` gains an optional `## Shared Decisions` section:
  cross-cutting decisions every batch inherits, one `### Decision: <short-name>`
  subsection per decision with `Decision:` / `Rationale:` / `Applies to:` lines
  (mill's shape). It is prose for humans and LLM sessions: Go does NOT parse it and no
  validation check reads it (mill has none either).
- Rationale: an implementer three batches in must not re-derive a decision batch 1
  already made. Zero-check is deliberate YAGNI — the task body allowed "one
  plan-validate check at most" and investigation found mill validates nothing here.
- Rejected: mandatory section (empty boilerplate on small tasks); an
  `Applies to:`-validity check (no consumer needs it).

### implementer-reads-overview

- Decision: The implementer prompt (`implementer-template.md`) changes from "never read
  `00-overview.md`" to: read your batch file AND `00-overview.md` (framing, Batch
  Index, Shared Decisions). The "never read another batch's file" rule stays.
- Rationale: Shared Decisions must reach the implementer somehow; mill's implementer
  brief points at the overview's Shared Decisions. With All Files Touched dropped, the
  lyx overview is small (frontmatter, one framing paragraph, Batch Index, Shared
  Decisions), so reading all of it is cheap and gives the implementer orientation.
- Rejected: mill's "read only the `## Shared Decisions` section" (marginal gain over
  reading the small whole file); `SpawnBatch` injecting a filtered section via stencil
  (more Go machinery without real gain when file-reading works).

### no-all-files-touched

- Decision: Mill's "All Files Touched" overview section and its
  `all-files-touched-mismatch` check are **not ported**, despite the task body listing
  them. Investigated firsthand: in mill, nothing consumes the section at runtime — the
  only consumer is the validator check itself; review bulking reads the batch files'
  fields directly (and excludes `Context:`, so the list was never the bulk source); the
  template's claim that mill-go reads it for parallel-overlap warnings is not true in
  code (that check computes from cards); and mill's own fix table calls the list
  "derivative; the cards are the source of truth" with a regenerate-from-cards
  auto-fix, which neuters its checksum value. Lyx additionally has no parallel batches
  (no DAG). Any needed union (with or without `Context:`) is one line of Go over the
  parsed typed fields.
- Rationale: long and bloaty exactly when plans are big (refactors), used by nothing.
- Rejected: compact directory-grouped variant of the list (still a maintained copy of
  derivable data).

### validator-check-set

- Decision: Full port of the move checks plus the new structural checks. New checks in
  `validate.go` (kebab-case names, one stable name each, extending plan-format.md's
  "Validation checks" list):
  1. `move-format` — every non-`none` `Moves:` sub-bullet matches the
     `` `src` -> `dst` `` grammar.
  2. `move-redundant` — a path is both a `Moves:` endpoint and in `Creates:`/`Deletes:`
     of the same batch.
  3. `move-source-missing` — a `Moves:` source neither exists on disk nor is a
     `Creates:` target or `Moves:` destination of an earlier batch (plan-wide
     suppression sets, so chained renames across batches don't false-positive).
  4. `move-target-collision` — a `Moves:` target already exists on disk, is targeted by
     more than one batch, or collides with another batch's `Creates:` (same-batch
     overlap is `move-redundant`'s job).
  5. `move-mechanic-missing` — non-empty `Moves:` in a batch without a
     `## Rename mechanic` section.
  6. `card-missing-field` — a card lacks one of the five required file-op fields (or
     `What:`).
  7. `card-numbering` — heading `NN.C`: `NN` ≠ batch number, or `C` not sequential
     1..M.
  8. `card-count-mismatch` — Batch Index `(C cards)` ≠ actual `### Card` heading count.
  9. `path-missing` — an `Edits:`/`Deletes:`/`Context:` path (or `Moves:` source,
     covered by check 3) that does not exist on disk and is not a `Creates:` target or
     `Moves:` destination of an earlier batch (mill's `non-existent-path`, adapted).
  10. `card-outside-scope` — an `Edits:`/`Creates:`/`Deletes:` path or `Moves:`
      endpoint that falls under none of the batch's `## Scope` prefixes. `Context:` is
      exempt (reading outside scope is legitimate).
  11. `commit-subject-mismatch` — a present `Commit:` value that does not start with
      the card's `NN.C: ` prefix.
  12. `card-field-overlap` — the same path appears in more than one of a single card's
      `Context:`/`Edits:`/`Creates:`/`Deletes:` fields or `Moves:` endpoints
      (per-card mutual exclusivity; also formalizes the "`Edits:` files are not
      repeated in `Context:`" rule). Cross-card overlap within a batch is NOT flagged
      here — `Creates:` then `Edits:` across cards is legitimate; only `Moves:`
      endpoints are batch-level-checked, by `move-redundant`.
  Existing checks adapt: `recognizedFormat` becomes 2 (`format: 1` plans are rejected
  by the existing `format-unrecognized` check — no dual-version support; no production
  v1 plans exist); `batch-oversized`'s estimate reads Scope + the typed per-card paths
  (see next decision); `scope-malformed` extends naturally to the normalized card
  paths' well-formedness (empty/absolute/`..`/unclean, plus the `root:`/`//` grammar
  violations).
- Rationale: the checks are cheap, the validator framework exists, and they are the
  entire payoff of typed fields — catching Planner mistakes before implementation.
- Rejected: lighter subset (drops exactly the checks — source-missing,
  target-collision — that catch real planner errors); `all-files-touched-mismatch`
  (section dropped).

### parser-shape

- Decision: `ParsePlan` stays fail-loud on document structure (frontmatter, headings,
  Batch Index lines, unreadable files), but card-level defects become enumerable
  validation findings rather than parse errors: the parser records per-card data
  leniently (e.g. a malformed `Moves:` sub-bullet is retained raw for `move-format` to
  flag; a missing field is recorded as absent for `card-missing-field`). `PlanBatch`
  gains `Root string` and `Cards []PlanCard`; `PlanCard` carries card number, title,
  the five typed path lists (normalized), `Moves` as ordered `(old, new)` pairs, raw
  unparsed move entries, optional `Commit`, optional per-card verify. `WhereFiles` is
  deleted outright — its only consumer is the batch-oversized estimate inside the same
  package, updated in the same change; `CardCount` becomes derived (`len(Cards)`), and
  the Batch Index card count is stored on the batch for the mismatch check.
- Rationale: findings-lists beat one-error-at-a-time discovery for a Planner fixing a
  plan (mill runs all card checks as findings); document-structure failures still stop
  everything per v1's fail-loud discipline. No external consumer of `WhereFiles` exists
  (verified by grep), so no compatibility shim.
- Rejected: keeping a derived flat `WhereFiles` (YAGNI); hard parse errors for card
  defects.

### context-estimate-inputs

- Decision: The `batch-oversized` context estimate becomes: bytes of every batch Scope
  entry plus every card path across all five fields and both `Moves:` sides, resolved
  on disk (`pathSizeOnDisk` semantics unchanged — nonexistent paths contribute 0, so
  `Creates:` targets and `Moves:` destinations naturally add nothing).
- Rationale: straight generalization of v1's "Scope + Where files"; matches mill's
  estimate spirit (Context+Edits+Creates bytes / 4) without special-casing which
  fields "count" — existence on disk already decides that.
- Rejected: hand-picking fields (Context+Edits only).

### verify-scope-guardrail

- Decision: Restated as a hard v2 doc rule, unchanged from v1 practice: per-batch
  `verify:` commands MUST stay narrowly package-scoped (`go test
  ./internal/<pkg>/...`), never a full-suite `go test ./...` per batch. Mill's optional
  module-wide overview `verify:` is NOT ported. plan-format.md v2 documents this
  explicitly as a design constraint with the rationale (per-batch full-suite runs are
  the exact slowdown lyx avoids; a future module-wide gate must be baseline-aware and
  boundary-gated, per the `module_verify_baseline` precedent).
- Rationale: hard constraint from the task author; already the pattern the builder
  module's own 8 batches used.
- Rejected: porting mill's overview-level `verify:` key.

## Technical context

- **Current parser** (`internal/builderengine/plan.go`): `ParsePlan` reads
  `00-overview.md` (strict frontmatter `format:`/`approved:`, framing, `## Batch
  Index` lines matched by `indexLineRe` accepting `—` or `-`/`--` separators), then
  each `NN-<batch-slug>.md`: optional frontmatter (`oversized:`, `verify: deferred`
  sentinel, `chain-end:`), `## Scope` bullet list (glob = parse error), `## Cards`
  (`### Card N` headings counted via `cardHeadingRe`; `**Where:**` comma-lists
  accumulated), `## verify:` section (mutually exclusive with `verify: deferred`).
  `splitFrontmatter` / `extractSection` are reusable as-is. The Batch Index regex and
  `cardHeadingRe` need the `(C cards)` and `NN.C` extensions; card parsing needs a real
  per-card sub-parser (v1 never stored per-card data).
- **Current validator** (`internal/builderengine/validate.go`): six checks
  (`format-unrecognized`/`plan-unapproved`, `index-file-mismatch`, `verify-missing`,
  `chain-end-dangling`, `batch-oversized`, `scope-malformed`), `ValidationError{Check,
  Batch, Detail}` findings model, deterministic ordering. New checks should follow the
  same shape; card-level findings will want the card's `NN.C` in `Detail` (or an added
  `Card` field — Planner's call; keep `Error()` output stable-ish).
- **Path normalization**: `scopeEntryMalformedReason` + `cleanPosixPath` exist for
  Scope entries; card-path normalization (`root:` join, `//` strip, cleanliness) can
  share them. Comparison semantics for `card-outside-scope` should reuse
  `pathCovers` (digest.go) — boundary-aware prefix match, already correct.
- **Estimate**: `pathSizeOnDisk` (validate.go) is reused unchanged.
- **Templates**: `implementer-template.md` is stencil-filled by `SpawnBatch`
  (`spawn.go`) — all markers required non-empty, no conditionals
  (`internal/stencil`). The v2 rewrite is prose-only (no new markers needed): typed
  fields, `NN.C` headings, `Commit:` fallback rule, read-the-overview instruction,
  Rename-mechanic compliance (run `git mv` first). Check
  `orchestrator-template.md` for v1 format references during planning.
- **Mill reference points** (read firsthand during discussion; note: these live in the
  **millhouse repo**, not this worktree — e.g.
  `C:\Code\millhouse\wts\millhouse\plugins\mill\...`):
  `plugins/mill/templates/plan-batch.md` (field grammar + Rename mechanic text),
  `plugins/mill/scripts/_plan_validate.py` (move-check semantics incl. suppression
  sets), `plugins/mill/skills/mill-plan/SKILL.md` (fix-table semantics). Mill is
  precedent, not dependency — nothing in lyx imports mill, and this discussion's own
  Decisions carry the canonical grammar and Rename-mechanic text; the plan must be
  writable without consulting the mill repo at all.
- **Consumers to keep honest**: `digest.go`'s `Distill` (Scope-based drift — unchanged),
  `poll.go` (unchanged), `spawn.go` (template fill — prose changes only),
  `fingerprint.go`/`state.go` (check during planning whether they hash or echo plan
  fields).
- **Doc updates in the same commits** (repo Task-completion rule):
  `docs/modules/plan-format.md` is the primary deliverable; check
  `docs/modules/builder.md` (or equivalent) for v1 format references;
  `docs/overview.md` only if the module table wording changes. `docs/roadmap.md` only
  if this completes a planned milestone entry.

## Constraints

- **CONSTRAINTS.md read and applies**: Hub Geometry Invariant (no new path
  construction outside `internal/hubgeometry` — plan paths already resolve via
  hubgeometry helpers at call sites; `ParsePlan` keeps taking a plain `planDir`),
  lyxtest Leaf Invariant (untouched), CLI/Cobra Invariant (if `lyx builder validate`
  help text mentions check names, re-read `Short`/`Long` accuracy — help accuracy is a
  review obligation), Documentation Lifecycle (docs updated in the same commit).
- **Verify guardrail (task-specific, hard)**: no full test-suite run per batch —
  per-batch `verify:` stays package-scoped. Do not port mill's module-wide verify.
- **Renames use `git mv`** (repo convention): the Rename mechanic text encodes it; the
  plan for THIS task should itself declare any file renames as `Moves:`-style `git mv`
  operations (none currently anticipated).
- **plan-format.md is a pinned contract doc**: v2 replaces v1 in place with the version
  bumped; fail-loud on unrecognized `format:` is preserved (v1 plans are rejected, not
  misread).

## Testing

- **Parser (`plan_test.go`)** — TDD candidate. Table-driven fixtures under `testdata/`
  rewritten to v2. Scenarios: all five fields with `none` sentinels; `Moves:` pair
  parsing (well-formed, malformed retained raw); `root:` + `//` normalization (root
  set/absent, escape path, `Moves:` across roots); `NN.C` heading parsing; `(C cards)`
  index parsing; optional `Commit:`/per-card verify; missing-field recorded as absent
  (not a parse error); document-structure failures still hard errors (frontmatter,
  index line, unterminated fence).
- **Validator (`validate_test.go`)** — TDD candidate. One focused test per new check,
  positive and negative: the five `move-*` checks (including the earlier-batch
  suppression sets for `move-source-missing` and chained renames; on-disk collision
  for `move-target-collision`), `card-missing-field`, `card-numbering` (wrong batch
  prefix; gap; duplicate), `card-count-mismatch`, `path-missing` (suppressed by
  earlier `Creates:`/`Moves:` target), `card-outside-scope` (boundary semantics:
  `internal/foo` must not cover `internal/foobar`; `Context:` exempt),
  `commit-subject-mismatch`; `card-field-overlap` (same path twice within one card
  flagged; `Creates:` then `Edits:` across cards of the same batch NOT flagged);
  `format: 1` now failing `format-unrecognized`; adapted `batch-oversized` estimate
  over typed fields.
- **Worked example as fixture**: keep the plan-format.md worked example and a testdata
  fixture byte-consistent (v1 discipline) — the example must demonstrate all five
  fields, `NN.C` numbering, a `Commit:` field, `root:` usage, and one `Moves:` card
  with its `## Rename mechanic` section.
- **Verify command for the implementation batches**: `go test ./internal/builderengine/...`
  (package-scoped, per the guardrail).

## Q&A log

- **Q:** Are the five file-op fields required per card or optional-when-absent? **A:**
  Required, with literal `none` when empty — omissions must be mechanically detectable
  (mill's `card-missing-field` lesson).
- **Q:** Is `Context:` mill's strict allowlist or advisory? **A:** Advisory read-list;
  allowlist contradicts lyx's "declared ownership, not a cage" philosophy and is
  unenforceable; it feeds the context estimate.
- **Q:** Does `NN.C` become the actual card heading? **A:** Yes — `### Card 02.3 —
  <title>`, matching the commit-subject convention 1:1; validated.
- **Q:** Dual-version parser (v1+v2)? **A:** No — v2 only; `recognizedFormat = 2`;
  no production v1 plans exist.
- **Q:** Keep a derived flat `WhereFiles`? **A:** No — replaced outright by typed
  fields; only consumer (oversized estimate) updates in the same change.
- **Q:** Long repeated path prefixes bloat plans (observed in mill) — shorten? **A:**
  Yes: optional per-batch `root:` frontmatter; card paths resolve against it; `//`
  prefix is always worktree-root-relative (the escape for files outside the root);
  parser normalizes; Scope unaffected.
- **Q:** Card counts visible in the overview? **A:** Yes — `- NN — <slug> (C cards) —
  <intent>` in the Batch Index, with a `card-count-mismatch` check.
- **Q:** Full mill check set or lighter subset? **A:** Full port of the five `move-*`
  checks plus `card-missing-field`, `card-numbering`, `card-count-mismatch`,
  `path-missing`, `card-outside-scope`, `commit-subject-mismatch`.
- **Q:** Port "All Files Touched"? **A:** No — verified firsthand that nothing in mill
  consumes it at runtime (review bulking reads batch-file fields directly and excludes
  `Context:`; the parallel-overlap claim in mill's template is not backed by code; the
  list is derivative with a regenerate auto-fix). Long/bloaty on refactors. Any union
  is derivable from the parsed typed fields when needed.
- **Q:** The "READS" files (read-but-not-edited) are still declared, right? **A:** Yes —
  that is exactly `Context:`; note mill's All Files Touched never included `Context:`
  anyway.
- **Q:** How do implementers see Shared Decisions, given v1 forbids reading
  `00-overview.md`? **A:** Rule changed: implementer reads its batch file AND the whole
  (now small) overview; never other batches' files. Mill precedent: its implementer
  brief points at the overview's Shared Decisions.
- **Q:** Validate pinned `Commit:` messages? **A:** Yes — must start with the card's
  `NN.C: ` prefix, else the git-log resume trail breaks.
- **Q:** Extend poll/digest drift logic with typed ops now? **A:** No — out of scope;
  format+parser+validator only; follow-up task when wanted.
