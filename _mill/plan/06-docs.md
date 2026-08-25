# Batch: docs

```yaml
task: 'loom: Discussion-Review producer'
batch: 'docs'
number: 6
cards: 3
verify: go vet -tags smoke ./internal/loomcli/...
depends-on: [5]
```

## Batch Scope

This batch closes the Documentation Lifecycle half of the task: loom's module doc records what the segment now is, the roadmap item moves to Done, and the one stale row-count claim left in Go prose is corrected.
It depends on batch 5 because every claim it writes is about rows that batch lands.
It ships no interface and changes no behaviour.

## Cards

### Card 22: update loom's module doc

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/stencils/loom/loom-rubric-discussion-review.md`
  - `internal/loomengine/config.go`
  - `internal/loomengine/template.yaml`
  - `CLAUDE.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Keep the producer table at one row for the segment, not two.
  Row 5's Producer cell becomes `Discussion-Review` followed by a parenthetical naming its two recipe rows, `Discussion-Bouncer` and `Discussion-Burler`;
  its `Kind` stays `bespoke`, its `Type` stays `LLM/review segment`, and its Input and Output cells are unchanged.
  The table's own entry count stays at fourteen.
  The reason the table does not split is the `Kind` column itself: it exists to mark a row as a bespoke black box, the doc already applies that framing to `Webster`, and it states outright that loom does not see the rounds, the handler/fixer, the cluster reviewers, or the progress-judge inside a review segment.
  Splitting the row would put the segment's internals into the very table whose black-box boundary is the design's stated point.
  Correct the prose that calls the recipe's list "thirteen rows" — the recipe now has fourteen rows and the table has fourteen entries, and the two sets are no longer the same set (the table carries `Plan-Sweep`, which the recipe does not, and the recipe carries two segment rows the table shows as one).
  Wherever the doc names a row count, say which of the two it means.
  Add a short note recording that the doc table and the recipe now diverge deliberately: the recipe is the shipped list and is authoritative for row names and routing;
  the table is the human-readable design record.
  Correct the sentence introducing the discussion-detail section, which says it carries the detail belonging to `Discussion-Validate` and `Discussion-Review` "instead — two producers not yet built".
  Neither is unbuilt any more: `Discussion-Validate` was already real before this task, and this task builds `Discussion-Review`.
  Drop or rewrite the clause;
  the sentence's actual point — that a mechanical validator's checklist and a review rubric are not part of what the *writing* agent needs to read — stands on its own and must survive.
  Rewrite the two `Discussion-Review rubric` subsections' opening line.
  Each currently says it is "the text the future `Bouncer` rubric ... must point at".
  The rubric is no longer future: name the shipped stencil as the rubric's home, and keep each subsection as the durable human-readable copy, stating that the stencil is the producer's instruction file and this doc is a doc about it, per the Producer Pointer-Rule Invariant.
  Do not restate the rubric's items here beyond what the subsections already say, and do not delete them — they are the transcription source the stencil was written from and the content test is pinned against.
  Record the review model's home while touching this doc: loom.yaml's `review:` and `review_timeout_min:` keys, threaded as run-wide `Env.Review*` values, rather than recipe-literal keys on the two rows.
  Follow CLAUDE.md's markdown rule throughout: semantic line breaks, one sentence per line, no fixed-column hard-wrap, table cells on one line.
- **Commit:** `docs(loom): record the shipped Discussion-Review segment and its rubric`

### Card 23: move the roadmap item to Done

- **Context:**
  - `manifest/designs/loom.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete the `loom: Discussion-Review producer` item from the Planned section, including its `See ...` continuation line, leaving the two remaining review-producer items and their surrounding framing paragraph intact.
  Correct that framing paragraph if it now counts wrong: it says "all four items below are unblocked", and one of the four is shipping here.
  Add a one-line Done entry naming the item and pointing at loom's module doc, following the Maintenance section's own rules — entries are a name plus one or two sentences, never a design writeup, and a Done entry points at the module's own documentation, which is where its durable detail lives from then on.
  Write the item literally as `1.` regardless of position: numbering is automatic and restarts at 1 in each section, so no other entry's digit needs touching.
  The Done section currently carries only its "Cleared 2026-08-25" note;
  keep that note and add the entry beneath it.
  Add nothing else to the roadmap — this task adds no new planned work.
- **Commit:** `docs(roadmap): move loom: Discussion-Review producer to Done`

### Card 24: correct the stale row-count prose in the smoke suite

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/stub.go`
  - `internal/loomrecipe/coverage_guard_test.go`
- **Edits:**
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The file's package-level comment claims loom's producer table "backs five of its thirteen rows with stub producers that report Done unconditionally".
  Correct both numbers: the recipe has fourteen rows, and exactly two of them (`Plan-Review` and `Webster-Review`) are still `Stub`.
  Note that only "thirteen" is a regression this task introduces — "five" was already wrong before it started, since only three rows (`Discussion-Review`, `Plan-Review`, `Webster-Review`) have ever used the `Stub` engine, as `internal/loomrecipe/coverage_guard_test.go`'s own row table shows.
  Fix both regardless;
  the corrected end-state text is the same either way.
  Check the sentence's surrounding claim about the driver's typical lifecycle still follows from the corrected numbers rather than from the old ones.
  Change no code and no assertion in this file — this is a comment-only edit to a `//go:build smoke` file, and the smoke suite's own behaviour is out of this task's scope.
- **Commit:** `docs(loomcli): correct the stale row-count prose in the smoke suite`

## Batch Tests

This batch changes two markdown files and one Go comment, so it has no behavioural surface to test.
`verify: go vet -tags smoke ./internal/loomcli/...` is chosen for exactly one reason: card 24 edits a `//go:build smoke` file, which no untagged build ever compiles, so a syntax error introduced there would otherwise reach `main` unnoticed.
`go vet` with the tag compiles the file without running the suite, which is what this batch needs — the real smoke suite requires a tmux server and a genuinely built `cmd/lyx` binary and is far too expensive to run once per fixer round.
The two markdown edits are covered by review, not by a runner;
the overview's module-wide `verify: go build ./...` still runs at this batch's boundary as a final whole-tree check.
