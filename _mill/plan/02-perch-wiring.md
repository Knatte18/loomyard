# Batch: perch-wiring

```yaml
task: 'loom: Webster-Review producer'
batch: 'perch-wiring'
number: 2
cards: 8
verify: go build ./... && go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/...
depends-on: [1]
```

## Batch Scope

This batch replaces the stubbed `Webster-Review` row with the real `Webster-Bouncer` + `Webster-Burler` perch: the two durable row-name constants, the recipe rows themselves, and every `internal/loomrecipe` test that asserts the built list's shape, order, coverage, and run sequence.

It is one batch because the pieces are one atomic rename plus its consumers.
Card 4 retires `loomshed.NameWebsterReview`, which cards 8 through 11 reference by symbol, so the package does not compile until all five land — splitting them would produce a batch whose own `verify:` cannot pass.
Card 7 is in the same batch for a different reason: `shedadapters.NewBouncer` probes its rubric stencil eagerly at construction, so every `internal/loomrecipe` test that calls `New` fails at construction until `seedBouncerStencils` seeds `loom-rubric-webster-review`.

Batch-local decision: `internal/shedrecipe` gets no new test.
Every config key this perch uses is already covered by the two shipped segments' cases in `entries_bouncer_test.go` and `entries_burler_test.go`, and this batch adds no key that is not already exercised there.

## Cards

### Card 4: Retire NameWebsterReview for the two perch row names

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `internal/loomshed/loomshed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in the `const` block, delete `NameWebsterReview = "Webster-Review"` and add, in its place and in this order, `NameWebsterBouncer = "Webster-Bouncer"` and `NameWebsterBurler = "Webster-Burler"`, keeping the block's existing gofmt alignment (re-align the whole block if the two longer names widen it).
  The two new constants sit between `NameWebster` and `NamePublish`, matching the recipe's own row order.
  Update the file's row counts: the file-header comment on line 1 and the const block's own doc comment both say "sixteen" and become "seventeen" — three sites in this file, the header plus two inside the const doc ("The sixteen producer names" and "spells the same sixteen names as yaml strings").
  These are producer-row counts, so they move per the Shared Decision; do not change any other count.
- **Commit:** `refactor(loomshed): replace NameWebsterReview with the Webster perch row names`

### Card 5: Retire the stub's loom-row claim

- **Context:** none
- **Edits:**
  - `internal/loomshed/doc.go`
  - `internal/loomshed/stub.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `internal/loomshed/doc.go`, the package doc's "its sixteen durable row names" becomes "seventeen".
  In `internal/loomshed/stub.go`, reword both comment sites — the file-header comment on line 2, which today says the type backs "the one row of loom's 16-row producer list that no task has built for real yet." and stops there, and `stubProducer`'s own doc comment, which says the same thing and then names the row: "-- Webster-Review -- replaced by a real producer in a later task".
  After this task the type backs no loom row at all, so this is a change of claim and not only of count: state instead that `stubProducer` is a placeholder `ShedProducer` that loom's own producer list no longer uses, kept because `internal/shedrecipe`'s registry is generic `Shed` machinery shared by reference with a future product's producer list rather than loom's private property, and that it exists so a list's sequencing, resume, crash-recovery, and pause behaviour is real from the start rather than retrofitted.
  Do not change `NewStub`, `Call`, or any other code in either file, and do not delete either file.
- **Commit:** `docs(loomshed): reword the stub's doc comments now no loom row uses it`

