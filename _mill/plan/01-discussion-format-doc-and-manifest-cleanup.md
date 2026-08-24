# Batch: discussion-format doc and manifest cleanup

```yaml
task: 'loom: redesign the Discussion format'
batch: 'discussion-format doc and manifest cleanup'
number: 1
cards: 6
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: []
```

## Batch Scope

This batch delivers the whole task: one new design doc (`manifest/designs/loom-format-discussion.md`), Fix 2's durable copy as a new subsection in `manifest/designs/loom.md`, and four manifest edits that reconcile the manifest around them — two stale supersession sentences reworded, one roadmap item trimmed to a pointer, one roadmap item moved from `## Planned` to `## Done`, and one cross-reference sentence added to `manifest/designs/review-finding-classification.md`.
It is one batch because the only machine gate on this task, `TestEnforcement_MarkdownLinks`, resolves every inline link and `.md` `#anchor` under `manifest/` and `docs/` at once: card 4 and card 5 add links pointing at a heading card 2 creates, and cards 2 and 4 add links pointing at a file card 1 creates, so any verify boundary drawn between them would run against a knowingly link-incomplete tree.
Card order inside the batch is therefore load-bearing — run cards 1 through 5 in order, then card 6, which is a zero-diff verification gate.
There is no external interface for a next batch to consume;
this batch is the task.
Batch-local decisions beyond `## Shared Decisions`: none.

## Cards

### Card 1: write manifest/designs/loom-format-discussion.md

- **Context:**
  - `manifest/designs/plan-card-format.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/roadmap.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `docs/overview.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `manifest/designs/loom-format-discussion.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write a new standalone design doc modeled on `manifest/designs/plan-card-format.md`'s house style — terse, decision-oriented prose, no meta-commentary, an H1 title, a `> **Status: …**` blockquote line directly under it, and an `## Open, not decided here` closing section.
  Do not restate this task's own planning process, and do not describe the doc as "this task"; write it as a standing design reference.
  The doc has exactly these sections, in this order.
  (a) H1 title naming the Discussion format redesign.
  (b) The status blockquote, carrying a **scoped** supersession claim: it supersedes `contracts/stencils/loom/loom-template-discussion.md`'s Step 2 ("Explore before asking", as bounded below) and Step 3's `Architecture` interview category specifically — and states in the same breath that it does not supersede Step 3's other five categories (Scope, Constraints, Edge cases, Security, Testing), does not supersede Step 5's section shapes, and does not supersede the stencil as a whole.
  Mark it "designed, not implemented" in the same blockquote, matching `plan-card-format.md`'s own opening words.
  (c) A "what is not changing" section stating that loom's `_lyx/discussion/` two-file split is correct as-is: `decision-record.md`'s seven required H2 sections and `support-log.md`'s four H2 sections are unchanged, and `internal/discussionparser/validate.go`'s mechanical checks are unaffected because they check heading presence and file existence, never content quality.
  (d) A Fix 1 section, "exploration scope", stating the bounded positive/negative instruction pair as a standing rule for `Discussion-Write`: it MAY ask, at a coarse level, whether the design conflicts with an existing pattern and which module boundary the work falls under; it MUST NOT enumerate exact signatures, file:line citations, interface shapes, or dependency lists, and MUST NOT do exhaustive existing-pattern research — that class of fact is Quarry's (or manual grep's) job, computed fresh at Plan time.
  State the bound once, at the exploration-behavior level, and say explicitly that it is stated once rather than per-file because it applies regardless of which of the two files the content would land in.
  Give Step 2 its own explicit disposition in this same section so the later stencil rewriter has no unresolved conflict: Step 2's instruction to read the codebase before asking the operator anything stays, because it bounds redundant *questions* rather than interview *content*, but it takes the same bound — pre-interview exploration must not become exhaustive architecture/interface/dependency gathering either.
  Say that Step 2 and Step 3 state the identical bound from their own angles, what to explore and what to ask.
  Include the rationale for a *pair* rather than a bare prohibition: a purely negative instruction under-specifies how much is too much and risks either continued over-gathering at the margin or an overcorrection that refuses to note a genuinely decision-relevant fact.
  (e) A Fix 2 section, "review-loop principle", stating the three parts: "belongs in `support-log.md` instead" and "doesn't belong in Discussion at all" are legitimate `Discussion-Review` findings on equal footing with gap-filling findings; the completeness-before-leanness test — before relocating anything, check whether it carries a requirement or constraint Planner needs, extract that into `decision-record.md`'s own Decisions or Constraints first, and only then move the surrounding deliberation narrative, because `Plan-Write` never reads `support-log.md` at all, so moving something out is a genuine loss rather than a lower-visibility relocation; and the writer/reviewer symmetry note — whatever the writer's stencil says not to gather, the reviewer's rubric must say not to flag as missing, or the same purely-additive, non-convergent bias reappears with the writer-side fix in place.
  This section must state plainly that its **durable, authoritative copy lives in `manifest/designs/loom.md`**, under the `Discussion-Review rubric — what to also flag (relocation and exclusion)` subsection, and that the eventual `loom: Discussion-Review producer` task reads that subsection, not this doc — because this doc is deleted at Wave 2, before that task starts.
  Link to it as `[loom.md](loom.md#discussion-review-rubric--what-to-also-flag-relocation-and-exclusion)`.
  Cross-reference `manifest/designs/review-finding-classification.md`'s item 5 as documenting the same symmetry principle independently, without adopting its finding-class vocabulary;
  link it as `[review-finding-classification.md](review-finding-classification.md)`.
  (f) A lifecycle section naming this doc's own disposition per `docs/overview.md`'s documentation lifecycle: it is a module-design draft that stays a draft until the `loom: Discussion-Write producer` task (Wave 2) lands, at which point Fix 1's content folds into the stencil itself and this doc is deleted.
  It must state that every inbound markdown link under `manifest/` pointing at this file breaks identically at that deletion, and that retargeting or removing them is the deleting task's job, not this doc's.
  Name where they sit rather than asserting a count: `manifest/designs/loom.md` (its producer-table row and its relocation-rubric subsection), `manifest/designs/plan-card-format.md`'s status blockquote, and `manifest/roadmap.md` (the card-format group intro and this item's `## Done` entry).
  The natural retarget for each is the stencil, where Fix 1's content lives after the rewrite;
  `manifest/roadmap.md`'s `## Done` entry may instead keep a historical reference and drop out of the Markdown Link Integrity scope — that task's call.
  State the content-versus-link distinction explicitly: Fix 2's *content* survives the deletion because its durable copy is in `manifest/designs/loom.md`, but the *links* pointing at this file from Fix-2-related spots break exactly like the Fix-1 ones and need the same treatment.
  Link the lifecycle section as `[documentation lifecycle](../../docs/overview.md#documentation-lifecycle)`, the same target `manifest/designs/loom.md` already uses.
  (g) The `## Open, not decided here` section, naming three deferrals: the stencil rewrite itself (the `loom: Discussion-Write producer` roadmap item), the actual `Discussion-Review` rubric text (the `loom: Discussion-Review producer` roadmap item), and `review-finding-classification.md`'s finding-class vocabulary, which stays its own Someday proposal.
  Every inline markdown link in this file must resolve, including its `#anchor`, since `manifest/` is a scanned root for `TestEnforcement_MarkdownLinks`;
  keep references to `contracts/stencils/loom/loom-template-discussion.md` as backtick prose rather than links, matching how `manifest/designs/plan-card-format.md` already names it.
  Do not link `manifest/roadmap.md`'s individual items by anchor.
