# Batch: wiring, render, and master template

```yaml
task: 'webster: DAG-derived card sequencing'
batch: 'wiring, render, and master template'
number: 2
cards: 10
verify: go test ./internal/websterengine/... ./internal/webstercli/... ./internal/batcher/... ./contracts/stencils/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
depends-on: [1]
```

## Batch Scope

This batch makes batch 1's `SequenceBatches` load-bearing everywhere: it is called at all five sites that compute execution batches, the three renderers switch from walking `plan.Cards` to walking the sequenced `[]batcher.Batch`, the master template is reworded so "in order" unambiguously means "the order listed", the two previous-digest lookups are corrected from `batchNumber-1` arithmetic to a true execution-predecessor lookup, and every affected test is updated.

It is one batch because the pieces cannot land separately without leaving the tree uncompilable at the boundary: `RenderMasterPrompt`'s signature change breaks `runlevel.go`'s call site in the same edit, and the predecessor-digest correction is only correct once every call site hands `BeginDeps.Batches`/`RecoverDeps.Batches` a sequenced slice.
The external interface batch 3 consumes is purely documentary — the behavior batch 3's doc edits describe.

Batch-local decision beyond `## Shared Decisions`: `RenderMasterPrompt` takes the sequenced `[]batcher.Batch` **instead of** the `*planparser.Plan`, not in addition to it.
The plan pointer was used for nothing else inside that function, so keeping it would leave two orderings reachable from one call — exactly the divergence this task removes.

## Cards

### Card 3: render the sequenced batch list, not the declared card list

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/integration.go`
- **Edits:**
  - `internal/websterengine/render.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `RenderBatchIndex` in `internal/websterengine/render.go` from `RenderBatchIndex(plan *planparser.Plan) string` to `RenderBatchIndex(batches []batcher.Batch) string`.
  It emits one line per batch, in the slice's own order — the caller is responsible for handing it a sequenced slice — using the existing `"%02d — %s — %s"` shape, where the number and slug come from `batchIdentity(b)` and the third field is every card's `Summary` in that batch joined with `"; "`.
  Under the identity batchifier a batch holds exactly one card, so the joined form renders byte-identically to today's output.
  A batch with no cards is skipped rather than emitting a `00 —` line.
  Update the doc comment to say the rendered order is the execution order and that the number on each line is the batch's identity, which the verbs key on, not its position in the list.

  Change `RenderProgress` from `RenderProgress(plan *planparser.Plan, st *State) string` to `RenderProgress(batches []batcher.Batch, st *State) string`.
  It walks `batches` in slice order, takes `number, slug := batchIdentity(b)`, and keeps today's `st.Batches[number]` lookup, terminal-only filter, `"%02d-%s: %s"` line shape, and `"none"` empty/nil-state returns exactly as they are.
  Walking batches rather than cards preserves the property `doc.go` already relies on — every batch number is positive, so `State.Batches`' reserved `-1` integration key can never surface here.

  Change `RenderMasterPrompt`'s first parameter from `plan *planparser.Plan` to `batches []batcher.Batch`, keeping every other parameter and its order unchanged, and update its two internal calls to `RenderBatchIndex` and `RenderProgress` accordingly.
  The `plan` parameter is removed entirely, not retained alongside `batches`: nothing else in the function body reads it.
  `RenderIntegrationPrompt` still takes `*planparser.Plan` and is untouched, so `render.go` keeps its `planparser` import.

  Update `render.go`'s file-level banner comment where it describes `RenderBatchIndex`/`RenderProgress` as batch-list/progress renderers, so it states they render the sequenced execution order.
- **Commit:** `refactor(websterengine): render the sequenced batch list in Master's prompt`

