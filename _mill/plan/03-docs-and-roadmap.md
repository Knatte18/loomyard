# Batch: docs-and-roadmap

```yaml
task: 'loom: Webster-Review producer'
batch: 'docs-and-roadmap'
number: 3
cards: 5
verify: go build ./... && go vet -tags smoke ./internal/loomcli/... && go test ./internal/lyxcwd/...
depends-on: [2]
```

## Batch Scope

This batch is the documentation half CLAUDE.md's task-completion rule requires: the module design doc, the recipe design doc, the status spec's mid-run example, the roadmap item, and the one test-file header comment whose prose (not its assertions) this task falsifies.
It is one batch because every card states a fact about the shipped list that only becomes true once batch 2 lands, and none of them changes runnable behaviour.

It depends on batch 2 rather than running beside it so no doc in the tree ever claims a perch the recipe does not yet carry.

Batch-local decision: the row-count sweep here is governed by the overview's row-count criterion, not by a token search.
`manifest/designs/loom.md` carries both kinds of "sixteen" — three producer-row counts and one `planparser` check-ID count — so it cannot be swept with a blind replace.

## Cards

### Card 12: Record the shipped perch in loom's design doc

- **Context:**
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** five edits.

  (a) Producer table, row 13.
  Its `Producer` cell becomes the collapsed-pair form both shipped segments already use, naming `Webster-Review` followed by the parenthesised `Webster-Bouncer` + `Webster-Burler` pair, exactly as rows 5 and 9 spell theirs.
  The row's other five cells are unchanged.

  (b) Lines 16 and 17, in the "What it is" section: "the recipe's sixteen rows" and "The recipe carries sixteen rows against the table below's fifteen entries" both move to seventeen.

  (c) The divergence note beneath the table (the paragraph beginning "**The table and the shipped recipe diverge deliberately.**").
  Its arithmetic changes twice: "sixteen rows against the table's fifteen entries" becomes seventeen, and the enumeration of collapsed segment pairs goes from two to three — extend the list so row 13's `Webster-Review` is named alongside row 5's and row 9's, keeping the sentence's existing shape and its closing pointer to the gate section.

  (d) The per-producer task list (the sentence beginning "The concrete breakdown of `loom`'s own rows").
  Add the `(shipped)` marker to `loom: Webster-Review producer`, matching the convention already applied to `loom: Plan-Review producer` in the same list.

  (e) The "## Webster-Review rubric" section.
  It currently opens "This is the text the future `Bouncer` rubric for `Webster-Review` must **point at**", which is false once the stencil ships.
  Replace that opening with a framing paragraph matching the shape both sibling sections carry — the "### Discussion-Review rubric — what to also flag (relocation and exclusion)" and "### Plan-Review rubric" sections are the models: name `contracts/stencils/loom/loom-rubric-webster-review.md` as the shipped rubric read by both rows of the perch, state that this section is a doc *about* that stencil per the Producer Pointer-Rule Invariant rather than a second copy it must point at, and that it is the durable human-readable record the stencil was transcribed from.
  Keep the section's two existing "also flag" bullets unchanged in substance.
  Then record the two things the stencil adds beyond them, briefly, so the durable record is not thinner than the shipped file: that the rubric derives its own review range from `product.parent` in loom's status file and blocks rather than guessing when it cannot, and that it carries a do-not-flag list keeping the three upstream gates' subjects — the plan's format, the plan itself, and the overlay artifacts — out of this gate's findings.

  Do not change line 155's "sixteen check IDs" — that is `internal/planparser`'s check count, not a producer-row count, per the overview's row-count criterion.
- **Commit:** `docs(loom): record the shipped Webster-Review perch in the design doc`

### Card 13: Move the recipe design doc's row count

- **Context:** none
- **Edits:**
  - `manifest/designs/shed-recipe.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** two occurrences, both producer-row counts, both moving from sixteen to seventeen: the "## The idea" section's "builds loom's sixteen-row `[]shedengine.ProducerDef`", and the test-ownership bullet's "the sequencing/cancellation/resume tests that build the real sixteen-row list".
  Nothing else in this file changes.
- **Commit:** `docs(shed-recipe): move loom's row count to seventeen`

### Card 14: Retarget the status spec's mid-run example at Webster

