# Tier 1 revision: where a pass goes and what to change, 2026-11-06

A measurement report on neve, the operator's Ryzen machine, at revision `23518a94` plus this report.
It ranks changes to tier 1 by the time they take off a gate pass.
It changes no code, test or config, and files no task; the ranked list at the end is for the operator to pick from.
Tier 2 stays in the `darn-write` and `code-burler-fix` gate keys, as the operator decided.

It builds on [fabric-test-speed-survey-2026-11-03.md](../fabric-test-speed-survey-2026-11-03.md) and [run-slowness-causes-2026-11-06](../run-slowness-causes-2026-11-06/report.md) and does not repeat their measuring where it still holds.
The survey's counts of git processes and its list of levers were checked against today's tree; its per-test seconds came from a trace2 run and are not comparable with the untraced seconds here.

## Answer in brief

- A whole-repository tier 1 pass takes 75.0 s on a quiet neve and a median of 111 s over the 43 whole-repository passes the gate recorded since 2026-11-05, which ran beside other gates.
  `internal/fabricengine` is the pass's wall: 57.0 s of the 75.0 s, and 77 percent of the wall on average over those passes.
  It starts after a floor of about 18 s of build, vet, link and binary starts, so the wall is the floor plus the package.
- The floor holds 130 test binaries that have no tier 1 test: 66 of the 196 packages the pass starts hold one.
  Starting only the 66 takes the floor from 20.5 s to 12.4 s, 8 s off every tier 1 pass, with no test lost.
  It is the one change that needs no test edit, and it weighs most on the hub's other machine, where a binary is held on its first start.
- Inside `internal/fabricengine`, none of the survey's levers has been taken since it was written, and all still stand.
  The package makes 98,760 git processes a run; at about 1.0 ms a process they are 63 percent of its serial time of 156.8 s.
  Ranked by what they take off the package's wall: pair templates the tests can build for themselves (about 10 s), `worktree list` through `dotgit` (5 s), then six changes of 1 to 2.5 s each.
  Together they take the package from 57 s to about 36 s, and a pass from 75 s to about 56 s.
- A pair template costs 0.18 s to build once per test binary and saves 67 ms for each pair a test adds; it pays from the third use of a start state.
  The closed `Shapes` set cannot hold the start states the tests need, so the lever is a template a test registers with a builder function, not more shapes.
- Folding and cutting tests is the smallest lever.
  Every candidate costs 0.1 to 3 s serially, together about 7 s of the package's 156.8 s, and the coverage-based redundancy report finds five removable tier 1 tests worth 1.4 s in the twelve packages it was run over.
  Tier 1 has no pile of redundant tests; its cost is the hub each test builds before it asserts.
- Sharing one repository across rows saves little at today's fixture prices.
  A fixture now costs 0.13 to 0.25 s a row, the shared-hub rows are a few of each table, and each share hides later rows behind a first failure.
  The shape worth taking is the third, a template to copy for a broken start state, because it removes the per-row build without touching the assertions.
- Together, the changes take a whole pass from 75 s to about 47 s, and a darn run's tier 1 time from 7.0 to 5.2 minutes alone and from 12.1 to 9.4 minutes beside two other runs.
  That is 4 to 6 percent of a darn run; the gate keys' tier 2 is a separate and larger saving.

## Route and method

Every tier 1 number comes from `lym gate test`, as the gate rule asks.
The gate's `tier1` command runs `go build ./...`, `go vet` and `go test -json` with a prebuilt `lym` under `GOFLAGS="-trimpath -p=4 -timeout=20m"`.
An untracked file sat in the worktree for the measuring runs so the gate read no green record, and was removed afterwards.

| Run | Command | Gate wall | One-minute load before and after |
|---|---|---|---|
| W1 | `--tier 1 --all -- -count=1`, cold, 196 binaries run | 75.0 s | 1.8 and 8.4 |
| W2 | the same with `-run '^$'`, no test runs | 20.6 s | 9.9 and 8.7 |
| W2 again | the W2 command repeated | 20.5 s | 3.9 and 4.3 |
| W3 | the W1 command with trace2 on | 83.2 s | 1.9 and 9.9 |
| T66 | `--tier 1` over the 66 packages with a tier 1 test, `-run '^$'` | 12.4 s | 3.9 and 3.9 |
| F1 | `--tier 1 ./internal/fabricengine -- -count=1`, trace2 on | 67.5 s | 5.7 and 7.1 |
| F2 | the same, `-parallel=1`, trace2 off | 162.2 s, package 156.8 s | 2.8 during |
| P | a probe of `hubforge` templates through the gate, `-parallel=1` | 8.2 s | 8.0 and 7.1 |

Bounds: the operator allowed three whole-repository runs and two of `internal/fabricengine`.
The whole-repository runs are W1, W2 and W3; W2 again is a fourth, run by mistake as a same-load repeat of W2, one over the bound, and it agrees with W2 to 0.1 s.
The `internal/fabricengine` runs are F1 and F2.
T66 and P are not whole-repository runs, and neither runs `internal/fabricengine`.
No run happened on the other machine; the last section reasons about it.

- Per-package seconds are the `launches` of the gate's run record: each package's `go test` elapsed time.
- Per-test seconds are the `--- PASS` lines of the gate's log.
  A test's cost is its own time plus every subtest that paused, folded up the subtest tree, as in the survey.
  `pertest.sh` beside this file computes it and ties a test to its package, because a gate log interleaves the packages.
- Git processes come from trace2 event files, as in the survey: `GIT_TRACE2_EVENT` with `GIT_TRACE2_ENV_VARS=LYM_FIXTURE_GIT`.
  `census.sh` beside this file reads them.
  A process is tied to a test through the test name in its `t.TempDir` path; 19 percent of F1's processes have no path and are children such as `receive-pack`.
  The fixture marker covers the gitkit helpers, every git while a hub builds and the production verbs a fixture drives, so it is not "git the test wrote", and counts below are totals unless they say code.
