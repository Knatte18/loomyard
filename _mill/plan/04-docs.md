# Batch: documentation lifecycle

```yaml
task: 'loom: Discussion-Write producer'
batch: 'documentation lifecycle'
number: 4
cards: 5
verify: go test ./internal/lyxcwd/...
depends-on: [3]
```

## Batch Scope

This batch closes the documentation lifecycle the earlier batches open: it deletes `manifest/designs/loom-format-discussion.md`, whose own Lifecycle section names this task as its deleter, retargets or removes all five inbound markdown links, corrects four design-doc claims that go stale the moment row 3 becomes real, moves this item from Planned to Done in `manifest/roadmap.md` while adding the interactive follow-up item, and moves `CONSTRAINTS.md`'s registry-count sentence from twelve to thirteen.

It depends on batch 3 because two of its edits name things batch 3 creates: `CONSTRAINTS.md`'s sentence names `TestRegistry_ShipsTwelveEntries`, which batch 3 renames, and the roadmap's Done entry describes a producer that is only real after batch 3 lands.

Batch-local decision: two of the five inbound links are not retargetable to the stencil.
`manifest/roadmap.md`'s card-format group intro and `manifest/designs/plan-card-format.md`'s status blockquote both read "the discussion stencil's own scoped supersession claim now lives in [loom-format-discussion.md]", and that sentence becomes false once Fix 1 folds into the stencil — there is no supersession left once the superseding content *is* the stencil.
Both clauses are deleted outright rather than repointed, because pointing them at the stencil would ship a wrong statement behind a working link.

## Cards

### Card 23: Retarget loom.md's three affected sites

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make three edits to `manifest/designs/loom.md`, and no others.
  (1) In the producer table's `Discussion-Write` row, delete the trailing `; exploration-scope bound in [loom-format-discussion.md](loom-format-discussion.md)` clause from the Output cell.
  The same cell already names `contracts/stencils/loom/loom-template-discussion.md`, which is where the bound now lives, so nothing is lost.
  Keep the row's other five cells exactly as they are, and keep the whole row on one physical line per this repo's table convention.
  (2) In the `Discussion-Review rubric — what to also flag (relocation and exclusion)` subsection, replace the sentence beginning "See [loom-format-discussion.md](loom-format-discussion.md) as the companion design doc" with a non-link statement that the writer-side half of this same principle now lives in `contracts/stencils/loom/loom-template-discussion.md`.
  Preserve the existing claim that this subsection, not that doc, is the durable copy — reword it so it no longer refers to a doc that is about to stop existing.
  Name the stencil in backticks, not as a markdown link: `contracts/` is outside the Markdown Link Integrity check's `manifest/` and `docs/` scope, so a link there would be unchecked, and a backtick path states the same fact without pretending otherwise.
  (3) In the module-decomposition table, correct the row claiming `DiscussionSpec` and `PlanSpec` are "both ✅ **built** but not yet wired into `Shed`".
  `DiscussionSpec` is wired now and `PlanSpec` is not, so the joint claim becomes a per-Spec one: `DiscussionSpec` built and wired as recipe row 3's `DiscussionWrite` engine, `PlanSpec` still built but unwired, still pointing at the `loom: Plan-Write producer` roadmap item.
  Apply semantic line breaks to any prose line this card rewrites.
- **Commit:** `docs(loom): retarget loom.md off the deleted discussion-format doc`

