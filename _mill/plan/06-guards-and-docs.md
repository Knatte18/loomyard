# Batch: guards and docs

```yaml
task: 'Shed recipe: engine registry'
batch: 'guards and docs'
number: 6
cards: 6
verify: go test ./internal/shedrecipe/... ./internal/lyxcwd/...
depends-on: [5]
```

## Batch Scope

This batch closes the task: the two guard tests that pin the registry's reason to exist, and the four documentation files `CLAUDE.md` requires to move in the same commit as the code.
It is one batch because both guards need the complete twelve-entry table, and because the new `CONSTRAINTS.md` invariant and the `seam_enforcement_test.go` that machine-enforces half of it are two halves of one statement — writing either without the other ships a false claim.
The coverage guard compares its own row-name-to-engine-name table against the row names in `loomshed.New`'s assembled list, failing in both directions, which is what makes it catch a row added to `loomshed` before piece 4 lands.
Nothing after this batch consumes an interface from it.

Batch-local decisions beyond `## Shared Decisions`:

- The coverage guard builds a real `loomshed.Deps` and calls `loomshed.New`, rather than iterating its own table alone.
  A table-only test would pass forever no matter what `loomshed` grew, which is precisely the failure the guard exists to prevent.
- Documentation edits follow this repo's markdown convention: one sentence per line, an internal break at an independent-clause boundary, plain newlines only, and no fixed-column hard wrap.

## Cards

### Card 19: The coverage guard

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/landingshed/deps.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
  - `internal/mergeresolve/deps.go`
  - `internal/fabricengine/doc.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/coverage_guard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write `TestCoverageGuard_EveryLoomRowHasAnEngine` in the `shedrecipe` package.

  Declare a package-level `var loomRowEngines = map[string]string{...}` mapping each of `loomshed.New`'s thirteen row names to the engine name backing it:
  `"Preflight"` -> `"Preflight"`;
  `"Loom-Preflight"` -> `"LoomPreflight"`;
  `"Discussion-Write"` -> `"Stub"`;
  `"Discussion-Validate"` -> `"DiscussionValidate"`;
  `"Discussion-Review"` -> `"Stub"`;
  `"Plan-Write"` -> `"Stub"`;
  `"Plan-Validate"` -> `"PlanValidate"`;
  `"Plan-Review"` -> `"Stub"`;
  `"Batchifier"` -> `"Batchifier"`;
  `"Webster"` -> `"Webster"`;
  `"Webster-Review"` -> `"Stub"`;
  `"Publish"` -> `"Publish"`;
  `"Finalize"` -> `"Finalize"`.
  Its comment states that the engine side genuinely has to be written down by hand because `shedengine.ProducerDef` carries no engine name, and that only the row-name side is derivable from `New`'s output.

  Build a `loomshed.Deps` good enough for `loomshed.New` to succeed — it never runs, only its assembled list is read.
  `Deps.Preflight` is a minimal fake implementing `shedengine.ShedProducer`, and `Deps.Landing` is a `landingshed.Deps` filled the way `internal/loomshed/fixture_test.go`'s `testLandingDeps` fills it: told absolute paths from `t.TempDir()`, non-nil `PushBranch`, and `OpenFabric`/`OpenParentFabric` closures returning a typed-nil `*fabricengine.Fabric` with a nil error, plus a minimal `mergeresolve.Shuttle` fake.
  Read `internal/loomshed/fixture_test.go` for the exact shape rather than re-deriving it, and state in a comment that these fakes exist only so `New` constructs, since the test reads `Shed.Producers[i].Name` and nothing else.

  Assert three things, each with its own failure message:
  every row name in `New`'s assembled `Producers` slice has an entry in `loomRowEngines` — the direction that catches a row added to `loomshed` between now and piece 4;
  every key in `loomRowEngines` names a row `New` actually has — the direction that keeps the table from accumulating dead entries;
  and every engine name the table maps to resolves through `Lookup` without error.

  Add a second test, `TestRegistry_ShipsTwelveEntries`, asserting `Names()` returns exactly the sorted twelve: `"Batchifier"`, `"Bouncer"`, `"BurlerRound"`, `"DiscussionValidate"`, `"Finalize"`, `"LoomPreflight"`, `"PlanValidate"`, `"Preflight"`, `"Publish"`, `"SingleLLM"`, `"Stub"`, `"Webster"`.

  Add a third test, `TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity`, asserting the `Publish` and `Finalize` rows in `New`'s list are named exactly `"Publish"` and `"Finalize"`.
  Its comment states why this pairing is pinned: both underlying constructors discard the `name` argument because their identity is a package constant carried by their log lines, error text, and stuck-reason filename, so a renamed row would produce a producer whose on-disk identity disagrees with its row name.
- **Commit:** `test(shedrecipe): add the coverage guard against loomshed's current row list`

### Card 20: The import allowlist guard

