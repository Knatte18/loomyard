# Batch: doc

```yaml
task: Prune and consolidate the test suite (board first)
batch: doc
number: 6
cards: 1
verify: null
depends-on: [1, 2, 3, 4, 5]
```

## Batch Scope

Document the prune. Append a count-focused history block to the timing doc once all five
package batches have landed (hence `depends-on: [1,2,3,4,5]`). This batch touches only a
markdown file — no runnable test surface, so `verify: null`. The block records the prune's
metric (top-level func count, not wall-clock) plus the equivalence guardrail (coverage
unchanged + folded/dropped name-map).

## Cards

### Card 17: Append count-focused history block to the timing doc

- **Context:**
  - `_mill/discussion.md`
  - `_mill/plan/baseline/board.txt`
  - `_mill/plan/baseline/worktree.txt`
  - `_mill/plan/baseline/weft.txt`
  - `_mill/plan/baseline/ide.txt`
  - `_mill/plan/baseline/muxpoc.txt`
- **Edits:**
  - `docs/benchmarks/test-suite-timing.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Append a new newest-first history block titled
  `### 2026-06-22 — after prune-board-tests` (place it above the existing
  `### 2026-06-22 — after optimize-remaining-test-suites` block, since History is
  append-only newest-first). Include: (1) a before/after **top-level func count** table
  per package — derive "before" from the line counts of the `_mill/plan/baseline/<pkg>.txt`
  files (board 61, worktree 22, weft 20, ide 20, muxpoc 19) and "after" by running
  `go test [-tags integration] ./internal/<pkg>/ -list '.*' | grep -c '^Test'` for each
  package (use `-tags integration` for worktree/weft/ide, default build for board/muxpoc);
  (2) the per-package statement coverage shown **unchanged / ≥ floor** (board 62.5%,
  worktree 68.6%, weft 64.6%, ide 75.4%, muxpoc 33.0%) by re-running `-cover`; (3) an
  "Equivalence guardrail" subsection with a **name-map**: derive removed names by diffing
  each baseline file against the current `-list` output, and for each removed name state
  the surviving `t.Run` subtest that now carries it or its drop justification. The known
  drops to document: board `TestRenderTaskStatus` (⊂ `TestRenderStatusVariants`),
  board `TestRemoveTask` (owned by `store_test.go:TestRemoveTaskMissing` + cli error
  table), board `TestUpsertTask` "update preserves fields" assertion (owned by
  `store_test.go:TestUpsertTaskPreservesFields`), worktree
  `TestWeftPrechecksHardRequireWeftRepo` (migrated into `add_test.go:TestAdd/NoWeftRepo`),
  weft `TestPullIntegration_FastForward` (⊂ `sync_test.go:TestPull_FastForward`), ide
  `TestSpawnColorSelection` (covered by `TestSpawnGeneratesConfig` +
  `vscode_test.go:TestWriteVSCodeConfigCreatesFilesWhenAbsent`), and ide
  `TestMenuZeroWorktreeMessage` if it was dropped rather than folded. Update the
  "Current best times" / headline narrative to note the count prune (wall-clock
  **unchanged** — do not claim a timing change). State explicitly that this prune relaxed
  the prior strict-superset guardrail to a justified subset/superset under a
  coverage-not-reduced check.
- **Commit:** `docs(benchmarks): record prune-board-tests count reduction`

## Batch Tests

`verify: null` — this batch edits only `docs/benchmarks/test-suite-timing.md`, which has
no runnable test surface. Correctness is checked by review: the count table must match the
actual post-prune `-list` counts, and the documented coverage numbers must be ≥ the floors
in the overview's coverage-guardrail decision. The implementer derives both by running the
`-list`/`-cover` commands named in Card 17 against the already-merged batches 1–5.
