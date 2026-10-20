# Gate test timing: OSL-1046 with Cortex XDR

This is a measurement report: it records single runs on one machine and goes stale as the suite changes.
It sits beside the cross-environment overview in [test-suite-timing.md](test-suite-timing.md) and the per-package Linux figures in [test-suite-weight.md](test-suite-weight.md).

Cortex XDR holds every executable whose hash it has not seen for 8-10 s before it runs, and passes a known hash at once.
Without `-trimpath` the checkout's directory is part of Go's build and test cache keys and is written into the binaries,
so every new task worktree starts with an empty test cache and links test binaries XDR has never seen.
The change measured here puts `-trimpath` into the GOFLAGS of every slotted gate run (`gateslot.GoFlags`) and into the tagged run's prebuild of `lyx`.

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

Each tier ran through the installed production `lyx`, from the prime worktree on `main`, under `/usr/bin/time -v`.
Wall is the elapsed time; CPU is user plus system time of `lyx` and its whole child tree, `go test` and the test binaries included.
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
| 1 (untagged) | `lyx gate test ./... -- -count=1` | 304.6 s | 151.9 s | 52.3 s | 204.2 s | 162 | 0 | 67 |
| 1 (untagged) | `lyx gate test ./...`, first cacheable | 147.5 s | 85.5 s | 46.4 s | 131.9 s | 162 | 0 | 26 |
| 1 (untagged) | `lyx gate test ./...`, cached | 98.6 s | 73.1 s | 45.6 s | 118.8 s | 65 | 97 | 11 |
| 2 (`integration`) | `lyx gate test --tags integration ./... -- -count=1` | 401.9 s | 321.7 s | 365.9 s | 687.6 s | 162 | 0 | 76 |
| 2 (`integration`) | `lyx gate test --tags integration ./...`, first cacheable | 382.8 s | 277.4 s | 376.8 s | 654.2 s | 162, one failed | 0 | 63 |
| 2 (`integration`) | `lyx gate test --tags integration ./...`, cached | 179.5 s | 219.4 s | 267.2 s | 486.6 s | 85 | 77 | 24 |

"Packages at 7 s or more" counts the `ok` lines whose elapsed time reached 7 s, the shape an XDR hold leaves on a package whose tests run in well under a second elsewhere, such as `internal/testkit/scankit` at 10.16 s in the first cacheable integration run.

The first cacheable integration run failed one test, `TestRun_KillsTheGroupOnTimeout` in `internal/testkit/lyxbin`: its sleeper never recorded its pids inside the test's 5 s timeout.
That fits a hold on the freshly started sleeper; the uncached and cached integration runs passed.

Against the Ryzen AI 7 445 on bare-metal Linux with no endpoint security in [test-suite-weight.md](test-suite-weight.md#comparison), whose After totals are 83.35 s wall and 202.99 s CPU for tier 1 and 155.72 s wall and 392.95 s CPU for tier 2, the method differs: that report runs packages serially through `go run ./cmd/testtiming -resources` and sums them, while this one clocks one parallel `lyx gate test` per tier.
Compare the two on order of magnitude only.

## After (`-p` cap and `-trimpath`)

```yaml
commit: 2d384150, the installed lyx
date: 2026-10-20
```

The installed `lyx` was built from the commit that added `-trimpath`, and the same method ran without the `-count=1` rows.
Each tier ran twice from the prime: the first run refills the cache under the new keys, the second shows the cached cost.
The 1-minute load average was 2.89 before the first run.

| Tier | Run | Checkout | Wall | User | Sys | CPU (user+sys) | Packages run | Packages cached | Packages at 7 s or more |
|---|---|---|---|---|---|---|---|---|---|
| 1 (untagged) | `lyx gate test ./...`, first | prime | 20.6 s | 59.5 s | 34.9 s | 94.4 s | 72 | 90 | 0 |
| 1 (untagged) | `lyx gate test ./...`, cached | prime | 20.3 s | 56.8 s | 34.0 s | 90.8 s | 65 | 97 | 0 |
| 1 (untagged) | `lyx gate test -C .scratch/xcopy ./...` | copy | 24.2 s | 67.6 s | 41.1 s | 108.7 s | 99 | 63 | 0 |
| 2 (`integration`) | `lyx gate test --tags integration ./...`, first | prime | 113.2 s | 218.5 s | 249.4 s | 467.9 s | 93, one failed | 69 | 3 |
| 2 (`integration`) | `lyx gate test --tags integration ./...`, cached | prime | 112.0 s | 205.7 s | 245.6 s | 451.3 s | 86 | 76 | 3 |

The first unit run found 90 packages cached already: the verification runs before the commit had passed `-trimpath` on the go test command line, which gives the same cache keys.

The first integration run failed `TestBuildWithLDFlags_ReachTheBuild` in `internal/testkit/lyxbin`, a fault of the change rather than of the machine.
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
