Summary: All 6 cards of the guard-and-docs batch (batch 8, final batch) are complete and committed. Card-count self-check: 6 of 6 cards committed, matching the batch's declared 6 cards.

Files touched:
- `internal/lyxcwd/enforcement_test.go` -- collapsed `_lyx`'s transitional co-owner row to `internal/configengine` alone; documented why `.lyx` stays unpoliced.
- `docs/shared-libs/lyxcwd.md` (git mv from `hubgeometry.md`), `README.md`, `configengine.md`, `envsource.md` -- rewrote the module doc set.
- `docs/overview.md`, `manifest/designs/fabric-unified-view.md` -- rewrote the Hub Geometry Invariants section and the design doc's anchoring/`_board`/`weftname` claims.
- `docs/reference/plan-format.md`, `builder-contract.md`, `discussion-format.md`, `status-schema.md`, `model-spec.md` -- corrected constructor-ownership claims.
- `manifest/designs/loom.md`, `pattern.md` -- corrected remaining hubgeometry references.
- `manifest/roadmap.md` -- marked slice 7 shipped.

Verify passed: `go vet -tags "integration smoke scout" ./...` and `go test ./internal/lyxcwd/... ./cmd/lyx/...` both green. Repo-wide `internal/hubgeometry` sweep grep returned zero hits, confirming full ownership migration. All commits pushed to `fabric-illusion-core`.

{"status":"success","commit_sha":"5cba7d6fe357304320c60046c5270102c2cb7382","session_id":"9b67e3a0-213f-40ea-9ad7-340a7ac88468","cards_done":[41,42,43,44,45,46]}