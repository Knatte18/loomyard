# Discussion: Speed up git-fixture tests: bench, analyse, hardlink

```yaml
task: 'Speed up git-fixture tests: bench, analyse, hardlink'
slug: faster-git-fixture-tests
status: discussing
parent: main
```

## Problem

Tier 2 (the opt-in integration loop, `go test -tags integration ./...`) runs ~208 s
on the operator's Windows box, floored by `internal/warpengine` (~152 s under
full-suite contention). The proposal's hypothesis was that the per-test fixture
copy (`lyxtest.copyDirRecursive` byte-copies 3–4 git repos into `tb.TempDir()`
per test) pays a per-file Cortex XDR/Defender scan and is the floor; the proposed
lever was hardlinking immutable `.git/objects/**`. The operator has **no admin**
on this machine, so environment levers (AV exclusions, RAM disk) are out — only
code-level levers are available.

Benchmarks were run **during this discussion** (operator directive) and they
**refute the fixture-copy hypothesis**: the copy is ~1–2 % of the tier. The
measured floor is **git process-spawn cost under AV**, and the single dominant
cost is **`core.fsmonitor=true` in the operator's global `~/.gitconfig`**:
every fresh test repo inherits it, so warpengine alone spawns **308
`fsmonitor--daemon` processes per run** (60 % of all git process-seconds),
plus 92 auto-`maintenance` spawns. Disabling fsmonitor + auto-maintenance via
environment cut warpengine from 102–111 s to 62–72 s alone (~-38 %), and from
~152 s to 87 s inside a full Tier 2 run. The task pivots to implementing that
lever: a **hermetic git environment for tests**.

## Benchmark report (2026-07-13, Windows-only)

All numbers from this machine: Intel Core Ultra 7 155U, `windows/amd64`, 14
logical CPUs, Windows 11 Enterprise, git 2.53.0.windows.2, Go 1.26.4, Cortex
XDR + Defender live, **no admin**. **These results are Windows-specific** —
the operator will record separate Linux benchmarks later; nothing here should
be assumed to transfer.

Method: throwaway stdlib-only Go harnesses (rebuilt the exact lyxtest template
recipe; copies placed via `os.MkdirTemp("")` in `%TEMP%`, exactly where
`tb.TempDir()` puts real fixtures, so the AV on-write cost measured is the one
the suite pays). Harness source lived in `.scratch/fixbench/` (ephemeral); the
method is recorded here and the permanent benchmark lands in
`internal/lyxtest/bench_test.go` (see Decisions).

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

All arms validated functionally (rev-parse + commit + push against the copy).
**Conclusion: refuted.** ~117 `lyxtest.Copy*` call sites × ~0.5 s contended ≈
a few seconds of aggregate wall across a ~208 s tier. Hardlink saves ~5 % of
that sliver, alternates ~33 % — noise either way.

### Process-spawn cost (the real floor)

| Spawn | serial p50 | parallel p50 (P=14) | machine throughput |
|---|---|---|---|
| no-op Go exe | 31 ms | 68 ms | ~184 spawns/s |
| `git rev-parse HEAD` | 72 ms | 208 ms | ~63 spawns/s |
| `git status --porcelain` | 85 ms | 239 ms | ~53 spawns/s |
| `git add` + `git commit` (2 spawns) | 323 ms | — | — |

Half the git spawn cost is pure Windows+AV process-creation tax (the no-op exe
number); config-read env vars (`GIT_CONFIG_NOSYSTEM`, `GIT_OPTIONAL_LOCKS=0`)
change nothing measurable.

### warpengine spawn census (GIT_TRACE2_EVENT, one trace file per git process)

1 831 git processes per warpengine run. Top offenders by summed process-seconds
(from a trace-inflated run; proportions are the signal, absolutes are not):

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
fresh test repo (template, clone, or raw `git init`) inherits it and spawns a
daemon on its first index-touching command. The 308 daemons linger (mean 8.9 s,
max 61 s), competing for CPU/AV attention with the tests.

