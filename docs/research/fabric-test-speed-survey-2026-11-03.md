# `internal/fabricengine` test time: where it goes and how to cut it, 2026-11-03

A measurement report on neve, the operator's Ryzen machine, with reasoning for OSL-1046, where the report cannot measure.
It records one afternoon of runs at revision `b2b64690` and is not meant to stay true.
It changes no code, test or config, and files no task; the ranked list at the end is for the operator to pick from.

`internal/fabricengine` is the floor of the `tier1` gate: 348.72 s of the 388.6 s tier wall on OSL-1046 and 45.69 s on neve in the last two audits of [test-suite-timing.md](../benchmarks/test-suite-timing.md).
On OSL-1046 Cortex XDR holds every binary it has not seen for about ten seconds and scans every file that is opened, so the questions are how many processes and files a run makes, and which fixtures and tests make them.

## Answer in brief

- One `tier1` run of the package starts 86,827 processes, 78,192 of them git, and opens 5.9 million files, 4.1 million of them from git.
  Git's own work is about 60 percent of the serial test time on neve, and the kernel side of the run (`sys`) is larger than the user side.
- Seven of every ten git processes carry the fixture marker: they build hubs and pairs, commit, push and read back state for the test harness.
  Three of ten come from the code under test, and one call, `git worktree list --porcelain`, is 15 percent of all git processes on both sides.
- `hubforge`'s template-and-copy model is in use at 388 call sites and works: a `CopyHub` costs about 7 git processes and 180 files.
  What it does not cover is the pair: tests add about 540 pairs per run through `AddPair` and `AddPairWith`, each on the order of 60 git processes with its foreground push, because the one pair template carries the slug `pair` and tests name their own slugs.
  Two builders still clone a hub fresh through `NewHub` 45 times per run, at about 70 to 80 git processes each.
- The levers, ranked by what they remove: pair templates the tests can use (about 38 percent of git processes), a `dotgit` read in place of `worktree list` (15 percent), `CopyHub` in the two `NewHub` builders plus shapes for the four builders that layer one commit on a copy (4 percent), folding the overlapping tests named below (about 9 percent of serial time), the post-checkout hook's five processes per worktree checkout (5 percent of process starts), and a slimmer hub copy without the per-copy fingerprint walk (file opens only).
- `t.Parallel` is already in place: the package runs 2.8 times faster at the default `-parallel` than serially, and CPU time is the same at every setting, so parallelism moves no cost on a scanner-bound machine.
  `-p` is irrelevant to this package alone; it decides only what the package overlaps with in a full tier run.

## Route and method

Every number comes from `lym gate test`, as the gate rule asks, with the package as the explicit target:

```sh
lym gate test --wait --tier 1 ./internal/fabricengine -- -count=1
lym gate test --wait --tier 1 ./internal/fabricengine -- -count=1 -parallel=1
lym gate test --wait --tier 0 ./internal/fabricengine -- -count=1
```

- The gate's `tier1` command runs `go build ./...`, `go vet` on the package and then `go test -json`, with a prebuilt `lym`, under `GOFLAGS="-trimpath -p=4 -timeout=20m"`; the gate wall includes the build and vet, about 8 s, and the package wall is the `ok` line's time.
- An untracked file sat in the worktree for the whole measurement so the gate read no green record, and was removed afterwards.
- Per-test seconds are the `--- PASS` lines of the gate's log.
  A parent's line excludes its parallel subtests, so a test's cost below is its own time plus the cost of every subtest that paused, folded up through the subtest tree.
  The sum of those costs, 224.1 s, exceeds the serial package wall of 209.6 s by the rounding of 1,766 lines to hundredths.
- Git processes come from git's trace2 event files: `GIT_TRACE2_EVENT=<dir>` with `GIT_TRACE2_ENV_VARS=LYM_FIXTURE_GIT` and `trace2.maxFiles=0`, the route `cmd/testtiming -resources` uses, set in the environment of one serial run and one default run.
  Each file names the subcommand, the fixture marker, the parent process and the repository path; a git process is attributed to a test through the test name in its `t.TempDir` path, which covers 82 percent of them.
  A process with no repository path, such as a child `unpack-objects` run in its cwd, stays unattributed.
- Files and processes come from `strace -f --seccomp-bpf -e trace=execve,openat,...,unlinkat,mkdirat,renameat2,symlinkat` around one default run, with the trace2 files beside it to tell fixture git from code git by pid.
  That run took 207 s against 71 to 83 s untraced, so it is a counting run, not a timing run.
- The fixture marker, `LYM_FIXTURE_GIT`, is set by every `gitkit` helper and by `hubforge` for the whole process while a hub builds, so "fixture" below means git the test harness ran, setup and assertions alike, and "code" means git the code under test ran outside a build.

### Machine state

```yaml
machine: AMD Ryzen AI 7 445 w/ Radeon 840M, 12 threads, 30 GB, neve
os: Linux 7.0.0-31-generic (Ubuntu), bare metal, no endpoint security
go: go1.26.8 linux/amd64
git: 2.53.0
revision: b2b64690
gate: slots 3, tier_parallel 4
date: 2026-11-03, 12:19 to 12:49
```

