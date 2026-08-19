# Batch: documentation lifecycle

```yaml
task: 'landing: Publish + Finalize producers'
batch: 'documentation lifecycle'
number: 6
cards: 6
verify: go test ./internal/lyxcwd/... ./cmd/lyx/... ./internal/landingshed/... ./internal/mergeresolve/...
depends-on: [5]
```

## Batch Scope

Closing the design document out per the Documentation Lifecycle: `manifest/designs/landing.md`'s own status banner says it deletes when both producers land, and this is where that happens.
Durable content folds into the two new packages' documentation, thirteen inbound references across seven files are repointed, the roadmap item moves and is rewritten to match what actually shipped, the module table gains both packages, and both packages join the Told-Geometry Invariant's machine-enforced list.

It is one batch because the deletion and every repoint must land in one commit or the link-integrity guard breaks, and because the folded content and the links pointing at it are two halves of the same edit — repointing a link at a package doc that does not yet carry the content it promised would be worse than leaving the link broken.
It depends on batch 5 so the whole implementation is in place before its documentation is written against it, and so this batch's edits to the two package documentation files never race the batches that create them.

Batch-local decisions beyond `## Shared Decisions`:

- **The design document is deleted rather than updated.** Keeping it corrected would leave two sources describing the same shipped code, which the lifecycle exists to prevent.
- **Three of its claims are corrected rather than carried across**, because this task overturned them: the conflict-shape detection it describes, the teardown step it names, and its statement that the first producer returns done once a pull request exists.

## Cards

### Card 36: fold durable content into the two package documentation files

- **Context:**
  - `manifest/designs/landing.md`
  - `manifest/designs/raddle.md`
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/publish.go`
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/mergeresolve/markers.go`
- **Edits:**
  - `internal/mergeresolve/doc.go`
  - `internal/landingshed/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fold the design document's durable content into the two packages' own documentation, which is where a shipped module's detail lives from here on.

  Into `internal/mergeresolve/doc.go`: what the engine is, why it is a plain package rather than a producer, the verify-before-conclude discipline and why the abort call is the only checkpoint that covers the attempt window, and the refusal to touch merge state a human left behind.

  Into `internal/landingshed/doc.go`: what each producer does, why the first returns a stuck verdict rather than done while a pull request is open, why the base-branch setting is a list rather than a bool, the squash default and the ancestry cost that was weighed against commit noise, and the merge-critical-section contract.
  That last item is load-bearing beyond its own value: `manifest/designs/raddle.md` carries an **anchored** link into the section of the deleted document that describes it, and card 38 repoints that link here — an anchored link needs a replacement target that genuinely carries the content, so this documentation must cover it before that repoint is truthful.

  Both files describe **one repository** throughout and name neither fabric-internal side, in identifiers, string literals, and comments alike.
  The source document discusses the two-sided split freely, so its content must be reworded rather than transcribed: this is the one place where copying the source text verbatim would fail the build.
- **Commit:** `docs(landing): fold the design document's durable content into the package docs`

### Card 37: delete the design document

- **Context:**
  - `internal/landingshed/doc.go`
  - `internal/mergeresolve/doc.go`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `manifest/designs/landing.md`
- **Moves:** none
- **Requirements:** Delete `manifest/designs/landing.md`.
  Its own status banner already states it deletes when both producers land, which is the same pattern the sibling design documents in that directory follow.
  Card 36 has already folded its durable content into the two package documentation files, and cards 38 through 41 repoint everything that pointed at it; this card is the deletion alone, so a reviewer can see the removal separately from the repointing.

  Do not leave a stub file, a tombstone, or a redirect note behind in its place — the lifecycle deletes the file outright, and every reference to it is repointed in this same commit series.
- **Commit:** `docs(landing): delete the design document per the documentation lifecycle`

### Card 38: repoint the Markdown links in the design documents

- **Context:**
  - `internal/landingshed/doc.go`
  - `internal/mergeresolve/doc.go`
