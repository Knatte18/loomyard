# Batch: docs

```yaml
task: 'Shed recipe: loader/builder'
batch: docs
number: 4
cards: 5
verify: null
depends-on: [3]
```

## Batch Scope

This batch lands every documentation change the task owes, in the five files the discussion settled: the module doc for the shed-recipe group, the one other design doc carrying a statement this task falsifies, the repo overview's module table and shed narrative, the invariant file's machine-enforced list, and the roadmap item this task completes.
It is one batch because every card is prose in a `.md` file with no runnable surface, and splitting prose across batches would only spread one coherent story over several commits.
It runs last so every claim it makes about the shipped package is already true on disk.
No batch-local decisions differ from the overview's `## Shared Decisions`, beyond the semantic-line-break rule that already applies to all five files.

## Cards

### Card 14: the shed-recipe module doc

- **Context:**
  - `internal/shedbuild/doc.go`
  - `internal/shedbuild/recipe.go`
  - `internal/shedbuild/parse.go`
  - `internal/shedbuild/build.go`
  - `internal/shedbuild/check.go`
  - `internal/shedbuild/testdata/loom-recipe.yaml`
- **Edits:**
  - `manifest/designs/shed-recipe.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update `manifest/designs/shed-recipe.md` in three places.
  First, the H1 title and the blockquote banner directly beneath it: both still say piece 1 alone is shipped and that pieces 2 through 4 are a draft nobody should implement from.
  Three of the four pieces are now shipped, so the title's parenthetical and the banner both become "pieces 1-3 shipped, piece 4 planned", and the banner's do-not-implement warning narrows to piece 4 alone.
  The status line naming the group's sequencing stays, updated to say the group's remaining work is the conversion item.
  Second, the paragraph headed "Not in a recipe row: `Segment`": rewrite it to record the reversal and its reason.
  A recipe row does carry an optional `segment`, mapped straight onto the producer definition's own segment field.
  The old argument — that leaving every row's segment unset is already a no-op in the shed's validation — holds only while every row leaves it unset, and three already-planned roadmap items each specify a producer pair sharing one segment name, so the premise breaks the moment any of them lands.
  Worse, the shed's validation enforces that a non-empty stuck target names a producer sharing the bouncing row's segment, so a list mixing recipe rows at the empty value with hand-wired rows at a real segment name would fail validation at run time rather than at authoring time.
  What survives from the old paragraph is the other half of its claim — that the segment field's cross-segment-wiring-detection job is superseded by the validity checker — and that half must be kept, since it is still true;
  what changes is only the prediction that the field departs.
  Third, add a section documenting piece 2 as shipped, covering: the package name and its one-way import direction;
  the document shape, with a required `version`, a told `entry`, a told `terminals` list, and a `producers` list whose rows carry `name`, `engine`, `config`, `on_done`, `on_stuck`, `segment`, and `max_bounces`;
  the four exported functions and what each is for;
  the strictness posture, where unknown keys and duplicate keys are errors at both document and row level and decoder-produced messages keep their own line numbers;
  the validation split, where this package owns file shape and engine-name resolution and nothing else, because the shed's own validation and the validity checker already own routing, cycles, and reachability;
  the fact that building is not filesystem-free, since four registry constructors reach disk of their own accord and this package is a pass-through for that;
  and the fact that the package defines no on-disk location for recipe files, which stays piece 4's decision.
  Update the "Pieces to build" list's second entry to mark the loader/builder shipped, matching how the first and third entries are already marked.
- **Commit:** `docs(shed-recipe): record piece 2 as shipped and correct the Segment ban`

### Card 15: the two superseded sentences in the shed design doc

- **Context:**
  - `manifest/designs/shed-recipe.md`
- **Edits:**
  - `manifest/designs/shed.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Correct the two sentences in `manifest/designs/shed.md`'s blind-gate discussion that predict the segment field's departure.
  The first calls the blind-gate finding "what replaces the departing `Segment` rule";
  the second says removing the field and its validation rule "belong to the recipe-loader items that actually drop `Segment`".
  This task is the recipe-loader item and it does not drop the field — it makes the field recipe-authorable — so both sentences become false the moment it lands.
  Rewrite them to keep what is still true and drop only the prediction: the blind-gate finding remains the real graph property that supersedes the segment field's cross-segment-wiring-detection job, expressed as a route-back property rather than as a matching label, and that is the substantive half of the claim.
  State plainly that the field and its same-segment validation rule are not going away, and point at the corrected paragraph in `manifest/designs/shed-recipe.md` for the reasoning, replacing the existing pointer at the same spot.
  Change nothing else in the blind-gate section — the finding-kind list above it and the done-cycle paragraph below it are both unaffected.
