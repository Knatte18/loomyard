All 5 of 5 cards in batch `03-snapshot-reader.md` are committed and pushed. Summary:

- Card 10 (`5ecc06aa`): generalized `scanWarpSHATrailers` in `internal/fabricengine/index.go` to capture `Snapshot` trailer values alongside `Warp-SHA`, added `--topo-order`, extracted the pure `parseTrailerScanRecord` helper, updated `RebuildIndex`'s ordering comment; added Tier-1 `internal/fabricengine/index_test.go`.
- Card 11 (`a5cb4e17`): added `internal/fabricengine/snapshot.go` with `Fabric.SnapshotWarpSHA`.
- Card 12 (`6e090a6e`): added `internal/fabricengine/snapshot_integration_test.go` with the 9 contract cases.
- Card 13 (`6c1a3a93`): extended that file with `TestSnapshotWarpSHA_TopologicalOrderBeatsCommitDate`, a back-dated-merge fixture proven (by temporarily reverting `--topo-order`) to actually discriminate the ordering change.
- Card 14 (`347decba`): documented the snapshot read path in `internal/fabricengine/doc.go`.

Batch verify (`go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./cmd/lyx/...`) passes. Note: had to invoke `go.exe` directly (`C:/Code/tools/go1.26.4.windows-amd64/go/bin/go.exe`) rather than the `go`/`go.cmd` wrapper on PATH, because passing a `-skip` regex containing `|` through the bash→`go.cmd` hop mis-splits on the pipe character in this environment; the underlying `go.exe` handles it fine.

One anomaly noted but deliberately left untouched (out of this batch's scope, `internal/fabricengine` only): `git status` shows unrelated tracked modifications to several `_mill/plan/*.md`, `_mill/discussion.md`, and `_mill/reviews/*.md` files (e.g. `03-snapshot-reader.md` shows "trace" example tags changed to "scout") that I never edited — apparent drift from another concurrent process. Per the "never edit files outside this batch's declared scope" rule I left these alone rather than staging/committing them.

{"status":"success","commit_sha":"347decba386178c55bd850c0b76e8022063e8d74","session_id":"859853ed-67ed-4f00-beb8-c39019e6ce6e","cards_done":[10,11,12,13,14]}
