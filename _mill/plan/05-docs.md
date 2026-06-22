# Batch: docs

```yaml
task: "Optimise and slim the rest of the test suite"
batch: docs
number: 5
cards: 1
verify: go test ./... -count=1 && go test -tags integration ./... -count=1
depends-on: [2, 3, 4]
```

## Batch Scope

Record the post-task whole-suite timing and the equivalence/parallel-safety notes in
`docs/benchmarks/test-suite-timing.md`, and perform the final whole-repo two-tier validation.
This batch runs last (after the gating, fixture migrations, and pruning have all landed) so
the recorded numbers and the offline guarantee reflect the final state.

Depends on batches 2, 3, 4 (and transitively 1) — it measures their combined result.

## Cards

### Card 11: Append dated timing block + final two-tier validation

- **Context:**
  - `docs/benchmarks/test-suite-timing.md`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Run `go test ./... -count=1` (Tier 1) and
  `go test -tags integration ./... -count=1` (Tier 2); capture whole-suite wall-clock and the
  per-package times for `board`, `board/boardtest`, `ide`, `git`. Append a **new dated block**
  `### 2026-06-22 — after optimize-remaining-test-suites` to
  `docs/benchmarks/test-suite-timing.md` (do NOT edit any prior block — the file's own
  convention), mirroring the structure of the existing `2026-06-21` block:
  (1) before/after Tier-1 + Tier-2 wall-clock for `board`/`ide`/`git`;
  (2) a `#### Equivalence guardrail` note recording the **superset across the board↔boardtest
  union** (the seven git/sync tests relocated, no loss) plus the `render_test`/`store_test`
  table-driven folds (assertions preserved);
  (3) a `#### Parallel safety` note: the moved board tests and ide `cli_test`/`menu_test` stay
  serial (`t.Setenv` / `os.Chdir`); lyxtest per-test copies are isolated; `-race` not a
  precondition (no CGO).
  State explicitly that Tier 1 (`go test ./...`) is now offline repo-wide — confirmed by the
  gated tests being absent from the default `-list` (the board git/sync, `internal/git`, and
  ide cli/menu tests run only under `-tags integration`). Treat the numbers as
  order-of-magnitude (Windows-noisy), as the file's Context section already notes.
- **Commit:** `docs(benchmarks): record whole-suite timing after optimize-remaining-test-suites`

## Batch Tests

`verify: go test ./... -count=1 && go test -tags integration ./... -count=1` — the final
whole-repo gate: Tier 1 green and offline, Tier 2 green with every gated test present. This
doubles as the measurement command for the timing block. The unbounded scope is justified
here (and only here) because this batch's purpose is the whole-suite final validation +
measurement, not a focused code change.
