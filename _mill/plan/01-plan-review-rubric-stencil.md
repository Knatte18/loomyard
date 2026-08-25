# Batch: plan-review-rubric-stencil

```yaml
task: 'loom: Plan-Review producer'
batch: 'plan-review-rubric-stencil'
number: 1
cards: 3
verify: go test ./contracts/stencils/...
depends-on: []
```

## Batch Scope

This batch delivers the one genuinely new artifact in the task: `Plan-Review`'s rubric stencil, written from scratch because no rubric has ever existed for the format-4 Card model.
It is one batch because all three cards live in `contracts/stencils`, share the same Context set, and are meaningless apart — a stencil file with no `entries` row is invisible to `lyx stencil list` and is never seeded, and a registration with no file fails `TestRegistry_MatchesOnDiskTree`.

The external interface the later batches consume is the registered stencil **name**, `loom-rubric-plan-review`, and the exported Go identifier `stencils.LoomRubricPlanReview`.
Batch 3's recipe rows name the former;
batch 3's `internal/loomrecipe` fixture seeds from the latter.

Batch-local decision, differing from no `## Shared Decisions` entry but worth stating here: the rubric **points at** `contracts/specs/loom-plan-spec.md` and `manifest/designs/plan-card-format.md` by name and never restates their content, per the Producer Pointer-Rule Invariant.
It does **not** point at `manifest/designs/loom.md`'s own rubric section — that section is the doc *about* this stencil, and pointing at it would be circular.

## Cards

### Card 1: Write the Plan-Review rubric stencil

- **Context:**
  - `contracts/stencils/loom/loom-rubric-discussion-review.md`
  - `contracts/stencils/bouncer/bouncer-template-judge.md`
  - `contracts/specs/loom-plan-spec.md`
  - `manifest/designs/plan-card-format.md`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the new file with exactly the content in the fenced block below, byte-for-byte, including the leading HTML comment and the trailing newline.

  Two properties are load-bearing and must survive any later prose edit.
  First, the file is a marker **value**, never a template: it carries no `{{.` substring anywhere, because it is interpolated into two Bouncer prompt templates' rubric marker and into the Burler round prompt the same way, and a marker inside it would render literally or be silently swallowed.
  Second, it opens with an HTML comment that `internal/stencil`'s `StripLeadingComment` removes before either consumer sees it — the same slot `stencilstore`'s own stamp banner merges into.

  Follow the repo's semantic-line-break rule: one sentence per line, plus a break at internal independent-clause boundaries, never a fixed-column hard wrap.

````markdown
<!-- This is the Plan-Review rubric. It is read by both rows of the Plan-Review perch:
     the Plan-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Plan-Burler row interpolates it the
     same way into internal/burlerengine's own round prompt.
     It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Plan-Review rubric

The subject under review is the current plan: `_lyx/plan/00-overview.md` and the card files its Card Index names.
The plan directory may also hold `archive-*/` subdirectories, which are rotations of superseded plans;
they are out of scope, and a finding raised against one is never legitimate.

The format contract is `contracts/specs/loom-plan-spec.md`, and the Card model it implements is described in `manifest/designs/plan-card-format.md`.
This rubric points at both and restates neither.
The mechanical checks over that contract are already enforced upstream by `Plan-Validate`.