### Card 24: Delete the two false supersession clauses

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `internal/shedrecipe/entries_discussionwrite.go`
- **Edits:**
  - `manifest/designs/plan-card-format.md`
  - `manifest/designs/shed-recipe.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/plan-card-format.md`'s status blockquote, delete the sentence "`contracts/stencils/loom/loom-template-discussion.md`'s own scoped supersession claim now lives in [loom-format-discussion.md](loom-format-discussion.md)." outright.
  Do not repoint it at the stencil: the claim describes a supersession relationship that stops existing once the superseding content folds into the stencil itself.
  Leave the blockquote's other three sentences — the `loom-plan-spec.md` supersession, the `loom-template-plan.md` supersession, and the `scout-plan-symbol-fields.md`/`webster-parallel-execution.md` staleness note — unchanged, and keep the blockquote on one physical line per this repo's blockquote convention.
  In `manifest/designs/shed-recipe.md`'s motivation paragraph, correct the assertion that `SingleLLMProducer` differs across `Discussion-Write`/`Plan-Write` "only in which prompt stencil and interactivity setting it's given, exactly the shape a declarative recipe expresses cleanly".
  That premise turns out to be false: `Discussion-Write`'s Spec also carries per-run values a static recipe `Config` cannot hold — the task slug and the mode-rules block — plus a model and timeout resolved from the `discussion` role's own config rather than from recipe strings, which is why it ships as its own `DiscussionWrite` registry entry over an injected `SpecSource` closure rather than as a `SingleLLM` row.
  Correct the paragraph rather than annotating it as historical rationale: it is a live design doc for a shipped module, and its argument is the one a future recipe author reasons from.
  Leave the rest of that paragraph — the question about whether the loom-specific rows resist the shape, and its answer — intact, and leave the `Config` bullet further down the file untouched.
- **Commit:** `docs: correct the two false supersession and motivation claims`

### Card 25: Move the roadmap item to Done and add the interactive follow-up

- **Context:**
  - `manifest/designs/loom-format-discussion.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `plugins/scribe/skills/INDEX.md`
  - `internal/shedrecipe/registry_test.go`
  - `internal/loomengine/discussion.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make five edits to `manifest/roadmap.md`, and no others.
  (1) In the `loom: rewrite for the new Plan Card format` group intro, delete the clause "the discussion stencil's own scoped supersession claim now lives in [designs/loom-format-discussion.md](designs/loom-format-discussion.md)" outright, for the same false-sentence reason as card 24's `plan-card-format.md` edit.
  Keep the rest of that paragraph, including its `designs/plan-card-format.md` link, intact.
  (2) Move the Wave 2 `loom: Discussion-Write producer` item out of Planned and into `## Done`, rewriting its body to describe what actually shipped rather than what was proposed: the `Discussion-Write` stub replaced by a `SingleLLMProducer` behind a `loomshed` commit decorator, reached through a new `DiscussionWrite` registry entry over two injected `shedrecipe.Env` closures;
  the stencil rewritten with the folded-in exploration bound, a bounded coarse-level architecture interview category, a `scribe:prose`/`scribe:conversation` load step, and a closing `lyx loom validate-discussion` self-check;
  the producer autonomous-only;
  and `manifest/designs/loom-format-discussion.md` deleted per its own Lifecycle section.
  Record the two manual operator prerequisites explicitly in this entry: `/plugin install scribe@loomyard`, which nothing in the tree performs or verifies, so a missing plugin degrades prose quality rather than breaking a run;
  and `lyx stencil sync`, because `stencilstore`'s `ModeDev` reconcile warns instead of writing for an untouched stencil, so an already-seeded hub on a `-dev` binary keeps the old stencil text until the sync forces a refresh.
  Point the entry's `See …` link at `contracts/stencils/loom/loom-template-discussion.md`'s containing design doc rather than at the deleted file;
  where the deleted doc must be named at all, name it in prose as a bare historical mention outside link scope.
  After the move, Wave 2 holds only the `planparser: Card-format migration` item — reword the Wave 2/Wave 3 framing lines so they still read correctly with one item in Wave 2.
  (3) Add a new Planned item, `loom: interactive Discussion-Write`, to the `loom: real LLM producers` group.
  Its body states the work: flip the `autonomous` argument at `internal/loomcli`'s `wire()` from `true` to whatever a real mode selector resolves, and solve the resume defect that made autonomous-only the right call — `shuttleengine`'s `Wait` classifies a turn ending without all output files present as `OutcomeAsking`, `SingleLLMProducer` maps that to `Stuck`, and on resume it archives both freshly-written files and spawns an agent that knows nothing of the interview.
  State the trap a naive fix walks into: a resume-on-output-files pre-check reporting `Done` when both files exist cannot distinguish an interrupted interview from a `Discussion-Validate` bounce, which also re-enters the row with both files present, and would ping-pong until the bounce budget is exhausted.
  Note that `loomengine.DiscussionSpec` already keeps its `autonomous` parameter and `prompt.go`'s `modeRules` already keeps both branches with their tests, so the prose half is done.
  (4) In the `## Done` entry for `loom: redesign the Discussion format`, retarget its `See [designs/loom-format-discussion.md](designs/loom-format-discussion.md).` line at `contracts/stencils/loom/loom-template-discussion.md`, where Fix 1's content landed, and name the deleted doc in prose only if it is named at all.
  (5) In the `## Done` entry for `Shed recipe: engine registry`, correct the parenthetical describing `internal/shedrecipe/registry_test.go` as "the exact-twelve-names pin" — the test is now `TestRegistry_ShipsThirteenEntries`, so the phrase becomes an exact-names pin without a stale count.
  Leave that entry's own historical statement that it registered twelve engine names at the time it shipped intact — it is a record of what that task did, not a claim about today.
  Apply semantic line breaks to every prose line this card writes or rewrites.