- **Context:** none
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** the mid-run example names the retired `Webster-Review` row in five places — the prose sentence introducing it, `current_producer`, `activity.now`, `activity.last`, and the third `history` entry's `producer`.
  Retarget every one at `Webster`, not at `Webster-Bouncer`.
  The example's whole point is a row that escalates because it has no `on_stuck` target, which its `error` and `activity.wait` both state as "stuck with no OnStuck target";
  `Webster-Bouncer` has an `on_stuck` target, so pointing the example there would make the example's own error string false, while `Webster` genuinely carries no `on_stuck` both before and after this change.
  Leave the example's `error`, `state`, `pause_requested`, `product` block, timestamps, and the two earlier history entries exactly as they are.
- **Commit:** `docs(loom-status-spec): retarget the mid-run example at the Webster row`

### Card 15: Move the roadmap item to Done

- **Context:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** remove the `**loom: Webster-Review producer**` item, and its `See [designs/loom.md](designs/loom.md#webster-review-rubric).` continuation line, from the "### loom: real LLM producers" subsection under Planned.
  Add it to the "## Done" section as a one-line entry in that section's existing shape, matching the `**loom: Discussion-Review producer**` entry already there: a past-tense sentence saying the `Webster-Review` stub row was replaced by a `Webster-Bouncer`/`Webster-Burler` segment gating the committed diff, followed by an indented `See [designs/loom.md](designs/loom.md#webster-review-rubric).` line.
  This is the move the file's own Maintenance section prescribes ("Move an item from Planned or Someday to Done ... when it ships"), and the `#webster-review-rubric` anchor still resolves because card 12 keeps that heading.
  Write the item literally as `1.` per the same Maintenance section — numbering renders automatically and no other item's number changes.
  Also update the "### loom: real LLM producers" preamble, which says "all three items below are unblocked" and now stands over two items.
- **Commit:** `docs(roadmap): move the Webster-Review producer item to Done`

### Card 16: Correct the smoke suite's driver-liveness note

- **Context:** none
- **Edits:**
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** the file carries no row-count assertion at all;
  its only affected content is the package-doc paragraph beginning "A note on driver-liveness timing", which says loom's producer table "backs one of its sixteen rows -- Webster-Review -- with a stub producer that reports Done unconditionally" and then reasons about driver-liveness timing from that claim.
  Both halves are now false: the count moved to seventeen and no row is stubbed.
  Rewrite the paragraph so the timing rationale rests on what is actually true of a seventeen-row list where every row is real — a freshly bootstrapped driver against a pair with no discussion or plan artifacts still bounces through `Discussion-Write`/`Discussion-Validate` a bounded number of times and then blocks, well before reaching any later row, so the lifecycle it describes can still complete in well under a second.
  Keep the paragraph's conclusion unchanged: tests here treat "a driver process exists" as a best-effort observation and lean on the status file's own history for the assertions that must hold unconditionally.
  Change no assertion, no test function, and no build tag in this file.
- **Commit:** `docs(loomcli): correct the smoke suite's driver-liveness note for the real Webster-Review row`

## Batch Tests

`verify: go build ./... && go vet -tags smoke ./internal/loomcli/... && go test ./internal/lyxcwd/...`

This batch has no runnable behaviour of its own, so the command is three compile/enforcement gates rather than a feature test.
`go build ./...` guards the untagged tree.
`go vet -tags smoke ./internal/loomcli/...` is what compiles `smoke_test.go` at all: the file carries `//go:build smoke`, so an untagged build never type-checks it, and card 16's edit is inside its package doc comment where a stray unterminated comment would otherwise go unnoticed until a tagged run.
It is `go vet` rather than `go test -tags smoke` deliberately — the smoke suite needs a real tmux server and a real built `cmd/lyx` binary, which is the repo's tagged tier and not something a per-round batch gate should spawn;
the tagged suite runs in `pipeline.done_gate`'s own `go test -tags integration` companion pass and in CI.
`go test ./internal/lyxcwd/...` runs the two enforcement walks this batch's `.md` edits fall inside: `TestEnforcement_MarkdownLinks`, which resolves every link in `manifest/` and `docs/` including the roadmap's `#webster-review-rubric` anchor card 15 depends on, and `TestEnforcement_FabricVocabulary`, which walks `internal/**/*.md` and the production Go under `internal/` and `cmd/` that card 16 edits.