`Plan-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
Sitting directly downstream of a sixteen-check mechanical validator makes this gate's over-flagging surface larger than that of a gate with no validator ahead of it, not smaller.

**`support-log.md` is outside this review entirely.**
It appears in neither the artifact list nor the answer key, and it must not be read or reasoned from.
`Plan-Write` provably never reads it, so a finding grounded in its content cannot be satisfied except by inventing the missing link.

## Do not flag

Do not flag any of the following as a finding:

- **Anything `Plan-Validate` already checks.**
  The sixteen check IDs `contracts/specs/loom-plan-spec.md`'s own validation-checks section lists, `format-unrecognized` through `commit-subject-mismatch`, are enforced deterministically upstream.
  Re-deriving them here is duplicated work whose only possible outcome is disagreement with the parser.
- **A missing `DependsOn`/`Produces` field, or an incomplete dependency list.**
  Dependency edges are derived, never authored — a card's `Uses` intersected against every other card's target list.
  Plan-time completeness of that intersection is explicitly not provable;
  the real gate is the post-merge build and test.
- **A `Rename`, `Move`, `Prosa`, or `Custom` card carrying no `ImpactSummary`.**
  It is required for `Edit` and `Delete` only, per the per-type table in `manifest/designs/plan-card-format.md`.
  For `Rename` the reason is specific: a correctly executed AST-aware rename is binary, with no graded blast radius to summarise.

## Also flag

- **Granularity.**
  One card per independently reviewable/testable unit, not one card per literal symbol.
  A private supporting type, or a constructor inseparable from its type, belongs in the other symbol's card;
  an independently testable symbol gets its own card even when one card is its only consumer.
- **`ImpactSummary` carries a real conclusion.**
  A one-line blast-radius conclusion — "3 callers, all local to the billing package, no cross-module effects" — never a restatement of `Intent`.
- **`Custom` is a last resort.**
  Used only where none of `Create`, `Edit`, `Delete`, `Rename`, `Move`, or `Prosa` genuinely fits, never as a shortcut around correct typing.
  A `Custom` card is exempt from `path-missing` on its own targets and from `prosa-symbol-target`, so a mistyped one silently escapes two checks the rest of the plan is held to.
- **Fidelity to the decision record.**
  Every Decision and every Constraint in `_lyx/discussion/decision-record.md` is carried by some card, and no card introduces scope that file does not license.
  That path is anchor-relative: it resolves from this session's own working directory, and it is deliberately not the absolute form the artifact list uses.
  The decision record is the measuring stick and never the subject — every finding is raised against the plan, never against the decision record.
````

- **Commit:** `feat(stencils): add the Plan-Review rubric stencil`

### Card 2: Register the rubric in the stencil registry

- **Context:**
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
- **Edits:**
  - `contracts/stencils/stencils.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the embedded-default var `LoomRubricPlanReview` directly after the existing `LoomRubricDiscussionReview` var, with its own doc comment and its own `//go:embed loom/loom-rubric-plan-review.md` directive, following the shape every other var in this file already uses.
  The doc comment names it as the Plan-Review rubric, read by both rows of the Plan-Review perch.

  Add the matching `entries` row `{"loom-rubric-plan-review", &LoomRubricPlanReview},` directly after the existing `{"loom-rubric-discussion-review", &LoomRubricDiscussionReview},` row.
  Position matters: `entries` order is `lyx stencil list`'s print order, and keeping the two loom rubrics adjacent is what makes that listing readable.

  Do not touch `registryEntry`, `Names`, `Default`, or `Registry` — this file's registration surface is exactly the var plus the row.
  Do not add a count assertion anywhere;
  this file declares no count.
- **Commit:** `feat(stencils): register loom-rubric-plan-review in the stencil registry`

### Card 3: Pin the rubric's required content and its marker-free shape

- **Context:**
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `contracts/stencils/stencils.go`
  - `contracts/stencils/registry_test.go`
- **Edits:**
  - `contracts/stencils/rubric_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestLoomRubricPlanReview_NamesEveryRequiredItem`, following the existing `TestLoomRubricDiscussionReview_NamesEveryRequiredItem` as the precedent exactly: a table of `{name, phrase}` pairs driven as subtests over `string(LoomRubricPlanReview)`, each assertion a short distinctive substring rather than a whole paragraph, so ordinary prose edits do not break the test.

  Cover eight items — the four "Also flag" items, the three "Do not flag" items, and the named support-log exclusion — with these exact phrases, written as Go string literals:

```go
"independently reviewable/testable unit"
"blast-radius conclusion"
"is a last resort"
"_lyx/discussion/decision-record.md"
"commit-subject-mismatch"
"Dependency edges are derived, never authored"
"no graded blast radius to summarise"
"support-log.md"
```

  Each subtest's `name` field states which rubric item the phrase stands for, following the precedent test's naming style.

  Add `TestLoomRubricPlanReview_CarriesNoStencilMarkers`, mirroring the existing `TestLoomRubricDiscussionReview_CarriesNoStencilMarkers`: assert `string(LoomRubricPlanReview)` contains no `{{.` substring.
  This is the test that catches a rubric author reaching for a template marker, and it must exist for the new stencil, not only the shipped one.

  Update the file's own header comment, which today names only the Discussion rubric and its six items: it now describes both rubrics, naming the new file and its eight pinned items alongside the existing six.

  Leave `TestLoomRubricDiscussionReview_NamesEveryRequiredItem` and `TestLoomRubricDiscussionReview_CarriesNoStencilMarkers` untouched.
- **Commit:** `test(stencils): pin the Plan-Review rubric's required items and marker-free shape`

## Batch Tests

`verify: go test ./contracts/stencils/...` runs the whole `contracts/stencils` package — `rubric_test.go` and `registry_test.go` together.
That scope is deliberate rather than a single-file run: card 1 and card 2 are exactly the pair `registry_test.go`'s `TestRegistry_MatchesOnDiskTree` cross-checks in both directions (a `.md` on disk with no `entries` row, and an `entries` row with no `.md`), and `TestRegistry_DefaultsAndRelPathAreConsistent` additionally proves `stencilstore.RelPath("loom-rubric-plan-review")` resolves to the file's real relative path under the `loom/` family folder.
Neither test carries a hardcoded stencil count, so both pass on the added pair without an edit — verify that rather than assume it;
if either turns out to carry a count, increment it in card 2.

`rubric_test.go`'s two new tests are the batch's own assertions: the eight-item content pin and the no-`{{.`-marker pin.