### The winning lever, measured

`GIT_CONFIG_COUNT` env override: `core.fsmonitor=false`,
`maintenance.auto=false`, `gc.auto=0`:

| Run | Baseline | Override |
|---|---|---|
| warpengine alone (run 1 / run 2) | 102.0 s / 110.7 s | **61.9 s / 72.0 s** |
| warpengine inside full Tier 2 | ~152 s (2026-07-12 doc) | **87.2 s** |

Every git-heavy package benefits (boardcli 73 s, initengine 51 s, perchcli 43 s,
configcli 32 s in the override run — all at or below their documented baselines
while running under full contention).

### Pre-existing red discovered

`cmd/lyx` FAILs in both tiers on this branch (inherited from main):
`TestTierPurity_UntaggedTestsSpawnNothing` flags 4 untagged spawning test files
from the freshly-merged builder module: `internal/buildercli/spawnbatch_test.go`,
`internal/buildercli/validate_test.go`, `internal/builderengine/config_test.go`,
`internal/builderengine/template_test.go`. Unrelated to this task's changes;
folded into scope (see Decisions) because a green suite is a precondition for
recording before/after numbers.

## Scope

**In:**

- Hermetic git test environment (two layers, see Decisions): lyxtest template
  config + a lyxtest hermetic-env helper wired via `TestMain` into every
  git-spawning test package.
- A machine-check guard (in `cmd/lyx`, tierpurity-style) enforcing "package
  with git-spawning tests ⇒ hermetic `TestMain` or allowlisted", plus a new
  CONSTRAINTS.md invariant entry in the same commit.
- Re-tier the 4 builder test files behind `//go:build integration` to fix the
  pre-existing tier-purity red.
- Permanent `BenchmarkCopyPaired` / `BenchmarkCopyPairedLocal` (serial +
  parallel) in `internal/lyxtest/bench_test.go`, `//go:build integration`.
- Docs: new `docs/benchmarks/fixture-copy.md` (this report, marked
  Windows-only); new dated block + refreshed "Current best times" in
  `docs/benchmarks/test-suite-timing.md`; refreshed Tier 2 figures in
  `docs/benchmarks/running-tests.md`. Official before/after via
  `go run ./cmd/testtiming` / `-full`, median of 3, both tiers.

**Out:**

- **No hardlink or alternates implementation** — `copyDirRecursive` stays a
  plain byte-copy (hypothesis refuted; both arms are suite-level noise).
- No product-code changes: `internal/gitexec`, `warpengine`, `weftengine`
  engines are untouched. This is test-infrastructure only.
- No edits to the operator's `~/.gitconfig` (fsmonitor stays on for
  interactive use), no AV exclusions, no RAM disk (no admin).
- Tier 1's CPU costs (`internal/perchengine` ~20 s unit suite, `cmd/lyx` ~19 s
  repo-walking guards) — different problem, not this task.
- Linux benchmarks — operator records those later, separately.

## Decisions

### pivot-from-hardlink-to-hermetic-git-env

- Decision: implement the fsmonitor/maintenance kill as the winning lever;
  do not implement hardlink or alternates copying.
- Rationale: measured. Copy cost is ~1–2 % of the tier; fsmonitor daemons are
  ~60 % of warpengine's git process-seconds; the override cut warpengine ~38 %
  alone and ~43 % under full contention. The proposal explicitly required
  pivoting if the hypothesis failed empirically.
- Rejected: hardlink objects (5 % of a sliver), alternates (33 % of a sliver
  plus shared-object semantics to reason about forever), template repack
  (no measurable change).

### two-layer-hermetic-mechanism

