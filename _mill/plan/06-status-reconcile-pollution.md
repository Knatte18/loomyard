# Batch: status-reconcile-pollution

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: status-reconcile-pollution
number: 6
cards: 5
verify: go build ./... && go test ./internal/warp/ ./internal/weft/
depends-on: [5]
```

## Batch Scope

Deliver the paired-view `warp status`, the `warp reconcile` repair sweep, and the host-pollution detection guard; and trim the junction-integrity reporting out of `weft status` (it now lives in warp). `warp status` shows every host-WT ↔ weft-WT, its branch, in-sync verdict (via the drift primitive), junction health, and flags any `_lyx`/`_codeguide` path tracked in the host index. `warp reconcile` walks worktrees (never the whole branch namespace), repairs a missing weft worktree (adopt the branch), repairs a broken/dangling junction, adopts a raw (non-lyx) host worktree, and reports an unmanaged-branch worktree without touching it.

External interface batch 7 consumes: the pair-enumeration used by status (the host↔weft pair list) for `prune`/`cleanup`.

Batch-local decisions: reconcile **reports** (does not auto-adopt) on an unmanaged branch — the safer default; the contrast with `checkout` (which forks) is intentional. Host-pollution: for `_lyx` the guard offers `git rm --cached` + restore junction/exclude; for `_codeguide` it is **report-only** this task (warp wires no `_codeguide` junction yet — `paths.HostJunctions` returns only the `_lyx` entry).

## Cards

### Card 18: warp status — paired view + host-pollution detection

- **Context:**
  - `internal/warp/drift.go`
  - `internal/warp/list.go`
  - `internal/warp/weftwiring.go`
  - `internal/weft/status.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/status.go`
- **Requirements:** Create `internal/warp/status.go` with `func (w *Worktree) Status(l *paths.Layout) (StatusResult, error)` returning, per host worktree, the paired view: host path, weft path, host branch, weft branch, `in_sync` (via `PairInSync`), junction health (reason string), plus a host-pollution scan — detect any `_lyx`/`_codeguide` path tracked in the **host** index (`gitexec` `ls-files -- _lyx _codeguide` in the host worktree). For a tracked `_lyx` path, mark it pollutable with the remedy `git rm --cached` + restore junction/exclude; for `_codeguide`, mark it **report-only** (no junction to restore). Move the junction-integrity check semantics from `weft/status.go`'s `checkJunction` here (read it from Context; the actual removal from weft is card 20). Define `StatusResult` JSON-tagged for the paired list.
- **Commit:** `feat(warp): paired status view with host-pollution detection`

### Card 19: warp reconcile — repair and adopt the pairing

- **Context:**
  - `internal/warp/add.go`
  - `internal/warp/weftwiring.go`
  - `internal/warp/junction.go`
  - `internal/warp/drift.go`
  - `internal/warp/list.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/reconcile.go`
- **Requirements:** Create `internal/warp/reconcile.go` with `func (w *Worktree) Reconcile(l *paths.Layout) (ReconcileResult, error)` that walks host worktrees (`paths.List`): for each, if the weft worktree is missing but its branch exists, recreate the weft worktree (adopt the branch via the adopt-or-create helper); if the junction is broken/dangling, re-point it via the junction primitive; if the host worktree was created outside lyx (no weft sibling at all), adopt it — create the weft side (branch + worktree) — leaving it dormant (no junction wiring; that is `lyx init`'s job); if the host worktree is on an unmanaged branch with no weft sibling, **report** it ("run `lyx warp add` / `lyx init`") and touch nothing. Never adopt arbitrary branches — walk worktrees only. Return a `ReconcileResult` summarizing actions taken/reported (JSON-tagged).
- **Commit:** `feat(warp): reconcile repairs and adopts the host↔weft pairing`

### Card 20: Trim junction-integrity reporting out of weft status

- **Context:**
  - `internal/warp/status.go`
  - `internal/weft/cli.go`
- **Edits:**
  - `internal/weft/status.go`
  - `internal/weft/status_test.go`
- **Creates:** none
- **Requirements:** In `internal/weft/status.go`, remove the junction-integrity reporting now owned by warp: drop `checkJunction` and the `junction_ok`/`junction_reason` keys from the `Status` map (and the `hostLink`/`weftLyxDir` params that only served the junction check, if they become unused — adjust the `weft/cli.go` caller accordingly). Keep the content fields (`weft_worktree`, `branch`, `dirty`, `ahead`, `behind`). Update `weft/status_test.go` to drop the junction assertions and keep the content-status assertions. `weft` remains the content-sync owner; topology/junction reporting is warp's.
- **Commit:** `refactor(weft): drop junction reporting (moved to warp status)`

### Card 21: Route status and reconcile through RunCLI

- **Context:**
  - `internal/warp/status.go`
  - `internal/warp/reconcile.go`
  - `internal/warp/worktreelifecycle.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warp/warp.go`
- **Creates:** none
- **Requirements:** In `internal/warp/warp.go` `RunCLI`, add `case "status"` and `case "reconcile"`: resolve layout, `LoadConfig(cwd, "warp")`, `New(cfg)`, call `Status`/`Reconcile`, emit their results via `output.Ok`. Usage strings `usage: lyx warp status` / `usage: lyx warp reconcile`.
- **Commit:** `feat(warp): route lyx warp status and reconcile`

### Card 22: status and reconcile tests

- **Context:**
  - `internal/warp/status.go`
  - `internal/warp/reconcile.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/status_test.go`
  - `internal/warp/reconcile_test.go`
- **Requirements:** `status_test.go`: paired view fields populated; in-sync vs drifted reported; junction health reflected; a force-added `_lyx` path (`git add -f`) flagged with the `git rm --cached` remedy; a force-added `_codeguide` path flagged report-only. `reconcile_test.go`: missing weft worktree (branch present) recreated; broken junction re-pointed; raw (non-lyx) host worktree adopted (weft side created, dormant); unmanaged-branch worktree reported and untouched. Integration-tagged where real git is driven. Seed config via `warp.ConfigTemplate()` at the call site.
- **Commit:** `test(warp): status paired-view and reconcile repair/adopt/report`

## Batch Tests

`verify: go build ./... && go test ./internal/warp/ ./internal/weft/`. `status_test.go`/`reconcile_test.go` are the new TDD surface. `go test ./internal/weft/` confirms the trimmed `weft status` still passes its content assertions and the `weft/cli.go` caller still compiles. The rest of the `internal/warp` suite stays green.