- **Edits:**
  - `manifest/designs/loom.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/raddle.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Repoint every inbound Markdown link to the deleted document.
  The inventory below was enumerated by grep rather than from memory, and is cited by file because line numbers drift; re-run the same grep before starting and again after finishing, to confirm nothing was missed and nothing remains.

  - `manifest/designs/loom.md` — four links: the producer-table rows for the two producers, the paragraph about a future regeneration step folding into the merge, and the build-ordering paragraph. Repoint each at the package documentation that now carries the content it was reaching for.
    While in the producer table, also drop the teardown claim from the second producer's Output cell — it sits in the same cell as one of those links, it names nothing in the codebase, and it is one of the three claims this batch's own scope records as overturned. Card 39 strips it from the roadmap and card 36 folds the corrected description into the package documentation; leaving it here would contradict both.
  - `manifest/designs/shed.md` — **three** links plus **one prose mention**: the links are the worked-example line for the second producer, the task-bundling paragraph, and the Related list entry; the status banner names the file inside a code span rather than link syntax, so it is prose the guard never enforces. Repoint the three and reword the prose mention in place — do not try to force the banner's code span into link syntax it never had.
  - `manifest/designs/raddle.md` — one **anchored** link. Its anchor pointed into a section of the deleted document; repoint it at the package documentation card 36 gave that content to. A `.go` target carries no anchor, so the anchor fragment is dropped rather than translated.
  - `manifest/designs/fabric-unified-view.md` — one link, which describes a document-driven conflict mechanism this task explicitly does not build. That reference needs **rewording**, not merely repointing: state that only the ordinary conflict shape shipped and that the other remains an unscheduled item, then point at the surviving target.

  Every one of these must land in the same commit as the deletion, or the link-integrity guard breaks.
  Do not add an allowlist entry for any of them — the allowlist is for known-broken links a task deliberately leaves behind, and every link here is being fixed rather than deferred.
- **Commit:** `docs: repoint the design-document links at the shipped package docs`

### Card 39: roadmap

- **Context:**
  - `internal/landingshed/doc.go`
  - `internal/mergeresolve/doc.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Move this task's Planned item to the Done section and rewrite its body to match what actually shipped.

  The current body is wrong in three specific ways this task overturned, and each must go rather than being softened: it describes conflict-shape detection, which is not built; it names a teardown step, which names nothing in the codebase and was loose wording; and it says the first producer returns done once a pull request exists, which is exactly the behaviour that would defeat the pull request and which the shipped producer replaces with a stuck verdict.

  The rewritten Done entry stays short — a name plus one or two sentences of what and why, per this file's own maintenance rules — and its trailing pointer names the two packages' own documentation rather than the deleted design document, since a Done entry points at the module's package documentation from then on.
  Numbering needs no edit anywhere: every item is written literally as `1.` and each heading starts a fresh list block.

  While in this file, also correct the Done entry for the earlier phase-machine scaffolding item.
  It states that the producer list carries twelve rows and enumerates seven stubbed rows by name — both already stale before this task, since a thirteenth row was added later, and both made further wrong by this task, which turns two of the rows it describes into real producers.
  Correct the row count and the stubbed-row list so the entry describes what that package now holds.
  This is a correction to an entry made wrong in part by this task's own change, in a file this card already edits, not an unrelated cleanup — and leaving it would state a row count that contradicts the module table this plan corrects one card later.

  Leave the Someday item about the other conflict shape exactly as it is.
  It already reads that only the ordinary shape shipped and the document shape is not built, which this task confirms rather than changes — an edit there would be churn.
- **Commit:** `docs(roadmap): move the landing item to Done and correct its body`

### Card 40: module table

- **Context:**
  - `internal/landingshed/doc.go`
  - `internal/mergeresolve/doc.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add both new packages to the module table in `docs/overview.md`, each with a one-line description in the same terse style as the surrounding rows and placed to keep the table's existing ordering.

  While in that table, correct the loom producer-list row's stated row count: it currently says twelve, and the list has had thirteen rows since the stub row for the first of these two producers was added.
  This is a one-word correction to a line this card is already editing and directly about the rows this task made real, not an unrelated cleanup.

  Do not restate either package's design here — the table is a one-line-per-module index, and the detail lives in each package's own documentation.
- **Commit:** `docs(overview): add the two landing packages to the module table`

### Card 41: invariant list and the stale worked example

- **Context:**
  - `internal/mergeresolve/seam_enforcement_test.go`
  - `internal/landingshed/seam_enforcement_test.go`
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two edits to `CONSTRAINTS.md`, both required by this task's own changes.

  First, under the Told-Geometry Invariant, add both new packages to the **machine-enforced** list, each naming the test that polices it, in the same form the existing entries use.
  They belong on that list rather than the review-obligation one because each ships its own import-policing test in this task, which is precisely what the invariant's own membership predicate calls machine-enforced.

  Second, the Markdown Link Integrity section cites the now-deleted design document as a live worked example of the anchor-resolution rule — it names that document's own outgoing anchored link into this very file.
  Deleting the file makes the example stale.
  It is prose rather than a link, so no test catches it: rewrite the bullet against a surviving example that genuinely demonstrates the same rule, keeping the rule's wording itself unchanged.

  Leave the gitrepo Client Boundary Invariant's method list exactly as batch 1 left it, and leave the gitexec Checked-Call Invariant's pinned raw-site counts untouched — this task adds no raw call site.
  Verify while here that the prose reference inside the loom producer-list source file was already repointed by batch 5; if it still names the deleted document, repoint it now.
- **Commit:** `docs(constraints): list both landing packages as machine-enforced and fix the stale example`

## Batch Tests

`verify:` runs four packages' fast tiers, all untagged — this batch changes documentation and comments only, so there is no new runnable behaviour, but three machine checks fire on exactly these edits and each is the real gate here.

- `./internal/lyxcwd/...` runs the two enforcement walks that matter most: the Markdown link integrity check, which is what proves the deletion and all eleven repoints landed together rather than leaving a broken link, and the Fabric Vocabulary walk, which covers both new packages' documentation and would fail on a forbidden token copied across from the deleted document — the single most likely mistake in card 36.
- `./cmd/lyx/...` re-runs the guard family, so a documentation edit that accidentally disturbed a pinned table is caught here rather than at the task-wide gate.
- `./internal/landingshed/...` and `./internal/mergeresolve/...` compile and re-run both packages' tiers, since card 36 edits Go source files in each and a malformed comment block breaks the build.

No `-tags integration` run is needed: this batch creates and edits no tagged test file, and the task-wide gate covers the tagged tiers once more before the task is marked done.