The machine was not quiet.
Other worktrees' gate runs held one or two of the three slots during every run but one, and six agent sessions were busy; the one-minute load average was between 4.6 and 25.4 at the starts of the runs, where the second audit saw 3.0 to 11.6.
The walls below are therefore an upper bound on a quiet machine's, with a spread of about 30 percent between runs of the same command, and the spread is why the parallelism sweep was run twice.
The counts of processes and files do not depend on load and are exact for their run.

## Where the time goes

### Runs

| Run | `-parallel` | Gate wall | Package wall | User | Sys | Load at start |
|---|---|---|---|---|---|---|
| cold | 12 | 91.4 s | 81.7 s | 92.0 s | 110.2 s | 13.4 |
| warm 1 | 12 | 89.4 s | 82.9 s | 89.4 s | 109.8 s | 18.9 |
| warm 2 | 12 | 82.7 s | 74.3 s | 89.8 s | 107.4 s | 25.4 |
| warm 3 | 12 | 79.1 s | 71.1 s | 88.7 s | 102.9 s | 22.9 |
| serial, trace2 | 1 | 218.3 s | 209.6 s | 96.5 s | 126.4 s | 14.4 |
| strace, trace2 | 12 | 214.9 s | 207.3 s | | | 14.9 |
| sweep A | 4 | 70.3 s | 63.0 s | 80.7 s | 81.5 s | 4.6 |
| sweep A | 24 | 56.2 s | 48.8 s | 80.9 s | 94.4 s | 9.0 |
| sweep B | 12 | 81.5 s | 76.2 s | 86.4 s | 106.0 s | 8.3 |
| sweep B | 4 | 91.6 s | 82.5 s | 89.5 s | 100.2 s | 24.4 |
| sweep B | 24 | 77.8 s | 69.7 s | 87.9 s | 104.7 s | 19.7 |
| sweep B | 1 | 157.1 s | 148.7 s | 83.8 s | 90.3 s | 24.6 |
| `tier0`, twice | 12 | 0.8 s | 0.09 s, 0.12 s | 1.2 s | 0.3 s | 12.4 |

User and sys are `/usr/bin/time` over the whole `lym` tree.
Every `tier1` run passed 1,607 of 1,766 discovered tests and skipped 159, the `tier0` tests of the one binary; `tier0` passed 877 and skipped 554.
The package holds 554 top-level `tier1` tests in 145 files and 159 `tier0` tests in 60 files; `tier0` costs a tenth of a second and is not discussed further.

Two readings hold whatever the load:

- CPU time is about 180 s of user plus sys in every run, and sys exceeds user.
  The work is process starts, file operations and page faults, not computation.
- The default `-parallel` (12, the thread count) runs the package 2 to 2.8 times faster than serially, and 4 or 24 are within the noise of 12.
  No single test is long enough to bound the wall: the longest own time in a parallel run is 7.2 s.

### Per test

The serial run's cost per top-level test, with the git processes attributed to it.
"Git top" is the time inside git processes the test binary started, so a push's children are inside it.

| Test | Cost | Of which own | Git processes | Fixture | Code | Git top | File opens |
|---|---|---|---|---|---|---|---|
| `TestAnchorChange` | 18.32 s | 10.83 s | 2,051 | 1,861 | 190 | 4.81 s | 53,477 |
| `TestCrossProduct` | 12.48 s | 0 | 3,391 | 2,508 | 883 | 3.4 s | 108,872 |
| `TestVerbCases_CleanState` | 11.04 s | 5.51 s | 757 | 472 | 285 | 1.50 s | 31,364 |
| `TestReconcileOrigin_Moves` | 9.61 s | 9.61 s | 2,202 | 1,719 | 483 | 4.35 s | 44,665 |
| `TestPushFailureClasses` | 9.25 s | 9.25 s | 2,454 | 1,288 | 1,166 | 5.76 s | 57,368 |
| `TestRecordsReplay` | 8.19 s | 0 | 1,750 | 1,653 | 97 | 2.96 s | 47,074 |
| `TestReconcileOrigin_Judge` | 6.50 s | 6.50 s | 1,960 | 1,404 | 556 | 3.23 s | 35,929 |
| `TestReconcileOrigin_Ended` | 5.63 s | 5.63 s | 1,248 | 992 | 256 | 2.69 s | 25,954 |
| `TestBolt_PullThenCommitWritten` | 4.44 s | 0 | 1,235 | 1,079 | 156 | 1.94 s | 36,147 |
| `TestReconcile_SweepsRetiredHubArtifacts` | 4.02 s | 2.11 s | 343 | 166 | 177 | 0.38 s | 10,477 |
| `TestReconcile_MigratesMarkerlessWefts` | 3.36 s | 3.36 s | 992 | 417 | 575 | 1.82 s | 24,004 |
| `TestStoreHousekeeping` | 3.30 s | 3.30 s | 605 | 370 | 235 | 1.08 s | 17,786 |
| `TestRegenerateHubWorkspace` | 2.76 s | 0.08 s | 1,346 | 1,188 | 158 | 1.27 s | 37,333 |
| `TestStates` | 2.42 s | 1.21 s | 131 | 129 | 2 | 0.29 s | 8,559 |
| `TestRemoveLocal` | 2.18 s | 2.18 s | 830 | 551 | 279 | 1.17 s | 16,484 |
| `TestPull_WeftDivergedWarpAdvancesCleanly` | 1.94 s | 0 | 699 | 528 | 171 | 1.08 s | 13,477 |
| `TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide` | 1.92 s | 1.92 s | 496 | 233 | 263 | 1.13 s | 14,402 |
| `TestBoltReplayers` | 1.82 s | 1.82 s | 357 | 332 | 25 | 0.70 s | 7,528 |
| `TestReadConfigChanges` | 1.80 s | 0 | 554 | 487 | 67 | 0.86 s | 15,292 |
| `TestEnsureAnchor` | 1.76 s | 0 | 239 | 208 | 31 | 0.63 s | 6,870 |