- F1 and W3 are traced and slower than untraced runs; the walls above name them so.
  The other walls are untraced.
- The redundancy report ran through `go run ./cmd/testtiming -redundancy -tags tier0,tier1` over twelve packages, never `internal/fabricengine`, which would have been a third run of it.
- The load was between 1.8 and 14 at the starts and ends of runs; other sessions held gate slots at times, and a spread of about 30 percent between runs of one command is to be expected.
  The counts of processes and tests do not depend on load.

## 1. Where a tier 1 pass goes now

### The pass

| Part | Seconds | How read |
|---|---|---|
| Whole pass, cold, quiet | 75.0 | W1 |
| Floor: build, vet, link and start 196 binaries | 20.5 to 20.6 | W2, twice |
| of which `go build ./...` and `go vet ./...` with the gate's flags, warm | 3.4 | 2.5 s and 0.9 s, repeated |
| Floor over the 66 packages with a tier 1 test | 12.4 | T66 |
| `internal/fabricengine`, the last package | 57.0 | W1's record |
| Sum of every package's time | 177.8 | W1's record, 196 packages |

Binary start is not the cost on neve: the gate's `launch_seconds` over 196 packages add up to 0.55 s.
The floor is compiling and linking 196 test binaries; a binary that holds no tier 1 test still links before it answers `no tests to run`.
Of the 196 binaries 130 hold no tier 1 test, and their package times add up to 1.2 s.

The pass's wall is the floor, about 18 s, then `internal/fabricengine` at 57 s, with every other package finishing inside its shadow.
Over the 43 whole-repository passes since 2026-11-05, in which the gate often ran beside others:

| Measure | Min | Quartiles | Median | Max |
|---|---|---|---|---|
| Wall | 20.6 | 103.7 and 139.2 | 111.1 | 202.6 |
| `internal/fabricengine` time | | 81.6 and 109.2 | 88.9 | 169.9 |
| Wall less `internal/fabricengine` | 18.0 | 21.2 and 28.6 | 23.4 | 46.0 |

`internal/fabricengine` is 77 percent of the wall on average.
Once it falls below about 35 s the pass is bound by the other packages, 121 s of package time over four workers, and the floor.
A list schedule of W1's package times over four workers, started after the floor, gives the wall for a shorter package:

| `internal/fabricengine` | 57 s | 45 s | 35 s | 25 s | 15 s |
|---|---|---|---|---|---|
| Modelled wall | 81.9 s | 69.9 s | 59.9 s | 57.6 s | 55.3 s |

The model reads 9 percent high against W1's measured 75.0 s, and its shape is what matters: below 35 s the next lever is the floor and the other packages, not the package.

### The packages that set the wall

Seconds of the package's own `go test` time in W1, and the mean over the recent whole passes, which ran beside other gates.

| Package | W1 | Mean of recent passes | What sets its time |
|---|---|---|---|
| `internal/fabricengine` | 57.0 | 101.6 | 1,792 tests, 98.8k git processes; see section 2 |
| `internal/fabriccli` | 17.7 | 20.4 | `TestRunCLI_ReconcileBoardFailureFailsTheVerb`, four rows that each clone a hub through the `fabric clone` verb |
| `internal/gatecli` | 16.0 | 17.0 | `TestGateTest_DetachedRun`'s 29 parallel subtests, with real stand-in delays of 2 to 5 s and leases of 3 s |
| `internal/battencli` | 8.5 | 13.6 | `TestBattenIntegration_Rows`, one serial test of about 38 steps; 8.5 s is the package |
| `internal/webstercli` | 7.2 | 10.4 | `TestResetCmd`, 28 serial fixtures that each add a pair and two commits |
| `internal/loomcli` | 7.1 | 10.4 | 42 tests that each copy a hub and run `lym` as a process |
| `cmd/lym` | 6.8 | 8.7 | a native `go build` of the module, a `dotnet build` and a `go list -deps`: 7.6 s of cost |
| `internal/testkit/lymbin` | 6.7 | 8.6 | three tests that each run a cold `go build ./cmd/lym` |
| `internal/hubreconcile` | 6.4 | 10.0 | `newStaleHub`, `CopyHub` plus a pair plus eight commits, built 12 times |
| `internal/gitexec` | 5.4 | 5.5 | `TestRun_RemoteBounds`, serial, waits real deadlines and a literal `sleep 2` |
| `internal/beamcli` | 4.3 | 5.0 | `TestDobby_ReexecsOntoAReplacedBinary`, a real detached process and a 1 s poll |

The ten packages after `internal/fabricengine` add up to 86 s, 71 percent of the other packages' 121 s.
A package's per-test cost, which sums parallel subtests, overstates its time: `TestGateTest_DetachedRun` costs 97 s and the package takes 16 s.

## 2. `internal/fabricengine`

### Today against the survey

| Measure | Survey, `b2b64690` | Today, `23518a94` |
|---|---|---|
| Tier 1 tests passed | 1,607 | 1,792 |
| Serial package time, untraced | 148.7 s | 156.8 s |
| Package time at the default `-parallel` | 71 to 83 s, loaded | 57.0 s in W1, quiet |
| Git processes | 78,192 | 98,760 |
| `worktree list` processes | 11,769 | 14,744 |
| Hub copies (`commit --amend` of `CopyHub`) | 1,145 | 1,272 |
| Pairs added (`worktree add` of both sides, halved) | about 540 | about 764 |

