All four cards in this batch are committed and verified.

Card-count self-check: range start `5e3e98a6` (mill-go start-batch commit). Commits since then: `fb58fcd9` (card 7), `2da81ba0` (card 8), `67710d06` (card 9), `e7b67881` (card 10), plus a follow-up `4c9de407` that only renamed helpers/tests in the card-10 file for clarity (content-preserving, re-verified). 4 of 4 cards committed — all complete.

Summary of work:

- `internal/fabricengine/mergeguards.go` (new) — `resolveMergeSources`, `pairDirtyReason`, `upstreamSHAAt`, `mergeInProgressReason`.
- `internal/fabricengine/merge.go` (new) — `MergeOptions`, `MergeResult`, `Fabric.MergeIn`, `selfAbortMergeAttempt`.
- `internal/fabricengine/mergelifecycle.go` (new) — `MergeContinue`, `MergeAbort`, `MergeInProgress`, `concludeMergeSides`.
- `internal/fabricengine/mutation.go` — added `KindMergeStaged`/`KindMergeCommitted`, with same-commit guard updates in `mutation_test.go`, `livestate_mutationoracle_test.go`, and `cmd/lyx/destructiveguard_test.go`.
- `internal/fabricengine/mergein_integration_test.go` (new) — MergeIn scenario matrix (clean, conflicts, byte-identical warp/weft-conflict shape, fast-forward + abort, already-up-to-date variants, MergeContinue/MergeAbort, never-squashes).
- `internal/fabricengine/mergein_recovery_integration_test.go` (new) — crash recovery via a fresh Fabric handle, conclude-phase partial failure + idempotent retry, foreign-merge-state disposition across all four verbs, the freshness rule's three outcomes, fabric-managed guard, dirty-pair byte-identical error, unmappable-path self-abort, conflict-marker content, subpath-anchored path mapping.

Verify (`go test ./internal/fabricengine/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration -run Merge ./internal/fabricengine/`) passes in full at HEAD.

{"status":"success","commit_sha":"4c9de407fd11a0acc48ff283650c678560d0da3d","session_id":"0a9a62cc-74a5-45e3-b6ac-3d7ce37a3fb3","cards_done":[7,8,9,10]}
