All 4 cards (3, 4, 5, 6) of this batch are committed, verify passes end-to-end, and the working tree has no uncommitted tracked changes.

Summary of work:
- `internal/fabricengine/mergeerrors.go` + `mergeerrors_test.go` (card 3): the seven typed merge errors with pinned `Error()` strings, the closed guard-reason constant set, and `newMergeGuardError`.
- `internal/fabricengine/mergestate.go` + `mergestate_integration_test.go` (card 4, extended in card 6) + `export_test.go` additions + `cmd/lyx/destructiveguard_test.go` allowlist entry: the on-disk `mergeState` record with locked atomic-replace load/save/delete, `mergeRecordExists`, and `foreignMergeStatePresent`.
- `internal/fabricengine/mergepaths.go` + `mergepaths_test.go` (card 5): `resolveMergeGeometry` and `unifyConflictPaths`.
- `internal/fabricengine/destroy.go`, `destroy_test.go`, `reconcile.go`, `export_test.go` (card 6): `resetMergeSides`, the new `ownedWeftCheckout` ownership kind, and the `isWarpCheckout` → `isAnyWorktreeOf` rename (which required a plan-edit commit first to add `internal/fabricengine/reconcile.go` to the card's `Edits:` list, since it called the renamed function directly).

4 of 4 cards committed (verified against the batch's own `## Cards` `Commit:` messages), verify command passed (both `go test` sub-invocations), and no tracked uncommitted changes remain.

{"status":"success","commit_sha":"dae49ee04a43b5b98c29d9c9c92c687897635af9","session_id":"428f05ba-030f-4a96-8d26-0be762b893e9","cards_done":[3,4,5,6]}
