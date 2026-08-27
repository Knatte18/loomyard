All 6 of 6 cards in batch `02-weft-guards-drop.md` (Cards 8–13) were implemented and committed this turn:

1. **Card 8** — `abf92494`: `pairDirtyReason`/`detachedHeadReason` narrowed to warp-only in `internal/fabricengine/mergeguards.go`.
2. **Card 9** — `872c455c`: `syncedToUpstreamReason` narrowed to warp-only in the same file.
3. **Card 10** — `4b8630df`: `resolveMergeSources` drops its `*lyxcwd.Location` param and the `mergeReasonNotFabricManaged`/gated `mergeReasonSourceNotFound` refusal in `mergeguards.go`; `merge.go`'s two call sites updated, `Merge`'s dead `l`/`lyxcwd.ResolveWorktree` removed.
4. **Card 11** — `1c356424`: `resetMergeSides` drops its `weftSHA` param in `destroy.go`; `ownedWeftCheckout` and its doc comment deleted; all four call sites (`merge.go` x3, `mergelifecycle.go`'s `MergeAbort`) and `export_test.go`'s `ResetMergeSidesForTest` updated.
5. **Card 12** — `7f5bad69` (preceded by plan-extension commit `8125d7e0` after discovering the full-package verify surfaced pre-existing tests outside the plan's declared file list — `mergecrucible_integration_test.go`, `mergein_recovery_integration_test.go` — which I added to card 12's `Edits:` before touching them): inverted `TestMerge_FetchedDivergedWeftRefuses` -> `...DoesNotRefuse`, fixed `TestMerge_DirtyTargetHalts`, `wantWorktreeResetEntries`/`TestMergeState_ResetMergeSides_*` in `mergestate_integration_test.go`, `TestMergeCrucible_DetachedHeadRefused`'s weft case split out, and `mergein_recovery_integration_test.go`'s two guard-pinning tests split/inverted.
6. **Card 13** — `f728ad85`: new `internal/fabricengine/weftguards_integration_test.go` (4 scenario functions) plus `doc.go`'s guard narrative amended to warp-only with the "weft lost its power to block a merge" sentence.

Verify (`go build ./cmd/lyx && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...`) passes clean. Working tree has no uncommitted tracked changes. Card-count check: 6 of 6 batch cards committed — all complete.

{"status":"success","commit_sha":"f728ad85f41919219131e0430031061a9655e596","session_id":"10c24189-70a6-4c62-90b0-64f94e797ccc","cards_done":[8,9,10,11,12,13]}