- Decision: both layers. **Layer A (template config):** `lyxtest.initRepo` and
  `initBareRemote` set `core.fsmonitor=false`, `maintenance.auto=false`,
  `gc.auto=0` on every template repo at build time; copies inherit via
  `.git/config`, worktrees share it. **Layer B (hermetic env):** a lyxtest
  helper writes one neutral global-config file per test process and points
  `GIT_CONFIG_GLOBAL` at it (plus `GIT_CONFIG_NOSYSTEM=1`); every git-spawning
  test package calls it from `TestMain` before `m.Run()`.
- Rationale: Layer A alone misses repos created by raw `git init` / `git clone`
  inside tests (fresh configs re-read the user's global — boardcli's `seedCwd`,
  configcli, warpengine clone tests). Layer B covers every spawn but is
  forgettable; Layer A makes fixtures self-describing even if a future package
  forgets `TestMain`. Belt and braces; Layer A is 3 lines per template builder.
- Layer B reaches **indirect** git spawns too: `os.Setenv` in `TestMain`
  mutates the test process's environment, which `exec.Command` children
  inherit by default and pass on to their own children — so git spawned by a
  launched binary (e.g. `cmd/lyx`'s e2e tests running the lyx binary, which
  itself spawns git) still sees `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_NOSYSTEM`.
  This is what makes the no-daemon acceptance check meaningful for `cmd/lyx`,
  which is covered by a real `TestMain` (not allowlisted). Caveat for the
  plan: any test that constructs an `exec.Cmd` with an explicit `Env`
  whitelist would break this inheritance — none do today; the guard's
  allowlist review is the checkpoint if one appears.
- Rejected: env-only (silent regression on forgotten TestMain — mitigated by
  the guard, but fixtures should still be self-contained); template-only
  (measured hole: clone/init-created repos); injecting `-c` flags in
  `gitexec.RunGit` (changes production behaviour to serve tests).

### neutral-global-config-contents

- Decision: the Layer B neutral config file contains at least:
  `core.fsmonitor=false`, `maintenance.auto=false`, `gc.auto=0`,
  `user.name=Test`, `user.email=test@test.com`, `init.defaultBranch=main`.
- Rationale: identity + defaultBranch are the two things test git ops may
  currently be silently inheriting from the operator's global config. Raw
  `git init` in tests (without `-b main`) would flip to `master` when the
  global config disappears; clone-created repos would lose committer identity.
  The hermetic file must replace what it removes.
- Rejected: pointing `GIT_CONFIG_GLOBAL` at `/dev/null`/`NUL` (removes
  identity/defaultBranch and breaks the above); leaving `GIT_CONFIG_NOSYSTEM`
  unset (system config is another machine-specific leak; Git for Windows ships
  autocrlf etc. there — removing it makes checkout behaviour deterministic,
  and test-created files are LF already).
- Verification note for the plan: the measured win used an *additive*
  `GIT_CONFIG_COUNT` override, not full hermeticity. The full-hermetic variant
  must be verified by a green full Tier 2 run during implementation; if some
  obscure test depends on system/global config, fall back to additive
  `GIT_CONFIG_COUNT` env (same three keys + identity) — the win is identical,
  only the hermeticity guarantee differs.

### hermetic-guard-and-constraints-entry

