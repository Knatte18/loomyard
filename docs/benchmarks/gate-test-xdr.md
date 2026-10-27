# Gate test timing: OSL-1046 with Cortex XDR

This is a measurement report: it records single runs on one machine and goes stale as the suite changes.
It sits beside the cross-environment overview in [test-suite-timing.md](test-suite-timing.md) and the per-package Linux figures in [test-suite-weight.md](test-suite-weight.md).

Cortex XDR holds every executable whose hash it has not seen for 8-10 s before it runs, and passes a known hash at once.
Without `-trimpath` the checkout's directory is part of Go's build and test cache keys and is written into the binaries,
so every new task worktree starts with an empty test cache and links test binaries XDR has never seen.
The change measured here puts `-trimpath` into the GOFLAGS of every slotted gate run (`gateslot.GoFlags`) and into the tagged run's prebuild of `lym`.
A second change, measured in [Re-run on an unchanged tree](#re-run-on-an-unchanged-tree), runs verify as the `integration` tier alone and keeps the test cache keys stable from one run to the next.

## Machine

```yaml
host: OSL-1046, the operator's work laptop
cpu: Intel Core Ultra 7 366H, 16 cores, 1 thread per core, 400-4900 MHz
l2_l3: 24 MiB L2, 18 MiB L3
memory: 30 GiB, 31 GiB swap
disk: NVMe SSD, Micron 2650 1024 GB
os: Ubuntu 24.04.5 LTS, Linux 7.0.0-38-generic x86_64, bare metal
go: go1.26.9 linux/amd64
git: 2.43.0
tmux: 3.4
security: Cortex XDR installed (/opt/traps, pmd running)
gate_config: {slots: 3, go_parallel: 4, busy_quit_sec: 120}
```

## Method

Each tier ran through the installed production `lym`, from the prime worktree on `main`, under `/usr/bin/time -v`.
Wall is the elapsed time; CPU is user plus system time of `lym` and its whole child tree, `go test` and the test binaries included.
Each row is one run, and the rows ran one after another in the order listed.
The machine was quiet apart from the orch session, the creel daemon, a browser and an editor; the 1-minute load average was 2.93 before the first run.

- **Uncached** passes `-- -count=1`, which skips the test result cache but keeps the build cache, in a worktree whose binaries XDR had seen before.
  A `-count=1` run does not fill the test cache, so the first cacheable run after it ran every package again.
- **Cached** is the second plain run in a row, the first one having filled the test cache.
  Packages whose tests read changing inputs never hit the cache, so the cached tier still runs part of the suite.

## Before (`-p` cap only)

```yaml
commit: 28e60988
date: 2026-10-20
```

| Tier | Run | Wall | User | Sys | CPU (user+sys) | Packages run | Packages cached | Packages at 7 s or more |
|---|---|---|---|---|---|---|---|---|
| 1 (untagged) | `lym gate test ./... -- -count=1` | 304.6 s | 151.9 s | 52.3 s | 204.2 s | 162 | 0 | 67 |
| 1 (untagged) | `lym gate test ./...`, first cacheable | 147.5 s | 85.5 s | 46.4 s | 131.9 s | 162 | 0 | 26 |
| 1 (untagged) | `lym gate test ./...`, cached | 98.6 s | 73.1 s | 45.6 s | 118.8 s | 65 | 97 | 11 |
| 2 (`integration`) | `lym gate test --tags integration ./... -- -count=1` | 401.9 s | 321.7 s | 365.9 s | 687.6 s | 162 | 0 | 76 |
| 2 (`integration`) | `lym gate test --tags integration ./...`, first cacheable | 382.8 s | 277.4 s | 376.8 s | 654.2 s | 162, one failed | 0 | 63 |
| 2 (`integration`) | `lym gate test --tags integration ./...`, cached | 179.5 s | 219.4 s | 267.2 s | 486.6 s | 85 | 77 | 24 |

"Packages at 7 s or more" counts the `ok` lines whose elapsed time reached 7 s, the shape an XDR hold leaves on a package whose tests run in well under a second elsewhere, such as `internal/testkit/scankit` at 10.16 s in the first cacheable integration run.

The first cacheable integration run failed one test, `TestRun_KillsTheGroupOnTimeout` in `internal/testkit/lymbin`: its sleeper never recorded its pids inside the test's 5 s timeout.
That fits a hold on the freshly started sleeper; the uncached and cached integration runs passed.

Against the Ryzen AI 7 445 on bare-metal Linux with no endpoint security in [test-suite-weight.md](test-suite-weight.md#comparison), whose After totals are 83.35 s wall and 202.99 s CPU for tier 1 and 155.72 s wall and 392.95 s CPU for tier 2, the method differs: that report runs packages serially through `go run ./cmd/testtiming -resources` and sums them, while this one clocks one parallel `lym gate test` per tier.
Compare the two on order of magnitude only.

## After (`-p` cap and `-trimpath`)

```yaml
commit: 2d384150, the installed lym
date: 2026-10-20
```

The installed `lym` was built from the commit that added `-trimpath`, and the same method ran without the `-count=1` rows.
Each tier ran twice from the prime: the first run refills the cache under the new keys, the second shows the cached cost.
The 1-minute load average was 2.89 before the first run.

| Tier | Run | Checkout | Wall | User | Sys | CPU (user+sys) | Packages run | Packages cached | Packages at 7 s or more |
|---|---|---|---|---|---|---|---|---|---|
| 1 (untagged) | `lym gate test ./...`, first | prime | 20.6 s | 59.5 s | 34.9 s | 94.4 s | 72 | 90 | 0 |
| 1 (untagged) | `lym gate test ./...`, cached | prime | 20.3 s | 56.8 s | 34.0 s | 90.8 s | 65 | 97 | 0 |
| 1 (untagged) | `lym gate test -C .scratch/xcopy ./...` | copy | 24.2 s | 67.6 s | 41.1 s | 108.7 s | 99 | 63 | 0 |
| 2 (`integration`) | `lym gate test --tags integration ./...`, first | prime | 113.2 s | 218.5 s | 249.4 s | 467.9 s | 93, one failed | 69 | 3 |
| 2 (`integration`) | `lym gate test --tags integration ./...`, cached | prime | 112.0 s | 205.7 s | 245.6 s | 451.3 s | 86 | 76 | 3 |

The first unit run found 90 packages cached already: the verification runs before the commit had passed `-trimpath` on the go test command line, which gives the same cache keys.

The first integration run failed `TestBuildWithLDFlags_ReachTheBuild` in `internal/testkit/lymbin`, a fault of the change rather than of the machine.
GOFLAGS now carries `-trimpath` into the kit's nested `go build`, and a `-trimpath` build omits its `-ldflags` setting from the build info the test read.
The test now checks the effect of `-s -w` instead, a binary smaller than the unstripped build, and the second integration run ran with that fix and passed.

The copy row is the cross-checkout check.
`git archive HEAD` unpacked the tree into `.scratch/xcopy` inside the prime, where `./...` does not reach, and the gate resolved it to the prime's worktree, so it ran slotted with `-trimpath`.
Without `-trimpath` the new directory would change every cache key, and no package would hit the cache.
63 packages hit the cache built in the prime, and no package reached 7 s, so the copy linked no binary XDR held.
The 34 packages cached in the prime but not in the copy include the source scans (`internal/testkit`, `internal/pattern`, `contracts/stencils`) and others that read files of the tree;
go test keys a cached result on the files a test opened, and every file in the copy was new, as it is in a fresh task worktree.

## Comparison

Cached rows against cached rows.
A fresh worktree before ran every package, as the uncached before row does, and now behaves like the copy row; neither before row ran in a fresh worktree, so that pair is indicative only.

| Tier | Cached before | Cached after | Uncached before | Copy after |
|---|---|---|---|---|
| 1 (untagged) wall | 98.6 s | 20.3 s | 304.6 s | 24.2 s |
| 1 (untagged) CPU | 118.8 s | 90.8 s | 204.2 s | 108.7 s |
| 2 (`integration`) wall | 179.5 s | 112.0 s | 401.9 s | not measured |
| 2 (`integration`) CPU | 486.6 s | 451.3 s | 687.6 s | not measured |

The cached runs hit the cache for about as many packages before and after, 97 against 97 in tier 1 and 77 against 76 in tier 2.
What fell with the wall time is the packages at 7 s or more, from 11 to 0 in tier 1 and from 24 to 3 in tier 2.

## Re-run on an unchanged tree

```yaml
before: 1552ceb4, the installed lym, verify "lym gate test ./... && lym gate test --tags integration ./..."
after: c2fad0ad, a .dev-bin build of the branch, verify "lym gate test --tags integration ./..."
date: 2026-10-20
```

The `integration` tag compiles every untagged test too, and no test file is excluded by it except TestMain shims whose tagged TestMain does the same setup, so verify now runs that tier alone.

`GODEBUG=gocachetest=1` showed why a re-run on an unchanged tree ran so many packages again: the input ID Go computes from what a test read changed on every run.
Three inputs changed:

- `LYM_TRACE_ID`, which every `lym` process mints and exports, so each gate run gave go test a new value, read by every package that logs.
  `lym gate test` now drops it from go test's environment.
- The prebuilt `lym` path of a tagged run, a fresh directory per run, read by every package that runs `lym` through `internal/testkit/lymbin`.
  Inside a hub the binary now goes to a store in the gate-slot directory under a name derived from its content.
- The directory `internal/shedbuild`, where one test wrote and removed a fixture, so every source scan that lists `internal/` saw a changed directory.
  The test now names one of the package's own files.

The before rows ran one after the other from this task's worktree with the installed `lym`, each tier once, over a test cache other worktrees' `-trimpath` runs had filled.
The after rows ran from the same worktree with the branch's `lym`: a first run that refilled the cache after the change, and the re-run that a verify after a failure or a second gate makes.
Times are `/usr/bin/time -v` of the whole `lym` tree, as in the rows above.

| Run | Wall | User | Sys | CPU (user+sys) | Packages run | Packages cached | Packages at 7 s or more |
|---|---|---|---|---|---|---|---|
| before, tier 1 (untagged) | 22.6 s | 64.2 s | 37.3 s | 101.6 s | 102 | 60 | 0 |
| before, tier 2 (`integration`) | 163.4 s | 250.4 s | 332.3 s | 582.7 s | 123 | 39 | 4 |
| before, verify (both tiers) | 186.0 s | 314.6 s | 369.6 s | 684.3 s | 225 | 99 | 4 |
| after, `integration` refill | 347.8 s | 344.9 s | 359.2 s | 704.1 s | 120 | 42 | 50 |
| after, `integration` re-run | 26.6 s | 21.9 s | 27.6 s | 49.5 s | 0 | 162 | 0 |

The refill rebuilt every package that imports `internal/logger`, which the change touched, on a busy machine: the 1-minute load average was 10.88 when the next run started.
Its 50 packages at 7 s or more are new test binaries XDR held.

A first re-run, before the `internal/shedbuild` fix, still ran 9 packages, the source scans among them, and took 65.1 s.
Part of those misses came from the measurement itself: its new output files in `.scratch` changed the worktree's root directory listing, which the scans of the whole tree read.
The final rows wrote into files that already existed, which changes no directory listing.

A fresh worktree still misses for every test that opens a file of the tree, since Go keys such a result on the file's absolute path and modification time.

## `go_parallel` sweep

```yaml
commit: 905bc88d, the installed lym
date: 2026-10-20
new_binaries: a fresh, empty GOCACHE directory under .scratch/gp-sweep/ per run, plus -ldflags=-B=0x<random 8 bytes> per run
includes: compile time of the whole module and the standard library, since every run started with an empty build cache
```

Each run was `/usr/bin/time -v lym gate test --tags integration ./... -- -count=1 -ldflags=-B=0x<id>` from the prime, with `GOCACHE` set to the run's own empty directory, which `lym gate test` passes through to go test.
`go_parallel` changed between runs through `lym config gate --layer default --set go_parallel=<n>`, and was set back to 4 afterwards.
The local layer refuses the key, since every gate key is cross-pair, and gate has no worktree layer, so the hub default, committed and pushed in `_board`, is the only layer a gate run reads, and the change reached Neve's hub too for the length of the sweep.
No batten run was live before any run, and each run started once the 1-minute load average had fallen below 4.

A first run at 4 with the empty `GOCACHE` alone ran every package uncached but held almost nothing: with `-trimpath` the build is reproducible, so a fresh cache links byte-identical binaries whose hashes XDR has seen.
The `-B` linker flag writes a GNU build ID note into every linked binary, so each run's test binaries have hashes no earlier run produced.
A probe binary built that way waited 9.95 s on its first exec.
In the tier runs XDR still held few test binaries: small packages such as `tools/wordswap` finished in under 0.5 s at 4, and most packages at 7 s or more are the suite's slow ones, such as `internal/fabricengine` at 117-175 s.
The rise in packages at 7 s or more at 8 and 12 follows contention between packages running at once, not holds.

| `go_parallel` | Load before | Wall | User | Sys | CPU (user+sys) | Max RSS | Packages run | Cached | Failed | Packages at 7 s or more |
|---|---|---|---|---|---|---|---|---|---|---|
| 4, `GOCACHE` only (pilot) | 3.25 | 184.2 s | 568.3 s | 333.1 s | 901.4 s | 451 MiB | 162 | 0 | 0 | 14 |
| 4 | 3.32 | 219.8 s | 550.5 s | 356.1 s | 906.5 s | 453 MiB | 162 | 0 | 0 | 17 |
| 8 | 3.73 | 206.5 s | 647.3 s | 391.9 s | 1039.2 s | 456 MiB | 162 | 0 | 0 | 32 |
| 12 | 3.87 | 215.1 s | 561.6 s | 391.7 s | 953.3 s | 455 MiB | 162 | 0 | 0 | 79 |

No run failed a test, so no test was re-run.

The wall time is bound by `internal/fabricengine`, the longest package, which rose from 140 s at 4 to 175 s at 8 and 12 as more packages shared the cores.

8 was the fastest value with no new failures, 6 percent under 4, but the two runs at 4 differ by 36 s, so the sweep shows no gain from a higher `go_parallel` beyond run-to-run noise when XDR holds few binaries.
