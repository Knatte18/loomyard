# Building and keeping per-test coverage maps cheaply: five directions weighed, 2026-11-15

Board task `testmap-smart-build` asked how LoomYard can build and keep per-test coverage maps cheaply enough to pay
for themselves, for the gate's test selection and for `lym gate redundant`.
This report weighs the five directions the task names, by reading the code, the earlier reports and Go's sources,
and by three probes on one package.
It ends in one recommended design; the decision is the operator's.
One machine with 12 CPUs, Go 1.26.8, one run per probe, at commit `055ef994`.

## Answer in brief

- The maps are slow because of `lym`'s own bookkeeping after each test, not because each test runs in its own process
  and not because of Go's coverage.
  For the probe package, `internal/loomcli`, today's measure loop took 239 s for 259 tests,
  and 205 s of that named the covered functions of the linked packages.
  It reads the whole profile once per linked package, 157 times per test.
- The tests themselves are a small part.
  The stored maps hold 5,327 entries whose own run times sum to 86 s; the first build of the set took 44 minutes.
- A process per test is cheap once the bookkeeping is gone: the 259 tests ran in 5.2 s, eight at a time.
  One process with a snapshot after each test took 3.3 s.
  It saves 2 s for the package and loses the code that runs once per process:
  84 of the 259 tests lacked the logger's first-use set-up in their record.
- Static reachability is too coarse and too slow here.
  Every test reaches the same 7,609 functions, 74 percent of what the binary links and 70 times what the median test
  covers, and the analysis took 2.0 s per test.
- Recording during ordinary gate runs cannot name a test:
  the counters are process-wide and the tests run in parallel.
  For a whole package it costs nothing measurable at run time: 1.40 s and 1.61 s with coverage against 1.57 s without.
- Recommended: keep one process per test and Go's block coverage, and make the measure loop lean.
  A full build is then estimated at 16 CPU-minutes, about 4 minutes at four tests side by side, against 44 minutes.
  Build the maps on command, run no pass per landing, and leave the gate's selection off until its saving on
  importing packages is measured.

## What the maps cost today

Read from the hub's stored maps, `<bolt>/.lym/testcover/`, today; nothing was written there.

| Measure | Value |
|---|---|
| Maps | 211 |
| Test entries | 5,327 |
| Entries with a passing measurement | 4,628 |
| The entries' own run times, summed | 86.4 s |
| Slowest package by that sum | `internal/fabricengine`, 27.9 s over 574 entries |
| Linked in-module packages per map, mean | 47 |
| Wall time of the build of the set, 2026-11-12 | 44 minutes, from the [pilot report](../coverage-map-pilot-2026-11-12/report.md) |

The tests' own time is about 3 percent of the build.
The [payoff report](../testmap-payoff-2026-11-15.md) measured a pass after one landing at 7.7 to 31 minutes,
nearly all of it whole-package rebuilds, at 0.5 to 0.9 s per test.

The hub's `gate.yaml` gives every gate key the scope `card`, `diff` or `all`.
No key has the scope `coverage`, so no gate of this hub selects tests from the maps today.

## Method

A scratch Go program, not committed, ran the probes from the task worktree.
It is a module outside the tree that imports `internal/testcover`, so it builds and measures with the real
`testcover.Toolchain`, `testcover.ParseProfile`, `testcover.KeyBlocks` and `testcover.FuncIndex`.
It changed no file of the repository and wrote nothing under Bolt.
Its plumbing was first checked on `internal/agentname` with two tests; no figure in this report comes from that check.

The probe package is `internal/loomcli`: 259 tier0 tests, of which 171 declare `t.Parallel`,
and a test binary that links 158 in-module packages.
All three probes ran its tier0 tests under `GOFLAGS=-trimpath` and `LYM_TEST_TIER=0`.
The results are in [probe-1.json](probe-1.json), [probe-2.json](probe-2.json) and [probe-3.json](probe-3.json).

