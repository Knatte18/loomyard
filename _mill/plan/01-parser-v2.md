# Batch: parser-v2

```yaml
task: "Add typed file-ops to lyx's plan-format"
batch: "parser-v2"
number: 1
cards: 5
verify: go test ./internal/builderengine/...
depends-on: []
```

## Rename mechanic

Not applicable — this batch has no `Moves:` entries.

## Batch Scope

This batch delivers the complete plan-format v2 parser: the typed per-card model
(`PlanCard`, `MovePair`), the five file-op fields with `none` sentinels, the
`### Card NN.C — <title>` heading grammar, `root:`/`//` path normalization, the
`(C cards)` Batch Index segment, the `format: 2` version bump, and the removal of
`WhereFiles`/`CardCount`. It also mechanically adapts `validate.go`'s existing six
checks (the batch-oversized estimate and card cap read the new fields) so the package
compiles and stays green — NEW checks land in batches 2 and 3. All three testdata
fixture sets are rewritten to v2 here, designed per the `fixture-self-reference`
Shared Decision so they stay zero-findings when later batches add checks. The external
interface batches 2-4 consume: the `PlanCard`/`MovePair` types, `PlanBatch.Cards`,
`PlanBatch.Root`, `PlanBatch.IndexCardCount`, and normalized-path semantics.

Batch-local decision: intermediate cards keep the production tree compiling (per-card
`verify: go build ./internal/builderengine/...` where cheap); test files may be broken
mid-batch — the batch `verify:` at the end is the gate (card 5 fixes all tests).

## Cards

