# Batch: code-fixes

```yaml
task: 'Restore the Tier 1 floor: guards + perchengine'
batch: code-fixes
number: 1
cards: 3
verify: go test ./internal/clihelp ./internal/perchengine ./internal/boardengine/boardtest ./cmd/lyx -count=1 && go test -tags integration ./internal/perchengine -run TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay -count=1 -v
depends-on: []
```

## Batch Scope

The three code changes that restore the Tier 1 floor: disable cobra's Windows
mousetrap check at the shared `clihelp` seam (the repo-wide lever, measured Tier 1
~37 s → ~23 s), relocate the one real-time perchengine test to Tier 2 (measured
perchengine ~14.6 s → ~2.5 s isolated), and one bounded fixture-volume reduction in
boardtest's concurrency test. They form one batch because they are independent
one-file changes sharing a single measurement story; batch 2 (the benchmarks doc
block) consumes their combined effect and must run after all three. No external
interface changes — no CLI behaviour, JSON envelope, or command tree changes.

Batch-local decision: card 3 is conditional-keep — the implementer measures and
reverts the constant if the win criterion is not met (the only card in this plan
whose final diff may legitimately be empty; if reverted, still commit the card
with the doc-comment note described in its Requirements).

## Cards

### Card 1: Disable cobra mousetrap at the clihelp seam

- **Context:**
  - `cmd/lyx/main.go`
- **Edits:**
  - `internal/clihelp/exec.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a package-level `func init()` to `internal/clihelp/exec.go`
  that sets `cobra.MousetrapHelpText = ""`. Doc comment on the init must state:
  (a) on Windows, cobra's `preExecHook` calls `mousetrap.StartedByExplorer()` —
  one `CreateToolhelp32Snapshot` walk of the OS process table — on every
  `Command.Execute()`, purely to detect launch-by-double-click from Explorer;
  (b) the empty string is cobra's documented off-switch (the hook short-circuits
  before the syscall); (c) lyx is an orchestration CLI never launched by
  double-click, so the message is dead weight, while the snapshot dominated the
  test suite (a CPU profile of this package showed 99% of samples in that syscall;
  measured: this package 8.0 s → 1.0 s, full Tier 1 ~37 s → ~23 s); (d) pointer to
  `docs/benchmarks/test-suite-timing.md` for the dated measurement block. `clihelp`
  already imports `github.com/spf13/cobra`; no import changes. `cmd/lyx/main.go` is
  context only — it shows every production invocation flows through
  `clihelp.RunRoot`, so this single `init()` covers all modules and `cmd/lyx`;
  do not edit it.
- **Commit:** `perf(clihelp): disable cobra's per-Execute mousetrap snapshot on Windows`

### Card 2: Re-tier the lingering-child gate test to integration

- **Context:**
  - `internal/perchengine/gate.go`
  - `internal/perchengine/testmain_test.go`
  - `cmd/lyx/tierpurity_test.go`
- **Edits:**
  - `internal/perchengine/gate_test.go`
- **Creates:**
  - `internal/perchengine/gate_lingering_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Relocate the test function
  `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay` — including its
  full doc comment — **verbatim** from `internal/perchengine/gate_test.go` into a
  new file `internal/perchengine/gate_lingering_test.go`. No timing constant,
  subtest, assertion, or comment inside the function changes. The new file: first
  non-empty line is `//go:build integration`; package `perchengine`; imports
  exactly what the function needs (`os`, `runtime`, `strings`, `testing`, `time`);
  file-level doc comment (placed after the build constraint) stating why this test
  is Tier 2: it spawns real `cmd`/`ping` child processes and, by design, sits in
  the production `gateWaitDelay` (10 s) pipe-abandon grace window — real-time cost
  that violates the offline Tier 1 loop's premise (see the Test Tier Purity
  Invariant in `CONSTRAINTS.md`), and note that it previously evaded the
  tierpurity guard because it spawns via the production `execGateCommand` wrapper
  rather than a banned token. In `gate_test.go`: delete the moved function, drop
  the now-unused `runtime` import, keep all other imports (`os`, `path/filepath`,
  `strings`, `testing`, `time`, `burlerengine` remain used by the six staying
  tests), and update the file's top doc comment so it no longer implies the
  lingering-child coverage lives there. Do NOT touch `testmain_test.go` (its
  untagged `TestMain` + `lyxtest.HermeticGitEnv()` already covers the new tagged
  file — `TestMain` applies to the whole test binary regardless of which files
  are compiled in) and do NOT touch any guard test or allowlist. Not a `Moves:`
  entry: no file is renamed; a function moves between files.
- **Commit:** `perf(perchengine): re-tier lingering-child gate test to integration`

### Card 3: Bounded shrink of boardtest's concurrency fixture volume

- **Context:**
  - `internal/boardengine/boardtest/bench_test.go`
- **Edits:**
  - `internal/boardengine/boardtest/concurrency_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** One bounded attempt, then stop regardless of outcome.
  Before the change, record the package's isolated baseline: run
  `go test ./internal/boardengine/boardtest -count=1` twice and note the second
  (warm) time. In `TestConcurrentReadsDuringUpserts`, reduce the `writes` const
  from `50` to `10` — the writer-iteration knob is the dominant cost (each write
  re-renders the 100-task Home.md/_Sidebar.md to disk) and is coupled to no
  assertion. Do NOT change the seeded task count (`seedWiki(t, 100)`) — it is
  locked in lockstep with the readers' `GetTask("task-50")` call and the
  count-stays-100 assertion. Do NOT change `readers = 8` or any assertion.
  Re-measure the same way. Keep the reduction only if the package's isolated
  time drops by at least ~1 s AND the test still demonstrates real reader/writer
  overlap (readers complete multiple validated reads while the writer is
  mid-flight — confirm by reasoning about the writer's remaining duration: 10
  upserts × ~30-40 ms of render I/O each still gives readers a few hundred
  milliseconds of overlap window; state this reasoning in the updated comment
  above the `writes` const). If the win criterion is not met, revert to
  `writes = 50` and instead add one sentence to the comment above the const
  recording that the shrink was attempted on 2026-07-13 and did not pay
  (per the bounded-attempt decision in `_mill/discussion.md`), so the next
  optimisation pass does not repeat it. Commit the card either way.
- **Commit:** `perf(boardtest): shrink concurrency-test writer iterations (bounded attempt)`

## Batch Tests

The frontmatter `verify:` runs two commands:

1. `go test ./internal/clihelp ./internal/perchengine ./internal/boardengine/boardtest ./cmd/lyx -count=1`
   — the three edited packages plus `cmd/lyx`, whose guard tests
   (`TestTierPurity_UntaggedTestsSpawnNothing`,
   `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) are the machine-checks
   for the invariants card 2 touches; they must stay green with the relocated
   file present. Expected side-effect visible here: `internal/clihelp` drops to
   ~1 s and `internal/perchengine` to ~2-3 s.
2. `go test -tags integration ./internal/perchengine -run
   TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay -count=1 -v` —
   proves the relocated test compiles under the `integration` tag and still
   passes with its realistic timings (~12 s; invisible to the plain run, so it
   needs this explicit gate).

Scope justification: the mousetrap change is process-global for every cobra
consumer, but its behavioural surface is "the Explorer-double-click message" —
untestable in-process and irrelevant to lyx; the repo-wide suite in batch 2's
verify is the regression net for everything else.
