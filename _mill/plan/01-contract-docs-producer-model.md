# Batch: contract-docs-producer-model

```yaml
task: 'format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate'
batch: 'contract-docs-producer-model'
number: 1
cards: 7
verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: []
```

## Batch Scope

This batch rewrites both pinned contract files — `docs/reference/discussion-format.md` and `docs/reference/plan-format.md` — in producer-model terms.
That means: a rewritten status blockquote, a new short `## Producer and contract` section naming producer / consumers / Input / Output, a sweep of every generic phase-word to the named producer, the re-grounding of the two-file-split rule, the rewrite of the validation checklist into the mechanical producer's check list, and both halves of the symmetric "what NOT to look for" rule.
It is one batch because all seven cards operate inside the same two files with the same producer-model vocabulary and the same three pinned decisions;
splitting them would force two implementers to independently re-derive the same vocabulary.

The external interface batch 2 consumes: after this batch, `discussion-format.md` carries a `## Validation checks (spec for the future validator)` section holding checks 1–2, which is what batch 2's new `Discussion-Validate` producer-table row points into, and a `## Producer and contract` section that batch 2's `loom.md` cells must stay consistent with.

Batch-local decision beyond the overview's shared set: **`decision-record.md` is not renamed**, and the two-file directory keeps its current filenames.
Pre-decided at `shed-followups.md:314–320` and recorded so no card reopens it.

## Cards

### Card 1: `discussion-format.md` title, status blockquote, and `## Producer and contract`

