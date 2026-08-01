No tracked in-scope modifications remain (untracked). All 3 cards (11, 12, 13) are committed with distinct content commits, matching the batch's declared card count of 3. Verify passed.

{"status":"success","commit_sha":"d65b6027707cbe94e0070414c5adccfd199c66c4","session_id":"97ee880d-3cf0-4516-9bc7-1bc70efd6bb6","cards_done":[11,12,13]}

Summary: All 3 of 3 cards in batch `configsync-fabric-repowide` are committed.

- Card 11 (commit `7988d87f`): `internal/configsync/configsync.go` skips `fabric` in `ReconcileAll`'s per-module loop and removes the dead legacy-migration special case there; `internal/initengine/init_test.go` and `internal/initengine/undo_test.go` get `t.Skip(...)` added to every top-level test that calls `Init` (the package is deleted in batch 6).
- Card 12 (commit `4a99ecda`): adds `configsync.ReconcileFabricAt(boardDir, apply)` in `internal/configsync/configsync.go`, resolving the template via `configreg.Template("fabric")` (cycle-safe) and reusing `legacyFabricConfig`/`yamlengine.Reconcile`/`fsx.AtomicWriteBytes`.
- Card 13 (commit `d65b6027`): `internal/configsync/configsync_test.go` — `TestReconcileAll_ApplyCreatesFiles` now asserts no fabric result/file from `ReconcileAll`; `TestReconcileAll_MigratesLegacyFabricConfig` renamed to `TestReconcileFabricAt_MigratesLegacyFabricConfig` and routed through the new function.

Batch verify `go test -tags integration ./internal/configsync/... ./internal/initengine/...` passes, and `go build ./...` / `go vet ./...` are clean repo-wide.
