# Batch: weft-hubgeometry-stderr-fix

```yaml
task: "CLI ergonomics from the sandbox run: config editor + warp error wrapping"
batch: weft-hubgeometry-stderr-fix
number: 3
cards: 2
verify: go test ./internal/weftengine/... ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

This batch fixes the remaining 2 raw-git-stderr-leak sites outside `internal/warpengine`
— one in `internal/weftengine/sync.go`, one in `internal/hubgeometry/worktreelist.go` —
per the same `## Shared Decisions` convention as `warpengine-stderr-fix`. It is its own
(small) batch rather than folded into `warpengine-stderr-fix` because these two files
belong to different packages with no shared local context beyond the convention itself;
grouping them here keeps `warpengine-stderr-fix` scoped to a single package and this
batch independently verifiable via
`go test ./internal/weftengine/... ./internal/hubgeometry/...`. Fully independent of the
other two batches — no dependency edges, no file overlap. No batch-local decisions beyond
`## Shared Decisions` in the overview.

## Cards

### Card 16: `weftengine/sync.go` — 1 site (push after rebase retry)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/weftengine/sync.go`
  - `internal/weftengine/sync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `pushUnpushed` (line ~160, reached after the rebase-retry loop's
  final attempt): replace `fmt.Errorf("push failed: %s", stderr)` with a message from
  `weftPath` and `code`, e.g.
  `fmt.Errorf("push from %q failed (git exit %d) after rebase retry", weftPath, code)`.
  Drop the now-fully-unused `stderr` binding per the same unused-variable rule used
  throughout `warpengine-stderr-fix` (replace with `_` in the `gitexec.RunGit` call if
  `stderr` is otherwise unused after this edit — note `stderr` is also read earlier in
  this same function by the `strings.Contains(stderr, "non-fast-forward")` branch, so it
  likely remains used; only blank it if the compiler reports it unused).
- **Tests:** Extend `TestPush` in `sync_test.go` (or add a small new test) forcing a push
  failure that survives the rebase retry (e.g. a genuine non-fast-forward conflict that
  a second `pull --rebase` cannot resolve), asserting the returned error contains no
  `"fatal:"` substring.
- **Commit:** `fix(weftengine): stop leaking git stderr in sync push error message`

### Card 17: `hubgeometry/worktreelist.go` — 1 site (worktree list)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/hubgeometry/worktreelist.go`
  - `internal/hubgeometry/worktreelist_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `List` (line ~40): replace
  `fmt.Errorf("git worktree list failed: %s", stderr)` with a message from `sourceDir` and
  `exitCode`, e.g.
  `fmt.Errorf("list git worktrees in %q failed (git exit %d)", sourceDir, exitCode)`. Drop
  the now-fully-unused `stderr` binding per the same rule as Card 16.
- **Tests:** Extend `TestList` in `worktreelist_test.go` (or add a small new test) calling
  `List` against a directory that is not a git worktree at all, forcing a non-zero exit,
  asserting the returned error contains no `"fatal:"` substring.
- **Commit:** `fix(hubgeometry): stop leaking git stderr in worktree list error message`

## Batch Tests

`verify: go test ./internal/weftengine/... ./internal/hubgeometry/...` runs both touched
test files (`sync_test.go`, `worktreelist_test.go`) plus each package's existing suite, so
both single-site fixes are covered by a direct, cheaply-reproducible test in the same
card that changes the site (no code-inspection-only fallback needed in this batch).
