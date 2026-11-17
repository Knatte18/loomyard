# Trial of the coverage-map tooling, 2026-11-17

A run of `lym gate test --wait --map` and `lym gate redundant` on two packages at `b92c95ee`, before `redundancy-sweep-existing` relies on them.
The trial changed no code; the defects it found are filed as notes.
Each package was mapped twice and `lym gate redundant` ran once per package, on a 12-core, 31 GB machine shared with other lym sessions.

## Packages

`internal/lymdirs`, the small package the brief named, has no tier0 or tier1 test.
`--map` answered `ok` with `left_out: "no tier0 or tier1 test and no map"` in 0.03 s and built nothing.
`internal/durfmt` (2 tests) stands in as the small package.
`internal/boardengine` (136 tests, 134 tier0 and 2 tier1) is the heavy one.

## Map build

| | `internal/durfmt` | `internal/boardengine` |
| --- | --- | --- |
| first run wall time | 1.58 s (build 0.89 s, run 0.04 s) | 12.41 s (build 7.27 s, run 3.96 s) |
| second run wall time | 1.31 s | 4.97 s |
| peak resident size of the `lym` process | 125 MB | 295 MB |
| tests mapped | 2 | 135 |

The second `boardengine` run is faster because the build cache was warm.
`/usr/bin/time -v` measures the `lym` process, not its test binaries.
Sampling `ps` every 50 ms during the second `boardengine` run showed at most 2 test binaries at once and at most 252 MB resident summed over them.
The sampler matched binaries by name and may have missed some, so treat that as a lower bound.
The tier0 entry of `gate.yaml` sets `parallel: 4` and no `memory_mb`, so the code's bound on side-by-side runs is 4 and no memory cap applies.
The run's report says 135 tests for `boardengine`, where its map holds 136 entries.

## Map correctness

Each test's entry was checked against a single-test run: `lym gate test --wait ./internal/<pkg> -- -run '^<Test>$' -count=1 -coverprofile=<path>`.
The check counts covered blocks per file in the profile against the entry's `blocks` grouped by file.

| package | test | result |
| --- | --- | --- |
| `internal/durfmt` | `TestFormat` | 9 of 9 blocks, equal |
| `internal/durfmt` | `TestLeafInvariant_StandardLibraryOnly` | 0 blocks, equal |
| `internal/boardengine` | `TestOutputs` | every file's count equal, 18 blocks |
| `internal/boardengine` | `TestStoreValidatesLabelsWithVocabulary` | every file's count equal, 209 blocks |
| `internal/boardengine` | `TestWriteSiblings` | every file's count equal, 909 blocks |

`durfmt` has two tests, so two were checked, not three.
The check is by count per file because the map's blocks carry no line range.
The map's `funcs` lists have no counterpart in a statement profile and were not checked.

A second run writes the same sets but not the same bytes.
Beside `built_at` and `measured_at`, the `funcs` array of four `boardengine` tests (`TestHandles_AssignedLowestFreeAndStable`, `TestHoldsRaddleFiling`, `TestRunLock`, `TestSetStatusReleasingProject_ReleasesDraftMembers`) lists the same ids in another order.
The set difference of every test is empty in both directions.
The two `durfmt` maps differ in timestamps alone.

132 of the 136 `boardengine` tests record `seconds: 0`, and the largest is 0.29.

## Leftovers

- `git status --short --ignored` in the worktree was the same before and after every command, and `git status --short` stayed empty throughout.
- The hub's gate run records name the worktree's slug five times, all from the single-test runs of the cross-check.
  Neither `--map` nor `lym gate redundant` wrote one.
- The scratch root `/tmp/lym-1000` grew during the trial, but other sessions run tests in it, so none of the growth can be pinned on these commands.

## `lym gate redundant`

| | `internal/durfmt` | `internal/boardengine` |
| --- | --- | --- |
| wall time | 0.01 s | 21 min 39 s |
| peak resident size of the `lym` process | not measured | 2.1 GB |
| candidates | 0 | 14 |
| refuted | 0 | 29 |
| kept | 0 | 8 |
| not judged | 1 (`no blocks`) | 1 (`TestSetOwners`, `unclassifiable call`) |

`boardengine` mutation cost: 1284 mutants, 52 of them unbuildable, 30994 test runs, 1296 s of test time.
383 runs at 10 source lines ended by timeout and 39 by the memory cap; the verb counts both as kills.
The 61 tests with a timeout were not examined for whether the mutant they "killed" was a real kill.

The 14 candidates are `TestBoardRefusesUnconfiguredLabel`, `TestConfigTemplate_ResolvesToDefaults`, `TestListAndFindLabelFilter`, `TestLoadConfig_DefaultRecipe`, `TestMergeTasksSetStatusRollback` (not removable), `TestProjectDependencies`, `TestPromoteMakesNoteATaskAndIsIdempotent`, `TestReadRaddleStore`, `TestRenderRecipeCellTakesTheDefaultRecipe`, `TestRunHolds`, `TestRunLock_BeamKindAndBeamChange`, `TestRunLock_SketchExemptionIsTheWholeNetChange`, `TestSetDeps` and `TestUpsertTaskDefaults`.
All 14 list `seconds: 0`.
The trial did not delete any candidate to test the list, so it says nothing about the false confirmations the earlier survey found.

`git status` showed no mutant or other changed file during the run (polled every 2 s) or after it.

## Defects and gaps, filed as notes

- `map-entry-order-unstable`: the `funcs` order of a map entry changes between builds.
- `map-test-seconds-zero`: per-test seconds are 0 for nearly every test, so the candidates carry no timing.
- `map-single-test-crosscheck`: no lym verb checks a map entry against a single-test run, the blocks carry no line range, and `funcs` has no profile counterpart.