- **Commit:** `docs(designs): add discussion-format.md — bounded exploration scope and the relocation-finding principle`

### Card 2: add Fix 2's durable subsection and the producer-table pointer to loom.md

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two edits to `manifest/designs/loom.md`, both additive.
  First, in the producer table under `## The phase machine — a flat producer list, no predefined slots`, the `Discussion-Write` row (row 3) currently ends its Output cell with `shape pinned in the producer's own stencil (`contracts/stencils/loom/loom-template-discussion.md`)`.
  Append a link-only pointer to the new doc inside that same table cell, for Fix 1's exploration-scope content — a short trailing clause carrying `[loom-format-discussion.md](loom-format-discussion.md)`, no restated content.
  The cell stays a single line, per this repo's convention that table cells are exempt from semantic line breaks.
  Second, add a new subsection immediately after the existing `### Discussion-Review rubric — what not to flag` subsection and immediately before `### Plan-Validate detail`.
  Its heading is exactly `### Discussion-Review rubric — what to also flag (relocation and exclusion)` — em dash, lowercase `what to also flag`, parenthesized `(relocation and exclusion)`, reproduced character for character, because `docsLinkSlug` derives the anchor two inbound links resolve against from this text.
  The existing "what not to flag" subsection, including its three-item list, stays untouched and remains the sole authority for those three items.
  The new subsection carries Fix 2's actual content, written directly here rather than as a pointer elsewhere, in the same voice as the subsection above it: (i) `Discussion-Review` must accept "this belongs in `support-log.md`, not `decision-record.md`" and "this doesn't belong in Discussion at all" as legitimate findings, on equal footing with gap-filling findings, since a review loop that can only resolve a finding by adding content is the concrete mechanism behind discussion files that only grow across rounds; (ii) the completeness-before-leanness test — before any relocation finding, check whether the content carries a requirement or constraint Planner needs, extract that into `decision-record.md`'s own Decisions or Constraints first, and move only the surrounding deliberation narrative, because `Plan-Write` never reads `support-log.md`, making a careless move a silent loss rather than a relocation; (iii) the writer/reviewer symmetry note — whatever `Discussion-Write`'s stencil says not to gather, this rubric must say not to flag as missing, or the additive bias reappears even with the writer-side fix in place.
  Open the subsection with the same pointer-rule framing the subsection above it uses — that this is text the future `Bouncer` rubric for `Discussion-Review` must point at rather than copy or paraphrase, per the Producer Pointer-Rule Invariant.
  Link `[loom-format-discussion.md](loom-format-discussion.md)` once from this subsection as the companion design doc stating the writer-side half, and note that this subsection, not that doc, is the durable copy.
