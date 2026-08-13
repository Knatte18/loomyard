{"status":"success","commit_sha":"40b2e512dc035a1929087f2d1675af40ba06d217","session_id":"1c626ce1-d075-4322-8a56-01d0c04616a1","cards_done":[13,14,15,16,17]}

Summary — all 5 of 5 cards in batch `fabric-destroy-executors` are committed and verified:

- Card 13: `internal/fabricengine/destroy.go` — re-signatured `removeGitWorktree`, `deleteBranch`, `createGitWorktree` to return plain `error` (or `(createdToken, error)`), using `gitexec.Run`.
- Card 14: `internal/fabricengine/remove.go`, `internal/fabricengine/prune.go` — split the destructive fallback on `errors.As(err, &gitErr)` rather than a message merge.
- Card 15: `internal/fabricengine/weftwiring.go`, `internal/fabricengine/add.go` — collapsed unified `err != nil || exitCode != 0` sites to `err != nil`; migrated `Add`'s `createGitWorktree` call under the default merge rule.
- Card 16: `internal/fabricengine/cleanup.go`, `internal/fabricengine/checkout.go` — merged the `entry.Error` string-sink assignment to `%v`; simplified `rollbackSwitch`'s `deleteBranch` call.
- Card 17: `internal/fabricengine/export_test.go`, `internal/fabricengine/destructivegaps_integration_test.go` — `DeleteBranchForTest` now returns `error`; both consumer call sites updated.

Verify ran clean: `go build ./...`, `go test ./internal/fabricengine/...`, and `go test -tags integration ./internal/fabricengine/...` all passed, plus confirmed `cmd/lyx/destructiveguard_test.go`'s bypass/mutation-record guards still pass unaffected.
