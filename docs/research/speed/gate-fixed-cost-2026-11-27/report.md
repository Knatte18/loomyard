# The fixed steps of a tier1 gate call, 2026-11-27

What each step of `lym gate test --tier 1` costs before and around the tests, at commit `b65f0be65`.
It follows [test-time-measure-2026-11-27](../test-time-measure-2026-11-27/report.md),
which found a fixed cost per call and did not split it.
The data of every call is in [calls.tsv](calls.tsv).

## Findings

- Two steps are nearly all of a small call's wall time: `go build ./...` (3.0 to 3.9 s)
  and the build of the `lym` binary (2.4 to 2.8 s).
- `go build ./...` takes that long on a tree where nothing changed since the last build.
- `go vet` of the call's packages costs under 0.3 s for one or ten packages, and `go vet ./...` 1.2 to 1.6 s.
- Linking a test binary costs 0.1 to 0.2 s for the small package, and `go test` already links it without DWARF,
  so `-ldflags=-w` changes nothing.
- The gate's own work outside these steps is under 0.3 s for explicit targets,
  and about 1 s for the whole set, most of it one `go list`.

## Method

- Cases:
  - **small:** `./internal/gitexec`.
  - **ten:** `internal/gateslot`, `verifytree`, `gitexec`, `logger`, `dotgit`, `hubregistry`, `shedrecipe`,
    `reedengine`, `shuttleengine/codexengine` and `shuttleengines`.
    All have short tier1 tests, so the call's time is mostly fixed steps.
  - **all:** `--all`, the whole tier1 set.
- Every call was `lym gate test --wait --no-carry --tier <n>`, run one at a time, on an unchanged tree with a warm build cache.
- small and ten ran three times, all ran twice to leave gate slots free for other work.
- tier1's command is `go build ./... && go vet {targets} && go test -json {test_targets} {args}`.
  Two temporary manual tiers with tier1's env and `parallel` ran its first two parts alone:
  tier4 `go build ./...` and tier5 `go vet {targets}`.
  tier4 takes no targets, so it is the same command in every case.
- The tier1 calls added `-- -debug-actiongraph=<file>`, which records the start and end of every action of `go test`.
- Step times:
  - **go build, go vet:** the wall time of the tier4 and tier5 runs.
  - **lym build:** between the log lines that start and end it in the call's trace log.
  - **link:** the `link` actions of the action graph.
  - **gate's own steps:** trace log lines and run records.
- `--tier` runs none of the steps only a `--gate` call has, so they are not timed here (see below).

## Machine and load

AMD Ryzen AI 7 445, 12 threads, Go 1.26.8, 19:02 to 19:10.
Other gate runs shared the machine.

| Case, run | 1-minute load average at start | Slots held by others at start |
|---|---|---|
| small, 1 | 21.3 to 22.5 | 1 to 2 |
| small, 2 and 3 | 15.3 to 17.2 | 0 |
| ten, 1 to 3 | 13.1 to 14.3 | 1 |
| ten with `-ldflags=-w` | 12.6 | 0 |
| all, 1 | 10.7 to 11.0 | 1 |
| all, 2 | 26.3 to 28.2 | 1 |

The hub has four slots, and no call waited for one.
`calls.tsv` holds the load and the holders per call.
The load was well below that of the earlier study, so these times are lower than its times.

## Timings

Seconds, runs in order.

| Step | small | ten | all |
|---|---|---|---|
| `go build ./...` (tier4) | 3.90, 3.47, 2.99 | 4.42, 3.68, 3.82 | 3.86, 3.13 |
| `go vet` of the targets (tier5) | 0.08, 0.08, 0.06 | 0.26, 0.22, 0.17 | 1.25, 1.58 |
| build of `lym` | 2.77, 2.70, 2.36 | 2.95, 2.71, 2.39 | 2.93, 3.20 |
| after the `lym` build, before the command | 0.05, 0.04, 0.06 | 0.06, 0.08, 0.07 | 0.79, 1.06 |
| of which `go list` | none | none | 0.73, 1.00 |
| link, summed over test binaries | 0.19, 0.12, 0.18 | 3.46, 2.98, 3.06 | 60.55, 79.24 |
| link, wall time with any link running | 0.19, 0.12, 0.18 | 1.25, 1.07, 1.07 | 47.36, 61.06 |
| tests, wall time with any test binary running | 0.20, 0.14, 0.22 | 0.70, 0.58, 0.63 | 147.97, 208.43 |
| tier1 command, whole | 4.01, 3.31, 3.55 | 5.19, 5.42, 4.69 | 154.58, 219.36 |
| tier1 call, wall | 6.83, 6.06, 5.97 | 8.20, 8.21, 7.15 | 158.29, 223.62 |