- Decision: add a guard test in `cmd/lyx` (sibling of `tierpurity_test.go`,
  same walk-the-module-root technique): any package whose `*_test.go` files
  contain a git-spawn token must contain a `TestMain` that calls the lyxtest
  hermetic-env helper (raw-substring check on the helper's name), or be named
  on an allowlist with a reason. The token set is tierpurity's set **plus the
  lyxtest helpers that spawn git internally**: `gitexec.RunGit`,
  `exec.Command`, `lyxtest.Copy`, `lyxtest.MustRun`, `lyxtest.SeedConfig` —
  without the last two, a package whose only git spawn goes through those
  helpers would contain no token and silently skip the hermetic requirement.
  Record the invariant in CONSTRAINTS.md in the same commit.
- **Scan semantics differ from tierpurity in one deliberate way:** tierpurity
  early-skips files whose first line is a `//go:build integration`/`smoke`
  constraint (its subject is untagged files only). The hermetic guard must
  scan **all `*_test.go` files regardless of build tag** — the git-spawning
  set is almost exactly the tagged set (all 15 warpengine git test files are
  integration-tagged), so copying tierpurity's skip verbatim would make the
  guard vacuous. Its vacuous-scan protection asserts a non-zero count of
  git-spawning packages found (which, given the tagging reality, implies
  tagged files were scanned).
- The `lyxtest.MustRun` token is **broader than git** — `MustRun(tb, dir,
  args...)` runs any command — so it can flag a package whose only spawn is
  non-git. This over-breadth is intentional and self-correcting via the
  allowlist re-derivation below: such a package gets an allowlist entry with
  a reason, not a meaningless TestMain. A plan writer should not treat these
  flags as guard bugs.
- The raw-substring check on the helper's name proves **presence, not
  semantics** — a comment mentioning the helper would pass it. The semantic
  half (a real `func TestMain` that calls the helper before `m.Run()`) stays
  a review obligation, exactly like the repo's other grep-guards (see the
  Shell Mechanics Seam and Provider-Seam entries in CONSTRAINTS.md, whose
  semantic halves are review obligations too).
- Allowlist (enumerated; non-git spawners for which a git-hermetic `TestMain`
  is meaningless, distinct from packages that get a real `TestMain`):
  `internal/proc` (spawns generic processes — process control is the package's
  subject); `internal/muxengine` (spawns `tmux`/`psmux`, not git); the guard's
  own test file in `cmd/lyx` (contains the banned tokens as its own test
  data, like `tierpurity_test.go`). `cmd/lyx` itself is **not** allowlisted:
  its tests spawn `go` (crosscompile) but also git (`main_test.go`,
  `main_integration_test.go`), so it gets a real `TestMain`. The plan should
  re-derive the exact allowlist from the guard's first failing run rather
  than trusting this enumeration blindly.
- Rationale: without enforcement the daemons return silently with the next new
  package; "exists ⇒ covered or allowlisted" is this repo's established guard
  discipline (tierpurity, sandbox coverage, CLI registration).
- Rejected: godoc-only documentation (nothing machine-checks it); an `init()`
  side effect in lyxtest (magic, order-dependent, and packages that spawn git
  without importing lyxtest would still be missed).

### fold-in-builder-retier

- Decision: tag the 4 flagged builder test files `//go:build integration`
  as part of this task (first batch, before baselines are recorded).
- Rationale: `cmd/lyx` is red in both tiers on main; the timing harness
  records "all packages passed" and this task's deliverable is before/after
  numbers — baselines against a red suite are tainted. The fix is mechanical
  re-tiering per the Test Tier Purity Invariant's own error message.
- Rejected: separate wiki task (blocks this task's acceptance gate for a
  4-line mechanical change).

### permanent-copy-benchmark

- Decision: keep a permanent fixture-copy benchmark:
  `internal/lyxtest/bench_test.go`, `//go:build integration`, with
  `BenchmarkCopyPaired` and `BenchmarkCopyPairedLocal` in serial and
  `b.RunParallel` variants. Current-implementation only (no arms).
- Rationale: operator wants Linux comparison numbers later; the benchmark also
  regression-guards copy cost. Matches the `boardtest/bench_test.go` precedent.
  The 4-arm comparison stays in this report — the arms lost and their harness
  is not worth maintaining.
- Rejected: throwaway-only harness (before-number becomes unreproducible);
  keeping all 4 arms permanently (maintains dead strategies).

## Technical context

- `internal/lyxtest/lyxtest.go` — the one choke point: `initRepo` (line ~83),
  `initBareRemote` (~101), `mustGit` (~112), template builders `buildHostHub` /
  `buildWeftPrime` / `buildWeftOnly` (sync.Once, once per test binary),
  `copyDirRecursive` (~351, stays untouched), `Copy*` public helpers.
  Layer A = extra `mustGit(dir, "config", ...)` calls in `initRepo` /
  `initBareRemote` (≈18 extra spawns once per binary — negligible).
