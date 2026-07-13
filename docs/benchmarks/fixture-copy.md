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

> **Windows + Linux.** The benchmark report below (2026-07-13) was measured on
> the operator's 155U Windows machine with Cortex XDR live. The previously-flagged
> Linux gap is **now filled** ([Linux benchmark](#linux-benchmark-2026-07-13)), and
> a third machine isolates the AV cost
> ([9800X3D A/B](#windows-clean-cpu-benchmark-ryzen-7-9800x3d-defender-ab)). Key
> finding: the huge 155U copy cost was **Cortex XDR** (an aggressive corporate EDR)
> plus a weak CPU — consumer **Defender barely touches this path** (~6 % A/B), and
> even AV-free Windows is still ~14× slower than Linux on the raw filesystem floor.
> Compare down each machine's column, never across.

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

## Linux benchmark (2026-07-13)

The Linux counterpart to the Windows "Reproducing" numbers above, from the same
permanent benchmark (`internal/lyxtest/bench_test.go`,
`//go:build integration`). Command:

```sh
go test -tags integration -bench BenchmarkCopy -run '^$' -benchtime 10x ./internal/lyxtest
```

- Machine: AMD Ryzen AI 7 445 w/ Radeon 840M, Ubuntu 26.04 LTS, `linux/amd64`, 12 logical CPUs, Go 1.26.0

```
goos: linux
goarch: amd64
pkg: github.com/Knatte18/loomyard/internal/lyxtest
cpu: AMD Ryzen AI 7 445 w/ Radeon 840M
BenchmarkCopyPaired-12                 	      10	   6060124 ns/op
BenchmarkCopyPairedLocal-12            	      10	   2164937 ns/op
BenchmarkCopyPairedParallel-12         	      10	   1691159 ns/op
BenchmarkCopyPairedLocalParallel-12    	      10	    456375 ns/op
PASS
```

Side by side with the Windows `ns/op` numbers above (**down the column per OS,
not across** — but the gap is the whole point here):

| Benchmark                        | Windows (Cortex XDR) | Linux (no AV) | Windows ÷ Linux |
|----------------------------------|----------------------|---------------|-----------------|
| `BenchmarkCopyPaired`            | 452 ms               | **6.06 ms**   | ~75×            |
| `BenchmarkCopyPairedLocal`       | 235 ms               | **2.16 ms**   | ~109×           |
| `BenchmarkCopyPairedParallel`    | 56 ms                | **1.69 ms**   | ~33×            |
| `BenchmarkCopyPairedLocalParallel` | 61 ms              | **0.46 ms**   | ~132×           |

### Windows clean-CPU benchmark (Ryzen 7 9800X3D, Defender A/B)

A third machine (2026-07-13), run to isolate the antivirus cost: same box, once
with Defender active and once with the repo + `%TEMP%` excluded. No Cortex XDR on
this machine, so it is a clean single-variable Defender on/off comparison.

- Machine: AMD Ryzen 7 9800X3D, Windows 11, 16 logical CPUs, Go 1.26.3

| Benchmark (ns/op)                  | Defender ACTIVE | Defender EXCLUDED |
|------------------------------------|-----------------|-------------------|
| `BenchmarkCopyPaired`              | 91.3 ms         | **85.5 ms**       |
| `BenchmarkCopyPairedLocal`         | 49.9 ms         | 47.0 ms           |
| `BenchmarkCopyPairedParallel`      | 11.95 ms        | 11.92 ms          |
| `BenchmarkCopyPairedLocalParallel` | 11.18 ms        | 11.78 ms          |

**This corrects an earlier claim.** An earlier version of this doc said "~99 % of
the measured Windows copy cost was the Cortex XDR / Defender on-write scan." The
9800X3D A/B run shows that lumping Cortex and Defender together was wrong:

- **Defender alone is nearly free for fixture-copy** — 91.3 → 85.5 ms with it
  excluded is ~6 % (within noise). Defender's real-time scanner does not tax this
  byte-copy path meaningfully (its cost shows up in in-process/compile work
  instead — see [test-suite-timing.md](test-suite-timing.md#windows-clean-cpu-baseline-ryzen-7-9800x3d-defender-ab)).
- **The 452 ms on the 155U was Cortex XDR (an aggressive corporate EDR) plus a
  weak 15 W CPU**, not Defender. Cortex is a different, far heavier scanner than
  consumer Defender; do not generalize "AV" from it.
- **Clean Windows is still ~14× slower than Linux** here (85.5 ms vs 6.06 ms) with
  **no AV on either side**. That gap is the Windows filesystem/process floor —
  NTFS, junctions, per-file open/close — not antivirus, and it does not go away.

### This settles the hardlink question

The report above **refuted** the hardlink-objects lever on the 155U (it saved
~5 %; alternates ~33 %) and pivoted to the hermetic git-env lever. The multi-machine
numbers make the refutation categorical for the *right* reason. On Linux a paired
copy is ~6 ms — nothing left to shave. On clean Windows it is ~85 ms, but that cost
is the OS filesystem/process floor, not the git object bytes a hardlink/alternates
lever targets (which the 155U arms already showed saving only ~5 %). So
`copyDirRecursive` stays a plain byte-copy on every platform. The task brief's open
question — "does the hardlink lever matter on Linux" — is answered **no**, with
numbers; and the sharper lesson is that the *original* Windows copy cost was AV
(Cortex), not the copy, so the lever was aimed at the wrong target from the start.
