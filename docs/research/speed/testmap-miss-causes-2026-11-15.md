# What sends a coverage-map upkeep pass into whole-package rebuilds, and where its time goes, 2026-11-15

Board task `testmap-miss-causes` follows [the payoff report](testmap-payoff-2026-11-15.md),
which found a pass after a landing takes 7.7 to 31 minutes and that most of it is whole-package rebuilds a selector miss queues.
This report names, on the same three commits, what sent each package to a whole rebuild,
what the note `selector-non-body-miss` would save, and what else takes the pass's time.
The decision on the maps is the operator's.
One machine, one run per commit, no repeats.

## Answer in brief

- **Triggers.**
  Every miss in the three commits is one of these kinds: an embedded file, a type, an added import, a test file or a build tag.
  No function body, signature, constant or variable change caused a miss; a function body or signature change is selected, not missed.
  The small commit's 54 misses are all one changed embedded file (1745 s).
  The medium commit's 7 are all one added import (825 s).
  The large commit's 119 are all one changed type, `configengine.Spec`, as the first miss (2253 s),
  and 54 of them also see a changed embedded file, an added method and three more changed types that the first miss hides.
- **What the note would save.**
  Following the names a changed type is referenced by would narrow 0 of 54, 0 of 7 and 64 of 119 rebuilds.
  Of the 64, 60 become a targeted re-measure or none, and 4 stay whole.
  On the large commit that is 630 s of rebuild falling to about 103 s, 527 s (23 percent) of that pass's 2255 s.
  The 55 other packages that miss see a trigger the note does not cover.
- **What else cuts the pass.**
  The per-round bookkeeping around each test run takes 1343 s of the small pass's 1751 s (77 percent), 760 of 933 s and 1709 of 2255 s.
  Building the coverage binaries takes 123, 38 and 158 s; running the tests takes 251, 120 and 331 s.
  The bookkeeping is a git status, a re-keying of every covered block and a rewrite of the whole map file after each round, 1.2 s a round on average.
  This is larger than the selector's share on the small and medium commits, and as large as the type miss on the large one.
- **The machine was not quiet.**
  Other work held 3 to 8 cores of 12 during the three runs, and the load average peaked at 22.6, 43.2 and 23.6.
  The medium run's 933 s against 459 s in the payoff report is probably load, in part; see Load.

## Inputs and method

The commits are those of the payoff report: small `4df9f81c`, medium `79f0eb4a`, large `36621f0e`.
The maps are a copy of `<bolt>/.lym/testcover/`, taken today, 211 package maps.
Nothing under Bolt was written, rebuilt or removed.

A scratch program, not committed, replays a pass.
It is the code of `internal/gatecli`'s `lym gate testmap` copied into a scratch module, driven by its real planner and measure loop:
`testcover.Selector`, `testcover.PlanUpkeep`, `Toolchain.LinkedPackages` and `BuildBinary`, the rounds of `RunAlone`, the self-check, `fillMapTests` and `writeMap`.
The copy differs from the production code in these ways:

- Each copied map is first rewound to the commit's parent: commit, file hashes, linked-package hashes and the environment stamp come from the parent's tree.
  The entries keep the coverage measured at `343405cf`, as in the payoff report.
- HEAD is pinned to the commit, so the plan runs against the commit's tree.
  The test declarations, the spawn scan and the builds read the worktree at `055ef994`, a few commits later than the maps.
- It holds no slot, never yields, runs jobs in import path order and skips the closing audit.
- It times each step of a job: `go list`, the build, the rounds, and the status, keying and write after each round.

To name every trigger of a package, and not only the first the selector reports,
the program sets each reported file as unchanged in a copy of the map and selects again until no miss remains.
The first miss is the selector's own; the rest are what that first miss hides.

## Q1. What triggers the misses

The rebuild seconds of a package are its job's wall in the replay: `go list`, build, the rounds and the bookkeeping.
`first` counts a package once, under the kind the selector reports.
`seen` counts it under every kind it sees.