1. **Per process against one process.**
   - *As today*: the coverage binary in `set` mode over the linked packages, each test alone in its own process with
     `-test.coverprofile`, in rounds of up to four tests that declare `t.Parallel` and any other test alone,
     then the calls `lym gate testmap` makes after a round: `KeyBlocks` once and `FuncIndex.CoveredFuncs` for each
     linked package and each run.
     It leaves out the slot, the worktree status and HEAD reads and the map write.
   - *Lean*: the same binary and one process per test, eight at a time whatever the test declares,
     with `-test.gocoverdir`, which writes Go's raw counter files; `go tool covdata textfmt` then renders each as text.
   - *One process*: the binary rebuilt in `atomic` mode, with a hook overlaid into `tmuxkit.Main`.
     The hook calls `m.Run` once per top-level test and, after each, `runtime/coverage.WriteCountersDir`
     and `ClearCounters`.
     The overlaid package, `internal/testkit/tmuxkit`, is left out of that build's coverage
     and out of every arm's block sets.
2. **Static reachability.**
   The package and its tests loaded with `go/packages`, built to SSA, and analysed with `rta`,
   the analysis the `PATTERN-test-only-code` guard runs.
   Each test was analysed alone, rooted at the test, `TestMain` and every package initializer,
   and compared with the functions the same test covered in the as-today arm.
3. **Recording during an ordinary run.**
   The package's tier0 tests run once as a whole, the way a gate runs them, with a plain test binary,
   the `set` coverage binary and the `atomic` coverage binary, the last two writing their counters at exit.

The machine's one-minute load average was 4 at the start of probe 1 and 8 to 18 during its arms,
from the probe itself and the hub's other runs; it was 4 before probe 3 and rose from 5 to 22 during probe 2.

## Measurements

### 1. Per process against one process

| Arm | Wall for the 259 tests | Per test |
|---|---|---|
| As today | 238.6 s | 0.92 s |
| Lean, one process per test, eight at a time | 5.2 s | 0.020 s |
| One process, a snapshot per test | 3.3 s | 0.013 s |

The as-today wall holds the parse and the bookkeeping.
The other two walls are the runs alone; turning their data into block sets is in the third table below.

Where the as-today arm's time went:

| Part | Seconds | Note |
|---|---|---|
| Naming the covered functions and keying the blocks after each round | 205.5 | 86 percent of the arm |
| The 131 rounds: processes and the profile parse | 33.1 | |
| of which processes, summed | 37.6 | 0.145 s each, the test's own time included |
| of which profile parse, summed | 28.4 | 0.11 s each |
| The tests' own time, summed, as the one-process arm reports it | 3.1 | |

Each test's text profile is 6.1 MB, 77,732 block lines over the 158 packages.
`fillMapTests` in `internal/gatecli/testmappackage.go` calls `FuncIndex.CoveredFuncs` for each run,
once per linked package,
and each call filters the whole profile through `testcover.PackageBlocks`.
That is 157 passes over 77,732 blocks per test.
The pilot report saw this from outside, as 2.6 s between rounds on a package that links 144.

The lean and one-process arms' other costs:

| Cost | Lean | One process |
|---|---|---|
| Processes, summed | 40.3 s, 0.156 s each | one process, 3.3 s |
| Raw data per test, in the one directory looked at | a 1.6 kB counter file beside the binary's 870 kB meta-data file | a 0.4 kB counter file |
| `go tool covdata textfmt` for all 259, eight at a time | 7.4 s | 7.1 s |
| Parsing the 259 text profiles with `testcover.ParseProfile`, summed | 32.9 s | 26.9 s |
| The 259 snapshots, summed | none | 0.066 s |
| Failed tests | 0 | 0 |

What each arm recorded, against the as-today arm:

| Compared | Tests with the same blocks | Tests with the same functions | Difference |
|---|---|---|---|
| Lean | 254 of 259 | 257 of 259 | 4 blocks missing and 6 extra over five tests; two tests add `lymtemp`'s `ignoreNotExist` |
| One process, with the 231 blocks start-up covers added to each test | 175 of 259 | 175 of 259 | 1,000 blocks and 415 functions missing over 84 tests, 2 blocks extra |

Every function the one-process arm misses is one of five in `internal/logger`:
`armDurableSinkLocked`, `resolveSinkDirLocked`, `adoptOrMintTraceID`, `mintTraceID` and `resolveTraceID`.
They are the logger's first-use set-up, which `sinkArmed` runs once per process.
The first test that logs gets them in its record, and the 84 later tests that log do not.

Build times of the package's test binary, each built once more after the first:

| Build | First | Again |
|---|---|---|
| Coverage binary, `set` mode, as `lym gate testmap` builds it | 3.3 s | 3.2 s |
| Coverage binary, `atomic` mode | 17.0 s | 3.4 s |
| Plain test binary | 2.1 s | not run |
| `go list` of the linked packages | 0.14 s | not run |

The `set` build found its instrumented packages in the build cache.
The `atomic` build compiled the 157 linked packages afresh,
so 17.0 s is the cost of instrumenting that set from nothing.

### 2. Static reachability

| Measure | Value |
|---|---|
| Functions of the linked packages' production files | 10,347 |
| Functions some test covered when run alone | 2,301 |
| Functions the initializers and `TestMain` reach statically, before any test | 7,609 |
| Functions one test reaches statically: 10th percentile, median, 90th, largest | 7,609 each |
| Functions one test covered: 10th percentile, median, 90th, largest | 78, 108, 245, 797 |
| Static over covered, per test: 10th percentile, median, 90th | 30, 70, 98 |
| Tests a changed covered function selects, mean | 16.4 by coverage, 259 by reachability |
| Covered functions the static set lacks | 0 |
| Loading the package and its tests, building SSA | 1.8 s, 0.8 s |
| One test's analysis: median, 90th percentile | 2.0 s, 3.2 s |
| All 259 analyses, four at a time | 135 s |
| One call graph over every test, then a walk per test | 1.8 s, then 31 s, 7,412 functions per test |

The static set is sound here, and it tells no two tests apart:
the initializers and `TestMain` alone reach the 7,609 functions, and no test adds one.
Rapid type analysis joins a call through a function value to every function of that signature whose address is taken.
That is the likely cause; the probe did not trace it.

### 3. Recording during an ordinary run

| Binary | Wall for the package's tier0 tests, run as a whole | Data written |
|---|---|---|
| Plain | 1.57 s | none |
| Coverage, `set` mode | 1.40 s | 0.9 MB |
| Coverage, `atomic` mode | 1.61 s | 0.9 MB |

The three walls differ by less than the machine's load moves them.
Such a run records which functions the package's tests reached together.
It cannot say which test reached them, since the counters are process-wide and 171 of the 259 tests run in parallel.

### What Go 1.26.8 allows

Read from the toolchain's sources under `GOROOT`, and from the probes.

| Fact | Source |
|---|---|
| `runtime/coverage`'s `WriteCountersDir`, `WriteCounters` and `ClearCounters` refuse a binary not built in `atomic` mode | `internal/coverage/cfile/apis.go` |
| In a test binary the write functions also fail until the meta-data hash is computed, which happens when the first `m.Run` ends | `internal/coverage/cfile/hooks.go`, `emit.go`; `testing.M.after` |
| `m.Run` may be called more than once, a use the `testing` package tolerates and did not intend | `testing/testing.go`, the comment citing go.dev/issue/23129 |
| The build cache keys an instrumented package by the coverage mode and the cover tool, not by the `-coverpkg` list | `cmd/go/internal/work/exec.go`, `buildActionID` |
| `cmd/cover` knows a per-function counter grain, and the go command always asks for per-block | `cmd/go/internal/work/exec.go`, `writeCoverPkgInputs` |
| `-coverprofile` is a cacheable test flag; `-exec` turns the test cache off | `cmd/go/internal/test/test.go` |
| `-overlay` cannot add a file to a package under coverage: the cover tool opens the real path | the plumbing check's first build, which failed that way |

The one-process arm got past the second row by calling `m.Run` once with a pattern that matches no test.

## The directions weighed

Costs per landing use the payoff report's three commits: 55, 10 and 119 packages rebuilt whole, holding 1,959, 523 and
3,975 tests.