- Layer B helper must live in `internal/lyxtest` (stdlib-only — satisfies the
  lyxtest Leaf Invariant). It needs `os.Setenv` before any test spawns git,
  i.e. `TestMain`. **No package in the repo currently has a `TestMain`** —
  all are new, one-liner files (e.g. `testmain_test.go`), and they must work
  for packages whose git-spawning tests are integration-tagged: an untagged
  `testmain_test.go` runs in Tier 1 too and merely sets env (spawns nothing,
  tier-purity-safe as long as the helper's name is not a banned token —
  do not name it anything matching `lyxtest.Copy*`).
- Packages with git-spawning test files (24 files found; the guard computes
  the authoritative set mechanically): `warpengine`, `warpcli`, `weftengine`,
  `weftcli`, `perchcli`, `perchengine`, `muxcli`, `muxpoccli`, `initengine`,
  `initcli`, `idecli`, `ideengine`, `hubgeometry`, `gitexec`, `configcli`,
  `builderengine`, `buildercli`, `boardengine/boardtest`, `boardcli`,
  `lyxtest` itself, `cmd/lyx`, `burlerengine`/`shuttlecli` (smoke-tagged).
- Guard technique to copy: `cmd/lyx/tierpurity_test.go` — resolves module root
  via `go env GOMOD`, walks `*_test.go`, raw-substring token matching,
  allowlist map with reasons.
- Trace facts for the plan's sanity checks: warpengine = 1 831 git spawns,
  308 fsmonitor daemons; `git worktree` 232 spawns is the next irreducible
  block (real product behaviour under test — out of scope).
- Timing harness: `cmd/testtiming` (`go run ./cmd/testtiming [-full]`),
  median of 3 warm runs, `-count=1`, `go build ./...` first. This produced
  every documented baseline; use it for the official before/after.
- Baselines to compare against (2026-07-12, `docs/benchmarks/test-suite-timing.md`):
  Tier 1 ~36 s, Tier 2 ~208 s, warpengine ~152 s. Expected outcome: Tier 2
  roughly halves (~87 s warpengine floor measured); Tier 1 roughly unchanged
  (its cost is CPU, not git).
- `docs/roadmap.md`: update only if this completes a planned milestone —
  check at implementation time; a perf fix is otherwise not a roadmap entry
  (per CLAUDE.md).

## Constraints

- **lyxtest Leaf Invariant** (CONSTRAINTS.md): lyxtest imports stdlib +
  `internal/hubgeometry` only. The helper and template changes are stdlib —
  compliant. Machine-enforced by `internal/lyxtest/leaf_enforcement_test.go`.
- **Test Tier Purity Invariant**: untagged test files must not contain
  `gitexec.RunGit` / `exec.Command` / `lyxtest.Copy` as raw substrings. The
  new `bench_test.go` must be `//go:build integration`; new `testmain_test.go`
  files must avoid banned tokens; the 4 builder files get tagged. Enforced by
  `cmd/lyx/tierpurity_test.go`.
- **New invariant to record (same commit as the guard):** git-spawning test
  packages run under the hermetic git env (`TestMain` + lyxtest helper), so
  no test behaviour depends on the operator's `~/.gitconfig` or the system
  gitconfig. Enforced by the new `cmd/lyx` guard test.
- **Tier 1 stays offline**; **warpengine's junction coverage stays on
  Windows** (nothing here moves or re-tags warpengine tests).
- **Documentation Lifecycle / task completion** (CLAUDE.md): docs updates land
  in the same commits as the behaviour they describe.

## Testing