| Kind | Small first (pk, s) | Small seen | Medium first | Medium seen | Large first | Large seen |
|---|---|---|---|---|---|---|
| function body | 0 | 0 | 0 | 0 | 0 | 0 |
| signature | 0 | 0 | 0 | 0 | 0 | 0 |
| type | 0 | 0 | 0 | 0 | 116 pk, 2024 s | 119 pk, 2253 s |
| method set | 0 | 0 | 0 | 0 | 0 | 54 pk, 1617 s |
| constant | 0 | 0 | 0 | 0 | 0 | 0 |
| variable | 0 | 0 | 0 | 0 | 0 | 0 |
| embedded file | 54 pk, 1745 s | 54 pk, 1745 s | 0 | 0 | 0 | 54 pk, 1617 s |
| build tag | 0 | 0 | 0 | 0 | 1 pk, 41 s | 1 pk, 41 s |
| test file | 0 | 0 | 0 | 1 pk, 46 s | 0 | 4 pk, 80 s |
| other: added or removed import | 0 | 0 | 7 pk, 825 s | 7 pk, 825 s | 2 pk, 187 s | 2 pk, 187 s |
| Total packages and seconds | 54 pk, 1745 s | | 7 pk, 825 s | | 119 pk, 2253 s | |

A function body or signature change is never a miss: the selector compares function declarations whole and selects the tests that covered a changed one.
A method's signature change is a method-set miss.
The rows are zero because none of the three commits changed a constant or variable declaration outside a function in a way that missed.

What each commit changed, by the file and the declaration the selector names:

| Commit | Change | Packages that see it |
|---|---|---|
| small | `internal/orchengine/template.yaml` changed; a `//go:embed` line in `internal/orchengine/template.go` names it, and `inert_paths` does not exempt it | 54 |
| medium | `internal/gatecli/redundant.go` adds an import of `github.com/spf13/cobra`, absent from those maps' linked sets | 7 |
| medium | `internal/gatecli/redundant_integration_test.go` adds an import of `github.com/google/go-cmp/cmp`, absent from the map's linked set | 1, the same package that also sees the cobra import |
| large | `internal/configengine/layers.go` changes `type Spec`: a `MigrateLayer` func field | 119 |
| large | `internal/dobbyengine` changes `type State`, `type Deps` and `type Config` | 54 |
| large | `internal/dobbyengine/template.yaml` changes, embedded | 54 |
| large | `internal/dobbyengine` adds the method `daemon.noteTestMapExit` | 54 |
| large | `internal/beamcli/dobbydeps.go` drops the import `strconv`, which a kept file may not do without a miss | 2 |
| large | `internal/fabriccli/anchorchild_linux_test.go`, a new test file, opens with `//go:build linux` | 1 |
| large | four test files change a helper, a const or an `export_test.go` function that is no `Test` function | 1 each |

Combinations the large commit's 119 packages see:

| Triggers seen | Packages | Rebuild seconds |
|---|---|---|
| type | 64 | 630 |
| type, embedded file, method set | 50 | 1378 |
| type, test file | 1 | 5 |
| type, embedded file, method set, test file | 1 | 10 |
| type, embedded file, method set, import | 1 | 163 |
| type, embedded file, method set, import, test file | 1 | 23 |
| type, embedded file, method set, build tag, test file | 1 | 41 |

The 54 maps that miss on the small commit's embedded file include every package whose test binary links `orchengine`.

## Q2. What `selector-non-body-miss` would save

The note proposes, for a changed declaration that is not a function, to select the tests that reach the declaration's references rather than rebuild the package.
The selector today (`internal/testcover/testselect.go`, `declcompare.go`) misses at the first changed `const`, `var` or `type`, method-set change, added import, `//go:` line, non-Go file or test-file change outside a `Test` function.
`internal/gatecli/testmapplan.go` turns a miss into a whole rebuild job.

### What the estimate does

For each missed package whose every trigger is a changed `type`, `const` or `var`, the estimate follows the changed names:

1. It lists the changed declaration names of each trigger file by printing each top-level declaration of the commit and its parent and comparing them.
2. It parses every non-test Go file of the packages the map links, and the package's own test files, at the commit.
   A function that mentions a changed name is a referencing function:
   a bare identifier in the declaring package, a qualified one through an import in any other package, or a method on the changed type.