The ten costliest tests are 89.5 s of 224.1 s, the top 25 are 119.9 s, the top 50 are 140.9 s and the top 100 are 164.1 s; 253 of the 554 tests cost under a tenth of a second.
`TestCrossProduct`'s git time is read from its cells' traces and is approximate.
Twelve of the top twenty never call `t.Parallel` at the top level; `TestAnchorChange`, the three `TestReconcileOrigin_*` tests and `TestPushFailureClasses` run their subtests serially too, and together they are 49 s of serial cost, the part no parallelism reaches.

### Per file

| File | Tests | Cost | Git | Fixture | Code | File opens | Builder |
|---|---|---|---|---|---|---|---|
| `anchorchange_integration_test.go` | 1 | 18.32 s | 2,051 | 1,861 | 190 | 53,477 | `newAnchorHub`: `CopyHub` plus two commits, then `AddPair`, 13 rows |
| `livestate_matrix_test.go` | 2 | 12.81 s | 3,391 | 2,508 | 883 | 108,872 | 144 `CopyHub`, 66 `AddPair`, one per cell |
| `livestate_verbs_selftest_test.go` | 2 | 11.04 s | 757 | 472 | 285 | 31,364 | 54 `CopyHub`, one per verb and anchor |
| `originreconcilemoves_integration_test.go` | 1 | 9.61 s | 2,202 | 1,719 | 483 | 44,665 | `newMovesPair`: `newOriginHub` plus `AddPair`, 19 sites |
| `pushclasses_integration_test.go` | 1 | 9.25 s | 2,454 | 1,288 | 1,166 | 57,368 | `newFailureFixture`, `newPairFixture`: 19 hubs |
| `replay_integration_test.go` | 1 | 8.19 s | 1,750 | 1,653 | 97 | 47,074 | `newRecordsFixture`: `NewHub`, 25 subtests |
| `merge_target_integration_test.go` | 14 | 6.61 s | 1,929 | 1,520 | 409 | 48,340 | `newMergeTargetFixture`: `newMergePairFixture` plus `AddPair` |
| `originreconcile_integration_test.go` | 1 | 6.50 s | 1,960 | 1,404 | 556 | 35,929 | `newOriginHub`, `newRewoundBranch`: 7 rewound pairs |
| `pull_integration_test.go` | 17 | 6.35 s | 2,155 | 1,475 | 680 | 42,231 | `buildReconcileFixture`: plain warp, bare and `CopyHub` |
| `originreconcileended_integration_test.go` | 1 | 5.63 s | 1,248 | 992 | 256 | 25,954 | `endedHub`: `newOriginHub` plus a pair and two boards |
| `boltpullfirst_integration_test.go` | 3 | 5.60 s | 1,768 | 1,498 | 270 | 52,067 | `newBoltFixture`: `NewHub`, 20 sites |
| `mergein_recovery_integration_test.go` | 25 | 5.05 s | 1,450 | 856 | 594 | 29,733 | `newMergePairFixture`, 22 sites |
| `add_remoteleftover_integration_test.go` | 9 | 4.18 s | 1,510 | 630 | 880 | 26,963 | `CopyHub` plus `AddPairWith` inline |
| `portals_sweep_integration_test.go` | 1 | 4.02 s | 343 | 166 | 177 | 10,477 | `CopyHub` inline |
| `mergecrucible_integration_test.go` | 11 | 3.81 s | 1,107 | 651 | 456 | 22,769 | `newMergePairFixture` plus a conflicting merge |
| `pushanchored_integration_test.go` | 6 | 3.78 s | 835 | 459 | 376 | 24,752 | `newPushPair`: `CopyHub` plus `AddPair` |
| `reconcile_migrates_markers_integration_test.go` | 1 | 3.36 s | 992 | 417 | 575 | 24,004 | `CopyHub` plus `AddPairWith` inline |
| `housekeeping_integration_test.go` | 1 | 3.30 s | 605 | 370 | 235 | 17,786 | `CopyHub` plus `AddPair`, 8 subtests |
| `parenttips_integration_test.go` | 12 | 3.04 s | 1,113 | 897 | 216 | 26,270 | `buildReconcileFixture` |
| `resetpairwarp_integration_test.go` | 7 | 2.93 s | 1,559 | 1,373 | 186 | 45,790 | `newRemoteHalfPair`: `CopyHub`, `AddPair`, two pushed commits |

The ten costliest files are 94.3 s of 224.1 s and the top twenty 133.4 s.
Three groups stand out: the live-state matrix and its self-tests (24 s, 218 hubs), the three `TestReconcileOrigin_*` tests (22 s, about 35 hubs each with a pair), and `TestAnchorChange` (18 s, 13 hubs each with two commits and a pair).

### What the time is

The serial run's 209.6 s package wall splits as:

| Part | Time | How read |
|---|---|---|
| Inside git started by the test binary | 116.0 s | sum of each process's `exit` time; a push's children are inside it |
| Inside git started by hooks and `sh` outside a push | about 1 s | the post-checkout hook's `rev-parse` calls |
| Process start and dynamic loading of git, before its `main` | not in the traces | 55,617 starts from the test binary; at the half to one millisecond a Linux exec of git costs, 30 to 55 s |
| The test binary itself and gaps | the rest, about 40 to 60 s | hub copies, fingerprint walks, go-git reads, assertions |

Git's own time by subcommand, serial run, children included where nested:

| Subcommand | Processes | Fixture | Code | In-git time |
|---|---|---|---|---|
| `push` | 3,078 | 2,733 | 345 | 44.1 s |
| `receive-pack` (child of push) | 3,076 | 2,733 | 343 | 33.8 s |
| `worktree` | 14,929 | 7,868 | 7,061 | 19.2 s |
| `rev-list` | 6,515 | 4,741 | 1,774 | 11.6 s |
| `commit` | 4,102 | 3,698 | 404 | 8.3 s |
| `fetch` | 742 | 134 | 608 | 6.4 s |
| `pack-objects` | 3,178 | 2,753 | 425 | 5.7 s |
| `upload-pack` | 2,738 | 1,395 | 1,343 | 5.4 s |
| `ls-remote` | 1,332 | 785 | 547 | 4.9 s |
| `add` | 3,986 | 3,548 | 438 | 4.7 s |
| `checkout` | 600 | 506 | 94 | 4.2 s |
| `rev-parse` | 6,401 | 5,266 | 1,135 | 3.9 s |
| `clone` | 686 | 476 | 210 | 3.7 s |
| `unpack-objects` | 3,162 | 2,750 | 412 | 3.0 s |
| `config` | 4,367 | 3,550 | 817 | 2.4 s |
| `diff` | 2,459 | 1,101 | 1,358 | 2.2 s |
| `status` | 1,978 | 854 | 1,124 | 2.1 s |
| `for-each-ref` | 3,328 | 2,707 | 621 | 2.0 s |

A push to a local bare costs 14 ms on average and starts four processes (`push`, `sh`, `receive-pack`, `unpack-objects` or `pack-objects`); the 3,078 pushes with their children are about a quarter of git's time.
`worktree` is the most started subcommand and the third in time.

## Git processes and files per run

### By spawner and marker

