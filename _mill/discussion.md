# Discussion: Restore the Tier 1 floor: guards + perchengine

```yaml
task: 'Restore the Tier 1 floor: guards + perchengine'
slug: restore-tier1-floor
status: discussing
parent: main
```

## Problem

Tier 1 (`go test ./...`, the default offline loop) was ~3.5 s on 2026-06-23 and is
~29–37 s today (the 2026-07-13 benchmarks block records ~29 s; a fresh baseline run
in this worktree measured ~37 s — same noise band). The wiki proposal attributed the
regression to (a) three `cmd/lyx` guard tests each AST-walking the repo and (b)
`internal/perchengine`'s large table-driven unit suite. **Discussion-phase
measurement refuted both attributions** and found the real causes:

1. **cobra's mousetrap check (repo-wide, Windows-only).** Every
   `cobra.Command.Execute()` on Windows calls `mousetrap.StartedByExplorer()` —
   a `CreateToolhelp32Snapshot` walk of the entire OS process table — to detect
   launch-by-double-click from Explorer. Every test that drives a command through
   the `clihelp.Execute`/`RunCLI` seam pays it; the suite makes hundreds of such
   calls, concurrently, across ~15 `*cli` packages. A CPU profile of
   `internal/clihelp` shows 99% of samples inside this syscall
   (`runtime.cgocall` ← `mousetrap.getProcessEntry`). Measured: disabling it took
   `internal/clihelp` from 8.0 s → 1.0 s and the full Tier 1 wall from ~37 s → ~23 s.
2. **One real-time test in perchengine.**
   `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay`
   (`internal/perchengine/gate_test.go`) runs two parallel subtests that each sit in
   the production `gateWaitDelay = 10 s` pipe-abandon grace window with real
   `cmd`/`ping` child processes. It accounts for ~12 s of perchengine's ~14.6 s
   isolated run; the package's other 44 tests sum to under 1 s. The 2026-07-12
   "table-driven CPU" hypothesis is wrong.

The four `cmd/lyx` guards (`tierpurity`, `hermeticenv`, `registration`,
`sandbox_coverage`) cost 0.00–0.11 s **each** in isolation (~0.25 s combined);
`cmd/lyx`'s inflated per-package number in full runs is contention attribution
noise, already warned about in the benchmarks doc's own noise note.

Measured projection with both fixes applied (simulated in this worktree): Tier 1
wall ~11.7 s, with `internal/boardengine/boardtest` (~5.2 s under contention,
~2.1 s isolated) as the next floor.

## Scope

**In:**

- Disable cobra's mousetrap check production-wide at the `internal/clihelp` seam
  (`cobra.MousetrapHelpText = ""`), covering every module's `RunCLI` and `cmd/lyx`.
- Re-tier `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay` behind
  `//go:build integration` (move the whole test to Tier 2, unchanged).
- Bounded extension (one attempt, then stop regardless of outcome): shrink
  `TestConcurrentReadsDuringUpserts`' fixture volume in
  `internal/boardengine/boardtest` (seeded task count / write iterations) without
  changing the test's concurrent shape, and re-measure.
- New dated append-only block in `docs/benchmarks/test-suite-timing.md` with
  before/after numbers, the mousetrap root-cause analysis, and an explicit
  correction of the earlier cmd/lyx-guards / perchengine-tables attribution.

**Out:**

- The proposal's "shared parse-pass for the cmd/lyx guards" — refuted by
  measurement (~0.25 s combined guard cost); not built.
- No changes to any guard test's enforcement semantics, allowlists, or banned-token
  sets (see Decisions: tierpurity-evasion-left-alone).
- No changes to perchengine production code (`gateWaitDelay` stays a `const`; no
  injection seam).
- No profiling/slimming of the rest of perchengine's suite — measured at <1 s
  total; nothing to slim.
- No chasing of the residual ~11–12 s contention floor beyond the bounded boardtest
  attempt (link/startup overhead of ~50 test binaries plus scheduler contention is
  not addressable per-package).
