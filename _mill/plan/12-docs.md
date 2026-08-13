# Batch: docs

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'docs'
number: 12
cards: 6
verify: go vet -tags integration ./... && go test ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [11]
```

## Batch Scope

Ten markdown files name `lyxtest` or `fabrictest`, and one of them is a build break waiting to happen: `manifest/roadmap.md` links `designs/lyxtest-real-hubs.md`, which the Documentation Lifecycle requires deleting on landing, and the Markdown Link Integrity invariant is machine-checked by `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks`.
So the roadmap move and the design-doc deletion are one card, not two.

This batch also updates `docs/overview.md`'s Tests section and Fabric Vocabulary owner set, and `crucible/review-prompt-template.md`, which names the retired "lyxtest Leaf" invariant — leaving that one alone would have every future crucible reviewer checking against a rule that no longer exists.

`CONSTRAINTS.md` is deliberately absent from this batch: it was rewritten ahead of the code at the start of this task and is already correct, so touching it here would be re-litigating a decision, not finishing one.

Batch-local decision: `docs/benchmarks/fixture-copy.md`'s recorded measurements are kept as historical rows with their date and hardware intact rather than rewritten.
The benchmarks were retargeted, not repeated;
overwriting old numbers with new identifiers would claim a measurement nobody took.

Markdown in this repo uses semantic line breaks — one sentence per line, with breaks at internal independent-clause boundaries — and never a fixed-column hard wrap.
Every edit in this batch follows that rule.

## Cards

### Card 72: Move the roadmap item to Done and delete its design doc

- **Context:**
  - `internal/lyxcwd/docslink_test.go`
  - `docs/overview.md`
  - `internal/gitkit/doc.go`
  - `internal/hubforge/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/lyxtest-real-hubs.md`
- **Moves:** none
- **Requirements:**
  Delete `manifest/designs/lyxtest-real-hubs.md` per the Documentation Lifecycle — its durable half is now the `gitkit` Leaf and `hubforge` Fabric-Fixture invariants in `CONSTRAINTS.md` plus the two packages' own doc comments.
  In `manifest/roadmap.md`, move Planned item 1 — the `lyxtest builds real fabric hubs` entry — into the `## Done` section, written literally as `1.` like every other item there (numbering is automatic and restarts per section, so no renumbering is needed anywhere).
  **Delete its `See [designs/lyxtest-real-hubs.md](designs/lyxtest-real-hubs.md)` link rather than repointing it**: Done entries deliberately carry no `designs/` link, because the doc is deleted when the item ships, and leaving a link to a deleted file is exactly the break `TestEnforcement_MarkdownLinks` catches.
  Rewrite the moved entry's body in past tense and to match what actually shipped, not what was predicted: the packages are `internal/gitkit` (leaf) and `internal/hubforge` (factory), `internal/fabricengine/fabrictest` no longer exists, the migration was 141 measured `Copy*` call expressions of which 132 moved to `hubforge.NewHub` and 9 stayed in `internal/lyxcwd` on `gitkit.CopyRepo`, and the predicted cost was +2.9 s on a ~132 s Tier 2 run.
  Also fix the `fabrictest` mention in the Done section's slice-13 entry so it names the relocated `package fabricengine_test` live-state harness.
  Run `go test ./internal/lyxcwd/...` after this card;
  `TestEnforcement_MarkdownLinks` is what proves the link surgery.
- **Commit:** `docs(roadmap): land lyxtest-real-hubs and delete its design doc`

### Card 73: Update the overview's Tests section and vocabulary owner set

- **Context:**
  - `internal/gitkit/doc.go`
  - `internal/hubforge/doc.go`
  - `internal/lyxcwd/enforcement_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite the `## Tests` section's `internal/fabricengine/fabrictest` paragraph: that package no longer exists, its hub factory is `internal/hubforge` and its live-state integration harness is a set of `package fabricengine_test` files inside `internal/fabricengine/`.
  Keep the substance — the harness drives real cloned hubs into dirty and hostile on-disk states and asserts what a destructive verb may and may not touch — and keep the `internal/boardengine/boardtest` comparison, which is still a live precedent for a test-only sibling package.
  Add `internal/gitkit` and `internal/hubforge` to that section as what they are: `gitkit` the below-fabric leaf holding git primitives, `hubforge` the repo-wide real-hub factory that builds every hub fixture through `fabriccli.CloneAndWire` and asserts nothing.
  In the `TestEnforcement_FabricVocabulary` paragraph, replace `lyxtest` in the owner set with `gitkit` and `hubforge`, matching the owner set already written into `CONSTRAINTS.md` and enforced by `internal/lyxcwd/enforcement_test.go`.
- **Commit:** `docs(overview): describe gitkit and hubforge in the Tests section`

### Card 74: Retarget the benchmark docs

- **Context:**
  - `internal/gitkit/bench_test.go`
  - `internal/gitkit/gitkit.go`
  - `internal/hubforge/bench_test.go`