### Card 4: sequence in Run, and surface detected cycles on the run result

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/integration.go`
  - `internal/websterengine/state.go`
- **Edits:**
  - `internal/websterengine/runlevel.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `Run` in `internal/websterengine/runlevel.go`, immediately after the existing zero-batch refusal that follows `batches := deps.Batcher.Batch(plan.Cards)`, re-bind the slice through batch 1's sequencer: `batches, cycles := SequenceBatches(batches)`.
  Ordering matters — the zero-batch refusal keeps running against the batchifier's own output, and its message still names the batchifier, because `SequenceBatches` is length-preserving and can neither create nor remove that condition.
  Every later use of `batches` in `Run` (the `RenderMasterPrompt` call, `mapMasterDone`, `runIntegrationStage`) then sees the sequenced slice.

  Update the `RenderMasterPrompt` call to pass `batches` in place of `plan`, matching card 3's new signature.
  `plan` stays in scope and is still used by the surrounding code (`planparser.Validate`, `ShouldRunIntegration`, `RenderIntegrationPrompt`).

  Add a new exported field to `RunResult`: `Cycles []Cycle`, documented as every non-trivial strongly-connected component `SequenceBatches` condensed for this run — always informational, never a failure, and empty for the overwhelmingly common acyclic plan.

  In the `shuttleengine.OutcomeDone` branch, after `mapMasterDone` returns `runResult`, set `runResult.Cycles = cycles` and prepend one warning per cycle to `runResult.Warnings` by calling each cycle's `Warning()` method — do not format the sentence inline here.
  Prepend rather than append so the sequencing observations, which describe the whole run, read ahead of the integration stage's own per-failure warnings.
  Document at the assignment, in a short comment, that the non-done outcomes return an error rather than a `RunResult`, so a cycle observed on a run that ends stuck/paused/died reaches the operator through that error path's own message rather than through `Cycles` — an accepted, stated limitation, not an oversight.

  Add no logger to `Run` and no logger seam to `RunDeps`: per the overview's `cycle-visibility-is-the-envelope, not a log line` Shared Decision, the `Cycles` field plus the prepended `Warnings` lines plus card 7's `cycles` envelope key are this task's whole cycle-visibility surface.

  Change nothing about `batchIdentity`, `verifyEveryBatchDone`, `ReportFileName`, or `State.Batches`' keying: this card reorders the slice and nothing else.
- **Commit:** `feat(websterengine): sequence execution batches in Run and report detected cycles`

### Card 5: begin-batch renders the execution predecessor's digest

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/runlevel.go`
- **Edits:**
  - `internal/websterengine/beginbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an unexported helper to `internal/websterengine/beginbatch.go`, beside the existing `digestSummaryLine`:
  `func predecessorDigestLine(batches []batcher.Batch, st *State, batchNumber int) string`.
  It locates `batchNumber`'s position in `batches` by `batchIdentity`, exactly as the existing `findBatch` does;
  returns `""` when the batch is absent from the slice or sits at index 0 (nothing executed before it);
  otherwise reads `batchIdentity(batches[idx-1])`'s number, looks that number up in `st.Batches`, and returns `digestSummaryLine` of that entry's `Digest`, or `""` when the entry or its digest is absent.
  It tolerates a nil `st` and a nil `st.Batches` by returning `""`.
  Its doc comment must state that `batches` is required to already be in execution order, that this is what `SequenceBatches` at every call site guarantees, and that the old `batchNumber-1` arithmetic was correct only while the identity batchifier made batch number and execution position coincide.

  In `BeginBatch`, replace the block

  ```go
	var prevDigest string
	if batchNumber > 1 {
		if prev, ok := deps.State.Batches[batchNumber-1]; ok && prev != nil {
			prevDigest = digestSummaryLine(prev.Digest)
		}
	}
  ```

  with a single call to the new helper against `deps.Batches`, `deps.State`, and `batchNumber`.
  Everything downstream is unchanged: `RenderForkPrompt` still substitutes the `noPrecedingBatchDigest` sentinel for an empty result, so the first batch in execution order still renders the no-previous-digest sentinel even when its number is not 1.

  Update `BeginDeps.Batches`' own field documentation to say the slice is the sequenced execution order, not merely the batchifier's output, and that the predecessor lookup depends on that ordering.
- **Commit:** `fix(websterengine): render the execution predecessor's digest into the fork prompt`