- `docs/roadmap.md` — untouched unless the wiki marks this task as a planned
  milestone (bugfix/hardening work is recorded by the benchmarks doc and git
  history per CLAUDE.md).

## Decisions

### mousetrap-disabled-production-wide

- Decision: set `cobra.MousetrapHelpText = ""` once in `internal/clihelp` (package
  `init()` in `exec.go`), not per-test and not per-call inside `RunRoot`.
- Rationale: `clihelp` is the single seam every module and `cmd/lyx` already goes
  through (CLI/Cobra Invariant), so one `init()` covers all tests and all
  production invocations; production also stops paying a pointless process-table
  snapshot per `lyx` call. lyx is an orchestration CLI never launched by
  double-click from Explorer, so losing cobra's "run from command line" message is
  no loss. Measured effect: Tier 1 ~37 s → ~23 s.
- Rejected: test-only disable via a `lyxtest` helper wired into every `*cli` test
  package (~15 packages touched, production keeps the syscall); per-call assignment
  in `RunRoot` (works, but an `init()` states the intent once instead of
  re-assigning a package global on every invocation).

### lingering-child-test-re-tiered

- Decision: move `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay` to
  Tier 2 by relocating the test function verbatim into a new
  `//go:build integration`-tagged file in the same package, leaving the rest of
  `gate_test.go` untagged. The test itself changes only its file location;
  timings, subtests, and assertions stay identical.
- Rationale: the test spawns real `cmd`/`ping` processes and waits in a 10 s
  real-time grace window by design — exactly the "expensive spawns" shape the Test
  Tier Purity Invariant sends to Tier 2. Re-tiering spawning tests is the repo's
  established pattern (builder files, boardcli, perchcli precedents). Zero flake
  risk, no production seam, no assertion dropped — the equivalence guardrail is
  satisfied by relocation, mirroring prior re-tiering tasks. Tier 2 absorbs the
  ~12 s invisibly (it runs ~50 packages in parallel with an ~84 s floor).
  Tagging the whole of `gate_test.go` is wrong because its other six tests are
  cheap and belong in Tier 1.
- Rejected: making `gateWaitDelay` an injectable `var` and shrinking
  pings/grace to keep the test in Tier 1 (~1–2 s) — adds a test seam to production
  code and tight timing margins with flake risk on a loaded Windows machine;
  keeping a shrunk Tier 1 copy plus a realistic Tier 2 copy — double machinery for
  a guard Tier 2 already covers.

### tierpurity-evasion-left-alone

- Decision: do not extend `tierpurity_test.go`'s `bannedTokens` to catch spawning
  via production wrappers (the lingering-child test evaded the guard because it
  spawns through `execGateCommand`, so the literal `exec.Command` token never
  appears in the test file).
- Rationale: the guard is documented as deliberately narrow ("raw substring", "this
  is deliberately narrower than 'spawn no processes'"); banning production wrapper
  names is whack-a-mole and would couple the guard to every engine's internals.
  After re-tiering, no expensive/real-time spawn remains in an untagged file: the
  four remaining untagged `execGateCommand` tests (pass, fail, not-found, 1 ns
  timeout) still spawn cheap `go` processes through the wrapper, intentionally and
  guard-invisibly — the cheap-subprocess shape the invariant deliberately permits.
  The evasion is worth a sentence in the benchmarks-doc block, not a guard change.
- Rejected: adding `execGateCommand` to `bannedTokens` (couples the repo-wide guard
  to one package's unexported helper; the helper's own untagged unit tests — pass,
  fail, not-found, 1 ns timeout — are cheap and legitimate Tier 1 tests that would
  then all need allowlisting).

### boardtest-bounded-shrink

- Decision: one bounded attempt at `TestConcurrentReadsDuringUpserts`
  (`internal/boardengine/boardtest/concurrency_test.go`): reduce fixture volume —
  primarily the `writes = 50` writer-iteration const (the dominant re-render cost,
  and uncoupled from any assertion); the seeded task count (100) only secondarily,
  because it is coupled to two other literals that must move in lockstep (readers
  call `GetTask("task-50")` and assert the task count stays 100) — while preserving
  the test's shape (1 writer goroutine, 8 reader goroutines, readers validating
  mid-write, non-mutating upserts so the count assertion holds). Re-measure; keep
  the reduction only if it wins ≥ ~1 s of package time without weakening the race
  exposure below usefulness (writer must still be mid-flight while readers read —
  verify the overlap is real, e.g. the writer loop still takes long enough that
  readers complete many reads during it). Stop after this one attempt either way.
