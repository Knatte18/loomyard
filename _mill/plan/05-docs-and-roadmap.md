# Batch: docs and roadmap

```yaml
task: 'loom: self-checkable mechanical gates'
batch: 'docs and roadmap'
number: 5
cards: 2
verify: go test ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [4]
```

## Batch Scope

This batch lands the three remaining documentation obligations `CLAUDE.md`'s task-completion rule creates: the loom module doc's account of the two gates and their standalone verbs, `docs/overview.md`'s module table and loom verb list, and the roadmap item's move to Done.
The two `CONSTRAINTS.md` invariants are deliberately **not** here — they landed with the code that made each true, in batches 1 and 4, which is the stricter reading of "same commit" and leaves this batch to the three genuinely descriptive documents.

It is one batch because all three files are prose describing the same shipped surface, and it comes last because every claim it makes must already be true on disk.
It depends on batch 4 alone, which transitively depends on everything else.

Batch-local decision: `manifest/roadmap.md` gets its own card rather than being folded into the module docs, because moving an item from Planned to Done is a different kind of edit — it rewrites a numbered list in two sections at once — and keeping it separate makes the reviewer's job on each half clearer.

## Cards

### Card 9: loom design doc and docs/overview.md

- **Context:**
  - `internal/loomcli/validate.go`
  - `internal/loomcli/cli.go`
  - `internal/discussionparser/doc.go`
  - `internal/loomshed/discussionvalidate.go`
  - `CONSTRAINTS.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/loom.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/designs/loom.md`:

  - In the producer table, extend row 4 (`Discussion-Validate`) and row 8 (`Plan-Validate`) so each names its standalone verb — `lyx loom validate-discussion` and `lyx loom validate-plan` respectively — alongside the existing input and pass/fail output columns.
    Keep each table row on one physical line, per `CLAUDE.md`'s table-cell exception to semantic line breaks.
  - In the `### Validation checks (spec for `Discussion-Validate`)` section, add a paragraph stating that the same two checks are callable standalone as `lyx loom validate-discussion`, that the verb and the row call the identical package function (`discussionparser.Validate`) so they can never disagree, and that the verb reports *which* file or heading failed while the row's `Stuck` deliberately carries an empty pointer.
    Point at the Gate Self-Check Parity Invariant in `CONSTRAINTS.md` for the rule itself rather than restating it, per the Producer Pointer-Rule Invariant's spirit.
  - Add the equivalent one-paragraph note for `Plan-Validate`, stating that `lyx loom validate-plan` makes the same three `planparser` calls the row makes, in the same order.
    Place it wherever the doc's existing structure makes it read naturally near row 8's account;
    if the doc has no `Plan-Validate` detail section today, a short subsection beside the Discussion validation-checks section is acceptable.
  - Leave the `Plan-Sweep` section's claim that it reuses the same section parsing standing, and note in passing that the parsing it will reuse now lives in `internal/discussionparser`.

  In `docs/overview.md`:

  - Add no entry to the module-tree listing for the new package.
    `internal/planparser` — the closest existing analog, a told-path sole-format-reader — has no tree entry either, so adding one only for the new package would make the tree inconsistent with the very package it mirrors.
    The module list below is where the discussion's "module table gains `internal/discussionparser`" obligation is discharged.
  - Add a `discussionparser` bullet to the module list, immediately after the existing `planparser` bullet and in the shape that bullet uses: the sole reader of `_lyx/discussion/`'s on-disk format, taking told absolute paths and declaring no location of its own (deliberately unlike `planparser`, because `loomengine`'s accessors take a `*lyxcwd.Location` a stdlib-only leaf may not import), consumed by `loomshed.discussionValidate` and by the `lyx loom validate-discussion` verb, with the ✅ Implemented marker the neighbouring entries carry.
  - In the `loom` module bullet, extend the parenthetical verb list from `lyx loom run|drive|status|pause` to include `validate-discussion` and `validate-plan`, and add two sentences describing them in the same one-sentence-per-verb style the existing `run` / `drive` / `status` / `pause` sentences use: each runs its phase's mechanical gate standalone, exits 0 on a clean gate and 1 otherwise, and emits the findings in the failure envelope so a writer agent can self-check before handing off.

  Every edited paragraph and list item uses semantic line breaks, per the `markdown-semantic-line-breaks` Shared Decision, and any markdown link added must resolve — both its file part and, for a `.md` target, its `#anchor`.
