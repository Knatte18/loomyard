# Batch: board-prune

```yaml
task: "Optimise and slim the rest of the test suite"
batch: board-prune
number: 4
cards: 2
verify: go test ./internal/board
depends-on: [1]
```

## Batch Scope

Conservatively shrink `internal/board`'s oversized Tier-1 unit suite by folding **pure
overlap** in the two largest files — `render_test.go` (20 funcs) and `store_test.go`
(19 funcs) — into table-driven subtests. This is the TDD-sensitive part: the rule is
**fold only cases with identical assertions over different inputs; drop nothing
behaviourally distinct**, and the equivalence guardrail (post-set superset of pre-set,
documented folds where assertions are preserved) is the safety net. No numeric target — stop
when only genuinely-distinct cases remain. These are pure offline unit tests; no fixture or
production change.

Depends on batch 1 so the board package's test set is already in its post-move shape (board's
`git_test`/`sync_test` are gone, so the `-list` baseline captured here is stable).

## Cards

### Card 9: Fold `render_test.go` overlap into table-driven cases

- **Context:**
  - `internal/board/render.go`
- **Edits:**
  - `internal/board/render_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Before editing, capture the baseline:
  `go test -list '.*' ./internal/board` and a `go test -run 'Test.*' -v ./internal/board`
  `=== RUN` listing (subtest names). Collapse only pure-overlap `Test*` cases in
  `render_test.go` (same assertion shape, differing input/expectation) into table-driven
  subtests using `t.Run(name, …)`, choosing subtest names that **preserve every prior
  top-level test name as a subtest name** (so the post-fold `=== RUN` set is a superset of
  the baseline). Do not merge cases whose assertions differ. Do not touch
  `BOARD_SKIP_GIT`/env usage. After editing, diff the new `-list`/`=== RUN` against the
  baseline and confirm superset (only intentional top-level→subtest folds differ, with
  assertions preserved).
- **Commit:** `test(board): fold render_test overlap into table-driven cases`

### Card 10: Fold `store_test.go` overlap into table-driven cases

- **Context:**
  - `internal/board/store.go`
- **Edits:**
  - `internal/board/store_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Same approach and guardrail as Card 9, applied to `store_test.go`'s 19
  funcs. Capture the `-list` + `=== RUN` baseline first (or reuse Card 9's whole-package
  baseline), fold only pure-overlap cases into table-driven subtests preserving every prior
  name as a subtest name, drop nothing behaviourally distinct, and prove the post-fold set is
  a superset. Keep all distinct store behaviours (load, save, round-trip, ordering,
  error paths) covered.
- **Commit:** `test(board): fold store_test overlap into table-driven cases`

## Batch Tests

`verify: go test ./internal/board` — runs the Tier-1 board unit suite (render/store + the
untouched config/cli/board/init/layer/task tests). The equivalence guardrail is the real
acceptance check: the post-fold `go test -list '.*' ./internal/board` + `=== RUN` subtest set
must be a **superset** of the pre-fold baseline, with every fold being a top-level→subtest
move that preserves assertions. State the fold list (which funcs collapsed into which
table-driven test) in the commit body so the reviewer can validate no distinct case was lost.
