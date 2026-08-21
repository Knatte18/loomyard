# Batch: docs-and-comment-sweep

```yaml
task: 'loom: convert to a Shed recipe'
batch: 'docs-and-comment-sweep'
number: 6
cards: 4
verify: go test ./internal/lyxcwd/ -run 'TestEnforcement_MarkdownLinks|TestEnforcement_GeometryLiterals'
depends-on: [5]
```

## Batch Scope

The task's documentation obligation, landed as the final batch of the same task branch so the whole change squash-merges as one commit — which is what the project's Documentation Lifecycle rule requires.
Six markdown files carry statements this task falsifies or records its completion;
five production Go doc comments name symbols this task deleted or moved.

No production behaviour changes here.
The only Go edits are comment-text repairs in packages this task consumes rather than revises, which the `no-production-change-to-the-three-consumed-packages` Shared Decision explicitly carves out: a comment pointing at a deleted symbol is not "unchanged", it is wrong.

Batch-local decision: no new invariant is added to `CONSTRAINTS.md`.
The two edits it takes are pointer corrections to existing invariants.
If the implementer concludes a genuinely new cross-cutting invariant is warranted (e.g. "loom's producer list is defined only in the recipe"), it is added in this batch with a named enforcing test — but the default is that none is needed, because the enforcement already exists as `internal/loomrecipe`'s coverage guard under the Shed Recipe Registry Invariant.

## Cards

### Card 23: The two `CONSTRAINTS.md` edits

- **Context:**
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/seam_enforcement_test.go`
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
  - `internal/loomshed/seam_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Make exactly two edits.

  First, the **Shed Recipe Registry Invariant**'s "Enforced by" line, which names `internal/shedrecipe/coverage_guard_test.go` (`TestCoverageGuard_EveryLoomRowHasAnEngine`) for the registry-coverage half.
  That file no longer exists — its loom-driving half moved to `internal/loomrecipe/coverage_guard_test.go` and its exact-twelve-names pin moved to `internal/shedrecipe/registry_test.go`.
  Repoint the line to name both homes rather than one, and say which half each carries.
  Leave the told-geometry half's pointer at `internal/shedrecipe/seam_enforcement_test.go` unchanged.

  Second, the **Told-Geometry Invariant**'s **Machine-enforced** bullet, which enumerates every package whose `seam_enforcement_test.go` runs `TestToldGeometryInvariant_AllowlistOnly`.
  Add `internal/loomrecipe` to that enumeration, alongside `internal/loomshed`, `internal/landingshed`, `internal/mergeresolve`, `internal/shedrecipe`, and `internal/shedbuild`.
  Then update the invariant's own "Enforced by the eleven tests named above" line to twelve — adding the test without the count is exactly the stale enumeration this edit exists to prevent.
  Count the enumerated tests rather than trusting the arithmetic here.

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
- **Edits:**
  - `manifest/designs/shed-recipe.md`
  - `manifest/designs/loom.md`
  - `manifest/roadmap.md`
  - `manifest/parallel-work.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/shed-recipe.md`: drop the blockquote banner saying piece 4 "remains an early concept sketch, not a settled design" and "do not implement piece 4 from this doc as written", and retitle the H1, which currently reads "(pieces 1-3 shipped; piece 4 planned)".
  Mark piece 4 shipped in the "Pieces to build" list alongside the other three.
  Record the three decisions the doc explicitly deferred and this task settled: the on-disk location (an embedded shipped default at `contracts/recipes/loom-recipe.yaml`, read through `shedbuild.Parse`, with no seeding, no operator override, and no runtime on-disk path), the consumer (`internal/loomrecipe`, sitting above `internal/loomshed` because the registry already imports `loomshed` and a back-import would close a compile-time cycle), and test ownership (the assembled-graph tests live in `internal/loomrecipe`).
  Note the accepted consequence that `shedbuild.Load` now has no production caller, and that this is deliberate — it stays exported and covered because it is the entry a future non-embedded consumer needs.
  Update the "Status" line, which says the group's remaining work is the conversion item.

  In `manifest/designs/loom.md`: state that loom's producer list is recipe-backed, pointing at `contracts/recipes/loom-recipe.yaml` and `internal/loomrecipe`.
  The natural sites are the sentence defining `loom` as "`Shed` + `loom`'s own ordered producer list" and the producer-table section that follows it.
  Keep the table itself unchanged — no row's name, engine, or routing changes in this task.

  In `manifest/roadmap.md`: move the **loom: convert to a Shed recipe** item from Planned to Done with a description of what shipped, and close out the "Shed recipe" group's remaining-work framing in the section intro, which currently says three pieces have shipped "leaving one here".
  Then correct the present-tense claims inside already-Done entries this task falsifies — at minimum the "Shed recipe: engine registry" entry's claim that a coverage guard at `internal/shedrecipe/coverage_guard_test.go` "pins the registry against `loomshed.New`'s current, real row list, both directions", its claim that "`loomshed.New` keeps its own Go literal producer list and `loomshed.Deps.Preflight`'s pre-injected field unchanged; nothing downstream of this piece consumes it yet", and the "Shed recipe: loader/builder" entry's description of the shedbuild loom-equivalence test as a shipped artefact.
  Treat those three as a starting set, not a closed list: sweep the whole file for `loomshed.New`, `loomshed.Deps`, `coverage_guard_test`, and `equivalence_test` and correct every hit.
  Restate each in the past tense and point at this item rather than rewriting the entries — both are written as claims about the tree's current state, not as a record of what a past task did, so leaving them would make the roadmap assert something false about `main`.

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

  Add `internal/loomrecipe` and `contracts/recipes` to the module tree listing, each with a one-line description matching the surrounding entries' style — `internal/loomrecipe` assembles loom's `*shedengine.Shed` from the embedded recipe;
  `contracts/recipes` holds the shipped-default recipe files and their `//go:embed` package.
  Place `internal/loomrecipe` beside `internal/shedrecipe`/`internal/shedbuild`, and `contracts/recipes` wherever the listing already covers `contracts/`.

  Correct the `internal/loomshed/` line, which describes the package as "loom's own 13-row producer list over `shedengine`" — that list is now the recipe's.

  Correct the loom module line, which spells loom as "`internal/loomcli` + `internal/loomengine` + `internal/loomshed`", to include `internal/loomrecipe`.

  Correct the Shed-recipe narrative paragraph, which says the recipe file format and loader/builder shipped "leaving only the conversion of loom's own list" — the conversion has now shipped too.
  Add `internal/loomrecipe` and `internal/shedrecipe` to that paragraph's list of package documentation to read if the surrounding sentence enumerates them.

  Use semantic line breaks, and keep every inline link resolvable — this file is inside the Markdown Link Integrity walk root.
