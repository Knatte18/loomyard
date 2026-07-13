# Batch: benchmarks-block

```yaml
task: 'Restore the Tier 1 floor: guards + perchengine'
batch: benchmarks-block
number: 2
cards: 1
verify: go test ./... -count=1
depends-on: [1]
```

## Batch Scope

The measurement-and-record batch: run the timing harness on the post-fix tree,
then append the new dated block to `docs/benchmarks/test-suite-timing.md`
per the doc's append-only discipline. Separate from batch 1 because the numbers
it records must be measured with all three code changes in place. Docs-only;
no code interface. The batch-local decision differing from nothing in Shared
Decisions: none — it implements the append-only-benchmarks-discipline decision
directly.

## Cards

### Card 4: Measure and append the 2026-07-13 restore-tier1-floor block

- **Context:**
  - `cmd/testtiming/main.go`
  - `_mill/discussion.md`
  - `internal/clihelp/exec.go`
  - `internal/perchengine/gate_lingering_test.go`
  - `internal/boardengine/boardtest/concurrency_test.go`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** First measure, then write.
  **Measure:** `go build ./...` to warm the build cache, then run
  `go run ./cmd/testtiming` three times and `go run ./cmd/testtiming -full`
  three times; take the median wall-clock per tier (the harness prints wall-clock,
  per-package times, and the slowest top-level tests per run — capture the median
  run's tables). All runs must report all packages passed; a red run blocks the
  card.
  **Write, preserving the doc's structure:** (1) retitle the current
  "Current best times" content (the 2026-07-13 hermetic-git-env block) into a new
  frozen History entry `### 2026-07-13 — hermetic git test environment (was
  "Current best times")` placed as the newest History entry, content unchanged;
  (2) write the new "Current best times" section, dated with today's date from the
  system clock and carrying a distinguishing descriptor in the established
  `As of **DATE** (<descriptor>)` form — use "restore-tier1-floor: mousetrap
  disabled + lingering-child test re-tiered" — so the two same-day headings stay
  distinguishable once this block later freezes into History; machine/Go/method
  lines in the established format ("median of 3 warm runs per tier via
  `go run ./cmd/testtiming[ -full]`"). The new block must
  contain: **Headline** table (Tier 1 and Tier 2 wall-clock, each with a
  "vs. previous" column against the superseded block's ~29 s / ~128 s); a
  **Cause** section attributing the Tier 1 drop to (a) disabling cobra's
  Windows mousetrap check in `internal/clihelp` (one `CreateToolhelp32Snapshot`
  process-table walk per `Command.Execute()`; a CPU profile of `internal/clihelp`
  showed 99% of samples in that syscall; measured package effect 8.0 s → 1.0 s)
  and (b) re-tiering
  `TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay` to Tier 2 (two
  parallel subtests each sitting in the production `gateWaitDelay` 10 s
  pipe-abandon grace window — ~12 s of perchengine's ~14.6 s isolated time), plus
  (c) the boardtest `writes` shrink result from card 3 (kept or reverted — state
  which and the measured effect); an explicit **supersession note** that the
  2026-07-12/13 blocks' causal claims — "cmd/lyx guard tests AST-parse/walk the
  repo" (guards measure 0.00–0.11 s each in isolation, ~0.25 s combined; the
  attributed 12–19 s was parallel-contention attribution noise) and
  "perchengine's cost is its large table-driven suite" (44 of its 45 tests sum
  to under 1 s) — are corrected by this block, with the frozen blocks left
  unedited per append-only discipline; one sentence noting the lingering-child
  test had evaded the tierpurity guard by spawning through the production
  `execGateCommand` wrapper (no guard change made — the guard's narrowness is
  deliberate); and the **Tier 1/Tier 2 where-the-time-goes and slowest-10
  tables** from the median runs in the established format. Numbers are
  Windows-only per the doc's convention. Do not edit any frozen History block's
  content; do not touch `docs/roadmap.md` or any other doc.
- **Commit:** `docs(benchmarks): record restore-tier1-floor block; correct guard/perchengine attribution`

## Batch Tests

Frontmatter `verify: go test ./... -count=1` — the repo-wide untagged suite is
the terminal gate for the whole task: it proves the tree the recorded numbers
describe is green, and its own wall-clock is the number being recorded (expected
~11-13 s, down from ~37 s). Unbounded scope is justified: this is the task's
final batch and the task's subject IS the repo-wide suite. The Tier 2 side is
exercised by the three `-full` harness runs required by the card itself (each a
full `-tags integration` pass); a red Tier 2 run blocks the card, so the moved
perchengine test is re-proven here too.
