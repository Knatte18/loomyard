# Batch: coverage-guard-move-and-fixture-retirement

```yaml
task: 'loom: convert to a Shed recipe'
batch: 'coverage-guard-move-and-fixture-retirement'
number: 3
cards: 3
verify: go test ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/...
depends-on: [2]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch clears the two remaining test-side consumers of `loomshed.New` outside `internal/loomcli`, so batch 5 can delete it.
`internal/shedrecipe/coverage_guard_test.go` moves to `internal/loomrecipe` and is repointed at the recipe, leaving its one registry-only test behind in `internal/shedrecipe/registry_test.go`;
`internal/shedbuild/equivalence_test.go` and its testdata fixture are deleted outright.

The move is forced, not chosen: `coverage_guard_test.go` lives in `package shedrecipe`, and testing the recipe-built list requires importing `internal/shedbuild`, which itself imports `internal/shedrecipe` — an import cycle no in-package `shedrecipe` test can have.

Batch-local decision: `TestRegistry_ShipsTwelveEntries` is appended to `internal/shedrecipe/registry_test.go` rather than left behind in a one-test `coverage_guard_test.go`.
It stays in `internal/shedrecipe` either way, which is what the exact-twelve-names pin requires;
`registry_test.go` already carries `TestNames`, covering `Names()`↔`registry` key agreement and sortedness, so the contents pin belongs beside it and avoids a file whose name no longer describes what it holds.

## Cards

### Card 12: Move the coverage guard onto the recipe

- **Context:**
  - `internal/loomrecipe/fixture_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/loomrecipe/recipe_test.go`
  - `internal/loomrecipe/shape_test.go`
  - `internal/shedrecipe/registry.go`
  - `internal/shedrecipe/registry_test.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/producer.go`
  - `internal/landingshed/publish.go`
  - `manifest/roadmap.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/shedrecipe/coverage_guard_test.go` -> `internal/loomrecipe/coverage_guard_test.go`
- **Requirements:** After `git mv`, rewrite the package declaration to `package loomrecipe` and repoint both surviving tests off `loomshed.New` onto `loomrecipe.New`.

  Delete `TestRegistry_ShipsTwelveEntries` from this file — card 13 re-homes it.
  Delete `coverageGuardShed`, `coverageGuardFakePreflight`, `coverageGuardFakeMergeShuttle`, `coverageGuardNilFabricOpener`, and `coverageGuardLandingDeps`: the moved file now builds through `testEnv(t)` from `internal/loomrecipe/shape_test.go`, which already supplies a filled `Env` including `Landing`, `WebsterRun`, and the four `WebsterDeps` seams.
  `coverageGuardFakePreflight` in particular has no successor — the row is built by `preflightEntry` from `Env.Cwd` now, and these two tests only read row names, so they need no row-1 substitution and must add none.

  Rewrite `loomRowEngines` to key off `loomshed.Name*` constants rather than the string literals it uses today, per the `row-name-authority-stays-with-the-go-constants` Shared Decision.
  The engine-name side stays hand-written string literals — `shedengine.ProducerDef` carries no engine name at all, so only the row-name side is derivable from the built list.
  The thirteen mappings are unchanged: `Preflight`→`Preflight`, `Loom-Preflight`→`LoomPreflight`, `Discussion-Write`→`Stub`, `Discussion-Validate`→`DiscussionValidate`, `Discussion-Review`→`Stub`, `Plan-Write`→`Stub`, `Plan-Validate`→`PlanValidate`, `Plan-Review`→`Stub`, `Batchifier`→`Batchifier`, `Webster`→`Webster`, `Webster-Review`→`Stub`, `Publish`→`Publish`, `Finalize`→`Finalize`.

  `TestCoverageGuard_EveryLoomRowHasAnEngine` keeps all three of its existing assertions — every built row has a table entry, every table key names a built row, every table engine resolves through `shedrecipe.Lookup` — and gains a **new fourth half**: the orphan check.
  Assert that `shedrecipe.Names()` carries no entry the recipe leaves unreachable beyond a named allowance of exactly three: `SingleLLM`, `Bouncer`, and `BurlerRound`.
  The registry ships twelve engines and the thirteen rows use nine distinct ones, so the allowance is what the difference is.
  This is a newly added half, not a weakening — today's test makes no claim at all about unused registry entries.
  Write the allowance down in the test with a comment pointing at the five `loom: real LLM producers` roadmap items that will consume the three, so a reader sees why the list is not empty rather than assuming the guard got laxer.

  `TestCoverageGuard_PublishAndFinalizeRowNamesMatchTheirProducerIdentity` keeps its subject intact: the rows named `loomshed.NamePublish` and `loomshed.NameFinalize` exist in the built list.
  Update its doc comment to say "the recipe's rows" rather than "loomshed.New's list", keeping the explanation of why the property matters — both `landingshed` constructors discard the `name` argument because their identity is the package constants `publishName`/`finalizeName`, so a renamed row would produce a producer whose on-disk identity disagrees with its row name.

  Rewrite the file's own header comment: it currently says the file "builds a real loomshed.Deps and calls loomshed.New" and that "the registry ships exactly the twelve entries this task built".
  The first clause is falsified by this card and the second moves to `internal/shedrecipe/registry_test.go` with card 13.
- **Commit:** `test(loomrecipe): move the registry coverage guard onto the recipe`

### Card 13: Re-home the registry-size pin in `internal/shedrecipe`

- **Context:**
  - `internal/shedrecipe/registry.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/shedrecipe/seam_enforcement_test.go`
- **Edits:**
  - `internal/shedrecipe/registry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append `TestRegistry_ShipsTwelveEntries` to `internal/shedrecipe/registry_test.go`, restored verbatim from the copy card 12 deletes out of the moved file: it asserts `Names()` returns exactly the sorted twelve `Batchifier`, `Bouncer`, `BurlerRound`, `DiscussionValidate`, `Finalize`, `LoomPreflight`, `PlanValidate`, `Preflight`, `Publish`, `SingleLLM`, `Stub`, `Webster`.
  It has no `loomshed` dependency and needs none — it reads `Names()` alone.
  Add a one-line doc comment noting that what is unique to this test relative to the file's existing `TestNames` is the exact-contents pin, and that the pin belongs with the registry rather than with any one consumer of it.

  Then confirm `internal/shedrecipe`'s remaining test files no longer *drive* `internal/loomshed` — grep the package's `_test.go` files for `loomshed` and expect hits only inside `seam_enforcement_test.go`, which legitimately names the package three times: twice in its header prose explaining the allowlist's shape, and once as the `shedrecipeAllowedImports` entry covering `internal/shedrecipe`'s own production import of it.
  Any hit outside that file means a test still reaches for `loomshed` and card 12 missed it.
  Leave `internal/shedrecipe/seam_enforcement_test.go` untouched: its allowlist polices **production** files only (it skips every `_test.go`), and `internal/shedrecipe`'s production code still imports `loomshed` for six registry constructors, so neither the entry nor the header prose is stale.