- Rationale: operator asked for the extension "if it is not a big job"; it is one
  test, the cost is upsert-render I/O volume (each upsert re-renders a 100-task
  Home.md/sidebar to disk 50 times under 8 concurrent readers), and the knobs are
  two constants. All assertions survive; only iteration counts change.
- Rejected: broader boardtest/next-tier contention hunting — the remaining floor is
  scheduler/link overhead across ~50 binaries, not fixable per-package; diminishing
  returns explicitly out of scope.

### benchmarks-doc-correction

- Decision: append one new dated block to `docs/benchmarks/test-suite-timing.md`
  (append-only discipline, newest first, "Current best times" pointer updated)
  containing: before/after headline (method: median of 3 warm runs via
  `go run ./cmd/testtiming`), the mousetrap root cause with the clihelp profile
  evidence, the perchengine one-test attribution, and an explicit note that the
  2026-07-12/13 blocks' "cmd/lyx guards AST-parse the repo" and "perchengine
  table-driven CPU" causal claims are superseded (the frozen blocks themselves are
  not edited).
- Rationale: append-only trend-log discipline is stated in the doc itself; the
  wrong attribution must be corrected where the next reader will look, not by
  rewriting history.
- Rejected: editing the frozen 2026-07-12/13 blocks in place.

## Technical context

- **`internal/clihelp/exec.go`** — the shared seam: `Execute()` → `RunRoot()` →
  `cmd.ExecuteContext()`. Every module's `RunCLI` and `cmd/lyx/main.go` go through
  `RunRoot`. An `init()` here is process-wide for every consumer. `clihelp` already
  imports `github.com/spf13/cobra`, so no new dependency.
- **cobra/mousetrap mechanics**: `cobra.preExecHook` (Windows build) checks
  `if MousetrapHelpText != "" && mousetrap.StartedByExplorer()`. Empty string is the
  documented off-switch — the snapshot syscall is then never made.
