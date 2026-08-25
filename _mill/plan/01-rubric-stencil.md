# Batch: rubric-stencil

```yaml
task: 'loom: Discussion-Review producer'
batch: 'rubric-stencil'
number: 1
cards: 3
verify: go test ./contracts/stencils/...
depends-on: []
```

## Batch Scope

This batch delivers the rubric itself: one new stencil file carrying the `Discussion-Review` rubric already written in `manifest/designs/loom.md`, its registration in the hand-maintained `contracts/stencils` registry, and a content test pinning the six items the design doc requires.
The external interface the later batches consume is the stencilstore name `loom-rubric-discussion-review` — batch 2's `rubric_stencil` key resolves it, and batch 5's two recipe rows both name it.
Nothing outside `contracts/stencils/` is touched, so this batch is independent of every other batch in the plan.

## Cards

### Card 1: write the loom-rubric-discussion-review stencil

- **Context:**
  - `manifest/designs/loom.md`
  - `contracts/stencils/bouncer/bouncer-template-judge.md`
  - `contracts/stencils/bouncer/bouncer-template-seed.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `contracts/stencils/loom/loom-rubric-discussion-review.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The new file is the rubric the `Discussion-Bouncer` and `Discussion-Burler` rows both apply.
  It is interpolated into `bouncer-template-seed` and `bouncer-template-judge` as the value of their `{{.rubric}}` marker (and into `internal/burlerengine`'s own round prompt the same way), so it is a marker VALUE, never a template: it must contain no `{{.X}}` markers of its own.
  Open the file with a leading HTML comment, in the same shape as the two bouncer stencils' own leading comments, stating that this is the `Discussion-Review` rubric, that it is read by both rows of the perch, and that `internal/stencil`'s `StripLeadingComment` removes this comment before interpolation.
  The body transcribes the rubric already written in the two `Discussion-Review rubric` subsections of `manifest/designs/loom.md` — this is a transcription, not a new rubric: do not invent criteria the design doc does not state.
  The body must cover all six items:
  the three do-not-flag items — a missing optional "Notes for the plan writer" subsection is never a deficiency; missing rejected alternatives in `decision-record.md` are by design because rejected alternatives belong in `support-log.md`; incomplete call-site or cross-reference enumeration belongs to the compiler and to `Plan-Sweep`, not to this review —
  and the three also-flag items — relocation and exclusion findings are legitimate on equal footing with gap-filling findings; the completeness-before-leanness test (before any relocation finding, check whether the content carries a requirement or constraint the plan writer needs, extract that into `decision-record.md`'s own Decisions or Constraints first, and move only the surrounding deliberation narrative, because `Plan-Write` never reads `support-log.md`); and the writer/reviewer symmetry note (whatever the discussion writer's own stencil says not to gather, this rubric must not flag as missing).
  State once, near the top, that the subject under review is the discussion artifact pair and that the mechanical section contract is enforced upstream by `Discussion-Validate` and is not this rubric's subject.
  The rubric must not restate or contradict the Review Round Invariant's round discipline (A-before-B, every recorded finding fixed in B, no self-grading, commit-per-fix, never push) — `internal/burlerengine` already implements and states it, and duplicating it here is exactly what the Producer Pointer-Rule Invariant forbids.
  Follow CLAUDE.md's markdown rule: semantic line breaks, one sentence per line, no fixed-column hard-wrap.
- **Commit:** `feat(stencils): add the loom-rubric-discussion-review rubric stencil`

### Card 2: register the rubric stencil

- **Context:**
  - `contracts/stencils/registry_test.go`
- **Edits:**
  - `contracts/stencils/stencils.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `LoomRubricDiscussionReview` byte var with a `//go:embed loom/loom-rubric-discussion-review.md` directive and a one-line doc comment, placed immediately after the existing `LoomTemplatePlan` var so the loom family stays contiguous.
  Add `{"loom-rubric-discussion-review", &LoomRubricDiscussionReview}` to the `entries` slice, immediately after the existing `{"loom-template-plan", &LoomTemplatePlan}` row, so `lyx stencil list`'s print order keeps the loom family together.
  The Stencil Ownership Invariant requires both halves — the embed and the registry row — so a file present in the tree but absent from `entries` is what `contracts/stencils/registry_test.go` already fails on.
- **Commit:** `feat(stencils): register loom-rubric-discussion-review`

### Card 3: pin the rubric's required content

- **Context:**
  - `contracts/stencils/registry_test.go`
  - `internal/burlerengine/template_test.go`
  - `contracts/stencils/loom/loom-rubric-discussion-review.md`
- **Edits:** none
- **Creates:**
  - `contracts/stencils/rubric_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestLoomRubricDiscussionReview_NamesEveryRequiredItem` in package `stencils`, asserting that the `LoomRubricDiscussionReview` bytes contain a distinctive phrase for each of the six items card 1 lists — three do-not-flag, three also-flag.
  Follow `internal/burlerengine/template_test.go`'s `TestTemplate_StatesRoundDiscipline` as the precedent: assert on short, distinctive phrases via a table of substrings, never on whole paragraphs, so ordinary prose edits do not break the test.
  Add a second assertion in the same file, `TestLoomRubricDiscussionReview_CarriesNoStencilMarkers`, asserting the bytes contain no `{{.` substring — the rubric is interpolated as a marker value, and a marker inside it would either render literally into the judge prompt or, worse, be silently swallowed.
  Do not assert on the leading HTML comment's text: `internal/stencil`'s `StripLeadingComment` removes it before either consumer ever sees it, so it is not part of the contract.
- **Commit:** `test(stencils): pin loom-rubric-discussion-review's required content`

## Batch Tests

`verify: go test ./contracts/stencils/...` runs the whole `contracts/stencils` package: the pre-existing `registry_test.go` (which card 2 must satisfy in both directions — every on-disk stencil registered, every registered name resolvable) and card 3's new `rubric_test.go`.
The package has no other test files, so this is the exact scope of what the batch touches.
Card 2's registry edit is expected to satisfy `registry_test.go` with no test edit — confirm that by running the verify rather than assuming it.
