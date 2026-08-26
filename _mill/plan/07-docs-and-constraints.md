# Batch: docs-and-constraints

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
batch: 'docs-and-constraints'
number: 7
cards: 5
verify: go test ./internal/lyxcwd/...
depends-on: [6]
```

## Batch Scope

This batch lands every doc the change obliges, per the Documentation Lifecycle: the module design doc, the pinned format spec, the two loom stencils, one Go package doc, and two `CONSTRAINTS.md` entries.
It is last because every statement it makes is about behaviour batches 1 through 6 have already landed, and it is one batch because the same two facts — that `plan-unapproved` moved downstream, and that the approval flag is written by `Plan-Bouncer`'s approved settle — thread through all five files.

`manifest/roadmap.md` deliberately does not move: this is a defect fix on a shipped module, not the completion of a planned item.
`internal/websterengine`, `internal/webstercli`, and `internal/batcher` are untouched in code and need no doc change either — they run after approval and their refusal of an unapproved plan is unchanged.

Batch-local decision: the rubric and the fixer instructions keep their contiguous-range phrasing and carve out the one exception rather than editing a bare count.
`plan-unapproved` sits at position two *inside* the range "`format-unrecognized` through `commit-subject-mismatch`", so "fifteen … through `commit-subject-mismatch`" would be self-contradictory.

## Cards

### Card 27: Update the loom design doc

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/planvalidate.go`
  - `internal/loomcli/validate.go`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the pipeline table's row 8 and row 10 entries so each names the mode its row runs in — row 8 the format-only mode that no longer demands the approval flag, row 10 the approval-enforcing mode that confirms it — and so row 10's description names the approval check as the second thing it catches after a fixer-introduced format regression.
  Update the `Plan-Validate detail` subsection, which currently states that the verb and the row make the same three `planparser` calls ending at `planparser.Validate`: the third call is now one of two functions, chosen by the row's `require_approved` key and by the verb's `--require-approved` flag, and the parity claim now reads that the verb reaches every mode the row set uses.
  In the `Plan-Review rubric` subsection's do-not-flag list, apply the same carve-out the rubric stencil itself gets: keep the contiguous range and say fifteen of the sixteen are enforced upstream by `Plan-Validate` while `plan-unapproved` is enforced downstream by `Plan-Revalidate`.
  Add a short statement, wherever the row 9 segment is described, that the approval flag is written by that segment's `Bouncer` on its approved settle, before its commit seam fires, so the flag lands inside the commit.
  Keep every existing markdown link resolvable and add no link whose target or anchor does not exist.
- **Commit:** `27: docs: loom design doc for the two-mode plan gate`

### Card 28: Update the pinned plan-format spec

- **Context:**
  - `internal/planparser/validate.go`
- **Edits:**
  - `contracts/specs/loom-plan-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the validation-checks section, keep the sixteen numbered rows in their existing order and do not renumber them — `plan-unapproved` stays at position two, which is what `planparser.Validate` still emits.
  Reframe the section's own preamble so it states that the sixteen IDs are split across two entry points: fifteen of them are the format-only set, and `plan-unapproved` is additionally checked by the full entry point, with the two named as `ValidateFormat` and `Validate`.
  Reframe the `plan-unapproved` row itself so its existing "else refuse to run" consumer-guard framing is made explicit: it names which callers enforce it — the post-segment `Plan-Revalidate` row and every standalone plan consumer — and states that the pre-review gate deliberately does not, because the writer is forbidden from setting the flag and the review segment is what writes it.
  Keep the section's opening count sentence accurate: it still describes sixteen distinct IDs.
- **Commit:** `28: docs: spec the ValidateFormat/Validate split`

### Card 29: Correct the two loom stencils

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomcli/validate.go`
- **Edits:**
  - `contracts/stencils/loom/loom-template-plan.md`
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the plan template stencil, keep `approved: false` in both the frontmatter block and the minimal skeleton, and keep the rule sentence "Always write `approved: false` — you never self-approve" verbatim.
  Correct only its trailing clause, which currently promises that a future review gate flips the value: name `Plan-Bouncer`'s approved settle as the row that writes it, in the present tense.
  Leave the stencil's Step 5 self-check block **verbatim**, including its statement that the verb takes no arguments and its instruction to re-run it until it exits 0 — the verb's new default mode is exactly what makes that instruction satisfiable over the unapproved plan the writer is ordered to produce, so the wording needs no change.
  Do not add the new flag to this stencil: doing so would re-impose the deadlock on the one agent that must never satisfy it.
  In the plan-review rubric stencil, apply the carve-out to both places that name the check set — the sentence describing the gate as sitting downstream of a sixteen-check mechanical validator, and the do-not-flag entry naming the contiguous range: keep the range and say fifteen of the sixteen are enforced upstream by `Plan-Validate` while `plan-unapproved` is enforced downstream by `Plan-Revalidate`.
  Leave the don't-re-derive instruction itself exactly as it is — the judge must not re-derive the approval flag in either direction, upstream or downstream.
- **Commit:** `29: docs: correct the plan stencil and review rubric`

### Card 30: Correct the plan producer's package doc

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/stencils/loom/loom-template-plan.md`
- **Edits:**
  - `internal/loomengine/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The file-level doc comment currently says the producer always writes `approved: false` and that flipping it to `true` after the review segment returns APPROVED is the future loom orchestrator's job, "not built here".
  Keep the first half — the producer still always writes `approved: false` and still has no review logic of its own — and correct the second: name `Plan-Bouncer`'s approved settle as the row that writes the flag, dropping the future tense and the "not built here" clause.
  Change no code in the file;
  this card is the doc comment alone.
- **Commit:** `30: docs: correct loomengine plan producer package doc`

### Card 31: Record the two invariants

- **Context:**
  - `internal/planparser/approve.go`
  - `internal/planparser/doc.go`
  - `internal/loomcli/parity_test.go`
  - `internal/loomcli/validate.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Widen the Planparser Sole-Parser Invariant from parse-only to parse-and-write: the package is the sole parser *and* the sole writer of the on-disk plan format, with the same "no other package parses or writes the overview file or the card files" bullet shape and the same review-obligation enforcement note the entry already carries.
  Name `SetApproved` as the one write path and keep the existing path-ownership and never-resolves-cwd bullets unchanged.
  Leaving the sole-writer property as an unwritten reading of a parse-only invariant is how the next task grows a second writer somewhere else, so state it.
  Update the Gate Self-Check Parity Invariant to say the verb reaches every mode its row set uses, not merely that the row and the verb call the same function: the plan gate is now one engine in two modes across two rows, and its today's-two-instances bullet must name both modes and both `planparser` entry points.
  Keep its structural `findings`-key discrimination bullet and its enforcement pointer at the parity test unchanged.
  Change no other entry in the file.
- **Commit:** `31: constraints: sole-writer and two-mode gate parity`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` is the one mechanical check that reaches this batch's edits.
That package holds the Markdown Link Integrity enforcement test, which scans every `.md` file under `manifest/` and `docs/` and resolves both the file part and the `#anchor` of every inline link — so card 27's edits to the loom design doc, and any link into `CONSTRAINTS.md` whose anchor card 31's heading edits could invalidate, are covered there and nowhere else.

The remaining four cards have no mechanical gate by design: `contracts/specs/`, `contracts/stencils/`, and `CONSTRAINTS.md` are review obligations, and card 30 edits a Go doc comment with no behavioural surface.
The repo-wide compile-and-test sweep that proves card 30 did not disturb `internal/loomengine` is already `pipeline.done_gate`'s job at the end of the run, so it is deliberately not duplicated here.