- **`internal/perchengine/gate_test.go`** — holds seven tests; only
  `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay` (lines ~123–188)
  moves. It is already Windows-only (`runtime.GOOS` skip). The move must keep the
  package's `TestMain` coverage intact — `testmain_test.go` is untagged, so the
  hermetic env still wires up for the new integration-tagged file
  (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` scans **all** test files
  regardless of build tags and the package already carries the presence token;
  no guard change needed). The new file needs a doc comment stating why it is
  Tier 2 (real spawns + 10 s real-time grace window by design).
- **`internal/boardengine/boardtest/concurrency_test.go`** — `seedWiki(t, 100)`
  seeds 100 tasks; the writer does 50 `UpsertTask` calls each re-rendering
  Home.md/_Sidebar.md; `SkipGit: true` so no spawns. Primary knob is the
  `writes = 50` const (uncoupled); the seeded `100` is coupled to the readers'
  `GetTask("task-50")` and the count-stays-100 assertion, which must move in
  lockstep if it changes; `readers = 8` is a possible third knob.
- **Verification harness**: `go run ./cmd/testtiming` (Tier 1) and
  `go run ./cmd/testtiming -full` (Tier 2) produce the medians the benchmarks doc
  uses; `go build ./...` first to warm the build cache. Numbers are Windows-only
  per the doc's convention.
- **Measured reference points for the plan's verify steps** (this machine, Intel
  Core Ultra 7 155U, 14 logical CPUs): baseline Tier 1 ~37 s; mousetrap fix alone
  ~23 s; both fixes ~11.7 s wall. Isolated: `perchengine` 14.6 s → ~2.5 s expected
  after re-tier; `clihelp` 8.0 s → 1.0 s measured; `boardtest` ~2.1 s isolated.
  Expect run-to-run noise of several seconds; assert on the order-of-magnitude
  drop, not exact numbers.
- **Tier 2 must stay green**: the re-tiered test now runs under
  `-tags integration`; one full Tier 2 run is part of final verification (it also
  proves the moved test still passes with its realistic timings).

## Constraints

From `CONSTRAINTS.md` (all still apply; none are weakened by this task):

- **Test Tier Purity Invariant** — the re-tier *strengthens* compliance in spirit
  (a process-spawning, real-time test leaves Tier 1). The guard's
  `allowedSpawners`/`bannedTokens` sets are not modified.
- **Hermetic Git Test Environment Invariant** — `hermeticenv_test.go` scans all
  test files including tagged ones; perchengine keeps its untagged
  `testmain_test.go` with `lyxtest.HermeticGitEnv()`, so the moved file changes
  nothing for this guard.
- **CLI / Cobra Invariant** — the `clihelp` `init()` touches the shared seam;
  no command tree, `Short`, or JSON-envelope behaviour changes, so no help-tree or
  drift test updates. `mousetrap` disabling alters only the double-click-from-
  Explorer path, which no lyx command depends on.
- **Documentation Lifecycle / Task completion** — benchmarks doc updated in the
  same commit as the code changes; no `docs/modules/` doc exists for `clihelp`,
  `perchengine`, or `boardtest` (checked), so none needs updating; this is
  performance hardening, not a roadmap milestone.

## Testing

- **No new test files for the mousetrap change itself** — the change is observable
  as suite wall-clock; a unit test asserting `cobra.MousetrapHelpText == ""` would
  test a constant. The `init()` gets a doc comment carrying the profile evidence
  (why: `CreateToolhelp32Snapshot` per Execute; measured 8 s → 1 s in clihelp).
- **Re-tier verification**: `go test ./internal/perchengine -count=1` (Tier 1,
  expect ~2–3 s, lingering-child test absent from `-list`);
  `go test -tags integration ./internal/perchengine -count=1 -run
  TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay -v` (test present and
  green in Tier 2). Existing guards (`tierpurity`, `hermeticenv`) must stay green —
  they are the machine-checks for the invariants this task touches.
- **boardtest attempt**: `go test ./internal/boardengine/boardtest -count=1`
  before/after; the concurrency test must still demonstrate reader/writer overlap
  (assert the shape survives by reading the test, not by adding instrumentation).
- **Full verification**: 3 warm Tier 1 runs + 3 warm Tier 2 runs via
  `go run ./cmd/testtiming[ -full]`, all green, medians recorded in the new
  benchmarks block. Success criterion per the wiki brief: Tier 1 from ~36 s
  "back toward ~5–10 s" — the measured projection is ~11–12 s, which the operator
  accepted as meeting the goal (the residual is cross-package contention, not any
  single package).
- **TDD candidates**: none — no new behaviour with a unit-testable contract; the
  deliverables are a one-line production change, a test relocation, a fixture-size
  reduction, and a docs block, each verified by the measurements above.

## Q&A log

- **Q:** Replace the proposal's "shared parse-pass for cmd/lyx guards" with the
  evidence-based reshape (mousetrap + perchengine WaitDelay test)? **A:** Yes —
  reshape; parse-pass refuted (~0.25 s combined guard cost) and dropped.
- **Q:** Disable mousetrap production-wide in clihelp, or test-only? **A:**
  Production-wide in `clihelp` (operator also asked what mousetrap is — answered:
  cobra's Windows launched-from-Explorer detector, one process-table snapshot per
  `Execute()`).
- **Q:** Fix the perchengine lingering-child test by re-tiering to integration,
  shrinking timings in place, or both? **A:** Re-tier to `//go:build integration`,
  unchanged.
- **Q:** Stop at the ~11.7 s projection, or also attempt the boardtest floor?
  **A:** Conditional — "if it is not a big job, do the extension." Assessed as
  small (two constants in one test); included as a bounded one-attempt scope item
  (see boardtest-bounded-shrink).