| Direction | Cost per landing | Cost per full build | Fresh | Exact | Needs from Go |
|---|---|---|---|---|---|
| 1. Record as tests run | no pass; a second, instrumented compile of what changed | none | at every run that records | per package only, never per test | a way to keep each package's counters through `go test`, not established |
| 2. One process, snapshots | as the lean loop, less about 0.13 s of start-up per test | about 10 CPU-minutes, estimated | as the pass that runs it | misses code that runs once per process | `atomic` mode, a hook in the test main, repeated `m.Run` |
| 3. Static reachability | no pass; 35 to 135 s of analysis per package when the gate selects | nothing is built | always, it reads source | sound, and 70 times too wide | `golang.org/x/tools`, already a dependency |
| 4. Cheaper instrumentation | no change | no change | no change | no change | a per-function grain the go command does not offer |
| Lean loop, one process per test (recommended) | 0.4 to 6 minutes if a pass runs, estimated | about 16 CPU-minutes, estimated | as the pass that runs it | as today | nothing new |

**1. Record as tests run.**
It cannot produce per-test maps: Go has one set of counters per process, and attributing a block to one of several
parallel tests and their goroutines would need instrumentation Go does not have.
At package grain it is nearly free at run time (probe 3).
A package-grain record could tell the gate which importing packages can reach a changed function at all.
It cannot serve `lym gate redundant`, which compares tests.
What it would need is not established:
the gate's test binaries become coverage builds, and each package's counter file must survive `go test`,
which writes it to a work directory it deletes.
An `-exec` wrapper could move the file out, and `-exec` turns the test cache off.
It would change the tier commands in `gate.yaml` and add a record beside the maps; `internal/testcover` would read it.

**2. One process, snapshots between tests.**
It works, through the hook and the repeated `m.Run` named above, and a snapshot costs 0.26 ms.
What it saves is each process's start-up, 0.13 s per test for this binary.
That is 2 s of wall for the package at eight at a time.
What it costs is exactness: code that runs once per process is recorded for the first test that reaches it.
The gate would then leave out a test that depends on such code when it changes.
It also takes a second, `atomic` build, a hook in the shared test main, and a use of `m.Run` Go does not promise.
The saving is small beside the 205 s that are bookkeeping.

**3. Static reachability.**
Sound for the probe package and useless for selection: a changed function selects every test.
It is also slower than measuring, at 2.0 s per test against 0.15 s for a process that runs the test.
A finer analysis than rapid type analysis was not probed.
`golang.org/x/tools` offers `vta`, which refines such a graph by type flow; its cost and yield here are unknown.

**4. Cheaper instrumentation.**
Instrumentation is not where the time goes: the coverage binary builds in 3.3 s against 2.1 s plain, and runs as fast.
The go command offers no per-function grain.
A narrower `-coverpkg` would shrink the 6.1 MB profile and drop the linked packages' functions the selector needs.
Build-cache reuse across packages is already there:
the cache key of an instrumented package ignores the `-coverpkg` list.

**5. Combinations.**
Static reachability for the gate with coverage on command for cuts fails on the static half, as above.
The combination the measurements support is coverage per test on command for cuts,
and for the gate either that same map kept by a lean pass or a package-grain record from ordinary runs.

## Recommended design

Keep the method: one process per test, Go's block coverage in `set` mode, the map as it is.
Change the measure loop, and when it runs.

1. **Read each profile once.**
   Group a profile's blocks by package in one pass and hand each `FuncIndex` its own blocks.
   This removes 86 percent of the probe's as-today time.
   It changes `FuncIndex.CoveredFuncs` and `ProfiledFuncs` in `internal/testcover`,
   and `fillMapTests` and `selfCheck` in `internal/gatecli`; the map's contents stay the same.
2. **Run every queued test side by side, up to the slot's parallel count.**
   Today a test that does not declare `t.Parallel` runs alone, which made 131 rounds of 259 tests.
   Each test has its own process, so its environment and working directory are its own.
   In the probe all 259 passed eight at a time, and 254 recorded the same blocks.
   A test that fails beside others is run once more alone before its failure is recorded.
   This changes `nextRound` and `runRounds` in `internal/gatecli`.
3. **Write the map and read the worktree once per package, not once per round.**
   The probe did not time these steps; time them once steps 1 and 2 are in, and keep the per-round form if it is cheap.
4. **Build the next package's binary while the current package's tests run.**
   A build is a link of 0.1 to 3.3 s once the instrumented packages are cached, and builds run one after another today.
5. **Build on command, as note `testmap-on-command` asks.**
   `lym gate testmap <package>` before `lym gate redundant <package>`; the whole set before a sweep.
   The dobby's pass stays off, and nothing in `internal/dobbyengine` is added.
