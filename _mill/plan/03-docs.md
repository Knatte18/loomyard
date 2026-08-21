# Batch: docs

```yaml
task: "Shed-setup validity checker"
batch: "docs"
number: 3
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: [1, 2]
```

## Batch Scope

This batch lands the four repo-doc updates the Documentation Lifecycle requires for a task that adds a module: the design section in `manifest/designs/shed.md`, the module-tree row and shed-section mention in `docs/overview.md`, the repointed piece-3 bullet in `manifest/designs/shed-recipe.md`, and the roadmap item's move from Planned to Done.
It is one batch because all four are prose edits against files no other batch touches, and because each of them describes a module that must already exist for its claims and links to be true — hence the dependency on both batches 1 and 2.

It depends on batch 2 as well as batch 1 because the `docs/overview.md` and `manifest/roadmap.md` entries state that the checker's enforcement point is a shipped `go test` invariant in `internal/loomshed`;
writing that before batch 2 lands would be documenting an intention rather than a fact.

Batch-local decision beyond `## Shared Decisions` in the overview: no new `manifest/designs/shed-check.md` is created, and no doc under `manifest/designs/` is deleted — `shed-recipe.md` stays, since its other three pieces are still unbuilt.

## Cards

### Card 5: the design section in `manifest/designs/shed.md`

- **Context:**
  - `internal/shedcheck/doc.go`
  - `internal/shedcheck/check.go`
  - `internal/shedengine/doc.go`
  - `internal/shedengine/validate.go`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Insert one new top-level section into `manifest/designs/shed.md`, titled exactly `## Checking an assembled producer list`, positioned immediately before the existing `## Testable cheaply — a throwaway producer list proves the skeleton` section.

  The section states:

  - The gap it closes, quoting `internal/shedengine/doc.go`'s own sentence verbatim: an omitted `OnDone` is indistinguishable from an intended terminal one and ends the run quietly, so a caller assembling a producer list is responsible for asserting its own routing table exhaustively rather than relying on Shed to catch a missing entry.
    Until now no caller did.
  - Where the checker lives and why it is not in `shedengine`: `internal/shedcheck` is an authoring-time analysis, not part of the engine's runtime contract, and putting it in the engine would imply `Run` enforces it.
    The import direction `shedcheck` → `shedengine` is the safe one, and the reverse is already forbidden by the Shed Producer-Seam Invariant.
  - That both endpoints are told and never inferred, with the reason: `Shed` has no entry field and no terminal field, and defaulting to `Producers[0]` would re-introduce the positional routing meaning this same doc's routing model disclaims.
  - The eight kinds — `bad-entry`, `no-terminals`, `bad-terminal`, `dangling-target`, `unreachable`, `unexpected-terminal`, `done-cycle`, `blind-gate` — one line each, in that order.
  - That `blind-gate` is what replaces the departing `Segment` rule, expressed as a real graph property (a gate whose bounce target never routes back to the gate) rather than as a matching label — and that this task removes neither the `Segment` field nor `validate()`'s same-`Segment` rule, which belong to the recipe-loader items that actually drop `Segment`.
  - That `done-cycle` generalises the length-1 case `internal/shedengine/validate.go` already rejects, and why the asymmetry that motivates it is real: a `Done` route consumes no bounce budget, so a done cycle is a statically certain infinite loop, whereas a stuck bounce is budgeted and therefore bounded.
  - That nothing in production calls it — the enforcement point is a `go test` invariant over loom's own list.
  - The one perch mis-wiring it cannot catch (a `Burler` handing back via `OnDone` instead of `OnStuck`), stated as a limit rather than buried, since an over-claimed guarantee is worse than a narrow one.

  Add one bullet to the existing `## Related` section at the end of `manifest/designs/shed.md`, pointing at the module's own package documentation the way that section's existing `internal/landingshed` bullet does.

  Follow the repo's markdown rule from `CLAUDE.md`: one sentence per line, with an extra break at an internal independent-clause boundary in a long sentence, using plain newlines only — never a fixed-column hard wrap, never trailing double-spaces or a backslash.
  Every inline link must resolve, file part and `#anchor` alike;
  the `TestEnforcement_MarkdownLinks` guard this batch's `verify:` runs is what checks that.