### Card 6: recover-batch renders the execution predecessor's digest

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/render.go`
- **Edits:**
  - `internal/websterengine/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `RecoverSpawnOrAttach` in `internal/websterengine/recoverbatch.go`, replace the identical `batchNumber > 1` / `deps.State.Batches[batchNumber-1]` block with a single call to card 5's `predecessorDigestLine` helper against `deps.Batches`, `deps.State`, and `batchNumber`.
  Both sites want the same "what actually ran before this" answer — `beginbatch` renders it into an in-session fork prompt, `recoverbatch` into a cold recovery prompt — so both take the identical correction and share the one helper rather than duplicating it.
  `RenderRecoveryPrompt`'s own empty-digest sentinel handling is unchanged.

  Update `RecoverDeps.Batches`' field documentation the same way card 5 updates `BeginDeps.Batches`': the slice is the sequenced execution order and the predecessor lookup depends on it.
- **Commit:** `fix(websterengine): render the execution predecessor's digest into the recovery prompt`

### Card 7: sequence at every webstercli batch-computation site

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/runlevel.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/wiring.go`
- **Edits:**
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/awaitbatch.go`
  - `internal/webstercli/recordbatch.go`
  - `internal/webstercli/recoverbatch.go`
  - `internal/webstercli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In each of `internal/webstercli/beginbatch.go`, `internal/webstercli/awaitbatch.go`, `internal/webstercli/recordbatch.go`, and `internal/webstercli/recoverbatch.go`, replace the bare `batches := c.batcher.Batch(plan.Cards)` line with a sequenced form: take the batchifier's output and pass it through `websterengine.SequenceBatches`, discarding the cycle list with `_` (the `run` verb is the one that surfaces cycles;
  a bracket verb re-reporting them on every call would be noise).
  Add a one-line comment at each site stating that every batch-computation site sequences, so all five agree on one order by construction rather than by comment.
  These four verbs look batches up by identity via `findBatch`/`batchSlugFor` and are order-insensitive in themselves;
  the sequencing matters because `BeginDeps.Batches` and `RecoverDeps.Batches` now feed card 5's and card 6's predecessor lookup.
  `internal/webstercli/recoverbatch.go`'s `batchSlugFor` helper is unchanged — it scans by number and does not care about order.

  In `internal/webstercli/run.go`'s `run` envelope, add a `"cycles"` key alongside the existing `"warnings"` key, carrying `result.Cycles` mapped to a plain `[][]int` of member batch numbers (build it with a small local loop over `result.Cycles`, defaulting to an empty, non-nil slice so the key renders as `[]` rather than `null` on the common acyclic run).
  Mention the new key in the `run` command's `Long` help text in one sentence: an acyclic plan reports an empty list, and a non-empty list names mutually-dependent batches that were condensed and run in declared order — never a failure.
  `runDeps` is untouched.
- **Commit:** `feat(webstercli): sequence execution batches at every verb and report cycles from run`

### Card 8: reword the master template so "in order" means the listed order

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
  - `internal/websterengine/render.go`
  - `internal/websterengine/template_test.go`
- **Edits:**
  - `contracts/stencils/webster/webster-template-master.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Reword the `## Your card list (fixed at spawn, or resume)` section's prose beneath `{{.batch_index}}` in `contracts/stencils/webster/webster-template-master.md` so the ordering instruction is unambiguous.
  Specifically:

  - The sentence currently reading "This ordered list is the plan's own flat card list — every card, in declared order, one line each: number, slug, one-line intent." must instead say the list is one line per execution batch in the order webster derived from the cards' own declared dependencies, still carrying number, slug, and one-line intent.
  - Keep the existing "It is your navigation source, not the execution unit" sentence and its batchifier explanation as they are.
  - Keep the literal phrase `Drive it STRICTLY in order` — an existing template property test asserts it — but make "in order" mean **the order listed above, top to bottom**, and state explicitly that this is NOT necessarily ascending batch number: a batch's number is its identity, never its position, so the list may legitimately run `03` before `02`.
  - Replace the clause "there is no DAG here to reorder around" — now false — with the statement that each entry assumes every entry ABOVE it in the list is already committed.
  - Keep the prohibition "no batch is ever skipped or reordered because it 'looks independent.'" verbatim.
  - In the `## The loop` section, change the line "For each batch not already reported, in order:" so it reads "top to bottom in your card list above" rather than a bare "in order".

  Constraints on the reword: per the **Producer Pointer-Rule Invariant** it describes the ordering rule and nothing else — it must not restate or paraphrase the plan format, the edge-derivation rule, or how the graph is built.
  Per the **Fabric Vocabulary Invariant** it introduces none of the policed `host`-sense phrases and no bare `weft`/`warp` usage.
  It must not introduce the words "oversized" or "chain" in any case, or a `## Scope` heading — `TestTemplates_NoDroppedBatchConceptsRemain` bans all three across this template.
  Add no new `{{.marker}}`: the template's eight markers are unchanged, which is what keeps `TestMasterTemplate_FillsWithAllMarkers` and `TestMasterTemplate_PatternDirectiveOptional` green.
  Use semantic line breaks throughout, matching the file's existing style.