### Card 6: Wire the Webster-Review perch into the recipe

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** three edits to this file.

  (a) Header comment, two changes.
  On line 2, "The sixteen row names below" becomes "The seventeen row names below".
  The paragraph beginning "Both review segments follow the same shared-segment mutual-on_stuck shape" (lines 12-16) enumerates the Discussion and Plan pairs by name and becomes three segments: reword its opening to "All three review segments", and extend the enumeration so `Webster-Bouncer` and `Webster-Burler` carrying `segment: Webster-Review` is named alongside the two existing pairs.
  Keep the paragraph's closing sentence about `internal/shedengine`'s validator rejecting an `OnStuck` naming a producer in a different `Segment` unchanged.

  (b) The `Webster` row's `on_done` changes from `Webster-Review` to `Webster-Bouncer`.

  (c) Replace the whole `Webster-Review` row (`engine: Stub`, `on_stuck: Webster`, `on_done: Publish`) with the two rows below, placed between the `Webster` row and the `Publish` row, verbatim including comments and blank-line spacing:

  ```
    - name: Webster-Bouncer
      engine: Bouncer
      segment: Webster-Review
      max_bounces: 5
      on_stuck: Webster-Burler
      on_done: Publish
      config:
        run_subdir: webster
        # The subject under review is the committed diff, which artifact_paths cannot name at all:
        # the key is required and non-empty, every entry resolves to an absolute path under
        # Env.WorktreeRoot, and the generic bouncer-template-judge.md renders the list as the
        # artifacts under review with "read each one". Whatever value is chosen is a workaround, so
        # the question is only which one gives the judge the most useful reading -- the single plan
        # directory entry is the card contract the diff is measured against, and Plan-Bouncer
        # already proves a bare directory entry works here (NewBouncer stats nothing). The rubric
        # is where the diff itself is named as the subject, which reaches the judge and the fixer
        # round alike because both rows read it.
        artifact_paths:
          - _lyx/plan
        rubric_stencil: loom-rubric-webster-review
        # No commit_seam key, deliberately unlike Plan-Bouncer: Webster-Burler runs fix-scope:
        # source and commits each fix itself, so there is no artifact left for a loop-owner seam to
        # commit. bouncerEntry documents the resulting nil Commit as a legitimate configuration and
        # never an error. The segment's own round artifacts are uncommitted by construction --
        # Env.RunRoot lands under the ephemeral .lyx tree, not the durable _lyx one.
        # No model/effort/version key: the absence is what makes this row take the run-wide
        # Env.Review* values loom.yaml supplies, rather than a recipe-literal, untunable-without-a-
        # rebuild model.

    - name: Webster-Burler
      engine: BurlerRound
      segment: Webster-Review
      max_bounces: 5
      on_stuck: Webster-Bouncer
      # BurlerProducer never returns Done, so this edge is unreachable -- but an empty on_done is
      # load-bearing and ends the whole run silently, which is a worse failure than a redundant edge,
      # so it is set explicitly to the row it can never actually reach.
      on_done: Webster-Bouncer
      config:
        # The same run_subdir value as Webster-Bouncer is what makes both rows write into one shared
        # run directory.
        run_subdir: webster
        profile:
          target:
            # No paths key, deliberately: burlerengine's own Profile.validate resolves and stats
            # every Target.Paths entry, and a diff has no such file on disk. validate accepts a
            # FileSet carrying Instructions and no Paths, which is exactly the escape this row
            # needs.
            instructions: >
              The subject under review is the committed diff. The rubric supplied in this prompt is
              the single definition of the review range: read its "Determining the review range"
              section and derive the range exactly as written there. This instruction deliberately
              does not restate the derivation.
          fasit:
            paths:
              - _lyx/plan
            instructions: >
              The plan directory named above is the answer key: the diff's job is to implement the
              cards it carries, and the Card model those cards implement is described in
              manifest/designs/plan-card-format.md. The mechanical checks over the plan's own format
              are already enforced upstream by Plan-Validate and Plan-Revalidate, so re-deriving
              them in this round is duplicated work.
          rubric_stencil: loom-rubric-webster-review
          # fix-scope: source, matching the shipped Discussion-Burler row rather than Plan-Burler's
          # overlay. The target is the repo's own source files, which the Fabric Git Invariant names
          # as the one explicitly permitted agent commit (commit-per-fix, never a push), whereas the
          # plan Plan-Burler fixes is overlay content the loop owner commits. An overlay round runs
          # no git at all and restricts writes to Target.Paths, which this profile deliberately
          # leaves empty -- a fixer that cannot write source cannot fix a diff.
          fix-scope: source
          # tool-use is required: the round reads _lyx/loom/status.json and runs read-only git to
          # obtain the diff, which is its entire subject.
          tool-use: true
          # No cluster-fan key: fork reviewers are read-only and may never run any git command,
          # which burlerengine's fork boilerplate states and its own round audit enforces, so under
          # a fan the forks could not reach a subject that exists only through git. Single reviewer,
          # matching both shipped segments.
        # No model/effort/timeout_s key, for the same Env fallback reason as Webster-Bouncer.
  ```