- **Commit:** `docs(shed): document the assembled-producer-list checker`

### Card 6: the module row and shed-section mention in `docs/overview.md`

- **Context:**
  - `internal/shedcheck/doc.go`
  - `manifest/designs/shed.md`
  - `CLAUDE.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two edits to `docs/overview.md`.

  First, add one row to the module tree, immediately after the existing `internal/shedadapters/` row, matching that block's existing `├── ` prefix and column alignment, describing `internal/shedcheck/` as the authoring-time structural checker for an assembled `OnDone`/`OnStuck` producer graph.

  Second, extend the existing **shed** bullet in the module list.
  Add a sentence naming `internal/shedcheck` as the shipped structural checker over an assembled producer list, stating that it is enforced by a `go test` invariant over loom's own list rather than called from any production constructor, and extend that bullet's existing "See the `internal/shedengine` and `internal/shedadapters` package documentation" sentence to name `internal/shedcheck` alongside them.
  Do not restate the eight kinds here — point at the new `manifest/designs/shed.md` section instead, which is where that detail lives.

  Follow the repo's markdown rule from `CLAUDE.md`: one sentence per line, no fixed-column hard wrap.
  Every inline link must resolve, file part and `#anchor` alike.
- **Commit:** `docs(overview): add internal/shedcheck to the module tree and shed section`

### Card 7: repoint `shed-recipe.md` piece 3 and move the roadmap item to Done

- **Context:**
  - `internal/shedcheck/doc.go`
  - `manifest/designs/shed.md`
  - `internal/loomshed/loomshed_test.go`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/shed-recipe.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two files.

  In `manifest/designs/shed-recipe.md`, rewrite item 3 of the `## Pieces to build` list so it points at the shipped module instead of describing an unbuilt one: name `internal/shedcheck`, say it is built and independent of the recipe work, and point at the new `## Checking an assembled producer list` section of `manifest/designs/shed.md` for the design.
  Leave items 1, 2, and 4 unchanged, and leave the file's DRAFT heading unchanged — the other three pieces are still unbuilt, so the doc stays.
  Keep the surrounding sentence's claim honest: the list is still described as four separable pieces, one of which has now landed.

  In `manifest/roadmap.md`, move the **Shed-setup validity checker** item out of the Planned `### Shed recipe: declarative producer lists` group and into `## Done`, inserted as the first item of that section.
  Per that file's own Maintenance rules, write it literally as `1.` and renumber nothing anywhere.
  The Done entry stays short — a bold item name plus one or two sentences of what and why — and points at the module's own package documentation and at the new `manifest/designs/shed.md` section, not at `designs/shed-recipe.md`.
  Say in it that the enforcement point is a `go test` invariant over loom's own producer list rather than a call from any production constructor.
  Delete the moved item's two source lines from the Planned group, including its `See [designs/shed-recipe.md](designs/shed-recipe.md).` continuation line, and leave the group's remaining three items untouched.

  Do not delete `manifest/designs/shed-recipe.md`, and do not change the wording of any other roadmap item.

  Follow the repo's markdown rule from `CLAUDE.md`: one sentence per line, no fixed-column hard wrap.
  Every inline link must resolve, file part and `#anchor` alike.
- **Commit:** `docs(roadmap): move the shed-setup validity checker to Done and repoint shed-recipe piece 3`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` is the scoped runner for this batch.
Every card here edits a `.md` file under `manifest/` or `docs/`, and the Markdown Link Integrity Invariant's enforcing test — `TestEnforcement_MarkdownLinks` in `internal/lyxcwd/docslink_test.go` — is the only machine check any of these edits can break.
It resolves both the file part and the `#anchor` of every inline link in those two roots, which is what catches a mistyped `manifest/designs/shed.md` anchor in the new `docs/overview.md` and `manifest/designs/shed-recipe.md` cross-references, and a broken link in the new `shed.md` section itself.
The package is run whole rather than filtered to that one test because it is small and offline, and because `internal/lyxcwd` carries the repo's other enforcement walks in the same package.

No Go source changes in this batch, so nothing else can regress here;
the overview's module-wide `verify: go vet ./...` runs at the batch boundary as the backstop, and `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) covers the whole tree before the task is marked done.