- **Context:**
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/shedengine/seam_enforcement_test.go`
  - `internal/shedrecipe/doc.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/shedrecipe/entries_singlellm.go`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
- **Edits:** none
- **Creates:**
  - `internal/shedrecipe/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write `TestToldGeometryInvariant_AllowlistOnly` in the `shedrecipe` package, modelled directly on `internal/loomshed/seam_enforcement_test.go`: `runtime.Caller(0)` to find the package directory, `filepath.WalkDir` over it, skipping `_test.go` files and non-`.go` files, `go/parser.ParseFile` in `parser.ImportsOnly` mode, and a failure for any import that is neither stdlib nor a member of the allowlist map.

  Declare `shedrecipeAllowedImports` holding exactly the internal packages production code in this package imports:
  `internal/shedengine`, `internal/shedadapters`, `internal/loomshed`, `internal/landingshed`, `internal/preflightshed`, `internal/websterengine`, `internal/burlerengine`, `internal/shuttleengine`, `internal/stencilstore`, and `internal/stencil`.
  Before writing the map, grep the package's production files for their actual import set and make the allowlist match it exactly — an allowlist entry no production file uses is dead, and a missing one fails the build's own test.

  The file's own header comment states three things: that the allowlist is deliberately a membership list rather than a bare `internal/lyxcwd` denylist, mirroring `internal/loomshed`'s reasoning;
  that this is the largest allowlist in the repo and that this is expected, because this package is the wiring layer that has to reach types from four producer-hosting packages at once;
  and that several allowlisted packages themselves import `internal/lyxcwd`, which is legal because the Told-Geometry Invariant's membership predicate is about a **direct** production import and transitive is explicitly fine.
  Add an explicit assertion that no production import path equals `github.com/Knatte18/loomyard/internal/lyxcwd`, with its own failure message naming the Shed Recipe Registry Invariant, so the specific excluded import is named in the test rather than only implied by the allowlist.
- **Commit:** `test(shedrecipe): add the production-import allowlist guard`

### Card 21: Record the Shed Recipe Registry Invariant

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
  - `internal/shedrecipe/coverage_guard_test.go`
  - `internal/shedrecipe/doc.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new `## Shed Recipe Registry Invariant` section immediately after the existing `## Shed Producer-Seam Invariant` section, matching this file's stated style — rules only, no rationale, no incident narrative.

  The section states two rules.
  First: every value in `internal/shedrecipe`'s registry constructs a `shedengine.ShedProducer` and nothing else — never an arbitrary Go module — and the registry is one central `map[string]Constructor` literal reached only through `Lookup` and `Names`, never by `init()` self-registration and never by a runtime `Register` call.
  Second: `internal/shedrecipe` takes every absolute path from its caller and has no direct production import of `internal/lyxcwd`, in the precise form the Told-Geometry Invariant requires — **every root is told and none is derived;
  the package's only path construction is joining a told root with a recipe-relative value.**
  Use that wording verbatim.
  A flatter claim that no path is computed inside `shedrecipe` would ship false, since `run_subdir`, `artifact_paths`, and `output_files` are all joined.

  Close the section with an **Enforced by** line naming `internal/shedrecipe/seam_enforcement_test.go` (`TestToldGeometryInvariant_AllowlistOnly`) for the told-geometry half and `internal/shedrecipe/coverage_guard_test.go` (`TestCoverageGuard_EveryLoomRowHasAnEngine`) for the registry-coverage half, and stating that the `ShedProducer`-only restriction itself is a review obligation, since the `Constructor` signature already makes it a compile-time fact.

  In the existing `## Told-Geometry Invariant` section, add `internal/shedrecipe` to the **Machine-enforced** bullet's list, in the same `seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly` group that already names `internal/loomshed`, `internal/landingshed`, and `internal/mergeresolve`.
  That bullet's trailing **Enforced by** bullet in the same section says "the seven tests named above".
  Do not simply increment that number: the count is already wrong before this task touches it, since the Machine-enforced bullet currently enumerates nine packages, each carrying its own test file.
  Recount the enumeration and write the correct total, which is ten once `internal/shedrecipe` joins it.
  Change nothing else in that section.
- **Commit:** `docs(constraints): record the Shed Recipe Registry Invariant`

### Card 22: Update the module table in docs/overview.md

- **Context:**
  - `internal/shedrecipe/doc.go`
  - `internal/shedrecipe/registry.go`
  - `manifest/designs/shed-recipe.md`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add one line to the repository tree listing, immediately after the `internal/loomshed/` line, in the same aligned two-column shape the surrounding lines use:
  `internal/shedrecipe/` described as the engine registry — the name to `ShedProducer`-constructor mapping a recipe loader resolves each row's `Engine` against.

  In the prose `shed` bullet further down the file — the one already naming `internal/shedengine` and `internal/shedadapters` and listing what is implemented — add a sentence stating that the engine registry (piece 1 of the Shed recipe group) is implemented as `internal/shedrecipe`, that it registers twelve engine names, and that the recipe file format, the loader/builder, and the validity checker are not built yet.
  Do not claim the recipe mechanism as a whole is implemented — only piece 1 is.

  Any markdown link this card adds must resolve, including its `#anchor` if it targets a `.md`;
  `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` scans this file and will fail otherwise.
