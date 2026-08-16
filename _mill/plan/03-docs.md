# Batch: docs

```yaml
task: 'PATTERN directives: move from Go constants to stencil files'
batch: docs
number: 3
cards: 4
verify: go build ./... && go test ./...
depends-on: [2]
```

## Batch Scope

This batch brings the four remaining documentation sites into line with what batch 2 made true, and changes no behaviour.
Three of the four are documents asserting something the shipped code now disproves — the module doc's claim that the pointer is baked into a directive constant, the design doc's four false claims, and the sandbox suite's literal stencil count — and the fourth is the roadmap item moving from Planned to Done.
The `CONSTRAINTS.md` amendment is deliberately **not** here: it belongs with the leaf-test change it restates, in batch 2 card 4.
It is one batch because the four edits share nothing but their trigger, and splitting four prose edits across batches would buy nothing.

Batch-local decision: `manifest/designs/pattern-directive-stencils.md` is corrected and kept rather than deleted, per the overview's Shared Decision on that file, which also records the documentation-lifecycle tension the choice carries.

## Cards

### Card 10: Correct the module doc's now-false claim about the directive constant

- **Context:**
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/pattern/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/pattern/doc.go`, the "Why the pointer stays relative" section states the pointer is "a literal relative string baked into the directive constant, never an interpolated absolute path built from a Location field".
  That sentence is false the moment the prose lives in a stencil file, so rewrite it: the pointer is a literal relative string in the stencil file's own body, still never an interpolated absolute path built from a `Location` field.
  The reason it stays relative is unchanged and must survive the rewrite intact — an absolute path would vary per worktree, which would make the fixed directive strings unable to be compared for equality or matched by substring across worktrees the way this package's own tests and any consumer's tests need.

  Neither the file-header comment on the first line nor the package doc's opening paragraph makes the returns-constants claim — that wording belongs to the header of `internal/pattern/pattern.go`, which card 3 already rewrote — so do not go looking for one to strike there.
  What both do need is to account for the read path, which they currently do not mention at all: extend the header comment's list of what this doc covers, and the opening paragraph's one-sentence summary of what the package does, to include it, then add a short subsection or paragraph stating that read path in full: `Directive` is told a `stencilsDir`, reads the role's stencil through `stencilstore.Read`, strips the leading banner with `stencil.StripLeadingComment` because its return value is injected as a producer template's marker value and so never passes through `stencil.Fill`, and returns an error rather than an empty string when an active PATTERN's stencil cannot be read.
  State the lazy-read property too: no read is attempted on a nil layout, an inactive PATTERN, or an unknown role.

  Do not touch the "The active check is pure existence", "Why three roles, not one", or "PathspecFile and PathspecDir" sections beyond what the above requires — the three edge cases and the fail-loud argument in the first of them are still exactly true.
- **Commit:** `docs(pattern): describe the stencil read path in the module doc`

### Card 11: Correct and close out the design doc

- **Context:**
  - `internal/pattern/pattern.go`
  - `internal/websterengine/render.go`
  - `docs/shared-libs/stencil.md`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/pattern-directive-stencils.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/designs/pattern-directive-stencils.md`, flip the `> **Status: Design — not built.**` line to a shipped status, and make four corrections.
  Leaving a shipped design doc asserting four things the code disproves is worse than having no design doc at all, so none of the four may be skipped.

  1. Step 3 specifies the fail-silent posture — a `logger.Warn` and a `""` return — which this task overrode.
  Correct it to the shipped posture: `Directive` returns `(string, error)` and fails loud, wrapping `stencilstore.Read`'s own error, and needs no `internal/logger` import as a result.

  2. Step 4 claims the change is "plumbing-free" because "`websterengine`'s functions already take it as a parameter".
  That is false: `RenderRecoveryPrompt` and `RenderMasterPrompt` in `internal/websterengine/render.go` each derived the directory internally instead, and each embedded the `Directive` call inline in a `values` map literal, so both needed the call hoisted out with its own error check.
  Correct the step to say two of the four call sites were simple assignments and two were map-literal hoists, and that no webster signature changed.

  3. No step mentions the banner strip at all, and without it the relocation is not behaviour-preserving.
  Add it: `stencilstore.Read` is a plain read that strips nothing, `stencilstore.Reconcile` stamps a `lyx-stencil:` line into every seeded file's leading banner, and `Directive` therefore calls `stencil.StripLeadingComment` on what it reads.

  4. The "Related" bullet naming `docs/shared-libs/stencil.md` calls it "the `Fill`/`FillOptional` contract these stencil files render through".
  These three are the first stencils that never pass through `Fill` — they are injected as a values-map string instead — so correct that bullet rather than deleting it.

  Also correct the "Test migration" section where it implies the consumer-template tests compare against the stencil file's content: what the shipped tests compare against is the **stripped body**, never whole-file bytes, because the on-disk file carries a banner and a stamp the return value never does.

  Do not delete this file.
  The repo's documentation lifecycle would normally have a `manifest/designs/` doc removed when its roadmap item ships;
  keeping and correcting it is the decision `_mill/discussion.md` records and the overview's Shared Decision restates, and it is not this card's to reverse.
