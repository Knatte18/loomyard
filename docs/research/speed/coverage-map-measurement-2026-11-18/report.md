# Measurement of the coverage maps, 2026-11-18

Board task `coverage-map-measurement` asked whether the coverage maps are real and affordable before `redundancy-sweep-existing` cuts tests with them.
Everything ran at commit `3746960e` on one 12-core, 31 GB machine that other lym sessions shared throughout,
so every wall time and memory figure includes their load.
The task changed no code and deleted no test.

## Answer in brief

- The full map build works and is affordable: 12 min 50 s wall, against about 45 minutes for the old `lym gate testmap`.
  No memory cap fired, no process was killed, and available memory never fell below 15.9 GB.
- `lym gate redundant internal/boardengine` does not run.
  Its mutation check dies on a mutant that does not compile, 5 s in, so the five known false candidates cannot be confirmed gone.
  The cause is `9ce7233b`, which no one had run on this package.
- A second package, `internal/adoptcli`, gave no candidates, so the crosscheck used a third package, `internal/discussionparser`, for its one candidate.
- Crosscheck: six tests, six agreements by covered blocks per file.
- Verdict: the maps are fit for the cut stage, `lym gate redundant` is not until the `boardengine` build failure is fixed.
  `map-single-test-crosscheck` (E1) is not needed on the evidence of this crosscheck, with the bound below.

## 1. Full map build

Command: `lym gate test --wait --map --all`, run once under `/usr/bin/time -v`, with a sampler writing available memory and the sizes of every `*.test` process each 0.5 s.
The sampler was set to kill the build if available memory fell below 2 GB; it never fired.

| Measure | Value |
|---|---|
| Wall time | 12 min 49.7 s (13:02:15 to 13:15:04) |
| CPU time | 1711 s user, 376 s system, 271 percent of one core |
| Packages and tests | 210 packages measured, 4911 tests |
| Sum of per-package build seconds | 464 s |
| Sum of per-package run seconds | 381 s |
| Largest process of the call, `time -v` | 2.12 GB resident (2,121,004 kB) |
| Largest `*.test` process the sampler saw | 2.14 GB resident |
| Most `*.test` processes at once | 38, summing to at most 2.4 GB |
| Machine available memory | 18.9 GB at the first sample, lowest 15.97 GB over 1407 samples |
| Memory cap or kill | none reported on stdout or stderr; the sampler's abort file was never written |

The sampler matches every `*.test` process on the machine, so its two test-process figures include other sessions' tests.
`time -v` reports the largest process under the call, which the walk's test binaries and `lym` itself make up.
The two agree, so the 2.1 GB peak is probably one test binary of the build, not another session's.
The build does not name that test.

The slowest packages by run seconds are `cmd/lym` (34.1 s), `internal/battencli` (27.6 s), `internal/loomcli` (27.6 s) and `internal/fabricengine` (22.2 s).

The call exits 1 although `ok` is true and nothing failed to build.
`failed_tests` lists ten tests: seven in `internal/proc`, one in `internal/githubclient` and two in `internal/reedengine`.
In `internal/proc` and `internal/githubclient` the listed tests sit in files tagged `//go:build windows`,
and a `-run` on them on this machine runs no test.
Their names suggest the `reedengine` ones are Windows-only too; that was not checked.
Per `internal/gatecli`'s doc, a non-empty `failed_tests` exits 1, so a full build on Linux always exits 1.
`passed_alone` lists `cmd/lym.TestTestOnlyCode_EveryProductionFunctionIsReachableFromAMain`, which failed in the round and passed alone.

## 2. `lym gate redundant`

Each package ran once, under `/usr/bin/time -v`.

| | `internal/boardengine` | `internal/adoptcli` | `internal/discussionparser` |
|---|---|---|---|
| Wall time | 5.0 s, then an error | 0.04 s | 35.3 s |
| Peak resident size of the `lym` process | 288 MB | 39 MB | 135 MB |
| Result | error, exit 1 | ok | ok |
| Tests in the map | not printed | 25 | 38 |
| Candidates | none printed | 0 | 1 |
| Refuted, unconfirmed, not judged, kept | none printed | 0, 0, 4, 1 | 4, 1, 1, 3 |

`internal/adoptcli` stood in as the package of the run's choosing, because the pilot report lists its candidates.
It gave none, so `internal/discussionparser` was run as well, a deviation from "one other package": a pure parser of 38 tests with a mutation check that takes seconds.
The raw answers are in [adoptcli-redundant.json](adoptcli-redundant.json), [discussionparser-redundant.json](discussionparser-redundant.json) and [boardengine-redundant-error.json](boardengine-redundant-error.json).

### `internal/boardengine` fails

The verb ends with:

```
the mutation check of internal/boardengine failed: mutant binary: go test -c ... exit status 1:
overlay-3/handles.go:62: invalid operation: digits[lymMutantInt(0, 274)] == lymMutantInt('0', 275) (mismatched types byte and rune)
```