- **Commit:** `docs(designs): add loom.md's what-to-also-flag rubric subsection and the discussion-format pointer`

### Card 3: rescope plan-card-format.md's supersession claim

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `contracts/specs/loom-plan-spec.md`
  - `contracts/stencils/loom/loom-template-plan.md`
  - `manifest/designs/scout-plan-symbol-fields.md`
  - `manifest/designs/webster-parallel-execution.md`
- **Edits:**
  - `manifest/designs/plan-card-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The status blockquote is the file's third line and currently reads, in part, `Supersedes `contracts/specs/loom-plan-spec.md`'s Card fields, `contracts/stencils/loom/loom-template-discussion.md`, and `contracts/stencils/loom/loom-template-plan.md` — none of these are rewritten yet.`
  Rewrite that sentence so it names only the two survivors, `contracts/specs/loom-plan-spec.md`'s Card fields and `contracts/stencils/loom/loom-template-plan.md`, and reads grammatically as a two-item list — a two-item "Supersedes X and Y" sentence, with the trailing "none of these are rewritten yet" clause adjusted to agree.
  Do not delete the discussion-stencil clause in place and leave the surrounding list punctuation as it was.
  In the same blockquote, add one short sentence pointing at the new doc as the holder of the discussion stencil's own scoped supersession claim, linked as `[loom-format-discussion.md](loom-format-discussion.md)`.
  Leave the rest of the blockquote — the `manifest/designs/scout-plan-symbol-fields.md` and `manifest/designs/webster-parallel-execution.md` staleness note — exactly as it is;
  reconciling those two belongs to the Card-format work that named them.
  Change nothing else in this file.
- **Commit:** `docs(designs): rescope plan-card-format.md's supersession claim off the discussion stencil`

