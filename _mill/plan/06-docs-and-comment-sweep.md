# Batch: docs-and-comment-sweep

```yaml
task: 'loom: convert to a Shed recipe'
batch: 'docs-and-comment-sweep'
number: 6
cards: 4
verify: go test ./internal/lyxcwd/ ./internal/preflightshed/ ./internal/shedrecipe/ ./internal/shedcheck/ ./internal/shedbuild/ ./internal/loomcli/ && go vet -tags smoke ./internal/loomcli/
depends-on: [5]
```

## Batch Scope

The task's documentation obligation, landed as the final batch of the same task branch so the whole change squash-merges as one commit — which is what the project's Documentation Lifecycle rule requires.
Six markdown files carry statements this task falsifies or records its completion;
six Go doc comments — three in production files, three in test files — name symbols this task deleted or moved.

No production behaviour changes here.
The only Go edits are comment-text repairs in packages this task consumes rather than revises, which the `no-production-change-to-the-consumed-packages` Shared Decision explicitly carves out: a comment pointing at a deleted symbol is not "unchanged", it is wrong.

Batch-local decision: `CONSTRAINTS.md` takes **three** edits, not two.
Two are pointer corrections to existing invariants;
the third adds a new Recipe-Format Sole-Parser Invariant, because `manifest/roadmap.md`'s entry for this exact item states that "this item is where a sole-parser invariant for the recipe format belongs, added to `CONSTRAINTS.md` in that same commit as a review obligation, by direct analogy with the plan-format sole-parser invariant already recorded there — it is premature until this item ships the first production recipe and the first real consumer".
This task ships both, so the precondition the roadmap names is now met.
No *further* invariant is added beyond that one: `internal/loomrecipe`'s coverage guard already pins row-name↔engine coverage under the existing Shed Recipe Registry Invariant, which is a different property from sole-parsing and neither substitutes for the other.

## Cards

### Card 23: The three `CONSTRAINTS.md` edits

- **Context:**
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/seam_enforcement_test.go`
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedbuild/doc.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedbuild/recipe.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make exactly three edits.

  First, the **Shed Recipe Registry Invariant**'s "Enforced by" line, which names `internal/shedrecipe/coverage_guard_test.go` (`TestCoverageGuard_EveryLoomRowHasAnEngine`) for the registry-coverage half.
  That file no longer exists — its loom-driving half moved to `internal/loomrecipe/coverage_guard_test.go` and its exact-twelve-names pin moved to `internal/shedrecipe/registry_test.go`.
  Repoint the line to name both homes rather than one, and say which half each carries.
  Leave the told-geometry half's pointer at `internal/shedrecipe/seam_enforcement_test.go` unchanged.

  Second, the **Told-Geometry Invariant**'s **Machine-enforced** bullet, which enumerates every package whose `seam_enforcement_test.go` runs `TestToldGeometryInvariant_AllowlistOnly`.
  Add `internal/loomrecipe` to that enumeration, alongside `internal/loomshed`, `internal/landingshed`, `internal/mergeresolve`, `internal/shedrecipe`, and `internal/shedbuild`.
  Then update the invariant's own "Enforced by the eleven tests named above" line to twelve — adding the test without the count is exactly the stale enumeration this edit exists to prevent.
  Count the enumerated tests rather than trusting the arithmetic here.

  Third, add a **Recipe-Format Sole-Parser Invariant** section, modelled on the Planparser Sole-Parser Invariant already in the file and placed near it.
  State that `internal/shedbuild` is the sole parser of the recipe file format: no other package decodes a recipe document, and every consumer reaches a recipe only through the `shedbuild.Recipe` model `Parse`/`Load` returns.
  State that `internal/shedbuild` declares no on-disk location for recipe files — no directory constant, no filename convention, no embedded default — which its own package doc already asserts, so a shipped recipe's location is its owning product's to declare, as `contracts/recipes` does for loom.
  State that `internal/shedbuild` reaches the engine registry only through `shedrecipe.Lookup`/`Names` and adds no registration mechanism of its own, cross-referencing the Shed Recipe Registry Invariant rather than restating it.
  Mark it **Enforced by** a review obligation today, exactly as the Planparser Sole-Parser Invariant is, with a candidate future import/grep guard noted the same way — do not claim a machine check this task does not build.

  Change nothing else in the file.
  Keep semantic line breaks: one sentence per line, breaking inside long sentences only at an internal independent-clause boundary, with a plain newline and never a trailing double-space.
- **Commit:** `docs(constraints): repoint the recipe and told-geometry enforcement lines`

### Card 24: `manifest/` — the design doc, the roadmap, and parallel-work

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/recipes/recipes.go`
  - `internal/loomrecipe/doc.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomshed/doc.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedbuild/doc.go`
  - `internal/shedrecipe/registry_test.go`
  - `internal/landingshed/deps.go`
