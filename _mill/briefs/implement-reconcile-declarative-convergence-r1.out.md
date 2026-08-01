{"status":"success","commit_sha":"2fe80d7767ab69f1ca3a4481e8d5ad8a665425a2","session_id":"29c2822e-9cee-4a2a-bed7-0860ccbde7da","cards_done":[6,7,8,9,10]}

All 5 of 5 cards in batch `02-reconcile-declarative-convergence` are committed and verified — this is a genuine full completion, not a partial report.

Summary of work:

- **Card 6** (`internal/fabricengine/junctionnames.go`): added `repoWideFabricBase(l)` and `RepoWiredNames(l)`, the single named source for the repo-wide `weft:main` fabric config base at `hubgeometry.BoardDir(l.Hub)`.
- **Card 7** (`internal/fabricengine/reconcile.go`, `status.go` (no change needed), `hostlayout.go`, `drift.go`, `checkout.go`, `remove.go`): migrated `checkJunctionHealth`, `Reconcile`'s repoint load, `junctionRepointedDetail`, `PairInSync`, `Topology.Checkout`, and `Topology.Remove` to read the junction name-set via `RepoWiredNames` instead of each pair's own weft base; switched `hostLayoutFor`'s non-sibling fallback from `hubgeometry.Resolve` to the gate-free `hubgeometry.ResolveWorktree`.
- **Card 8** (`internal/fabricengine/reconcile.go`): added `scanOnDiskJunctionNames`, the read-only on-disk link-scan helper excluding `hubgeometry.HubReservedNames()`.
- **Card 9** (`internal/fabricengine/reconcile.go`): added `applyStaleRemoval` and `ReconcileActionStaleRemoved`; wired into `Reconcile`'s weft-exists branch only, fail-closed on config/scan errors, appends to (never overwrites) `pr.Detail`.
- **Card 10** (multiple `*_test.go` files in `internal/fabricengine/`): migrated shared fixtures (`newFabricFixture` + new `seedRepoWideFabricConfig` helper) to also seed the repo-wide config at `BoardDir`; fixed a latent bug found while doing this — `lyxtest.PairedFixture.Hub` is the worktree root, not `hubgeometry.Layout.Hub` (the parent), so seeding must use `fixture.Layout.Hub`; created `internal/fabricengine/reconcile_stale_removal_test.go` covering add-missing/stale-removed/no-op, cross-worktree convergence, fail-closed on unparseable config, reserved-name exclusion, and a repo-wide-only-config regression proving `PairInSync`/`Checkout`/`Remove` resolve from `BoardDir` alone.

Verify command `go test -tags integration ./internal/fabricengine/...` passes cleanly (fresh test cache). Final commit: `2fe80d7767ab69f1ca3a4481e8d5ad8a665425a2`.

{"status":"success","commit_sha":"2fe80d7767ab69f1ca3a4481e8d5ad8a665425a2","session_id":"29c2822e-9cee-4a2a-bed7-0860ccbde7da","cards_done":[6,7,8,9,10]}
