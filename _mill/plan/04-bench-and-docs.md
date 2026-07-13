# Batch: bench-and-docs

```yaml
task: 'Speed up git-fixture tests: bench, analyse, hardlink'
batch: 'bench-and-docs'
number: 4
cards: 2
verify: go test -tags integration -run '^$' -bench BenchmarkCopy -benchtime 1x -count=1 ./internal/lyxtest
depends-on: [3]
```

## Batch Scope

Deliverables 1–3 of the task in permanent form, plus the official
before/after: the permanent fixture-copy benchmark in lyxtest, the deep
analysis document (`docs/benchmarks/fixture-copy.md`, ported from the
benchmark report in `_mill/discussion.md`), and the refreshed timing docs with
a new dated block recorded via the repo's own harness. Depends on batch 3 so
the recorded "after" numbers include the full hermetic environment and a green
suite in both tiers. Every number produced here is explicitly marked
Windows-only (the operator records separate Linux benchmarks later).

## Cards

### Card 11: Permanent fixture-copy benchmarks

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/boardengine/boardtest/bench_test.go`
- **Creates:**
  - `internal/lyxtest/bench_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file, first non-empty line `//go:build integration`
  (it calls the `Copy*` fixtures — Test Tier Purity Invariant), `package
  lyxtest`. Four benchmarks: `BenchmarkCopyPaired` and
  `BenchmarkCopyPairedLocal` (serial `for b.Loop()` or classic `b.N` loop
  calling `CopyPaired(b)` / `CopyPairedLocal(b)`), plus
  `BenchmarkCopyPairedParallel` and `BenchmarkCopyPairedLocalParallel` using
  `b.RunParallel` (each goroutine calling the same helper) — the parallel
  variants exist because contended cost is what the suite actually pays
  (serial ~128 ms vs ~500 ms contended on the reference machine). The
  `Copy*` helpers take `testing.TB`, so passing `b` works. File-top comment:
  these are the permanent probes behind the numbers in
  `docs/benchmarks/fixture-copy.md`; run via
  `go test -tags integration -bench BenchmarkCopy -run '^$' ./internal/lyxtest`;
  note that `b.TempDir()` cleanup accumulates to the benchmark's end, which
  matches how real tests defer fixture cleanup to test end.
- **Commit:** `test(lyxtest): permanent CopyPaired/CopyPairedLocal benchmarks`

### Card 12: Analysis doc + official before/after timing refresh

- **Context:**
  - `_mill/discussion.md`
  - `cmd/testtiming/main.go`
- **Creates:**
  - `docs/benchmarks/fixture-copy.md`
  - `internal/buildercli/testdata_test.go`
  - `internal/buildercli/pause_spawnbatch_test.go`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/benchmarks/running-tests.md`
  - `internal/buildercli/validate_test.go`
  - `internal/buildercli/poll_test.go`
  - `internal/buildercli/status_test.go`
  - `internal/buildercli/pause_test.go`
- **Deletes:** none
- **Moves:** none
- **Discovered-scope note:** the official Tier 1 run (`go run
  ./cmd/testtiming`) surfaced a real `go test ./...` compile failure in
  `internal/buildercli` that predates this card: batch 1's mechanical
  `//go:build integration` tagging of `spawnbatch_test.go` /
  `validate_test.go` (commit `77393ab`) hid helper functions
  (`mustGit`/`newScratchRepo`/`commitFile`/`newSpawnBatchFixture`/
  `seedBuilderFixture`/`builderengineTestdataDir`/`seedPlanFixture`) that
  several **untagged** sibling test files in the same package
  (`poll_test.go`, `status_test.go`, `run_test.go`, and one test in
  `pause_test.go`) still reference, breaking the untagged (Tier 1) build.
  This was invisible to every prior verify command in this task because
  they all pass `-tags integration`, which compiles the hiding files back
  in. Fix, split by whether the referencing test actually spawns real git:
  the two pure-file-I/O helpers (`builderengineTestdataDir`,
  `seedPlanFixture`) move to a new untagged `testdata_test.go` so
  `run_test.go` keeps compiling at Tier 1 without gaining a git dependency;
  `poll_test.go` and `status_test.go` are genuinely git-spawning throughout
  (every test builds a real git fixture) and get `//go:build integration`;
  `pause_test.go`'s one git-dependent test
  (`TestSpawnBatchCmd_ObservesPauseFlagWrittenByPauseCmd`) moves to a new
  integration-tagged `pause_spawnbatch_test.go`, leaving its two
  git-free tests in Tier 1. No test assertion changes; this is a pure
  compile-visibility fix required for card 12's "green suite in both
  tiers" precondition (batch scope). `internal/buildercli`'s `smoke`-tagged
  tier has the same latent gap (`smoke_test.go` also references the
  git-spawning helpers) but is unaffected by any tier this task measures
  (`go test ./...` and `go test -tags integration ./...` both exclude it)
  and is left alone — out of this task's scope. **Follow-up gap found
  while fixing the above:** `pollFakeMux` (a git-free
  `shuttleengine.MuxOps` double defined in `poll_test.go`) is also
  referenced by the untagged `run_test.go`; tagging `poll_test.go`
  integration hid it the same way. Moves to the untagged
  `testdata_test.go` alongside the other shared, git-free test doubles.