3. A type, const or var declaration that mentions a changed name is itself changed, repeated until nothing grows.
4. A test is re-measured when the map records it covering a referencing function; a `Test` function that mentions a changed name is re-measured whatever the map says.
5. The package's cost then is its `go list` and build, the re-measured tests' wall, one probe run and a share of the bookkeeping.
   A package with no re-measured test costs nothing: its map moves to the commit.
6. A package that cannot be followed, because a test helper or a package-level initializer mentions a changed name, stays a whole rebuild.

The estimate reads syntax, not types.
Any identifier of the same spelling counts as a reference, which over-counts.
A use that never spells the name, such as `x := f()` for a function `f` that returns the type, or a struct field read through a value, does not count, which under-counts.
Interface satisfaction is not followed.
The estimate therefore bounds nothing; the build would need type information to be sound.

### Result

| Commit | Missed packages | Narrowable (every trigger a type, const or var) | Of those: no test, targeted, left whole | Whole seconds | Targeted seconds | Saved |
|---|---|---|---|---|---|---|
| small | 54 | 0 (all embedded file) | | | | 0 s |
| medium | 7 | 0 (all added import) | | | | 0 s |
| large | 119 | 64 | 30, 30, 4 | 630 s | 103 s | 527 s (23 percent of 2255 s) |

The 64 packages see only `configengine.Spec` changed.
A package has 46 to 81 referencing functions.
210 of the 2055 recorded tests of those packages are re-measured, 10 percent.
The four left whole are `internal/shuttleengines`, `internal/standalonegeom` and `tools/tokencount`, whose package-level `registry` initializer mentions `Spec`, and `internal/reviewbudget`, whose test helper `writeLayer` does.

The other 55 large-commit packages stay whole under the note: the 50 that also see the embedded `template.yaml` and the added method, and 5 that see a test-file, import or build-tag trigger.
Those triggers take 1623 s of the large pass.
Adding the added method as no-reference (a new name unchanged code cannot mention) does not change the count, since the embedded file blocks the same 50.
The note says nothing of an embedded file, an added import or a test file, which are the whole small and medium misses.

## Q3. What else takes the pass

Seconds of each commit's pass, summed over its jobs.
Plan is the plan phase before the first job.

| Step | Small | Medium | Large |
|---|---|---|---|
| Jobs run | 56 | 16 | 120 |
| Plan | 24 | 36 | 31 |
| Pass wall after the plan | 1751 | 933 | 2255 |
| `go list` of linked packages | 4 | 1 | 7 |
| Build the coverage binary | 123 | 38 | 158 |
| Rounds of test runs (wall) | 251 | 120 | 331 |
| Bookkeeping after each round | 1343 | 760 | 1709 |
| Build `lym` once for tier 1 | 2 | 4 | 3 |
| Bookkeeping share of the pass | 77 percent | 81 percent | 76 percent |

The jobs are 56, 16 and 120 because the queue also holds a few packages whose tests the worktree at `055ef994` declares and the maps lack; see Limits.

### Bookkeeping

After each round the pass reads the worktree status and HEAD, keys every block of the profile and names every covered function of each linked package,
merges that into the package's map, and writes the whole map file.
A round is one test, or up to four tests that call `t.Parallel()`.
The small pass ran 1112 rounds for 1976 tests, 1.2 s of bookkeeping per round on average.

Split of that time on three packages of the small commit, each rebuilt alone in the same program:

| Package | Job | Rounds | Status and HEAD | Key blocks and covered functions | Write the map |
|---|---|---|---|---|---|
| `cmd/lym` | 115 s | 68 | 15 s | 68 s | 3 s |
| `internal/loomcli` | 268 s | 134 | 28 s | 151 s | 57 s |
| `internal/orchengine` | 37 s | 88 | 18 s | 7 s | 4 s |

