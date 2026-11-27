---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: consistency
    location: docs/overview.md:407
    summary: The internal/testcover tree line still describes the selector and upkeep planning this diff deletes
    fix: In the `internal/testcover/` line of docs/overview.md, delete the clause ", and compares a map with a commit to plan which tests a build re-measures or to name the miss that rebuilds the package whole" and join the remaining list with "and".
    origin: "lens:code-affected-set"
  - id: F2
    severity: MEDIUM
    class: design
    location: internal/gatecli/testmap_integration_test.go#TestGateTestMap
    summary: No test shows that two tests measured side by side each get only their own blocks, the D3 acceptance criterion
    fix: "Make two tests of one tier that cover different blocks overlap for certain, and assert each entry's own blocks. The pair under the comment `TestAlphaOne and TestAlphaTwo each keep their own blocks` is tier0 and tier1, so it never runs side by side, and both `meets` tests cover block 0. In runTestMapStandIn, let a test whose name makes it cover block 1 also wait in meetsAnother (for example by matching a Meets marker that is not the final suffix beside the `Two` suffix), then assert Blocks [0] for one entry and [0, 1] for the other. Correct the comment on the alpha assertion."
    origin: "lens:code-tests"
  - id: F3
    severity: MEDIUM
    class: design
    location: internal/gatecli/testmapstamp_test.go#TestMapSideBySide
    summary: No row fails when the map run passes the wrong parallel to the memory clamp or swaps the tiers' counts (D4)
    fix: "In TestMapSideBySide add a row with memory_mb 0 and the `asGiven` capped function wanting 4, the tier's Parallel. In TestGateTestMap add an assertion that each tier uses its own count: give tier0 `parallel: 1` and tier1 `parallel: 2`, and show that two tier0 Excludes tests both pass with passed_alone empty."
    origin: "lens:code-tests"
  - id: F4
    severity: MEDIUM
    class: consistency
    location: internal/gatecli/testmap_integration_test.go#TestGateTestMap
    summary: The subtest PrebuiltLymIsBuiltOncePerCall and the row WhileTheLymBuildHangs exist at the review base and are removed with no Loosened-Check trailer naming them
    fix: "Add a commit in the review range (an empty commit will do) carrying `Loosened-Check: TestGateTestMap/PrebuiltLymIsBuiltOncePerCall — replaced by AMapRunBuildsNoLym — a map run makes 0 lym builds` and `Loosened-Check: TestGateTestMap/ACallEndedDuringABuildOrATestWritesNothing/WhileTheLymBuildHangs — removed — a map run makes 0 lym builds, so no lym build can hang`."
    origin: handler
  - id: F5
    severity: LOW
    class: consistency
    location: internal/gatecli/testmappackage.go#testMapRun.runTest
    summary: "Both logger.Warn messages of the map run still open with the retired verb, gate testmap:"
    fix: "Change the prefix `gate testmap:` to `gate test --map:` in both logger.Warn calls of testMapRun.runTest."
    origin: "lens:code-correctness"
  - id: F6
    severity: LOW
    class: consistency
    location: internal/gateslot/template.yaml:5
    summary: "The redundancy_build_timeout_min comment still bounds a lym build of a coverage map run, which builds no lym"
    fix: "Reword the comment so the lym build belongs to the redundancy report step only, for example `minutes each coverage build of a coverage map run, each coverage and lym build of the redundancy report step and each build of lym gate redundant may take ...`. Check each verb's builds against its code before naming them."
    origin: "lens:code-affected-set"
  - id: F7
    severity: LOW
    class: scope
    location: internal/gatecli/testmappackage.go#fillMapTests
    summary: fillMapTests still re-indexes kept entries' blocks and functions, which served only the retired incremental planning
    fix: "Every job now starts from an empty map, so the only entries already in m are spawn-marked ones with no Blocks and no Funcs. Remove the funcIndex seeding from kept entries, the re-indexing loop over them, the `reindexed` helper, the m.Blocks fallback with its blockIndex, and `ranNames` if nothing else uses it. Append the existing entries unchanged, set m.Blocks to table.keys when table is not nil, and rewrite the doc comment to match."
    origin: "lens:code-affected-set"
  - id: F8
    severity: LOW
    class: design
    location: internal/gatecli/testmap.go#runTestMap
    summary: A map call computes its scope with no deadline, so a stalled go list hangs it, where a plain call is bounded by preTierCeiling
    fix: "In runTestMap, call computeScope under context.WithTimeout(runCtx, request.scope.preTierCeiling) and refuse on its expiry with a way forward, as runTest does through preTierExpiry. The test requests built by testMapScope carry no preTierCeiling, so set one there."
    origin: "lens:code-correctness"
  - id: F9
    severity: LOW
    class: design
    location: internal/gatecli/testmap_integration_test.go#TestGateTestMapLinkedPackages
    summary: Nothing fails if the clean check between the coverage build and the tests is removed, so a package dirty at that point could run its tests
    fix: In ADirtyLinkedPackageLeavesTheMapAsItWasAndCountsNothingAgainstTheCap, record the coverage runs (testMapLymSeenEnv) and assert that no test of caller or follower ran.
    origin: "lens:code-tests"
  - id: F10
    severity: LOW
    class: design
    location: internal/gatecli/testmap_integration_test.go#TestGateTestMap
    summary: "No row names a dir/... target, so the subtree filter below a directory other than the module root is untested"
    fix: "In AnEllipsisTargetMapsEveryPackageBelowIt, add a call with a `<dir>/...` target over a fixture package that has a sibling sharing its name prefix, such as `linked/...` in the linked-packages fixture, and assert the built set."
    origin: "lens:code-tests"
  - id: F11
    severity: LOW
    class: design
    location: internal/gatecli/testmap_integration_test.go#TestGateTestMap
    summary: No test covers a package with no map whose test file does not parse, which must enter the set and record the failure
    fix: Add a subtest that commits a package with no map and an unparsable _test.go file, maps it by name, and asserts it is listed in failed, written with a BuildFailure and absent from left_out.
    origin: "lens:code-tests"
  - id: F12
    severity: LOW
    class: design
    location: internal/gatecli/testmap_integration_test.go#TestGateTestMap
    summary: ATiersTestsRunBeforeTheNextTiers shows only that the tiers do not overlap, so reversing their order still passes
    fix: In that subtest, record the order in which the stand-in runs the two tests and assert that the tier0 test ran first.
    origin: "lens:code-tests"
  - id: F13
    severity: LOW
    class: design
    location: internal/gatecli/testmap_integration_test.go#TestGateTestMapInATaskWorktreeKeepsItsOwnMaps
    summary: The whole-module dirty check has no row for a module below the worktree root
    fix: "In the task-worktree test, whose module lies under backend, fail the go list (testMapFailListEnv) once with an untracked file under backend, asserting the package is unmeasured naming it, and once with an untracked file outside backend, asserting the failure is recorded."
    origin: "lens:code-tests"