- **Commit:** `feat(loom): replace the Webster-Review stub row with a Bouncer/Burler perch`

### Card 7: Seed the Webster-Review rubric in the recipe test fixture

- **Context:**
  - `contracts/stencils/stencils.go`
- **Edits:**
  - `internal/loomrecipe/fixture_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add `"loom-rubric-webster-review": stencils.LoomRubricWebsterReview,` to `seedBouncerStencils`'s `seeds` map, after the `loom-rubric-plan-review` entry.
  Without it, `shedadapters.NewBouncer`'s eager rubric probe fails at construction and every test in this package that calls `New` fails — this is the single most likely source of a first-run failure in this batch.
  Update `seedBouncerStencils`'s own doc comment, which today says the helper "writes the four stencils a live Discussion-Review or Plan-Review segment reads" and enumerates both rubrics by name: it becomes five stencils across three segments, naming the Webster-Review rubric alongside the other two.
  Update `fakeLoomShuttle`'s doc comment where it says the fake serves "both segments' Bouncer rows' spawn roles" — it now serves all three segments' Bouncer rows;
  the fake itself branches only on `spec.Role` and needs no code change.
- **Commit:** `test(loomrecipe): seed the Webster-Review rubric in the fixture stencil set`

### Card 8: Point the coverage guard at the perch's two engines

- **Context:**
  - `internal/shedrecipe/registry.go`
- **Edits:**
  - `internal/loomrecipe/coverage_guard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `loomRowEngines`, delete the `loomshed.NameWebsterReview: "Stub",` entry and add `loomshed.NameWebsterBouncer: "Bouncer",` and `loomshed.NameWebsterBurler: "BurlerRound",` in its place, keeping the map's recipe-order layout.
  Add `"Stub": true,` to `coverageGuardAllowedUnreachableEngines` alongside the existing `"SingleLLM"` entry, and rewrite that variable's doc comment to say why: no loom row reaches `Stub` any more now that the last stubbed row is real, and the engine stays registered because `internal/shedrecipe`'s registry is generic `Shed` machinery shared by reference with a future product's producer list rather than loom's private property.
  Update the `loomRowEngines` doc comment's "each of New's sixteen row names" and the allowlist comment's "any of the sixteen built rows" to seventeen.
  Both existing directions of `TestCoverageGuard_EveryLoomRowHasAnEngine` then cover the change with no new test.
  Do not touch `internal/shedrecipe/registry_test.go` — `TestRegistry_ShipsFourteenEntries` still pins fourteen registry names, because this task removes no registry entry.
- **Commit:** `test(loomrecipe): map the Webster perch rows to their engines in the coverage guard`

### Card 9: Extend the producer table with the two perch rows

- **Edits:**
  - `internal/loomrecipe/shape_test.go`
