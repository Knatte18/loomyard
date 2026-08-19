2 of 2 cards committed and verified: Card 11 (`feat(fabricengine): Merge — squash-capable target-pair merge with sync and self-abort`, commit `4f1e4fa0`) and Card 12 (`test(fabricengine): Merge target-pair scenarios`, commit `387e3599`). Batch verify (`go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabricengine/`) passed in full, including all new `TestMerge_*` target-verb integration tests. Working tree is clean.

Key files touched:
- `/home/knatte/Code/loomyard/wts/fabric-merge-conflict-primitive/internal/fabricengine/merge.go` — added `Fabric.Merge(source, opts)` and `syncSideBeforeMerge`.
- `/home/knatte/Code/loomyard/wts/fabric-merge-conflict-primitive/internal/fabricengine/mergeguards.go` — added `syncedToUpstreamReason` and `sideNotSyncedToUpstream`.
- `/home/knatte/Code/loomyard/wts/fabric-merge-conflict-primitive/internal/fabricengine/merge_target_integration_test.go` — new integration test file covering the full target-verb scenario matrix.

{"status":"success","commit_sha":"387e3599538908aa24372e5bf47dc24a54827fc1","session_id":"6e228a6c-6c32-4cb7-8c47-221328ee76e6","cards_done":[11,12]}