- **Guard test (TDD candidate #1):** write the hermetic-env guard first; it
  must fail listing every git-spawning package lacking `TestMain`, then go
  green as TestMains are added. Include the vacuous-scan protection style of
  the sibling guards (fail if the walk finds zero git-spawning packages).
- **Hermetic helper unit test:** after the helper runs, `git config --get
  core.fsmonitor` inside a fresh `git init` repo reports `false`, and
  identity/defaultBranch come from the neutral file (integration-tagged —
  it spawns git).
- **Template config test:** a `Copy*` fixture's `.git/config` contains the
  three keys (can be a pure file-read assertion on the copy; also
  integration-tagged since it calls `Copy*`).
- **No-daemon acceptance check:** re-run the warpengine suite under
  `GIT_TRACE2_EVENT` once during implementation and assert (manually, in the
  benchmark report) that `fsmonitor--daemon` and `maintenance` spawn counts
  are 0. This is the empirical proof the lever landed; not a permanent test
  (trace2 tracing itself costs ~2.5× wall).
- **Parallel isolation unchanged:** `go test -tags integration
  ./internal/warpengine -count=2` green (copies are still full byte-copies;
  nothing shared beyond what already was).
- **Full gates:** both tiers green (`go test ./...` and `-tags integration`),
  including the previously-red `cmd/lyx` (builder files re-tiered) — then
  `cmd/testtiming` before/after, median of 3, recorded in docs.
- **Benchmarks:** `go test -tags integration -bench BenchmarkCopy -run '^$'
  ./internal/lyxtest` runs on demand; numbers recorded in
  `docs/benchmarks/fixture-copy.md`.

## Q&A log

- **Q:** Benchmark arms — hardlink-only or the full comparison? **A:** Measure
  four arms (baseline, hardlink, hardlink+repack, alternates); implement the
  winner. Operator also directed: run the benchmark **during mill-start** and
  write the report (this file's Benchmark report section).
- **Q:** `os.Link` failure handling (if hardlink had won)? **A:** Per-file
  silent fallback to byte-copy, `git clone --local` semantics. (Moot after
  refutation; recorded for history.)
- **Q:** `os.SameFile` isolation guard for the hardlink boundary? **A:** Yes,
  both directions. (Moot after refutation — superseded by the hermetic-env
  guard + template-config test.)
- **Q:** Where do overview + deep analysis land? **A:** Split: dated block in
  `test-suite-timing.md` + refreshed `running-tests.md`, deep analysis in new
  `docs/benchmarks/fixture-copy.md`. **All numbers marked Windows-only; the
  operator will record separate Linux benchmarks later.**
- **Q:** Benchmark permanence? **A:** Permanent `Benchmark*` functions in
  `internal/lyxtest`, integration-tagged, serial + parallel.
- **Q:** Fix mechanism after the pivot? **A:** Both layers — template config
  in lyxtest builders + hermetic `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_NOSYSTEM`
  env via `TestMain` in every git-spawning test package.
- **Q:** Enforcement? **A:** Yes — `cmd/lyx` guard test + CONSTRAINTS.md
  entry, same commit.
- **Q:** Pre-existing builder tier-purity red? **A:** Fold the fix into this
  task — tag the 4 files `//go:build integration` before recording baselines.
- **Q:** Implement hardlink/alternates anyway? **A:** No — byte-copy stays;
  report + permanent baseline benchmark only.
- **Q:** (review r1 gap) Guard token set misses `lyxtest.MustRun` /
  `lyxtest.SeedConfig`, which spawn git inside lyxtest? **A:** Add both to the
  guard's token set (recommended option; operator standing directive: accept
  recommended resolution on all mill-start review findings).
- **Q:** (review r2 gap) Hermetic guard copied tierpurity's technique, which
  skips tagged files — but git-spawning files are precisely the tagged set,
  making the guard vacuous? **A:** Scan all `*_test.go` regardless of build
  tag; vacuous-scan floor asserts non-zero git-spawning packages found
  (recommended option, per standing directive).