| Spawner | Git processes | In-git time |
|---|---|---|
| the test binary | 55,617 | 116.0 s |
| `sh` (git's own `sh -c` for a local `receive-pack` or `upload-pack`) | 5,814 | 39.1 s |
| git (children: `pack-objects`, `unpack-objects`, `rev-list`) | 9,198 | 11.2 s |
| `git-receive-pack` | 5,872 | 11.4 s |
| the post-checkout hook | 1,432 | 0.9 s |
| `git-upload-pack`, `git-merge-octopus`, `lym` | 259 | 0.4 s |

| Marker | Git processes | In-git time |
|---|---|---|
| fixture: `gitkit` helpers and every git during a `hubforge` build | 53,992 | 140.4 s |
| code under test | 24,200 | 38.7 s |

By attribution class: 63,780 processes belong to a test by path, 383 to the four template builds (0.8 s in all), and 14,029 have no path in their trace.

### What the code under test runs

The code side's calls, by the first two arguments, from the serial run:

| Call | Processes |
|---|---|
| `worktree list --porcelain` | 6,101 |
| `status --porcelain` | 994 |
| `diff --name-only` | 821 |
| `config` | 738 |
| `rev-parse --verify` | 681 |
| `merge-base --is-ancestor` | 569 |
| `rev-list --count` | 530 |
| `ls-remote --heads` | 530 |
| `fetch` | 520 |
| `diff --cached` | 490 |
| `show` | 467 |
| `branch --format=...` | 461 |
| `ls-tree --name-only` | 451 |
| `ls-files` | 439 |
| `add` | 395 |
| `for-each-ref --format=...` | 356 |
| `commit -m` | 337 |
| `push` | 307 |
| `worktree remove` | 303 |
| `branch -D` | 255 |
| `worktree add` | 231 |
| `worktree repair` | 222 |
| `ls-tree HEAD` | 214 |
| `worktree prune` | 200 |
| `gc --auto` | 170 |

The fixture side runs the same `worktree list --porcelain` 5,668 times, because `AddPair` drives the production `Add` verb, so the call is 11,769 of the run's 78,192 git processes.
It is a read of `.git/worktrees/`, which `internal/dotgit` could serve without a process.
`rev-parse --verify`, `merge-base --is-ancestor`, `rev-list --count`, `for-each-ref`, `ls-tree` and `show` are reads too, about 3,000 processes, that go-git can answer; `status`, `diff` and `ls-files` are index reads that go-git answers slowly on a large tree but these trees are tiny.

### Files

From the traced default run; the gate's build and vet are included but small:

| Operation | Count | Of which |
|---|---|---|
| process starts (`execve`) | 86,827 | git family 78,192; `sh` 5,815; `post-checkout`, `basename`, `dirname` 2,625; `pre-receive` 58; Go toolchain and `lym` about 130 |
| file opens, successful | 5,872,093 | git 3,777,013; `git-receive-pack` 216,407; `git-upload-pack` 122,326; the test binary 1,639,555; `go` and `lym` 75,828; hook shells and tools 34,774 |
| file opens, failed | about 260,000 | git probing for config, hooks and templates |
| files created (`O_CREAT` or `creat`) | 539,673 | fixture git 271,055; the test binary 243,319; code git 23,549; hooks 1,459; 78,192 of the total are the measurement's own trace files |
| directories created | 238,713 | |
| unlinks | 844,355 | `t.TempDir` cleanup and git's own temporaries |
| renames | 53,358 | git's atomic writes of refs, index and packed objects |
| links and symlinks | 75,753 | junctions and git's own |

Opens by path:

| Path class | Opens | Created |
|---|---|---|
| fixture trees under the temp root | 2,622,320 | 399,058 |
| system files: shared libraries, locale, `/etc` | 1,764,590 | 1,455 |
| relative paths, git inside its cwd | 1,325,028 | 60,679 |
| the worktree, mostly the trace files and the Go build | 93,473 | 78,203 |
| the Go build cache | 31,391 | 70 |

Every git process opens about 22 system files before it reads a repository: the loader's libraries, the locale, `/etc/gitconfig` and its templates.
That is 1.7 million opens per run that only fewer processes can cut.
The test binary's 1.64 million opens are mostly hub copies: 480,000 are reads of the four templates, 243,000 are writes of the copies, and the rest are go-git and assertion reads.

### Per fixture

| Fixture step | Git processes | Files | Per run |
|---|---|---|---|
| template build (`buildBareTemplate`, four `templateFor` shapes) | 383 in all | about 700 to 1,000 entries per template, 180 regular files | once per binary |
| `CopyHub` | 3 from the amend and force-push, about 7 with their children | about 180 files written, about 420 template entries read, and a fingerprint walk of every template file before the copy | 1,145 |
| `SharedHub` | 0 | a fingerprint walk before and after | 1 site, `warplayout_test.go` |
| `AddPair`, `AddPairWith` | on the order of 60 with the foreground push of both sides: two `worktree add`, two `worktree repair`, many `worktree list`, `ls-remote --heads`, commits and pushes | two worktrees, a junction, launchers | about 540, read from 1,086 fixture `worktree add` calls |
| `NewHub` (`fabriccli.CloneAndWire`, two clones, the board push, the correspondence rewrite) | 70 to 80 | two clones' worth | 45 through `newRecordsFixture` and `newBoltFixture`, plus `CloneSecond` and direct sites |
| a `gitkit.CommitFile` | 3 | one file | 2,416 fixture commits |
| a `git clone` of a bare by a test, the "second machine" | about 6 with children | a full clone | 476 fixture clones, two of them per `NewHub` |

A `TestStates` cell, which copies a hub and plants one state, shows the floor: 131 git processes over 20 copies, 6.5 per copy, and 8,559 file opens.

## What `hubforge` covers, and what still builds with git

`hubforge` builds the bare template once per binary and, since the template-and-copy model was extended to the hub, four hub templates: the anchors `.` and `backend`, each bare and each with the one pair `pair`.
`CopyHub` relocates one into the test's own directory, rewriting every path, and `SharedHub` hands out the template to a test that only reads.
In `internal/fabricengine` that model is the default: 388 `CopyHub` call sites against 41 `NewHub` sites, and the live-state matrix, the merge fixtures, the origin-reconcile fixtures and the push fixtures all start from a copy.

What the templates do not cover, from a read of every builder in the package:

| Builder | Builds | Beyond a template | Callers |
|---|---|---|---|
| `newRecordsFixture` | `NewHub(".")` with the board and heddle replayers | nothing: the shape `.` with no pair | 27 sites, `TestRecordsReplay` and `TestBoltReplayers` |
| `newBoltFixture` | `NewHub(".")` and a `Bolt` over the board | nothing: the shape `.` with no pair | 20 sites in `boltpullfirst_integration_test.go` |
| `newAnchorHub`, `newAtAnchorHub`, `newCheckoutHub` | `CopyHub(".")` plus one or two commits of a `.keep` under a tracked directory | the extra commits, then a pair with a named slug | 13, 8 and 6 sites |
| `newDeadOriginHub` | `CopyHub(".")` with both origins repointed to a dead path | the rewritten remotes | 6 sites and a helper |
| `newOriginHub` and its three layers `newMovesPair`, `endedHub`, `newRewoundBranch` | `CopyHub(".")`, the records branch pushed with an upstream, then a pair `task` with a published branch, boards, a second clone or a rewound origin | the pair slug and the extra pushes | 32 sites |
| `newMergePairFixture` and its layers | `CopyHub(anchor)` and `OpenFabric`, then branch commits at 7 git processes each | the commits; the bare fixture fits the template | about 76 sites in 11 files |
| `newPushPair`, `newPairFixture`, `newLocalModePair`, `newRemoteHalfPair`, `discardHub`, `archivedDeleteFixture`, `newMergeSiblingsFixture`, `sweepFixture` | `CopyHub(".")` plus `AddPair` with the test's own slug | only the slug, or the slug and pushed commits | 10 to 20 sites each |
| `buildReconcileFixture`, `newDivergedWeftFixture`, `newTaskPatchFixture`, `staleCorrespondenceFixture` | a plain warp repository and bare built with `gitkit`, and a `CopyHub` for its weft alone | the plain warp is not a hub | 28, 11, 6 and 1 sites |
| `newCommitFixture`, `newPlainWarpRepo`, `newPlainWeftRepo`, `newUnbornWarpRepo`, `newUnbornWeftRepo`, `newGitRepoForExcludeTest`, `newIntegrateFixture`, `newBoltRepo`, `newBoltBareRemote` | plain repositories through `gitkit` or `exec`, 1 to 12 git processes each | not hubs; the internal `package fabricengine` tests cannot import `hubforge` | 1 to 13 sites each |
| `newClonedHubFixture` | `fabricengine.CloneHub` from two seeded bares | it tests the clone itself | 10 sites |
| the live-state matrix | one `CopyHub` per cell, `AddPair` in the pair verbs' arrange | nothing for the copy; the pair for 66 cells | 144 copies and 66 pairs in `TestCrossProduct` and `TestCloneHubReset`, 54 more in `TestVerbCases_CleanState`, 20 in `TestStates` |

The pattern across the package: a test copies a template in a few milliseconds and then adds a pair under its own name, which costs ten times the copy.
The slug `pair` of the pair-carrying templates appears at no fixture site; the slugs are `task`, `pushpair`, `localmode`, `beam`, `target`, `live`, a table's row name or a formatted `anchorstop%d`.
Nearly every caller mutates its hub, so `SharedHub` has one site in the package and could gain a handful more at most: the read-only rows of `TestAnchorChange`'s refusal table, the four error-return arms of `TestMergeCrucible_ConflictsIsEmptyNeverNil`, the `parenttips` tests after their build, and `staleCorrespondenceFixture`'s one caller.

## Levers

Each lever is judged against the measured split: 78,192 git processes of which 53,992 fixture, 5.9 million opens of which 4.1 million from git, and 224 s of serial cost of which about 116 s inside git.

### More fixtures on the hub template, or templates of their own

- **Pair templates the tests can use.** About 540 pair adds per run cost on the order of 32,000 git processes, 41 percent of all git and most of the fixture side, plus the worktrees' files.
  Two routes: the fixtures take `hubforge.TemplatePairSlug` where the slug carries no meaning, so `CopyHub` of the pair-carrying shape replaces `CopyHub` plus `AddPair`; or `Shapes` opens to a slug, so a template is built once per slug the package uses, at about a dozen slugs.
  Either way a pair costs the copy's 7 git processes instead of 60.
  Expected: 25 to 35 percent of the package's git processes and serial time on neve, the same share of process starts on OSL-1046.
- **Shapes for the one-commit builders.** `newAnchorHub`, `newAtAnchorHub` and `newCheckoutHub` layer a tracked `.keep` commit on the copy, 3 to 6 git processes and a push each over 27 sites; `newOriginHub` pushes the records branch with an upstream, 3 processes over 32 sites.
  A shape per layer removes about 300 processes; small, but it unblocks the pair templates for the three heaviest files.
- **`CopyHub` in `newRecordsFixture` and `newBoltFixture`.** Both need only the `.` shape with no pair and clone through `NewHub` 45 times per run at 70 to 80 git processes each.
  Expected: about 3,000 git processes, 4 percent, and 3 to 5 s serial.

### Shared read-only fixtures

`SharedHub` fits a test that reads files, links or go-git state and runs no verb.
That describes almost nothing in this package: a verb is what each test runs.
The candidates named above are a few rows of a few tests, under 2 percent of the cost.
Not a lever on its own; worth taking where a fold (below) already touches the test.

### Fewer files per fixture

A `CopyHub` reads about 420 template entries, writes about 180 files and, before that, walks every template file for the fingerprint; 1,145 copies make 480,000 reads, 210,000 writes and about 400,000 stats per run, almost all of the test binary's 1.64 million opens.
On neve this is a few seconds.
Levers: skip the fingerprint on `CopyHub` and keep it on `SharedHub`, whose sharing is what it protects; a smaller template, since 138 of the biggest template's entries are the three repositories' loose objects, which a `gc` at build time packs into a handful of files; and no launchers or workspace files in a template a test never reads.
Expected on neve: 2 to 4 s; on OSL-1046 the lever is the opens, see below.

### Fewer git process starts in the code under test

- **`worktree list --porcelain` through `dotgit`.** 11,769 processes per run, 15 percent, 19 s inside git serially, and the production verbs make the call several times per `Add` and per `Remove`.
  `internal/dotgit` reads `.git` for local answers already; a reader of `.git/worktrees/*/gitdir` and `HEAD` answers the same question.
  Expected: 12 to 15 percent of git processes, 8 to 12 percent of serial time on neve.
- **The post-checkout hook.** `Add` and `CloneHub` install fabric's drift-warning hook, which git runs on every `worktree add` and checkout: 875 runs per test run, each an `sh`, a `dirname`, a `basename` and one to three `git rev-parse`, 4,400 process starts, 5 percent of the run's.
  The hook is behavior under test in `hook_test.go`, so the lever is in the script, not in the install: `${WORKTREE_ROOT##*/}` and `${WORKTREE_ROOT%/*}` replace the two tool spawns, and one `rev-parse --show-toplevel` can serve both branch reads through `--abbrev-ref` on the same call.
  Expected: 1 to 2 s on neve, about 2,600 process starts.
- **go-git for the other reads.** `rev-parse --verify`, `merge-base --is-ancestor`, `rev-list --count`, `for-each-ref`, `ls-tree` and `show`, about 3,000 code-side processes, are reference and object reads go-git answers in-process.
  Expected: 4 percent of git processes.
  `status`, `diff` and `ls-files`, 2,700 processes, are index reads; go-git's status is slower than git's on a large tree, but every tree here is a few files.
- **Fewer pushes.** 3,078 pushes with their four processes each are a quarter of git's time.
  Most are fixture pushes, which the pair templates remove; the code side's 307 stay.

### Merging or cutting overlapping tests

The coverage-based redundancy report lists no candidate for the package, so these come from reading the 35 costliest tests against the test-economy rules.
Each names what the candidate alone pins, so a fold keeps it.

| Candidate | Cost | Overlaps | Alone pins | Proposal |
|---|---|---|---|---|
| `TestVerbCases_CleanState` | 11.0 s | `TestCrossProduct` runs the same build, arrange, verb, expect and no-unpermitted-change sequence for every verb in the clean state | three preconditions: Add's launcher blocker is a directory, Checkout's branch differs, Pull's bare advanced | fold the three checks into `runCell` and delete the test |
| `TestAnchorChange`, the three refusal rows with no setup | part of 18.3 s | each other | the target string of the refusal | one hub for the three rows, which only refuse |
| `TestReconcileOrigin_Moves`, `owned_elsewhere` | part of 9.6 s | its own six iterations differ only in the `OwnerOf` reply and no origin moves | the reply per subject and scope | one pair for the six |
| `TestPushFailureClasses`, `ThroughPublishesPath` | part of 9.3 s | the two settle rows rebuild the same rewound pair and push it first | the hold-back record's class, dropped commit and mail | assert the record on the first push of one settle row and delete the subtest |
| `TestPushFailureClasses`, the two backoff rows | part of 9.3 s | each other: a refused origin, a transport record, the backoff count | the entry point and the error class | one table with two rows |
| `TestRecordsReplay`, `EndedSet` | part of 8.2 s | four rows each clone a full hub through `NewHub` and only call `ReadEndedSetForTest` | the read of each set | a bare plus clone fixture, or one hub |
| `TestRecordsReplay`, the project-file replay row | part of 8.2 s | the two-machine status replay with the same base and outcome | the extra files in the local commit | a row of the covering test |
| `TestBolt_PullThenCommitWritten`, three pairs of subtests | part of 4.4 s | each pair shares its upstream move and conflict and differs in one assertion | the record kinds, the way-forward text, the seed commit | three two-row tables |
| `TestBolt_PushRecorded_DropsSeedCommitsOnRebaseConflict` | 1.0 s | its two subtests assert the same error, head and clean board | whether a seed commit is ahead | a two-row table |
| `TestStoreHousekeeping`, two subtests | part of 3.3 s | the in-flight probe rows and the gc-prune subtest pack the same stores | that the default probe packs | delete both, add one assertion to the gc-prune subtest |
| `TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses` | 1.1 s | its six rows each build a fixture and plant a foreign merge state | five verbs refuse per shape and side | one fixture, cleaned between rows |
| `TestMergeCrucible_ConflictsIsEmptyNeverNil`, four arms | part of 0.9 s | each other: a refusal with no merge in progress | the `"conflicts":[]` shape | one fixture for the four arms |
| `TestRegenerateHubWorkspace`, the skip rows | part of 2.8 s | four breakages each skipped in the folder list | each breakage | one hub carrying all four at the `.` anchor |
| `TestPull_WeftDivergedWarpAdvancesCleanly`, two stop rows | part of 1.9 s | each other: a stop that leaves state unchanged | the marker name against the way forward | one fixture, or a table |
| `TestVerbsRegenerateHubWorkspace`, the dirty-weft row | part of 1.2 s | `PreFlightRefusalsRegenerateNothing` runs two refusals on one hub | the verb and the dirt target | a third refusal on that hub |
| `TestAddLocal`, `RefusesOverOrdinaryBranch` | part of 1.4 s | its four rows each build a dead-origin hub and plant a branch | each planted branch or ref | one hub, the branch deleted between rows |

Expected together: about 20 s of the 224 s serial cost, 9 percent, and about 3,000 git processes; the wall gain on neve is less because most of these run in parallel.
Each row of the table above that rebuilds a hub for a refusal is also where `SharedHub` would fit once the rows share one.

### The effect of `t.Parallel` and `-p`

- Serial against the default: 209.6 s and 148.7 s against 71 to 83 s, so the package's own parallelism takes 2 to 2.8 times off the wall at the same CPU.
  Every test file but 15, holding 19 `tier1` tests, calls `t.Parallel`; the top-level tests that still run serially are the subtest-heavy ones named under Per test, and they hold 49 s of serial cost.
  `-parallel=24` was faster than 12 in both sweeps, by 7 to 35 percent, and 4 was within noise of 12 once and slower once; under this load the sweep cannot rank 4, 12 and 24.
- `-p` is the gate's package parallelism, 4 in `gate.yaml`, and does nothing to a package run alone.
  In a full `tier1` run it decides how many packages share the machine with `fabricengine`, each at its own `-parallel=12`; the package is the tier's floor, so a lower `-p` lengthens the tier and a higher one lengthens the package's own time by contention.
  The earlier sweeps on OSL-1046 found no gain from 8 over 4.
- What parallelism cannot do: cut the 180 s of CPU, the 86,827 process starts or the 5.9 million opens.
  On a machine where a scanner serializes those, parallelism hides less than it does here.

## What carries over to OSL-1046

The report cannot measure there, so this is reasoning from the counts and from what the operator says Cortex XDR does: hold each binary it has not seen for about ten seconds, and scan every file that is opened.

- **The hold is paid once per test binary, not per git.** `git` is a known binary and runs at once; the package's one test binary is new after every change to the package or its imports, so a run pays one hold of about ten seconds.
  That is 3 percent of the 348 s and not where the gap to neve is.
- **The gap is per operation.** OSL-1046 ran the package in 348.72 s against 45.69 s on neve, 7.6 times, where its CPU is not seven times slower and the earlier `/dev/shm` experiment showed that where the files land changes nothing.
  The run's 5.9 million opens and 86,827 process starts are the two counts a per-operation scanner multiplies, and each is a few tens of microseconds, or a few milliseconds per start, away from explaining the whole gap.
  The report cannot say which of the two it is.
- **So the same levers carry over, with a different ranking.** Anything that removes git processes removes both their starts and the 22 system opens each makes before it reads a repository, 1.7 million opens per run: the pair templates, `dotgit` for `worktree list`, `CopyHub` for the two `NewHub` builders and the hook's tool spawns rank first there too, and higher than here.
  Fewer files per copy, which is a few seconds on neve, removes a fifth of the opens on OSL-1046, since the test binary makes 1.64 million of them.
  Folding tests removes its share of both.
- **What does not carry over.** `-parallel` and `t.Parallel`: a scanner that serializes opens or starts caps what parallel tests can overlap, and the CPU-side gain here is already taken.
  Memory and disk: the fixtures are 35 MB and tmpfs made no difference.
- **A measurement OSL-1046 could make.** One run with `GIT_TRACE2_EVENT` set, as here, gives the per-process `t_abs`; a mean per `rev-parse` far above neve's 0.6 ms says the start is scanned, and a mean near it says the opens are.
  That one number ranks the levers there.

## Proposed follow-up tasks, ranked

None is filed.
Savings are against the package's own run on neve; on OSL-1046 the first four weigh more, as the previous section reasons.

1. **Pair templates the fixtures use.** Open `hubforge.Shapes` to the slugs the package uses, or move the fixtures that carry no meaning in their slug onto `TemplatePairSlug`, and point `newPushPair`, `newLocalModePair`, `newMovesPair`, `endedHub`, `newRemoteHalfPair`, `discardHub`, `archivedDeleteFixture`, `newMergeTargetFixture`, the live-state arrange and the inline `CopyHub` plus `AddPair` sites at it.
   Expected: 25 to 35 percent of the package's git processes and serial time; the largest cut of process starts for OSL-1046.
2. **`worktree list` through `dotgit`.** A reader of `.git/worktrees` in `internal/dotgit` behind the production call sites, under `PATTERN-gitrepo-client-boundary`'s local-read rule.
   Expected: 12 to 15 percent of git processes, 8 to 12 percent of serial time, and the only lever that also speeds `lym` itself.
3. **Fold the overlapping tests.** The sixteen rows of the candidates table, with `TestVerbCases_CleanState` first.
   Expected: about 9 percent of serial cost and 3,000 git processes.
4. **`CopyHub` in `newRecordsFixture` and `newBoltFixture`, and shapes for the one-commit builders.** Expected: about 4 percent of git processes, 3 to 5 s serial.
5. **A cheaper post-checkout hook.** Shell parameter expansion for `dirname` and `basename`, one `rev-parse` for both branches.
   Expected: 2,600 fewer process starts per run, 1 to 2 s on neve.
6. **A slimmer hub copy.** No fingerprint walk on `CopyHub`, a packed template, no files a test never reads.
   Expected: 2 to 4 s on neve, about a fifth of the file opens on OSL-1046.
7. **go-git for the code side's reference reads.** `rev-parse --verify`, `merge-base`, `rev-list --count`, `for-each-ref`, `ls-tree`, `show`.
   Expected: 4 percent of git processes.
8. **`t.Parallel` in the serial top-level tests, and `-parallel=24` for the package in the gate's args.** Expected: 0 to 25 percent of wall on neve within this report's noise, nothing in CPU, little on OSL-1046.
   Measure on a quiet machine before deciding.
9. **One trace2 run on OSL-1046.** Not a code task: the one number that ranks the levers there.

## Limits

- Load: every wall above was measured while other gate runs held slots; the counts are exact, the seconds are an upper bound with a spread of about 30 percent.
- Attribution: 82 percent of git processes are tied to a test by their repository path; a child process with no path stays in the totals but not in the per-test rows.
  File opens are tied to a test the same way, and system and relative paths are not tied at all.
- Process start cost: the traces measure a git process from its `main`; the exec and loading before it is estimated, not measured.
- Fixture against code: the marker is process-wide during a `hubforge` build and is set by every `gitkit` helper, so "fixture" holds assertions' git as well as setup, and the production verbs a fixture drives.
- The overlap candidates come from a read of 35 tests, not from a coverage run; each names what it alone pins so a fold can keep it, and a fold's saving per row is not measured.