---

The diff makes the change D1 to D9 settle: every card's work is present, and the deleted symbols have no reference left in `internal/` or `cmd/`.
`go vet` over `internal/gatecli`, `internal/testcover` and `internal/gateslot` passes; I ran no test.
No finding blocks. The open points are two stale texts and two stale log prefixes, one leftover of the old planning, one unbounded wait, missing test coverage and a missing commit trailer.

### [MEDIUM] Overview still describes the deleted planning

**Location:** `docs/overview.md:407`

**Issue:** The line says `internal/testcover` compares a map with a commit to plan re-measures or to name a miss.
`upkeep.go`, `testselect.go` and `declcompare.go`, which did that, are deleted in this range.

### [MEDIUM] Per-test blocks of side-by-side tests are unpinned

**Location:** `internal/gatecli/testmap_integration_test.go#TestGateTestMap`

**Issue:** `TestAlphaOne` is tier0 and `TestAlphaTwo` tier1, and the tiers run one after the other.
The pair forced to overlap covers the same block, so workers sharing a profile path would pass.

### [MEDIUM] The per-tier parallel bound is unpinned

**Location:** `internal/gatecli/testmapstamp_test.go#TestMapSideBySide`

**Issue:** The `cappedTo` rows ignore the argument, and the `asGiven` rows expect the memory quotient.
No row fails when the wrong parallel reaches the clamp.

### [MEDIUM] Removed checks have no trailer

**Location:** `internal/gatecli/testmap_integration_test.go#TestGateTestMap`

**Issue:** The commits after the first two carry no `Loosened-Check:` trailer, and no trailer in the range names these two removed checks.

### [LOW] Log messages name the retired verb

**Location:** `internal/gatecli/testmappackage.go#testMapRun.runTest`

**Issue:** The two warnings open `gate testmap:`, while every refusal of the run was reworded.

### [LOW] Template comment gives a map run a lym build

**Location:** `internal/gateslot/template.yaml:5`

**Issue:** The comment bounds each "coverage, mutant and lym build of a coverage map run, ...", and a map run now builds no `lym`.

### [LOW] Kept-entry merge outlived its planning

**Location:** `internal/gatecli/testmappackage.go#fillMapTests`

**Issue:** `mapPackage` starts every map empty and adds only spawn-marked entries.
The re-indexing and the doc comment's promise about kept entries' keys describe a case that cannot occur.

### [LOW] Scope computation has no ceiling

**Location:** `internal/gatecli/testmap.go#runTestMap`

**Issue:** `runTest` returns into the map run before it builds the pre-tier context.
`computeScope` can run `go list` and relies on its context to end a stalled one.

### [LOW] The check before the tests is unpinned

**Location:** `internal/gatecli/testmap_integration_test.go#TestGateTestMapLinkedPackages`

**Issue:** The check after the tests gives the same unmeasured reason, so removing the earlier check passes.

### [LOW] No `dir/...` target row

**Location:** `internal/gatecli/testmap_integration_test.go#TestGateTestMap`

**Issue:** Only `./...` is tried.

### [LOW] No row for an unparsable test file

**Location:** `internal/gatecli/testmap_integration_test.go#TestGateTestMap`

**Issue:** `leftOutReason` lets such a package in so that its failure is recorded; the `garbled` fixture breaks a non-test file instead.

### [LOW] Tier order is unpinned

**Location:** `internal/gatecli/testmap_integration_test.go#TestGateTestMap`

**Issue:** D4 runs tier0 first, and the subtest named for that order checks only separation.

### [LOW] No subdirectory-module row for the whole-module check

**Location:** `internal/gatecli/testmap_integration_test.go#TestGateTestMapInATaskWorktreeKeepsItsOwnMaps`

**Issue:** The failed-listing subtest runs only with the module at the worktree root, so the `moduleRel` filter of `dirtyModuleFile` is untested.

## Rejected

- Records-side paths can mark a root package unmeasured (code-correctness): `dirtyOwnedFile` predates this range unchanged, card 4 prescribes it, and the case needs a package that owns the records directory.
- Help sentences dropped from `testCmd.Long` (code-affected-set): the 2000-byte cap in `cmd/lym/clitree_test.go` forces long text into the package doc when the `--map` paragraph is added.
- `lym gate test --map` spelled literally in refusals (handler, `PATTERN-code-conventions`): message prose spells command names throughout the package, while the flag registration, the replay words and the slot site use constants.