- **Commit:** `docs(roadmap): ship loom: Discussion-Write producer, plan the interactive follow-up`

### Card 26: Move the registry-count sentence to thirteen

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/registry_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`'s Shed Recipe Registry Invariant, correct the one sentence naming `internal/shedrecipe/registry_test.go` and `TestRegistry_ShipsTwelveEntries`: the test is renamed to `TestRegistry_ShipsThirteenEntries` and pins the registry's exact thirteen names.
  Both the symbol name and the count must change, since the count is a machine-checked fact that must not go stale.
  Add no new invariant: nothing cross-cutting is introduced by this task — the existing Shed Recipe Registry, Told-Geometry, Cwd Resolution, Fabric Write-Side Containment, Fabric Git, Mutation Record, Stencil Ownership, Producer Pointer-Rule, Markdown Link Integrity, and Test Tier Purity invariants already cover the new entry, the commit decorator, and the stencil rewrite between them.
  Change no other line of `CONSTRAINTS.md`.
- **Commit:** `docs(constraints): move the registry-names pin from twelve to thirteen`

### Card 27: Delete the discussion-format design doc

- **Context:**
  - `manifest/designs/loom.md`
  - `manifest/designs/plan-card-format.md`
  - `manifest/roadmap.md`
  - `contracts/stencils/loom/loom-template-discussion.md`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `manifest/designs/loom-format-discussion.md`
- **Moves:** none
- **Requirements:** Delete `manifest/designs/loom-format-discussion.md` with `git rm`.
  This is the file's own Lifecycle section's instruction, which names the `loom: Discussion-Write producer` task as its deleter, and it is the last card of the batch precisely so all five inbound links are already retargeted or removed by cards 23, 24, and 25 before the target disappears.
  Fix 1's content survives inside `contracts/stencils/loom/loom-template-discussion.md`, and Fix 2's content survives in `manifest/designs/loom.md`'s own durable copy of the relocation rubric, so nothing is lost.
  Before committing, grep `manifest/` and `docs/` for any remaining `loom-format-discussion` occurrence and resolve it — the deletion is only correct if the repo-wide reference count reaches zero.
- **Commit:** `docs: delete loom-format-discussion.md per its own lifecycle`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` is the right scope for a batch whose only runnable surface is markdown-link integrity.
`internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` walks every inline link under `manifest/` and `docs/` and fails on any unresolved file part or `#anchor`, which is exactly what card 27's deletion risks and what cards 23, 24, and 25 exist to prevent.
The same run also executes that package's anchor-extraction and heading-anchor tests, so a retargeted link whose `#anchor` fragment no longer resolves is caught too, not only a missing file.

No other test in the tree reads any of these documents, so a wider scope would buy nothing here;
`pipeline.done_gate`'s repo-wide sweep at the end of the run is what confirms the earlier batches' code is still green after this docs-only batch.
Card 27's own grep-to-zero step is a manual completeness check the link test cannot make on its own: the link test only sees markdown links, while a bare backtick or prose mention of the deleted filename would slip past it.
