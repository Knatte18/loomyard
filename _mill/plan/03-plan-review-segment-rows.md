# Batch: plan-review-segment-rows

```yaml
task: 'loom: Plan-Review producer'
batch: 'plan-review-segment-rows'
number: 3
cards: 2
verify: go build ./... && go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/shedbuild/...
depends-on: [1, 2]
```

## Batch Scope

This batch replaces loom's stubbed `Plan-Review` row with a real `Plan-Bouncer`/`Plan-Burler` perch, taking the recipe from fourteen rows to fifteen.
It depends on batch 1 for the rubric stencil the two rows name and on batch 2 for the `commit_seam` key the Bouncer row carries.

Card 9 is deliberately one large card rather than several small ones.
The row-name swap is genuinely atomic: `loomshed.NamePlanReview` is referenced from three `internal/loomrecipe` test files, and removing it while adding the two replacements breaks compilation in all of them until every reference moves in the same commit.
There is no alias mechanism in the recipe format, and the recipe cannot carry the old row and the new pair at once.
Splitting the work across cards would produce commits where `go build ./...` fails, which the plan's own source-first Shared Decision exists to prevent — so the atomic change is one card with one commit, per the never-require-two-cards-in-one-commit rule.

Card 10 is genuinely separable — it changes only doc comments in `internal/loomshed`, which compile identically before and after.

The external interface batch 4 documents is the shipped fifteen-row list and the two new row names.

## Cards

### Card 9: Swap the stubbed Plan-Review row for the Plan-Bouncer/Plan-Burler perch

- **Context:**
  - `contracts/stencils/loom/loom-rubric-plan-review.md`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/shedengine/validate.go`
  - `internal/loomrecipe/resume_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `_mill/discussion.md`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
  - `internal/loomshed/loomshed.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/loomrecipe/recipe_test.go`
  - `internal/loomrecipe/sequence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Seven files, one commit.
  Work them in the order below;
  the whole set must compile and pass together, and no intermediate state is committed separately.

  **1. `internal/loomshed/loomshed.go`.**
  Remove the `NamePlanReview` constant and add `NamePlanBouncer = "Plan-Bouncer"` and `NamePlanBurler = "Plan-Burler"` in its place, keeping the block's existing top-to-bottom recipe order (after `NamePlanValidate`, before `NameBatchifier`).
  The segment label `Plan-Review` gets no constant of its own: it stays a recipe-literal yaml string, exactly as `Discussion-Review` does, and adding one here would be a lone inconsistency.
  Removing `NamePlanReview` breaks resume for any in-flight task, which is accepted — the same trade the `Discussion-Review` rename already made, and there is no in-flight task to protect.
  Update the file's own header comment and the constant block's leading comment: fourteen durable row names become fifteen.

  **2. `contracts/recipes/loom-recipe.yaml`.**
  Repoint `Plan-Validate`'s `on_done` from `Plan-Review` to `Plan-Bouncer`, leaving its `on_stuck: Plan-Write` unchanged, and replace the `Plan-Review` `Stub` row with the two rows below, verbatim including their comments:

```yaml
  - name: Plan-Bouncer
    engine: Bouncer
    segment: Plan-Review
    max_bounces: 5
    on_stuck: Plan-Burler
    on_done: Batchifier
    config:
      run_subdir: plan
      # A plan is 00-overview.md plus a variable number of card files, so no enumeration written
      # here stays correct across plans -- the single directory entry is what stays right. Both
      # consumers accept a directory: shedadapters.NewBouncer stats nothing, and burlerengine's own
      # path check documents that a file and a directory both satisfy it. This entry resolves
      # against Env.WorktreeRoot, which is knowingly not the AnchorPath() root Env.CommitPlan
      # anchors at; the two are identical while AnchorRel is "." (its default), the shipped
      # Discussion pair carries the same shape, and the fix is filed as its own roadmap item rather
      # than made here.
      artifact_paths:
        - _lyx/plan
      rubric_stencil: loom-rubric-plan-review
      # commit_seam is required rather than optional here: Plan-Burler runs fix-scope: overlay and
      # an overlay round runs no git at all, and nothing else in this segment commits, so without
      # the seam every approved fix would stay uncommitted in the weft working tree.
      commit_seam: plan
      # No model/effort/version key: the absence is what makes this row take the run-wide
      # Env.Review* values loom.yaml supplies, rather than a recipe-literal, untunable-without-a-
      # rebuild model.

  - name: Plan-Burler
    engine: BurlerRound
    segment: Plan-Review
    max_bounces: 5
    on_stuck: Plan-Bouncer
    # BurlerProducer never returns Done, so this edge is unreachable -- but an empty on_done is
    # load-bearing and ends the whole run silently, which is a worse failure than a redundant edge,
    # so it is set explicitly to the row it can never actually reach.
    on_done: Plan-Bouncer
    config:
      # The same run_subdir value as Plan-Bouncer is what makes both rows write into one shared
      # run directory.
      run_subdir: plan
      profile:
        target:
          paths:
            - _lyx/plan
        fasit:
          paths:
            - _lyx/discussion/decision-record.md
          instructions: >
            The decision record named above is the answer key: the plan's job is to implement the
            decisions and constraints it settles. The format authority is
            contracts/specs/loom-plan-spec.md, and the mechanical checks over that format are
            already enforced upstream by Plan-Validate, so re-deriving them in this round is
            duplicated work.
        rubric_stencil: loom-rubric-plan-review
        # fix-scope: overlay, deliberately NOT the shipped Discussion-Burler row's source. The plan
        # directory is weft content reached through the junction, and the Fabric Git Invariant
        # reserves committing that class of file to the loop owner, never an agent. The Discussion
        # row's source value is the same violation, recorded and left to its own roadmap item, so
        # this divergence is on purpose rather than a copy-paste slip.
        fix-scope: overlay
        # tool-use is required: the round walks the plan directory's card files (the set is
        # variable and only the Card Index names it), resolves symbol-shaped target entries against
        # the repo, and reads the decision record.
        tool-use: true
      # No model/effort/timeout_s key, for the same Env fallback reason as Plan-Bouncer.