The serial run is 2.75 times the default-parallel one.
No lever of the survey has landed: `git log` shows no commit to `internal/hubforge` since `6c5727df`, and `worktreelist.go`, the post-checkout hook script and the shape set are as the survey read them.

Where the 156.8 s goes, by test, from F2:

| Share | Tests | Seconds |
|---|---|---|
| Top 10 | `TestCrossProduct` 11.8, `TestRecordsReplay` 6.5, `TestAnchorChange` 5.5, `TestReconcileOrigin_Moves` 5.0, `_Judge` 4.9, `TestPushFailureClasses` 4.8, `TestReconcileOrigin_Ended` 3.7, `TestBolt_PullThenCommitWritten` 3.3, `TestAdd_AdoptOnly` 3.3, `TestVerbCases_CleanState` 3.2 | 51.8 (33 percent) |
| Top 25 | | 75.9 |
| Top 50 | | 94.0 |
| Top 100 | | 112.0 |
| All 589 | 297 of them under 0.1 s | 156.6 |

A third of the time is in ten tests and no test is long: the longest cost is `TestCrossProduct` at 11.8 s over 150 parallel cells.
One hub-bearing subtest costs 0.13 to 0.25 s serially in `TestAnchorChange`, `TestPushFailureClasses` and `TestReconcileOrigin_Moves`: the time is hubs, not assertions.

The serial time follows the git process count.
At 1.0 ms of wall for one `git worktree list --porcelain` in a small repository, measured over 300 starts, 98,760 processes are 99 s of the 156.8 s, and the rest, 58 s, is the test binary: hub copies, go-git reads and assertions.
A change is therefore worth about 0.6 ms of serial time for each git process it removes, and the process counts below are the ranking's currency.

### What the pair templates cost to build

The survey gave what pair templates save; `template-probe_test.go.txt` beside this file measures what they cost, warm, serial, at a load of 7 to 8:

| Step | Time | Note |
|---|---|---|
| Cold `CopyHub` of `.`, including the bare templates and the plain template | 148 ms | once per test binary |
| Cold `CopyHub` of `.` with the pair `pair`, including the pair template | 195 ms | the pair template is about 180 ms of it: clone, wire, `AddPair`, fingerprint |
| Cold `CopyHub` of `backend`, and of `backend` with the pair | 123 ms and 194 ms | |
| Four shapes cold, in all | about 0.66 s | the most any binary pays; the package pays only for the shapes its tests use |
| Warm `CopyHub` of `.` | 16.6 ms | median of 15 |
| Warm `CopyHub` of `.` with the pair | 17.8 ms | the pair costs 1.3 ms more |
| Warm `CopyHub` of `.` then `AddPair` | 84.2 ms | `AddPair` is 67 ms and about 60 git processes |
| Warm `NewHub` of `.` | 103 ms | a fresh two-clone hub |

A template built for one more start state costs about 0.18 s once per binary and saves 67 ms for every pair a test would otherwise add, so it pays from the third use.
`templateFor` holds one mutex for every build, so templates build one after another; a template per start state needs a lock per key or the first tests of the run queue behind each other.

### The levers at today's tree

Savings are serial seconds of the package, then the share of its 57.0 s default-parallel wall at the same proportion (0.364 s of wall for each serial second).

| Rank | Lever | Still stands | Serial saving | Wall saving | Size |
|---|---|---|---|---|---|
| 1 | Templates a test registers with a builder, replacing `CopyHub` plus `AddPair` plus the layered commits | Yes; the shapes are closed | 24 to 31 s, net of about 5 s of template builds | 9 to 11 s | Large: about 150 sites, 25 builders |
| 2 | `worktree list` through `dotgit` in `fabricengine.List` | Yes; unchanged | 14.7 s alone, 6 s after rank 1 | 5.3 s alone, 2.2 s after | Small to medium: one function, 35 production callers |
| 3 | A slimmer `CopyHub`: no fingerprint walk per copy, a packed template, no `amend` and `push --force` per copy | Yes | 4 to 8 s | 1.5 to 3 s | Medium |
| 4 | Fold the overlapping tests (section 4) | Yes; none was taken | about 7 s | 2.5 s | Medium |
| 5 | Write the quiet config into `.git/config` from Go, not 4 to 5 `git config` processes per `git init` | New | about 5.6 s | 2 s | Small |
| 6 | `CopyHub` in `newRecordsFixture` and the `adoptcli`, `hubreconcile` `NewHub` sites | Yes | about 4 s for 47 hubs here | 1.5 s | Small |
| 7 | The post-checkout hook without `dirname` and `basename` | Yes | about 2 s | 0.7 s | Small |

The order is by wall saving, then size.
Rank 2 is the first to take: it is one function and a parity test, it gives 5 s, and it also speeds `lym`, which calls `List` three or more times in each `Add` and `Remove`.
Rank 1 gives the most and is the largest; once it lands, rank 2's saving falls to 6 s, because each removed pair also removes its `worktree list` calls.
Taken together, ranks 1 to 7 remove about 57 serial seconds, 37 percent, and take the package from 57 s to about 36 s.

- **Rank 1, why a builder and not more shapes.**
  The fixtures' start states are a hub plus one to three commits plus a pair under the test's own slug: `newAnchorHub` adds two tracked directories, `newOriginHub` pushes the records branch with an upstream, `newStaleHub` in `hubreconcile` adds eight commits, `beamHub` in `loomcli` adds a beam and two members.
  The slug is `task`, `task1`, `mover`, `pair-a`, `owned-task` or a table row's name: of 206 `AddPair` call sites, 103 pass a variable named `slug`, and the rest pass literals such as `task` and `task1` or other variables.
  A template per slug works only where the slug carries no meaning; a registered builder works for any start state, built once per binary and copied at 17 ms.
  The tests that exercise `Add` itself, about 8 percent of the pairs added, stay as they are.
