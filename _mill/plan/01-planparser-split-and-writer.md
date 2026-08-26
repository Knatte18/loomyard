# Batch: planparser-split-and-writer

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
batch: 'planparser-split-and-writer'
number: 1
cards: 4
verify: go test ./internal/planparser/...
depends-on: []
```

## Batch Scope

This batch does all of the `internal/planparser` work and nothing else: it splits the package's single validation entry point into a format-only `ValidateFormat` and the existing approval-enforcing `Validate`, and it gives the package its first write path, `SetApproved`.
Both are pure additions to the package's exported seam — no existing call site anywhere in the repo changes signature, so the tree still builds and every other package still behaves identically after this batch.

The external interface the next batches consume is exactly two new exported functions: `planparser.ValidateFormat(plan *Plan, worktreeRoot string) []ValidationError`, which batch 3's `loomshed` two-mode producer and batch 5's `--require-approved`-absent CLI verb both call, and `planparser.SetApproved(planDir string) error`, which batch 5's `Env.ApprovePlan` closure calls.

Batch-local decision: `Validate` splices the `plan-unapproved` finding at position two rather than appending it last, so the sixteen-finding order the format spec pins is preserved byte-for-byte and that spec's ordered list needs no renumbering.
Both exported functions are thin wrappers over one unexported `validate` taking a `requireApproved bool` — that bool is an implementation detail of ordering, not a second exported seam.

## Cards

### Card 1: Split the validation entry point into ValidateFormat and Validate

- **Context:**
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `internal/planparser/validate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `checkFormatAndApproval` with two helpers, `checkFormatRecognized(plan *Plan) []ValidationError` emitting only the `format-unrecognized` finding and `checkApproved(plan *Plan) []ValidationError` emitting only the `plan-unapproved` finding.
  Neither finding's `Check` ID and neither one's `Detail` string changes.
  Add an unexported `validate(plan *Plan, worktreeRoot string, requireApproved bool) []ValidationError` holding the dispatch list currently inside `Validate`: it appends `checkFormatRecognized`, then `checkApproved` when and only when `requireApproved` is true, then the remaining fourteen `check*` calls in their existing order, starting at `checkIndexFileConsistency` and ending at `checkCommitSubjectMismatch`.
  Keep `Validate(plan *Plan, worktreeRoot string) []ValidationError` with its current exported signature, now returning `validate(plan, worktreeRoot, true)`, so its sixteen-finding order is unchanged and `plan-unapproved` still occupies position two.
  Add `ValidateFormat(plan *Plan, worktreeRoot string) []ValidationError` returning `validate(plan, worktreeRoot, false)` — the same fifteen IDs minus `plan-unapproved`.
  Give each exported function a godoc comment stating which check IDs it covers and, for `ValidateFormat`, that approval is deliberately not its business because the flag is written after the review segment settles.
  Update the file's own package doc comment at the top of the file: it currently attributes the first two check IDs to `checkFormatAndApproval`, a function this card removes, so re-attribute `format-unrecognized` to `checkFormatRecognized` and `plan-unapproved` to `checkApproved`, and state that `ValidateFormat` covers fifteen of the sixteen while `Validate` covers all sixteen.
- **Commit:** `1: planparser: split Validate into ValidateFormat and Validate`

### Card 2: Add SetApproved, planparser's first plan-format write path

- **Context:**
  - `internal/planparser/parse.go`
- **Edits:**
  - `internal/planparser/doc.go`