- **Commit:** `docs(shed): drop the prediction that Segment departs`

### Card 16: the repo overview

- **Context:**
  - `internal/shedbuild/doc.go`
  - `manifest/designs/shed-recipe.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update `docs/overview.md` in two places.
  First, the module tree: add one row for the new package directly beneath the existing row for the engine registry, keeping the tree's own column alignment, described as the recipe file format's loader and builder — the package that decodes a recipe document and assembles the producer-definition list the shed engine already consumes.
  Second, the shed narrative bullet: it currently says the engine registry is the group's one shipped piece and that the recipe file format and the loader/builder are not built yet.
  Replace that second sentence with the shipped state — the file format and the loader/builder are shipped as the new package, leaving only the conversion of loom's own list — and extend the closing "see the package documentation" sentence so it names the new package alongside the three it already names.
  Change nothing else in either location;
  the module table's ordering convention and the narrative's surrounding sentences both stay as they are.
- **Commit:** `docs(overview): add shedbuild to the module table and shed narrative`

### Card 17: the invariant file

- **Context:**
  - `internal/shedbuild/seam_enforcement_test.go`
  - `internal/shedbuild/doc.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Update `CONSTRAINTS.md` in the Told-Geometry Invariant section, as two changes rather than one.
  Append the new package to the machine-enforced bullet's list, in the same trailing group as the four packages already enforced by a test named `TestToldGeometryInvariant_AllowlistOnly`, since the new guard carries that same function name.
  Then update the section's closing "Enforced by" bullet, which currently reads "the ten tests named above": with the new entry the count is eleven, and a list append that leaves the count reading "ten" is exactly the silent drift this file exists to prevent.
  Separately, add one sentence to the Shed Recipe Registry Invariant section recording that the new package is the registry's first outside caller and reaches it only through the two exported accessors, adding no registration mechanism of its own — a recipe naming an unregistered engine is an error, never a reason to register one.
  Add no new invariant section.
  In particular, do not add a sole-parser invariant for the recipe format: after this task there is no on-disk recipe format in production at all, only this package's own test fixtures, so a second parser cannot plausibly appear yet and an invariant nobody can violate teaches the next author nothing.
  That obligation is carried forward by the roadmap instead.
- **Commit:** `docs(constraints): machine-enforce told geometry for shedbuild`

### Card 18: the roadmap

- **Context:**
  - `manifest/designs/shed-recipe.md`
  - `internal/shedbuild/doc.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Move the "Shed recipe: loader/builder" item in `manifest/roadmap.md` out of the Planned group and into Done, since this task completes a planned item.
  Place it in the Done group immediately adjacent to the two already-shipped pieces of the same group — the engine-registry entry and the validity-checker entry, which sit at the top of Done — so all three pieces of one initiative read together;
  the file's own Maintenance section makes the numbering itself cosmetic, so adjacency is the only placement property that matters here.
  Rewrite it in the Done group's own past-tense, what-shipped voice, matching the entries already there: name the package, its four exported functions, the document shape, the strict-decode posture, the validation split against the shed's own validation and the validity checker, the fact that building inherits construction-time filesystem effects from four registry constructors, and the loom-equivalence test as the proof the format expresses loom's real thirteen-row list.
  State that the task shipped no production recipe file — the only recipe documents it added are its own test fixtures — and that it added no exported surface to the engine registry and touched no existing production file.
  Update the group's own intro paragraph, which currently says two pieces remain planned: one remains.
  Extend the remaining "loom: convert to a Shed recipe" item with two carried-forward facts this task established.
  First, that item is where a sole-parser invariant for the recipe format belongs, added to the invariant file in that same commit as a review obligation, by direct analogy with the plan-format sole-parser invariant already recorded there — it is premature until that item ships the first production recipe and the first real consumer.
  Second, the recipe's consumer cannot be loom's own producer-list package: the engine registry already imports that package for six of its constructors, so having it import the loader in turn would close a production import cycle that does not compile.
  The consumer must sit above it — loom's CLI wiring already holds every told path — or the producer-list package must shed the constructors the registry reaches for, and choosing between those two stays that item's call.
  Change nothing else in the file;
  the numbering convention is described in its own Maintenance section and is not touched here.
- **Commit:** `docs(roadmap): move the Shed recipe loader/builder item to Done`

## Batch Tests

`verify: null`, because this batch is documentation only: every card edits a `.md` file, none touches Go source, and there is no runnable surface for a test command to exercise.
The three preceding batches already pin every behavioural claim these documents make, and each ran `go test ./internal/shedbuild/...` at its own boundary.
Correctness here is a review property — that each edited sentence matches what the shipped package actually does — not a test property.
