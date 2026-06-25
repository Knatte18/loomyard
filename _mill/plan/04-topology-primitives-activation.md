# Batch: topology-primitives-activation

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: topology-primitives-activation
number: 4
cards: 4
verify: go build ./... && go test -tags integration ./internal/warp/ ./internal/initcli/ ./internal/configcli/
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
- **Requirements:** Create `internal/warp/junction.go` exporting a junction primitive (e.g. `func WireJunctions(l *paths.Layout, slug string) error`) that, for each entry in `l.HostJunctions(slug)`, creates the directory junction via `fslink.CreateDirLink` AND appends the junction `Name` to `.git/info/exclude` (resolved via `gitexec` `rev-parse --git-path info/exclude`) **atomically** — never the junction without the exclude entry. It must be idempotent (create-or-verify via `fslink.IsLink`/`fslink.PointsTo`; exclude append is line-exact idempotent) and cwd-keyed (operate on the host link for the given layout/subfolder, not assume the worktree root). Refactor the existing `seedLyxJunction` + `seedGitExclude` in `weftwiring.go` so both delegate to (or are replaced by) this primitive — no duplicated junction/exclude logic. Preserve the "refuse if host holds a real dir predating weft" guard. The primitive is **slug-keyed** via `l.HostJunctions(slug)`; the caller supplies the slug identifying the target worktree (e.g. the spawn slug for `add`; `filepath.Base(l.WorktreeRoot)` for the current worktree at `init` — see card 14).
- **Commit:** `feat(warp): atomic cwd-keyed junction primitive`

### Card 12: Make warp add dormant and adopt-or-create the weft branch