- **Commit:** `docs(stencils): make Master's ordering instruction mean the listed order`

### Card 9: render and master-template test updates

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/sequence.go`
  - `contracts/stencils/webster/webster-template-master.md`
- **Edits:**
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update every call site in `internal/websterengine/template_test.go` that the card 3 signature changes break, and add the new coverage the reworded template and reordered renderers need.

  Signature fallout:

  - `TestRenderMasterPrompt_MissingPatternStencilErrors` and `TestRenderMasterPrompt_NeverFillsWorktreeRoot` currently build a `*planparser.Plan` and pass it as `RenderMasterPrompt`'s first argument;
    both must build a `[]batcher.Batch` instead, reusing the file's existing `cardWithSourcePath` helper to make each batch's single card.
  - `TestRenderProgress_ListsOnlyTerminalBatches` currently builds a four-card `*planparser.Plan`;
    convert it to a four-element `[]batcher.Batch`, keeping every existing sub-test and its expected output string unchanged — the nil-state `"none"` case, the no-terminal-batches `"none"` case, and the mixed case's `"01-seam-extensions: done\n02-webster-foundation: stuck"` expectation.
    That test must keep passing with its assertions intact;
    only the fixture's type changes.

  New coverage:

  - A `RenderBatchIndex` test asserting the emitted lines follow the slice's own order, not ascending batch number: hand it a slice already ordered `03`, `01`, `02` and assert the rendered text's lines appear in that order, each still carrying its own batch number.
  - A `RenderProgress` test asserting the same ordering property against a state where all three batches are terminal.
  - A `RenderMasterPrompt` test asserting the `{{.batch_index}}` region of the rendered prompt reflects the sequenced order it was handed, so the rendered list and the verbs cannot diverge.
  - Extend `TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder`, or add a sibling template property test beside it, asserting the reworded template: it still contains `Drive it STRICTLY in order`, it now states that the order is the listed one and not ascending batch number, it still forbids skipping or self-directed reordering (`looks independent`), and it no longer contains the retired clause `there is no DAG here to reorder around`.

  Keep the file in package `websterengine_test` and Tier 1 — no new spawn, no new git, no `time.Sleep`.
- **Commit:** `test(websterengine): cover sequenced rendering and the reworded master template`

### Card 10: predecessor-digest test updates

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/planparser/plan.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/render.go`
- **Edits:**
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/recoverbatch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/websterengine/beginbatch_test.go`, rewrite `TestBeginBatch_PromptFilePrevDigest` so it asserts execution-predecessor semantics rather than the retired `batchNumber-1` arithmetic:

  - Keep the existing "batch 1 renders the first-batch sentinel" and "prompt path is under PromptsDir" sub-tests as they are.
  - Keep the existing "batch N>1 renders the persisted predecessor digest" sub-test, whose fixture batches are already in the order `1, 2`, so batch 2's predecessor is batch 1 under both the old and the new rule.
  - Add a sub-test for a reordered plan: override `fx.Deps.Batches` with the fixture's two batches in the order `2, 1` (batch 2 first in execution order), seed `fx.Deps.State.Batches` with a terminal digest for batch 2, call `BeginBatch(fx.Deps, 1)`, and assert the written prompt file carries batch 2's digest — the batch that actually ran before it — and not the first-batch sentinel.
  - Add a sub-test asserting that the batch sitting FIRST in a reordered `fx.Deps.Batches` renders the `none (first batch)` sentinel even though its number is not 1: with the same `2, 1` ordering, `BeginBatch(fx.Deps, 2)` must render the sentinel.
    Batch 2's report must not already exist for this call, since `BeginBatch` refuses a batch whose report is on disk.

  In `internal/websterengine/recoverbatch_test.go`, add the mirror coverage for `RecoverSpawnOrAttach` using that file's own existing recovery fixture: a reordered `Batches` slice makes the recovery prompt carry the execution predecessor's digest, and the first batch in execution order renders the no-previous-digest sentinel regardless of its number.
  Follow whatever assertion style that file already uses for inspecting a rendered recovery prompt;
  do not introduce a new fixture shape.

  Both files carry `//go:build integration` and are Tier 2 by design — they drive real scratch git repos through the existing fixtures.
  Keep that posture: stay in package `websterengine_test`, keep the build tag, and reuse the fixtures rather than introducing a new spawn shape.