- **Commit:** `docs(loom): document the two standalone gate verbs and the discussionparser module`

### Card 10: move the roadmap item to Done

- **Context:**
  - `manifest/designs/loom.md`
  - `docs/overview.md`
  - `CONSTRAINTS.md`
  - `internal/loomcli/validate.go`
  - `internal/discussionparser/doc.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Remove the `### loom: self-checkable mechanical gates` heading, its two framing paragraphs, and its single numbered item from the `## Planned` section, and add a corresponding entry to the `## Done` section, written in the retrospective, past-tense style every existing Done entry uses — what shipped, the decisions worth preserving, and what it deliberately did not build.

  The Done entry must record: that `Discussion-Validate`'s two checks moved into `internal/discussionparser`, a stdlib-only leaf returning `[]Finding` rather than a bare bool, mirroring `internal/planparser`'s existing split from `loomshed.planValidate`;
  that `loomshed.discussionValidate.Call` became a thin wrap with its outward `Done`/`Stuck`/returned-error contract deliberately unchanged, which the short-circuit order pinned in the new package is what makes checkable;
  that `lyx loom validate-discussion` and `lyx loom validate-plan` shipped as zero-argument verbs on the existing subtree, each calling the exact function its `ShedProducer` row calls, exiting 0 on a clean gate and 1 otherwise, with findings in the failure envelope under a `findings` key;
  that a three-way parity test per gate now asserts the verb and the row agree across `Done`, `Stuck`, and returned-error, and that two invariants were recorded in `CONSTRAINTS.md` (Discussionparser Sole-Parser, Gate Self-Check Parity);
  and that no stencil under `contracts/stencils/loom/` was edited — instructing the writer agent to call these verbs belongs to `loom: Discussion-Write producer` and `loom: Plan-Write producer`, which this item was sequenced ahead of precisely so the verbs would exist first.

  Update the two forward references that name this item by title: the `loom: Discussion-Write producer` and `loom: Plan-Write producer` entries each say "the prompt must instruct the agent to call the `loom: self-checkable mechanical gates` CLI verb" — replace the item reference with the concrete verb name (`lyx loom validate-discussion` and `lyx loom validate-plan` respectively), since the item is no longer in Planned for a reader to look up.

  Follow the file's own Maintenance section for how the numbering works, and renumber the surrounding lists as that section requires.
  Use semantic line breaks throughout.
  Do not add, remove, or reword any other roadmap item.
- **Commit:** `docs(roadmap): move loom: self-checkable mechanical gates to Done`

## Batch Tests

`verify: go test ./internal/lyxcwd/... ./cmd/lyx/...` is the right scope for a documentation-only batch because the repo's markdown guarantees are machine-checked from Go tests, not from a markdown linter.
`internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` is the Markdown Link Integrity check over `manifest/` and `docs/` — both of this batch's edited roots — resolving every inline link's file part and, for a `.md` target, its `#anchor`, including the two new `CONSTRAINTS.md` anchors batches 1 and 4 added that card 9 links to.
`internal/lyxcwd/enforcement_test.go` covers the Fabric Vocabulary walk over the repo's `.md` files, which the new prose must not trip.
`./cmd/lyx/...` is included as a cheap guard that the verb names written into the docs are the verb names actually registered — `helptree_test.go` exercises the real tree — so a typo in a doc's verb name is caught beside the doc that introduced it.
No card in this batch changes Go source, so there is no new behaviour to test beyond these existing enforcement suites.