`9ce7233b` ("testcover: mutate data literals and delete boolean-chain operands") rewrites a rune literal as a call returning an `int`,
and the comparison `digits[i] == '0'` in `internal/boardengine/handles.go` then compares a `byte` with that value.
The verb takes this one failed compile of the whole mutant binary as a failure of the check.
The earlier runs of 2026-11-17 handled 52 unbuildable mutants and finished, so the failure comes from the data-literal operator.
Whether a typed literal elsewhere breaks other packages is not known: `internal/adoptcli` and `internal/discussionparser` finished.

So the five known false `boardengine` candidates cannot be checked against the new rules.
The map itself is fine for this package: it was built in the full build and its entries were read in the crosscheck below.

### Comparison with the pilot report

[coverage-map-pilot-2026-11-12/report.md](../coverage-map-pilot-2026-11-12/report.md) lists two `internal/adoptcli` candidates, `TestReconcileSeamWaitsOnAPushLock` and `TestWorkerArgs`.
Neither name is in the package's map today, which holds 25 tests where the pilot judged 34 entries, so they were removed or renamed since.
The empty candidate list is therefore not evidence that a candidate went away.
Of the pilot's two keep markers, `TestAdoptFields` is still kept; `TestTriesStore` is no longer an entry.
Four tests are not judged for an `unclassifiable call`, two of them the pilot's own.

`internal/discussionparser` lists one candidate, `TestAppendDecision_EmptyFieldWritesNothing` (0.000024 s, removable),
covered by `TestAppendDecisions_WritesBatchWholeOrNothing`, which killed 3 mutants.
It was not read against its cover for what the cover asserts, which this task did not ask.

## 3. Hand crosscheck

For each test, `lym gate test --wait ./<package> -- -run '^<Test>$' -count=1 -coverprofile=<path>` ran alone.
The check counts the profile's blocks with a count above zero per file and compares that with the map entry's blocks grouped by file.
It reads the map as `lym gate redundant` does, and it works on counts because the map's blocks carry no line range.

Only one real candidate exists, so the tests are that candidate, its cover, and four of the 2026-11-17 `boardengine` candidates, whose package could not be judged this time:

| Package | Test | Per-file blocks, profile and map | Result |
|---|---|---|---|
| `internal/discussionparser` | `TestAppendDecision_EmptyFieldWritesNothing` (candidate) | `append.go` 9 | equal |
| `internal/discussionparser` | `TestAppendDecisions_WritesBatchWholeOrNothing` (its cover) | `append.go` 47, `patternchanges.go` 54, `validate.go` 13 | equal |
| `internal/boardengine` | `TestRunHolds` | `render.go` 1, `runlock.go` 8, `store.go` 4, `task.go` 9 | equal |
| `internal/boardengine` | `TestRunLock_BeamKindAndBeamChange` | `render.go` 1, `runlock.go` 22, `store.go` 4, `task.go` 9 | equal |
| `internal/boardengine` | `TestReadRaddleStore` | `raddlestore.go` 20, `render.go` 1, `store.go` 4, `task.go` 9 | equal |
| `internal/boardengine` | `TestConfigTemplate_ResolvesToDefaults` | `render.go` 1, `store.go` 4, `task.go` 9, `template.go` 1 | equal |

That is six tests, one above the five asked for, because the candidate's cover was checked with it.
No file's count differs, in either direction.
The three `boardengine` tests of 2026-11-17's check (`TestOutputs`, `TestStoreValidatesLabelsWithVocabulary`, `TestWriteSiblings`) were not repeated.

Bound: a count per file cannot show that the same blocks are covered, nor read the map's `funcs`.
A map that lists one block instead of another of the same file would pass.
The crosscheck therefore supports the map's totals, not its identity of blocks, which only `map-single-test-crosscheck` (E1) would test.

## Verdict

- Maps fit for the cut stage: yes for the build, at 13 minutes and a 2.1 GB peak process under a machine that stayed above 15.9 GB available.
  The crosscheck found no disagreement, so E1 is not needed by the task's own rule, which needs it on a disagreement.
  It stays a sound addition, since the check cannot see which blocks.
- `lym gate redundant` is not ready: `internal/boardengine` fails in its mutation build, so the cut stage cannot judge that package until `9ce7233b`'s literal operator keeps the type of a byte or rune literal.
- A full build on Linux exits 1 on ten Windows-only tests; a caller that reads the exit code, rather than `failed_tests`, sees a failure.

## Defects for notes

- `redundant-mutant-build-aborts-check`: a mutant that does not compile fails the whole `lym gate redundant` call for `internal/boardengine`.
- `map-build-exits-1-on-windows-only-tests`: `failed_tests` lists tests that cannot run on the host's platform.