- **Commit:** `test(websterengine): assert execution-predecessor digest lookup in begin/recover-batch`

### Card 11: Run-level sequencing and cycle-reporting tests

- **Context:**
  - `_mill/discussion.md`
  - `internal/batcher/batcher.go`
  - `internal/planparser/plan.go`
  - `internal/planparser/validate.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/integration.go`
  - `internal/websterengine/integration_test.go`
- **Edits:**
  - `internal/websterengine/runlevel_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add Run-level coverage to `internal/websterengine/runlevel_test.go` for the sequencing wired in by card 4.
  Reuse `newRunFixture`, `seedRunPlanDir`, `seedMatchingState`, `seedShuttleRunState`, and the file's existing fake `MasterStarter`;
  do not build a new fixture shape.

  Add a small local helper that rewrites one already-seeded card file under the fixture's plan dir to carry a `**Uses:**` field naming a given ref, modelled on `integration_test.go`'s own `appendIntegrationVerify` (line 36), which mutates an already-seeded plan the same way and sits in the same `websterengine_test` package.
  Two facts constrain the fixture and must both be honored, or `planparser.Validate` will refuse the run before sequencing is ever reached: a path-shaped `Uses` entry IS existence-checked against `Geom.WorktreeRoot` (`checkPathMissing`), while a Create card's own targets are exempt from that check.
  So any card file the helper points at another card's target path must have that path created as a real file inside `fx.Worktree` first.

  Cover:

  - **Reordering is observable.** Build a two-card fixture, give card 1 a `**Uses:**` naming card 2's own target path (and create that path under the worktree), so card 2 must run before card 1.
    Drive the run to a done outcome the way `TestRun_DoneOutcomeWithValidSummaryAndCleanAuditPopulatesResult` already does, and assert the rendered Master prompt the fake starter captured lists batch `02` above batch `01` in its `{{.batch_index}}` region.
  - **Cycle reporting.** Build a two-card fixture where card 1 `Uses` card 2's target and card 2 `Uses` card 1's target, creating both files under the worktree.
    Assert the resulting `RunResult.Cycles` carries exactly one cycle naming batches 1 and 2, and that `RunResult.Warnings` carries that cycle's own `Warning()` line.
    Assert the run still reaches its ordinary done outcome — a cycle is never fatal and never changes the exit path.
  - **The acyclic case reports nothing.** An unmodified `newRunFixture` plan, whose cards reference nothing of each other's, produces an empty `RunResult.Cycles` and adds no sequencing warning.

  Leave `TestRun_ZeroBatchPlanRefusedLoud` and its expectations untouched: `SequenceBatches` runs after that refusal and is length-preserving, so the zero-batch path is unchanged by design and this card must prove that by not needing to edit it.

  Keep the file's existing tier posture: it carries `//go:build integration` and is Tier 2, driving a real scratch git repo through `newRunFixture`.
  Keep the build tag and add nothing that changes which tier the file sits in.
- **Commit:** `test(websterengine): assert Run sequences batches and reports condensed cycles`