- **Edits:**
  - `manifest/designs/shed-recipe.md`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
  - `manifest/parallel-work.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/shed-recipe.md`: rewrite the "The idea" paragraph, whose opening sentence — "`internal/loomshed.New()` builds its 13-row `[]shedengine.ProducerDef` as a Go literal (`loomshed.go:137-151`)" — is falsified outright by batch 5, and whose framing of the whole doc as exploring a possible replacement is falsified by this task shipping it.
  Then drop the blockquote banner saying piece 4 "remains an early concept sketch, not a settled design" and "do not implement piece 4 from this doc as written", and retitle the H1, which currently reads "(pieces 1-3 shipped; piece 4 planned)".
  Mark piece 4 shipped in the "Pieces to build" list alongside the other three.
  Record the three decisions the doc explicitly deferred and this task settled: the on-disk location (an embedded shipped default at `contracts/recipes/loom-recipe.yaml`, read through `shedbuild.Parse`, with no seeding, no operator override, and no runtime on-disk path), the consumer (`internal/loomrecipe`, sitting above `internal/loomshed` because the registry already imports `loomshed` and a back-import would close a compile-time cycle), and test ownership (the assembled-graph tests live in `internal/loomrecipe`).
  Note the accepted consequence that `shedbuild.Load` now has no production caller, and that this is deliberate — it stays exported and covered because it is the entry a future non-embedded consumer needs.
  Update the "Status" line, which says the group's remaining work is the conversion item.

  In `manifest/designs/loom.md`: state that loom's producer list is recipe-backed, pointing at `contracts/recipes/loom-recipe.yaml` and `internal/loomrecipe`.
  The natural sites are the sentence defining `loom` as "`Shed` + `loom`'s own ordered producer list" and the producer-table section that follows it.
  Keep the table itself unchanged — no row's name, engine, or routing changes in this task.

  In `manifest/roadmap.md`: move the **loom: convert to a Shed recipe** item from Planned to Done with a description of what shipped, and close out the "Shed recipe" group's remaining-work framing in the section intro, which currently says three pieces have shipped "leaving one here".
  Then correct the present-tense claims inside already-Done entries this task falsifies — at minimum the "Shed recipe: engine registry" entry's claim that a coverage guard at `internal/shedrecipe/coverage_guard_test.go` "pins the registry against `loomshed.New`'s current, real row list, both directions", its claim that "`loomshed.New` keeps its own Go literal producer list and `loomshed.Deps.Preflight`'s pre-injected field unchanged; nothing downstream of this piece consumes it yet", and the "Shed recipe: loader/builder" entry's description of the shedbuild loom-equivalence test as a shipped artefact.
  The "Shed recipe" section intro also carries the same falsified `loomshed.go:137-151` Go-literal reference `manifest/designs/shed-recipe.md` does;
  correct it there too.
  A fourth Done entry is falsified and matches none of the tokens below: **loom: phase-machine scaffolding**, which asserts "`internal/loomshed` carries loom's full 13-row producer list".
  After batch 5 that package carries the thirteen row-name constants and six producer constructors, while the list itself is `contracts/recipes/loom-recipe.yaml`'s;
  restate it that way, keeping the rest of the entry (which rows are real, which are stubbed, and the `Plan-Sweep` note) untouched, since none of that changes.
  Treat those as a starting set, not a closed list: sweep both files for `loomshed.New`, `loomshed.Deps`, `coverage_guard_test`, `equivalence_test`, `loomshed.go:137-151`, `Go literal`, `13-row`, and `producer list`, and correct every hit.
  Restate each in the past tense and point at this item rather than rewriting the entries — both are written as claims about the tree's current state, not as a record of what a past task did, so leaving them would make the roadmap assert something false about `main`.

  Moving the conversion item to Done empties the `### Shed recipe: declarative producer lists` Planned section, which holds that one item and nothing else.
  Delete that heading and its intro paragraph rather than leaving a heading with zero items or a pointer stub: all four pieces of the group have now shipped, and the four Done entries carry the record.
  The `### loom: real LLM producers` section's own sentence "Sequenced after the \"Shed recipe\" group above so these five tasks write their rows directly as recipe entries" then names a section that no longer exists;
  restate it to point at the Done entries instead, keeping the ordering rationale intact.

  Also in `manifest/roadmap.md`: add a new Planned item covering the parent-fabric resolution chain that fills `landingshed.Deps`' `OpenFabric`/`OpenParentFabric`/`PushBranch` for loom.
  Give it its own `###` section under Planned, placed ahead of `### loom: real LLM producers` — it is not an LLM-producer task and does not belong inside that group.
  This is recording an already-documented gap, not proposing new scope: `internal/landingshed/deps.go` states that the resolution chain "belongs to the layer that legitimately resolves geometry, and the next roadmap item builds it", and no such item exists in Planned, Someday, or Done today — so that comment is currently false and the `landing-parity` Shared Decision has nothing to point at.
  Describe the work the way `deps.go` already does — list the worktrees, match the entry whose branch equals the parent branch, resolve that path, open it — and state the consequence that makes it worth an item: until it lands, `Publish` and `Finalize` construction fails for want of `Env.Landing`, so loom cannot complete a run.
  State that loom cannot complete an end-to-end run until it lands, without claiming the five LLM-producer tasks are blocked on it — they add rows and are developable independently.
  Do not implement any of it here.

  In `manifest/parallel-work.md`: the line stating that several items below touch `internal/loomshed/loomshed.go` stops being true the moment the literal is deleted.
  The items it refers to — the five `loom: real LLM producers` tasks — touch `contracts/recipes/loom-recipe.yaml` instead.
  Restate it accordingly.

  Every markdown edit uses semantic line breaks per the repo convention.
  Every inline link must resolve, file part and `#anchor` alike — the Markdown Link Integrity invariant is machine-enforced over `manifest/` and `docs/`, and this card edits four files under `manifest/`.