- **Edits:**
  - `docs/benchmarks/fixture-copy.md`
  - `docs/benchmarks/running-tests.md`
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/benchmarks/scout-vs-grep.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `docs/benchmarks/fixture-copy.md` — thirteen references, including a Reproducing section — rewrite the Reproducing section to name the new benchmark identifiers and their packages: `BenchmarkNewHub`, `BenchmarkNewHubParallel` and `BenchmarkCopyBares` in `internal/hubforge`, and `BenchmarkCopyRepo` in `internal/gitkit`.
  **Keep every recorded measurement as a historical row with its date and hardware intact**, labelled as measuring the retired `CopyPaired`/`CopyPairedLocal` fixtures, and add a short note stating what changed: fixtures are now real hubs built by cloning, measured at 24 ms against the old copy-only 2.3 ms, predicting about +2.9 s on a ~132 s Tier 2 run.
  Do not rewrite an old number under a new identifier — that would claim a measurement nobody took.
  In `docs/benchmarks/running-tests.md`, rename `lyxtest.HermeticGitEnv()` to `gitkit.HermeticGitEnv()`.
  In `docs/benchmarks/test-suite-timing.md`, rename the same call in the hermetic-environment table row and rename `lyxtest` to `gitkit` in the 2026-06-21 history row, leaving both rows' recorded numbers untouched.
  In `docs/benchmarks/scout-vs-grep.md`, the reference is to `internal/lyxtest/lyxtest.go` inside a recorded account of a past agent comparison — update the path to `internal/gitkit/gitkit.go` and add a parenthetical noting the file was renamed, rather than silently rewriting the history of what the agents saw.
- **Commit:** `docs(benchmarks): retarget the fixture benchmark trail onto gitkit and hubforge`

### Card 75: Update the crucible review prompt

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `crucible/review-prompt-template.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the repo-rules line naming the invariants a reviewer must check, replace `lyxtest Leaf` with `gitkit Leaf` and add `hubforge Fabric-Fixture`, so the list reads `Hub Geometry, CLI/Cobra, gitkit Leaf, hubforge Fabric-Fixture, Sandbox Suite Coverage, Documentation Lifecycle`.
  This is the one markdown file whose staleness has behavioural consequences: every crucible reviewer reads it, and an invariant name that no longer exists in `CONSTRAINTS.md` sends them checking a rule that was retired.
- **Commit:** `docs(crucible): name the gitkit and hubforge invariants in the review prompt`

### Card 76: Update the remaining prose references

- **Context:**
  - `internal/gitkit/doc.go`
  - `internal/hubforge/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `CLAUDE.md`
  - `docs/shared-libs/lyxcwd.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `CLAUDE.md`'s invariant list, replace `**lyxtest Leaf Invariant**` with `**gitkit Leaf Invariant**` and add the `**hubforge Fabric-Fixture Invariant**` alongside it, matching `CONSTRAINTS.md`'s own section names.
  In `docs/shared-libs/lyxcwd.md`, rewrite the `ResolveWithAnchor` bypass sentence so its second caller is `gitkit`'s primitive repo fixtures rather than "lyxtest injects anchors into synthetic hubs to build fixtures" — synthetic hubs no longer exist, which is the point, and the wording must match `CONSTRAINTS.md`'s Cwd Resolution Invariant, which already reads "fabric's clone, `gitkit`'s primitive repo fixtures".
  In `manifest/designs/fabric-unified-view.md`, the two `lyxtest` references sit in a historical explanation of why the cycle forced `weftname` to exist as a stdlib-only leaf.
  Keep the explanation — `weftname` still exists for that reason — but update the package names to `gitkit`/`hubforge` and add one sentence recording that the cycle this passage describes was subsequently broken by the `lyxtest-real-hubs` task, so a reader is not left believing the constraint is still live.
- **Commit:** `docs: retarget the remaining lyxtest and fabrictest prose references`

### Card 77: Prove no markdown reference survives

- **Context:**
  - `CLAUDE.md`
  - `CONSTRAINTS.md`
  - `docs/overview.md`
  - `docs/shared-libs/lyxcwd.md`
  - `docs/benchmarks/fixture-copy.md`
  - `docs/benchmarks/running-tests.md`
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/benchmarks/scout-vs-grep.md`
  - `manifest/roadmap.md`
  - `manifest/designs/fabric-unified-view.md`
  - `crucible/review-prompt-template.md`
  - `internal/lyxcwd/docslink_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification-only gate, no diff.
  Run `grep -rln 'lyxtest\|fabrictest' --include=*.md .` excluding `_mill/` and confirm the only surviving hits are the deliberate historical mentions card 72 and card 74 kept — the roadmap's Done entry naming the task by its slug, `docs/benchmarks/scout-vs-grep.md`'s recorded agent comparison, and `docs/benchmarks/fixture-copy.md`'s historical measurement rows.
  Every other hit is a miss and must be fixed under the card that owns the file.
  Confirm `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks` passes, which is the machine half of the link surgery in card 72.
  Confirm every file this batch edited still uses semantic line breaks and gained no fixed-column hard wrap.
- **Commit:** none

## Batch Tests

`verify:` compile-checks the repo under `-tags integration` — cheap insurance that a docs batch changed no code — then runs the two untagged suites that machine-check documentation: `internal/lyxcwd` for `TestEnforcement_MarkdownLinks` (the Markdown Link Integrity invariant, which card 72's roadmap-link surgery and design-doc deletion directly exercise) and `TestEnforcement_FabricVocabulary` (the owner set card 73 updates), and `cmd/lyx` for the guard suites that read invariant names.

No `-tags integration` test run is needed for this batch's own content: it edits eleven markdown files and deletes one, and the only executable consequence is link and vocabulary resolution, both of which are untagged tests.
