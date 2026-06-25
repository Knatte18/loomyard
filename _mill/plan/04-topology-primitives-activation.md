# Batch: topology-primitives-activation

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: topology-primitives-activation
number: 4
cards: 4
verify: go build ./... && go test ./internal/warp/ ./internal/initcli/
depends-on: [3]
```

## Batch Scope

Introduce warp's two topology primitives and relocate junction wiring from `warp add` into `lyx init`. The **junction primitive** (`junction.go`) wires a junction + its `.git/info/exclude` entry atomically and cwd-keyed. The **drift primitive** (`drift.go`) is the stateless host-branch-vs-weft-branch in-sync check. `warp add` becomes **dormant** (creates the pairing, no junctions) and **adopts** an existing weft branch instead of aborting. `lyx init` becomes the activator: it wires the cwd-keyed junction(s) via warp's primitive *then* reconciles config, and reports "no weft pairing" when run on an unpaired host worktree.

External interface batches 5–8 consume: `warp.WireJunctions(l *paths.Layout, ...)` (or equivalent exported primitive) and `warp.PairInSync(l *paths.Layout) (bool, reason string, err error)`.

Batch-local decisions: the activation direction is strictly `initcli → warp` (warp never imports `initcli`/`configsync`). The drift primitive is stateless — it derives the weft sibling deterministically as `<base>-weft` and makes two `gitexec.RunGit` `rev-parse --abbrev-ref HEAD` calls (one in the host worktree, one in the weft sibling) plus a junction stat; no registry.

## Cards

### Card 11: Junction primitive — atomic, cwd-keyed wire

- **Context:**
  - `internal/warp/weftwiring.go`
  - `internal/paths/paths.go`
  - `internal/fslink/fslink.go`
  - `internal/fslink/fslink_windows.go`
- **Edits:**
  - `internal/warp/weftwiring.go`
- **Creates:**
  - `internal/warp/junction.go`
- **Deletes:** none
- **Requirements:** Create `internal/warp/junction.go` exporting a junction primitive (e.g. `func WireJunctions(l *paths.Layout, slug string) error`) that, for each entry in `l.HostJunctions(slug)`, creates the directory junction via `fslink.CreateDirLink` AND appends the junction `Name` to `.git/info/exclude` (resolved via `gitexec` `rev-parse --git-path info/exclude`) **atomically** — never the junction without the exclude entry. It must be idempotent (create-or-verify via `fslink.IsLink`/`fslink.PointsTo`; exclude append is line-exact idempotent) and cwd-keyed (operate on the host link for the given layout/subfolder, not assume the worktree root). Refactor the existing `seedLyxJunction` + `seedGitExclude` in `weftwiring.go` so both delegate to (or are replaced by) this primitive — no duplicated junction/exclude logic. Preserve the "refuse if host holds a real dir predating weft" guard.
- **Commit:** `feat(warp): atomic cwd-keyed junction primitive`

### Card 12: Make warp add dormant and adopt-or-create the weft branch

- **Context:**
  - `internal/warp/weftwiring.go`
  - `internal/warp/junction.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warp/add.go`
  - `internal/warp/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `Add` (`internal/warp/add.go`): (1) **dormant** — remove the `seedLyxJunction`/`seedGitExclude` calls so a fresh pairing has no junctions; update `rollbackAdd` so it no longer removes a junction `Add` never created (drop the `removeHostJunction` step from the create-rollback path). (2) **adopt-or-create** — replace the precheck that aborts when the weft branch already exists with adopt-or-create: if `weftBranchExists` is true, build the weft worktree from the existing branch (pass it as the start point / skip the `-b` create); otherwise create it from the parent's weft branch (existing fork-point logic). Update `add_test.go` to assert no junctions after `Add` and to cover the adopt-existing-weft-branch path (no longer an error). Keep host+weft branch creation, portal/launcher writes, and push behaviour intact.
- **Commit:** `feat(warp): dormant add; adopt-or-create weft branch`

### Card 13: Stateless drift / pair-in-sync primitive

- **Context:**
  - `internal/warp/weftwiring.go`
  - `internal/paths/paths.go`
  - `internal/fslink/fslink.go`
  - `internal/gitexec/gitexec.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/drift.go`
  - `internal/warp/drift_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/warp/drift.go` with `func PairInSync(l *paths.Layout) (ok bool, reason string, err error)`: derive the deterministic weft sibling (`<base>-weft` via `paths` geometry), run `gitexec` `rev-parse --abbrev-ref HEAD` in the host worktree and in the weft sibling, return not-ok with reason `"host on X, weft on Y"` when they differ; also stat the host `_lyx` junction (reuse the junction-resolution check) and return not-ok `"junction missing/points elsewhere"` when broken. Stateless — no registry, no status.md read. Add `drift_test.go` covering in-sync, branch-divergence, and broken-junction cases. (`drift_test.go` is created here; it is not in the overview All Files Touched list and the validator will reconcile it.)
- **Commit:** `feat(warp): stateless pair-in-sync drift primitive`

### Card 14: lyx init activates junctions then reconciles config

- **Context:**
  - `internal/warp/junction.go`
  - `internal/warp/drift.go`
  - `internal/paths/paths.go`
  - `internal/configsync/configsync.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/initcli/initcli.go` `RunInit`: before the `configsync.ReconcileAll(cwd, true)` call, invoke warp's junction primitive to wire the cwd-keyed junction(s) for the current worktree (junctions first, config second). If the host worktree has no weft sibling (dormant pairing absent), do **not** create topology — return a clear report ("no weft pairing — run `lyx warp add` / `lyx warp clone`") and stop before reconcile. Add the `internal/warp` import (direction `initcli → warp` only; warp must not import initcli). Update `initcli_test.go` to cover: activation wires the junction then reconciles; missing-pairing reports and does not reconcile. Confirm no import cycle (`go build ./...`).
- **Commit:** `feat(init): activate cwd-keyed junctions via warp, then reconcile`

## Batch Tests

`verify: go build ./... && go test ./internal/warp/ ./internal/initcli/`. `go test ./internal/warp/` runs the updated `add_test.go` (dormant + adopt), `drift_test.go`, and the unchanged moved suite (which must still pass — note any moved test that previously asserted junctions-after-add is updated in card 12 to reflect the dormant model and to call the junction primitive / init where it needs an active junction). `go test ./internal/initcli/` covers activation ordering and the missing-pairing report. `go build ./...` proves the `initcli → warp` edge introduces no cycle.