- **Rank 2.**
  `worktree list` is 14,744 processes in F1 and 22,636 in a whole pass: 14.9 and 14.3 percent of all git.
  The `.git/worktrees/<name>/` entries hold `gitdir`, `HEAD` and `locked`; the `prunable` mark is a `gitdir` whose target is gone.
  Reading them is a local read under `PATTERN-gitrepo-client-boundary`, as `dotgit` already serves for geometry.
- **Rank 3.**
  1,272 copies at 16.6 ms are 21 s of serial time, 13 percent; three of the 17 ms are the `amend` and `push --force` that fold the relocated binding into the board's one commit, and the rest is the fingerprint walk and the copy of 180 files.
  The survey measured the fingerprint walk as a file-opens lever; it is also a time lever.
- **Rank 5.**
  1,601 `config maintenance.auto`, 1,601 `config gc.auto`, 411 `user.name` and 411 `user.email` processes a run, and the same for `core.fsmonitor`, come from `initRepo` and `initScratchRepo` in `gitkit` and `hubforge`.
  `HermeticGitEnv` already sets the same keys in the global config every test binary inherits, so the per-repository calls repeat a state the environment holds.
  Whether a test runs without that environment is for the author to settle before the change; the layer exists for a subprocess that does not inherit it.
- **Not worth a task.**
  `go-git` for the code side's other reads, 4 percent of processes, is section 6; `t.Parallel` in the serial top-level tests and `-parallel=24` are noise within this report's load, as the survey found.
  `gitrepo.DropRemoteTracking` runs `update-ref -d` and two `config --unset` per teardown, 3,036 processes, and is a write under the remote-refs lock: it stays on git.

## 3. The other slow packages

Seconds are the package's `go test` time in W1; the lever's saving is of that time.

| Package | Costliest tests and fixtures | What makes it slow | Lever, saving |
|---|---|---|---|
| `internal/fabriccli` 17.7 | `TestRunCLI_ReconcileBoardFailureFailsTheVerb` 16.5 s cost; `TestRunHubPull_WaitsForAHeldParentLock` 4.2; `TestRunCLI_PullHubFromATaskPair` 3.2 | Each of the four rows of the first builds a hub through the `fabric clone` verb and two bares, `makeCLICloneWarpBare` and `makeCLICloneWeftBare`, about 9 processes plus the clone's own; the clone is fixture, not the behavior | `CopyHub` for the four rows: about 14 s of cost, 6 s of package time |
| `internal/gatecli` 16.0 | `TestGateTest_DetachedRun` 97 s cost over 29 subtests; `TestGateTest_Scenario` 10.5 s serial, about 60 rows; `TestGateRunner` 9.0; `TestGateTest_DotnetFixture` 8.6; `TestGateTest_Scopes` 6.4 | Each subtest re-executes the test binary as `lym gate test` and waits stand-in delays of 2 to 5 s, leases of 3 s and a 2.5 s sleep; `TestGateTest_DotnetFixture` runs a real `dotnet build` | Shorter injected delays and leases; the lease rows and the SIGTERM row overlap `TestGateRunner`: about 4 to 5 s |
| `internal/battencli` 8.5 | `TestBattenIntegration_Rows` 8.5 s serial; `TestBattenIntegration_GoneTaskWorktree` 4.6 | One serial test of 38 steps, about 20 `AddPair` calls and real `git show` and teardown pushes; it is the package's wall | Template pair for the steps whose slug carries no meaning: about 1.3 s; splitting the steps that do not depend on each other into parallel subtests, at the price of the flow |
| `internal/webstercli` 7.2 | `TestResetCmd` 3.5 s serial; `TestBatchVerbs_RefuseUnderAPause` 1.2 | 28 `newResetFixture` calls, each a pair plus two commits, serial because the test sets `FABRIC_SKIP_GIT`; `newVerbsFixture` runs `git init` and two commits 23 times | A registered template for the reset start: about 2 s |
| `internal/loomcli` 7.1 | `TestLoomFailedReedUpRefuses` 2.1 s; `TestRunOwnerVerdict` 2.0; `TestDecisionParent_ABeamsRecordIsItsOwnOrchs` 1.9 | `newBadReedUpFixture` is `CopyHub`, five seeds, `AddPair` and a real `lym loom` process, three times; `beamHub` is built five times | A beam template: 1 to 2 s |
| `cmd/lym` 6.8 | `TestCrossCompileLinux` 3.5 s; `TestCSharpFixtures_CompileWithDotnet` 3.1; `TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt` 1.0 | A `go build ./...` that is native on a linux host, a `dotnet build` of one fixture and a `go list -test -deps ./...` | Run the first only when a `_linux.go` file changes, the second only when a `testdata` C# file does: about 5 s |
| `internal/testkit/lymbin` 6.7 | The three build tests, 2.3, 2.2 and 1.9 s | Each runs a cold `go build ./cmd/lym` because each blanks the prebuilt variable; `TestBuildInto_UnwritableDirReturnsError` builds a binary only to fail writing it | Fail the write before the build or take a runner seam: 2.3 s |
| `internal/hubreconcile` 6.4 | `TestEnsure_FailedWalkNamesTheFileAndLeavesTheTreeAsItWas` 7.2 s cost over 7 rows | `newStaleHub` is `CopyHub`, `AddPair("pair-a")`, two migrations and eight commits, built 12 times | A stale-hub template: 2 s |
| `internal/gitexec` 5.4 | `TestRun_RemoteBounds` 5.3 s serial | Six rows wait real deadlines of about 1 s each and an alias that runs `sleep 2`; the deadline is injectable and the rows derive their other bounds from it | A deadline of 0.3 s and a 0.4 s sleep: about 3.5 s |
| `internal/beamcli` 4.3 | `TestDobby_ReexecsOntoAReplacedBinary` 3.6 s | A real detached process, a 1.26 MB log rotation and a 1 s poll that needs two sightings | None; the re-exec needs the process |