- **Context:**
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/shed-followups.md`
  - `docs/overview.md`
  - `docs/reference/plan-format.md`
  - `CLAUDE.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/reference/discussion-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three edits at the top of the file.
  (a) Rewrite the H1 at `:1`, which currently reads `# Discussion format — the `discussion.md` ↔ Plan contract` and names the nonexistent artifact `discussion.md`.
  The new title names `_lyx/discussion/` and `Plan-Write` instead — e.g. `# Discussion format — the `_lyx/discussion/` ↔ `Plan-Write` contract`.
  (b) Rewrite the status blockquote at `:3`, keeping its existing **form** (a single-line `> **Status: Contract — pinned.**` blockquote, per `CONSTRAINTS.md`'s Documentation Lifecycle) and its existing "Durable reference doc — kept, not deleted on landing — the loom analogue of [plan-format.md](plan-format.md)" clause with its working relative link.
  Replace its middle clause "the artifact the Discussion phase produces and the Plan producer consumes" so it names `Discussion-Write` as the producer and `Plan-Write` as the consumer.
  This subsumes the `:3` site of the generic-phrase sweep card 2 performs — card 2 must find zero remaining generic phrases on this line.
  (c) Insert a new `## Producer and contract` section as the **first** `##` section of the body, immediately after the blockquote and immediately before the existing `## What it is, and who consumes it` heading.
  Write it in the vocabulary of `shed.md:22–29`: a producer's contract is exactly two parts, Input and Output, each a *pointer* to the format-contract file defining the consumed artifact's shape, never a restated copy.
  The section's pinned content is: this directory is produced by `Discussion-Write`;
  validated by `Discussion-Validate` (pointing at this file's own validation-checks section);
  reviewed by `Discussion-Review`;
  `decision-record.md` is consumed by `Plan-Sweep` and `Plan-Write`;
  `support-log.md` is consumed by `Discussion-Review` only.
  Keep the section short — it names producers and points, it does not restate the schema below it.
  Any intra-repo link the section adds must resolve, since `docs/` is inside the Markdown Link Integrity scan scope.
- **Commit:** `docs(discussion-format): name Discussion-Write and Plan-Write in the title, blockquote, and a new Producer and contract section`

### Card 2: `discussion-format.md` generic-phrase sweep

- **Context:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed.md`
  - `CLAUDE.md`
- **Edits:**
  - `docs/reference/discussion-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace every generic phase/producer phrase in the body with the named producer.
  This is substitution, not deletion and not a rewrite — each site keeps its sentence, and every sentence must still read correctly after the substitution.
  The enumerated sites, by their pre-edit line numbers and quoted text: "the Plan producer" at `:7`, `:10`, `:14`, `:15`, `:31`, `:54`, `:83` → **`Plan-Write`**;
  "the Discussion producer" at `:47` and `:71` → **`Discussion-Write`**;
  "the perch discussion-review gate" at `:72` → **`Discussion-Review`**.
  `:3`'s "the Discussion phase" and "the Plan producer" are already handled by card 1 and must not be re-edited here.
  `:12`'s "the **Discussion-review gate**" is card 3's, not this card's.
  `:14`'s sentence is re-grounded by card 4;
  this card performs only its `Plan-Write` substitution, leaving the rest of the sentence for card 4.
  Two sites are load-bearing and must survive the substitution with their force intact: `:10`'s "The Plan producer's **sole** input" becomes a statement about `Plan-Write`'s sole input, and `:83`'s "the Plan producer's declared input set" becomes a statement about `Plan-Write`'s declared input set.
  Naming them is the point — it turns each into a statement about a *named producer's* declared Input.
  The acceptance criterion for this card is that `grep -n "the Plan producer\|the Discussion producer\|the Discussion phase\|discussion-review gate\|the planner" docs/reference/discussion-format.md` returns nothing.
- **Commit:** `docs(discussion-format): sweep generic phase words to Plan-Write, Discussion-Write, and Discussion-Review`

### Card 3: `discussion-format.md:12` names `Discussion-Review` as `support-log.md`'s reader

- **Context:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `docs/reference/discussion-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The `support-log.md` bullet at `:11–12` currently reads, in full:

  ```markdown
- **`support-log.md`** — the raw support log.
  Read by the **Discussion-review gate**, **never** by the Plan producer.
  ```

  Rewrite the second line so that **`Discussion-Review`** — the LLM/`perch` producer — is named as the reader, and add that `Discussion-Validate` only existence-checks the file rather than reading its contents.
  The load-bearing half of the boundary must be preserved verbatim in force: `Plan-Write` **never** reads it.
  Do not name both producers jointly as "the reader" — `Discussion-Review` opens and reasons over the file for the anti-circling Review-rounds ledger at `:67–69`, whereas `Discussion-Validate` only stats the path;
  blurring the two would misattribute the anti-circling mechanism to something with no judgment.
- **Commit:** `docs(discussion-format): name Discussion-Review as support-log's reader and Discussion-Validate as its existence check`

### Card 4: `discussion-format.md:14` re-grounding of the distilled-digest rule

- **Context:**
  - `internal/websterengine/doc.go`
  - `docs/overview.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `docs/reference/discussion-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `:14` today reads, in full, with no citation to anything:

  ```markdown
Two files, not two sections of one file, on purpose: a distilled digest, never raw prose, is what the Plan producer should ever see.
  ```

  (Card 2 has already substituted `Plan-Write` for "the Plan producer" by the time this card runs.)
  This is an **addition, not a repair** — `shed-followups.md:286` describes the line as citing the deleted `builder-contract.md`, but task A's commit `0149776a` already stripped that citation outright.
  There is no dangling pointer to fix.
  Keep the "a distilled digest, never raw prose" rule exactly as it stands and **add** the attribution it currently lacks, restating the sentence in producer-model terms while doing so.
  Cite **both** live sources: `internal/websterengine`'s package documentation (`doc.go`, which states the distilled-`Digest`-persisted-at-terminal contract directly — see also `recordbatch.go`'s `RecordResult.Digest` handling) **and** `docs/overview.md`'s architecture-level "Go-distilled digests, never raw prose" principle.
  Do not cite only one: `docs/overview.md` is the architectural statement and `websterengine`'s `doc.go` is the implementing contract.
  If the citation is written as a relative markdown link rather than a bare path, it must resolve — `docs/reference/` is inside the Markdown Link Integrity scan scope.
- **Commit:** `docs(discussion-format): ground the distilled-digest rule in websterengine's doc.go and overview.md`

### Card 5: `discussion-format.md` writer-side "what NOT to look for"

- **Context:**
  - `manifest/designs/review-finding-classification.md`
  - `manifest/designs/loom.md`
- **Edits:**
  - `docs/reference/discussion-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the **writer side** of three "what NOT to look for" instructions into the body, from the writer's perspective — the "do not enumerate X here, it belongs to <stage>" form `review-finding-classification.md:55` specifies.
  Two of the three already have a writer-side statement and need only be made explicit as a boundary rather than restated from scratch;
  the third is new.
  (1) **"Notes for the plan writer" absent is never a finding** — it is optional by contract;
  the existing statements at `:55–56` and `:82` already say so, and this card makes the boundary explicit at the `:55–56` compaction rule.
  (2) **Rejected alternatives absent from `decision-record.md` is never a finding** — they belong in `support-log.md`'s Rejected alternatives section;
  the existing `:50` compaction rule and `:63` support-log section already place them, and this card makes the boundary explicit at `:50`.
  (3) **Incomplete call-site / cross-reference enumeration is never a finding at this stage** — new;
  that enumeration belongs to the compiler and to `Plan-Sweep`'s mechanical inventory, not to discussion writing.
  Add it as a new bullet in the same compaction-rules list at `:47–56`.
  Do **not** add a fourth item about section-ordering deviations: `:33` pins the seven sections in order on the writer's side, so a matching reviewer-side "never flag ordering" rule would contradict rather than mirror it.
  Card 6 writes the reviewer-side half of these same three items;
  each of the three must end up with both halves present in this file.
- **Commit:** `docs(discussion-format): state the writer-side half of the three what-not-to-look-for boundaries`

### Card 6: `discussion-format.md` validation-checks rewrite and the `Discussion-Review` rubric section

- **Context:**
  - `docs/reference/plan-format.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/designs/shed-followups.md`
  - `manifest/designs/loom.md`
- **Edits:**
  - `docs/reference/discussion-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two sections.
  (a) Rewrite the existing `## Validation checklist` section (`:76–83`, heading plus its "Spec for a future validator:" line and three bullets) into `## Validation checks (spec for the future validator)` — the same **neutral** heading `plan-format.md:187` already uses.
  Do not name the producer in the heading;
  `Discussion-Validate` is pinned only in `loom.md`'s table row, which points into this section.
  The section holds exactly two per-run checks, carried over from the current bullets: both files exist under `_lyx/discussion/` (`decision-record.md` and `support-log.md`), and `decision-record.md` has all seven required sections present (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria), with "Notes for the plan writer" optional and its absence not a violation.
  The current third bullet — the Plan-never-reads-`support-log` boundary — is **removed from the per-run check list** and recorded instead as a short note inside the same section, stating in so many words (i) the boundary itself, (ii) that it is asserted once at build/test time over the `Plan-Write` producer *definition* rather than re-evaluated per run, because it is a property of the definition and there is nothing per-run for a mechanical producer to evaluate, and (iii) that it lands with `Shed`.
  Write it explicitly enough that nobody re-files it later as a missing check.
  Also state in this section that the mechanical producer is **exhaustively defined by the checks listed here** — it has no judgment, and nothing else is "its" to look for.
  (b) Add a new `## Discussion-Review rubric — what not to flag` section, explicitly marked as the text the future `perch` profile for `Discussion-Review` must **point at** rather than copy, per the pointer rule.
  It carries the reviewer-side half of card 5's three items, from the reviewer's perspective ("do not flag missing X here, it belongs to <stage>"): a missing "Notes for the plan writer" subsection, missing rejected alternatives in `decision-record.md`, and incomplete call-site or cross-reference enumeration.
  Aim this section at `Discussion-Review`, the LLM producer, not at the mechanical one — over-flagging is a judgment failure mode, and a mechanical producer that has only checks cannot over-flag.
  Place it after the validation-checks section and before `## Worked example`, so both halves of each symmetric rule stay one scroll apart.
- **Commit:** `docs(discussion-format): rewrite the validation checks and add the Discussion-Review what-not-to-flag rubric`

### Card 7: `plan-format.md` producer-model rewrite

- **Context:**
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `docs/overview.md`
  - `docs/reference/discussion-format.md`
  - `CLAUDE.md`
- **Edits:**
  - `docs/reference/plan-format.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three edits;
  the body **schema** — the field grammar, section lists, compaction rules, validation-check list at `:187`, and the worked example — is correct and is **not** rewritten.
  (a) Rewrite the status blockquote at `:3`, keeping its single-line blockquote form and its working `[documentation lifecycle](../overview.md#documentation-lifecycle)` link, so it names `Plan-Write` as the producer of the artifact this doc pins.
  Keep the existing statement that webster (`internal/websterengine`, via its sole parser `internal/planparser`) consumes it and that the doc is durable and was kept when webster shipped.
  (b) Insert a new `## Producer and contract` section as the first `##` section of the body, immediately after the blockquote and immediately before `## What a card is`, in the same shape and vocabulary card 1 used in `discussion-format.md`.
  Its pinned content: produced by `Plan-Write`;
  validated by `Plan-Validate`, pointing at this file's own `## Validation checks (spec for the future validator)` section at `:187`;
  reviewed by `Plan-Review`;
  consumed by `Batchifier` and `Webster` (via `internal/planparser`, webster's sole parser).
  Keep it short and pointer-shaped — it must not restate the card schema below it.
  (c) Sweep "the planner" at `:24` to **`Plan-Write`**.
  That is the only generic-phrase site in this file.
  The acceptance criterion is that `grep -n "the Plan producer\|the Discussion producer\|the Discussion phase\|discussion-review gate\|the planner" docs/reference/plan-format.md` returns nothing.
  `Plan-Review-Gate` does not occur anywhere in this file, so this card performs no rename.
- **Commit:** `docs(plan-format): name Plan-Write, Plan-Validate, and Plan-Review in the blockquote and a new Producer and contract section`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` runs `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks`, which enforces `CONSTRAINTS.md`'s Markdown Link Integrity invariant: every inline `[text](target)` link in a `.md` file under `manifest/` or `docs/` must resolve, both the file part and any `#anchor` on a `.md` target.
Both files this batch edits are inside that scan scope, and cards 1 and 7 each add a new `## Producer and contract` heading, so this is the machine check that catches a broken relative link or a stale anchor introduced by the rewrite.
Do not add an allowlist entry to work around a break this batch creates — fix the link.

The batch is otherwise docs-only with no runnable surface of its own (`shed-followups.md:340`).
Three additional criteria are verified by grep rather than by a test, and the batch is not done until all three pass:

1. `grep -n "the Plan producer\|the Discussion producer\|the Discussion phase\|discussion-review gate\|the planner" docs/reference/discussion-format.md docs/reference/plan-format.md` returns nothing — the generic-phrase sweep is complete.
   Each site must have been **replaced** by the named producer per cards 2, 3, and 7, not deleted.
2. Each of the three "what NOT to look for" items has **both** halves present in `discussion-format.md`: the writer-side statement in the body (card 5) and the reviewer-side statement in the rubric section (card 6).
   A missing half is the exact non-convergent-loop failure `review-finding-classification.md:53` describes.
3. `discussion-format.md`'s validation section has the neutral heading `## Validation checks (spec for the future validator)` and does **not** carry `Discussion-Validate` as a heading;
   check 3 appears as a build-time-assertion note rather than as a third per-run check.
