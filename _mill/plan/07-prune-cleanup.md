# Batch: prune-cleanup

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: prune-cleanup
number: 7
cards: 4
verify: go build ./... && go test ./internal/warp/
depends-on: [6]
```

## Batch Scope

Add the two destructive maintenance verbs. `warp prune` removes orphaned/stale pairs (new functionality — there was no `lyx worktree prune` before; the old `prune.go` was an empty-dir sweeper, now `ancestors.go`). `warp cleanup` deletes weft branches with no host sibling, guarded by an explicit flag matrix and a conservative `_codeguide` merge-back gate. Both default to dry-run/report; the board is never touched.

Batch-local decisions: cleanup flag matrix — **no flag** = dry-run/report; **`--apply`** = delete non-gate-protected orphans; **`--apply --force`** = additionally delete gate-protected task branches; `--force` requires `--apply` (does not imply it). The `codeguideFoldedBack(branch) bool` gate is wired in but, until codeguide merge-back exists, conservatively returns false for task branches (so they are protected unless `--force`); it is the extension point for the real check later.

## Cards

### Card 23: warp prune — remove orphaned/stale pairs

- **Context:**
  - `internal/warp/status.go`
  - `internal/warp/list.go`
  - `internal/warp/weftwiring.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/prune.go`
- **Requirements:** Create `internal/warp/prune.go` with `func (w *Worktree) Prune(l *paths.Layout, apply bool) (PruneResult, error)`: identify orphaned/stale pairs (a weft worktree whose host worktree is gone, or a registered worktree whose directory no longer exists) using the pair enumeration from `Status`/`paths.List`; with `apply == false` report what would be pruned; with `apply == true` remove the stale weft worktree(s) and run `git worktree prune` on both repos. Do not touch live pairs or the board. JSON-tagged `PruneResult`. This is a distinct file from `ancestors.go` (the empty-dir sweeper) — no symbol collision.
- **Commit:** `feat(warp): prune orphaned/stale host↔weft pairs`

### Card 24: warp cleanup — delete orphan weft branches, gated

- **Context:**
  - `internal/warp/list.go`
  - `internal/warp/status.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/cleanup.go`
- **Requirements:** Create `internal/warp/cleanup.go` with `func (w *Worktree) Cleanup(l *paths.Layout, apply, force bool) (CleanupResult, error)` and an unexported `codeguideFoldedBack(branch string) bool`. Enumerate weft branches with no host sibling (compare weft branch list against host worktree branches; the board repo is excluded entirely). Flag matrix: `apply == false` → report only; `apply == true && !force` → delete only non-gate-protected orphans (a task branch where `codeguideFoldedBack` is false is **skipped** and reported as protected); `apply == true && force` → also delete gate-protected task branches; `force && !apply` → report only (force does not imply apply). `codeguideFoldedBack` returns false for task branches until codeguide merge-back exists (conservative protect) — leave a clear comment marking it the extension point. JSON-tagged `CleanupResult` listing deleted/skipped/reported branches.
- **Commit:** `feat(warp): gated cleanup of orphan weft branches`

### Card 25: Route prune and cleanup through RunCLI

- **Context:**
  - `internal/warp/prune.go`
  - `internal/warp/cleanup.go`
  - `internal/warp/worktreelifecycle.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warp/warp.go`
- **Creates:** none
- **Requirements:** In `internal/warp/warp.go` `RunCLI`, add `case "prune"` (flag `--apply`) and `case "cleanup"` (flags `--apply`, `--force`), parsed via `flag.FlagSet`. Resolve layout, `LoadConfig(cwd, "warp")`, `New(cfg)`, call `Prune`/`Cleanup`, emit results via `output.Ok`. Usage strings document the flag matrix for cleanup.
- **Commit:** `feat(warp): route lyx warp prune and cleanup`

### Card 26: prune and cleanup tests

- **Context:**
  - `internal/warp/prune.go`
  - `internal/warp/cleanup.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/prune_test.go`
  - `internal/warp/cleanup_test.go`
- **Requirements:** `prune_test.go`: an orphaned/stale pair is reported in dry-run and removed under `--apply`; a live pair is never touched. `cleanup_test.go`: the full flag matrix — no flag reports only; `--apply` deletes a non-task orphan but skips a gate-protected task branch (reported protected); `--apply --force` deletes the task branch; `--force` alone reports only; the board repo/branch is never a deletion candidate. Integration-tagged where real git is driven. Seed config via `warp.ConfigTemplate()` at the call site.
- **Commit:** `test(warp): prune and cleanup flag-matrix and gate`

## Batch Tests

`verify: go build ./... && go test ./internal/warp/`. `prune_test.go`/`cleanup_test.go` are the new TDD surface; assert the destructive paths only trigger under the correct flags and the `_codeguide` gate protects task branches without `--force`. The rest of the `internal/warp` suite stays green.