- Inside the tier1 command, the time before `go test`'s first action was 3.0 to 3.6 s for small,
  3.3 to 4.0 s for ten and 5.4 and 8.6 s for all.
  That is within 1.2 s of tier4 plus tier5 in every run but the second of all, which ran at the highest load.
- No call compiled or vetted a package inside `go test`: the cache held every one, so `go test` only linked and ran.
- Links overlap each other and the tests, so in the whole set their time is not additive to the wall time.
  The median link there was 0.75 and 0.92 s.
- The gate's remaining work (startup, config, the green record, the wait records, the stray-file listing, writing records)
  is outside the run record's wall time.
  The first small call took at most 0.15 s longer from the shell than its record says, a `lym gate status` call included.

## Variants on ten

Each is one sample.

| Variant | Result |
|---|---|
| `-ldflags=-w` | No effect: links summed 3.26 s against 2.98 to 3.46 s, call 8.95 s against 7.15 to 8.21 s. |
| Skip `go build ./...` | Saves the whole step, 3.7 to 4.4 s of a 7.2 to 8.2 s call. |

- `go test` links a test binary with `-s -w` unless the binary is kept,
  as the link commands in the action graph show and `cmd/go/internal/test/test.go` sets with `OmitDebug`.
- The skip-build figure is derived, not run: tier1 minus tier4 of the same run, which gives 3.8, 4.5 and 3.3 s.
  It holds on an unchanged tree, where `go vet` and `go test` have nothing to compile that the build would have compiled.

## Why `go build ./...` is slow on an unchanged tree

This is read from Go's source, not timed on its own.
`go build` with several packages makes a link action for every `package main` among them
(`AutoAction` in `cmd/go/internal/work/action.go`) and discards the binaries,
so there is no output a later run could reuse.
The module has `cmd/lym`, `cmd/testtiming` and the commands under `tools/`, all linked on every call.
The `lym` build that runs just before has linked `cmd/lym` already.

## The steps in the gate code

| Step | Where | When it runs now | Once per tree, or dropped |
|---|---|---|---|
| `go build ./...` | the `command` of tier1, run by `runInvocations` in `internal/gatecli/tierrun.go` | every run of tier1's command, also over a remainder the green record left | Both possible: `go vet ./...` type-checks every package. |
| `go vet {targets}` | the same command | every run | Neither needed at its cost; it merges into `go vet ./...`. |
| build of `lym` | `prebuildLym` in `internal/gatecli/prebuild.go` | every run of a tier with `prebuilt_lym` that has a target, whether or not a test uses the binary | Both possible. |
| store of the `lym` binary | `storePrebuilt` | every such run | The store is keyed by the binary's content, so it saves no build. |
| `go list` of the targets | `tierTestTargets` and `expandPackages` | only for a `...` target | Could run once per tree. |
| test binary link | `go test` | every run, since the gate turns the test result cache off with `-count=1` | Neither. |
| green record, stray-file listing, run records | `internal/gatecli` | every call | Neither; they cost too little to matter. |

- A call whose tree and targets the green record already holds green runs no command at all.
  That is the only once-per-tree reuse today, and a dirty worktree gets none of it.
- `internal/gatecli/build.go` has a shared build step that runs once per call, not once per tree.
  The measured hub declares none.
- A `--gate` call also scans for `//lym:only` tests (`only.go`), derives a diff scope for a `diff` key (`scope.go`)
  and scans for guard tests.
  These were not timed.

## Recommended change per step

1. **`go build ./...`:** replace `go build ./... && go vet {targets}` in `gate.tiers.tier1.command` with `go vet ./...`.
   From the figures above this saves about 2 to 3 s on a narrow call, and the whole build step on the whole set,
   which vets `./...` already.
   It gives up one check: `go vet` links nothing,
   so a link error in a command without tests would pass a call that does not build that command.
2. **`go vet` of the targets:** no change of its own; it is part of the command above.
3. **build of `lym`:** build it only when a test file of a targeted package imports `internal/testkit/lymbin`.
   No test file of the small or ten packages names it, so both cases save 2.4 to 3.0 s.
   The whole set still pays it.
   Check first that no test reaches the prebuilt binary another way.
4. **test binary link:** keep it as it is; `-ldflags=-w` is already in effect.
5. **`go list`:** keep it; it costs under 1.1 s and only on `...` targets, where the call runs for minutes.
6. **the gate's other steps:** keep them.

With changes 1 and 3, the small case's fixed steps would be `go vet ./...` and one link, about 2 s instead of about 6 s.
That is an estimate from the separate timings.

## Limits

- Every call ran on an unchanged tree.
  After an edit, `go build` and `go vet` also compile and analyse the changed packages and those that import them,
  and `go vet ./...` was not compared with `go build ./...` in that state.
- Each variant was one sample, and the load varied from call to call.
- The all case ran with `--all`, not with a `--gate` key, so it ran the tests marked `//lym:only` too
  and none of tier0 or tier2.