- **Creates:**
  - `internal/planparser/approve.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `SetApproved(planDir string) error` in the new file `internal/planparser/approve.go`, with a file-level comment naming it the package's first write path and stating why the write must live here (the Planparser Sole-Parser Invariant reserves plan-format writes to this package).
  It resolves the overview as `filepath.Join(planDir, overviewFileName)` — reusing the package's own existing `overviewFileName` constant rather than a new literal, and never composing the `_lyx` literal itself, per the Lyxdirs Single-Declarer Invariant.
  It reads the file, separates the leading frontmatter block with the package's existing `splitFrontmatter` helper, rewrites only the `approved:` line's value to `true`, and writes the file back.
  Every other byte survives: the remaining frontmatter keys, their order, the `---` fences, the framing paragraph, the Card Index, and every plan-level body section.
  Do not parse-and-re-serialize the frontmatter through the YAML decoder — a round-trip would not preserve the body and would reorder or requote the keys.
  It is idempotent: an overview already carrying `approved: true` is a successful no-op leaving the file byte-identical.
  When the frontmatter block is otherwise well-formed but carries no `approved:` key at all, insert `approved: true` into the block rather than failing, so the seam is total over every plan `ParsePlan` accepts.
  A missing overview file, an unreadable overview file, and a file with no `---`-fenced frontmatter block are each an error, wrapped with the package's existing `planparser:` error prefix convention.
  In `internal/planparser/doc.go`, widen the package doc's opening sole-parser statement to say the package is the sole parser and the sole writer of the on-disk plan format, and name `SetApproved` as the one write path.
- **Commit:** `2: planparser: add SetApproved, the plan format's sole writer`

### Card 3: Re-point the validate tests at the two-function split

- **Context:**
  - `internal/planparser/validate.go`
  - `contracts/specs/loom-plan-spec.md`
- **Edits:**
  - `internal/planparser/validate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `TestValidate_FormatAndApproval` — the existing direct test of the check this batch split — with two tests.
  The first drives `ValidateFormat` and asserts `format-unrecognized` still fires on an unrecognized `format:` value and that `plan-unapproved` never appears in its findings regardless of whether the fixture's `approved:` is `true`, `false`, or absent.
  The second drives `Validate` and asserts `plan-unapproved` fires exactly when the parsed plan's `Approved` is false and never when it is true, with `format-unrecognized` behaving identically to the first test.
  Add one further case asserting `Validate`'s finding order still matches the spec's fixed order over a plan that trips both of the first two checks at once: `format-unrecognized` first, `plan-unapproved` second, and any remaining findings after them.
  Leave every other test in the file untouched — they all drive `Validate` and their expectations are unchanged by the split.
- **Commit:** `3: planparser: cover the ValidateFormat/Validate split`

### Card 4: Table test for SetApproved

- **Context:**
  - `internal/planparser/approve.go`
  - `internal/planparser/parse.go`
  - `internal/planparser/validate_test.go`
- **Edits:** none
- **Creates:**
  - `internal/planparser/approve_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write a table test for `SetApproved` covering: an overview carrying `approved: false` flips to `true`;
  an overview already carrying `approved: true` is an idempotent no-op whose file bytes are identical afterwards;
  an otherwise valid frontmatter block with no `approved:` key at all gets `approved: true` inserted;
  every other frontmatter key and the key ordering survive unchanged;
  an overview carrying a `root:` key plus a multi-section body (framing paragraph, Card Index, and at least one plan-level `##` section) survives byte-for-byte outside the single rewritten line;
  a plan directory with no overview file returns an error;
  and a file with no `---`-fenced frontmatter block returns an error.
  In at least the flip case and the insert case, round-trip the result through `ParsePlan` and assert the returned plan's `Approved` field is true, so the writer and the parser are pinned against each other rather than against a hand-written expectation.
  Follow the existing fixture style in this package's tests — build each fixture under `t.TempDir()`, never under a system temp path chosen by hand.
- **Commit:** `4: planparser: table test SetApproved`

## Batch Tests

`verify: go test ./internal/planparser/...` runs this package's whole test suite, which is the exact scope the batch touches: `validate_test.go` (card 3) and the new `approve_test.go` (card 4), plus the package's existing parse, sections, normalize, classify, and planpath tests, which card 1 and card 2 must leave passing unchanged.
The package is a tier-1 pure leaf — no git spawn, no process spawn — so the whole suite is fast and there is no reason to scope narrower than the package.