The packages after `internal/fabricengine` matter to a pass's wall only when the package is below about 35 s, and then at a quarter of a second of wall per second of package time, from the four workers.
The levers above remove about 20 s of package time, 5 s of wall.

Git processes in W3 by package, for the packages that build hubs: `internal/fabricengine` 78,394, `internal/battencli` 7,134, `internal/loomcli` 6,004, `internal/fabriccli` 5,611, `internal/hubreconcile` 4,125, `internal/webstercli` 3,291, `internal/beamcli` 1,631, `internal/gitrepo` 1,515 and `internal/websterengine` 1,496.
32,903 processes in all have no test path and 2,967 have a path no test name matches, which holds the rest of `internal/fabricengine`'s 98.8k.

## 4. Tests that can go, fold or move

### Overlap candidates for `internal/fabricengine`

The coverage report finds no candidate here, as it found none at the survey, so these come from reading the costliest tests.
Each names what it alone pins.
The three marked read were read again today; the others are the survey's rows, found again by name, with today's serial cost.

| Candidate | Cost today | Overlaps | Alone pins | Proposal |
|---|---|---|---|---|
| `TestVerbCases_CleanState`, read | 3.2 s | `TestCrossProduct` runs the same build, arrange, state, capture, run and no-unpermitted-change sequence for every verb in the clean state, at both anchors | three preconditions: Add's launcher blocker is a directory, Checkout's branch differs, Pull's bare advanced | the three checks into `runCell`, then delete the test |
| `TestAnchorChange`, the three refusal rows with no setup, read | 0.4 s of 5.5 | each other | the refusal text for a target the validator rejects, a reserved segment and an untracked one | one hub, and `SharedHub`; the 11 other rows plant state and stay |
| `TestRecordsReplay`, `EndedSet`, read | 0.9 s | five rows that each build a hub with `NewHub` and call `ReadEndedSetForTest` | the ended set of each shape | `CopyHub` for the rows; one hub does not fit, since each row's origin commits differ |
| `TestStoreHousekeeping`, two probe subtests | 1.1 s | the probe rows and the gc-prune row pack the same stores at 0.5 s each | that the default probe packs | one assertion on the gc-prune row, delete two |
| `TestReconcileOrigin_Moves`, `owned_elsewhere` | about 0.6 s | six iterations that differ in the `OwnerOf` reply | the reply per scope | one pair for the six |
| `TestPushFailureClasses`, two settle rows and the two backoff rows | about 0.8 s | each pair rebuilds the same rewound pair or the same refused origin | the hold-back record's class, the error class | assert on the first push, one table for the backoff rows |
| `TestBolt_PullThenCommitWritten`, three pairs of subtests | about 1.0 s | each pair shares an upstream move and a conflict | the record kinds, the way-forward text, the seed commit | three two-row tables |
| The rest of the survey's table: `TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses`, `TestMergeCrucible_ConflictsIsEmptyNeverNil`, `TestRegenerateHubWorkspace` skip rows, `TestPull_WeftDivergedWarpAdvancesCleanly` stop rows, `TestVerbsRegenerateHubWorkspace`, the `AddLocal` rows | about 3 s | the survey's reading | as the survey | as the survey |

Together about 7 s of 156.8 s serial, 4.5 percent.
The survey's estimate of 9 percent came from a traced run where a hub-bearing row cost twice as much; at today's prices a row is 0.13 to 0.25 s.
`TestVerbCases_CleanState` alone is 46 percent of the saving and the only fold worth a task of its own.

### The redundancy report, over twelve packages

`go run ./cmd/testtiming -redundancy -tags tier0,tier1` over `adoptcli`, `battencli`, `beamcli`, `fabriccli`, `gatecli`, `heddlecli`, `hubforge`, `hubreconcile`, `loomcli`, `planglyph`, `preflight` and `webstercli` found 106 candidates.
101 are tier 0 tests, which cost nothing in tier 1.
The five tier 1 candidates, all removable, are `TestRunCLI_CloneDestination` 0.84 s, `TestPairSlugOf` 0.26 s, `TestEnsure_BoardWithNoRemoteCommitsHubWideConfigAndWritesStamp` 0.17 s, `TestEnsure_HeldLockTimesOutWithoutStamping` 0.13 s and `TestPersistentPreRun_OpenFabricWiredButUninvoked` 0.01 s: 1.4 s in all.
Coverage sees blocks and not assertions, so each stays a candidate until read; `TestRunCLI_CloneDestination` is covered by `TestRunCLI_ReconcileBacksFillsWarpBinding` and `TestSandboxedRoleGuard`, whose names are not about clone destinations, so it needs reading before it goes.
The checked-in `docs/benchmarks/test-redundancy.md` was written under the pre-tier tag names of 2026-11-01 and no longer matches the tree.

### Tier 1 tests that need no real git, or could sit in tier 0

Moving a test to tier 0 saves its tier 1 cost and no more, and the tests named are cheap; the list is for a pass over the test when it is touched, not for a task.