- **Requirements:** Three parts. **(1) `fixture-copy.md`:** port the entire
  "Benchmark report (2026-07-13, Windows-only)" section of
  `_mill/discussion.md` — machine spec, Windows-only banner ("separate Linux
  benchmarks will be recorded later; nothing here transfers"), method
  (throwaway stdlib harnesses; copies placed via `os.MkdirTemp("")` in
  `%TEMP%` to pay the same AV cost as `tb.TempDir()`), template
  file-count/bytes table, the four copy-arm results table, the process-spawn
  cost table (no-op exe vs git), the warpengine spawn census (1 831 spawns,
  308 fsmonitor daemons at 60 % of git process-seconds), the winning-lever
  measurements (102–111 s → 62–72 s alone; ~152 s → 87 s in-tier), and the
  explicit refutation conclusion (hardlink/alternates rejected — copy cost is
  ~1–2 % of the tier). End with a "Reproducing" section: the card 11
  benchmark command for the copy numbers, and fresh benchmark output from
  actually running those benchmarks once (record the numbers). Link the doc
  from the places it is referenced (test-suite-timing.md's new block). **(2)
  `test-suite-timing.md`:** produce the official after-numbers with the
  repo's harness — `go run ./cmd/testtiming` and `go run ./cmd/testtiming
  -full`, median of 3 warm runs each (`go build ./...` first; the harness
  sets `-count=1`) — then add a new dated "Current best times" block per the
  file's append-only discipline (demote the current 2026-07-12 block into
  History unchanged): headline table (expected shape: Tier 1 roughly
  unchanged ~36 s since its cost is CPU; Tier 2 roughly halved from ~208 s —
  the measured warpengine floor with the hermetic env was 87 s in-tier),
  where-the-time-goes tables from the harness output, and a cause paragraph
  naming the hermetic git env (fsmonitor/maintenance kill), the builder
  re-tier fix, and linking `fixture-copy.md` for the analysis. All numbers
  marked Windows-only. **(3) `running-tests.md`:** refresh the Tier 2
  wall-clock figures/prose to the new numbers and add one sentence pointing
  at the Hermetic Git Test Environment Invariant (CONSTRAINTS.md) so tier
  documentation names the mechanism. Do NOT touch `docs/roadmap.md` — no
  planned milestone covers this task (verified during planning; CLAUDE.md
  forbids roadmap entries for perf/hardening work). **Contingency:** the
  timing runs here are the first full-tier executions under the hermetic env.
  If either tier goes red with a failure traceable to the removed
  global/system git config (the `neutral-global-config-contents` decision in
  `_mill/discussion.md` names Git-for-Windows `autocrlf` as the candidate
  class), switch `HermeticGitEnv` from `GIT_CONFIG_GLOBAL`+
  `GIT_CONFIG_NOSYSTEM` to the additive `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_n`/
  `GIT_CONFIG_VALUE_n` form carrying the same keys (quiet trio + identity +
  `init.defaultBranch`) — the measured win is identical, only the hermeticity
  guarantee weakens — update the helper's godoc and the CONSTRAINTS entry
  wording accordingly, and re-run the timing.
- **Commit:** `docs(benchmarks): fixture-copy analysis + hermetic-env before/after timing`

## Batch Tests

`verify:` compiles the integration-tagged bench file and executes each
`BenchmarkCopy*` exactly once (`-benchtime 1x`, `-run '^$'`) — a smoke gate
proving the benchmarks build and the fixtures they drive still work, in
seconds. The full timing evidence is card 12's recorded 3-run medians via
`cmd/testtiming`, written into the docs rather than run per round. Docs
correctness (tables, Windows-only marking, append-only history discipline) is
reviewed, not machine-checked.
