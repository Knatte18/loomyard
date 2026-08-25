# Batch: rubric-stencil

```yaml
task: 'loom: Webster-Review producer'
batch: 'rubric-stencil'
number: 1
cards: 3
verify: go build ./... && go test ./contracts/stencils/... ./internal/lyxcwd/...
depends-on: []
```

## Batch Scope

This batch delivers the `Webster-Review` rubric as a shipped stencil and registers it, plus the two tests that pin its content.
It is one batch because the three pieces are inseparable: the registry row will not compile without the file, `TestRegistry_MatchesOnDiskTree` fails if either half lands alone, and the phrase-pin test is written against the exact text card 1 writes.

The external interface batch 2 consumes is the pair `stencils.LoomRubricWebsterReview` (the exported byte var, read by `internal/loomrecipe/fixture_test.go`'s stencil seeder) and the registered stencil name `loom-rubric-webster-review` (named by both recipe rows' `rubric_stencil` keys).

Batch-local decision: the rubric's structure is copied from `contracts/stencils/loom/loom-rubric-plan-review.md` — leading HTML comment, H1, framing prose, `## Do not flag`, `## Also flag` — with one extra section, `## Determining the review range`, which the two shipped rubrics have no counterpart for because their subjects are files on disk.

## Cards

### Card 1: Write the Webster-Review rubric stencil