- **Commit:** `docs(manifest): record the recipe conversion and correct falsified claims`

### Card 25: `docs/overview.md`

- **Context:**
  - `internal/loomrecipe/doc.go`
  - `contracts/recipes/recipes.go`
  - `internal/loomshed/doc.go`
  - `manifest/designs/shed-recipe.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Four edits.

  Add `internal/loomrecipe` to the module tree listing with a one-line description matching the surrounding entries' style — it assembles loom's `*shedengine.Shed` from the embedded recipe — placed beside `internal/shedrecipe` and `internal/shedbuild`.

  Do **not** add `contracts/recipes` to that tree, and do not open a `contracts/` region in it.
  The tree enumerates `cmd/lyx/` and `internal/*` only, and the sentence closing it says so outright: "`cmd/lyx` is `package main`; everything else is in `internal/`."
  `contracts/stencils` is a production Go package outside `internal/` today and is likewise absent from the tree, documented in prose instead.
  Follow that precedent: document `contracts/recipes` in the Shed-recipe narrative paragraph below, naming the recipe file's path and the embed package, and leave the tree's stated `cmd/lyx` + `internal/*` scope intact.
  Restructuring that tree to admit a `contracts/` region is a larger doc change than this task should make as a side effect;
  if a future task wants it, it takes both `contracts/` packages at once.

  Correct the `internal/loomshed/` line, which describes the package as "loom's own 13-row producer list over `shedengine`" — that list is now the recipe's.

  Correct the loom module line, which spells loom as "`internal/loomcli` + `internal/loomengine` + `internal/loomshed`", to include `internal/loomrecipe`.

  Correct the Shed-recipe narrative paragraph, which says the recipe file format and loader/builder shipped "leaving only the conversion of loom's own list" — the conversion has now shipped too.
  Add `internal/loomrecipe` and `internal/shedrecipe` to that paragraph's list of package documentation to read if the surrounding sentence enumerates them.

  Use semantic line breaks, and keep every inline link resolvable — this file is inside the Markdown Link Integrity walk root.
- **Commit:** `docs(overview): add loomrecipe to the module map, recipes in prose`

### Card 26: The production doc-comment sweep

- **Context:**
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/cancellation_test.go`
  - `internal/loomshed/seed.go`
  - `internal/preflightshed/preflight_integration_test.go`
  - `contracts/specs/loom-status-spec.md`
  - `internal/shedbuild/check.go`
  - `internal/landingshed/publish.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:**
  - `internal/shedcheck/doc.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/preflightshed/preflight_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/loomcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Six comment-only repairs, no behaviour change in any file.
  The sweep covers test-file comments as well as production ones — a stale path in a `_test.go` doc comment is just as wrong, and three of the six sites are test files.

  `internal/shedcheck/doc.go` says "Neither `shedengine.Run` nor `loomshed.New` calls `Check`".
  `loomshed.New` no longer exists;
  the production build path that does not call `Check` is now `loomrecipe.New`.
  Restate, keeping the sentence's point — `Check` is authoring-time only, because a resumed run legitimately starts mid-graph.

  `internal/shedrecipe/recipe.go` says the landing values are "told wholesale by `loomshed.Deps.Landing` today".
  That field is gone;
  they are told through `shedrecipe.Env.Landing` now, filled by whichever caller invokes the registry.

  `internal/shedrecipe/entries_simple.go` names `coverage_guard_test.go` twice — once in `publishEntry`'s doc comment and once in `finalizeEntry`'s — as the test pinning each row's registry key against `landingshed`'s own `publishName`/`finalizeName` constants.
  That pin now lives in `internal/loomrecipe/coverage_guard_test.go`;
  repoint both by full path so the reference is unambiguous from inside `package shedrecipe`.

  `internal/preflightshed/preflight_test.go` names `internal/loomshed/resume_test.go`'s `TestCancellation_RealProducersReturnErrorNotStuck` by file path.
  That test still lives in `internal/loomshed`, but in `cancellation_test.go` now;
  repoint the path.

  `internal/shedbuild/fixture_test.go`'s `testLandingDeps` doc comment names two files by path that no longer exist — "the same shape `coverageGuardLandingDeps` in `internal/shedrecipe/coverage_guard_test.go` and `testLandingDeps` in `internal/loomshed/fixture_test.go` both use".
  Card 12 moved the first to `internal/loomrecipe/coverage_guard_test.go` and deleted `coverageGuardLandingDeps` outright;
  card 5 moved the second to `internal/loomrecipe/fixture_test.go`.
  Repoint the sentence at `internal/loomrecipe/fixture_test.go`'s single surviving copy, and drop the reference to the deleted helper rather than repointing it at a name that no longer exists.

  `internal/loomcli/smoke_test.go`'s file header says "loom's own producer table (`internal/loomshed`) backs five of its thirteen rows with stub producers that report Done unconditionally".
  The five stub rows and their unconditional Done are unchanged, and `loomshed.NewStub` still builds them via `stubEntry` — only the parenthetical is stale, because the table is now the recipe's.
  Repoint that parenthetical at `contracts/recipes/loom-recipe.yaml` and leave the rest of the driver-liveness-timing paragraph alone;
  the bounce-budget behaviour it describes is unaffected by this task.
  Note that this file carries a `//go:build smoke` tag, so its compile check comes from this batch's own `&& go vet -tags smoke ./internal/loomcli/` verify tail — not from the done gate, whose `-tags integration` does not enable `smoke`.

  Then sweep wider than these four, because the enumerated set is a starting point, not a complete one.
  Grep every `.go` file, test files and build-tagged files included, for `loomshed.New`, `loomshed.Deps`, `coverage_guard_test`, `equivalence_test`, `coverageGuardLandingDeps`, `internal/loomshed` in both its trailing-slash and slashless spellings, and the moved test-file basenames `resume_test`, `sequence_test`, `loomshed_test`.
  Add the unqualified in-package spellings `Deps` and `New(` when grepping `internal/loomshed` itself, where a doc comment names the deleted symbols without a package prefix;
  card 20 already repairs the three known sites there, so a hit surviving into this batch means one was missed rather than that a new edit is due.
  The slashless spelling matters: `internal/loomcli/smoke_test.go`'s stale hit is written `(internal/loomshed)`, which a trailing-slash token misses.
  Repair every hit that is now false.
  Ignore hits inside `_mill/` — that is this task's own working directory, not repo documentation.
  Two known hits are legitimately unchanged and must be left alone: `internal/preflightshed/preflight_integration_test.go`'s references to `internal/loomshed`'s *former* copy of a file (historical, already past-tense) and `contracts/specs/loom-status-spec.md`'s reference to `internal/loomshed/seed.go`, which still exists and still writes the seed.
  If the sweep turns up a hit not covered by this card's `Edits:` list, stop and report it rather than editing a file outside the list.
- **Commit:** `docs: repoint comments naming symbols this task moved or deleted`

## Batch Tests

`verify: go test ./internal/lyxcwd/ ./internal/preflightshed/ ./internal/shedrecipe/ ./internal/shedcheck/ ./internal/shedbuild/ ./internal/loomcli/` runs each package this batch edits a file in, plus `internal/lyxcwd`, which owns the two machine-enforced guards these edits can break.

`TestEnforcement_MarkdownLinks` is the Markdown Link Integrity invariant's enforcer and walks every `.md` under `manifest/` and `docs/`, resolving each inline link's file part and, for a `.md` target, its `#anchor` — this batch edits five files inside that walk root plus `CONSTRAINTS.md`, which several of them link into by anchor.
`TestEnforcement_GeometryLiterals` runs in the same package and guards path-construction literals in production Go;
card 24 and card 25 write new prose naming `_lyx`, so running it is cheap insurance that no doc edit strayed into a code change.
Both live in `internal/lyxcwd` as a file-layout convenience, not an ownership claim — `CONSTRAINTS.md` states that caveat for both invariants.
The whole package is run rather than a `-run` filter, so a sibling enforcement test this batch's doc edits happen to break is caught too.

No Go behaviour is asserted here because none changes: card 26's six edits are comment text only.
One of the six needs its own gate, and the `&& go vet -tags smoke ./internal/loomcli/` tail is it: `internal/loomcli/smoke_test.go` carries a `//go:build smoke` tag, so neither `go test ./internal/loomcli/` nor the module-wide `go vet ./...` compiles it — both skip tagged files by default.
Nothing else in this task reaches it either: `pipeline.done_gate` is `go test ./... && go test -tags integration ./...`, and `-tags integration` does not enable `smoke`, so without that tail the edited file would never be compiled by any gate in this plan.
`go vet -tags smoke` rather than `go test -tags smoke` is deliberate: the edit is a single stale parenthetical in a header comment, so typechecking the file is the whole requirement, and actually running the smoke suite at a batch boundary would spawn tmux sessions and git for no gain.
The module-wide `go vet ./...` at the batch boundary is what proves the other five comment edits did not break a file — `go vet` rather than `go build`, since two of the five sites are `_test.go` files a build would never compile.
