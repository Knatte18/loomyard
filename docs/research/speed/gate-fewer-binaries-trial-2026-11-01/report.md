# Gate trials: fewer new test binaries per run, 2026-11-01

## Verdict

One test binary per package for all tiers, the tier chosen at run time, is the cut worth making permanent.
On the module's package graph it takes a cold run from 532 test binaries to 182 and a change of mean fan-in from about 101 new binaries to about 35, and on a ten-package sample every test that passed before still passed.
At about 10 s of scanner hold per new binary on OSL-1046, that is about 660 s saved per mean change and about 3600 s per change to a universally imported package.
Cutting the five universal imports out of the common `TestMain` path saves only when one of the five changes (about a third of the binaries for `internal/agentname`, nothing for any other change), leaves every tier2 binary untouched, and breaks one test whose contract it cannot keep.
Merging small leaf test packages was not trialled: the entry made it conditional on trials 1 and 2 falling short, and trial 2 does not.
The OSL-1046 hold is the Neve count times about 10 s; the operator's one check run there is described under Check on OSL-1046.

## Method

Every number comes from `lym gate test` records on Neve, from this task's worktree, with the hub's gate config (`go` scoper, `-trimpath -p=4`).
A test binary counts as new when its package is listed in a tier record's `launches`, which is a package the Go test cache missed.
Each run is one call per tier (`--tier 0`, `--tier 1`, `--tier 2`) over the same targets, by `scripts/gate-run.sh` (trial 1's first runs used one `--upto 2` call, the same tiers), which appends one row per tier to the trial's `results.tsv`: label, tier, launches, discovered, passed, failed, skipped, wall seconds, launch seconds.
Per trial the cap was three gate runs on a warm tree before the change and three after, one run per perturbed package.
A run perturbs one package with `scripts/perturb.sh`, which adds a file whose `init` stores a fresh constant in a package variable.
An unused variable does not work: the linker drops it and most binaries stay identical (the first attempt, 54 of 182 tier0 binaries, showed it).
Each perturbation value is used once, so no run is answered from an earlier run's cache.
The three perturbed packages are:

- `internal/agentname`, imported by every test binary (universal);
- `internal/hubforge`, imported by 34 of 181 tier1 binaries (mid);
- a package with few importers (leaf): `internal/boardcli` in trial 1, `internal/arbiterstop` in trial 2.

A run with no perturbation (none) gives the floor of binaries that launch on an unchanged tree.
Those are mostly tests that read the prebuilt `lym`, which changes with any tree change, and so miss the test cache although their test binary is the same file the scanner has seen.
The record cannot tell those from a new link, so a launch count is an upper bound on new binaries; the floor is subtracted below where it matters.
The trial changes were never committed; their patches, the scripts and the tables are beside this report.

## Trial 1: the five universal imports out of `TestMain`

### Change

`tmuxkit.Main` imports `lymtemp` (which imports `lock`), `proc` and `agentname`.
The trial compiles `tmuxkit.go` only under `tier2` or `tier3` and adds `main_light.go`, which supplies `Main` and `ServerEnviron` for tier0 and tier1: a private run directory in the `<system temp>/lym-<uid>/r<random>` layout, the temp and git-ceiling variables, no sweep, no socket check, no leftover scan.
The kit's own tier0 tests need the full kit, so three of its test files move to tier2.
Tier2 binaries keep the full kit, because their tests start tmux servers.
Patch: `trial1/trial1.patch`.

### Fan-in of the five

Test binaries importing each package, tier0, tier1, tier2, from the package graph (`tables/fanin-tier<N>-before.tsv` and `-trial1.tsv`):

| Package | Before | After |
|---|---|---|
| `internal/agentname` | 182, 181, 181 | 88, 85, 181 |
| `internal/lock` | 182, 181, 181 | 114, 110, 181 |
| `internal/lymtemp` | 182, 181, 181 | 99, 96, 181 |
| `internal/proc` | 182, 181, 181 | 136, 133, 181 |

The production code of the other packages still imports each of the four, so the cut lowers the fan-in without removing it.
`internal/testkit/tmuxkit` itself stays imported by every binary, as before.

### Measured

New binaries per tier (tier0, tier1, tier2) and their sum, from `trial1/results.tsv`:

| Perturbed | Before | After | Saved | OSL-1046 hold saved |
|---|---|---|---|---|
| none (floor) | 13, 2, 0 = 15 | 7, 3, 0 = 10 | 5 | about 50 s |
| `internal/agentname` | 182, 181, 181 = 544 | 91, 88, 181 = 360 | 184 | about 1840 s |
| `internal/hubforge` | 14, 34, 5 = 53 | 14, 36, 5 = 55 | -2 | none |
| leaf (`internal/boardcli`) | 13, 14, 4 = 31 | 13, 15, 4 = 32 | -1 | none |

The after counts for `internal/agentname` match the graph (88 and 85 plus the floor).

### Test results

The three tiers discover 25737 tests before and after; 25 tier0 tests of `tmuxkit` move to tier2 (tier0 21803 to 21778, tier2 266 to 291).
Results are not the same:

- `TestRunDirectory_HoldsEveryTempFileAndIsGoneAfterTheRun` in `internal/gitkit` (tier1) fails.
  It requires that the temp files of the test process land in the one run directory `lymtemp` made for it.
  The light `Main` makes its own directory without `lymtemp`, so `lymtemp.Dir` in the test process makes a second one.
  Sharing the directory needs `lymtemp`, which imports `lock`, so the contract and the cut conflict.
- A first attempt that created the directory with `os.MkdirTemp("", ...)` also failed `TestBareTemp_NoCreationInTheBareSystemTempDirectory` (`PATTERN-lym-temp-root`), and a second that used a flat sub-path failed three tier1 tests that read the run directory's place (`t1-attempt1-*` and `t1-attempt2-*` rows).

The light path also drops the socket check and the leftover-process scan that `tmuxkit.Main` runs at the end of every tier0 and tier1 package.
Those are what `PATTERN-test-speed` names as the end-of-package check for tier1; a permanent cut would need them back without the five imports.

### Reading

The cut saves only when one of the five changes, which is rare, and then about a third of the binaries.
It does nothing for tier2, and it costs the end-of-package check and one contract test as built.

## Trial 2: one test binary per package, the tier chosen at run time

### Change

The sample is the ten packages with the most tiers among test files, ties broken by test file count (`trial2/sample.txt`): `internal/reedcli`, `webstercli`, `creelcli`, `loomcli`, `orchcli`, `pairteardown`, `treadleengine`, `standalonegeom`, `fabricengine` and `reedengine`.
The change has three parts:

- `internal/testkit/tiergate` (new, stdlib only) reads `LYM_TEST_TIER`; `Require(t, tier)` skips a test whose tier is not the running one.
- `tmuxkit` reads its tmux-server switch from `tiergate` at run time instead of from `allow_tmux.go`'s `tier2 || tier3` tag, since a tag-dependent file would make the tier2 binary differ from the others.
- `scripts/untag.sh` drops the `tierN` term from every `//go:build` line of the sample's test files and opens each `Test` function with `tiergate.Require(t, N)` for its file's tier.
  The three packages with a `!tier1` `TestMain` beside a tier1 one keep the tier1 one.

The hand-written part is `trial2/trial2-handwritten.patch`; `untag.sh` regenerates the rest.
`scripts/gate-run.sh` sets `LYM_TEST_TIER` to the tier of each call, since the hub's `gate.yaml` passes only tags.
Compiled sets are identical across tiers: no non-test file of the sample's dependencies carries a tier tag once `allow_tmux.go` is runtime, so the three tier calls link the same binary per package.

### Measured on the sample

New binaries per tier and their sum; for the after rows the distinct column counts each package once across tiers, since the three tier calls run the same binary (`launched/` holds the package lists), from `trial2/results.tsv`:

| Perturbed | Before | After, summed | After, distinct | Saved | OSL-1046 hold saved |
|---|---|---|---|---|---|
| none (floor) | 6, 1, 3 = 10 | 2, 0, 0 = 2 | 2 | 8 | about 80 s |
| `internal/agentname` | 10, 10, 10 = 30 | 10, 10, 10 = 30 | 10 | 20 | about 200 s |
| `internal/hubforge` | 6, 6, 5 = 17 | 8, 8, 8 = 24 | 8 | 9 | about 90 s |
| leaf (`internal/arbiterstop`) | 5, 2, 2 = 9 | 2, 2, 2 = 6 | 4 | 5 | about 50 s |

The summed after figure is what the gate records: the test cache keys on `LYM_TEST_TIER`, so each tier call reruns the binary and counts it as a launch although the file is the same one.
A scanner that holds a file by content hash holds it once; the hold saved above assumes that, and the OSL-1046 check below tests it.
The first run after applying the change linked all ten packages once (10 in each tier call), as any change does.

### Scaling to the whole module

The package graph gives the binary count and fan-in of the one-binary layout without building it: `scripts/fanin-union.sh` unions the dependencies of a package's test binary over tiers 0 to 3, as one untagged binary links them.
Over the sample it predicts 10 for `internal/agentname` and 8 for `internal/hubforge`, which the runs confirm (leaf: 1 predicted, 4 measured with the floor).
Over the whole module (`tables/fanin-union-all.tsv`):

| Measure | Before (tiers 0 to 2) | After |
|---|---|---|
| Test binaries on a cold tree | 532 | 182 |
| Mean binaries a package change relinks | 101.3 (32.8, 35.3, 33.3) | 35.3 |
| `internal/agentname` change | 544 | 182 |

That is 350 fewer on a cold tree (about 3500 s on OSL-1046), about 66 fewer per mean change (about 660 s) and 362 fewer per universal change (about 3620 s).
The after fan-in includes the dependencies of tier3 files, which now link into every binary.

### Test results

On the sample, passed tests per tier are 3813, 1930 and 256 before and after, with 0 failures; the selector skips 716, 775 and 1349 tests in tier0, tier1 and tier2 (`discovered` counts them).
Not the same:

- `TestLLMTier_OnlyLLMFilesReachAnLLM` and `TestTierPurity_UntaggedTestsSpawnNothing` in `cmd/lym` fail, since they read tier tags to find a file's tier (`trial2/guard-findings.txt`).
  `TestTierTags_OneTagPerTestFile` asserts a tag on every test file and would fail too.
- The tier tag no longer shows what a file may do, so the `tier0` rule "no spawn" holds by the runtime skip and review alone, not by a guard.

### Costs of a permanent change

- `PATTERN-test-speed` changes from disjoint tags to a run-time selector; the tag guards, `go vet` per tag and the gate's `tier` command must pass `LYM_TEST_TIER` (an `env` entry on each tier in `gate.yaml`).
- Every Test function of about 180 packages needs the `Require` call.
- Tier3 dependencies (`llmkit`) join every binary and raise its fan-in.
- A binary is larger and its test cache entry per tier is lost when a test reads `LYM_TEST_TIER`, so each tier call reruns the binary.

## Trial 3: merging small leaf packages

Not trialled.
Trial 2 takes the mean change from about 101 to about 35, and the entry made trial 3 conditional on both earlier trials falling short; the operator decides whether the remaining 182 binaries on a cold tree call for it.
For scale only: 84 of the module's test packages hold three or fewer test files, an upper bound of 83 binaries saved on a cold tree, with each merge losing the per-package test cache.
The one-binary-for-the-whole-module cut was not trialled, as the entry says.

## Check on OSL-1046

The Neve count of new binaries is the OSL-1046 count; the hold is that count times about 10 s.
To confirm it, the operator runs `scripts/perturb.sh internal/agentname <value>` and then `scripts/gate-run.sh <label>` on OSL-1046 once on the baseline tree and once on the trial 2 tree, and compares the scanner hold of each run with the launches column times 10 s.

## Limits

- Each row is one run on one machine; the noise floor (15 and 10 binaries on an unchanged tree in trial 1, 10 and 2 in trial 2) is larger than the leaf and mid differences.
- Trial 2 ran on a ten-package sample; the whole-module figures come from the package graph, validated against the sample.
- The launch count includes tests that read the prebuilt `lym` and miss the test cache with an unchanged test binary.
- Whether the scanner holds by content hash was not measured.
