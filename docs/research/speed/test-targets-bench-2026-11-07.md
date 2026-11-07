# Tier 1 with `{test_targets}` switched on, 2026-11-07

One whole tier 1 pass and one `internal/fabricengine` pass after the hub's tier commands began running `go test` over `{test_targets}`,
read against `fabric-test-speed-remeasure-2026-11-07.md` and the 48 s expectation of the gate landing (`3aca2e50`).

## Answer in brief

- The whole pass took 63.9 s against the re-measure's 65.4 s and the expected 48 s.
  The switch cut 1.5 s, not the expected 8 s.
- `internal/fabricengine` alone took 43.9 s package time (49.4 s pass wall) against the re-measure's 41.5 s (47.1 s pass wall).
  It is no faster than before the switch.
- The whole pass started 67 test binaries, each of them a package that holds a tier 1 test.
- No tier 1 test was lost by narrowing, by a static check; the unnarrowed pass was not run, so no count was compared.
- Neve carried other work during all three starts, so every figure reads high by an unmeasured amount.

## Method

All runs were made once, in the foreground, in the worktree of this task on a clean tree at `202f18ec`.
`lym loom peers` listed no peer run before any of them.

| Pass | Command | Load before (1, 5, 15 min) |
|---|---|---|
| Whole | `lym gate test --tier 1 --all -- -count=1 -shuffle=off` | 1.42, 2.47, 5.29 |
| Package, as specified | `lym gate test --tier 1 ./internal/fabricengine -- -count=1 -shuffle=off` | 7.37, 4.35, 5.73 |
| Package, added by a parent decision | `lym gate test --tier 1 ./internal/fabricengine -- -count=1 -shuffle=off -run .` | 4.94, 4.09, 5.58 |

The package pass as specified ran nothing:
the gate printed `targets carried discovered 0 wall 0.0s`.
The whole pass had already run with the identical args after `--` over the same tree,
so its green record covered the package and the gate carried it forward.
`-shuffle=off` defeats the carry only when the earlier pass did not use it.
The added pass changes the argument list with `-run .`, which selects every test, so it runs the same work and measures the package.

## Results

| Figure | Re-measure (before the switch) | Expected | This run |
|---|---|---|---|
| Whole pass wall, including `go build ./...` and `go vet` | 65.4 s | about 48 s | 63.9 s |
| `internal/fabricengine` inside the whole pass | 47.7 s | | 47.1 s |
| `internal/fabricengine` pass wall, alone | 47.1 s | | 49.4 s |
| `internal/fabricengine` package time, alone | 41.5 s | about 36 s | 43.9 s |

The whole pass discovered 6326 tests: 4295 passed, 0 failed, 2031 skipped.
The package pass discovered 1897: 1728 passed, 0 failed, 169 skipped.
`internal/fabricengine` is still the last package of the whole pass and sets its wall.

## Binaries started

| Pass | Test binaries started |
|---|---|
| Unnarrowed, as the brief gives it | 196, of which 66 hold a tier 1 test |
| Whole pass here | 67 |

The 67 `ok` results in the whole pass's output match the 67 packages the gate launched.
Counting tracked directories that hold a `_test.go` file gives 199 here, not the brief's 196,
and the 67 started binaries are one above the brief's 66 with a tier 1 test.
The gap is not explained: the unnarrowed pass was not run, and the brief's two figures were not re-derived by its method.

## Did the narrowed pass lose a test

By a static check, no.
Every package the whole pass launched holds a `tiergate.Require(..., 1)` call in a test file,
and every package with such a call was launched except `internal/testkit/tiergate`.
That package's own tests call `tiergate.Require(t, 0)` and pass a recording `testing.TB` to `Require` at tier 1,
so it holds no test that runs at tier 1.
A test count of the unnarrowed pass over the same tree would settle the question directly and was outside the cap.

## Reading

- The switch saves binary launches but the pass wall sits on `internal/fabricengine`, which `{test_targets}` leaves whole.
  That explains why 129 fewer binaries moved the wall by 1.5 s.
- The package alone is 2.4 s slower than the re-measure's figure under a comparable load,
  so the two package figures do not separate the switch from load.
- The 48 s expectation is not met by 15.9 s, and the package's 36 s is not met by 7.9 s.
  Neither is shown to be a load effect: load before the runs was between 1.4 and 7.4.