Keying and naming covered functions is 54 percent of those jobs, the map write 15 percent and the status read 15 percent.
The build and the test runs are the rest, 16 percent.
The map write grows with the map: `loomcli`'s map holds the most tests and linked functions of the three.
Doing the bookkeeping once per package after its last round would leave one such step per job, 56 in the small pass, instead of 1112.
At the average above that is about 67 s instead of 1343 s; this is an estimate from the averages, not a measurement of such a change.

### Test runs and builds

- Each test runs in its own process.
  The processes' wall summed 424, 209 and 554 s against 79, 32 and 89 s the test binaries report for their tests,
  which leaves about 0.17 s of process start, coverage-profile write and profile parse per test.
- Of the small pass's 1112 rounds, 809 ran one test alone (134 s of wall) and 303 ran several side by side (116 s).
  The large pass ran 1713 single rounds of 2299 (188 s of 331 s).
  A test that does not call `t.Parallel()` can only run alone.
- Two tests of the small pass ran over 5 s, and the longest ran 14.6 s; the large pass's longest was 14.7 s.
  No test runs long enough to dominate a package.
- The build cost depends on the build cache.
  One coverage build of `internal/orchengine`, 49 linked packages, took 13.0 s on an empty cache (76 s of CPU), 6.0 s once after the pass, and 0.3 s on a second run.
  These replays ran in that order on one cache, so the later commits found it warm.
  A pass on a cold cache would add compile time that these replays do not hold.
- The pass uses a small share of the machine:
  its children took 0.35, 0.23 and 0.35 CPUs on average, and its own process took about 0.9 CPUs more in the one sample taken of it.
  Of 12 cores it is the one serial job at a time, in one gate slot.

### Package count

The pass's fixed cost per job, `go list` plus build, averages 2.3 s on the small pass (127 s over 56 jobs), 2.5 s on the medium and 1.4 s on the large.
The package count matters through the bookkeeping, which grows with each package's rounds, more than through the per-job cost.

## Load

The load was not quiet, and the bound asked for a quiet machine.
Machine-wide busy cores over the measure phase, from `/proc/stat` every 5 s, against the cores the pass's children used:

| Run | Wall | Machine busy | Pass children | Load average at start, mean and peak |
|---|---|---|---|---|
| small | 1770 s | 4.8 cores | 0.35 | 1.4, 7.3, 22.6 |
| medium | 965 s | 8.5 cores | 0.23 | 4.8, 19.2, 43.2 |
| large | 2285 s | 3.8 cores | 0.35 | 19.5, 5.5, 23.6 |

The program's own in-process work, about 0.9 CPUs by one `ps` sample, is part of machine busy and not of the children's column.
So other work held about 3.6 cores during the small run, 7.4 during the medium and 2.6 during the large.
Other agents' gate runs were the likely source; the replay did not check.
The payoff report recorded 0.5 to 3.2 CPUs of other work.
The medium run is the one most affected: its 7 whole rebuilds took 825 s, where the payoff report's replay of them took 438 s.
The small and large totals, 1745 s and 2253 s, sit within 17 percent of the payoff report's 1497 s and 1877 s.

## Limits

- One run per commit.
  The tables hold what that run saw under the load above, not a mean.
- The builds and the test declarations come from `055ef994`, and the maps' entries from `343405cf`; the commits are earlier.
  Packages that changed since, notably `quarrycli`, `selfreportcli` and, on the medium commit, `battencli`, `burlercli`, `configcli`,
  `preflightshed`, `shuttlecli`, `webstercli` and `testcover`, queued extra jobs for tests the maps lack.
  They cost 6, 108 and 2 s, are no part of the miss rows, and are in the pass totals.
  `internal/shedcheck` exists at the commits and not at `055ef994`; its job failed at once.
- Jobs run in import path order, not in the order the gate's run records give.
  The closing audit and the slot wait are not replayed.
- The Q2 estimate reads syntax and over- and under-counts as stated there.
  It also uses the commit's own rewound maps, whose entries date from `343405cf`.
- The cold-cache build is one package, measured once, and the machine was not idle.
- The split of the bookkeeping is from three packages of the small commit, one run each.
- The self-check, which a whole rebuild skips, found 2, 11 and 2 mismatches in the non-whole jobs.