```

  Update the file's own header comment in the same edit: "The fourteen row names below" becomes fifteen, and the paragraph explaining the shared-`segment` mutual-`on_stuck` rule now describes both perches rather than the Discussion pair alone.

  **3. `internal/loomrecipe/fixture_test.go`.**
  Add `loom-rubric-plan-review` to `seedBouncerStencils`' map, seeded from `stencils.LoomRubricPlanReview`, alongside the three entries already there.
  This is not optional polish: `burlerRoundProfile` reads its `rubric_stencil` eagerly at construction, so without it every `New(env, paths)` call in this package fails.
  Update that helper's doc comment from three stencils to four, and from "a live Discussion-Review segment" to both live review segments.
  Update `fakeLoomBurler`'s and `fakeLoomShuttle`'s doc comments where they say the fake serves "the Discussion-Bouncer row" or that a later test asserts "the segment ran exactly one round" — both fakes now serve both segments, and the counts are two.
  Change no behaviour in either fake: both are already role-keyed and segment-agnostic.

  **4. `internal/loomrecipe/coverage_guard_test.go`.**
  In `loomRowEngines`, replace the `loomshed.NamePlanReview: "Stub"` entry with `loomshed.NamePlanBouncer: "Bouncer"` and `loomshed.NamePlanBurler: "BurlerRound"`, keeping the map's recipe-order layout.
  Update the map's own doc comment: fourteen row names become fifteen.
  Update `coverageGuardAllowedUnreachableEngines`' doc comment, which today explains that `Stub` stays reachable via the still-stubbed `Plan-Review` and `Webster-Review` rows — only `Webster-Review` keeps it reachable now.
  Do not add or remove an entry in `coverageGuardAllowedUnreachableEngines` itself: `SingleLLM` stays its sole entry, and `Stub` stays out of it because `Webster-Review` still reaches it.

  **5. `internal/loomrecipe/shape_test.go`.**
  In `wantProducerTable`, change the `NamePlanValidate` row's `onDone` column to `loomshed.NamePlanBouncer`, then replace the single `NamePlanReview` row with two rows, in recipe order:

```go
	{loomshed.NamePlanBouncer, loomshed.NamePlanBurler, loomshed.NameBatchifier, "Plan-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},
	{loomshed.NamePlanBurler, loomshed.NamePlanBouncer, loomshed.NamePlanBouncer, "Plan-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},
```

  The table now has fifteen rows.
  Update the file's own header comment where it says "the literal fourteen-row producer table", and `TestNew_ProducerTableOrderUnchangedByWiring`'s doc comment where it says "the fourteen rows stay in their existing table order".
  Every test in this file already sizes itself off `len(wantProducerTable)` rather than a literal, so no assertion body changes — verify that rather than assume it.

  **6. `internal/loomrecipe/recipe_test.go`.**
  `TestNew_ShapeMatchesRecipe` carries two literal `14` comparisons, one against `len(shed.Producers)` and one against `len(wantProducerTable)`;
  both become `15`, and the three failure messages naming 14 change with them.
  Update the test's own doc comment where it says "asserts exactly fourteen rows".
  Leave `TestRecipe_StructuralCheckHasNoFindings`, `TestNew_StatusPathCoherence`, `TestNew_ConstructionFailureNamesOffendingRow`, and `TestRecipe_SeedAndResumeRowNamesExist` untouched — all four are count-free and must keep passing unchanged.

  **7. `internal/loomrecipe/sequence_test.go`.**
  In `wantSequenceOrder`, replace the single `{loomshed.NamePlanReview, shedengine.Done}` entry with the same three-entry segment shape the Discussion pair already contributes, in this order: `{loomshed.NamePlanBouncer, shedengine.Stuck}` (the seed call), `{loomshed.NamePlanBurler, shedengine.Stuck}` (one completed review round), `{loomshed.NamePlanBouncer, shedengine.Done}` (the judge call whose fixture-scripted APPROVED verdict advances the run).
  The list goes from fourteen entries to sixteen — minus one, plus three.

  In `TestSequence_FullRunBlocksAtPublish`, three counter assertions change and one does not.
  `loomBurler.calls` becomes 2, `loomShuttle.bouncerJudgeCalls` becomes 2, and `loomShuttle.commitPlanCalls` becomes 2 — the last is the scenario proof that the new commit seam is reached on approval, so assert it explicitly with a comment saying so, rather than letting the number drift.
  `loomShuttle.commitDiscussionCalls` stays at exactly 1.
  Update `wantSequenceOrder`'s own doc comment: it explains the review segment's three-entry shape in terms of the Discussion pair alone, and must now describe both segments and the sixteen-entry total.
  Update `TestSequence_FullRunBlocksAtPublish`'s own doc comment where it says "the fourteen-row list".

  **Verify rather than assume, in this same card.**
  `internal/loomrecipe/resume_test.go` names no plan row and asserts no row count today;
  confirm that is still true after the change and leave the file untouched if so.
  `internal/shedrecipe/registry_test.go`'s `TestRegistry_ShipsFourteenEntries` pins the **engine registry** at fourteen and must not change: fifteen rows in loom's list, still fourteen engines in the registry.
  Confusing those two counts is the likeliest mistake in this task.
- **Commit:** `feat(loom): replace the stubbed Plan-Review row with a Plan-Bouncer/Plan-Burler perch`

### Card 10: Retarget stubProducer's documentation to Webster-Review alone

- **Context:**
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `stubProducer` and `NewStub` both stay — `Webster-Review` is still a `Stub` in the recipe and has its own roadmap item, and deleting the type would leave that row with no engine and break the coverage guard.
  Only the documentation changes.

  In `internal/loomshed/stub.go`, rewrite the file header comment and `stubProducer`'s own doc comment: the placeholder now backs **one** row of loom's fifteen-row producer list, `Webster-Review`, not two rows of a fourteen-row list.
  Leave the `NewStub` and `Call` doc comments alone except where they repeat the count.

  In `internal/loomshed/doc.go`, update the package doc's "its fourteen durable row names" to fifteen.
  Verify the "eight producer constructors" figure in the same sentence is still correct before touching it — this task adds no constructor to this package, so it should be unchanged.
- **Commit:** `docs(loomshed): retarget the stub producer's documentation to Webster-Review alone`

## Batch Tests

`verify: go build ./... && go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/shedbuild/...`

The `go build ./...` prefix is load-bearing for this batch specifically: card 9 removes an exported constant referenced from three test files, and a repo-wide build is the cheapest proof nothing outside the three edited test files still names it.
It runs in a couple of seconds and is not the full test suite.

`internal/loomrecipe` is the batch's real gate and carries every assertion that matters:

- `TestCoverageGuard_EveryLoomRowHasAnEngine` — both directions of the row-to-engine table, plus the fourth half asserting no registry engine is left unreachable outside the allowlist.
- `TestNew_ProducerTable`, `TestNew_ProducerTableOrderUnchangedByWiring`, and `TestNew_ShapeMatchesRecipe` — the fifteen-row shape, names, routing, segment labels, bounce budgets, and concrete producer types.
- `TestNew_RoutingGraphIsClean` — the highest-value assertion in the task: it is what proves the new mutual-bounce edges and the shared `Plan-Review` segment label are consistent, catching a Burler left with an empty `OnDone`, a Bouncer whose `OnDone` never exits its segment, and a Bouncer whose `OnStuck` never routes back.
- `TestNew_PassesShedValidation` and `TestRecipe_StructuralCheckHasNoFindings` — `shedengine`'s own validator, which rejects an `OnStuck` naming a producer in a different `Segment`, and `shedcheck` over the parsed recipe.
- `TestSequence_FullRunBlocksAtPublish` — the end-to-end proof, including `commitPlanCalls == 2`, which is the only place the new commit seam is exercised through a real `Shed` run rather than a unit fixture.

`internal/loomshed` covers card 10's package compiling with its own tests (`TestStubProducer_Call` is unaffected — only the comment above `stubProducer` changes) and card 9's constant block.
`internal/shedbuild` is included because it parses and builds recipes and carries its own fixtures over the engine registry;
it is a fast package and a cheap guard against the recipe edit breaking the loader's own expectations.

The suite is deliberately not the whole repo: `internal/shedadapters` and `internal/shedrecipe` are batch 2's scope and are unchanged here, and `go build ./...` already proves they still compile against the edited constants.