- **Commit:** `docs(overview): add internal/shedrecipe to the module table`

### Card 23: Narrow the shed-recipe design doc's DRAFT banner

- **Context:**
  - `internal/shedrecipe/doc.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/registry.go`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/shed-recipe.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite the file's title line and the DRAFT blockquote so neither forbids this task any more.
  The banner must no longer say "do not implement from this doc yet" and must no longer mark the whole concept unsettled.
  It instead records that piece 1, the engine registry, is built and shipped as `internal/shedrecipe`, and that pieces 2-4 — the recipe file format, the loader/builder, and the validity checker — are still unsettled and should not be implemented from this doc as written.
  Do not delete the banner: the recipe file format genuinely is still open, and this task deliberately does not settle it.
  Update the title's own parenthetical accordingly rather than leaving it reading DRAFT for the whole document.

  In the `## Pieces to build` list, mark piece 1 as built and name `internal/shedrecipe` as where it lives.

  In the `## What's never in a recipe` section, add a second paragraph covering the gap this doc currently leaves open: alongside geometry, the **live seams** a producer needs — `shedadapters.Shuttle`, `shedadapters.BurlerRunner`, `shedadapters.WebsterRunner`, `websterengine.RunDeps`, `landingshed.Deps`'s closures and its `modelspec.Registry`, and the injected clock — are never in a recipe either, because none of them can be written in a file at all.
  State that they travel in the same told bundle as geometry, `shedrecipe.Env`, filled once by whichever caller invokes the registry, and that this extends the existing discipline rather than inventing a second one.
  State the rule that makes the `Env`-versus-`Config` split decidable: `Env` holds roots and run-wide values only, never a value that differs between two rows, and anything per-row is a relative path or scalar in `Config` resolved against one of those roots by the entry.

  Every markdown link this card adds must resolve, file part and `#anchor` alike — this file is under `manifest/` and is scanned by `TestEnforcement_MarkdownLinks`.
- **Commit:** `docs(shed-recipe): narrow the DRAFT banner and record the Env decision`

### Card 24: Move the roadmap item to Done

- **Context:**
  - `manifest/designs/shed-recipe.md`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/doc.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Remove the **Shed recipe: engine registry** item from the `### Shed recipe: declarative producer lists` group under `## Planned`, leaving the group's remaining three items and their ordering untouched.
  The group's own intro paragraph says "Four separable pieces, in dependency order" — update it to reflect that the first piece has shipped and to point at the Done entry, rather than leaving a count that no longer matches the list beneath it.
  Two of the remaining items carry a dependency reference that goes stale once the first item leaves the group, and they do not share one phrase — fix each on its own terms.
  The **Shed recipe: loader/builder** item ends "Depends on the engine-registry item above";
  reword it to point at the shipped registry rather than at an item above it.
  The **loom: convert to a Shed recipe** item says "Depends on all three items above";
  its count is now wrong as well as its position, since only two items remain above it — reword it to name the shipped registry plus the two still-planned items.

  Add a corresponding entry at the top of the `## Done` list, in the shape the existing Done entries use — a bold title, then what actually shipped, then a pointer line.
  It must name: the new `internal/shedrecipe` package;
  the twelve registered engine names;
  the fixed `Constructor` signature and the `Config`/`Env` split;
  the six `internal/loomshed` constructors exported to reach it;
  and the coverage guard pinning the registry against `loomshed.New`'s current row list.
  It must also record what this piece deliberately did **not** do, so the next piece's author is not misled: no recipe file format, no loader, no validity checker, `loomshed.New` keeps its Go literal, and `loomshed.Deps.Preflight` keeps its pre-injected field.
  Close with a pointer line to `designs/shed-recipe.md` and the `internal/shedrecipe` package documentation, matching how the neighbouring Done entries close.

  Every markdown link this card adds or reworks must resolve, file part and `#anchor` alike — this file is under `manifest/` and is scanned by `TestEnforcement_MarkdownLinks`.
- **Commit:** `docs(roadmap): move the Shed recipe engine registry item to Done`

## Batch Tests

`verify: go test ./internal/shedrecipe/... ./internal/lyxcwd/...` covers both halves of this batch.
`internal/shedrecipe` runs the two new guard tests plus the full suite the previous four batches built, so the coverage guard's dependency on the complete twelve-entry table is checked against the real table rather than a stale copy.
`internal/lyxcwd` is included because it hosts `docslink_test.go`'s `TestEnforcement_MarkdownLinks`, the machine check over every markdown link under `manifest/` and `docs/` — three of this batch's four documentation cards edit a file that test scans, so a broken link or a dangling `#anchor` surfaces at this batch's own gate rather than at the task-wide done gate.
The plan's module-wide `verify: go vet ./...` still runs after this batch's own command and is what catches any cross-package compile regression the two scoped packages would miss.