### Card 12: doc.go seam rewrite and package-doc corrections

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
  - `internal/websterengine/sequence.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/batcher/doc.go`
- **Edits:**
  - `internal/websterengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite `internal/websterengine/doc.go`'s `# Declared order now, a dead DAG seam for later` section, which is now false in every particular.
  Retitle the section to name what the package actually does — deriving the execution order from the plan's own card refs — and replace its body with an accurate description: `sequence.go`'s `SequenceBatches` derives edges from `Targets`/`Uses` across the plan's cards, condenses strongly-connected components, and returns a deterministic topological order plus the cycles it condensed;
  a cycle is reported and never fatal;
  an already dependency-correct plan sequences to exactly its declared order.
  Delete the `if card.HasSymbolFields() { … } else { … }` code sketch and every mention of `HasSymbolFields` — that method never existed, and this doc comment held the only two references to the name in the repo, so nothing in code needs deleting alongside it.
  State that sequencing is unconditional: there is no config key and no opt-in.
  State that the mechanism is why every batch-computation site — `Run` plus the four `internal/webstercli` bracket verbs — must sequence, and that the previous-digest lookup in `beginbatch.go`/`recoverbatch.go` depends on that ordering.

  Correct the package doc's opening paragraph, which currently says forks run "sequentially, in the plan's declared card order": execution is still strictly sequential — one fork at a time, one worktree — but the order is now derived, not declared.
  Do not weaken the sequential claim while correcting the order claim;
  concurrency remains explicitly out of scope.

  Correct the parenthetical at the `RenderProgress` mention further down the file, which says the reserved `-1` integration key can never surface because `RenderProgress` walks positive card numbers: it now walks batch numbers, which are equally positive, so update the wording to match without changing the guarantee it states.

  Keep the **Batcher Registry+Config Invariant**'s line intact in spirit: state plainly that `internal/batcher` still owns grouping and that this package owns only the sequencing of the batches a batchifier returned.
- **Commit:** `docs(websterengine): replace the dead DAG seam with the shipped sequencing mechanism`

## Batch Tests

The batch's `verify:` is two `&&`-chained `go test` invocations: an untagged run over `./internal/websterengine/...`, `./internal/webstercli/...`, `./internal/batcher/...`, and `./contracts/stencils/...`, then a `-tags integration` run over `./internal/websterengine/...` and `./internal/webstercli/...`.
The tagged half is not redundant: `beginbatch_test.go`, `recoverbatch_test.go`, and `runlevel_test.go` — the three files cards 10 and 11 edit — all carry `//go:build integration`, so an untagged run does not compile or execute them at all.
Between them the two invocations cover exactly the packages this batch's `Edits:` touch, plus two it must not break.

- `internal/websterengine` holds every renderer, verb-logic, and sequencing change plus cards 9, 10, and 11's tests — the untagged `template_test.go` and batch 1's `sequence_test.go` in the first invocation, and the integration-tagged `beginbatch_test.go`, `recoverbatch_test.go`, and `runlevel_test.go` in the second.
- `internal/webstercli` holds card 7's four verb sites and the `run` envelope, and its `cli_test.go`/`verbs_test.go`/`smoke_test.go` are what catch a wiring regression there.
- `internal/batcher` must stay untouched by this task and is run to prove it: a failure there means the batch strayed across the **Batcher Registry+Config Invariant**'s line.
- `contracts/stencils` holds `registry_test.go`, the registry-completeness guard over the seed stencils card 8 edits.

The overview's module-wide `verify: go build ./...` additionally catches any caller of `RenderMasterPrompt`/`RenderBatchIndex`/`RenderProgress` outside these four packages that card 3's signature change would break.

Two repo-wide guards this batch must keep green are outside its own verify scope and are covered by the run-level `pipeline.done_gate` instead: `cmd/lyx/tierpurity_test.go` (card 9's additions land in the untagged `template_test.go` and must stay pure;
cards 10 and 11's land in already-integration-tagged files, which that guard does not police) and `cmd/lyx/hermeticenv_test.go` (no new package starts spawning git, and `internal/websterengine` already carries its `TestMain`).