6. **Leave the gate's selection off until its saving is measured.**
   Replay the selector, plan only, over the diffs of a week's real gate runs,
   and sum the run time of the importing packages it would leave out.
   A plan takes 3 s and builds nothing.
   Turn a pass per landing back on only if that saving exceeds the pass's cost below.

Expected cost, estimated from probe 1 by scaling with the number of linked packages,
over the 201 packages and 5,418 tests of the pilot's `packages.tsv`:

| Part | Basis | CPU |
|---|---|---|
| Process start-up and profile write | 0.133 s per test at 158 linked packages | 365 s |
| The tests' own time | the stored maps' sum | 86 s |
| Profile parse, with today's parser | 0.11 s per test at 158 linked packages | 304 s |
| Coverage builds, instrumented packages cached | 3.3 s per package at 158 linked packages | 210 s |
| Full build | | about 965 s, 16 CPU-minutes |

| | Today, measured | Lean loop, estimated |
|---|---|---|
| Full build | 44 minutes | 16 CPU-minutes; about 4 minutes at four tests side by side |
| One package of `internal/loomcli`'s size | 239 s for its tier0 tests | about 20 s at four side by side |
| Per landing, built on command | no pass | no pass |
| A pass after the small commit, 55 packages rebuilt whole | 25.0 minutes | 1.4 to 2.9 minutes at four side by side |
| A pass after the medium commit, 10 packages | 7.7 minutes | 0.4 to 0.7 minutes |
| A pass after the large commit, 119 packages | 31.3 minutes | 2.8 to 5.9 minutes |

The low end of each pass uses the set's mean cost per test and per package, 0.14 s and 1.0 s of CPU.
The high end uses `internal/loomcli`'s, 0.26 s and 3.3 s, since the packages a miss rebuilds link more than the mean.
A first build after the instrumented packages left the cache adds about 17 s.

The payoff report measured the gate time the maps saved on the packages a diff touches at 0 to 4.2 s per commit.
A lean pass still costs more than that.
The saving on importing packages, which is where a selection leaves whole test binaries unbuilt and unrun,
has not been measured, and step 6 is that measurement.
Its ceiling is the run it narrows: a whole-suite run of tier0 and tier1 took 22 s and 126 s on a warm cache
([gate launches report](../gate-fewer-launches-2026-10-31/report.md)).

Board task `testmap-miss-causes` is measuring what sends a pass into whole-package rebuilds.
Its result multiplies with this one:
it lowers the number of packages a pass rebuilds, and this lowers the cost of each.

## Open risks

- **Tests side by side in separate processes.**
  The probe ran tier0 tests of one package.
  A tier1 test that spawns git or `lym`, or two tests that share a path outside their temporary directories,
  may fail or record differently beside others; the retry alone bounds a failure, not a changed record.
- **Run-to-run differences.**
  Five of 259 tests recorded a few blocks differently between two per-process runs.
  The audit round counts such differences today; a lean loop keeps it.
- **The estimate's scaling.**
  Start-up, parse and build costs are assumed proportional to the linked package count, from one package.
  The slot, the worktree reads and the map writes are not in it.
- **The parse.**
  After step 1 the profile parse is the largest part left, 0.11 s per test for a 6.1 MB profile.
  A faster parser, or reading the 1.6 kB counter file in place of the text, would cut it;
  the counter file's format is internal to Go.
- **The gate's saving stays unknown.**
  If step 6 finds a small saving, the maps pay for the redundancy cut alone, built on command.
- **A package-grain record from ordinary runs** was not built or probed past its run-time cost.
  It is the only direction with no pass at all, and it answers the gate's question only.

## Limits

- One package, one run per arm, on a machine whose load rose during the probes.
  The as-today arm's 205 s of bookkeeping is single-threaded work, which load inflates less than it inflates the rounds.
- The as-today arm repeats the calls of `fillMapTests`; it does not run `lym gate testmap` itself.
- Tier0 tests only; the package's tier1 tests need a prebuilt `lym` and were left out.
- Probe 2 rooted each test with every package initializer and `TestMain`, as a sound analysis must.
  An analysis rooted at the test alone was not run.
- The static and one-process findings are those of one package.
  Another package may hold more code that runs once per process, or none.
