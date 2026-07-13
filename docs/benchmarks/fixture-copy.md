# Fixture-copy benchmark report

Deep analysis behind the "Speed up git-fixture tests" task. This is the full
benchmark report recorded during the task's discussion phase (2026-07-13),
ported here verbatim as the permanent record, plus a "Reproducing" section
showing how to regenerate the copy numbers with the permanent benchmark this
task shipped (`internal/lyxtest/bench_test.go`). For the headline
before/after wall-clock numbers this task produced, see
[test-suite-timing.md](test-suite-timing.md#current-best-times); for the
"how to run the suite" documentation, see
[running-tests.md](running-tests.md).

> **Windows-only.** Every number on this page was measured on the operator's
> Windows machine. Separate Linux benchmarks will be recorded later; nothing
> here should be assumed to transfer to another OS or filesystem.

## Benchmark report (2026-07-13, Windows-only)

All numbers from this machine: Intel Core Ultra 7 155U, `windows/amd64`, 14
logical CPUs, Windows 11 Enterprise, git 2.53.0.windows.2, Go 1.26.4, Cortex
XDR + Defender live, **no admin**.

Method: throwaway stdlib-only Go harnesses (rebuilt the exact lyxtest
template recipe; copies placed via `os.MkdirTemp("")` in `%TEMP%`, exactly
where `tb.TempDir()` puts real fixtures, so the AV on-write cost measured is
the one the suite pays). Harness source lived in `.scratch/fixbench/`
(ephemeral, not committed); the method is recorded here and the permanent
benchmark lands in `internal/lyxtest/bench_test.go` (see "Reproducing"
below).

### Template shape (file count vs bytes)

| Template | Files | Dirs | Bytes |
|---|---|---|---|
| hub | 13 | 18 | 1 619 |
| bare | 4 | 9 | 440 |
| weft-prime | 15 | 22 | 1 823 |
| weft-bare | 4 | 9 | 440 |
| **Paired fixture total** | **36** | **58** | **~4.3 KB** |

### Fixture-copy arms (paired fixture = hub + bare + weft-prime + weft-bare)

Serial N=10; parallel P=14 workers × 3 ops each (42 ops):

| Arm | ser p50 | ser p90 | par p50 | par p90 | par wall (42 ops) |
|---|---|---|---|---|---|
| A full byte-copy (current) | 128 ms | 135 ms | 502 ms | 522 ms | 1.40 s |
| B hardlink objects/** | 121 ms | 124 ms | 402 ms | 532 ms | 1.33 s |
| C hardlink + repacked template | 127 ms | 135 ms | 466 ms | 521 ms | 1.39 s |
| D objects/info/alternates | 117 ms | 125 ms | 318 ms | 366 ms | 0.94 s |

All arms validated functionally (rev-parse + commit + push against the
copy). **Conclusion: refuted.** ~117 `lyxtest.Copy*` call sites × ~0.5 s
contended ≈ a few seconds of aggregate wall across a ~208 s tier. Hardlink
saves ~5 % of that sliver, alternates ~33 % — noise either way.

### Process-spawn cost (the real floor)

| Spawn | serial p50 | parallel p50 (P=14) | machine throughput |
|---|---|---|---|
| no-op Go exe | 31 ms | 68 ms | ~184 spawns/s |
| `git rev-parse HEAD` | 72 ms | 208 ms | ~63 spawns/s |
| `git status --porcelain` | 85 ms | 239 ms | ~53 spawns/s |
| `git add` + `git commit` (2 spawns) | 323 ms | — | — |

Half the git spawn cost is pure Windows+AV process-creation tax (the no-op
exe number); config-read env vars (`GIT_CONFIG_NOSYSTEM`,
`GIT_OPTIONAL_LOCKS=0`) change nothing measurable.

### warpengine spawn census (GIT_TRACE2_EVENT, one trace file per git process)

1 831 git processes per warpengine run. Top offenders by summed
process-seconds (from a trace-inflated run; proportions are the signal,
absolutes are not):

| Subcommand | Spawns | Sum (s) | Mean (ms) |
|---|---|---|---|
| **fsmonitor--daemon** | **308** | **2 728** | **8 856** |
| worktree | 232 | 404 | 1 740 |
| rev-parse | 401 | 259 | 646 |
| push | 47 | 214 | 4 556 |
| reset | 68 | 182 | 2 676 |
| receive-pack | 47 | 174 | 3 708 |
| maintenance (auto) | 92 | 53 | 577 |
| (all others) | ~636 | ~565 | — |

`core.fsmonitor=true` comes from the operator's global `~/.gitconfig`; every
fresh test repo (template, clone, or raw `git init`) inherits it and spawns
a daemon on its first index-touching command. The 308 daemons linger (mean
8.9 s, max 61 s), competing for CPU/AV attention with the tests.

### The winning lever, measured

`GIT_CONFIG_COUNT` env override: `core.fsmonitor=false`,
`maintenance.auto=false`, `gc.auto=0`:

| Run | Baseline | Override |
|---|---|---|
| warpengine alone (run 1 / run 2) | 102.0 s / 110.7 s | **61.9 s / 72.0 s** |
| warpengine inside full Tier 2 | ~152 s (2026-07-12 doc) | **87.2 s** |

Every git-heavy package benefits (boardcli 73 s, initengine 51 s, perchcli
43 s, configcli 32 s in the override run — all at or below their documented
baselines while running under full contention).

### Pre-existing red discovered

`cmd/lyx` FAILed in both tiers on the branch this task started from
(inherited from `main`): `TestTierPurity_UntaggedTestsSpawnNothing` flagged
4 untagged spawning test files from the freshly-merged builder module:
`internal/buildercli/spawnbatch_test.go`, `internal/buildercli/validate_test.go`,
`internal/builderengine/config_test.go`, `internal/builderengine/template_test.go`.
Unrelated to this task's core hypothesis; folded into scope (batch 1,
`fold-in-builder-retier`) because a green suite is a precondition for
recording before/after numbers.

A second, related gap surfaced later, during this doc's own before/after
timing run (batch 4): batch 1's mechanical `//go:build integration` tagging
of those same two `buildercli` files hid helper functions that other,
untagged sibling test files in the same package still referenced
(`poll_test.go`, `status_test.go`, `run_test.go`, one test in
`pause_test.go`), breaking the untagged (Tier 1) build. This was invisible
to every batch's own `verify:` command because they all pass `-tags
integration`, which compiles the hiding files back in — only the official,
untagged `go run ./cmd/testtiming` run (this card) exercises the exact
`go test ./...` invocation that caught it. Fixed by splitting helpers by
whether they actually spawn git: the pure file-I/O ones moved to a new
untagged `internal/buildercli/testdata_test.go`; the genuinely git-spawning
test files got `//go:build integration`. No test assertion changed.

## The winning lever: hermetic git test environment

The task pivoted from the original hardlink-objects hypothesis (refuted
above) to a **hermetic git test environment**, implemented in two layers:

- **Layer A (template config):** `lyxtest.initRepo` and `initBareRemote` set
  `core.fsmonitor=false`, `maintenance.auto=false`, `gc.auto=0` on every
  template repo at build time; copies inherit this via `.git/config`,
  worktrees share it.
- **Layer B (hermetic env):** `lyxtest.HermeticGitEnv()` writes one neutral
  global-config file per test process and points `GIT_CONFIG_GLOBAL` at it
  (plus `GIT_CONFIG_NOSYSTEM=1`), wired via `TestMain` into every
  git-spawning test package. This reaches indirect git spawns too — child
  processes (and any binaries those children launch) inherit the env vars.

Enforced by `cmd/lyx/hermeticenv_test.go`
(`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) and recorded as the
**Hermetic Git Test Environment Invariant** in `CONSTRAINTS.md`. See that
invariant for the full mechanics and allowlist.

`copyDirRecursive` (the fixture-copy engine itself) is untouched: a plain
byte-copy, no hardlink, no alternates — the measured arms above show that
lever is not worth its added complexity.

## Reproducing

The permanent copy-cost probes (this task's Deliverable 1) live in
`internal/lyxtest/bench_test.go`, `//go:build integration`:
`BenchmarkCopyPaired`, `BenchmarkCopyPairedLocal` (serial), and their
`BenchmarkCopyPairedParallel` / `BenchmarkCopyPairedLocalParallel`
counterparts (`b.RunParallel`) — contended cost is what the suite actually
pays (serial ~128 ms vs ~500 ms contended on this machine, per the arms
table above).

```sh
go test -tags integration -bench BenchmarkCopy -run '^$' ./internal/lyxtest
```

Fresh output from actually running that command once (`-benchtime 10x` to
get a stable per-op number; the default time-based `-benchtime` stops after
1 iteration here because the very first call across the whole run also pays
the one-time, `sync.Once`-cached template-build cost, which dwarfs a single
copy — run more than one iteration if you want a number that isn't
dominated by that one-time cost):

```
goos: windows
goarch: amd64
pkg: github.com/Knatte18/loomyard/internal/lyxtest
cpu: Intel(R) Core(TM) Ultra 7 155U
BenchmarkCopyPaired-14                 	      10	 452476860 ns/op
BenchmarkCopyPairedLocal-14            	      10	 234929330 ns/op
BenchmarkCopyPairedParallel-14         	      10	  56055880 ns/op
BenchmarkCopyPairedLocalParallel-14    	      10	  61574420 ns/op
PASS
ok  	github.com/Knatte18/loomyard/internal/lyxtest	9.390s
```

Note the parallel numbers here are Go's own `ns/op` (total time divided by
total operations across all `GOMAXPROCS` workers), which is a different
metric from the discussion-phase harness's per-op p50/p90 latency under
contention (the arms table above) — both are legitimate ways to look at the
same cost, they are not directly comparable line-for-line.