- `internal/beamcli`: `TestDobbyLandings_CountsOnlyThisMachine` writes status files and builds no pair; rows 5 to 7 of `TestStart_RefusesBeforeTheReconcile` need no pair and row 6 no git; the `dobbydeps` tests use `AddPair("task1")` only as a geometry key.
- `internal/battencli`: `TestNoticeSite` builds a hub with three pairs to call a pure function, 1.1 s of cost; the park-marker and watched-marker steps of `TestBattenIntegration_Rows` read files.
- `internal/heddlecli`: the store-verb subtests of `TestHeddleCLI` need a Bolt directory, not a hub.
- `internal/webstercli`: `TestBatchVerbs_RefuseUnderAPause`'s `envMasterStrand` row needs no git, and its refusal rows would run on a fake `websterengine.Git`, as `statusFailingGit` already does.
- `internal/hubreconcile`: `TestEnsure_HeldLockTimesOutWithoutStamping` needs a hub only for its geometry.
- `internal/fabricengine`: no tier 1 test above 0.6 s runs no git; the zero-git tests are all under that.

## 5. Sharing one repository

The three shapes, weighed against the price of a hub today: a copy is 17 ms, a pair 67 ms, a layered commit three git processes, and a whole pass makes 1,792 copies, about 1,226 pairs and 22,636 `worktree list` processes.

| Shape | What it saves | What it loses | Where it fits | Verdict |
|---|---|---|---|---|
| Refusal and read-only rows on one shared hub | One `CopyHub` per row, 17 ms, and the row's `AddPair` if the pair is shared, 67 ms | A row that writes poisons the shared hub, which `SharedHub` catches only at cleanup and blames a parallel sharer; a failure names the row that tripped it, not the one that caused it | The three setup-free rows of `TestAnchorChange`; the `EndedSet` rows do not fit; `TestRunOwnerVerdict` writes per row; `TestStart_RefusesBeforeTheReconcile` already shares one hub; the survey's other rows are a few of each table | Take where a fold already touches the test; under 2 percent on its own |
| One longer flow test in place of several tests that build the same start | The build of each test after the first | A first failure hides every later step (`TestBattenIntegration_Rows`, `TestResetCmd` and `TestRunCLI_HubMutationScenario` already stop at it); a step fails because an earlier one left state; one serial test sets the package's wall, as `TestBattenIntegration_Rows` does at 8.5 s | `TestRunCLI_PullHubFromATaskPair`'s two rows, `TestRunHubPull_WaitsForAHeldParentLock`'s two, `TestAdoptIntegration_SecondCloneTakesOver` already one flow; the reconcile-origin tests are a parallel table, which a flow would serialise | Avoid. The flows in the repository already pay this loss, and two of them are their package's wall |
| A template to copy where a test needs a broken start state | The whole build, not one share of it: `CopyHub`, pair and layered commits become one copy | A start state is built once per binary, so a test that wrongly writes to its copy cannot hurt another, but a template built wrongly fails every user of it; builds serialise on one mutex today | `newAnchorHub` plus `AddPair` (13 and 6 sites), `newOriginHub` and its layers (32), `newStaleHub` (12), `beamHub` (5), `newBadReedUpFixture` (3), `newMergePairFixture` and its layers (about 76), `newRecordsFixture` | Take. It is rank 1 of section 2 |

The two cheap forms of the first shape have the same limit: at 17 ms a copy, sharing saves little, and what each row spends is the pair and the layered commits, which only a template removes.
The second shape costs coverage of independence and buys one build per test; the template removes the same build without hiding a row.

## 6. Reads that start git through `gitexec`

`internal/gitrepo/doc.go` keeps writes and remote work on real git, and its read surface already answers `CurrentSHA`, `SHAExists`, `RefSHA`, `RefTree`, `BranchRefs`, `TagRefs`, `Log` and `SubdirsAtRevision` through go-git.
Counts are processes in F1 by the first two arguments, total and the share the trace marks as code; a total includes the production verbs a fixture drives and the gitkit helpers' own reads.
The non-test call sites of `gitexec.Run` and its variants number about 170, found with `grep -rn 'gitexec\.Run' --include=*.go`; the table groups those that read.

| Call and site | Total | Code | Answered by |
|---|---|---|---|
| `worktree list --porcelain`: `fabricengine.List` (`worktreelist.go`), `websterengine` (`gitwrap.go`) | 14,744 | 3,018 | `dotgit`: `.git/worktrees/*/{gitdir,HEAD,locked}` |
| `rev-list --count`: `index.go`; the count also holds `gitrepo`'s `HasUnpushed`, which is on git by decision, and the split is not measured | 2,726 | 299 | go-git first-parent walk, an upper bound |
| `status --porcelain`: `dirtiness.go`, `status.go`, `anchortarget.go`, `snapshotuncommitted.go`, `verifytree`, `websterengine` | 2,268 | 482 | Index reads; go-git's status is slow on a large worktree, and `verifytree` and `websterengine` read the repository's own tree: leave them |
| `config --get`: `recordsgit.go`, `recordsreads.go` | 1,004 | 26 | go-git config read, or `dotgit` |
| `rev-parse --verify`: `destroy.go`, `impactset`, `websterengine` | 991 | 308 | go-git `ResolveRevision` |
| `remote get-url`, `remote`: `reconcile.go`, `add.go` | 2,191 | 247 | config read |
| `diff --name-only`: `originreconcilemoves.go`, `parenttips.go`, `configchanges.go` | 1,134 | 399 | go-git tree diff |
| `for-each-ref --format=%(objectname)`: `archive.go`, `snapshotuncommitted.go`, `reflog.go`, `replay.go`, `destroy.go` | 596 | 276 | `gitrepo.RefSHA`, `TagRefs` |
| `branch --format=%(refname:short)%(worktreepath)`: `destroy.go` | 576 | 232 | refs plus the `dotgit` worktree map |
| `ls-tree`: `recordsprobe.go`, `anchormarker.go`, `anchortarget.go`, `resetsubpathcheck.go`, `upstreamrun.go`, `checkout.go`, `configchanges.go` | 811 | 341 | `gitrepo.RefTree`, `SubdirsAtRevision` |
| `show <rev>:<path>`: `anchormarker.go`, `recordsprobe.go`, `resetsubpathcheck.go`, `configchanges.go` | about 320 | about 130 | a blob read |
| `branch --show-current`, `symbolic-ref`: `clone.go`, `resetsubpathcheck.go`, `addadopt.go` | about 270 | about 25 | `HeadRef`, `CurrentBranch` |
| `ls-files`: `pathtracked.go`, `recordsgit.go` | 2,113 | 257 | Index reads, as `status` |
| `rev-parse --git-path`: `hook.go` | 910 | 40 | `dotgit` geometry |
| `merge-base --is-ancestor` and `rev-list --objects`: `gitrepo` | 762 and 4,230 | 411 and 514 | Stay: the `IsAncestor`, `HasUnpushed` and push paths are on git by decision |
| `ls-remote`, `fetch`: `recordsprobe.go`, `remoteleftover.go`, `cleanupremotewarp.go` | 1,821 and 561 | 457 and 221 | Stay: remote work |

