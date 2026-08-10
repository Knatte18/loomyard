{"status":"success","commit_sha":"c248d402fa9a0ab171a97a52bfeef8ac3a325469","session_id":"2c58d66a-c805-4f7d-b687-de9606e95776","cards_done":[1,2,3]}

All 3 of 3 cards in batch `01-dirtiness-probe.md` are committed:

1. `1a204476` — created `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/dirtiness.go` with `dirtyScope`, `scopeTracked`/`scopeAll`, and `worktreeDirty`.
2. `a6f3e765` — migrated the four tracked-only probe sites in `internal/fabricengine/add.go`, `checkout.go`, `prune.go`, `pull.go`.
3. `c248d402` — migrated the four untracked-inclusive probe sites in `internal/fabricengine/remove.go` (×2), `warpclean.go`, `reconcile.go`.

`verify: go test ./internal/fabricengine/...` passed. `git status --porcelain --untracked-files=no` is clean. No files outside the batch's declared scope were touched.

{"status":"success","commit_sha":"c248d402fa9a0ab171a97a52bfeef8ac3a325469","session_id":"2c58d66a-c805-4f7d-b687-de9606e95776","cards_done":[1,2,3]}