- **Context:**
  - `manifest/designs/loom.md`
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `contracts/stencils/loom/loom-rubric-discussion-review.md`
  - `manifest/designs/plan-card-format.md`
  - `manifest/designs/code-comment-conventions.md`
  - `contracts/specs/loom-plan-spec.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create the file with exactly the content below, byte for byte.
  It is transcribed from `manifest/designs/loom.md`'s "Webster-Review rubric" section (its two "also flag" dimensions) plus the do-not-flag list and the range-derivation section the discussion settled.
  Three properties the file must hold, each machine-checked by an existing test: it contains no `{{.` substring anywhere (`contracts/stencils/rubric_test.go`), it uses no bare `weft`/`warp` token and no policed `host` phrase (`internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` walks `contracts/stencils/**/*.md`), and it follows CLAUDE.md's semantic-line-break rule — one sentence per line, plain newlines, never a trailing double space.
  Do not paraphrase or re-order the content below.

  ```
  <!-- This is the Webster-Review rubric. It is read by both rows of the Webster-Review perch:
       the Webster-Bouncer row interpolates it as bouncer-template-seed.md's and
       bouncer-template-judge.md's rubric marker value, and the Webster-Burler row interpolates it
       the same way into internal/burlerengine's own round prompt.
       It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
       internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
       sees it. -->

  # Webster-Review rubric

  The subject under review is the **committed diff**, not a file and not a directory.
  Ordinary diff review is the base: read the diff as code, with no checklist supplied, and judge it the way a careful reviewer judges any change.
  The two dimensions under `## Also flag` are added on top of that base, never a replacement for it.

  The measuring stick is the plan — `_lyx/plan/00-overview.md` and the card files its Card Index names.
  The Card model the plan implements is described in `manifest/designs/plan-card-format.md`, and the format contract is `contracts/specs/loom-plan-spec.md`.
  This rubric points at both and restates neither.

  `Webster-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
  This gate sits downstream of three separate upstream gates, so a finding re-derived from one of their subjects is duplicated work rather than coverage.

  ## Determining the review range

  This section is the single definition of the review range;
  nothing else states it.

  1. Read `_lyx/loom/status.json` and take `product.parent`, the branch this run started from.
  2. Review `git diff $(git merge-base <product.parent> HEAD)..HEAD` — every commit the current branch introduces over that merge base.

  Both steps are read-only.
  If `_lyx/loom/status.json` cannot be read, or its `product.parent` is empty or absent, raise a BLOCKING finding stating that the review range could not be determined, and review nothing.
  Silently reviewing a guessed range is a worse failure than an honest block.

  ## Do not flag

  Do not flag any of the following as a finding:

  - **Anything `Plan-Validate` or `Plan-Revalidate` already checks.**
    The plan's *format* is enforced deterministically upstream and is not this gate's subject.
  - **Findings raised against the plan itself.**
    The plan is the measuring stick and never the subject, exactly as the decision record is for `Plan-Review`.
    A plan-authoring finding cannot be satisfied by changing the diff, which is the only thing this segment can fix.
  - **A missing `ImpactSummary` on any card, or an incomplete `DependsOn`/`Produces` list.**
    Both belong to `Plan-Review`, which has already passed.
  - **Anything that is not the diff.**
    The discussion pair and the plan directory under `_lyx`, and this segment's own round artifacts under `.lyx/loom/reviews/webster/`, are never the subject of a finding.

  ## Also flag

  - **Comment-convention compliance.**
    Any new or changed doc comment follows `manifest/designs/code-comment-conventions.md`.
    This rubric points at that file and restates none of it.
  - **Per-card mechanical check.**
    Confirm the card's Type-specific mechanical check actually ran and passed — the AST-script-plus-grep for a `Rename` card, `assert-no-callers` for a `Delete` card, per the per-type table in `manifest/designs/plan-card-format.md` — not merely that the diff compiles and its tests pass.
  ```
- **Commit:** `feat(stencils): add the Webster-Review rubric stencil`

### Card 2: Register the rubric in the stencils registry

- **Context:**
  - `contracts/stencils/registry_test.go`
- **Edits:**
  - `contracts/stencils/stencils.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add the embedded byte var and its registry row, following `LoomRubricPlanReview` exactly.
  Declare `LoomRubricWebsterReview []byte` immediately after `LoomRubricPlanReview`, with the doc comment `// LoomRubricWebsterReview is the Webster-Review rubric, read by both rows of the Webster-Review` / `// perch.` and the directive `//go:embed loom/loom-rubric-webster-review.md`.
  Add `{"loom-rubric-webster-review", &LoomRubricWebsterReview},` to the `entries` slice immediately after the `{"loom-rubric-plan-review", &LoomRubricPlanReview},` row, so `lyx stencil list` prints the three loom rubrics in their segment order.
  `contracts/stencils/registry_test.go`'s `TestRegistry_MatchesOnDiskTree` then covers both directions with no new test.
- **Commit:** `feat(stencils): register loom-rubric-webster-review in the stencil registry`

### Card 3: Pin the rubric's required content

- **Context:**
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
- **Edits:**
  - `contracts/stencils/rubric_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add two tests mirroring the shipped `TestLoomRubricPlanReview_*` pair exactly in shape.
  `TestLoomRubricWebsterReview_NamesEveryRequiredItem` runs a `[]struct{ name, phrase string }` table through `strings.Contains(string(LoomRubricWebsterReview), tt.phrase)` inside a `t.Run(tt.name, ...)`, with these nine phrases, each a short distinctive substring rather than a paragraph so ordinary prose edits do not break the test: `Ordinary diff review is the base`, `git merge-base`, `could not be determined`, `Plan-Revalidate`, `measuring stick and never the subject`, `Both belong to `, `.lyx/loom/reviews/webster/`, `code-comment-conventions.md`, `assert-no-callers`.
  `TestLoomRubricWebsterReview_CarriesNoStencilMarkers` asserts `strings.Contains(string(LoomRubricWebsterReview), "{{.")` is false, with the same failure message shape the two shipped `_CarriesNoStencilMarkers` tests use.
  Rewrite the file's own header comment: it currently enumerates two rubrics and their item counts ("the six items ... the eight items ..."), and becomes three rubrics — six items for Discussion-Review, eight for Plan-Review, nine for Webster-Review — while keeping its existing statement of the marker-value-not-template constraint.
- **Commit:** `test(stencils): pin the Webster-Review rubric's required items`

## Batch Tests

`verify: go build ./... && go test ./contracts/stencils/... ./internal/lyxcwd/...`

`go build ./...` is what proves the `//go:embed` directive in card 2 resolves — a missing or misnamed stencil file is a compile error, not a test failure.
`./contracts/stencils/...` runs the two new tests plus `TestRegistry_MatchesOnDiskTree`, which is the both-directions guard over the file/registry pair.
`./internal/lyxcwd/...` is included because that package owns two enforcement walks this batch's new `.md` file falls inside: `TestEnforcement_FabricVocabulary` walks `contracts/stencils/**/*.md`, and `TestEnforcement_MarkdownLinks` covers the `manifest/` sources the rubric is transcribed from.
Neither test lives in the package this batch edits, so scoping the command to `contracts/stencils` alone would leave the vocabulary walk unrun until the done gate.