- **Commit:** `docs(manifest): correct and close out the pattern-directive-stencils design`

### Card 12: Move the roadmap item to Done

- **Context:**
  - `manifest/designs/pattern-directive-stencils.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `manifest/roadmap.md`, move the **PATTERN directives: move from Go constants to stencil files** item out of the Planned section and into the Done section.
  Per the file's own Maintenance note, no renumbering is needed anywhere — every item is written literally as `1.` and each `##` section renders its own sequence — so do not renumber anything, and do not change any other item.
  Per the same note, a Done entry points at the module's own package documentation rather than at its design doc, so the moved entry's pointer becomes `internal/pattern`'s package documentation.
  Keep the entry short: the bold item name plus one or two sentences of what shipped, matching the length of the entries already in Done.
  Drop the "Independent of Shed/loom above, no ordering dependency either way" sentence — it is scheduling information with no meaning once the item has shipped.
- **Commit:** `docs(roadmap): move the PATTERN directive stencils item to Done`

### Card 13: Correct the sandbox suite's stencil count

- **Context:** none
- **Edits:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `tools/sandbox/SANDBOX-CORE-SUITE.md`, the stencil scenario's **Watch** line asks whether `lyx stencil list` names "all fifteen registered stencils".
  There are now eighteen, so change the word `fifteen` to `eighteen`.
  Change nothing else on that line or in that scenario — the board-copy path, the edit-state list, the `lyx stencil validate` question, and the read-only note are all still exactly right.
  Check the rest of the file for any other literal count of registered stencils and correct it the same way if one exists;
  this is a document whose whole purpose is to be read and executed by a human or agent running the suite, so a stale number in it is a live defect, not a cosmetic one.
- **Commit:** `docs(sandbox): update the registered-stencil count to eighteen`

## Batch Tests

`verify:` is the full `go build ./... && go test ./...`, per the overview's Shared Decision on verify scope.
Three of this batch's four files are pure markdown with no runnable surface, so the suite proves nothing about them directly;
it is kept rather than set to `null` because card 10 edits `internal/pattern/doc.go`, which is Go source that must still compile and whose package must still pass its own tests, and because a docs batch that silently skipped verification would be indistinguishable from one that broke the build.

The one guard with real teeth in this batch is `internal/lyxcwd`'s Fabric Vocabulary walk over `internal/**/*.md` and the Go tree, which covers the rewritten `doc.go` prose.
There is no automated check on the design doc, the roadmap entry, or the sandbox count — those three rest on review, which is why each card names the specific false claim it must falsify rather than asking for a general update pass.
