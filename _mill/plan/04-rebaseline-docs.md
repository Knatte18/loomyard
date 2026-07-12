# Batch: rebaseline-docs

```yaml
task: 'Fix test-suite regression: slow Tier 1 + 2 red packages + stale benchmarks'
batch: rebaseline-docs
number: 4
cards: 1
verify: null
depends-on: [1, 2, 3]
```

## Batch Scope

Record the post-fix reality in the benchmark docs: fresh timing medians for
both tiers, the 2026-07-12 regression block moved into the trend log, and the
"premise violated" warnings in running-tests.md replaced by the restored-state
figures. Depends on all three prior batches — the numbers this batch records
must be measured on the tree where the reds are fixed, the re-tiering has
landed, and the guard is active. Docs only; no code changes.

## Cards

### Card 10: re-baseline benchmark docs with post-fix numbers

- **Context:**
  - `_mill/discussion.md`
  - `docs/benchmarks/board-performance.md`
  - `cmd/testtiming/main.go`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/benchmarks/running-tests.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** First measure: run `go run ./cmd/testtiming` three times
  and `go run ./cmd/testtiming -full` three times (warm build — run
  `go build ./...` first; the harness sets `-count=1`), recording each
  wall-clock; use the median per tier. Both tiers must report
  `RESULT: all packages passed` — a FAIL row means an earlier batch broke
  something and this card must not proceed. Then edit
  `docs/benchmarks/test-suite-timing.md`: write a new `## Current best times`
  block dated with the actual run date (same structure as the current block:
  machine/method lines, Headline table comparing against the 2026-07-12
  regression figures ~44 s / ~181 s, a short per-package "where the time
  goes" note per tier from the harness output, slowest-tests table) and
  state explicitly that the two red packages are fixed and the offline tier
  is spawn-free again (guarded by
  `cmd/lyx/tierpurity_test.go` / `TestTierPurity_UntaggedTestsSpawnNothing`).
  Move the entire 2026-07-12 "Current best times" block into `## History
  (trend log)` as `### 2026-07-12 — regression baseline (pre-fix state)`,
  newest-first (above the 2026-06-23 state block), content unchanged. In
  `docs/benchmarks/running-tests.md`: delete the `> **Current state
  (2026-07-12) …**` blockquote in `## The two tiers` and restore the tier
  bullets to plain premise statements with the fresh figures (Tier 1 fast
  again — state the measured median; Tier 2 — state the measured median);
  update the two command comments in `## Commands` and `## Timing harness`
  (drop "currently violated" / "regressed" wording, state fresh durations)
  and the `## Reducing wall-clock` item 3 (drop the premise-violated
  parenthetical, keep the warp/weft/hubgeometry/board/ide package list and
  update the Tier 2 budget wording to the measured figure).
  `docs/benchmarks/board-performance.md` needs no edit (checked 2026-07-12) —
  it is Context only.
- **Commit:** `docs(benchmarks): re-baseline after tier restoration and red fixes`

## Batch Tests

`verify: null` — pure docs batch with no runnable surface. The measurement
protocol inside the card (three runs per tier, medians, both tiers must pass)
is the batch's real gate: it re-runs the entire suite in both tiers on the
final tree, which is strictly stronger than any scoped verify command. Code
correctness is already gated by batches 1–3's verifies plus the module-wide
overview verify.