- **Context:**
  - `internal/warp/weftwiring.go`
  - `internal/warp/junction.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warp/add.go`
  - `internal/warp/add_test.go`
  - `internal/warp/weftwiring_test.go`
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `Add` (`internal/warp/add.go`): (1) **dormant** — remove the `seedLyxJunction`/`seedGitExclude` calls so a fresh pairing has no junctions; update `rollbackAdd` so it no longer removes a junction `Add` never created (drop the `removeHostJunction` step from the create-rollback path). Keep `removeHostJunction` itself — it is **retained**, still used by `Remove`/teardown (`remove.go`), so it does not become an unused symbol. (2) **adopt-or-create** — replace the precheck that aborts when the weft branch already exists with adopt-or-create: if `weftBranchExists` is true, build the weft worktree from the existing branch (pass it as the start point / skip the `-b` create); otherwise create it from the parent's weft branch (existing fork-point logic). Update `add_test.go` to assert no junctions after `Add` and to cover the adopt-existing-weft-branch path (no longer an error). Also update `internal/warp/weftwiring_test.go` (the moved `weft_test.go`) for the new behaviour: the junction-after-`Add` tests `TestWeftSpawnCreatesJunction`, `TestWeftSpawnSeedsExclude`, and `TestSeederParity` must be **retargeted** — `Add` no longer wires junctions, so either assert no junction/exclude after `Add` and exercise junction creation by calling the `WireJunctions` primitive directly (the junction-presence assertions now belong to the primitive/init activation path), or move those assertions into a `WireJunctions` test; the `TestWeftPrechecks` `RejectExistingWeftBranch` sub-case must change from "abort" to "adopt" (an existing weft branch is now adopted, not an error); and `TestWeftRollbackOnPostHostCreateFailure` must drop the expectation that rollback removes a host junction (there is none to remove after a dormant `Add`). Keep host+weft branch creation, portal/launcher writes, and push behaviour intact. Also update `internal/configcli/configcli_integration_test.go`: it currently relies on `warp.Add` wiring the `_lyx` junction to seed config through it — now that `Add` is dormant, after `warp.New().Add(...)` the test must wire the junction explicitly via warp's junction primitive (`WireJunctions(layout, slug)`) before exercising config edits, so the through-junction behaviour it tests still holds.
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
- **Requirements:** Create `internal/warp/drift.go` with `func PairInSync(l *paths.Layout) (ok bool, reason string, err error)`: derive the deterministic weft sibling (`<base>-weft` via `paths` geometry), run `gitexec` `rev-parse --abbrev-ref HEAD` in the host worktree and in the weft sibling, return not-ok with reason `"host on X, weft on Y"` when they differ; also stat the host `_lyx` junction (reuse the junction-resolution check) and return not-ok `"junction missing/points elsewhere"` when broken. Stateless — no registry, no status.md read. Add `drift_test.go` covering in-sync, branch-divergence, and broken-junction cases — integration-tagged (`//go:build integration`, mirroring `clone_integration_test.go`) since it drives real host+weft worktrees.
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
- **Requirements:** In `internal/initcli/initcli.go` `RunInit`: first obtain the layout via `l, err := paths.Resolve(cwd)` (today `RunInit` calls only `paths.Getwd()` and builds no `Layout`; the slug/sibling derivation below needs `l`). Then, before the `configsync.ReconcileAll(cwd, true)` call, invoke warp's junction primitive to wire the cwd-keyed junction(s) for the current worktree (junctions first, config second). **Slug source:** pass `filepath.Base(l.WorktreeRoot)` as the slug to `WireJunctions(l, slug)` — `HostJunctions(slug)` keys on `HostLyxLink(slug)` (`Hub/slug/RelPath/_lyx`), which resolves to the current worktree's host link (`HostLyxLinkHere`) precisely when `slug == filepath.Base(l.WorktreeRoot)`; deriving the slug this way (not from a hardcoded name) makes a renamed/subpath worktree wire the correct link. If the host worktree has no weft sibling (dormant pairing absent) — detected by `os.Stat(l.WeftWorktree())` not existing (equivalently warp's `weftRepoExists`-style sibling check) — do **not** create topology: return a clear report ("no weft pairing — run `lyx warp add` / `lyx warp clone`") and stop before reconcile. Add the `internal/warp` import (direction `initcli → warp` only; warp must not import initcli). Update `initcli_test.go`: **adapt the existing `TestRunInit_FirstRun` and `TestRunInit_Idempotent`** — they run `RunInit` in a bare `git init` tmpDir with no `<base>-weft` sibling, which now hits the no-pairing early-return and skips `configsync.ReconcileAll`, so the config files they assert would never be created; seed a `<base>-weft` weft-sibling pairing first (so the activation+reconcile path runs and their config-creation / idempotency assertions still hold). Add new cases covering: activation wires the junction then reconciles; and missing-pairing reports and does **not** reconcile (integration-tagged where these cases drive real git). Confirm no import cycle (`go build ./...`).
  **Contract note (intended, not a regression):** in the weft-overlay model `_lyx` lives in the weft repo and is reached through the junction, so `lyx init` is the *activator* run only inside a warp-hub worktree that already has a weft pairing (from `warp clone`/`add`). There is no standalone non-warp `lyx init` — scaffolding config into an unpaired host would write `_lyx` into the pristine host repo, violating the host-pristine invariant. The no-pairing path therefore **reports** ("run `warp add`/`clone`") instead of reconciling by design; this is the deliberate new contract, and the adapted `TestRunInit_*` tests (seeded with a weft sibling) encode it.
- **Commit:** `feat(init): activate cwd-keyed junctions via warp, then reconcile`

## Batch Tests

`verify: go build ./... && go test -tags integration ./internal/warp/ ./internal/initcli/ ./internal/configcli/`. The new real-git tests here — `drift_test.go` (which needs real host+weft worktrees on branches to exercise `PairInSync`) and the activation/missing-pairing cases in `initcli_test.go` — are **integration-tagged** (`//go:build integration`, mirroring `clone_integration_test.go`), so the `-tags integration` verify is required for them to run. The updated `add_test.go` (dormant + adopt) and the unchanged moved suite also run (untagged tests run under any tag set); any moved test that previously asserted junctions-after-add is updated in card 12 to reflect the dormant model and to call the junction primitive / init where it needs an active junction. `go build ./...` proves the `initcli → warp` edge introduces no cycle.