- **Context:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `wantProducerTable`, change the `loomshed.NameWebster` row's `onDone` column from `loomshed.NameWebsterReview` to `loomshed.NameWebsterBouncer`, then replace the `loomshed.NameWebsterReview` row with these two, in this order:

  `{loomshed.NameWebsterBouncer, loomshed.NameWebsterBurler, loomshed.NamePublish, "Webster-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},`

  `{loomshed.NameWebsterBurler, loomshed.NameWebsterBouncer, loomshed.NameWebsterBouncer, "Webster-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},`

  The table then has seventeen rows, and `TestNew_RoutingGraphIsClean` proves the new edges resolve with no new test.
  Update the file-header comment's "the literal sixteen-row producer table" and `TestNew_ProducerTableOrderUnchangedByWiring`'s "the sixteen rows stay in their existing table order ... regardless of what backs rows 15 and 16" — the counts become seventeen, and the trailing row indices in that second comment must be re-derived against the new table rather than merely incremented, since `Publish` and `Finalize` are now rows 16 and 17.
- **Commit:** `test(loomrecipe): add the Webster perch rows to the producer table`

### Card 10: Extend the run sequence with the Webster-Review segment

- **Edits:**
  - `internal/loomrecipe/sequence_test.go`
- **Context:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `wantSequenceOrder`, replace the single `{loomshed.NameWebsterReview, shedengine.Done},` entry with the segment's own three-entry shape, in this order: `{loomshed.NameWebsterBouncer, shedengine.Stuck},`, `{loomshed.NameWebsterBurler, shedengine.Stuck},`, `{loomshed.NameWebsterBouncer, shedengine.Done},`.
  The list then runs to nineteen entries and still ends at `{loomshed.NamePublish, shedengine.Stuck}`.
  In `TestSequence_FullRunBlocksAtPublish`, the two scenario counts at the end of the test both rise by one, because a third segment now runs: `fakeLoomBurler.calls` becomes 3 and `fakeLoomShuttle.bouncerJudgeCalls` becomes 3, in both the comparison and its failure message.
  `commitDiscussionCalls` stays 1 and `commitPlanCalls` stays 2 — the Webster segment configures no commit seam, so it adds no commit call.
  Update `wantSequenceOrder`'s doc comment: "Every entry but the two review segments and the trailing Publish" becomes three review segments, "The list runs to seventeen entries total" becomes nineteen, and the sentence describing each segment's three-entry shape stays true unchanged.
  Update the comment above the two scenario counts, which says both review segments genuinely ran, to name all three.
  Update `TestSequence_FullRunBlocksAtPublish`'s own doc comment, whose "the sixteen-row list" becomes seventeen-row.
- **Commit:** `test(loomrecipe): extend the run sequence with the Webster-Review segment`

### Card 11: Move the recipe shape test's row count to seventeen

- **Edits:**
  - `internal/loomrecipe/recipe_test.go`
- **Context:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `TestNew_ShapeMatchesRecipe`, change both literal row-count guards from `16` to `17` — the `len(shed.Producers) != 16` check and the `len(wantProducerTable) != 16` check — including the `want 16` text in both `t.Fatalf` messages.
  Update the test's doc comment, whose "asserts exactly sixteen rows" becomes seventeen.
  No other test in this file asserts a row count.
- **Commit:** `test(loomrecipe): move the recipe shape test's row count to seventeen`

## Batch Tests

`verify: go build ./... && go test ./internal/loomshed/... ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/...`

`go build ./...` catches the constant rename's reach across production packages before any test runs.
`./internal/loomrecipe/...` is the batch's real gate: it holds the coverage guard, the producer-table and routing-graph guards, the recipe shape test, and the full-run sequence test, and every one of them builds the real recipe through `New`, so a mis-wired edge, a missing engine mapping, or an unseeded rubric stencil fails here.
`./internal/loomshed/...` covers the edited row-name and stub files, including `stub_test.go`, which this batch deliberately leaves passing unchanged.
`./internal/shedrecipe/...` proves the two perch rows' config keys are still exactly the ones the registry entries recognise, and that `TestRegistry_ShipsFourteenEntries` still holds with `Stub` registered.
`./internal/shedbuild/...` is included because it is the recipe-format parser the recipe edit in card 6 feeds, and its own test package builds recipes out of `Stub` rows that must keep working.