- **Commit:** `docs(overview): add loomrecipe and contracts/recipes to the module map`

### Card 26: The production doc-comment sweep

- **Context:**
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/coverage_guard_test.go`
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
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Four comment-only repairs, no behaviour change in any file.

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

  Then sweep wider than these four, because the enumerated set is a starting point, not a complete one.
  Grep production files for `loomshed.New`, `loomshed.Deps`, `coverage_guard_test`, `equivalence_test`, bare `internal/loomshed/` path mentions, and the moved test-file basenames `resume_test`, `sequence_test`, `loomshed_test`.
  Repair every hit that is now false.
  Ignore hits inside `_mill/` — that is this task's own working directory, not repo documentation.
  Two known hits are legitimately unchanged and must be left alone: `internal/preflightshed/preflight_integration_test.go`'s references to `internal/loomshed`'s *former* copy of a file (historical, already past-tense) and `contracts/specs/loom-status-spec.md`'s reference to `internal/loomshed/seed.go`, which still exists and still writes the seed.
  If the sweep turns up a hit not covered by this card's `Edits:` list, stop and report it rather than editing a file outside the list.
- **Commit:** `docs: repoint comments naming symbols this task moved or deleted`

## Batch Tests

`verify: go test ./internal/lyxcwd/ -run 'TestEnforcement_MarkdownLinks|TestEnforcement_GeometryLiterals'` runs the two machine-enforced markdown/geometry guards this batch's edits can break.
`TestEnforcement_MarkdownLinks` is the Markdown Link Integrity invariant's enforcer and walks every `.md` under `manifest/` and `docs/`, resolving each inline link's file part and, for a `.md` target, its `#anchor` — this batch edits five files inside that walk root plus `CONSTRAINTS.md`, which several of them link into by anchor.
`TestEnforcement_GeometryLiterals` is included because card 24 and card 25 write new prose naming `_lyx`, and its scope is path-construction literals in production Go rather than markdown;
running it here is cheap insurance that no comment edit strayed into a code change.

The two `-run` names are combined in one regex rather than two invocations so a single command covers both.
Both live in `internal/lyxcwd` as a file-layout convenience, not an ownership claim — `CONSTRAINTS.md` states that caveat for both invariants.

No Go behaviour is asserted here because none changes: card 26's four edits are comment text only.
The module-wide `go build ./...` at the batch boundary is what proves those comment edits did not accidentally break a file, and the `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) is the whole-repo gate that runs before the task is marked done, covering the full suite plus the integration-tagged `internal/loomcli/smoke_test.go`.