### Card 1: PlanCard model + card sub-parser, WhereFiles/CardCount removed

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/plan-format.md`
- **Edits:**
  - `internal/builderengine/plan.go`
  - `internal/builderengine/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `plan.go`, add `type MovePair struct { Old, New string }` (normalized paths)
    and `type PlanCard struct` with fields: `BatchPrefix int` (the `NN` as written in
    the heading — kept so the batch-3 `card-numbering` check can compare it against
    the batch's own number), `Number int` (the `C` part), `Title string`,
    `ContextFiles, EditsFiles, CreatesFiles, DeletesFiles []string` (normalized;
    `nil` when the field is absent — distinct from present-but-`none`, which parses
    to an empty non-nil slice), `Moves []MovePair`, `MovesRaw []string` (sub-bullets
    under `Moves:` that failed the pair grammar, retained verbatim),
    `HasContext, HasEdits, HasCreates, HasDeletes, HasMoves bool` (field-label
    presence, for `card-missing-field`), `Commit string` (empty when absent),
    `VerifyCommand string` (the card's optional `**verify:**` line, empty when
    absent).
  - On `PlanBatch`: add `Cards []PlanCard`; DELETE `WhereFiles` and `CardCount`
    (grep confirms no consumers outside this package's plan.go/validate.go/tests).
  - Replace `parseCardsSection` with a card sub-parser: split the `## Cards` section
    at `### ` headings via a new heading regex accepting
    `^###\s+Card\s+(\d{2})\.(\d+)\s*(?:—|-{1,2})\s*(.*)$` (em dash or one-or-two
    ASCII hyphens, mirroring `indexLineRe`'s tolerance). A `### ` heading inside
    `## Cards` that does NOT match the card-heading shape is a fail-loud parse error
    naming the line (document structure, not card-level detail).
  - Within a card, scan lines for field labels `**What:**`, `**Context:**`,
    `**Edits:**`, `**Creates:**`, `**Deletes:**`, `**Moves:**`, `**Commit:**`,
    `**verify:**` (exact bold-label prefixes, matching v1's `whereLinePrefix`
    approach). For the five file-op fields: inline literal `none`
    (case-insensitive, matching mill) yields the empty non-nil slice; otherwise
    consume subsequent `- `-prefixed lines until the next label/heading, strip the
    bullet marker and surrounding backticks (a bullet whose payload is not
    backtick-wrapped is retained as-is after trimming — well-formedness is
    validator territory). For `Moves:`: each bullet matching
    ``^`([^`]+)` -> `([^`]+)`$`` (after bullet-strip) becomes a `MovePair` (both
    sides normalized per card 2); non-matching bullets go verbatim into `MovesRaw`.
  - `**Commit:**` takes the rest of the line, with surrounding backticks stripped.
    `**verify:**` takes the rest of the line verbatim (v1 card-verify semantics).
    `**What:**` prose is not stored (v1 precedent: Intent comes from the index; the
    card prose is for the implementer) — but record `HasWhat bool` on `PlanCard` so
    `card-missing-field` can flag a card without a `What:`.
  - In `validate.go`, adapt the two consumers mechanically so the package compiles
    and existing tests stay green: `checkBatchOversized` sums `pathSizeOnDisk` over
    `b.Scope` plus every card's `ContextFiles`/`EditsFiles`/`CreatesFiles`/
    `DeletesFiles` and both sides of every `Moves` pair (nonexistent paths
    contribute 0, so Creates/Move-targets naturally add nothing — discussion
    `context-estimate-inputs`); the card cap compares `len(b.Cards)` instead of
    `b.CardCount`. Update the two file-banner comments (`plan.go`, `validate.go`)
    from "plan-format v1" wording to v2.
- **verify:** go build ./internal/builderengine/...
- **Commit:** `01.1: plan.go typed card model; drop WhereFiles/CardCount`

### Card 2: per-batch root: frontmatter + // path normalization

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add `Root *string \`yaml:"root"\`` to `batchFrontmatter` and `Root string` to
    `PlanBatch` (empty when absent).
  - Add `func normalizeCardPath(root, raw string) string`: trim whitespace; if the
    path starts with `//`, strip exactly that prefix and return the remainder
    (worktree-root-relative — ALWAYS, whether or not root is set); otherwise, when
    `root != ""`, return `root + "/" + raw`; otherwise return `raw` unchanged. Do
    NOT reject absolute paths or `..` here — `scope-malformed` (batch 3) owns
    well-formedness. Apply it to every path stored into `PlanCard`'s five field
    slices and both `MovePair` sides. `MovesRaw` entries are NOT normalized (they
    are retained verbatim for `move-format`).
  - Godoc on `PlanBatch.Root` and `normalizeCardPath` states the three-case rule
    and that `## Scope` entries are NOT root-resolved (Scope stays
    worktree-relative — discussion `per-batch-root-path-shorthand`).
- **verify:** go build ./internal/builderengine/...
- **Commit:** `01.2: per-batch root: + // card-path normalization`

### Card 3: Batch Index (C cards) segment

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Extend `indexLineRe` (and `indexEntry`/`PlanBatch`) to the v2 entry shape
    `NN — <batch-slug> (C cards) — <one-line intent>`: capture the integer count
    from a mandatory `(\d+ cards?)` segment between slug and the second separator
    (accept singular `(1 card)`). Store as `IndexCardCount int` on `indexEntry` and
    `PlanBatch`.
  - An index line WITHOUT the `(C cards)` segment is the existing fail-loud
    "unparseable batch index line" error (document structure, per
    `lenient-card-parse`). Comparing the count against the batch file's actual
    cards is batch 3's `card-count-mismatch` — no comparison here.
- **verify:** go build ./internal/builderengine/...
- **Commit:** `01.3: mandatory (C cards) segment in Batch Index entries`

### Card 4: format 2 + v2 testdata fixtures

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/validate.go`
  - `internal/builderengine/testdata/plan-valid/00-overview.md`
  - `internal/builderengine/testdata/plan-valid/01-json-flag.md`
  - `internal/builderengine/testdata/plan-valid/02-list-tests.md`
  - `internal/builderengine/testdata/plan-valid/03-refactor-a.md`
  - `internal/builderengine/testdata/plan-valid/04-refactor-b.md`
  - `internal/builderengine/testdata/plan-valid/05-oversized.md`
  - `internal/builderengine/testdata/plan-unapproved/00-overview.md`
  - `internal/builderengine/testdata/plan-unapproved/01-only.md`
  - `internal/builderengine/testdata/plan-broken-chain/00-overview.md`
  - `internal/builderengine/testdata/plan-broken-chain/01-first.md`
  - `internal/builderengine/testdata/plan-broken-chain/02-second.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `validate.go`, change `recognizedFormat` from 1 to 2 (a `format: 1` plan now
    trips the existing `format-unrecognized` check — no dual-version support,
    discussion Q4).
  - Rewrite all three fixture sets to v2: `format: 2` in overviews; every Batch
    Index entry gains its `(C cards)` segment; every card becomes
    `### Card NN.C — <title>` with all five file-op fields (`none` where empty),
    keeping the existing batch slugs and filenames.
  - `plan-valid` must exercise, across its five batches: (a) one batch with
    `root:` set and at least one `//`-escaped path alongside root-relative ones;
    (b) one card with a `Moves:` pair — source an existing fixture file, target a
    non-existent path — in a batch that carries a `## Rename mechanic` section
    (any non-empty body text; the batch-2 check only requires the heading);
    (c) at least one card with a `**Commit:**` field whose value starts with that
    card's own `NN.C: ` prefix; (d) the existing oversized and deferred-chain
    frontmatter cases unchanged. Per the `fixture-self-reference` Shared Decision,
    all `Context:`/`Edits:`/`Deletes:` paths and `Moves:` sources must resolve to
    files that exist inside the fixture dir (the batch .md files themselves);
    `Creates:` entries and the `Moves:` target must NOT exist and must not collide
    with each other across batches; `Edits:`/`Creates:`/`Deletes:`/`Moves:`
    endpoints must fall under the batch's `## Scope` prefixes; card numbering must
    be `NN.C`-correct and sequential — so `plan-valid` stays zero-findings through
    every batch-2/3 check.
  - `plan-unapproved` and `plan-broken-chain` change minimally: v2 syntax, same
    designed single-purpose failures (`approved: false`; dangling/deferred
    `chain-end`).
- **verify:** go build ./internal/builderengine/...
- **Commit:** `01.4: recognizedFormat=2; rewrite testdata fixtures to v2`

### Card 5: parser + validator tests updated to v2

- **Context:**
  - `_mill/discussion.md`
  - `internal/builderengine/plan.go`
  - `internal/builderengine/validate.go`
- **Edits:**
  - `internal/builderengine/plan_test.go`
  - `internal/builderengine/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Update `plan_test.go` to the v2 fixtures and the new model, following the
    file's existing table-driven style. Cover at minimum: the five typed fields
    parse with `none` sentinels (empty non-nil slice) vs absent fields (nil slice,
    `HasX == false`); `Moves:` well-formed pairs land in `Moves` and malformed
    bullets land verbatim in `MovesRaw`; `root:` + `//` normalization (root set,
    root absent, `//` escape under root, `Moves:` pair crossing the root boundary);
    `### Card NN.C` headings populate `BatchPrefix`/`Number`/`Title` (both em-dash
    and ASCII separators); a non-card `### ` heading inside `## Cards` is a parse
    error; `**Commit:**` backtick-stripping and per-card `**verify:**`;
    `IndexCardCount` from `(C cards)` and the missing-segment parse error;
    unchanged v1 document-structure errors (missing frontmatter keys, unterminated
    fence, missing `## Batch Index`, glob in Scope, `verify: deferred` +
    `## verify:` conflict).
  - Update `validate_test.go`: existing six checks stay green against the v2
    fixtures (`plan-valid` zero findings, `plan-unapproved`, `plan-broken-chain`);
    the synthetic in-memory plans switch `CardCount`/`WhereFiles` usage to
    `Cards []PlanCard` literals; `checkBatchOversized` coverage proves the estimate
    now sums Scope + card paths (and that nonexistent Creates/Move-target paths
    contribute zero).
  - Update the two test files' banner comments to v2 wording.
- **Commit:** `01.5: v2 parser and validator tests`

## Batch Tests

`verify:` runs `go test ./internal/builderengine/...` — the package's own suite:
`plan_test.go` (parser, rewritten in card 5), `validate_test.go` (existing checks
over v2 fixtures), plus the untouched sibling tests (digest, poll, chain, template,
...) which prove the model change did not ripple. Package-scoped per the
`package-scoped-verify` Shared Decision.