### Card 4: roadmap.md — rescope, trim, and move the item to Done

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `manifest/designs/loom.md`
  - `contracts/specs/loom-plan-spec.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/loom/loom-template-plan.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three edits to `manifest/roadmap.md`, all inside or under `## Planned`.
  First, the `### loom: rewrite for the new Plan Card format` group intro paragraph currently claims the group's design doc "Supersedes `contracts/specs/loom-plan-spec.md`'s Card fields and both `contracts/stencils/loom/loom-template-discussion.md` and `loom-template-plan.md` outright".
  Rewrite that sentence so it claims supersession of `contracts/specs/loom-plan-spec.md`'s Card fields and `loom-template-plan.md` only, reading grammatically without the "both … and … outright" construction, and add a short clause noting that the discussion stencil's own scoped supersession claim now lives in `[designs/loom-format-discussion.md](designs/loom-format-discussion.md)`.
  Leave the rest of that paragraph, including the "Two waves, in order — Wave 2 depends on Wave 1" sentence and the stale-docs note, unchanged.
  Second, in the `### loom: real LLM producers` group, the **loom: Discussion-Review producer** item's second paragraph currently restates Fix 2 in full: its second sentence begins "Per the group's item above, the rubric must accept …" and its third begins "Every such finding must also check completeness before relocation: …".
  Replace those two sentences with a single pointer sentence at the new subsection, linked as `[designs/loom.md](designs/loom.md#discussion-review-rubric--what-to-also-flag-relocation-and-exclusion)`, stating that the rubric must implement the relocation-and-exclusion principle recorded there.
  Keep that paragraph's first sentence — the rubric-scope description covering `decision-record.md` leanness and technical-claim exclusion — and keep the item's remaining paragraphs, its Bouncer/Burler wiring text, its dependency note, and its existing trailing `See [designs/loom.md](…)` line, all unchanged.
  Third, move the **loom: redesign the Discussion format** item out of `## Planned`'s Wave 1 and into `## Done`, as the section's first entry.
  Delete the whole Planned entry, both its long paragraph and its trailing `See …` line.
  Write the Done entry per `manifest/roadmap.md`'s own Maintenance rule — a bold item name plus one or two sentences of what and why, then a `See [designs/loom-format-discussion.md](designs/loom-format-discussion.md).` line.
  Do not carry over the Planned entry's Fix 1 and Fix 2 prose;
  the Done entry says what shipped (a companion design doc bounding `Discussion-Write`'s exploration scope, plus the relocation-and-exclusion rubric principle recorded in `designs/loom.md`) and why, and nothing more.
  Write the entry's list marker as the literal `1.`, like every other entry in the file, since numbering renders automatically.
  Leave the Wave 1 and Wave 2 headings, the group intro's wave sentence, and the remaining Wave 1 item (**loom: code-writing skills — comments, build, testing**) structurally untouched — Wave 1 simply has one item after the move.
  Do not renumber anything anywhere.
  Fourth, repair the two positional cross-references the move above invalidates.
  The Wave 2 **loom: Discussion-Write producer** item currently reads "a prompt rewritten for the new Discussion format (Wave 1's first item), instructing the agent to load the new code-writing skills (Wave 1's second item)".
  After the move, the Discussion-format item is not in Wave 1 at all and Wave 1 has no second item, so both parentheticals are false.
  Reword them to cross-reference by bold item name instead of by position — the Discussion-format item as the shipped Done entry it now is, and the code-writing-skills item by its own name — per this file's own Maintenance rule that numbers are not stable cross-reference IDs and references go by bold item name.
  Change nothing else in that item.
- **Commit:** `docs(roadmap): rescope the card-format supersession, trim the Fix 2 restatement, ship the discussion-format item`

### Card 5: point review-finding-classification.md's item 5 at the new subsection

- **Context:**
  - `manifest/designs/loom.md`
- **Edits:**
  - `manifest/designs/review-finding-classification.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Under `## Concrete proposal`, item 5 is the numbered entry opening "**The "what NOT to look for" instruction must be written symmetrically, into both sides, not just the reviewer.**" and closing with the sentence about call-site enumeration belonging to the compiler on both sides.
  Append exactly one sentence to the end of that item, naming `manifest/designs/loom.md`'s new subsection as a concrete instance of the principle item 5 states, linked as `[loom.md](loom.md#discussion-review-rubric--what-to-also-flag-relocation-and-exclusion)`.
  The added sentence goes on its own line, indented to match the item's existing continuation lines.
  Item 1's existing `loom.md#discussion-producer-detail--validation-checks-and-review-rubric` link is correct as it stands and must not change — the new subsection lives inside that same section.
  Change nothing else in this file, and do not change its `Status: Someday` header.
- **Commit:** `docs(designs): point review-finding-classification.md item 5 at loom.md's relocation rubric`

### Card 6: verify link integrity and the acceptance greps

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/plan-card-format.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/roadmap.md`
  - `CONSTRAINTS.md`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** A zero-diff verification gate over cards 1 through 5.
  Run `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` and confirm it passes;
  a failure here means a link or a `#anchor` added by an earlier card does not resolve, and the fix belongs in whichever card's file broke it, not in this card.
  Then run `grep -rn "loom-template-discussion" --include="*.md" manifest/ docs/ CONSTRAINTS.md` and confirm three things about its output: no line in `manifest/designs/plan-card-format.md` or `manifest/roadmap.md` still claims outright supersession of the discussion stencil, both reworded sentences read as grammatical prose rather than clause-dropped fragments, and `manifest/designs/loom-format-discussion.md`'s own scoped Step 2 / Step 3 supersession claim appears.
  Then run `grep -n "loom: redesign the Discussion format" manifest/roadmap.md` and confirm the item appears once, under `## Done`, and no longer under `## Planned`.
  Then confirm `grep -n "^### Discussion-Review rubric" manifest/designs/loom.md` reports both subsections, the existing "what not to flag" one and the new "what to also flag (relocation and exclusion)" one, in that order.
  This card changes no file and makes no commit.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` is the batch's only runnable gate, and it is the right one: this task is markdown-only, and `TestEnforcement_MarkdownLinks` (`internal/lyxcwd/docslink_test.go`, the Markdown Link Integrity Invariant's enforcement point) resolves every inline markdown link and every `.md` `#anchor` in every file under `manifest/` and `docs/`.
That covers all five links this batch introduces — the new doc's own outgoing links, the producer-table pointer at `manifest/designs/loom.md`, `manifest/roadmap.md`'s two new anchored pointers, and `manifest/designs/review-finding-classification.md` item 5's added sentence — including the derived `discussion-review-rubric--what-to-also-flag-relocation-and-exclusion` anchor that card 2's pinned heading produces.
No Go source changes, so no package-scoped test beyond this one applies;
`internal/discussionparser`'s validator is untouched by construction, since no card edits it or the section shapes it checks.
The repo-wide `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) runs the same enforcement test again as part of the full suite at task end, which is the backstop for anything a scoped run would miss.
Card 6's greps are the batch's non-runnable acceptance checks, run by the implementer rather than by `verify:`, since they assert prose properties (a reworded sentence reads grammatically) that no test expresses.