Without the status, ls-files, ancestry and remote rows, the reads go-git or `dotgit` could answer are 14,744 + 2,726 + 1,004 + 991 + 2,191 + 1,134 + 596 + 576 + 811 + 320 + 270 + 910, about 26,000 processes, 27 percent of F1's.
`worktree list` is 56 percent of that and the rest is spread over twenty call sites with one to a few hundred processes each, which is why the report ranks `worktree list` alone and rolls the others into one low-ranked task.
`rev-parse HEAD`, 2,301 processes with 3 coded, is a fixture assertion in `gitkit.RevParse`, not production; `gitkit` could read it with go-git too.

## 7. What the whole would give

### A whole pass

From the list schedule of section 1, fitted to W1.
Only the first and last rows are measured: the 8 s of the second is T66 against W2.

| State | Modelled wall | Scaled to W1's 75.0 s |
|---|---|---|
| Today | 81.9 s | 75.0 s |
| Only the 66 binaries with a tier 1 test (floor 18 s to 10 s) | 73.9 s | 67.7 s |
| Ranks 1 to 7 of section 2 (`internal/fabricengine` 57 s to 36 s) | 60.9 s | 55.8 s |
| Both | 52.9 s | 48.5 s |
| Both, and the other packages' 20 s of package time | 51.7 s | 47.4 s |

### A darn run's gate

Tier 1 passes per run in the now era of the earlier report: 2.2 of `darn-write`, 1.27 of `code-burler-fix` and one of `finalize`, from 6.6, 3.8 and 3.0 recorded tier runs.
Wall of a tier 1 run whose scope includes `internal/fabricengine`, which is 72 of 85 `darn-write` and 55 of 57 `code-burler-fix` runs since 2026-11-05, by the number of other gate runs under its midpoint:

| Key | Alone | One beside | Two or more beside |
|---|---|---|---|
| `darn-write`, computed scope | 110.7 s (22) | 135.7 s (16) | 169.9 s (27) |
| `code-burler-fix`, computed scope | 72.1 s (14) | 100.3 s (21) | 164.3 s (19) |
| `finalize`, all, from the earlier report | 85.9 s | | 142.4 s |

The after is a model, and it is stated so: alone, `internal/fabricengine`'s 21 s come off the wall while it is the critical path, and the computed scope drops a few of the binaries with no tier 1 test; beside two or more runs the wall follows total CPU, and the changes remove about 22 percent of a pass's.

| Per darn run, tier 1 only | Alone, before | Alone, after | Beside two, before | Beside two, after |
|---|---|---|---|---|
| `darn-write` 2.2 passes | 243 s | 187 s | 374 s | 293 s |
| `code-burler-fix` 1.27 passes | 92 s | 65 s | 209 s | 163 s |
| `finalize` 1 pass | 86 s | 62 s | 142 s | 111 s |
| Total | 421 s, 7.0 min | 314 s, 5.2 min | 725 s, 12.1 min | 567 s, 9.4 min |

That is 1.8 minutes a darn run alone and 2.7 beside two others, 4 to 6 percent of a 43-minute run.
The estimate holds as long as the changes land as sized; the folds and the template builds are the least certain parts, within 20 percent either way.

## What carries over to the other machine

No run happened there; this is reasoning from the counts and from what the survey recorded of it.

- **The 130 binaries weigh most.**
  The survey recorded that Cortex XDR holds a binary it has not seen for about ten seconds.
  A tier 1 pass starts 196 test binaries, 130 of which answer `no tests to run`; whether a binary is new on each pass depends on the package and its imports changing, so the share is unknown.
  Every one of those the scanner meets for the first time is a hold that starting only the 66 removes, which on a first-start rate of even a fifth is 26 holds, about 65 s of a four-worker pass, before any test runs.
  One `GIT_TRACE2_EVENT` run there, as the survey proposed, and one `lym gate report` of its launch times, would give the rate.
- **Fewer processes and files carry over and weigh more.**
  Ranks 1, 2, 3, 5 and 6 of section 2 remove process starts and the 22 system files each git process opens before it reads a repository; the survey put those at 1.7 million opens a run.
- **Fewer tests carry over by their share**; sleeps and deadlines in `gitexec`, `termwindow` and `verifyrun` carry over unchanged, since they are real time and not scanned work.
- **Parallelism does not carry over**: `-parallel` and `t.Parallel` hide less where a scanner serialises opens and starts.

## Proposed follow-up tasks, ranked

None is filed.
The order is saving per unit of size, so the small and measured changes come before the large one that saves most.
Savings are of a whole tier 1 pass on neve, quiet, from 75.0 s, unless they say otherwise; each also weighs more on the other machine.
Size is small (under a day for a writer run), medium or large.
Risk to coverage is what the change can stop a test from seeing.