- **Commit:** `test(shedrecipe): re-home the registry-size pin beside TestNames`

### Card 14: Retire the shedbuild equivalence fixture

- **Context:**
  - `internal/shedbuild/parse_test.go`
  - `internal/shedbuild/build_test.go`
  - `internal/shedbuild/build_engines_test.go`
  - `internal/shedbuild/load_test.go`
  - `internal/shedbuild/check_test.go`
  - `internal/shedbuild/fixture_test.go`
  - `internal/shedbuild/seam_enforcement_test.go`
  - `internal/loomrecipe/recipe_test.go`
  - `contracts/recipes/loom-recipe.yaml`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `internal/shedbuild/equivalence_test.go`
  - `internal/shedbuild/testdata/loom-recipe.yaml`
- **Moves:** none
- **Requirements:** Before deleting anything, grep `internal/shedbuild`'s remaining test files for `testdata` and for `loom-recipe` and confirm `equivalence_test.go` is the only consumer of the testdata fixture — its header claims to be, and this card verifies the claim rather than trusting it.
  If another test turns out to read the deleted testdata, keep the minimum that test needs rather than resurrecting the loom fixture.

  Then delete `internal/shedbuild/equivalence_test.go` and `internal/shedbuild/testdata/loom-recipe.yaml`.
  Remove the now-empty `internal/shedbuild/testdata/` directory if nothing else lives under it.
  Their replacement already exists: `internal/loomrecipe/recipe_test.go`'s shape assertion and structural check, built against the real embedded `contracts/recipes/loom-recipe.yaml`.

  This deletion also removes `internal/shedbuild`'s test-only imports of `internal/loomshed` and `internal/preflightshed`.
  Leave `internal/shedbuild/seam_enforcement_test.go`'s allowlist alone — it polices production files only.

  Run `go test ./internal/shedbuild/... -count=1` afterwards and confirm the remaining seven test files pass with no edits: `fixture_test.go`, `parse_test.go`, `build_test.go`, `build_engines_test.go`, `load_test.go`, `check_test.go`, and `seam_enforcement_test.go`.
  `fixture_test.go` survives and is consumed by `build_test.go` and `build_engines_test.go`;
  its own stale doc comment is repaired by card 26, not here.
  Do not delete `Load` from `internal/shedbuild/parse.go` even though it now has no production caller anywhere in the tree — it is exported, documented, covered by `load_test.go`, and is the entry a future non-embedded consumer needs.
  Equally, do not add a contrived production call to justify it.
- **Commit:** `test(shedbuild): retire the loom equivalence fixture`

## Batch Tests

`verify: go test ./internal/loomrecipe/... ./internal/shedrecipe/... ./internal/shedbuild/...` covers all three packages this batch touches.

On `internal/loomrecipe` it runs the moved guard against the recipe-built list — both directions of the row-table assertion plus the newly added three-engine orphan allowance — alongside batch 2's suite, which must stay green.
On `internal/shedrecipe` it runs `registry_test.go` with the re-homed exact-twelve pin, proving the pin survived the move unchanged, plus the package's own entry tests, which never touched `loomshed.New`.
On `internal/shedbuild` it proves the remaining seven test files — `fixture_test.go` included — stand on their own after the equivalence test and its testdata are gone.

The scope is three packages rather than one because the deletions and the move are two halves of one fact: the loom-driving assertions leave `shedrecipe`/`shedbuild` and land in `loomrecipe` in the same batch, and a partial pass on one side would be a false green.

At this batch's boundary `internal/loomcli` is still the sole remaining caller of `loomshed.New`, which batch 4 removes;
the module-wide `go build ./...` is what proves the tree still compiles with that one caller left standing.
