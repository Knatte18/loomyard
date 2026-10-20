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

> **To be measured** once the change is deployed: the same six runs from the prime worktree, and the uncached and first cacheable runs from a second checkout, which without `-trimpath` would start with an empty cache and unseen binaries.

| Tier | Run | Checkout | Wall | User | Sys | CPU (user+sys) | Packages run | Packages cached | Packages at 7 s or more |
|---|---|---|---|---|---|---|---|---|---|