1. **Start only the tier's test binaries.**
   Derive the packages that hold a `tiergate.Require` of the tier, and have the gate's `tier1` run those, plus a guard that a package gaining its first tier 1 test enters the list without a hand edit.
   Saving: 8 s, 11 percent, measured; the largest on the other machine.
   Size: medium, in `internal/gatecli`.
   Risk: none if the list is derived; a hand-kept list could skip a new package, which the guard covers.
2. **`worktree list` through `dotgit`.**
   A reader of `.git/worktrees/*/{gitdir,HEAD,locked}` behind `fabricengine.List` and the `websterengine` call, with a test that checks it against `git worktree list --porcelain` on a repository with a detached, a locked and a prunable worktree.
   Saving: 5 s of the pass, 14.7 serial seconds of `internal/fabricengine`; also fewer processes in every `lym` verb.
   Size: small to medium.
   Risk: none to the tests; the reader's parity with git is the new behavior to pin, `prunable` and `locked` above all.
3. **Registered hub templates, with the largest fixtures moved onto them.**
   A `hubforge` entry that takes a key and a build function, builds the template once per binary under a lock per key, and copies it like a shape; then move `newAnchorHub`, `newOriginHub` and its layers, `newMergePairFixture`, the live-state arrange, `newStaleHub`, `beamHub` and `newBadReedUpFixture` onto it, one package per run.
   Saving: 9 to 11 s of the pass, about 27 serial seconds of `internal/fabricengine`, and 2 to 4 s across `hubreconcile`, `loomcli`, `battencli`, `webstercli` and `beamcli`.
   Size: large; the mechanism is small and each fixture is one edit.
   Risk: a template built wrongly fails every user, and a test that read the template's setup now reads a copy; the tests that exercise `Add` keep building their pairs.
4. **Fold `TestVerbCases_CleanState` into the cross product and take the survey's other overlapping rows.**
   Saving: about 2.5 s, 3.2 serial seconds from the first fold alone.
   Size: medium.
   Risk: low if each fold keeps what its row alone pins, as the table names; the report found no redundancy by coverage.
5. **A slimmer `CopyHub`.**
   No fingerprint walk per copy, a packed template, and no `amend` and `push --force` per copy.
   Saving: 1.5 to 3 s.
   Size: medium.
   Risk: the fingerprint is what catches a write to a shared template, so it stays on `SharedHub`.
6. **Quiet config written from Go in the `gitkit` and `hubforge` repository initialisers.**
   Saving: 2 s, 5.6k processes.
   Size: small.
   Risk: none if `HermeticGitEnv` is the contract for every test binary; check that no test runs without it.
7. **Shorten the waits that are real time.**
   `internal/gitexec` deadline and `sleep 2` (3.5 s of that package), `internal/gatecli` stand-in delays and leases (4 to 5 s), `internal/termwindow`'s 3 s row and `internal/verifyrun`'s 1 s expiries.
   Saving: 1 to 2 s of a pass once `internal/fabricengine` is under 35 s; 10 s of package time.
   Size: small each.
   Risk: a shorter margin raises the flake rate on a loaded machine; the packages' own comments name the margins.
8. **`CopyHub` in the `NewHub` fixtures and the hook script without `dirname` and `basename`.**
   Saving: 1.5 s and 0.7 s.
   Size: small.
   Risk: none.
9. **Test builds that repeat.**
   `TestBuildInto_UnwritableDirReturnsError` builds a binary to fail writing it (2.3 s); `TestCrossCompileLinux` and the dotnet fixture build run on every pass (5 s of `cmd/lym`).
   Saving: 1 to 2 s of a pass, once `internal/fabricengine` is under 35 s.
   Size: small.
   Risk: `TestCrossCompileLinux` guards `_linux.go` files; run it when one changes, and it must still run in the full pass.
10. **Move the tier 1 tests of section 4 that run no git to tier 0, as each is touched.**
    Saving: under 1 s of wall.
    Size: small each.
    Risk: none.

## Limits

- Load: the walls are one run each at a load of 1.8 to 8.7, with a spread of about 30 percent between runs of a command; the counts of tests and processes are exact for their run.
- Per-test cost: a cost sums parallel subtests, so it exceeds the package time wherever tests ran beside each other; the serial run F2 is the one where costs add up to the time.
- Attribution: a process is tied to a test by its repository path; 19 percent of F1's processes have none and are children, and a test whose name is a prefix of another's can take its subtests' processes.
- The model: the list schedule is fitted to one run, reads 9 percent high, and treats a package's time as fixed while the machine's load changes with the schedule; the darn run's after figures apply the pass's reduction to runs whose scope and build cost differ.
- The overlap candidates beyond the three marked read come from the survey's reading; a fold's saving per row is not measured, and the sum is bounded by the rows' own serial costs.
- The other packages' notes come from reading the costliest tests of each, not every test; the readers named what they did not read.
- The other machine: not measured.

## Reproduce

- The gate's records and logs sit in the hub's `.lym/gate/runs`, one `.json` and one `.log` per tier run, and `lym gate report` prints their sums.
- `pertest.sh <log>` prints per-test costs; the package table is `jq -r '.launches[] | [.package, .elapsed_seconds] | @tsv'` over a record.
- `census.sh <trace-dir>` prints the git processes of a traced run; the call table is `cut -f1 procs.tsv | sort | uniq -c | sort -nr`.
- `template-probe_test.go.txt` is the `hubforge` template probe; its header says where to save it and how to run it.
- The redundancy report is `go run ./cmd/testtiming -redundancy -pkg <packages> -tags tier0,tier1 -out <file>`.
